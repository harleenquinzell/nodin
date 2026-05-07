package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CreatePage creates a new child page under parentID with the given title.
func (c *Client) CreatePage(ctx context.Context, parentID, title string) (*Page, error) {
	body := map[string]any{
		"parent": map[string]any{"type": "page_id", "page_id": normalizeID(parentID)},
		"properties": map[string]any{
			"title": map[string]any{
				"title": []map[string]any{
					{"type": "text", "text": map[string]any{"content": title}},
				},
			},
		},
	}
	data, err := c.do(ctx, "POST", "/pages", body)
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	var page Page
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parse created page: %w", err)
	}
	return &page, nil
}

// GetPage fetches a Notion page by ID.
// Accepts both hyphenated and unhyphenated UUIDs.
func (c *Client) GetPage(ctx context.Context, id string) (*Page, error) {
	data, err := c.do(ctx, "GET", "/pages/"+normalizeID(id), nil)
	if err != nil {
		var ne *NotionError
		if errors.As(err, &ne) && ne.Code == "object_not_found" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get page %s: %w", id, err)
	}
	var page Page
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("parse page %s: %w", id, err)
	}
	return &page, nil
}

// UpdatePageProperties patches a set of database entry properties.
// Only the properties in props are sent; unmentioned properties are unchanged.
// Computed properties (formula, rollup, etc.) are silently skipped.
func (c *Client) UpdatePageProperties(ctx context.Context, id string, props map[string]PropertyValue) error {
	apiProps := make(map[string]any, len(props))
	for name, pv := range props {
		if pv.Computed {
			continue
		}
		apiProps[name] = propertyValueToAPI(pv)
	}
	if len(apiProps) == 0 {
		return nil
	}
	_, err := c.do(ctx, "PATCH", "/pages/"+normalizeID(id), map[string]any{"properties": apiProps})
	if err != nil {
		return fmt.Errorf("update properties for page %s: %w", id, err)
	}
	return nil
}

// propertyValueToAPI converts a PropertyValue to the Notion API patch format.
func propertyValueToAPI(pv PropertyValue) map[string]any {
	switch pv.Type {
	case "rich_text":
		return map[string]any{"rich_text": []map[string]any{
			{"type": "text", "text": map[string]any{"content": pv.Text}},
		}}
	case "number":
		if pv.Number == nil {
			return map[string]any{"number": nil}
		}
		return map[string]any{"number": *pv.Number}
	case "select":
		if pv.Select == "" {
			return map[string]any{"select": nil}
		}
		return map[string]any{"select": map[string]any{"name": pv.Select}}
	case "status":
		return map[string]any{"status": map[string]any{"name": pv.Select}}
	case "multi_select":
		opts := make([]map[string]any, len(pv.MultiSel))
		for i, s := range pv.MultiSel {
			opts[i] = map[string]any{"name": s}
		}
		return map[string]any{"multi_select": opts}
	case "date":
		if pv.Date == nil {
			return map[string]any{"date": nil}
		}
		d := map[string]any{"start": pv.Date.Start}
		if pv.Date.End != "" {
			d["end"] = pv.Date.End
		}
		if pv.Date.TZ != "" {
			d["time_zone"] = pv.Date.TZ
		}
		return map[string]any{"date": d}
	case "checkbox":
		if pv.Checkbox == nil {
			return map[string]any{"checkbox": false}
		}
		return map[string]any{"checkbox": *pv.Checkbox}
	case "url":
		return map[string]any{"url": pv.Text}
	case "email":
		return map[string]any{"email": pv.Text}
	case "phone_number":
		return map[string]any{"phone_number": pv.Text}
	case "people":
		people := make([]map[string]any, len(pv.People))
		for i, id := range pv.People {
			people[i] = map[string]any{"object": "user", "id": id}
		}
		return map[string]any{"people": people}
	case "relation":
		rels := make([]map[string]any, len(pv.Relation))
		for i, id := range pv.Relation {
			rels[i] = map[string]any{"id": id}
		}
		return map[string]any{"relation": rels}
	default:
		return map[string]any{}
	}
}

// UpdatePage updates the title of a Notion page.
func (c *Client) UpdatePage(ctx context.Context, id, title string) error {
	body := map[string]any{
		"properties": map[string]any{
			"title": map[string]any{
				"title": []map[string]any{
					{"type": "text", "text": map[string]any{"content": title}},
				},
			},
		},
	}
	_, err := c.do(ctx, "PATCH", "/pages/"+normalizeID(id), body)
	if err != nil {
		return fmt.Errorf("update page %s: %w", id, err)
	}
	return nil
}

// ArchivePage soft-deletes a Notion page by setting archived=true.
func (c *Client) ArchivePage(ctx context.Context, id string) error {
	_, err := c.do(ctx, "PATCH", "/pages/"+normalizeID(id), map[string]any{"archived": true})
	if err != nil {
		return fmt.Errorf("archive page %s: %w", id, err)
	}
	return nil
}

// normalizeID returns the hyphenated form of a Notion UUID.
// Both "3589c9400284..." and "3589c940-0284-..." are accepted.
func normalizeID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) != 32 {
		return id
	}
	return clean[0:8] + "-" + clean[8:12] + "-" + clean[12:16] + "-" + clean[16:20] + "-" + clean[20:32]
}
