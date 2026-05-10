package convert

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// PushPage converts a raw markdown document (with optional YAML frontmatter) into
// a Notion page and block tree.
func PushPage(rawMarkdown string) (notion.Page, []notion.Block, error) {
	rawMarkdown = strings.ReplaceAll(rawMarkdown, "\r\n", "\n")

	fm, body, err := ParseFrontmatter(rawMarkdown)
	if err != nil {
		return notion.Page{}, nil, fmt.Errorf("push: parse frontmatter: %w", err)
	}

	blocks, err := parseMarkdownBlocks(body)
	if err != nil {
		return notion.Page{}, nil, fmt.Errorf("push: parse blocks: %w", err)
	}

	page := buildPageFromFrontmatter(fm)
	return page, blocks, nil
}

// buildPageFromFrontmatter constructs a notion.Page from parsed frontmatter.
// Only the title property is populated; other fields are set by the Notion API.
func buildPageFromFrontmatter(fm Frontmatter) notion.Page {
	if fm.Title == "" {
		return notion.Page{}
	}
	titleRaw, _ := json.Marshal(map[string]any{
		"type": "title",
		"title": []map[string]any{{
			"type":       "text",
			"plain_text": fm.Title,
			"text":       map[string]any{"content": fm.Title, "link": nil},
			"annotations": map[string]bool{
				"bold": false, "italic": false,
				"strikethrough": false, "underline": false, "code": false,
			},
		}},
	})
	return notion.Page{
		Properties: map[string]json.RawMessage{
			"title": json.RawMessage(titleRaw),
		},
	}
}

// parseMarkdownBlocks parses a markdown body string into a slice of Notion blocks.
func parseMarkdownBlocks(body string) ([]notion.Block, error) {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	p := &lineParser{lines: lines}
	return p.parse(0)
}

// lineParser is a line-oriented state machine for converting markdown to blocks.
type lineParser struct {
	lines  []string
	pos    int
	anchor string // pending block ID from <!--nid:id-->
}

func (p *lineParser) peek() string {
	if p.pos >= len(p.lines) {
		return ""
	}
	return p.lines[p.pos]
}

func (p *lineParser) consume() string {
	line := p.lines[p.pos]
	p.pos++
	return line
}

func (p *lineParser) done() bool {
	return p.pos >= len(p.lines)
}

// parse processes lines at the given indentation prefix, stopping when a line
// doesn't start with the prefix (at indent > 0).
func (p *lineParser) parse(indent int) ([]notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	var blocks []notion.Block

	for !p.done() {
		raw := p.peek()

		// At deeper indentation, stop if we've left the indented region.
		if indent > 0 && raw != "" && !strings.HasPrefix(raw, prefix) {
			break
		}

		// Strip indent prefix for processing.
		line := strings.TrimPrefix(raw, prefix)

		// Skip blank lines.
		if strings.TrimSpace(line) == "" {
			p.consume()
			continue
		}

		// Anchor comment.
		if id := ExtractAnchorID(line); id != "" {
			p.anchor = id
			p.consume()
			continue
		}

		// Capture anchor BEFORE parsing so child block parsing can't clobber it.
		pendingAnchor := p.anchor
		p.anchor = ""

		// Parse one block.
		b, err := p.parseBlock(indent)
		if err != nil {
			return nil, err
		}

		// Attach the anchor captured before parsing.
		if pendingAnchor != "" {
			b.ID = pendingAnchor
		}

		blocks = append(blocks, b)
	}
	return blocks, nil
}

// parseBlock parses one block starting at the current position.
func (p *lineParser) parseBlock(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	line := strings.TrimPrefix(p.peek(), prefix)

	switch {
	case strings.HasPrefix(line, "<!-- unsupported block:") && strings.HasSuffix(line, "-->"):
		return p.parseUnsupportedComment(indent)

	case strings.HasPrefix(line, "<!--") && strings.HasSuffix(strings.TrimSpace(line), "-->"):
		p.consume()
		return notion.Block{Type: "unsupported", Content: &notion.UnsupportedContent{BlockType: "unsupported"}}, nil

	case line == "<details>":
		return p.parseToggle(indent)

	case strings.HasPrefix(line, "### "):
		return p.parseHeading(indent, 3)
	case strings.HasPrefix(line, "## "):
		return p.parseHeading(indent, 2)
	case strings.HasPrefix(line, "# "):
		return p.parseHeading(indent, 1)

	case strings.HasPrefix(line, "- [ ] ") || strings.HasPrefix(line, "- [x] ") || strings.HasPrefix(line, "- [X] "):
		return p.parseToDo(indent)

	case strings.HasPrefix(line, "- "):
		return p.parseBulletedListItem(indent)

	case numberedListPrefix(line) != "":
		return p.parseNumberedListItem(indent)

	case strings.HasPrefix(line, "> "):
		return p.parseQuoteOrCallout(indent)

	case strings.HasPrefix(line, "```"):
		return p.parseCodeBlock(indent)

	case line == "---":
		p.consume()
		return notion.Block{Type: "divider", Content: &notion.DividerContent{}}, nil

	case line == "$$":
		return p.parseEquationBlock(indent)

	case strings.HasPrefix(line, "|"):
		return p.parseTable(indent)

	case strings.HasPrefix(line, "!["):
		return p.parseImage(indent)

	case isStandaloneLink(line):
		return p.parseBookmark(indent)

	default:
		return p.parseParagraph(indent)
	}
}

// parseHeading parses a heading block at the given level.
func (p *lineParser) parseHeading(indent, level int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	hPrefix := strings.Repeat("#", level) + " "
	text := strings.TrimPrefix(line, hPrefix)

	return notion.Block{
		Type: fmt.Sprintf("heading_%d", level),
		Content: &notion.HeadingContent{
			Level:    level,
			RichText: parseInlineRichText(text),
		},
	}, nil
}

// parseBulletedListItem parses a `- text` line, collecting indented children.
func (p *lineParser) parseBulletedListItem(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)
	text := strings.TrimPrefix(line, "- ")

	b := notion.Block{
		Type:    "bulleted_list_item",
		Content: &notion.ListItemContent{RichText: parseInlineRichText(text)},
	}

	// Check for indented children (indent+2).
	children, err := p.parse(indent + 2)
	if err != nil {
		return b, err
	}
	if len(children) > 0 {
		b.Children = children
		b.HasChildren = true
	}

	return b, nil
}

// parseNumberedListItem parses a `N. text` line.
func (p *lineParser) parseNumberedListItem(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)
	// Strip "N. " prefix.
	dotIdx := strings.Index(line, ". ")
	text := line[dotIdx+2:]

	return notion.Block{
		Type:    "numbered_list_item",
		Content: &notion.NumberedListItemContent{RichText: parseInlineRichText(text)},
	}, nil
}

// parseToDo parses a `- [ ] text` or `- [x] text` line.
func (p *lineParser) parseToDo(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	checked := strings.HasPrefix(line, "- [x] ") || strings.HasPrefix(line, "- [X] ")
	var text string
	if checked {
		text = strings.TrimPrefix(strings.TrimPrefix(line, "- [x] "), "- [X] ")
	} else {
		text = strings.TrimPrefix(line, "- [ ] ")
	}

	return notion.Block{
		Type: "to_do",
		Content: &notion.ToDoContent{
			RichText: parseInlineRichText(text),
			Checked:  checked,
		},
	}, nil
}

// parseQuoteOrCallout parses `> text` as quote, or `> emoji text` as callout.
func (p *lineParser) parseQuoteOrCallout(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)

	// Collect all consecutive `> ` lines.
	var textLines []string
	for !p.done() {
		raw := p.peek()
		line := strings.TrimPrefix(raw, prefix)
		if !strings.HasPrefix(line, "> ") {
			break
		}
		textLines = append(textLines, strings.TrimPrefix(line, "> "))
		p.consume()
	}

	text := strings.Join(textLines, "\n")

	// Detect callout: text starts with an emoji (rune > U+1F000 or common emoji range).
	if r, ok := startsWithEmoji(text); ok {
		_, size := utf8.DecodeRuneInString(text)
		rest := text[size:]
		if len(rest) > 0 && rest[0] == ' ' {
			rest = rest[1:]
		}
		return notion.Block{
			Type: "callout",
			Content: &notion.CalloutContent{
				RichText: parseInlineRichText(rest),
				Icon:     &notion.Icon{Type: "emoji", Emoji: string(r)},
			},
		}, nil
	}

	return notion.Block{
		Type: "quote",
		Content: &notion.QuoteContent{
			RichText: parseInlineRichText(text),
		},
	}, nil
}

// parseCodeBlock parses a fenced code block.
func (p *lineParser) parseCodeBlock(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	lang := strings.TrimPrefix(line, "```")

	var codeLines []string
	for !p.done() {
		raw = p.consume()
		line = strings.TrimPrefix(raw, prefix)
		if line == "```" {
			break
		}
		codeLines = append(codeLines, line)
	}

	code := strings.Join(codeLines, "\n")
	rts := []notion.RichText{notion.NewRichText(code)}

	return notion.Block{
		Type: "code",
		Content: &notion.CodeContent{
			Language: lang,
			RichText: rts,
		},
	}, nil
}

// parseEquationBlock parses a `$$\n...\n$$` block equation.
func (p *lineParser) parseEquationBlock(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	p.consume() // consume opening `$$`

	var exprLines []string
	for !p.done() {
		raw := p.consume()
		line := strings.TrimPrefix(raw, prefix)
		if line == "$$" {
			break
		}
		exprLines = append(exprLines, line)
	}

	return notion.Block{
		Type:    "equation",
		Content: &notion.EquationContent{Expression: strings.Join(exprLines, "\n")},
	}, nil
}

// parseTable parses a markdown table into a Notion table block with row children.
func (p *lineParser) parseTable(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)

	var rows [][]notion.RichText
	hasHeader := false
	firstRow := true

	for !p.done() {
		raw := p.peek()
		line := strings.TrimPrefix(raw, prefix)
		if !strings.HasPrefix(line, "|") {
			break
		}
		p.consume()

		// Detect separator row: | --- | --- |
		if isSeparatorRow(line) {
			hasHeader = true
			firstRow = false
			continue
		}
		if firstRow {
			firstRow = false
		}

		cells := parseTableRow(line)
		rows = append(rows, cells)
	}

	tableContent := &notion.TableContent{
		TableWidth:      0,
		HasColumnHeader: hasHeader,
	}
	if len(rows) > 0 {
		tableContent.TableWidth = len(rows[0])
	}

	// Build child table_row blocks.
	children := make([]notion.Block, 0, len(rows))
	for _, row := range rows {
		cells := make([][]notion.RichText, 0, len(row))
		for _, cell := range row {
			cells = append(cells, []notion.RichText{cell})
		}
		children = append(children, notion.Block{
			Type:    "table_row",
			Content: &notion.TableRowContent{Cells: cells},
		})
	}

	return notion.Block{
		Type:        "table",
		Content:     tableContent,
		HasChildren: len(children) > 0,
		Children:    children,
	}, nil
}

// parseImage parses a `![alt](url)` line into an image block.
func (p *lineParser) parseImage(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	_, url := parseImageMarkdown(line)

	// Check for optional caption on next line: `*caption*`
	var caption []notion.RichText
	if !p.done() {
		next := strings.TrimPrefix(p.peek(), prefix)
		if strings.HasPrefix(next, "*") && strings.HasSuffix(next, "*") && len(next) > 2 {
			p.consume()
			capText := next[1 : len(next)-1]
			caption = []notion.RichText{notion.NewRichText(capText)}
		}
	}

	return notion.Block{
		Type: "image",
		Content: &notion.ImageContent{
			Type: "external",
			External: &struct {
				URL string `json:"url"`
			}{URL: url},
			Caption: caption,
		},
	}, nil
}

// parseBookmark parses a `[caption](url)` standalone link as a bookmark.
func (p *lineParser) parseBookmark(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	caption, url := parseLinkMarkdown(line)

	var capRT []notion.RichText
	if caption != "" && caption != url {
		capRT = []notion.RichText{notion.NewRichText(caption)}
	}

	return notion.Block{
		Type: "bookmark",
		Content: &notion.BookmarkContent{
			URL:     url,
			Caption: capRT,
		},
	}, nil
}

// parseToggle parses a `<details>\n<summary>text</summary>\n...\n</details>` block.
func (p *lineParser) parseToggle(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	p.consume() // consume `<details>`

	// Parse `<summary>text</summary>`.
	var summaryText string
	if !p.done() {
		raw := p.consume()
		line := strings.TrimPrefix(raw, prefix)
		if strings.HasPrefix(line, "<summary>") && strings.HasSuffix(line, "</summary>") {
			summaryText = line[len("<summary>") : len(line)-len("</summary>")]
		}
	}

	// Parse body until `</details>`.
	var bodyLines []string
	for !p.done() {
		raw := p.peek()
		line := strings.TrimPrefix(raw, prefix)
		if line == "</details>" {
			p.consume()
			break
		}
		bodyLines = append(bodyLines, raw)
		p.consume()
	}

	// Parse body as child blocks.
	bodyText := strings.Join(bodyLines, "\n")
	children, err := parseMarkdownBlocks(bodyText)
	if err != nil {
		return notion.Block{}, err
	}

	return notion.Block{
		Type: "toggle",
		Content: &notion.ToggleContent{
			RichText: parseInlineRichText(summaryText),
		},
		HasChildren: len(children) > 0,
		Children:    children,
	}, nil
}

// parseUnsupportedComment parses a `<!-- unsupported block: type -->` line.
func (p *lineParser) parseUnsupportedComment(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)
	raw := p.consume()
	line := strings.TrimPrefix(raw, prefix)

	// Extract type from `<!-- unsupported block: TYPE -->`
	const pfx = "<!-- unsupported block: "
	const sfx = " -->"
	blockType := line[len(pfx) : len(line)-len(sfx)]

	return notion.Block{
		Type:    blockType,
		Content: &notion.UnsupportedContent{BlockType: blockType},
	}, nil
}

// parseParagraph collects lines until an empty line into a paragraph block.
func (p *lineParser) parseParagraph(indent int) (notion.Block, error) {
	prefix := strings.Repeat(" ", indent)

	var textLines []string
	for !p.done() {
		raw := p.peek()
		line := strings.TrimPrefix(raw, prefix)

		// Stop at empty lines or block-starting prefixes.
		if line == "" || isBlockStart(line) {
			break
		}
		textLines = append(textLines, line)
		p.consume()
	}

	text := strings.Join(textLines, "\n")
	// Convert trailing-space markdown hard breaks back to embedded newlines.
	text = strings.ReplaceAll(text, "  \n", "\n")
	return notion.Block{
		Type:    "paragraph",
		Content: &notion.ParagraphContent{RichText: parseInlineRichText(text)},
	}, nil
}

// --- Inline parser ---

// parseInlineRichText converts a markdown inline string to Notion RichText segments.
func parseInlineRichText(text string) []notion.RichText {
	if text == "" {
		return nil
	}

	var result []notion.RichText
	i := 0
	plainStart := 0

	flushPlain := func(end int) {
		if plainStart < end {
			s := text[plainStart:end]
			if s != "" {
				result = append(result, notion.NewRichText(s))
			}
		}
		plainStart = end
	}

	for i < len(text) {
		rest := text[i:]
		var seg notion.RichText
		var consumed int

		switch {
		case strings.HasPrefix(rest, "***"):
			seg, consumed = tryBoldItalic(rest)
		case strings.HasPrefix(rest, "**"):
			seg, consumed = tryBold(rest)
		case strings.HasPrefix(rest, "_"):
			seg, consumed = tryItalic(rest)
		case strings.HasPrefix(rest, "~~"):
			seg, consumed = tryStrikethrough(rest)
		case strings.HasPrefix(rest, "`"):
			seg, consumed = tryCode(rest)
		case strings.HasPrefix(rest, "["):
			seg, consumed = tryLink(rest)
		case strings.HasPrefix(rest, "$"):
			seg, consumed = tryInlineEquation(rest)
		}

		if consumed > 0 {
			flushPlain(i)
			result = append(result, seg)
			i += consumed
			plainStart = i
		} else {
			_, size := utf8.DecodeRuneInString(rest)
			i += size
		}
	}
	flushPlain(len(text))

	if len(result) == 0 {
		return []notion.RichText{notion.NewRichText(text)}
	}
	return result
}

func tryBoldItalic(text string) (notion.RichText, int) {
	// text starts with "***"
	const marker = "***"
	end := strings.Index(text[3:], marker)
	if end < 0 {
		return notion.RichText{}, 0
	}
	content := text[3 : 3+end]
	ann := notion.Annotations{Bold: true, Italic: true}
	return notion.NewFormattedRichText(content, ann, ""), 3 + end + 3
}

func tryBold(text string) (notion.RichText, int) {
	const marker = "**"
	end := strings.Index(text[2:], marker)
	if end < 0 {
		return notion.RichText{}, 0
	}
	content := text[2 : 2+end]
	ann := notion.Annotations{Bold: true}
	return notion.NewFormattedRichText(content, ann, ""), 2 + end + 2
}

func tryItalic(text string) (notion.RichText, int) {
	// Match _word_ (underscore italic)
	if len(text) < 3 || text[0] != '_' {
		return notion.RichText{}, 0
	}
	end := strings.Index(text[1:], "_")
	if end < 0 {
		return notion.RichText{}, 0
	}
	content := text[1 : 1+end]
	if content == "" {
		return notion.RichText{}, 0
	}
	ann := notion.Annotations{Italic: true}
	return notion.NewFormattedRichText(content, ann, ""), 1 + end + 1
}

func tryStrikethrough(text string) (notion.RichText, int) {
	const marker = "~~"
	end := strings.Index(text[2:], marker)
	if end < 0 {
		return notion.RichText{}, 0
	}
	content := text[2 : 2+end]
	ann := notion.Annotations{Strikethrough: true}
	return notion.NewFormattedRichText(content, ann, ""), 2 + end + 2
}

func tryCode(text string) (notion.RichText, int) {
	if len(text) < 2 || text[0] != '`' {
		return notion.RichText{}, 0
	}
	// Don't treat ``` as inline code (that's a code block marker).
	if strings.HasPrefix(text, "```") {
		return notion.RichText{}, 0
	}
	end := strings.Index(text[1:], "`")
	if end < 0 {
		return notion.RichText{}, 0
	}
	content := text[1 : 1+end]
	ann := notion.Annotations{Code: true}
	rt := notion.NewFormattedRichText(content, ann, "")
	return rt, 1 + end + 1
}

func tryLink(text string) (notion.RichText, int) {
	// Match [text](url)
	if len(text) < 4 || text[0] != '[' {
		return notion.RichText{}, 0
	}
	closeIdx := strings.Index(text, "](")
	if closeIdx < 0 {
		return notion.RichText{}, 0
	}
	linkText := text[1:closeIdx]
	rest := text[closeIdx+2:]
	urlEnd := strings.Index(rest, ")")
	if urlEnd < 0 {
		return notion.RichText{}, 0
	}
	url := rest[:urlEnd]
	total := 1 + closeIdx + 1 + urlEnd + 1 // [text](url)

	// Handle `[code](url)` where linkText is code-formatted.
	if strings.HasPrefix(linkText, "`") && strings.HasSuffix(linkText, "`") && len(linkText) > 2 {
		inner := linkText[1 : len(linkText)-1]
		ann := notion.Annotations{Code: true}
		return notion.NewFormattedRichText(inner, ann, url), total
	}

	return notion.NewFormattedRichText(linkText, notion.Annotations{}, url), total
}

func tryInlineEquation(text string) (notion.RichText, int) {
	if len(text) < 2 || text[0] != '$' {
		return notion.RichText{}, 0
	}
	// Don't match $$ (block equation marker).
	if strings.HasPrefix(text, "$$") {
		return notion.RichText{}, 0
	}
	end := strings.Index(text[1:], "$")
	if end < 0 {
		return notion.RichText{}, 0
	}
	expr := text[1 : 1+end]
	return notion.NewEquationRichText(expr), 1 + end + 1
}

// --- Helpers ---

var numberedListRe = regexp.MustCompile(`^\d+\. `)

// numberedListPrefix returns the "N. " prefix if line is a numbered list item.
func numberedListPrefix(line string) string {
	loc := numberedListRe.FindString(line)
	return loc
}

// isSeparatorRow returns true if line is a markdown table separator (| --- | --- |).
func isSeparatorRow(line string) bool {
	cells := strings.Split(strings.Trim(line, "|"), "|")
	for _, c := range cells {
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			continue
		}
		// Must be only dashes (with optional colons for alignment).
		cleaned := strings.Trim(trimmed, ":-")
		if cleaned != "" {
			return false
		}
	}
	return true
}

// parseTableRow splits a `| a | b | c |` line into RichText cells.
func parseTableRow(line string) []notion.RichText {
	// Trim leading/trailing `|`.
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]notion.RichText, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, notion.NewRichText(strings.TrimSpace(p)))
	}
	return cells
}

// isStandaloneLink returns true if line is a `[text](url)` link with nothing else.
func isStandaloneLink(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	closeIdx := strings.Index(line, "](")
	if closeIdx < 0 {
		return false
	}
	rest := line[closeIdx+2:]
	urlEnd := strings.Index(rest, ")")
	if urlEnd < 0 {
		return false
	}
	return urlEnd == len(rest)-1
}

// parseLinkMarkdown extracts (caption, url) from a `[caption](url)` string.
func parseLinkMarkdown(line string) (caption, url string) {
	closeIdx := strings.Index(line, "](")
	if closeIdx < 0 {
		return "", line
	}
	caption = line[1:closeIdx]
	rest := line[closeIdx+2:]
	urlEnd := strings.Index(rest, ")")
	if urlEnd < 0 {
		return caption, rest
	}
	return caption, rest[:urlEnd]
}

// parseImageMarkdown extracts (alt, url) from `![alt](url)`.
func parseImageMarkdown(line string) (alt, url string) {
	// `![alt](url)` → alt = text between `![` and `]`, url = between `(` and `)`
	closeIdx := strings.Index(line, "](")
	if closeIdx < 0 {
		return "", ""
	}
	alt = line[2:closeIdx] // skip `![`
	rest := line[closeIdx+2:]
	urlEnd := strings.Index(rest, ")")
	if urlEnd < 0 {
		return alt, rest
	}
	return alt, rest[:urlEnd]
}

// startsWithEmoji returns the first rune and true if text starts with an emoji.
// Emoji detection: rune > U+1F000 (supplementary emoji plane).
func startsWithEmoji(text string) (rune, bool) {
	r, _ := utf8.DecodeRuneInString(text)
	if r == utf8.RuneError {
		return 0, false
	}
	// Check supplementary emoji plane and common emoji ranges.
	if r >= 0x1F000 || (r >= 0x2600 && r <= 0x27FF) {
		return r, true
	}
	return 0, false
}

// isBlockStart returns true if line begins a new block that shouldn't be
// consumed as part of a paragraph.
func isBlockStart(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "- [") {
		return true
	}
	if numberedListPrefix(line) != "" {
		return true
	}
	if strings.HasPrefix(line, "> ") {
		return true
	}
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "$$") {
		return true
	}
	if line == "---" {
		return true
	}
	if strings.HasPrefix(line, "|") {
		return true
	}
	if strings.HasPrefix(line, "![") {
		return true
	}
	if strings.HasPrefix(line, "<!--") {
		return true
	}
	if line == "<details>" {
		return true
	}
	return false
}
