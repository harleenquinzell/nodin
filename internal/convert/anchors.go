package convert

import (
	"fmt"
	"strings"

	"github.com/harleenquinzell/nodin/internal/notion"
)

const anchorPrefix = "<!--nid:"
const anchorSuffix = "-->"

// AnchorRules controls whether and where anchor comments are emitted.
type AnchorRules struct {
	Enabled bool
}

// DefaultAnchorRules returns the default anchor thinning rules (anchors enabled).
func DefaultAnchorRules() AnchorRules {
	return AnchorRules{Enabled: true}
}

// anchorComment returns the HTML comment for a block ID, e.g. <!--nid:abc123-->.
func anchorComment(id string) string {
	return fmt.Sprintf("%s%s%s", anchorPrefix, id, anchorSuffix)
}

// ShouldAnchor returns true if a block of the given type and content should
// have an anchor emitted immediately before it.
// For list items, only the first item in a run is anchored (handled by the caller).
func ShouldAnchor(b notion.Block, isFirstInListRun bool) bool {
	switch b.Type {
	case "heading_1", "heading_2", "heading_3",
		"code", "quote", "callout", "toggle",
		"image", "file", "bookmark", "equation",
		"table":
		return true

	case "bulleted_list_item", "numbered_list_item", "to_do":
		return isFirstInListRun

	case "paragraph":
		// Anchor paragraphs that have formatting or hard line breaks.
		if pc, ok := b.Content.(*notion.ParagraphContent); ok {
			return paragraphNeedsAnchor(pc.RichText)
		}
		return false

	case "divider":
		return false

	case "table_row", "child_page", "child_database":
		return false

	default:
		// Unknown/unsupported blocks always get an anchor.
		return true
	}
}

// paragraphNeedsAnchor returns true if the paragraph has any inline formatting
// (bold, italic, code, strikethrough, links) or contains a hard line break.
func paragraphNeedsAnchor(rts []notion.RichText) bool {
	for _, rt := range rts {
		if rt.Annotations.Bold || rt.Annotations.Italic ||
			rt.Annotations.Code || rt.Annotations.Strikethrough {
			return true
		}
		if rt.Href != nil && *rt.Href != "" {
			return true
		}
		if rt.Type == "equation" {
			return true
		}
		if strings.Contains(rt.PlainText, "\n") {
			return true
		}
	}
	return false
}

// ExtractAnchorID parses an anchor comment and returns the block ID.
// Returns "" if the line is not an anchor comment.
func ExtractAnchorID(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, anchorPrefix) {
		return ""
	}
	if !strings.HasSuffix(line, anchorSuffix) {
		return ""
	}
	return line[len(anchorPrefix) : len(line)-len(anchorSuffix)]
}
