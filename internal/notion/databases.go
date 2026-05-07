package notion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// QueryOpts configures a database query request.
type QueryOpts struct {
	Cursor string
	Limit  int
	Since  time.Time // filter entries edited after this time (zero = all)
}

// GetDatabase fetches a Notion database schema by ID.
func (c *Client) GetDatabase(ctx context.Context, id string) (*Database, error) {
	data, err := c.do(ctx, "GET", "/databases/"+normalizeID(id), nil)
	if err != nil {
		return nil, fmt.Errorf("get database %s: %w", id, err)
	}
	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parse database %s: %w", id, err)
	}
	return &db, nil
}

// QueryDatabase returns all pages in a database, with optional incremental filtering.
// When opts.Since is non-zero, only pages edited after that time are returned.
func (c *Client) QueryDatabase(ctx context.Context, id string, opts QueryOpts) ([]Page, error) {
	return walkCursor(ctx,
		func(cursor string) ([]Page, string, bool, error) {
			body := map[string]any{}
			if cursor != "" {
				body["start_cursor"] = cursor
			}
			if opts.Limit > 0 {
				body["page_size"] = opts.Limit
			}
			if !opts.Since.IsZero() {
				body["filter"] = map[string]any{
					"timestamp": "last_edited_time",
					"last_edited_time": map[string]string{
						"after": opts.Since.UTC().Format(time.RFC3339),
					},
				}
			}

			data, err := c.do(ctx, "POST", "/databases/"+normalizeID(id)+"/query", body)
			if err != nil {
				return nil, "", false, err
			}
			var resp listResponse[Page]
			if err := json.Unmarshal(data, &resp); err != nil {
				return nil, "", false, fmt.Errorf("parse query response: %w", err)
			}
			return resp.Results, resp.NextCursor, resp.HasMore, nil
		},
		nil,
	)
}
