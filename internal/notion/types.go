package notion

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Icon represents a page or block icon (emoji or external/file URL).
type Icon struct {
	Type     string `json:"type"`
	Emoji    string `json:"emoji,omitempty"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	File *struct {
		URL        string    `json:"url"`
		ExpiryTime time.Time `json:"expiry_time"`
	} `json:"file,omitempty"`
}

// Value returns the emoji string or URL, whichever is set.
func (i *Icon) Value() string {
	if i == nil {
		return ""
	}
	switch i.Type {
	case "emoji":
		return i.Emoji
	case "external":
		if i.External != nil {
			return i.External.URL
		}
	case "file":
		if i.File != nil {
			return i.File.URL
		}
	}
	return ""
}

// FileRef is a reference to a file hosted by Notion or externally.
type FileRef struct {
	Type     string `json:"type"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	File *struct {
		URL        string    `json:"url"`
		ExpiryTime time.Time `json:"expiry_time"`
	} `json:"file,omitempty"`
}

// URL returns the URL of the file reference.
func (f *FileRef) URL() string {
	if f == nil {
		return ""
	}
	switch f.Type {
	case "external":
		if f.External != nil {
			return f.External.URL
		}
	case "file":
		if f.File != nil {
			return f.File.URL
		}
	}
	return ""
}

// Parent describes what a page or block is nested inside.
type Parent struct {
	Type       string `json:"type"`
	PageID     string `json:"page_id,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
	Workspace  bool   `json:"workspace,omitempty"`
	BlockID    string `json:"block_id,omitempty"`
}

// ID returns the parent's ID (empty for workspace parents).
func (p Parent) ID() string {
	switch p.Type {
	case "page_id":
		return p.PageID
	case "database_id":
		return p.DatabaseID
	case "block_id":
		return p.BlockID
	}
	return ""
}

// RichText is a segment of styled text.
type RichText struct {
	Type        string      `json:"type"`
	PlainText   string      `json:"plain_text"`
	Href        *string     `json:"href"`
	Annotations Annotations `json:"annotations"`
	Text        *struct {
		Content string `json:"content"`
		Link    *struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"text,omitempty"`
	Mention  *json.RawMessage `json:"mention,omitempty"`
	Equation *struct {
		Expression string `json:"expression"`
	} `json:"equation,omitempty"`
}

// Annotations holds inline style flags for a RichText segment.
type Annotations struct {
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Strikethrough bool   `json:"strikethrough"`
	Underline     bool   `json:"underline"`
	Code          bool   `json:"code"`
	Color         string `json:"color"`
}

// Page represents a Notion page object.
type Page struct {
	Object         string                     `json:"object"`
	ID             string                     `json:"id"`
	CreatedTime    time.Time                  `json:"created_time"`
	LastEditedTime time.Time                  `json:"last_edited_time"`
	Archived       bool                       `json:"archived"`
	Icon           *Icon                      `json:"icon"`
	Cover          *FileRef                   `json:"cover"`
	Parent         Parent                     `json:"parent"`
	Properties     map[string]json.RawMessage `json:"properties"`
	URL            string                     `json:"url"`
}

// Title returns the page's title by scanning the properties map for the title property.
func (p *Page) Title() string {
	for _, v := range p.Properties {
		var prop struct {
			Type  string     `json:"type"`
			Title []RichText `json:"title"`
		}
		if err := json.Unmarshal(v, &prop); err != nil {
			continue
		}
		if prop.Type == "title" {
			var sb strings.Builder
			for _, rt := range prop.Title {
				sb.WriteString(rt.PlainText)
			}
			return sb.String()
		}
	}
	return ""
}

// NotionError is an API error returned by the Notion API.
type NotionError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *NotionError) Error() string {
	return fmt.Sprintf("notion: %s: %s", e.Code, redactTokens(e.Message))
}

// listResponse is the generic Notion paginated list envelope.
type listResponse[T any] struct {
	Object     string `json:"object"`
	Results    []T    `json:"results"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}
