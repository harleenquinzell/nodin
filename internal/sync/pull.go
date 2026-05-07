package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/harleenquinzell/nodin/internal/assets"
	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/merge"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/pathmap"
	"github.com/harleenquinzell/nodin/internal/state"
)

// PullReport summarises the results of a pull.
type PullReport struct {
	mu        sync.Mutex
	Pulled    int
	Updated   int
	Conflicts int
	Pages     []string
}

// Summary returns a one-line summary string.
func (r *PullReport) Summary() string {
	return fmt.Sprintf("%d pulled, %d updated, %d conflicts", r.Pulled, r.Updated, r.Conflicts)
}

// Pull fetches pages updated since the last sync and writes them to disk.
func Pull(ctx context.Context, cfg *config.Config, store *state.Store, client *notion.Client) (*PullReport, error) {
	if err := PreCommit(cfg.SyncDir, "pull", cfg.AutoCommit); err != nil {
		return nil, fmt.Errorf("pre-pull commit: %w", err)
	}

	st, err := store.ReadState()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	pages, err := client.IncrementalPages(ctx, st.LastSync)
	if err != nil {
		return nil, fmt.Errorf("fetch pages: %w", err)
	}

	// Build an in-memory lookup for path resolution.
	pageMap := make(map[string]notion.Page, len(pages))
	for _, p := range pages {
		pageMap[p.ID] = p
	}
	lookup := func(id string) (notion.Page, bool) {
		p, ok := pageMap[id]
		return p, ok
	}

	opts := convert.PullOptions{
		AnchorRules:    convert.DefaultAnchorRules(),
		DownloadAssets: cfg.DownloadAssets,
	}

	report := &PullReport{}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)

	for _, page := range pages {
		page := page // capture
		g.Go(func() error {
			if err := pullPage(ctx, cfg, store, client, page, lookup, opts, report); err != nil {
				return fmt.Errorf("pull page %s: %w", page.ID, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return report, err
	}

	// Update LastSync to the minimum LastEditedTime in this batch to avoid skew.
	if len(pages) > 0 {
		minTime := pages[0].LastEditedTime
		for _, p := range pages[1:] {
			if p.LastEditedTime.Before(minTime) {
				minTime = p.LastEditedTime
			}
		}
		st.LastSync = minTime.Add(-time.Second)
		if err := store.WriteState(st); err != nil {
			return report, fmt.Errorf("write state: %w", err)
		}
	}

	if err := PostCommit(cfg.SyncDir, "pull", report.Summary(), cfg.AutoCommit); err != nil {
		return report, fmt.Errorf("post-pull commit: %w", err)
	}

	return report, nil
}

func pullPage(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	page notion.Page,
	lookup func(string) (notion.Page, bool),
	opts convert.PullOptions,
	report *PullReport,
) error {
	blocks, err := client.GetBlocks(ctx, page.ID)
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	converted, err := convert.PullPage(page, blocks, opts)
	if err != nil {
		return fmt.Errorf("convert page: %w", err)
	}

	localPath, err := pathmap.PagePath(page, lookup)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	absPath := filepath.Join(cfg.SyncDir, localPath)

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	body := converted.Frontmatter + converted.Body

	snapshot, err := store.ReadSnapshot(page.ID)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}

	if snapshot == "" {
		// First pull: write directly, no merge needed.
		if err := writeFileAtomic(absPath, body); err != nil {
			return err
		}
	} else {
		// Check if local file has been modified since last sync.
		existing, readErr := os.ReadFile(absPath)
		localUnchanged := readErr != nil || checksum(string(existing)) == checksum(snapshot)

		if localUnchanged {
			if err := writeFileAtomic(absPath, body); err != nil {
				return err
			}
		} else {
			// Three-way merge: base=snapshot, local=existing, remote=converted.
			result, mergeErr := merge.ThreeWay(snapshot, string(existing), body)
			if mergeErr != nil {
				// git unavailable or catastrophic failure; fall back to remote.
				if err := writeFileAtomic(absPath, body); err != nil {
					return err
				}
			} else {
				if err := writeFileAtomic(absPath, result.Content); err != nil {
					return err
				}
				if result.Conflicts {
					report.mu.Lock()
					report.Conflicts++
					report.mu.Unlock()
				}
			}
		}
	}

	// Write snapshot tracking the remote state.
	if err := store.WriteSnapshot(page.ID, body); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	// Download assets if enabled.
	if cfg.DownloadAssets && len(converted.AssetRefs) > 0 {
		httpClient := &http.Client{}
		for _, ref := range converted.AssetRefs {
			_, err := assets.Download(ctx, httpClient, ref.URL, ref.FileID, cfg.SyncDir)
			if err != nil {
				// Non-fatal: log and continue.
				_ = err
			}
		}
	}

	if err := store.UpdateEntry(page.ID, func(e state.IndexEntry) state.IndexEntry {
		e.NotionID = page.ID
		e.LocalPath = localPath
		e.Checksum = checksum(body)
		e.Type = "page"
		e.LastSync = page.LastEditedTime
		return e
	}); err != nil {
		return fmt.Errorf("update index: %w", err)
	}

	report.mu.Lock()
	report.Pulled++
	report.Pages = append(report.Pages, localPath)
	report.mu.Unlock()

	return nil
}

// writeFileAtomic writes content to path using a temp file + rename.
func writeFileAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// checksum returns the SHA-256 hex digest of s.
func checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
