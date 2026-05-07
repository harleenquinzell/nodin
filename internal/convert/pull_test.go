package convert_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/notion"
)

// makeRawProp encodes a Notion property value envelope as json.RawMessage.
func makeRawProp(typ string, payload map[string]any) json.RawMessage {
	envelope := map[string]any{"type": typ, "id": "abc"}
	for k, v := range payload {
		envelope[k] = v
	}
	data, _ := json.Marshal(envelope)
	return data
}

func TestPullDatabaseEntry(t *testing.T) {
	page := notion.Page{
		ID: "page-1",
		Parent: notion.Parent{
			Type:       "database_id",
			DatabaseID: "db-1",
		},
		Properties: map[string]json.RawMessage{
			"Name": makeRawProp("title", map[string]any{
				"title": []map[string]any{{
					"type":       "text",
					"plain_text": "My Entry",
					"text":       map[string]any{"content": "My Entry"},
					"annotations": map[string]bool{
						"bold": false, "italic": false,
						"strikethrough": false, "underline": false, "code": false,
					},
				}},
			}),
			"Status": makeRawProp("select", map[string]any{
				"select": map[string]any{"name": "In Progress"},
			}),
			"Priority": makeRawProp("number", map[string]any{
				"number": 3.0,
			}),
			"Done": makeRawProp("checkbox", map[string]any{
				"checkbox": true,
			}),
		},
	}

	opts := convert.PullOptions{}
	cp, err := convert.PullPage(page, nil, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Title must appear in frontmatter.
	if !strings.Contains(cp.Frontmatter, "title: My Entry") {
		t.Errorf("frontmatter missing title, got:\n%s", cp.Frontmatter)
	}

	// Non-title properties must appear under properties: section.
	if !strings.Contains(cp.Frontmatter, "properties:") {
		t.Errorf("frontmatter missing properties: section, got:\n%s", cp.Frontmatter)
	}

	for _, want := range []string{"Status", "Priority", "Done"} {
		if !strings.Contains(cp.Frontmatter, want) {
			t.Errorf("frontmatter missing property %q, got:\n%s", want, cp.Frontmatter)
		}
	}

	// Title property must NOT appear in properties: section.
	// (It is always skipped during iteration in PullPage.)
	if strings.Contains(cp.Frontmatter, "Name:") {
		t.Errorf("title property 'Name' should not appear under properties:, got:\n%s", cp.Frontmatter)
	}
}

func TestPushPageTitle(t *testing.T) {
	md := "---\ntitle: Hello World\n---\n\nSome paragraph.\n"

	page, _, err := convert.PushPage(md)
	if err != nil {
		t.Fatal(err)
	}

	if got := page.Title(); got != "Hello World" {
		t.Errorf("page.Title() = %q, want %q", got, "Hello World")
	}
}

func TestPushPageNoTitle(t *testing.T) {
	md := "Just a body with no frontmatter.\n"

	page, blocks, err := convert.PushPage(md)
	if err != nil {
		t.Fatal(err)
	}

	if got := page.Title(); got != "" {
		t.Errorf("expected empty title, got %q", got)
	}
	if len(blocks) == 0 {
		t.Error("expected at least one block")
	}
}

// mustLoadBlocks loads a .notion.json fixture file.
// It supports an optional "children" array field on each block for test fixtures
// (the real API returns children via a separate endpoint).
func mustLoadBlocks(t *testing.T, path string) []notion.Block {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	blocks, err := loadBlocksWithChildren(data)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return blocks
}

// loadBlocksWithChildren parses a JSON array of blocks, recursively populating
// any "children" field into Block.Children. This is a test-only helper.
func loadBlocksWithChildren(data []byte) ([]notion.Block, error) {
	// First pass: unmarshal as raw messages to extract both the block and children.
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(data, &rawBlocks); err != nil {
		return nil, err
	}

	blocks := make([]notion.Block, 0, len(rawBlocks))
	for _, rb := range rawBlocks {
		var b notion.Block
		if err := json.Unmarshal(rb, &b); err != nil {
			return nil, err
		}

		// Extract "children" from the raw JSON (fixture-only field).
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(rb, &envelope); err == nil {
			if rawChildren, ok := envelope["children"]; ok {
				children, err := loadBlocksWithChildren(rawChildren)
				if err != nil {
					return nil, fmt.Errorf("block %s children: %w", b.ID, err)
				}
				b.Children = children
				b.HasChildren = true
			}
		}

		blocks = append(blocks, b)
	}
	return blocks, nil
}

// mustLoad reads a text file for expected output comparison.
func mustLoad(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// unifiedDiff returns a simple diff between want and got for test failure messages.
func unifiedDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	var sb strings.Builder
	max := len(wantLines)
	if len(gotLines) > max {
		max = len(gotLines)
	}
	for i := 0; i < max; i++ {
		w := ""
		g := ""
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			sb.WriteString(fmt.Sprintf("line %d:\n  want: %q\n   got: %q\n", i+1, w, g))
		}
	}
	return sb.String()
}

func TestRoundTrip(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	opts := convert.PullOptions{
		AnchorRules: convert.DefaultAnchorRules(),
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".notion.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".notion.json")
		t.Run(name, func(t *testing.T) {
			blocks := mustLoadBlocks(t, "testdata/"+name+".notion.json")
			page := notion.Page{}

			cp1, err := convert.PullPage(page, blocks, opts)
			if err != nil {
				t.Fatal(err)
			}

			_, blocks2, err := convert.PushPage(cp1.Frontmatter + cp1.Body)
			if err != nil {
				t.Fatalf("PushPage failed: %v", err)
			}

			cp2, err := convert.PullPage(page, blocks2, opts)
			if err != nil {
				t.Fatal(err)
			}

			if cp1.Body != cp2.Body {
				t.Errorf("round-trip body mismatch for %q:\n%s", name, unifiedDiff(cp1.Body, cp2.Body))
			}
		})
	}
}

func TestPullBlocks(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	opts := convert.PullOptions{
		AnchorRules: convert.DefaultAnchorRules(),
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".notion.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".notion.json")
		t.Run(name, func(t *testing.T) {
			blocks := mustLoadBlocks(t, "testdata/"+name+".notion.json")
			want := mustLoad(t, "testdata/"+name+".md")
			page := notion.Page{}

			got, err := convert.PullPage(page, blocks, opts)
			if err != nil {
				t.Fatal(err)
			}

			if got.Body != want {
				t.Errorf("body mismatch for fixture %q:\n%s", name, unifiedDiff(want, got.Body))
			}
		})
	}
}
