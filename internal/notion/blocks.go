package notion

import (
	"context"
	"encoding/json"
	"fmt"
)

// Block represents a Notion block object.
type Block struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	HasChildren bool            `json:"has_children"`
	raw         json.RawMessage // captures the type-specific content field
	Children    []Block         // populated by GetBlocks recursive fetch
}

// UnmarshalJSON captures the full raw block so we can extract type-specific content.
func (b *Block) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		HasChildren bool            `json:"has_children"`
		Raw         json.RawMessage `json:"-"`
	}

	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	b.ID = a.ID
	b.Type = a.Type
	b.HasChildren = a.HasChildren
	b.raw = data
	return nil
}

// Content returns the parsed content for this block's type.
// Returns UnsupportedContent for unknown types.
func (b *Block) Content() BlockContent {
	if b.raw == nil {
		return &UnsupportedContent{BlockType: b.Type}
	}

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(b.raw, &outer); err != nil {
		return &UnsupportedContent{BlockType: b.Type, Raw: b.raw}
	}

	typeData, ok := outer[b.Type]
	if !ok {
		return &UnsupportedContent{BlockType: b.Type}
	}

	switch b.Type {
	case "paragraph":
		var c ParagraphContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "heading_1":
		var c HeadingContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			c.Level = 1
			return &c
		}
	case "heading_2":
		var c HeadingContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			c.Level = 2
			return &c
		}
	case "heading_3":
		var c HeadingContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			c.Level = 3
			return &c
		}
	case "bulleted_list_item":
		var c ListItemContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "numbered_list_item":
		var c NumberedListItemContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "to_do":
		var c ToDoContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "toggle":
		var c ToggleContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "quote":
		var c QuoteContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "callout":
		var c CalloutContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "code":
		var c CodeContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "divider":
		return &DividerContent{}
	case "table":
		var c TableContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "table_row":
		var c TableRowContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "equation":
		var c EquationContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "image":
		var c ImageContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "file":
		var c FileBlockContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "bookmark":
		var c BookmarkContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "child_page":
		var c ChildPageContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	case "child_database":
		var c ChildDatabaseContent
		if err := json.Unmarshal(typeData, &c); err == nil {
			return &c
		}
	}

	return &UnsupportedContent{BlockType: b.Type, Raw: typeData}
}

// BlockContent is implemented by all block content types.
type BlockContent interface {
	blockContentType() string
}

// ParagraphContent holds a paragraph block's rich text.
type ParagraphContent struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

func (c *ParagraphContent) blockContentType() string { return "paragraph" }

// HeadingContent holds a heading block's rich text and level.
// Level is set by the Block.Content() method, not from JSON.
type HeadingContent struct {
	RichText     []RichText `json:"rich_text"`
	Color        string     `json:"color"`
	IsToggleable bool       `json:"is_toggleable"`
	Level        int        `json:"-"`
}

func (c *HeadingContent) blockContentType() string { return "heading" }

// ListItemContent holds a bulleted list item's rich text.
type ListItemContent struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

func (c *ListItemContent) blockContentType() string { return "bulleted_list_item" }

// NumberedListItemContent holds a numbered list item's rich text.
type NumberedListItemContent struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

func (c *NumberedListItemContent) blockContentType() string { return "numbered_list_item" }

// ToDoContent holds a to-do block's rich text and checked state.
type ToDoContent struct {
	RichText []RichText `json:"rich_text"`
	Checked  bool       `json:"checked"`
	Color    string     `json:"color"`
}

func (c *ToDoContent) blockContentType() string { return "to_do" }

// ToggleContent holds a toggle block's rich text.
type ToggleContent struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

func (c *ToggleContent) blockContentType() string { return "toggle" }

// QuoteContent holds a quote block's rich text.
type QuoteContent struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

func (c *QuoteContent) blockContentType() string { return "quote" }

// CalloutContent holds a callout block's rich text and icon.
type CalloutContent struct {
	RichText []RichText `json:"rich_text"`
	Icon     *Icon      `json:"icon"`
	Color    string     `json:"color"`
}

func (c *CalloutContent) blockContentType() string { return "callout" }

// CodeContent holds a code block's language tag, code text, and caption.
type CodeContent struct {
	RichText []RichText `json:"rich_text"`
	Caption  []RichText `json:"caption"`
	Language string     `json:"language"`
}

func (c *CodeContent) blockContentType() string { return "code" }

// DividerContent represents a divider block (no fields).
type DividerContent struct{}

func (c *DividerContent) blockContentType() string { return "divider" }

// TableContent holds a table block's column/row header flags.
// Rows are populated by GetBlocks via child table_row blocks.
type TableContent struct {
	TableWidth      int     `json:"table_width"`
	HasColumnHeader bool    `json:"has_column_header"`
	HasRowHeader    bool    `json:"has_row_header"`
	Rows            []Block `json:"-"` // populated from children
}

func (c *TableContent) blockContentType() string { return "table" }

// TableRowContent holds one row of a table.
type TableRowContent struct {
	Cells [][]RichText `json:"cells"`
}

func (c *TableRowContent) blockContentType() string { return "table_row" }

// EquationContent holds a block-level equation expression.
type EquationContent struct {
	Expression string `json:"expression"`
}

func (c *EquationContent) blockContentType() string { return "equation" }

// ImageContent holds an image block's file reference and caption.
type ImageContent struct {
	Type     string     `json:"type"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	File *struct {
		URL        string `json:"url"`
		ExpiryTime string `json:"expiry_time,omitempty"`
	} `json:"file,omitempty"`
	Caption []RichText `json:"caption"`
}

func (c *ImageContent) blockContentType() string { return "image" }

// URL returns the image URL, regardless of hosting type.
func (c *ImageContent) URL() string {
	switch c.Type {
	case "external":
		if c.External != nil {
			return c.External.URL
		}
	case "file":
		if c.File != nil {
			return c.File.URL
		}
	}
	return ""
}

// FileBlockContent holds a file block's reference and caption.
type FileBlockContent struct {
	Type     string     `json:"type"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	File *struct {
		URL string `json:"url"`
	} `json:"file,omitempty"`
	Caption []RichText `json:"caption"`
	Name    string     `json:"name"`
}

func (c *FileBlockContent) blockContentType() string { return "file" }

// URL returns the file URL.
func (c *FileBlockContent) URL() string {
	switch c.Type {
	case "external":
		if c.External != nil {
			return c.External.URL
		}
	case "file":
		if c.File != nil {
			return c.File.URL
		}
	}
	return ""
}

// BookmarkContent holds a bookmark block's URL and caption.
type BookmarkContent struct {
	URL     string     `json:"url"`
	Caption []RichText `json:"caption"`
}

func (c *BookmarkContent) blockContentType() string { return "bookmark" }

// ChildPageContent holds the title of an embedded child page.
type ChildPageContent struct {
	Title string `json:"title"`
}

func (c *ChildPageContent) blockContentType() string { return "child_page" }

// ChildDatabaseContent holds the title of an embedded child database.
type ChildDatabaseContent struct {
	Title string `json:"title"`
}

func (c *ChildDatabaseContent) blockContentType() string { return "child_database" }

// UnsupportedContent is a placeholder for block types not yet handled.
type UnsupportedContent struct {
	BlockType string
	Raw       json.RawMessage
}

func (c *UnsupportedContent) blockContentType() string { return c.BlockType }

// GetBlocks fetches all blocks under parentID, recursively expanding children.
func (c *Client) GetBlocks(ctx context.Context, parentID string) ([]Block, error) {
	blocks, err := c.getBlocksPage(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for i := range blocks {
		if !blocks[i].HasChildren {
			continue
		}
		// Tables: fetch children as rows (stored inline in TableContent, not Children).
		if blocks[i].Type == "table" {
			rows, err := c.getBlocksPage(ctx, blocks[i].ID)
			if err != nil {
				return nil, fmt.Errorf("get table rows %s: %w", blocks[i].ID, err)
			}
			blocks[i].Children = rows
		} else {
			children, err := c.GetBlocks(ctx, blocks[i].ID)
			if err != nil {
				return nil, fmt.Errorf("get children of %s: %w", blocks[i].ID, err)
			}
			blocks[i].Children = children
		}
	}
	return blocks, nil
}

// getBlocksPage fetches a single paginated level of children (no recursion).
func (c *Client) getBlocksPage(ctx context.Context, parentID string) ([]Block, error) {
	return walkCursor(ctx,
		func(cursor string) ([]Block, string, bool, error) {
			path := "/blocks/" + normalizeID(parentID) + "/children"
			if cursor != "" {
				path += "?start_cursor=" + cursor
			}
			data, err := c.do(ctx, "GET", path, nil)
			if err != nil {
				return nil, "", false, err
			}
			var resp listResponse[Block]
			if err := json.Unmarshal(data, &resp); err != nil {
				return nil, "", false, fmt.Errorf("parse blocks response: %w", err)
			}
			return resp.Results, resp.NextCursor, resp.HasMore, nil
		},
		nil, // no early-exit predicate
	)
}
