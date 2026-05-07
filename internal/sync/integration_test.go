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

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

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
