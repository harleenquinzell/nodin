package notion_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// blockListResponse wraps a slice of raw block JSON in a Notion list response.
func blockListResponse(blocks ...string) string {
	return fmt.Sprintf(`{"object":"list","results":[%s],"next_cursor":"","has_more":false}`,
		strings.Join(blocks, ","))
}

const toggleBlockClosed = `{
	"id": "toggle-001",
	"type": "toggle",
	"has_children": false,
	"toggle": {
		"rich_text": [{"type":"text","plain_text":"My toggle","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"text":{"content":"My toggle"}}],
		"color": "default"
	}
}`

const paragraphChild = `{
	"id": "para-001",
	"type": "paragraph",
	"has_children": false,
	"paragraph": {
		"rich_text": [{"type":"text","plain_text":"Hidden content","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"text":{"content":"Hidden content"}}],
		"color": "default"
	}
}`

// TestGetBlocks_ClosedToggleForceFetch verifies that GetBlocks fetches children
// for a toggle block even when has_children is false, because Notion can return
// an inaccurate has_children for collapsed toggles.
func TestGetBlocks_ClosedToggleForceFetch(t *testing.T) {
	var toggleChildrenFetched atomic.Bool

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "page-001/children"):
			fmt.Fprint(w, blockListResponse(toggleBlockClosed))
		case strings.HasSuffix(r.URL.Path, "toggle-001/children"):
			toggleChildrenFetched.Store(true)
			fmt.Fprint(w, blockListResponse(paragraphChild))
		default:
			http.NotFound(w, r)
		}
	}))

	blocks, err := client.GetBlocks(context.Background(), "page-001")
	if err != nil {
		t.Fatal(err)
	}
	if !toggleChildrenFetched.Load() {
		t.Error("children endpoint was not called for closed toggle block")
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d top-level blocks, want 1", len(blocks))
	}
	if !blocks[0].HasChildren {
		t.Error("HasChildren should be true after children were fetched")
	}
	if len(blocks[0].Children) != 1 {
		t.Errorf("got %d children, want 1", len(blocks[0].Children))
	}
}

const toggleableHeadingClosed = `{
	"id": "heading-001",
	"type": "heading_2",
	"has_children": false,
	"heading_2": {
		"rich_text": [{"type":"text","plain_text":"Collapsible section","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"text":{"content":"Collapsible section"}}],
		"color": "default",
		"is_toggleable": true
	}
}`

// TestGetBlocks_ToggleableHeadingForceFetch verifies the same force-fetch for
// headings with is_toggleable: true.
func TestGetBlocks_ToggleableHeadingForceFetch(t *testing.T) {
	var headingChildrenFetched atomic.Bool

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "page-002/children"):
			fmt.Fprint(w, blockListResponse(toggleableHeadingClosed))
		case strings.HasSuffix(r.URL.Path, "heading-001/children"):
			headingChildrenFetched.Store(true)
			fmt.Fprint(w, blockListResponse(paragraphChild))
		default:
			http.NotFound(w, r)
		}
	}))

	blocks, err := client.GetBlocks(context.Background(), "page-002")
	if err != nil {
		t.Fatal(err)
	}
	if !headingChildrenFetched.Load() {
		t.Error("children endpoint was not called for toggleable heading")
	}
	if len(blocks[0].Children) != 1 {
		t.Errorf("got %d children, want 1", len(blocks[0].Children))
	}
}

// TestGetBlocks_NonToggleSkipped verifies that a plain block with has_children:false
// does NOT trigger an extra fetch.
func TestGetBlocks_NonToggleSkipped(t *testing.T) {
	const plainParagraph = `{
		"id": "para-top",
		"type": "paragraph",
		"has_children": false,
		"paragraph": {
			"rich_text": [{"type":"text","plain_text":"Plain text","annotations":{"bold":false,"italic":false,"strikethrough":false,"underline":false,"code":false,"color":"default"},"text":{"content":"Plain text"}}],
			"color": "default"
		}
	}`

	var extraCalls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "page-003/children") {
			fmt.Fprint(w, blockListResponse(plainParagraph))
		} else {
			extraCalls.Add(1)
			fmt.Fprint(w, blockListResponse())
		}
	}))

	_, err := client.GetBlocks(context.Background(), "page-003")
	if err != nil {
		t.Fatal(err)
	}
	if n := extraCalls.Load(); n != 0 {
		t.Errorf("expected no extra fetches for plain paragraph, got %d", n)
	}
}
