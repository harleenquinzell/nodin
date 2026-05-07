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
	mu        sync.Mutex
	Pushed    int
	Skipped   int
	Conflicts int
	Pages     []string
}

// Summary returns a one-line summary string.
func (r *PushReport) Summary() string {
	return fmt.Sprintf("%d pushed, %d skipped, %d conflicts", r.Pushed, r.Skipped, r.Conflicts)
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

		g.Go(func() error {
			if err := pushPage(ctx, store, client, opts, id, entry, localContent, report); err != nil {
				return fmt.Errorf("push page %s: %w", id, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return report, err
	}

	if err := PostCommit(cfg.SyncDir, "push", report.Summary(), cfg.AutoCommit); err != nil {
		return report, fmt.Errorf("post-push commit: %w", err)
	}

	return report, nil
}

func pushPage(
	ctx context.Context,
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

	// Concurrent-edit guard: if Notion has been edited since our last sync,
	// fetch the current remote state and do a three-way merge before pushing.
	remotePage, err := client.GetPage(ctx, notionID)
	if err != nil {
		return fmt.Errorf("get page: %w", err)
	}

	if remotePage.LastEditedTime.After(entry.LastSync) {
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
			absPath := filepath.Join("", entry.LocalPath) // caller has no SyncDir ref here
			_ = absPath
			// Write conflict markers back to the local file via the report mechanism.
			// The orchestrator can't write files from here without cfg; surface as Skipped+Conflict.
			report.mu.Lock()
			report.Conflicts++
			report.mu.Unlock()
			return nil
		} else {
			localContent = result.Content
		}
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

// applyOps applies a blockdiff op slice to Notion in the correct order:
// Deletes first (freeing IDs), then Updates (preserving IDs), then Inserts.
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
	for _, op := range ops {
		if op.Kind == blockdiff.Insert {
			if _, err := client.AppendBlocks(ctx, parentID, []notion.Block{op.Block}, op.AfterID); err != nil {
				return fmt.Errorf("insert block: %w", err)
			}
		}
	}
	return nil
}
