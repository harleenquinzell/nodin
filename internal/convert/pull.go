package convert

import (
	"fmt"
	"strings"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// PullOptions configures the pull conversion.
type PullOptions struct {
	AnchorRules    AnchorRules
	DownloadAssets bool
}

// AssetRef holds a reference to an asset that should be downloaded.
type AssetRef struct {
	URL       string // original Notion URL
	FileID    string // derived from block ID for naming
	Extension string // e.g. "png"
}

// ConvertedPage holds the result of converting a Notion page to markdown.
type ConvertedPage struct {
	Frontmatter string
	Body        string
	AssetRefs   []AssetRef
}

// BlockMarkdown renders a single block to markdown for comparison purposes.
// Used by blockdiff; no anchors are emitted.
func BlockMarkdown(b notion.Block) string {
	md, _, _ := pullBlock(b, PullOptions{}, 0)
	return md
}

// PullPage converts a Notion page and its blocks to a ConvertedPage.
// Asset URLs are left as-is; AssetRefs describes what to download.
func PullPage(p notion.Page, blocks []notion.Block, opts PullOptions) (ConvertedPage, error) {
	cp := ConvertedPage{}

	fm := Frontmatter{
		Title: p.Title(),
	}
	cp.Frontmatter = RenderFrontmatter(fm)

	body, refs, err := pullBlocks(blocks, opts, false)
	if err != nil {
		return cp, err
	}
	cp.Body = body
	cp.AssetRefs = refs
	return cp, nil
}

// pullBlocks converts a slice of blocks to markdown text.
// inList tracks whether we're inside a list context (for anchor thinning).
func pullBlocks(blocks []notion.Block, opts PullOptions, _ bool) (string, []AssetRef, error) {
	var sb strings.Builder
	var refs []AssetRef

	// Track list runs for anchor thinning and trailing blank lines.
	type listState struct {
		typ string // "bulleted_list_item" | "numbered_list_item" | "to_do"
		seq int    // 1-based counter for numbered lists
	}
	var cur listState
	prevWasList := false

	for i, b := range blocks {
		isListType := b.Type == "bulleted_list_item" || b.Type == "numbered_list_item" || b.Type == "to_do"
		isFirstInRun := false

		// When transitioning from list to non-list, add a blank line to end the list.
		if prevWasList && !isListType {
			sb.WriteByte('\n')
		}

		if isListType {
			if cur.typ != b.Type {
				// New list run started.
				cur.typ = b.Type
				cur.seq = 1
				isFirstInRun = true
			} else {
				cur.seq++
			}
		} else {
			cur = listState{}
		}
		prevWasList = isListType

		// Emit anchor if rules say so.
		if opts.AnchorRules.Enabled && b.ID != "" {
			if ShouldAnchor(b, isFirstInRun) {
				sb.WriteString(anchorComment(b.ID))
				sb.WriteByte('\n')
			}
		}

		md, blockRefs, err := pullBlock(b, opts, cur.seq)
		if err != nil {
			return "", nil, fmt.Errorf("block %d (%s): %w", i, b.Type, err)
		}
		sb.WriteString(md)
		refs = append(refs, blockRefs...)
	}

	// End a trailing list run with a blank line.
	if prevWasList {
		sb.WriteByte('\n')
	}

	return sb.String(), refs, nil
}

// pullBlock converts a single block to its markdown representation.
// seq is the 1-based counter for numbered list items.
func pullBlock(b notion.Block, opts PullOptions, seq int) (string, []AssetRef, error) {
	switch c := b.Content.(type) {
	case *notion.ParagraphContent:
		text := RenderRichText(c.RichText)
		if text == "" {
			return "\n", nil, nil
		}
		return text + "\n\n", nil, nil

	case *notion.HeadingContent:
		prefix := strings.Repeat("#", c.Level)
		text := RenderRichText(c.RichText)
		return prefix + " " + text + "\n\n", nil, nil

	case *notion.ListItemContent:
		text := RenderRichText(c.RichText)
		result := "- " + text + "\n"
		if b.HasChildren && len(b.Children) > 0 {
			childMD, _, err := pullBlocksIndented(b.Children, opts)
			if err != nil {
				return "", nil, err
			}
			result += childMD
		}
		return result, nil, nil

	case *notion.NumberedListItemContent:
		text := RenderRichText(c.RichText)
		result := fmt.Sprintf("%d. %s\n", seq, text)
		if b.HasChildren && len(b.Children) > 0 {
			childMD, _, err := pullBlocksIndented(b.Children, opts)
			if err != nil {
				return "", nil, err
			}
			result += childMD
		}
		return result, nil, nil

	case *notion.ToDoContent:
		text := RenderRichText(c.RichText)
		check := " "
		if c.Checked {
			check = "x"
		}
		result := fmt.Sprintf("- [%s] %s\n", check, text)
		return result, nil, nil

	case *notion.ToggleContent:
		text := RenderRichText(c.RichText)
		result := "<details>\n<summary>" + text + "</summary>\n\n"
		if b.HasChildren && len(b.Children) > 0 {
			childMD, _, err := pullBlocks(b.Children, opts, false)
			if err != nil {
				return "", nil, err
			}
			result += childMD
		}
		result += "</details>\n\n"
		return result, nil, nil

	case *notion.QuoteContent:
		text := RenderRichText(c.RichText)
		lines := strings.Split(text, "\n")
		var quoted []string
		for _, l := range lines {
			quoted = append(quoted, "> "+l)
		}
		return strings.Join(quoted, "\n") + "\n\n", nil, nil

	case *notion.CalloutContent:
		icon := ""
		if c.Icon != nil {
			icon = c.Icon.Value()
		}
		text := RenderRichText(c.RichText)
		prefix := "> "
		if icon != "" {
			prefix = "> " + icon + " "
		}
		lines := strings.Split(text, "\n")
		var quoted []string
		for _, l := range lines {
			quoted = append(quoted, prefix+l)
		}
		return strings.Join(quoted, "\n") + "\n\n", nil, nil

	case *notion.CodeContent:
		lang := c.Language
		if lang == "plain text" {
			lang = ""
		}
		text := RenderRichText(c.RichText)
		return "```" + lang + "\n" + text + "\n```\n\n", nil, nil

	case *notion.DividerContent:
		return "---\n\n", nil, nil

	case *notion.TableContent:
		return pullTable(b, c, opts)

	case *notion.EquationContent:
		return "$$\n" + c.Expression + "\n$$\n\n", nil, nil

	case *notion.ImageContent:
		url := c.URL()
		caption := RenderCaption(c.Caption)
		altText := caption
		if altText == "" {
			altText = "image"
		}
		var refs []AssetRef
		if opts.DownloadAssets && url != "" {
			refs = append(refs, AssetRef{
				URL:       url,
				FileID:    b.ID,
				Extension: guessExtension(url),
			})
		}
		if caption != "" {
			return fmt.Sprintf("![%s](%s)\n*%s*\n\n", altText, url, caption), refs, nil
		}
		return fmt.Sprintf("![%s](%s)\n\n", altText, url), refs, nil

	case *notion.FileBlockContent:
		url := c.URL()
		name := c.Name
		if name == "" {
			name = "file"
		}
		caption := RenderCaption(c.Caption)
		var refs []AssetRef
		if opts.DownloadAssets && url != "" {
			refs = append(refs, AssetRef{
				URL:       url,
				FileID:    b.ID,
				Extension: guessExtension(url),
			})
		}
		if caption != "" {
			return fmt.Sprintf("[%s](%s) — %s\n\n", name, url, caption), refs, nil
		}
		return fmt.Sprintf("[%s](%s)\n\n", name, url), refs, nil

	case *notion.BookmarkContent:
		caption := RenderCaption(c.Caption)
		if caption != "" {
			return fmt.Sprintf("[%s](%s)\n\n", caption, c.URL), nil, nil
		}
		return fmt.Sprintf("[%s](%s)\n\n", c.URL, c.URL), nil, nil

	case *notion.ChildPageContent:
		return fmt.Sprintf("<!-- child page: %s -->\n\n", c.Title), nil, nil

	case *notion.ChildDatabaseContent:
		return fmt.Sprintf("<!-- child database: %s -->\n\n", c.Title), nil, nil

	case *notion.UnsupportedContent:
		return fmt.Sprintf("<!-- unsupported block: %s -->\n\n", c.BlockType), nil, nil

	default:
		return fmt.Sprintf("<!-- unsupported block: %s -->\n\n", b.Type), nil, nil
	}
}

// pullBlocksIndented renders children of a list item with 2-space indentation.
func pullBlocksIndented(blocks []notion.Block, opts PullOptions) (string, []AssetRef, error) {
	raw, refs, err := pullBlocks(blocks, opts, false)
	if err != nil {
		return "", nil, err
	}
	// Trim trailing blank lines so the parent item doesn't get an extra gap.
	raw = strings.TrimRight(raw, "\n") + "\n"
	// Indent each line by 2 spaces.
	lines := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	var out []string
	for _, l := range lines {
		if l != "" {
			out = append(out, "  "+l)
		} else {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n") + "\n", refs, nil
}

// pullTable renders a table block, using its children as rows.
func pullTable(b notion.Block, c *notion.TableContent, _ PullOptions) (string, []AssetRef, error) {
	rows := b.Children
	if len(rows) == 0 {
		return "", nil, nil
	}

	var sb strings.Builder

	for i, row := range rows {
		trc, ok := row.Content.(*notion.TableRowContent)
		if !ok {
			continue
		}

		// Render cells.
		var cells []string
		for _, cell := range trc.Cells {
			cells = append(cells, RenderRichText(cell))
		}
		sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")

		// After header row, emit separator.
		if i == 0 && c.HasColumnHeader {
			sep := make([]string, len(cells))
			for j := range sep {
				sep[j] = "---"
			}
			sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}
	}

	sb.WriteByte('\n')
	return sb.String(), nil, nil
}

// guessExtension returns a file extension guessed from the URL path.
func guessExtension(url string) string {
	// Strip query string.
	if idx := strings.Index(url, "?"); idx >= 0 {
		url = url[:idx]
	}
	if idx := strings.LastIndex(url, "."); idx >= 0 {
		ext := url[idx:]
		if len(ext) <= 5 && strings.IndexByte(ext, '/') == -1 {
			return ext
		}
	}
	return ""
}
