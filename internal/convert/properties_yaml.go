package convert

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// ErrComputedModified is returned when a computed property appears in the
// editable properties section, indicating the user has moved it.
var ErrComputedModified = errors.New("computed property found in editable properties section")

// PropertiesToYAML converts a map of PropertyValues to a YAML-serializable map.
// Computed properties are placed under a separate "computed" map.
func PropertiesToYAML(props map[string]notion.PropertyValue) (editable map[string]any, computed map[string]any) {
	editable = map[string]any{}
	computed = map[string]any{}

	for name, pv := range props {
		if pv.Computed {
			computed[name] = propertyToYAMLValue(pv)
		} else {
			editable[name] = propertyToYAMLValue(pv)
		}
	}
	return editable, computed
}

// propertyToYAMLValue returns the YAML-serializable value for a PropertyValue.
func propertyToYAMLValue(pv notion.PropertyValue) any {
	switch pv.Type {
	case "title", "rich_text", "url", "email", "phone_number",
		"created_time", "last_edited_time", "created_by", "last_edited_by":
		return pv.Text

	case "number":
		if pv.Number == nil {
			return nil
		}
		return *pv.Number

	case "select", "status":
		return pv.Select

	case "multi_select":
		if len(pv.MultiSel) == 0 {
			return []string{}
		}
		return pv.MultiSel

	case "date":
		if pv.Date == nil {
			return nil
		}
		d := pv.Date
		if d.End == "" && d.TZ == "" {
			return d.Start
		}
		m := map[string]string{"start": d.Start}
		if d.End != "" {
			m["end"] = d.End
		}
		if d.TZ != "" {
			m["tz"] = d.TZ
		}
		return m

	case "checkbox":
		if pv.Checkbox == nil {
			return false
		}
		return *pv.Checkbox

	case "people":
		if len(pv.People) == 0 {
			return []string{}
		}
		return pv.People

	case "relation":
		if len(pv.Relation) == 0 {
			return []string{}
		}
		return pv.Relation

	case "files":
		paths := make([]string, 0, len(pv.Files))
		for _, f := range pv.Files {
			if f.LocalPath != "" {
				paths = append(paths, f.LocalPath)
			} else {
				paths = append(paths, f.ExternalURL)
			}
		}
		return paths

	case "formula":
		return pv.Formula

	case "rollup":
		return pv.Rollup

	case "unique_id":
		return pv.Formula // stored in Formula field as raw value

	default:
		return nil
	}
}

// YAMLToProperties parses a YAML map back into a PropertyValue map.
// editableYAML is the "properties:" section; computedYAML is the "computed:" section.
// Schema provides type hints for the target database; if nil, type is inferred.
func YAMLToProperties(
	editableYAML map[string]any,
	computedYAML map[string]any,
	schema map[string]string, // property name → type; nil = best-effort
) (map[string]notion.PropertyValue, error) {
	result := map[string]notion.PropertyValue{}

	for name, val := range editableYAML {
		typ := ""
		if schema != nil {
			typ = schema[name]
		}
		pv, err := yamlValueToProperty(name, val, typ, false)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		result[name] = pv
	}

	for name, val := range computedYAML {
		typ := ""
		if schema != nil {
			typ = schema[name]
		}
		pv, err := yamlValueToProperty(name, val, typ, true)
		if err != nil {
			return nil, fmt.Errorf("computed property %q: %w", name, err)
		}
		result[name] = pv
	}

	return result, nil
}

// yamlValueToProperty converts a YAML value to a PropertyValue, using typ as a hint.
func yamlValueToProperty(name string, val any, typ string, computed bool) (notion.PropertyValue, error) {
	pv := notion.PropertyValue{Computed: computed}

	// If no type hint, infer from YAML value shape.
	if typ == "" {
		typ = inferType(val)
	}
	pv.Type = typ

	switch typ {
	case "title", "rich_text", "url", "email", "phone_number",
		"created_time", "last_edited_time", "created_by", "last_edited_by":
		s, ok := val.(string)
		if !ok {
			return pv, fmt.Errorf("expected string, got %T", val)
		}
		pv.Text = s

	case "number":
		switch v := val.(type) {
		case int:
			f := float64(v)
			pv.Number = &f
		case int64:
			f := float64(v)
			pv.Number = &f
		case float64:
			pv.Number = &v
		case nil:
			// nil means absent
		default:
			return pv, fmt.Errorf("expected number, got %T", val)
		}

	case "select", "status":
		if val == nil {
			pv.Select = ""
		} else {
			s, ok := val.(string)
			if !ok {
				return pv, fmt.Errorf("expected string option, got %T", val)
			}
			pv.Select = s
		}

	case "multi_select":
		switch v := val.(type) {
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return pv, fmt.Errorf("multi_select items must be strings, got %T", item)
				}
				pv.MultiSel = append(pv.MultiSel, s)
			}
		case []string:
			pv.MultiSel = v
		case nil:
			// empty
		default:
			return pv, fmt.Errorf("expected list of strings, got %T", val)
		}

	case "date":
		switch v := val.(type) {
		case string:
			pv.Date = &notion.DateValue{Start: v}
		case map[string]any:
			dv := &notion.DateValue{}
			if s, ok := v["start"].(string); ok {
				dv.Start = s
			}
			if e, ok := v["end"].(string); ok {
				dv.End = e
			}
			if tz, ok := v["tz"].(string); ok {
				dv.TZ = tz
			}
			pv.Date = dv
		case nil:
			// no date
		default:
			return pv, fmt.Errorf("expected string or {start,end,tz} for date, got %T", val)
		}

	case "checkbox":
		b, ok := val.(bool)
		if !ok {
			return pv, fmt.Errorf("expected bool for checkbox, got %T", val)
		}
		pv.Checkbox = &b

	case "people", "relation":
		switch v := val.(type) {
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return pv, fmt.Errorf("%s items must be strings, got %T", typ, item)
				}
				if typ == "people" {
					pv.People = append(pv.People, s)
				} else {
					pv.Relation = append(pv.Relation, s)
				}
			}
		case []string:
			if typ == "people" {
				pv.People = v
			} else {
				pv.Relation = v
			}
		case nil:
			// empty
		default:
			return pv, fmt.Errorf("expected list of UUIDs, got %T", val)
		}

	case "files":
		switch v := val.(type) {
		case []any:
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return pv, fmt.Errorf("files items must be strings, got %T", item)
				}
				pv.Files = append(pv.Files, notion.FilePropValue{ExternalURL: s})
			}
		case []string:
			for _, s := range v {
				pv.Files = append(pv.Files, notion.FilePropValue{ExternalURL: s})
			}
		case nil:
			// empty
		default:
			return pv, fmt.Errorf("expected list of paths/URLs, got %T", val)
		}

	case "formula", "rollup", "unique_id":
		pv.Computed = true
		pv.Formula = val

	default:
		// Store unknown type as computed.
		pv.Computed = true
		pv.Formula = val
	}

	return pv, nil
}

// inferType guesses a Notion property type from a YAML value's Go type.
func inferType(val any) string {
	switch val.(type) {
	case bool:
		return "checkbox"
	case int, int64, float64:
		return "number"
	case []any, []string:
		return "multi_select"
	case map[string]any:
		return "date"
	default:
		return "rich_text"
	}
}

// MarshalPropertyYAML serializes a single PropertyValue to YAML bytes.
// Used in tests.
func MarshalPropertyYAML(pv notion.PropertyValue) ([]byte, error) {
	return yaml.Marshal(propertyToYAMLValue(pv))
}
