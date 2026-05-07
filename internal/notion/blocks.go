package notion

import (
	"context"
	"encoding/json"
	"fmt"
)

// Block represents a Notion block object.
// Content is populated eagerly on JSON unmarshal; it can also be set
// directly when constructing blocks for push.
type Block struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	HasChildren bool         `json:"has_children"`
	Content     BlockContent `json:"-"`
	Children    []Block      `json:"-"`
}

// UnmarshalJSON eagerly parses the block's type-specific content.
func (b *Block) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		HasChildren bool   `json:"has_children"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	b.ID = a.ID
	b.Type = a.Type
	b.HasChildren = a.HasChildren
	b.Content = parseBlockContent(a.Type, data)
	return nil
}

// parseBlockContent extracts the type-specific content field from a raw block JSON.
func parseBlockContent(blockType string, data []byte) BlockContent {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil {
		return &UnsupportedContent{BlockType: blockType}
	}

	typeData, ok := outer[blockType]
	if !ok {
		return &UnsupportedContent{BlockType: blockType}
	}

	switch blockType {
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

	return &UnsupportedContent{BlockType: blockType, Raw: typeData}
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
// Rows are populated from Children after GetBlocks fetches them.
type TableContent struct {
	TableWidth      int  `json:"table_width"`
	HasColumnHeader bool `json:"has_column_header"`
	HasRowHeader    bool `json:"has_row_header"`
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
	Type     string `json:"type"`
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
	Type     string `json:"type"`
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

// NewRichText creates a plain-text RichText segment.
func NewRichText(text string) RichText {
	return RichText{
		Type:        "text",
		PlainText:   text,
		Annotations: Annotations{},
	}
}

// NewFormattedRichText creates a RichText segment with annotations and optional href.
func NewFormattedRichText(text string, ann Annotations, href string) RichText {
	rt := RichText{
		Type:        "text",
		PlainText:   text,
		Annotations: ann,
	}
	if href != "" {
		rt.Href = &href
	}
	return rt
}

// NewEquationRichText creates an inline equation RichText segment.
func NewEquationRichText(expr string) RichText {
	return RichText{
		Type:      "equation",
		PlainText: expr,
		Equation:  &struct{ Expression string `json:"expression"` }{Expression: expr},
	}
}

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
		children, err := c.GetBlocks(ctx, blocks[i].ID)
		if err != nil {
			return nil, fmt.Errorf("get children of %s: %w", blocks[i].ID, err)
		}
		blocks[i].Children = children
	}
	return blocks, nil
}

// getBlocksPage fetches one paginated level of children.
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
		nil,
	)
}

// AppendBlocks appends blocks as children of parentID, after the block with
// afterID (empty string = prepend). Returns the created blocks with their IDs.
func (c *Client) AppendBlocks(ctx context.Context, parentID string, blocks []Block, afterID string) ([]Block, error) {
	children := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		children = append(children, blockToAPIMap(b))
	}

	body := map[string]any{"children": children}
	if afterID != "" {
		body["after"] = afterID
	}

	data, err := c.do(ctx, "PATCH", "/blocks/"+normalizeID(parentID)+"/children", body)
	if err != nil {
		return nil, fmt.Errorf("append blocks to %s: %w", parentID, err)
	}
	var resp listResponse[Block]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse append response: %w", err)
	}
	return resp.Results, nil
}

// UpdateBlock updates the content of an existing block in place.
func (c *Client) UpdateBlock(ctx context.Context, id string, b Block) error {
	body := blockToAPIMap(b)
	_, err := c.do(ctx, "PATCH", "/blocks/"+normalizeID(id), body)
	if err != nil {
		return fmt.Errorf("update block %s: %w", id, err)
	}
	return nil
}

// DeleteBlock archives (soft-deletes) a block.
func (c *Client) DeleteBlock(ctx context.Context, id string) error {
	_, err := c.do(ctx, "DELETE", "/blocks/"+normalizeID(id), nil)
	if err != nil {
		return fmt.Errorf("delete block %s: %w", id, err)
	}
	return nil
}

// blockToAPIMap converts a Block to the Notion API request format.
func blockToAPIMap(b Block) map[string]any {
	m := map[string]any{
		"object": "block",
		"type":   b.Type,
	}

	if b.Content == nil {
		m[b.Type] = map[string]any{}
		return m
	}

	switch c := b.Content.(type) {
	case *ParagraphContent:
		m["paragraph"] = map[string]any{
			"rich_text": richTextsToAPI(c.RichText),
		}
	case *HeadingContent:
		key := fmt.Sprintf("heading_%d", c.Level)
		m[key] = map[string]any{
			"rich_text":     richTextsToAPI(c.RichText),
			"is_toggleable": c.IsToggleable,
		}
	case *ListItemContent:
		content := map[string]any{"rich_text": richTextsToAPI(c.RichText)}
		if len(b.Children) > 0 {
			children := make([]map[string]any, 0, len(b.Children))
			for _, child := range b.Children {
				children = append(children, blockToAPIMap(child))
			}
			content["children"] = children
		}
		m["bulleted_list_item"] = content
	case *NumberedListItemContent:
		content := map[string]any{"rich_text": richTextsToAPI(c.RichText)}
		if len(b.Children) > 0 {
			children := make([]map[string]any, 0, len(b.Children))
			for _, child := range b.Children {
				children = append(children, blockToAPIMap(child))
			}
			content["children"] = children
		}
		m["numbered_list_item"] = content
	case *ToDoContent:
		m["to_do"] = map[string]any{
			"rich_text": richTextsToAPI(c.RichText),
			"checked":   c.Checked,
		}
	case *ToggleContent:
		content := map[string]any{"rich_text": richTextsToAPI(c.RichText)}
		if len(b.Children) > 0 {
			children := make([]map[string]any, 0, len(b.Children))
			for _, child := range b.Children {
				children = append(children, blockToAPIMap(child))
			}
			content["children"] = children
		}
		m["toggle"] = content
	case *QuoteContent:
		m["quote"] = map[string]any{"rich_text": richTextsToAPI(c.RichText)}
	case *CalloutContent:
		callout := map[string]any{"rich_text": richTextsToAPI(c.RichText)}
		if c.Icon != nil {
			callout["icon"] = c.Icon
		}
		m["callout"] = callout
	case *CodeContent:
		lang := c.Language
		if lang == "" {
			lang = "plain text"
		}
		m["code"] = map[string]any{
			"rich_text": richTextsToAPI(c.RichText),
			"language":  lang,
			"caption":   richTextsToAPI(c.Caption),
		}
	case *DividerContent:
		m["divider"] = map[string]any{}
	case *TableContent:
		m["table"] = map[string]any{
			"table_width":       c.TableWidth,
			"has_column_header": c.HasColumnHeader,
			"has_row_header":    c.HasRowHeader,
		}
		if len(b.Children) > 0 {
			children := make([]map[string]any, 0, len(b.Children))
			for _, child := range b.Children {
				children = append(children, blockToAPIMap(child))
			}
			m["children"] = children
		}
	case *TableRowContent:
		cells := make([][]map[string]any, 0, len(c.Cells))
		for _, cell := range c.Cells {
			cells = append(cells, richTextsToAPISlice(cell))
		}
		m["table_row"] = map[string]any{"cells": cells}
	case *EquationContent:
		m["equation"] = map[string]any{"expression": c.Expression}
	case *ImageContent:
		img := map[string]any{"type": c.Type}
		if c.External != nil {
			img["external"] = map[string]any{"url": c.External.URL}
		}
		if len(c.Caption) > 0 {
			img["caption"] = richTextsToAPI(c.Caption)
		}
		m["image"] = img
	case *BookmarkContent:
		m["bookmark"] = map[string]any{
			"url":     c.URL,
			"caption": richTextsToAPI(c.Caption),
		}
	default:
		m[b.Type] = map[string]any{}
	}

	return m
}

// richTextsToAPI converts []RichText to the Notion API format.
func richTextsToAPI(rts []RichText) []map[string]any {
	if len(rts) == 0 {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(rts))
	for _, rt := range rts {
		m := map[string]any{
			"type":       rt.Type,
			"plain_text": rt.PlainText,
			"annotations": map[string]any{
				"bold":          rt.Annotations.Bold,
				"italic":        rt.Annotations.Italic,
				"strikethrough": rt.Annotations.Strikethrough,
				"underline":     rt.Annotations.Underline,
				"code":          rt.Annotations.Code,
				"color":         "default",
			},
		}
		if rt.Type == "equation" && rt.Equation != nil {
			m["equation"] = map[string]any{"expression": rt.Equation.Expression}
		} else {
			text := map[string]any{"content": rt.PlainText, "link": nil}
			if rt.Href != nil && *rt.Href != "" {
				text["link"] = map[string]any{"url": *rt.Href}
			}
			m["text"] = text
		}
		if rt.Href != nil {
			m["href"] = *rt.Href
		}
		result = append(result, m)
	}
	return result
}

func richTextsToAPISlice(rts []RichText) []map[string]any {
	return richTextsToAPI(rts)
}
