//go:build integration

package notion_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// integrationClient returns a client from env vars, skipping if they are absent.
func integrationClient(t *testing.T) *notion.Client {
	t.Helper()
	token := os.Getenv("NODIN_TEST_TOKEN")
	if token == "" {
		t.Skip("NODIN_TEST_TOKEN not set; skipping integration test")
	}
	return notion.NewClient(token, 3)
}

// integrationPageID returns the test parent page ID, skipping if absent.
func integrationPageID(t *testing.T) string {
	t.Helper()
	id := os.Getenv("NODIN_TEST_PAGE_ID")
	if id == "" {
		t.Skip("NODIN_TEST_PAGE_ID not set; skipping integration test")
	}
	return id
}

// testPageTitle returns a unique page title for a test.
func testPageTitle(testName string) string {
	return fmt.Sprintf("nodin-test-%s-%d", testName, rand.Int63n(1e9))
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestIntegration_GetPage(t *testing.T) {
	client := integrationClient(t)
	pageID := integrationPageID(t)
	ctx := context.Background()

	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page.ID == "" {
		t.Error("page ID is empty")
	}
}

func TestIntegration_AppendAndDeleteBlocks(t *testing.T) {
	client := integrationClient(t)
	pageID := integrationPageID(t)
	ctx := context.Background()

	blocks := []notion.Block{
		{
			Type:    "paragraph",
			Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("integration test paragraph")}},
		},
	}

	created, err := client.AppendBlocks(ctx, pageID, blocks, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}
	if len(created) == 0 {
		t.Fatal("AppendBlocks returned empty slice")
	}

	t.Cleanup(func() {
		for _, b := range created {
			_ = client.DeleteBlock(context.Background(), b.ID)
		}
	})

	if created[0].ID == "" {
		t.Error("created block has empty ID")
	}
}

func TestIntegration_UpdateBlock(t *testing.T) {
	client := integrationClient(t)
	pageID := integrationPageID(t)
	ctx := context.Background()

	// Create a block to update.
	blocks := []notion.Block{
		{
			Type:    "paragraph",
			Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("original")}},
		},
	}
	created, err := client.AppendBlocks(ctx, pageID, blocks, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteBlock(context.Background(), created[0].ID)
	})

	updated := notion.Block{
		Type:    "paragraph",
		Content: &notion.ParagraphContent{RichText: []notion.RichText{notion.NewRichText("updated")}},
	}
	if err := client.UpdateBlock(ctx, created[0].ID, updated); err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}
}

func TestIntegration_IncrementalPages_ReturnsSomething(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()

	// Full pull — zero time means "all pages".
	pages, err := client.IncrementalPages(ctx, time.Time{})
	if err != nil {
		t.Fatalf("IncrementalPages: %v", err)
	}
	if len(pages) == 0 {
		t.Error("expected at least one page, got 0")
	}
}
