package convert

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds the parsed YAML front matter of a markdown file.
type Frontmatter struct {
	Title      string
	Properties map[string]any
	Computed   map[string]any
	Extra      map[string]any // unknown keys not under properties:/computed:
}

// ParseFrontmatter splits a raw markdown document into front matter and body.
// The front matter must be delimited by "---" lines. Returns empty Frontmatter
// and the full content as body if no delimiter is found.
func ParseFrontmatter(raw string) (Frontmatter, string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")

	if !strings.HasPrefix(raw, "---\n") {
		return Frontmatter{}, raw, nil
	}

	rest := raw[4:] // skip first "---\n"
	end := strings.Index(rest, "\n---\n")
	var yamlSrc, body string
	if end == -1 {
		// Check if document ends with ---
		if strings.HasSuffix(strings.TrimRight(rest, "\n"), "\n---") ||
			strings.TrimRight(rest, "\n") == "---" {
			idx := strings.LastIndex(strings.TrimRight(rest, "\n"), "\n---")
			if idx >= 0 {
				yamlSrc = rest[:idx]
				body = ""
			} else {
				return Frontmatter{}, raw, nil
			}
		} else {
			return Frontmatter{}, raw, nil
		}
	} else {
		yamlSrc = rest[:end]
		body = rest[end+5:] // skip "\n---\n"
	}

	fm, err := parseFrontmatterYAML(yamlSrc)
	if err != nil {
		return Frontmatter{}, raw, fmt.Errorf("parse frontmatter: %w", err)
	}

	return fm, body, nil
}

func parseFrontmatterYAML(src string) (Frontmatter, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(src), &raw); err != nil {
		return Frontmatter{}, err
	}
	if raw == nil {
		return Frontmatter{}, nil
	}

	fm := Frontmatter{
		Extra: map[string]any{},
	}

	if t, ok := raw["title"].(string); ok {
		fm.Title = t
	}

	if props, ok := raw["properties"].(map[string]any); ok {
		fm.Properties = props
	}

	if comp, ok := raw["computed"].(map[string]any); ok {
		fm.Computed = comp
	}

	for k, v := range raw {
		switch k {
		case "title", "properties", "computed":
			// handled above
		default:
			fm.Extra[k] = v
		}
	}

	return fm, nil
}

// RenderFrontmatter serializes a Frontmatter struct into a YAML front matter block.
func RenderFrontmatter(fm Frontmatter) string {
	out := map[string]any{}

	if fm.Title != "" {
		out["title"] = fm.Title
	}

	for k, v := range fm.Extra {
		out[k] = v
	}

	if len(fm.Properties) > 0 {
		out["properties"] = fm.Properties
	}

	if len(fm.Computed) > 0 {
		out["computed"] = fm.Computed
	}

	if len(out) == 0 {
		return ""
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return ""
	}

	return "---\n" + strings.TrimRight(string(data), "\n") + "\n---\n"
}
