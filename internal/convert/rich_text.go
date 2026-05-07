package convert

import (
	"strings"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// RenderRichText converts a slice of Notion RichText segments to markdown inline text.
func RenderRichText(rts []notion.RichText) string {
	var sb strings.Builder
	for _, rt := range rts {
		sb.WriteString(renderSegment(rt))
	}
	return sb.String()
}

// renderSegment converts one RichText segment to its markdown representation.
func renderSegment(rt notion.RichText) string {
	// Inline equations
	if rt.Type == "equation" && rt.Equation != nil {
		return "$" + rt.Equation.Expression + "$"
	}

	// Mentions: emit plain text
	if rt.Type == "mention" {
		return rt.PlainText
	}

	text := rt.PlainText
	if text == "" {
		return ""
	}

	a := rt.Annotations

	// Wrap with code first (innermost) since backticks don't nest.
	if a.Code {
		text = "`" + text + "`"
		// Links still apply around code spans.
		if rt.Href != nil && *rt.Href != "" {
			text = "[" + text + "](" + *rt.Href + ")"
		}
		return text
	}

	// Apply bold, italic, strikethrough (order: bold > italic > strikethrough > underline).
	// We use the CommonMark convention: ** for bold, _ for italic (but * also valid).
	if a.Bold && a.Italic {
		text = "***" + text + "***"
	} else if a.Bold {
		text = "**" + text + "**"
	} else if a.Italic {
		text = "_" + text + "_"
	}

	if a.Strikethrough {
		text = "~~" + text + "~~"
	}

	// Notion underline has no standard markdown equivalent; skip.

	// Links
	if rt.Href != nil && *rt.Href != "" {
		text = "[" + text + "](" + *rt.Href + ")"
	}

	return text
}

// RenderCaption renders a caption []RichText into a plain-text string.
func RenderCaption(rts []notion.RichText) string {
	return RenderRichText(rts)
}
