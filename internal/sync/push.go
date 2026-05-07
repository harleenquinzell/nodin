package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/harleenquinzell/nodin/internal/blockdiff"
	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
)

// PushReport summarises the results of a push.
type PushReport struct {
	mu      sync.Mutex
	Pushed  int
	Skipped int
	Pages   []string
}

// Summary returns a one-line summary string.
func (r *PushReport) Summary() string {
	return fmt.Sprintf("%d pushed, %d skipped", r.Pushed, r.Skipped)
}

// Push reads locally modified pages and uploads the changes to Notion.
func Push(ctx context.Context, cfg *config.Config, store *state.Store, client *notion.Client) (*PushReport, error) {
	if err := PreCommit(cfg.SyncDir, "push", cfg.AutoCommit); err != nil {
		return nil, fmt.Errorf("pre-push commit: %w", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	report := &PushReport{}

	for id, entry := range idx {
		if entry.Type != "page" {
			continue
		}

		absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			// File deleted locally or unreadable; skip for now.
			continue
		}

		localContent := string(data)
		if checksum(localContent) == entry.Checksum {
			report.Skipped++
			continue
		}

		if err := pushPage(ctx, store, client, id, entry, localContent, report); err != nil {
			return report, fmt.Errorf("push page %s: %w", id, err)
		}
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
	notionID string,
	entry state.IndexEntry,
	localContent string,
	report *PushReport,
) error {
	snapshot, err := store.ReadSnapshot(notionID)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}

	// Parse local markdown → blocks.
	_, localBlocks, err := convert.PushPage(localContent)
	if err != nil {
		return fmt.Errorf("parse local markdown: %w", err)
	}

	// Parse snapshot → blocks (represents last known Notion state).
	// If there is no snapshot yet, treat it as an empty page.
	var snapshotBlocks []notion.Block
	if snapshot != "" {
		_, snapshotBlocks, err = convert.PushPage(snapshot)
		if err != nil {
			return fmt.Errorf("parse snapshot: %w", err)
		}
	}

	ops := blockdiff.Diff(snapshotBlocks, localBlocks)

	for _, op := range ops {
		switch op.Kind {
		case blockdiff.Update:
			if err := client.UpdateBlock(ctx, op.ID, op.Block); err != nil {
				return fmt.Errorf("update block %s: %w", op.ID, err)
			}
		case blockdiff.Insert:
			if _, err := client.AppendBlocks(ctx, notionID, []notion.Block{op.Block}, op.AfterID); err != nil {
				return fmt.Errorf("insert block: %w", err)
			}
		case blockdiff.Delete:
			if err := client.DeleteBlock(ctx, op.ID); err != nil {
				return fmt.Errorf("delete block %s: %w", op.ID, err)
			}
		}
	}

	if err := store.WriteSnapshot(notionID, localContent); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	if err := store.UpdateEntry(notionID, func(e state.IndexEntry) state.IndexEntry {
		e.Checksum = checksum(localContent)
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
