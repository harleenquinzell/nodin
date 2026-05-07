package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

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

// normalizeID returns the hyphenated form of a Notion UUID.
// Both "3589c9400284..." and "3589c940-0284-..." are accepted.
func normalizeID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) != 32 {
		return id
	}
	return clean[0:8] + "-" + clean[8:12] + "-" + clean[12:16] + "-" + clean[16:20] + "-" + clean[20:32]
}
