package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
)

func TestSchemaToAPIProperties(t *testing.T) {
	s := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Name":   {Type: "title"},
			"Notes":  {Type: "rich_text"},
			"Count":  {Type: "number"},
			"Status": {Type: "select", Options: []SelectOption{{Name: "Todo", Color: "gray"}}},
		},
	}
	api := schemaToAPIProperties(s)

	titleEntry := api["Name"].(map[string]any)
	if _, ok := titleEntry["title"]; !ok {
		t.Errorf("Name not shaped as {title: {}}: %+v", titleEntry)
	}

	numberEntry := api["Count"].(map[string]any)
	numberCfg := numberEntry["number"].(map[string]any)
	if numberCfg["format"] != "number" {
		t.Errorf("number missing format: %+v", numberCfg)
	}

	statusEntry := api["Status"].(map[string]any)
	statusCfg := statusEntry["select"].(map[string]any)
	opts, ok := statusCfg["options"].([]map[string]any)
	if !ok || len(opts) != 1 {
		t.Fatalf("select options not shaped correctly: %+v", statusCfg)
	}
	if opts[0]["name"] != "Todo" || opts[0]["color"] != "gray" {
		t.Errorf("select option contents wrong: %+v", opts[0])
	}
}

// fakeNotionDatabaseServer is a one-endpoint stand-in for the Notion API that
// captures the create-database POST body and returns a canned database object.
func fakeNotionDatabaseServer(t *testing.T, dbID string, capture *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/databases" {
			http.Error(w, fmt.Sprintf("unexpected %s %s", r.Method, r.URL.Path), http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, capture); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Pull title from the captured payload so the response matches the request.
		title := "Tasks"
		if t, ok := (*capture)["title"].([]any); ok && len(t) > 0 {
			if rt, ok := t[0].(map[string]any); ok {
				if text, ok := rt["text"].(map[string]any); ok {
					if c, ok := text["content"].(string); ok {
						title = c
					}
				}
			}
		}

		resp := map[string]any{
			"object":           "database",
			"id":               dbID,
			"created_time":     "2026-05-11T00:00:00.000Z",
			"last_edited_time": "2026-05-11T00:00:00.000Z",
			"title": []map[string]any{
				{
					"type":       "text",
					"plain_text": title,
					"text":       map[string]any{"content": title},
					"annotations": map[string]bool{
						"bold": false, "italic": false, "strikethrough": false,
						"underline": false, "code": false,
					},
				},
			},
			"parent":     map[string]any{"type": "page_id", "page_id": "parent-page-id"},
			"properties": map[string]any{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateDatabase_WritesSchemaAndIndex(t *testing.T) {
	const dbID = "11111111-1111-1111-1111-111111111111"
	var captured map[string]any
	srv := fakeNotionDatabaseServer(t, dbID, &captured)

	client := notion.NewClient("secret_test", 100).WithBaseURL(srv.URL)
	tmp := t.TempDir()
	cfg := &config.Config{
		SyncDir:    tmp,
		RootPageID: "root-page-id",
		RPS:        100,
	}
	store := state.Open(tmp)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	schema := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Name":   {Type: "title"},
			"Status": {Type: "select", Options: []SelectOption{{Name: "Todo", Color: "gray"}}},
		},
	}

	db, err := CreateDatabase(context.Background(), cfg, store, client, schema, "")
	if err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if db.ID != dbID {
		t.Errorf("returned ID = %q, want %q", db.ID, dbID)
	}

	// Parent defaulted to cfg.RootPageID.
	parent, _ := captured["parent"].(map[string]any)
	if parent["page_id"] != "rootpageid" && parent["page_id"] != "root-page-id" {
		// normalizeID strips hyphens — accept either form
		t.Errorf("parent.page_id = %v, want root-page-id (or normalised)", parent["page_id"])
	}

	// Local _schema.json exists at databases/tasks-<shortID>/ and round-trips.
	var schemaPath string
	_ = filepath.Walk(tmp, func(p string, _ os.FileInfo, _ error) error {
		if strings.HasSuffix(p, "_schema.json") {
			schemaPath = p
		}
		return nil
	})
	if schemaPath == "" {
		t.Fatal("no _schema.json found under sync dir")
	}
	if !strings.Contains(filepath.ToSlash(schemaPath), "databases/tasks-") {
		t.Errorf("schema written at unexpected path: %s", schemaPath)
	}
	written, err := ReadDatabaseSchema(schemaPath)
	if err != nil {
		t.Fatalf("ReadDatabaseSchema: %v", err)
	}
	if written.Title != "Tasks" || written.Properties["Status"].Options[0].Name != "Todo" {
		t.Errorf("schema on disk lost data: %+v", written)
	}

	// Index has the new database entry.
	idx, err := store.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	entry, ok := idx[dbID]
	if !ok {
		t.Fatalf("index missing entry for %s; got %+v", dbID, idx)
	}
	if entry.Type != "database" {
		t.Errorf("entry.Type = %q, want database", entry.Type)
	}
	if !strings.HasPrefix(entry.LocalPath, "databases/tasks-") {
		t.Errorf("entry.LocalPath = %q", entry.LocalPath)
	}
}

func TestCreateDatabase_NoParent(t *testing.T) {
	// no httptest server — the call should fail before any HTTP request.
	client := notion.NewClient("secret_test", 100)
	tmp := t.TempDir()
	cfg := &config.Config{SyncDir: tmp, RPS: 100}
	store := state.Open(tmp)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	schema := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Name": {Type: "title"},
		},
	}

	_, err := CreateDatabase(context.Background(), cfg, store, client, schema, "")
	if err == nil {
		t.Fatal("expected error when no parent is configured")
	}
	if !strings.Contains(err.Error(), "no parent page") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateDatabase_InvalidSchema(t *testing.T) {
	client := notion.NewClient("secret_test", 100)
	tmp := t.TempDir()
	cfg := &config.Config{SyncDir: tmp, RootPageID: "root", RPS: 100}
	store := state.Open(tmp)
	if err := store.Init(); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	// Missing title property — should fail in ValidateSchema before any API call.
	schema := DatabaseSchema{
		Title: "Tasks",
		Properties: map[string]PropertySpec{
			"Notes": {Type: "rich_text"},
		},
	}
	_, err := CreateDatabase(context.Background(), cfg, store, client, schema, "")
	if err == nil {
		t.Fatal("expected error for schema missing title property")
	}
	if !strings.Contains(err.Error(), "invalid schema") {
		t.Errorf("error not wrapped as invalid schema: %v", err)
	}
}
