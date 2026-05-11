package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/pathmap"
	"github.com/harleenquinzell/nodin/internal/state"
)

// CreateDatabaseOptions configures CreateDatabase.
type CreateDatabaseOptions struct {
	// ParentPageID is the Notion page the new database lives under.
	// If empty, cfg.RootPageID is used.
	ParentPageID string

	// LocalPath overrides the on-disk location of the new database
	// (relative to cfg.SyncDir, e.g. "databases/tasks"). If empty, the
	// canonical "databases/<slug>-<shortID>/" path is used. Push passes
	// the user's existing directory here; new-db leaves it empty.
	LocalPath string
}

// CreateDatabase creates a new Notion database from the given schema and
// records it locally.
//
// On success:
//   - The database exists on Notion.
//   - <localPath>/_schema.json is written in the rich format (defaulting to
//     databases/<slug>-<shortID>/ when LocalPath is empty).
//   - The database is registered in the index as type="database".
//
// CreateDatabase is the shared core called by both nodin new-db (no LocalPath)
// and push (LocalPath = the user's untracked databases/<slug>/ folder).
func CreateDatabase(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	schema DatabaseSchema,
	opts CreateDatabaseOptions,
) (*notion.Database, error) {
	if err := ValidateSchema(schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	parent := opts.ParentPageID
	if parent == "" {
		parent = cfg.RootPageID
	}
	if parent == "" {
		return nil, fmt.Errorf("no parent page: pass ParentPageID or set root_page_id in config")
	}

	apiProps := schemaToAPIProperties(schema)

	db, err := client.CreateDatabase(ctx, parent, schema.Title, apiProps)
	if err != nil {
		return nil, fmt.Errorf("create database on notion: %w", err)
	}

	dbDirRel := opts.LocalPath
	if dbDirRel == "" {
		dbDirRel = pathmap.DatabasePath(db.AsPage())
	}
	dbDir := filepath.Join(cfg.SyncDir, dbDirRel)

	// If the directory exists, it must be one we already know about (e.g. the
	// user pre-created it before push). Hitting an unknown directory here
	// means the on-disk path collides with something the user authored —
	// surface that rather than overwriting.
	if opts.LocalPath == "" {
		if _, err := os.Stat(dbDir); err == nil {
			return db, fmt.Errorf("database created on notion (id %s) but local path %s already exists; resolve manually", db.ID, dbDirRel)
		}
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return db, fmt.Errorf("create %s: %w", dbDirRel, err)
	}

	schemaPath := filepath.Join(dbDir, "_schema.json")
	if err := WriteDatabaseSchema(schemaPath, schema); err != nil {
		return db, fmt.Errorf("write schema: %w", err)
	}

	if err := store.UpdateEntry(db.ID, func(e state.IndexEntry) state.IndexEntry {
		e.NotionID = db.ID
		e.LocalPath = dbDirRel
		e.Type = "database"
		e.LastSync = db.LastEditedTime
		return e
	}); err != nil {
		return db, fmt.Errorf("update index: %w", err)
	}

	return db, nil
}

// findUntrackedDatabases walks syncDir for "databases/<slug>/" directories
// that contain a _schema.json but are not yet recorded in the index.
// Returned paths are syncDir-relative with forward slashes.
func findUntrackedDatabases(syncDir string, idx map[string]state.IndexEntry) ([]string, error) {
	known := make(map[string]bool, len(idx))
	for _, e := range idx {
		if e.Type == "database" && e.LocalPath != "" {
			known[filepath.ToSlash(e.LocalPath)] = true
		}
	}

	databasesRoot := filepath.Join(syncDir, "databases")
	entries, err := os.ReadDir(databasesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var untracked []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("databases", name))
		if known[rel] {
			continue
		}
		schemaPath := filepath.Join(syncDir, rel, "_schema.json")
		if _, err := os.Stat(schemaPath); err != nil {
			continue
		}
		untracked = append(untracked, rel)
	}
	sort.Strings(untracked)
	return untracked, nil
}

// pushNewDatabases creates a Notion database for every untracked
// "databases/<slug>/" directory that has a _schema.json. It must run before
// pushNewPages so that any untracked entries inside the new databases can
// resolve their parent in the index.
func pushNewDatabases(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	report *PushReport,
) error {
	idx, err := store.ReadIndex()
	if err != nil {
		return fmt.Errorf("read index for new databases: %w", err)
	}

	dirs, err := findUntrackedDatabases(cfg.SyncDir, idx)
	if err != nil {
		return fmt.Errorf("scan for untracked databases: %w", err)
	}

	for _, localPath := range dirs {
		schemaPath := filepath.Join(cfg.SyncDir, localPath, "_schema.json")
		schema, err := ReadDatabaseSchema(schemaPath)
		if err != nil {
			return fmt.Errorf("read schema for %s: %w", localPath, err)
		}
		if _, err := CreateDatabase(ctx, cfg, store, client, schema, CreateDatabaseOptions{
			LocalPath: localPath,
		}); err != nil {
			return fmt.Errorf("create database %s: %w", localPath, err)
		}
		report.mu.Lock()
		report.DatabasesCreated++
		report.Databases = append(report.Databases, localPath)
		report.mu.Unlock()
	}
	return nil
}

// schemaToAPIProperties converts the local rich schema into the property-config
// map shape expected by the Notion "create database" endpoint.
func schemaToAPIProperties(s DatabaseSchema) map[string]any {
	props := make(map[string]any, len(s.Properties))
	for name, spec := range s.Properties {
		config := map[string]any{}
		switch spec.Type {
		case "select", "multi_select":
			opts := make([]map[string]any, 0, len(spec.Options))
			for _, o := range spec.Options {
				entry := map[string]any{"name": o.Name}
				if o.Color != "" {
					entry["color"] = o.Color
				}
				opts = append(opts, entry)
			}
			config["options"] = opts
		case "number":
			config["format"] = "number"
		}
		props[name] = map[string]any{spec.Type: config}
	}
	return props
}
