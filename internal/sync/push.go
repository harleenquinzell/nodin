package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/harleenquinzell/nodin/internal/blockdiff"
	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/merge"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
)

// PushOptions configures the behaviour of a push operation.
type PushOptions struct {
	// PageID, if non-empty, restricts the push to the page with this Notion ID
	// or local path. Both forms are accepted.
	PageID string
}

// PushReport summarises the results of a push.
type PushReport struct {
	mu               sync.Mutex
	Pushed           int
	Created          int
	DatabasesCreated int
	Skipped          int
	Conflicts        int
	Pages            []string
	Databases        []string
	ConflictedPaths  []string
}

// Summary returns a one-line summary string.
func (r *PushReport) Summary() string {
	s := fmt.Sprintf("%d pushed, %d created, %d skipped, %d conflicts",
		r.Pushed, r.Created, r.Skipped, r.Conflicts)
	if r.DatabasesCreated > 0 {
		s += fmt.Sprintf(", %d databases created", r.DatabasesCreated)
	}
	return s
}

// Push reads locally modified pages and uploads the changes to Notion.
func Push(ctx context.Context, cfg *config.Config, store *state.Store, client *notion.Client, pushOpts PushOptions) (*PushReport, error) {
	if err := PreCommit(cfg.SyncDir, "push", cfg.AutoCommit); err != nil {
		return nil, fmt.Errorf("pre-push commit: %w", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	report := &PushReport{}
	opts := convert.PullOptions{AnchorRules: convert.DefaultAnchorRules()}

	// Keep the original ctx for work that runs after the errgroup; the errgroup
	// cancels its derived context as soon as Wait returns.
	parentCtx := ctx
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)

	for id, entry := range idx {
		if entry.Type != "page" {
			report.mu.Lock()
			report.Skipped++
			report.mu.Unlock()
			continue
		}

		// Apply --page filter: match by Notion ID or local path.
		if pushOpts.PageID != "" && id != pushOpts.PageID && entry.LocalPath != pushOpts.PageID {
			report.mu.Lock()
			report.Skipped++
			report.mu.Unlock()
			continue
		}

		id, entry := id, entry // capture

		absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			// File deleted locally; skip for now.
			continue
		}

		localContent := string(data)
		if checksum(localContent) == entry.Checksum {
			report.mu.Lock()
			report.Skipped++
			report.mu.Unlock()
			continue
		}
		if strings.Contains(localContent, "<<<<<<<") {
			report.mu.Lock()
			report.Conflicts++
			report.ConflictedPaths = append(report.ConflictedPaths, entry.LocalPath)
			report.mu.Unlock()
			continue
		}

		g.Go(func() error {
			if err := pushPage(ctx, cfg.SyncDir, store, client, opts, id, entry, localContent, report); err != nil {
				return fmt.Errorf("push page %s: %w", id, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return report, err
	}

	// Create new databases first so that any untracked .md files inside them
	// have a parent to attach to in the pushNewPages step below.
	// Skipped when --page filters to a Notion ID (no DB creation from an ID).
	if pushOpts.PageID == "" || strings.Contains(pushOpts.PageID, "/") {
		if err := pushNewDatabases(parentCtx, cfg, store, client, report); err != nil {
			return report, err
		}
	}

	// Create new pages for any untracked .md files. Skipped when --page filters
	// to a Notion ID (no creation possible from an ID), but allowed when --page
	// is a path to scope creation to that one file.
	if pushOpts.PageID == "" || strings.Contains(pushOpts.PageID, "/") {
		if err := pushNewPages(parentCtx, cfg, store, client, pushOpts, report); err != nil {
			return report, err
		}
	}

	if err := PostCommit(cfg.SyncDir, "push", report.Summary(), cfg.AutoCommit); err != nil {
		return report, fmt.Errorf("post-push commit: %w", err)
	}

	return report, nil
}

// pushNewPages walks the sync directory for .md files that have no index entry
// and creates them as new Notion pages or database entries.
func pushNewPages(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	pushOpts PushOptions,
	report *PushReport,
) error {
	// Re-read the index after the main push loop so that any rows we just wrote
	// (or that worker goroutines wrote) are visible here.
	idx, err := store.ReadIndex()
	if err != nil {
		return fmt.Errorf("read index for new pages: %w", err)
	}

	untracked, err := findUntrackedPages(cfg.SyncDir, idx)
	if err != nil {
		return fmt.Errorf("scan for untracked files: %w", err)
	}

	for _, localPath := range untracked {
		// Honour --page=<path> filter: only create the file the user pointed at.
		if pushOpts.PageID != "" && filepath.ToSlash(pushOpts.PageID) != localPath {
			continue
		}
		newID, err := createNewPage(ctx, cfg, store, client, idx, localPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", localPath, err)
		}
		// Refresh idx so subsequent files in the same push can resolve a parent
		// that was just created (e.g. parent page created earlier in this run).
		idx[newID] = state.IndexEntry{
			NotionID:  newID,
			LocalPath: localPath,
			Type:      "page",
		}
		report.mu.Lock()
		report.Created++
		report.Pages = append(report.Pages, localPath)
		report.mu.Unlock()
	}
	return nil
}

func pushPage(
	ctx context.Context,
	syncDir string,
	store *state.Store,
	client *notion.Client,
	opts convert.PullOptions,
	notionID string,
	entry state.IndexEntry,
	localContent string,
	report *PushReport,
) error {
	snapshot, err := store.ReadSnapshot(notionID)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	if snapshot == "" {
		return fmt.Errorf("no snapshot for %s; run 'nodin pull' first", notionID)
	}

	// Concurrent-edit guard: always fetch the current remote state and do a
	// three-way merge before pushing. We cannot rely on page.last_edited_time
	// to detect block-level changes — Notion does not propagate block edits to
	// the parent page's last_edited_time. The merge is a no-op when remote
	// matches the snapshot, so the only overhead is one extra GetBlocks call.
	remotePage, err := client.GetPage(ctx, notionID)
	if err != nil {
		return fmt.Errorf("get page: %w", err)
	}

	remoteBlocks, err := client.GetBlocks(ctx, notionID)
	if err != nil {
		return fmt.Errorf("get remote blocks: %w", err)
	}
	converted, err := convert.PullPage(*remotePage, remoteBlocks, opts)
	if err != nil {
		return fmt.Errorf("convert remote: %w", err)
	}
	remoteMD := converted.Frontmatter + converted.Body

	result, mergeErr := merge.ThreeWay(snapshot, localContent, remoteMD)
	if mergeErr != nil {
		// git unavailable; use local content as-is and risk overwriting remote changes.
	} else if result.Conflicts {
		absPath := filepath.Join(syncDir, entry.LocalPath)
		if err := writeFileAtomic(absPath, result.Content); err != nil {
			return fmt.Errorf("write conflict markers: %w", err)
		}
		report.mu.Lock()
		report.Conflicts++
		report.ConflictedPaths = append(report.ConflictedPaths, entry.LocalPath)
		report.mu.Unlock()
		return nil
	} else {
		localContent = result.Content
	}

	// Parse local markdown → blocks.
	localPage, localBlocks, err := convert.PushPage(localContent)
	if err != nil {
		return fmt.Errorf("parse local markdown: %w", err)
	}

	// Parse snapshot → blocks (last known Notion state).
	_, snapshotBlocks, err := convert.PushPage(snapshot)
	if err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}

	ops := blockdiff.Diff(snapshotBlocks, localBlocks)

	// Apply in order: Deletes → Updates → Inserts.
	if err := applyOps(ctx, client, notionID, ops); err != nil {
		return err
	}

	// Push title change if it differs from remote.
	if localTitle := localPage.Title(); localTitle != "" && localTitle != remotePage.Title() {
		if err := client.UpdatePage(ctx, notionID, localTitle); err != nil {
			return fmt.Errorf("update title: %w", err)
		}
	}

	// Push property changes for database entries.
	if remotePage.Parent.Type == "database_id" {
		if err := pushProperties(ctx, client, remotePage, localContent); err != nil {
			return fmt.Errorf("push properties: %w", err)
		}
	}

	if err := store.WriteSnapshot(notionID, localContent); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	if err := store.UpdateEntry(notionID, func(e state.IndexEntry) state.IndexEntry {
		e.Checksum = checksum(localContent)
		e.LastSync = remotePage.LastEditedTime
		return e
	}); err != nil {
		return fmt.Errorf("update index: %w", err)
	}

	report.mu.Lock()
	report.Pushed++
	report.Pages = append(report.Pages, entry.LocalPath)
	report.mu.Unlock()

	return nil
}

// pushProperties parses the frontmatter of localContent and pushes any editable
// property changes back to the Notion page. The database schema is fetched to
// provide type hints for YAML → PropertyValue conversion.
func pushProperties(ctx context.Context, client *notion.Client, page *notion.Page, localContent string) error {
	fm, _, err := convert.ParseFrontmatter(localContent)
	if err != nil || len(fm.Properties) == 0 {
		return err
	}

	db, err := client.GetDatabase(ctx, page.Parent.DatabaseID)
	if err != nil {
		return fmt.Errorf("get database schema: %w", err)
	}
	schema := db.Schema()

	// Pre-flight: reject property names that don't exist in the database schema.
	var unknown []string
	for name := range fm.Properties {
		if _, ok := schema[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("properties not in database schema: %s", strings.Join(unknown, ", "))
	}

	props, err := convert.YAMLToProperties(fm.Properties, fm.Computed, schema)
	if err != nil {
		return fmt.Errorf("parse properties: %w", err)
	}

	return client.UpdatePageProperties(ctx, page.ID, props)
}

// maxBlocksPerAppend is the Notion API limit for a single AppendBlocks call.
const maxBlocksPerAppend = 100

// applyOps applies a blockdiff op slice to Notion in the correct order:
// Deletes first (freeing IDs), then Updates (preserving IDs), then Inserts.
// Consecutive inserts that share the same AfterID are batched into a single
// AppendBlocks call (up to maxBlocksPerAppend at a time) to reduce API round-trips.
func applyOps(ctx context.Context, client *notion.Client, parentID string, ops []blockdiff.Op) error {
	for _, op := range ops {
		if op.Kind == blockdiff.Delete {
			if err := client.DeleteBlock(ctx, op.ID); err != nil {
				return fmt.Errorf("delete block %s: %w", op.ID, err)
			}
		}
	}
	for _, op := range ops {
		if op.Kind == blockdiff.Update {
			if err := client.UpdateBlock(ctx, op.ID, op.Block); err != nil {
				return fmt.Errorf("update block %s: %w", op.ID, err)
			}
		}
	}

	// Collect inserts in order, then batch consecutive ops sharing the same AfterID.
	var inserts []blockdiff.Op
	for _, op := range ops {
		if op.Kind == blockdiff.Insert {
			inserts = append(inserts, op)
		}
	}
	for i := 0; i < len(inserts); {
		afterID := inserts[i].AfterID
		j := i + 1
		for j < len(inserts) && inserts[j].AfterID == afterID {
			j++
		}
		// Send inserts[i:j] in chunks of maxBlocksPerAppend.
		currentAfterID := afterID
		for k := i; k < j; k += maxBlocksPerAppend {
			end := k + maxBlocksPerAppend
			if end > j {
				end = j
			}
			batch := make([]notion.Block, end-k)
			for n, op := range inserts[k:end] {
				batch[n] = op.Block
			}
			created, err := client.AppendBlocks(ctx, parentID, batch, currentAfterID)
			if err != nil {
				return fmt.Errorf("insert blocks: %w", err)
			}
			if len(created) > 0 {
				currentAfterID = created[len(created)-1].ID
			}
		}
		i = j
	}
	return nil
}
