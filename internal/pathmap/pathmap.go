package pathmap

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// PagePath computes the local file path for a Notion page, relative to the sync root.
// lookup resolves a Notion ID to a Page (used for parent chain traversal).
func PagePath(p notion.Page, lookup func(id string) (notion.Page, bool)) (string, error) {
	slug := Slugify(p.Title()) + "-" + shortID(p.ID)

	switch p.Parent.Type {
	case "workspace":
		return filepath.Join("pages", slug, slug+".md"), nil

	case "page_id":
		parent, ok := lookup(p.Parent.PageID)
		if !ok {
			return filepath.Join("_orphans", slug, slug+".md"), nil
		}
		parentPath, err := PagePath(parent, lookup)
		if err != nil {
			return filepath.Join("_orphans", slug, slug+".md"), nil
		}
		return filepath.Join(filepath.Dir(parentPath), slug, slug+".md"), nil

	case "database_id":
		db, ok := lookup(p.Parent.DatabaseID)
		if !ok {
			return filepath.Join("_orphans", slug+".md"), nil
		}
		dbSlug := Slugify(db.Title()) + "-" + shortID(db.ID)
		return filepath.Join("databases", dbSlug, slug+".md"), nil

	case "block_id":
		// Page is embedded inside a block (e.g. inside a toggle or callout).
		// Traversing block→page requires an extra API call we don't have here,
		// so fall back to _orphans/ rather than failing the whole pull.
		return filepath.Join("_orphans", slug, slug+".md"), nil

	default:
		return "", fmt.Errorf("unknown parent type %q", p.Parent.Type)
	}
}

// DatabasePath computes the local directory path for a Notion database.
func DatabasePath(db notion.Page) string {
	slug := Slugify(db.Title()) + "-" + shortID(db.ID)
	return filepath.Join("databases", slug)
}

// Slugify converts a title into a URL-safe slug.
// Rules: NFC-normalize → lowercase → replace any char not in [a-z0-9] with "-" →
// collapse runs of "-" → trim leading/trailing "-" → "untitled" if empty.
func Slugify(title string) string {
	normalized := norm.NFC.String(strings.ToLower(title))

	var sb strings.Builder
	prevDash := true // start true to trim leading dashes

	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prevDash = false
		} else {
			// Replace any non-[a-z0-9] character with a single dash (collapse runs).
			if !prevDash {
				sb.WriteByte('-')
				prevDash = true
			}
		}
	}

	// Trim trailing dash.
	result := strings.TrimRight(sb.String(), "-")
	if result == "" {
		return "untitled"
	}
	return result
}

// shortID returns the first 8 hex characters of a Notion UUID (without hyphens).
func shortID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) >= 8 {
		return clean[:8]
	}
	return clean
}
