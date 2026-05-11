package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/pathmap"
	"github.com/harleenquinzell/nodin/internal/state"
)

// CreateDatabase creates a new Notion database from the given schema and
// records it locally. parentPageID is the Notion page the database lives
// under; if empty, cfg.RootPageID is used.
//
// On success:
//   - The database exists on Notion.
//   - databases/<slug>-<shortID>/_schema.json is written in the rich format.
//   - The database is registered in the index as type="database".
//
// CreateDatabase is the shared core that nodin new-db calls today and that
// push will call later when it finds an untracked databases/<slug>/ folder.
func CreateDatabase(
	ctx context.Context,
	cfg *config.Config,
	store *state.Store,
	client *notion.Client,
	schema DatabaseSchema,
	parentPageID string,
) (*notion.Database, error) {
	if err := ValidateSchema(schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}

	parent := parentPageID
	if parent == "" {
		parent = cfg.RootPageID
	}
	if parent == "" {
		return nil, fmt.Errorf("no parent page: pass parentPageID or set root_page_id in config")
	}

	apiProps := schemaToAPIProperties(schema)

	db, err := client.CreateDatabase(ctx, parent, schema.Title, apiProps)
	if err != nil {
		return nil, fmt.Errorf("create database on notion: %w", err)
	}

	dbDirRel := pathmap.DatabasePath(db.AsPage())
	dbDir := filepath.Join(cfg.SyncDir, dbDirRel)
	if _, err := os.Stat(dbDir); err == nil {
		return db, fmt.Errorf("database created on notion (id %s) but local path %s already exists; resolve manually", db.ID, dbDirRel)
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return db, fmt.Errorf("create %s: %w", dbDirRel, err)
	}

	schemaPath := filepath.Join(dbDir, "_schema.json")
	if err := WriteDatabaseSchema(schemaPath, schema); err != nil {
		return db, fmt.Errorf("write schema: %w", err)
	}

	if err := store.UpdateEntry(db.ID, func(e state.IndexEntry) state.IndexEntry {
		e.NotionID = db.ID
		e.LocalPath = dbDirRel
		e.Type = "database"
		e.LastSync = db.LastEditedTime
		return e
	}); err != nil {
		return db, fmt.Errorf("update index: %w", err)
	}

	return db, nil
}

// schemaToAPIProperties converts the local rich schema into the property-config
// map shape expected by the Notion "create database" endpoint.
func schemaToAPIProperties(s DatabaseSchema) map[string]any {
	props := make(map[string]any, len(s.Properties))
	for name, spec := range s.Properties {
		config := map[string]any{}
		switch spec.Type {
		case "select", "multi_select":
			opts := make([]map[string]any, 0, len(spec.Options))
			for _, o := range spec.Options {
				entry := map[string]any{"name": o.Name}
				if o.Color != "" {
					entry["color"] = o.Color
				}
				opts = append(opts, entry)
			}
			config["options"] = opts
		case "number":
			config["format"] = "number"
		}
		props[name] = map[string]any{spec.Type: config}
	}
	return props
}
