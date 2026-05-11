package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDatabaseSchema_RichFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_schema.json")
	contents := `{
		"title": "Tasks",
		"properties": {
			"Name":   { "type": "title" },
			"Status": { "type": "select", "options": [
				{"name":"Todo", "color":"gray"},
				{"name":"Done", "color":"green"}
			]}
		}
	}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := ReadDatabaseSchema(path)
	if err != nil {
		t.Fatalf("ReadDatabaseSchema: %v", err)
	}
	if s.Title != "Tasks" {
		t.Errorf("Title = %q, want %q", s.Title, "Tasks")
	}
	if s.Properties["Name"].Type != "title" {
		t.Errorf("Name.Type = %q, want title", s.Properties["Name"].Type)
	}
	status := s.Properties["Status"]
	if status.Type != "select" {
		t.Errorf("Status.Type = %q, want select", status.Type)
	}
	if len(status.Options) != 2 {
		t.Fatalf("Status.Options len = %d, want 2", len(status.Options))
	}
	if status.Options[0].Name != "Todo" || status.Options[0].Color != "gray" {
		t.Errorf("Status.Options[0] = %+v", status.Options[0])
	}
}

func TestReadDatabaseSchema_ThinFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_schema.json")
	contents := `{"Name": "title", "Notes": "rich_text", "Status": "select"}`
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := ReadDatabaseSchema(path)
	if err != nil {
		t.Fatalf("ReadDatabaseSchema: %v", err)
	}
	if s.Title != "" {
		t.Errorf("thin schema should have empty Title, got %q", s.Title)
	}
	if s.Properties["Name"].Type != "title" {
		t.Errorf("Name.Type = %q, want title", s.Properties["Name"].Type)
	}
	if s.Properties["Notes"].Type != "rich_text" {
		t.Errorf("Notes.Type = %q, want rich_text", s.Properties["Notes"].Type)
	}
	if len(s.Properties["Status"].Options) != 0 {
		t.Errorf("thin format must not synthesise options, got %d", len(s.Properties["Status"].Options))
	}

	// .Thin() round-trips the type-only view that entry-push uses.
	thin := s.Thin()
	if thin["Name"] != "title" || thin["Status"] != "select" || thin["Notes"] != "rich_text" {
		t.Errorf("Thin() = %+v", thin)
	}
}

func TestValidateSchema(t *testing.T) {
	valid := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Name":   {Type: "title"},
			"Status": {Type: "select", Options: []SelectOption{{Name: "Todo", Color: "gray"}}},
			"Notes":  {Type: "rich_text"},
		},
	}
	if err := ValidateSchema(valid); err != nil {
		t.Errorf("valid schema rejected: %v", err)
	}

	cases := []struct {
		name   string
		schema DatabaseSchema
		want   string // substring expected in error
	}{
		{
			name:   "empty title",
			schema: DatabaseSchema{Properties: valid.Properties},
			want:   "title is required",
		},
		{
			name: "no title property",
			schema: DatabaseSchema{
				Title: "X",
				Properties: map[string]PropertySpec{
					"Notes": {Type: "rich_text"},
				},
			},
			want: "exactly one property with type \"title\"",
		},
		{
			name: "two title properties",
			schema: DatabaseSchema{
				Title: "X",
				Properties: map[string]PropertySpec{
					"A": {Type: "title"},
					"B": {Type: "title"},
				},
			},
			want: "2 title properties",
		},
		{
			name: "unsupported type",
			schema: DatabaseSchema{
				Title: "X",
				Properties: map[string]PropertySpec{
					"Name": {Type: "title"},
					"Owns": {Type: "relation"},
				},
			},
			want: "unsupported property types",
		},
		{
			name: "select without options",
			schema: DatabaseSchema{
				Title: "X",
				Properties: map[string]PropertySpec{
					"Name":   {Type: "title"},
					"Status": {Type: "select"},
				},
			},
			want: "select properties without options",
		},
		{
			name: "bad color",
			schema: DatabaseSchema{
				Title: "X",
				Properties: map[string]PropertySpec{
					"Name":   {Type: "title"},
					"Status": {Type: "select", Options: []SelectOption{{Name: "X", Color: "neon"}}},
				},
			},
			want: "invalid select option colors",
		},
		{
			name:   "no properties",
			schema: DatabaseSchema{Title: "X"},
			want:   "at least one property",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSchema(tc.schema)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestWriteAndRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_schema.json")
	in := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Name":   {Type: "title"},
			"Status": {Type: "select", Options: []SelectOption{{Name: "Todo"}}},
		},
	}
	if err := WriteDatabaseSchema(path, in); err != nil {
		t.Fatalf("WriteDatabaseSchema: %v", err)
	}
	out, err := ReadDatabaseSchema(path)
	if err != nil {
		t.Fatalf("ReadDatabaseSchema: %v", err)
	}
	if out.Title != in.Title {
		t.Errorf("Title round-trip: %q → %q", in.Title, out.Title)
	}
	if out.Properties["Status"].Options[0].Name != "Todo" {
		t.Errorf("option round-trip lost: %+v", out.Properties["Status"])
	}
}
