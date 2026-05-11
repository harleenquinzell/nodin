//go:build integration

package sync_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

// TestMain runs a sweeper before any test to archive nodin-test-* pages older
// than 10 minutes. These are left over from runs killed before t.Cleanup fired.
func TestMain(m *testing.M) {
	if token := os.Getenv("NODIN_TEST_TOKEN"); token != "" {
		sweepLeakedTestPages(token)
	}
	os.Exit(m.Run())
}

func sweepLeakedTestPages(token string) {
	client := notion.NewClient(token, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().Add(-10 * time.Minute)
	resp, err := client.Search(ctx, notion.SearchOpts{Query: "nodin-test-", Filter: "page"})
	if err != nil {
		return
	}
	for _, p := range resp.Results {
		if strings.HasPrefix(p.Title(), "nodin-test-") && p.CreatedTime.Before(cutoff) {
			_ = client.ArchivePage(ctx, p.ID)
		}
	}
}

// integrationSetup skips the test if the required env vars are absent and
// returns a ready-to-use client and a minimal config backed by a temp dir.
func integrationSetup(t *testing.T) (*notion.Client, *config.Config) {
	t.Helper()
	token := os.Getenv("NODIN_TEST_TOKEN")
	if token == "" {
		t.Skip("NODIN_TEST_TOKEN not set; skipping integration test")
	}
	rootID := os.Getenv("NODIN_TEST_PAGE_ID")
	if rootID == "" {
		t.Skip("NODIN_TEST_PAGE_ID not set; skipping integration test")
	}
	client := notion.NewClient(token, 3)
	cfg := &config.Config{
		Token:       token,
		RootPageID:  rootID,
		SyncDir:     t.TempDir(),
		RPS:         3,
		Concurrency: 1,
		AutoCommit:  false,
	}
	return client, cfg
}

// createTestPage creates a child page and registers a cleanup to archive it.
func createTestPage(t *testing.T, ctx context.Context, client *notion.Client, parentID, title string) *notion.Page {
	t.Helper()
	page, err := client.CreatePage(ctx, parentID, title)
	if err != nil {
		t.Fatalf("CreatePage %q: %v", title, err)
	}
	t.Cleanup(func() {
		_ = client.ArchivePage(context.Background(), page.ID)
	})
	return page
}

func randSuffix() string {
	return fmt.Sprintf("%d", rand.Int63n(1_000_000_000))
}

// TestIntegration_SyncPull verifies that Pull writes a markdown file for a
// known page and records it in the state index.
func TestIntegration_SyncPull(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	pullOpts := internalsync.PullOptions{PageID: cfg.RootPageID}
	report, err := internalsync.Pull(ctx, cfg, store, client, pullOpts)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if report.Pulled == 0 {
		t.Error("expected Pulled > 0")
	}

	// At least one .md file must exist on disk.
	var found bool
	_ = filepath.Walk(cfg.SyncDir, func(path string, _ os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".md") {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("no .md files written to sync dir after pull")
	}

	// The index must contain the page.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if _, ok := idx[cfg.RootPageID]; !ok {
		t.Errorf("page %s not found in index", cfg.RootPageID)
	}
}

// TestIntegration_SyncPushRoundTrip creates a fresh page, pulls it, appends a
// paragraph locally, pushes, then verifies the paragraph appears in Notion.
func TestIntegration_SyncPushRoundTrip(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	title := "nodin-test-roundtrip-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, title)

	// Seed the page with one block so there is something to diff against.
	seed := []notion.Block{{
		Type:    "paragraph",
		Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("original content")}},
	}}
	if _, err := client.AppendBlocks(ctx, testPage.ID, seed, ""); err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Pull the page.
	pullOpts := internalsync.PullOptions{PageID: testPage.ID}
	if _, err := internalsync.Pull(ctx, cfg, store, client, pullOpts); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Locate the written file via the index.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not found in index after pull")
	}
	absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)

	// Append a new paragraph to the local file.
	existing, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	const newParagraph = "New paragraph from integration test."
	if err := os.WriteFile(absPath, []byte(string(existing)+"\n"+newParagraph+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Push the change.
	pushOpts := internalsync.PushOptions{PageID: testPage.ID}
	report, err := internalsync.Push(ctx, cfg, store, client, pushOpts)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if report.Pushed == 0 {
		t.Error("expected Pushed > 0")
	}

	// Verify the new paragraph appears in Notion.
	blocks, err := client.GetBlocks(ctx, testPage.ID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	var found bool
	for _, b := range blocks {
		p, ok := b.Content.(*notion.ParagraphContent)
		if !ok {
			continue
		}
		for _, rt := range p.RichText {
			if strings.Contains(rt.PlainText, newParagraph) {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("pushed paragraph %q not found in Notion blocks", newParagraph)
	}
}

// TestIntegration_SyncTitleChange creates a fresh page, pulls it, renames it
// in the frontmatter, pushes, then verifies Notion reflects the new title.
func TestIntegration_SyncTitleChange(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	origTitle := "nodin-test-title-orig-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, origTitle)

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Pull the page.
	pullOpts := internalsync.PullOptions{PageID: testPage.ID}
	if _, err := internalsync.Pull(ctx, cfg, store, client, pullOpts); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Find the written file.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not found in index after pull")
	}
	absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Swap the title in the frontmatter.
	newTitle := "nodin-test-title-updated-" + randSuffix()
	updated := strings.Replace(string(content), origTitle, newTitle, 1)
	if updated == string(content) {
		t.Fatalf("original title %q not found in pulled file:\n%s", origTitle, content)
	}
	if err := os.WriteFile(absPath, []byte(updated), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Push the title change.
	pushOpts := internalsync.PushOptions{PageID: testPage.ID}
	if _, err := internalsync.Push(ctx, cfg, store, client, pushOpts); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify Notion has the new title.
	fetched, err := client.GetPage(ctx, testPage.ID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got := fetched.Title(); got != newTitle {
		t.Errorf("page title = %q, want %q", got, newTitle)
	}
}

// TestIntegration_Push_Conflict pulls a page, edits it locally, then edits the
// same block via the Notion API (simulating a concurrent remote change), and
// verifies that push writes conflict markers and reports a conflict.
func TestIntegration_Push_Conflict(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	title := "nodin-test-conflict-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, title)

	// Bold paragraph → ShouldAnchor=true → anchor written on pull → block ID
	// is tracked, so the conflict guard sees a real content change on both sides.
	seed := []notion.Block{{
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{
				notion.NewFormattedRichText("conflict base line", notion.Annotations{Bold: true}, ""),
			},
		},
	}}
	seededBlocks, err := client.AppendBlocks(ctx, testPage.ID, seed, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Pull so we have a snapshot and index entry with LastSync.
	if _, err := internalsync.Pull(ctx, cfg, store, client, internalsync.PullOptions{PageID: testPage.ID}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not in index after pull")
	}
	absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)

	// Edit local file: change the paragraph to a different value.
	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	localContent := strings.ReplaceAll(string(content), "conflict base line", "local version of the line")
	if err := os.WriteFile(absPath, []byte(localContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Update the same block via the Notion API to a third value (the "remote" edit).
	// The conflict guard no longer relies on page.last_edited_time (Notion does not
	// propagate block edits to the parent page's timestamp), so no sleep is needed.
	remoteBlock := notion.Block{
		ID:   seededBlocks[0].ID,
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{
				notion.NewFormattedRichText("remote version of the line", notion.Annotations{Bold: true}, ""),
			},
		},
	}
	if err := client.UpdateBlock(ctx, seededBlocks[0].ID, remoteBlock); err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}

	// Push: conflict guard fetches remote blocks, sees divergence, writes markers.
	report, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{PageID: testPage.ID})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if report.Conflicts == 0 {
		t.Error("expected Conflicts > 0, got 0")
	}

	// Conflict markers must be present in the local file.
	written, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile after push: %v", err)
	}
	if !strings.Contains(string(written), "<<<<<<<") {
		t.Errorf("conflict markers not written to file:\n%s", written)
	}
}

// TestIntegration_Push_BlockUpdate edits a pulled paragraph while keeping its
// anchor comment, pushes, and verifies the same block ID has the new text in
// Notion (i.e. the block was updated in place, not deleted and re-created).
func TestIntegration_Push_BlockUpdate(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	title := "nodin-test-blockupdate-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, title)

	// Bold paragraph: formatting triggers anchor emission on pull so the block
	// ID is embedded in the markdown and push can issue an Update, not Insert.
	seed := []notion.Block{{
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{
				notion.NewFormattedRichText("original bold text", notion.Annotations{Bold: true}, ""),
			},
		},
	}}
	seededBlocks, err := client.AppendBlocks(ctx, testPage.ID, seed, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}
	blockID := seededBlocks[0].ID

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Pull: file now contains <!--nid:blockID--> before the paragraph.
	if _, err := internalsync.Pull(ctx, cfg, store, client, internalsync.PullOptions{PageID: testPage.ID}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not in index after pull")
	}
	absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "<!--nid:"+blockID+"-->") {
		t.Skipf("block %s not anchored in pulled markdown; cannot verify ID preservation", blockID)
	}

	// Edit text while keeping the anchor so blockdiff emits Update, not Insert.
	newContent := strings.ReplaceAll(string(content), "original bold text", "updated bold text")
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pushReport, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{PageID: testPage.ID})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if pushReport.Pushed == 0 {
		t.Error("expected Pushed > 0")
	}

	// Verify: the same block ID now carries the updated text.
	blocks, err := client.GetBlocks(ctx, testPage.ID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	var found bool
	for _, b := range blocks {
		if b.ID != blockID {
			continue
		}
		if p, ok := b.Content.(*notion.ParagraphContent); ok {
			for _, rt := range p.RichText {
				if strings.Contains(rt.PlainText, "updated bold text") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("block %s not found or missing updated text after push", blockID)
	}
}

// TestIntegration_Push_Delete removes a block from the local file, pushes, and
// verifies the block is no longer present in Notion.
func TestIntegration_Push_Delete(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	title := "nodin-test-delete-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, title)

	// Two bold paragraphs: both get anchor comments on pull.
	seed := []notion.Block{
		{
			Type: "paragraph",
			Content: &notion.ParagraphContent{
				RichText: []notion.RichText{
					notion.NewFormattedRichText("keeper block", notion.Annotations{Bold: true}, ""),
				},
			},
		},
		{
			Type: "paragraph",
			Content: &notion.ParagraphContent{
				RichText: []notion.RichText{
					notion.NewFormattedRichText("block to delete", notion.Annotations{Bold: true}, ""),
				},
			},
		},
	}
	seededBlocks, err := client.AppendBlocks(ctx, testPage.ID, seed, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}
	deleteID := seededBlocks[1].ID

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	if _, err := internalsync.Pull(ctx, cfg, store, client, internalsync.PullOptions{PageID: testPage.ID}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not in index after pull")
	}
	absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "<!--nid:"+deleteID+"-->") {
		t.Skipf("block %s not anchored in pulled markdown; cannot test delete", deleteID)
	}

	// Remove the anchor and the immediately following line (the block content).
	anchor := "<!--nid:" + deleteID + "-->"
	lines := strings.Split(string(content), "\n")
	kept := make([]string, 0, len(lines))
	skipNext := false
	for _, line := range lines {
		if strings.TrimSpace(line) == anchor {
			skipNext = true
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(absPath, []byte(strings.Join(kept, "\n")), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pushReport, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{PageID: testPage.ID})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if pushReport.Pushed == 0 {
		t.Error("expected Pushed > 0")
	}

	// Verify the deleted block is gone from Notion.
	blocks, err := client.GetBlocks(ctx, testPage.ID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	for _, b := range blocks {
		if b.ID == deleteID {
			t.Errorf("block %s still present in Notion after delete push", deleteID)
		}
	}
}

// TestIntegration_Pull_ToggleBlocks creates a page containing a toggle block
// (with children) and a toggleable heading (with children appended after
// creation), then pulls and verifies both render with their child content.
//
// Note: programmatically-created toggles have has_children=true so the
// force-fetch path is not exercised here — that is covered by the mock-server
// unit tests. This test validates the full pull→convert pipeline end-to-end.
func TestIntegration_Pull_ToggleBlocks(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	title := "nodin-test-toggles-" + randSuffix()
	testPage := createTestPage(t, ctx, client, cfg.RootPageID, title)

	// Append a toggle block with one child paragraph inline.
	_, err := client.AppendBlocks(ctx, testPage.ID, []notion.Block{
		{
			Type: "toggle",
			Content: &notion.ToggleContent{
				RichText: []notion.RichText{notion.NewRichText("Toggle header")},
			},
			Children: []notion.Block{
				{
					Type:    "paragraph",
					Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("Toggle child content")}},
				},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("AppendBlocks (toggle): %v", err)
	}

	// Append a toggleable heading (H2 with is_toggleable=true).
	headings, err := client.AppendBlocks(ctx, testPage.ID, []notion.Block{
		{
			Type: "heading_2",
			Content: &notion.HeadingContent{
				RichText:     []notion.RichText{notion.NewRichText("Toggleable heading")},
				IsToggleable: true,
				Level:        2,
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("AppendBlocks (heading): %v", err)
	}

	// Append a child paragraph to the toggleable heading.
	if _, err := client.AppendBlocks(ctx, headings[0].ID, []notion.Block{
		{
			Type:    "paragraph",
			Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("Heading child content")}},
		},
	}, ""); err != nil {
		t.Fatalf("AppendBlocks (heading child): %v", err)
	}

	// Pull the page.
	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	if _, err := internalsync.Pull(ctx, cfg, store, client, internalsync.PullOptions{PageID: testPage.ID}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[testPage.ID]
	if !ok {
		t.Fatal("test page not in index after pull")
	}
	md, err := os.ReadFile(filepath.Join(cfg.SyncDir, entry.LocalPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(md)

	// Toggle block must render as <details>/<summary> with child text inside.
	for _, want := range []string{"<details>", "Toggle header", "Toggle child content"} {
		if !strings.Contains(body, want) {
			t.Errorf("toggle block: %q not found in pulled markdown:\n%s", want, body)
		}
	}

	// Toggleable heading must render with its child text below it.
	for _, want := range []string{"## Toggleable heading", "Heading child content"} {
		if !strings.Contains(body, want) {
			t.Errorf("toggleable heading: %q not found in pulled markdown:\n%s", want, body)
		}
	}
}

// TestIntegration_Push_CreateTopLevelPage creates a brand-new local file at
// pages/<slug>/<slug>.md, runs Push (no prior pull), and verifies that:
//   - Notion has a new page under RootPageID with the inferred title and body
//   - the index has an entry pointing at the local path
//   - the snapshot was written
func TestIntegration_Push_CreateTopLevelPage(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	slug := "nodin-test-create-" + randSuffix()
	dir := filepath.Join(cfg.SyncDir, "pages", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mdPath := filepath.Join(dir, slug+".md")
	body := "# " + slug + " title\n\nFirst paragraph from create test.\n"
	if err := os.WriteFile(mdPath, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	report, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if report.Created != 1 {
		t.Fatalf("Created = %d, want 1; report: %+v", report.Created, report)
	}

	// Locate the index entry for the new page.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	relPath := filepath.ToSlash(filepath.Join("pages", slug, slug+".md"))
	var newID string
	for id, e := range idx {
		if e.Type == "page" && filepath.ToSlash(e.LocalPath) == relPath {
			newID = id
			break
		}
	}
	if newID == "" {
		t.Fatalf("no index entry created for %s", relPath)
	}
	t.Cleanup(func() { _ = client.ArchivePage(context.Background(), newID) })

	// Verify Notion has the page with the right title.
	gotPage, err := client.GetPage(ctx, newID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	wantTitle := slug + " title" // first H1 wins over filename slug
	if gotPage.Title() != wantTitle {
		t.Errorf("page title = %q, want %q", gotPage.Title(), wantTitle)
	}
	if gotPage.Parent.Type != "page_id" || gotPage.Parent.PageID == "" {
		t.Errorf("parent = %+v; expected page_id parent", gotPage.Parent)
	}

	// Verify the body block was appended.
	gotBlocks, err := client.GetBlocks(ctx, newID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	var foundBody bool
	for _, b := range gotBlocks {
		if pc, ok := b.Content.(*notion.ParagraphContent); ok {
			for _, rt := range pc.RichText {
				if strings.Contains(rt.PlainText, "First paragraph from create test.") {
					foundBody = true
				}
			}
		}
	}
	if !foundBody {
		t.Errorf("body paragraph not found in created page's blocks")
	}

	// Verify snapshot was written.
	snap, err := store.ReadSnapshot(newID)
	if err != nil || snap == "" {
		t.Errorf("snapshot for %s: snap=%q err=%v", newID, snap, err)
	}
}

// TestIntegration_PushCreate_RoundTrip creates a local file, pushes (creating a
// new page), then edits it and pushes again to verify the second push goes
// through the regular update path (Pushed++, not Created++).
func TestIntegration_PushCreate_RoundTrip(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	slug := "nodin-test-roundtrip-" + randSuffix()
	dir := filepath.Join(cfg.SyncDir, "pages", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mdPath := filepath.Join(dir, slug+".md")
	if err := os.WriteFile(mdPath, []byte("# initial title\n\nfirst body\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	report1, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{})
	if err != nil {
		t.Fatalf("first Push: %v", err)
	}
	if report1.Created != 1 || report1.Pushed != 0 {
		t.Fatalf("first Push: Created=%d Pushed=%d, want 1/0", report1.Created, report1.Pushed)
	}

	idx, _ := store.ReadIndex()
	relPath := filepath.ToSlash(filepath.Join("pages", slug, slug+".md"))
	var newID string
	for id, e := range idx {
		if e.Type == "page" && filepath.ToSlash(e.LocalPath) == relPath {
			newID = id
		}
	}
	if newID == "" {
		t.Fatalf("page not in index after first push")
	}
	t.Cleanup(func() { _ = client.ArchivePage(context.Background(), newID) })

	// Edit the local file: append a second paragraph.
	existing, _ := os.ReadFile(mdPath)
	updated := string(existing) + "\nsecond paragraph appended\n"
	if err := os.WriteFile(mdPath, []byte(updated), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	report2, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{})
	if err != nil {
		t.Fatalf("second Push: %v", err)
	}
	if report2.Pushed != 1 || report2.Created != 0 || report2.Conflicts != 0 {
		t.Errorf("second Push: Pushed=%d Created=%d Conflicts=%d, want 1/0/0",
			report2.Pushed, report2.Created, report2.Conflicts)
	}

	// Verify Notion now has the second paragraph.
	blocks, err := client.GetBlocks(ctx, newID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	var foundSecond bool
	for _, b := range blocks {
		if pc, ok := b.Content.(*notion.ParagraphContent); ok {
			for _, rt := range pc.RichText {
				if strings.Contains(rt.PlainText, "second paragraph appended") {
					foundSecond = true
				}
			}
		}
	}
	if !foundSecond {
		t.Errorf("second paragraph not found in Notion after update push")
	}
}

// TestIntegration_PushCreate_PreservesPath verifies that after creating a page
// from a user-chosen filename, a subsequent pull writes back to the SAME file
// (not the canonical slug path), preventing duplicates.
func TestIntegration_PushCreate_PreservesPath(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Use a filename that won't match the canonical "<slug>-<shortID>" pattern.
	slug := "nodin-test-path-" + randSuffix()
	dir := filepath.Join(cfg.SyncDir, "pages", slug)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mdPath := filepath.Join(dir, slug+".md")
	if err := os.WriteFile(mdPath, []byte("# preserve path test\n\nbody.\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := internalsync.Push(ctx, cfg, store, client, internalsync.PushOptions{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	idx, _ := store.ReadIndex()
	relPath := filepath.ToSlash(filepath.Join("pages", slug, slug+".md"))
	var newID string
	for id, e := range idx {
		if e.Type == "page" && filepath.ToSlash(e.LocalPath) == relPath {
			newID = id
		}
	}
	if newID == "" {
		t.Fatalf("page not in index after push")
	}
	t.Cleanup(func() { _ = client.ArchivePage(context.Background(), newID) })

	// Pull this specific page; the file MUST still live at the user's path.
	if _, err := internalsync.Pull(ctx, cfg, store, client, internalsync.PullOptions{PageID: newID}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("user-chosen file %s missing after pull: %v", relPath, err)
	}

	// And no duplicate at the canonical slug path should exist.
	canonicalDir := slug + "-" + newID[:8]
	canonicalPath := filepath.Join(cfg.SyncDir, "pages", canonicalDir, canonicalDir+".md")
	if _, err := os.Stat(canonicalPath); err == nil {
		t.Errorf("duplicate canonical-path file written at %s; pull should respect existing index", canonicalPath)
	}
}

// TestIntegration_CreateDatabase exercises the new-db core path against the
// real Notion API: builds a rich schema, calls sync.CreateDatabase, then
// verifies the DB exists on Notion with the expected title + properties, that
// _schema.json round-trips, and that the index has a database-typed entry.
func TestIntegration_CreateDatabase(t *testing.T) {
	client, cfg := integrationSetup(t)
	ctx := context.Background()

	store := state.Open(cfg.SyncDir)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	title := "nodin-test-db-" + randSuffix()
	schema := internalsync.DatabaseSchema{
		Title: title,
		Properties: map[string]internalsync.PropertySpec{
			"Name":  {Type: "title"},
			"Notes": {Type: "rich_text"},
			"Status": {
				Type: "select",
				Options: []internalsync.SelectOption{
					{Name: "Todo", Color: "gray"},
					{Name: "Done", Color: "green"},
				},
			},
		},
	}

	db, err := internalsync.CreateDatabase(ctx, cfg, store, client, schema, "")
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	t.Cleanup(func() { _ = client.ArchiveDatabase(context.Background(), db.ID) })

	if db.TitleText() != title {
		t.Errorf("notion title = %q, want %q", db.TitleText(), title)
	}

	// Fetch the DB back from Notion and verify its property schema.
	fetched, err := client.GetDatabase(ctx, db.ID)
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	thinFromNotion := fetched.Schema() // omits "title"
	if thinFromNotion["Notes"] != "rich_text" {
		t.Errorf("Notion-side Notes type = %q, want rich_text", thinFromNotion["Notes"])
	}
	if thinFromNotion["Status"] != "select" {
		t.Errorf("Notion-side Status type = %q, want select", thinFromNotion["Status"])
	}

	// Index has the database entry at the canonical path.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[db.ID]
	if !ok {
		t.Fatalf("index missing entry for new database %s", db.ID)
	}
	if entry.Type != "database" {
		t.Errorf("entry.Type = %q, want database", entry.Type)
	}

	// _schema.json on disk round-trips the rich form.
	schemaPath := filepath.Join(cfg.SyncDir, entry.LocalPath, "_schema.json")
	onDisk, err := internalsync.ReadDatabaseSchema(schemaPath)
	if err != nil {
		t.Fatalf("ReadDatabaseSchema(%s): %v", schemaPath, err)
	}
	if onDisk.Title != title {
		t.Errorf("on-disk title = %q, want %q", onDisk.Title, title)
	}
	if got := onDisk.Properties["Status"]; len(got.Options) != 2 {
		t.Errorf("on-disk Status options = %+v, want 2 options", got.Options)
	}
}
