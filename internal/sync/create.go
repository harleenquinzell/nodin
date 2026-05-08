package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
)

// findUntrackedPages walks syncDir and returns the relative paths of .md files
// not present in the index. It skips the .nodin/ and .git/ directories, the
// _orphans/ tree, _schema.json files, and dotfiles.
func findUntrackedPages(syncDir string, idx map[string]state.IndexEntry) ([]string, error) {
	known := make(map[string]bool, len(idx))
	for _, e := range idx {
		if e.Type == "page" && e.LocalPath != "" {
			known[filepath.ToSlash(e.LocalPath)] = true
		}
	}

	var untracked []string
	walkErr := filepath.WalkDir(syncDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(syncDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		base := d.Name()
		if d.IsDir() {
			if base == ".nodin" || base == ".git" || base == "_orphans" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if base == "_schema.json" || strings.HasPrefix(base, ".") {
			return nil
		}
		if !strings.HasSuffix(base, ".md") {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if known[relSlash] {
			return nil
		}
		untracked = append(untracked, relSlash)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(untracked)
	return untracked, nil
}

// newPageParent describes how to create a new page on Notion: under which
// parent and (for database entries) with which schema.
type newPageParent struct {
	parentType string            // "page_id" or "database_id"
	parentID   string            // Notion ID of the parent page or database
	schema     map[string]string // database schema (nil for page parents)
}

// resolveNewPageParent infers the Notion parent for a new local file from its
// directory layout. Supported patterns:
//
//   - databases/<db-slug>/<file>.md          → entry of the database at <db-slug>
//   - pages/<slug>/<slug>.md                 → top-level page (parent = root)
//   - pages/.../<parent>/<slug>/<slug>.md    → child of the page at <parent>/<parent>.md
func resolveNewPageParent(syncDir, localPath string, idx map[string]state.IndexEntry, cfg *config.Config) (newPageParent, error) {
	rel := filepath.ToSlash(localPath)
	parts := strings.Split(rel, "/")

	// databases/<db-slug>/<file>.md
	if len(parts) == 3 && parts[0] == "databases" {
		dbDir := parts[0] + "/" + parts[1]
		var dbID string
		for _, e := range idx {
			if e.Type == "database" && filepath.ToSlash(e.LocalPath) == dbDir {
				dbID = e.NotionID
				break
			}
		}
		if dbID == "" {
			return newPageParent{}, fmt.Errorf("no database found at %s; run 'nodin pull' first to register the database", dbDir)
		}
		schema, err := readDatabaseSchema(filepath.Join(syncDir, dbDir, "_schema.json"))
		if err != nil {
			return newPageParent{}, fmt.Errorf("read schema for %s: %w", dbDir, err)
		}
		return newPageParent{parentType: "database_id", parentID: dbID, schema: schema}, nil
	}

	// pages/<slug>/<slug>.md → top-level under the configured root.
	// pages/.../<parent>/<slug>/<slug>.md → child of <parent>.
	if len(parts) >= 3 && parts[0] == "pages" {
		fileName := parts[len(parts)-1]
		dirName := parts[len(parts)-2]
		if fileName != dirName+".md" {
			return newPageParent{}, fmt.Errorf("path %s does not match the pages/<slug>/<slug>.md convention", rel)
		}

		// Top-level: pages/<slug>/<slug>.md (exactly 3 segments).
		if len(parts) == 3 {
			if cfg.RootPageID == "" {
				return newPageParent{}, fmt.Errorf("cannot create top-level page %s: no root_page_id configured", rel)
			}
			return newPageParent{parentType: "page_id", parentID: cfg.RootPageID}, nil
		}

		// Nested: parent file is <parent-dir>/<parent-dir>.md inside the same parent path.
		parentDirSlug := parts[len(parts)-3]
		parentFileRel := strings.Join(parts[:len(parts)-2], "/") + "/" + parentDirSlug + ".md"
		var parentID string
		for _, e := range idx {
			if e.Type == "page" && filepath.ToSlash(e.LocalPath) == parentFileRel {
				parentID = e.NotionID
				break
			}
		}
		if parentID == "" {
			return newPageParent{}, fmt.Errorf("parent page %s not found in index for new file %s", parentFileRel, rel)
		}
		return newPageParent{parentType: "page_id", parentID: parentID}, nil
	}

	return newPageParent{}, fmt.Errorf("unsupported path %s: place new pages under pages/<slug>/<slug>.md or databases/<db>/<entry>.md", rel)
}

// readDatabaseSchema reads property name → type from _schema.json.
func readDatabaseSchema(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var schema map[string]string
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return schema, nil
}

// inferTitle extracts a title for a brand-new page. Order of preference:
// frontmatter "title:", first "# H1" line in body, slug derived from filename.
func inferTitle(fm convert.Frontmatter, body, localPath string) string {
	if t := strings.TrimSpace(fm.Title); t != "" {
		return t
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
		break
	}
	base := filepath.Base(localPath)
	return strings.TrimSuffix(base, ".md")
}

// titlePropertyName returns the schema key whose type is "title", or "" if none.
func titlePropertyName(schema map[string]string) string {
	for name, typ := range schema {
		if typ == "title" {
			return name
		}
	}
	return ""
}

// createNewPage creates a Notion page for a new local file, appends its body as
// blocks, and records it in the index. localPath is relative to syncDir.
func createNewPage(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	idx map[string]state.IndexEntry,
	localPath string,
) (string, error) {
	absPath := filepath.Join(cfg.SyncDir, localPath)
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", localPath, err)
	}
	content := string(raw)

	parent, err := resolveNewPageParent(cfg.SyncDir, localPath, idx, cfg)
	if err != nil {
		return "", err
	}

	fm, body, err := convert.ParseFrontmatter(content)
	if err != nil {
		return "", fmt.Errorf("parse frontmatter for %s: %w", localPath, err)
	}
	title := inferTitle(fm, body, localPath)

	blocks, err := parseBodyBlocks(body)
	if err != nil {
		return "", fmt.Errorf("parse body of %s: %w", localPath, err)
	}

	var newPage *notion.Page
	switch parent.parentType {
	case "database_id":
		titleProp := titlePropertyName(parent.schema)
		if titleProp == "" {
			return "", fmt.Errorf("database %s has no title property in schema", parent.parentID)
		}
		props, err := convert.YAMLToProperties(fm.Properties, fm.Computed, parent.schema)
		if err != nil {
			return "", fmt.Errorf("convert frontmatter properties for %s: %w", localPath, err)
		}
		// Reject unknown property names up-front.
		var unknown []string
		for name := range fm.Properties {
			if _, ok := parent.schema[name]; !ok {
				unknown = append(unknown, name)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return "", fmt.Errorf("properties not in database schema: %s", strings.Join(unknown, ", "))
		}
		newPage, err = client.CreatePageInDatabase(ctx, parent.parentID, titleProp, title, props)
		if err != nil {
			return "", fmt.Errorf("create database entry: %w", err)
		}
	case "page_id":
		newPage, err = client.CreatePage(ctx, parent.parentID, title)
		if err != nil {
			return "", fmt.Errorf("create page: %w", err)
		}
	default:
		return "", fmt.Errorf("unknown parent type %q", parent.parentType)
	}

	if len(blocks) > 0 {
		if _, err := client.AppendBlocks(ctx, newPage.ID, blocks, ""); err != nil {
			return "", fmt.Errorf("append blocks for %s: %w", localPath, err)
		}
	}

	// Pull the page back so the snapshot and local file match what Notion will
	// return on subsequent reads (with frontmatter, anchor comments, etc.).
	// Without this, the next push would see a snapshot/remote mismatch and the
	// three-way merge would emit spurious conflict markers on the first edit.
	canonical, err := renderRemote(ctx, client, newPage.ID)
	if err != nil {
		return "", fmt.Errorf("render created page: %w", err)
	}
	if err := writeFileAtomic(absPath, canonical); err != nil {
		return "", fmt.Errorf("rewrite local file: %w", err)
	}
	if err := store.WriteSnapshot(newPage.ID, canonical); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	// Re-fetch the page to record an accurate LastSync after the block append.
	finalPage, err := client.GetPage(ctx, newPage.ID)
	if err != nil {
		return "", fmt.Errorf("refresh page: %w", err)
	}
	if err := store.UpdateEntry(newPage.ID, func(e state.IndexEntry) state.IndexEntry {
		e.NotionID = newPage.ID
		e.LocalPath = localPath
		e.Checksum = checksum(canonical)
		e.Type = "page"
		e.LastSync = finalPage.LastEditedTime
		return e
	}); err != nil {
		return "", fmt.Errorf("update index: %w", err)
	}

	return newPage.ID, nil
}

// renderRemote fetches a page and its blocks from Notion and returns the
// canonical local-markdown rendering (frontmatter + body).
func renderRemote(ctx context.Context, client *notion.Client, pageID string) (string, error) {
	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		return "", fmt.Errorf("get page: %w", err)
	}
	blocks, err := client.GetBlocks(ctx, pageID)
	if err != nil {
		return "", fmt.Errorf("get blocks: %w", err)
	}
	cp, err := convert.PullPage(*page, blocks, convert.PullOptions{
		AnchorRules: convert.DefaultAnchorRules(),
	})
	if err != nil {
		return "", fmt.Errorf("convert page: %w", err)
	}
	return cp.Frontmatter + cp.Body, nil
}

// parseBodyBlocks parses a markdown body (no frontmatter) into Notion blocks
// by reusing PushPage with empty frontmatter.
func parseBodyBlocks(body string) ([]notion.Block, error) {
	_, blocks, err := convert.PushPage(body)
	return blocks, err
}
