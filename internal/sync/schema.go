package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// DatabaseSchema is the rich on-disk schema for a Notion database.
// It is stored in databases/<slug>/_schema.json and is used by both
// nodin new-db (creating the database) and push (creating entries inside it).
type DatabaseSchema struct {
	Title      string                  `json:"title"`
	Properties map[string]PropertySpec `json:"properties"`
}

// PropertySpec describes one column of a database.
type PropertySpec struct {
	Type    string         `json:"type"`
	Options []SelectOption `json:"options,omitempty"`
}

// SelectOption is one choice in a select / multi_select property.
type SelectOption struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// SupportedPropertyTypes lists the property types nodin can create today.
// Other types (relation, formula, rollup, status, files, people, …) are
// deferred and rejected by ValidateSchema.
var SupportedPropertyTypes = map[string]bool{
	"title":        true,
	"rich_text":    true,
	"number":       true,
	"date":         true,
	"checkbox":     true,
	"url":          true,
	"email":        true,
	"phone_number": true,
	"select":       true,
	"multi_select": true,
}

// SupportedSelectColors lists the colors Notion accepts on a select option.
// An empty color is also allowed (Notion picks one).
var SupportedSelectColors = map[string]bool{
	"":        true,
	"default": true,
	"gray":    true,
	"brown":   true,
	"orange":  true,
	"yellow":  true,
	"green":   true,
	"blue":    true,
	"purple":  true,
	"pink":    true,
	"red":     true,
}

// Thin returns the legacy "property name → type" map used by entry-push
// codepaths that only need a type lookup.
func (s DatabaseSchema) Thin() map[string]string {
	out := make(map[string]string, len(s.Properties))
	for name, spec := range s.Properties {
		out[name] = spec.Type
	}
	return out
}

// TitleProperty returns the name of the schema's title property, or "" if none.
func (s DatabaseSchema) TitleProperty() string {
	for name, spec := range s.Properties {
		if spec.Type == "title" {
			return name
		}
	}
	return ""
}

// ReadDatabaseSchema reads _schema.json from path, accepting both the rich
// format (object with "title" + "properties") and the legacy thin format
// (map of "name" → "type"). For the thin format, Title is left empty and
// select-type options are empty — callers that need them must fail loudly.
func ReadDatabaseSchema(path string) (DatabaseSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DatabaseSchema{}, err
	}
	return parseSchemaBytes(data, path)
}

func parseSchemaBytes(data []byte, path string) (DatabaseSchema, error) {
	// Try the rich format first.
	var rich struct {
		Title      *string                 `json:"title"`
		Properties map[string]PropertySpec `json:"properties"`
	}
	if err := json.Unmarshal(data, &rich); err == nil && rich.Properties != nil {
		s := DatabaseSchema{Properties: rich.Properties}
		if rich.Title != nil {
			s.Title = *rich.Title
		}
		return s, nil
	}

	// Fall back to the thin format: { "Name": "title", "Status": "select", ... }.
	var thin map[string]string
	if err := json.Unmarshal(data, &thin); err != nil {
		return DatabaseSchema{}, fmt.Errorf("parse %s: %w", path, err)
	}
	props := make(map[string]PropertySpec, len(thin))
	for name, typ := range thin {
		props[name] = PropertySpec{Type: typ}
	}
	return DatabaseSchema{Properties: props}, nil
}

// WriteDatabaseSchema atomically writes the rich form to path.
func WriteDatabaseSchema(path string, schema DatabaseSchema) error {
	return writeJSONFile(path, schema)
}

// ValidateSchema returns an error if the schema cannot be used to create a
// new Notion database. Required: non-empty title, exactly one title-typed
// property, all property types supported, every select / multi_select has at
// least one option with a supported color.
func ValidateSchema(s DatabaseSchema) error {
	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("schema title is required")
	}
	if len(s.Properties) == 0 {
		return fmt.Errorf("schema must define at least one property")
	}

	var (
		titleProps   []string
		unsupported  []string
		emptySelects []string
		badColors    []string
	)
	for name, spec := range s.Properties {
		if spec.Type == "title" {
			titleProps = append(titleProps, name)
		}
		if !SupportedPropertyTypes[spec.Type] {
			unsupported = append(unsupported, fmt.Sprintf("%s (%s)", name, spec.Type))
			continue
		}
		if spec.Type == "select" || spec.Type == "multi_select" {
			if len(spec.Options) == 0 {
				emptySelects = append(emptySelects, name)
				continue
			}
			for _, opt := range spec.Options {
				if !SupportedSelectColors[opt.Color] {
					badColors = append(badColors, fmt.Sprintf("%s.%s=%s", name, opt.Name, opt.Color))
				}
			}
		}
	}

	switch len(titleProps) {
	case 0:
		return fmt.Errorf("schema must have exactly one property with type \"title\"")
	case 1:
		// ok
	default:
		sort.Strings(titleProps)
		return fmt.Errorf("schema has %d title properties (%s); exactly one is allowed",
			len(titleProps), strings.Join(titleProps, ", "))
	}

	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		return fmt.Errorf("unsupported property types: %s", strings.Join(unsupported, ", "))
	}
	if len(emptySelects) > 0 {
		sort.Strings(emptySelects)
		return fmt.Errorf("select properties without options: %s", strings.Join(emptySelects, ", "))
	}
	if len(badColors) > 0 {
		sort.Strings(badColors)
		return fmt.Errorf("invalid select option colors: %s", strings.Join(badColors, ", "))
	}
	return nil
}
