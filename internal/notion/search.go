package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SearchOpts configures a search request.
type SearchOpts struct {
	// Filter limits results by object type: "page", "database", or "" for both.
	Filter string
	// Query is a text search string matched against page titles.
	Query  string
	Cursor string
	Limit  int // page_size; 0 → API default (100)
}

// SearchResponse is the result of a Search call.
type SearchResponse struct {
	Results    []Page
	NextCursor string
	HasMore    bool
}

// Search queries the Notion search endpoint, sorted by last_edited_time descending.
func (c *Client) Search(ctx context.Context, opts SearchOpts) (*SearchResponse, error) {
	body := map[string]any{
		"sort": map[string]string{
			"direction": "descending",
			"timestamp": "last_edited_time",
		},
	}
	if opts.Query != "" {
		body["query"] = opts.Query
	}
	if opts.Filter != "" {
		body["filter"] = map[string]string{
			"value":    opts.Filter,
			"property": "object",
		}
	}
	if opts.Cursor != "" {
		body["start_cursor"] = opts.Cursor
	}
	if opts.Limit > 0 {
		body["page_size"] = opts.Limit
	}

	data, err := c.do(ctx, "POST", "/search", body)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var raw struct {
		Results    []Page `json:"results"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	return &SearchResponse{
		Results:    raw.Results,
		NextCursor: raw.NextCursor,
		HasMore:    raw.HasMore,
	}, nil
}

// IncrementalPages returns all pages edited after since (zero time = all pages).
// Results are sorted descending by last_edited_time; we stop as soon as we see
// a page that is not newer than since.
func (c *Client) IncrementalPages(ctx context.Context, since time.Time) ([]Page, error) {
	return walkCursor(ctx,
		func(cursor string) ([]Page, string, bool, error) {
			resp, err := c.Search(ctx, SearchOpts{
				Filter: "page",
				Cursor: cursor,
				Limit:  100,
			})
			if err != nil {
				return nil, "", false, err
			}
			return resp.Results, resp.NextCursor, resp.HasMore, nil
		},
		func(p Page) bool {
			if since.IsZero() {
				return false
			}
			return !p.LastEditedTime.After(since)
		},
	)
}
