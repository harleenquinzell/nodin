package pathmap_test

import (
	"testing"

	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/pathmap"
)

func makePage(id, title, parentType, parentID string) notion.Page {
	p := notion.Page{
		ID:     id,
		Parent: notion.Parent{Type: parentType},
	}
	if parentType == "page_id" {
		p.Parent.PageID = parentID
	} else if parentType == "database_id" {
		p.Parent.DatabaseID = parentID
	}
	// Inject title via Properties map.
	// We use a minimal JSON approach: the Page.Title() method scans Properties.
	// For test simplicity, use a fake lookup that sets title from a map.
	_ = title // handled via titleMap in test
	return p
}

func noLookup(id string) (notion.Page, bool) {
	return notion.Page{}, false
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  Leading/Trailing  ", "leading-trailing"},
		{"café", "caf"},           // 'é' isn't in [a-z0-9]; replaced by '-' then trimmed
		{"", "untitled"},
		{"123", "123"},
		{"Hello---World", "hello-world"},
		{"nodin/sync", "nodin-sync"},
	}
	for _, tc := range cases {
		got := pathmap.Slugify(tc.input)
		if got != tc.want {
			t.Errorf("Slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPath_TopLevel(t *testing.T) {
	pages := map[string]notion.Page{
		"aabbccdd-0000-0000-0000-000000000000": {
			ID:     "aabbccdd-0000-0000-0000-000000000000",
			Parent: notion.Parent{Type: "workspace"},
		},
	}
	lookup := func(id string) (notion.Page, bool) {
		p, ok := pages[id]
		return p, ok
	}

	p := pages["aabbccdd-0000-0000-0000-000000000000"]
	// Title is empty → slug = "untitled-aabbccdd"
	got, err := pathmap.PagePath(p, lookup)
	if err != nil {
		t.Fatal(err)
	}
	want := "pages/untitled-aabbccdd/untitled-aabbccdd.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPath_OrphanInaccessibleParent(t *testing.T) {
	p := notion.Page{
		ID:     "deadbeef-0000-0000-0000-000000000000",
		Parent: notion.Parent{Type: "page_id", PageID: "missing-parent-id"},
	}
	got, err := pathmap.PagePath(p, noLookup)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("expected non-empty path for orphan")
	}
	// Should start with _orphans/
	if got[:8] != "_orphans" {
		t.Errorf("orphan path should start with _orphans, got %q", got)
	}
}

func TestPath_SlugCollision(t *testing.T) {
	// Two pages with same title but different IDs should produce different paths.
	p1 := notion.Page{
		ID:     "aaaaaaaa-0000-0000-0000-000000000000",
		Parent: notion.Parent{Type: "workspace"},
	}
	p2 := notion.Page{
		ID:     "bbbbbbbb-0000-0000-0000-000000000000",
		Parent: notion.Parent{Type: "workspace"},
	}

	lookup := func(id string) (notion.Page, bool) { return notion.Page{}, false }

	path1, _ := pathmap.PagePath(p1, lookup)
	path2, _ := pathmap.PagePath(p2, lookup)

	if path1 == path2 {
		t.Errorf("different IDs produced the same path: %q", path1)
	}
}

func TestPath_SlugEmptyTitle(t *testing.T) {
	p := notion.Page{
		ID:     "cccccccc-0000-0000-0000-000000000000",
		Parent: notion.Parent{Type: "workspace"},
	}
	lookup := func(id string) (notion.Page, bool) { return notion.Page{}, false }
	got, err := pathmap.PagePath(p, lookup)
	if err != nil {
		t.Fatal(err)
	}
	// Slug for empty title should be "untitled"
	if got != "pages/untitled-cccccccc/untitled-cccccccc.md" {
		t.Errorf("got %q, want pages/untitled-cccccccc/untitled-cccccccc.md", got)
	}
}
