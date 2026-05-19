package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

// PullOptions configures the behaviour of a pull operation.
type PullOptions struct {
	// PageID, if non-empty, fetches only that specific page instead of using
	// the incremental cursor. Accepts both hyphenated and bare UUIDs.
	PageID string
	// Since, if non-zero, overrides the stored LastSync cursor for this run.
	// The stored cursor is not updated when this is set.
	Since time.Time
	// Progress, if set, is called after each page is successfully written to disk.
	// done is the number of pages completed so far; total is the full count for this run.
	Progress func(done, total int, localPath string)
}

// PullReport summarises the results of a pull.
type PullReport struct {
	mu        sync.Mutex
	Pulled    int
	Updated   int
	Conflicts int
	Removed   int
	Pages     []string
}

// Summary returns a one-line summary string.
func (r *PullReport) Summary() string {
	return fmt.Sprintf("%d pulled, %d updated, %d conflicts, %d removed", r.Pulled, r.Updated, r.Conflicts, r.Removed)
}

// Pull fetches pages updated since the last sync and writes them to disk.
func Pull(ctx context.Context, cfg *config.Config, store *state.Store, client *notion.Client, pullOpts PullOptions) (*PullReport, error) {
	if err := PreCommit(cfg.SyncDir, "pull", cfg.AutoCommit); err != nil {
		return nil, fmt.Errorf("pre-pull commit: %w", err)
	}

	st, err := store.ReadState()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	var pages []notion.Page
	if pullOpts.PageID != "" {
		// Single-page fetch: bypass the incremental cursor entirely.
		page, err := client.GetPage(ctx, pullOpts.PageID)
		if err != nil {
			return nil, fmt.Errorf("get page: %w", err)
		}
		pages = []notion.Page{*page}
	} else {
		cursor := st.LastSync
		if !pullOpts.Since.IsZero() {
			cursor = pullOpts.Since
		}
		pages, err = client.IncrementalPages(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("fetch pages: %w", err)
		}
	}

	// Build an in-memory lookup for path resolution.
	// Include database parents so database entries resolve to databases/ instead of _orphans/.
	pageMap := make(map[string]notion.Page, len(pages))
	for _, p := range pages {
		pageMap[p.ID] = p
	}

	// Collect unique database parent IDs from this batch.
	dbIDs := make(map[string]bool)
	for _, p := range pages {
		if p.Parent.Type == "database_id" {
			dbIDs[p.Parent.DatabaseID] = true
		}
	}
	// Fetch each database and add a Page-compatible entry so pathmap can resolve the slug.
	schemas := make(map[string]map[string]string) // db ID → property schema
	for dbID := range dbIDs {
		db, err := client.GetDatabase(ctx, dbID)
		if err != nil {
			continue // treat entries as orphans if database is inaccessible
		}
		pageMap[dbID] = db.AsPage()
		schemas[dbID] = db.Schema()
	}

	lookup := func(id string) (notion.Page, bool) {
		p, ok := pageMap[id]
		return p, ok
	}

	// Write _schema.json for each database directory and record the database in
	// the index. The index entry lets push resolve the database ID when creating
	// new entries in that directory.
	//
	// We read the index before the loop so that previously-recorded LocalPath
	// values are preserved — both for DBs the user created locally (via push or
	// new-db) and for DBs that were pulled to a now-renamed path. Without this
	// check, every pull would re-derive the canonical "<slug>-<shortID>" path
	// and silently duplicate user-chosen directories.
	existingIdx, err := store.ReadIndex()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	for dbID, schema := range schemas {
		dbPage, ok := pageMap[dbID]
		if !ok {
			continue
		}
		dbDirRel := pathmap.DatabasePath(dbPage)
		if e, ok := existingIdx[dbID]; ok && e.LocalPath != "" {
			dbDirRel = e.LocalPath
		}
		dbDir := filepath.Join(cfg.SyncDir, dbDirRel)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			continue
		}
		schemaPath := filepath.Join(dbDir, "_schema.json")
		if err := writeJSONFile(schemaPath, schema); err != nil {
			continue
		}
		_ = store.UpdateEntry(dbID, func(e state.IndexEntry) state.IndexEntry {
			e.NotionID = dbID
			e.LocalPath = dbDirRel
			e.Type = "database"
			e.LastSync = dbPage.LastEditedTime
			return e
		})
	}

	opts := convert.PullOptions{
		AnchorRules:    convert.DefaultAnchorRules(),
		DownloadAssets: cfg.DownloadAssets,
	}

	idx, err := store.ReadIndex()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	report := &PullReport{}

	total := len(pages)
	var doneCount atomic.Int32
	notifyProgress := func(localPath string) {
		if pullOpts.Progress != nil {
			n := int(doneCount.Add(1))
			pullOpts.Progress(n, total, localPath)
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrency)

	for _, page := range pages {
		page := page // capture
		g.Go(func() error {
			if err := pullPage(ctx, cfg, store, client, page, lookup, idx, opts, report, notifyProgress); err != nil {
				return fmt.Errorf("pull page %s: %w", page.ID, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return report, err
	}

	// Prune local files for pages that have been removed or moved out of scope.
	// Skip for single-page fetches — we only saw one page, can't infer removals.
	if pullOpts.PageID == "" {
		returnedIDs := make(map[string]bool, len(pages))
		for _, p := range pages {
			returnedIDs[p.ID] = true
		}
		isFullPull := st.LastSync.IsZero() && pullOpts.Since.IsZero()
		currentIdx, idxErr := store.ReadIndex()
		if idxErr == nil {
			for id, entry := range currentIdx {
				if entry.Type != "page" || returnedIDs[id] {
					continue
				}
				shouldPrune := false
				if isFullPull {
					// Full pull returned every accessible page — this one is gone.
					shouldPrune = true
				} else {
					// Incremental: verify directly with the API.
					p, err := client.GetPage(ctx, id)
					if errors.Is(err, notion.ErrNotFound) || (err == nil && p.Archived) {
						shouldPrune = true
					}
				}
				if !shouldPrune {
					continue
				}
				_ = os.Remove(filepath.Join(cfg.SyncDir, entry.LocalPath))
				_ = store.DeleteSnapshot(id)
				if store.DeleteEntry(id) == nil {
					report.mu.Lock()
					report.Removed++
					report.mu.Unlock()
				}
			}
		}
	}

	// Update LastSync to the minimum LastEditedTime in this batch to avoid skew.
	// Skip when --page or --since overrides were used; those runs are not full syncs.
	if len(pages) > 0 && pullOpts.PageID == "" && pullOpts.Since.IsZero() {
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
	idx map[string]state.IndexEntry,
	opts convert.PullOptions,
	report *PullReport,
	notify func(localPath string),
) error {
	blocks, err := client.GetBlocks(ctx, page.ID)
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	converted, err := convert.PullPage(page, blocks, opts)
	if err != nil {
		return fmt.Errorf("convert page: %w", err)
	}

	// Prefer an existing index entry's path so pages created locally (with a
	// user-chosen filename) keep that filename across pulls instead of getting
	// duplicated under their canonical slug.
	var localPath string
	if e, ok := idx[page.ID]; ok && e.LocalPath != "" && e.Type == "page" {
		localPath = e.LocalPath
	}
	if localPath == "" {
		localPath, err = pathmap.PagePath(page, lookup)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}
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

	isNew := snapshot == ""

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
			if _, err := assets.Download(ctx, httpClient, ref.URL, ref.FileID, cfg.SyncDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: download asset %s: %v\n", ref.FileID, err)
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
	if isNew {
		report.Pulled++
	} else {
		report.Updated++
	}
	report.Pages = append(report.Pages, localPath)
	report.mu.Unlock()

	if notify != nil {
		notify(localPath)
	}
	return nil
}

// writeJSONFile atomically writes v as indented JSON to path.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	return writeFileAtomic(path, string(data)+"\n")
}

// writeFileAtomic writes content to path using a temp file + rename.
func writeFileAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// checksum returns the SHA-256 hex digest of s.
func checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
