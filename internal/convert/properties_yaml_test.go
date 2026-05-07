package convert_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/harleenquinzell/nodin/internal/convert"
	"github.com/harleenquinzell/nodin/internal/notion"
)

func pf(f float64) *float64 { return &f }
func pb(b bool) *bool        { return &b }

// roundTripProperty marshals pv to YAML and parses it back via YAMLToProperties.
// The property type is preserved as a schema hint so parsing is unambiguous.
func roundTripProperty(t *testing.T, pv notion.PropertyValue) notion.PropertyValue {
	t.Helper()
	data, err := convert.MarshalPropertyYAML(pv)
	if err != nil {
		t.Fatalf("MarshalPropertyYAML: %v", err)
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	schema := map[string]string{"prop": pv.Type}
	editable := map[string]any{}
	computed := map[string]any{}
	if pv.Computed {
		computed["prop"] = raw
	} else {
		editable["prop"] = raw
	}

	got, err := convert.YAMLToProperties(editable, computed, schema)
	if err != nil {
		t.Fatalf("YAMLToProperties: %v", err)
	}
	result, ok := got["prop"]
	if !ok {
		t.Fatal("property 'prop' not found in result")
	}
	return result
}

func TestProperty_RichText(t *testing.T) {
	pv := notion.PropertyValue{Type: "rich_text", Text: "hello world"}
	got := roundTripProperty(t, pv)
	if got.Text != pv.Text {
		t.Errorf("Text = %q, want %q", got.Text, pv.Text)
	}
}

func TestProperty_Number(t *testing.T) {
	pv := notion.PropertyValue{Type: "number", Number: pf(42.5)}
	got := roundTripProperty(t, pv)
	if got.Number == nil || *got.Number != 42.5 {
		t.Errorf("Number = %v, want 42.5", got.Number)
	}
}

func TestProperty_NumberZero(t *testing.T) {
	pv := notion.PropertyValue{Type: "number", Number: pf(0)}
	got := roundTripProperty(t, pv)
	if got.Number == nil || *got.Number != 0 {
		t.Errorf("Number = %v, want 0", got.Number)
	}
}

func TestProperty_NumberAbsent(t *testing.T) {
	pv := notion.PropertyValue{Type: "number", Number: nil}
	got := roundTripProperty(t, pv)
	if got.Number != nil {
		t.Errorf("Number = %v, want nil", got.Number)
	}
}

func TestProperty_Select(t *testing.T) {
	pv := notion.PropertyValue{Type: "select", Select: "In Progress"}
	got := roundTripProperty(t, pv)
	if got.Select != "In Progress" {
		t.Errorf("Select = %q, want %q", got.Select, "In Progress")
	}
}

func TestProperty_MultiSelect(t *testing.T) {
	pv := notion.PropertyValue{Type: "multi_select", MultiSel: []string{"api", "infra"}}
	got := roundTripProperty(t, pv)
	if len(got.MultiSel) != 2 || got.MultiSel[0] != "api" || got.MultiSel[1] != "infra" {
		t.Errorf("MultiSel = %v, want [api infra]", got.MultiSel)
	}
}

func TestProperty_DatePoint(t *testing.T) {
	pv := notion.PropertyValue{Type: "date", Date: &notion.DateValue{Start: "2026-06-01"}}
	got := roundTripProperty(t, pv)
	if got.Date == nil || got.Date.Start != "2026-06-01" || got.Date.End != "" {
		t.Errorf("Date = %+v", got.Date)
	}
}

func TestProperty_DateRange(t *testing.T) {
	pv := notion.PropertyValue{Type: "date", Date: &notion.DateValue{Start: "2026-06-01", End: "2026-06-15"}}
	got := roundTripProperty(t, pv)
	if got.Date == nil || got.Date.Start != "2026-06-01" || got.Date.End != "2026-06-15" {
		t.Errorf("Date = %+v", got.Date)
	}
}

func TestProperty_DateWithTZ(t *testing.T) {
	pv := notion.PropertyValue{Type: "date", Date: &notion.DateValue{Start: "2026-06-01T09:00", TZ: "America/Los_Angeles"}}
	got := roundTripProperty(t, pv)
	if got.Date == nil || got.Date.TZ != "America/Los_Angeles" {
		t.Errorf("Date.TZ = %q, want America/Los_Angeles", got.Date.TZ)
	}
}

func TestProperty_Checkbox(t *testing.T) {
	for _, v := range []bool{true, false} {
		v := v
		t.Run(map[bool]string{true: "true", false: "false"}[v], func(t *testing.T) {
			pv := notion.PropertyValue{Type: "checkbox", Checkbox: pb(v)}
			got := roundTripProperty(t, pv)
			if got.Checkbox == nil || *got.Checkbox != v {
				t.Errorf("Checkbox = %v, want %v", got.Checkbox, v)
			}
		})
	}
}

func TestProperty_URL(t *testing.T) {
	pv := notion.PropertyValue{Type: "url", Text: "https://example.com"}
	got := roundTripProperty(t, pv)
	if got.Text != pv.Text {
		t.Errorf("URL = %q, want %q", got.Text, pv.Text)
	}
}

func TestProperty_Email(t *testing.T) {
	pv := notion.PropertyValue{Type: "email", Text: "test@example.com"}
	got := roundTripProperty(t, pv)
	if got.Text != pv.Text {
		t.Errorf("Email = %q, want %q", got.Text, pv.Text)
	}
}

func TestProperty_PhoneNumber(t *testing.T) {
	pv := notion.PropertyValue{Type: "phone_number", Text: "+1-555-0100"}
	got := roundTripProperty(t, pv)
	if got.Text != pv.Text {
		t.Errorf("PhoneNumber = %q, want %q", got.Text, pv.Text)
	}
}

func TestProperty_People(t *testing.T) {
	ids := []string{"uuid-aaa", "uuid-bbb"}
	pv := notion.PropertyValue{Type: "people", People: ids}
	got := roundTripProperty(t, pv)
	if len(got.People) != 2 || got.People[0] != "uuid-aaa" || got.People[1] != "uuid-bbb" {
		t.Errorf("People = %v, want %v", got.People, ids)
	}
}

func TestProperty_Relation(t *testing.T) {
	ids := []string{"page-aaa", "page-bbb"}
	pv := notion.PropertyValue{Type: "relation", Relation: ids}
	got := roundTripProperty(t, pv)
	if len(got.Relation) != 2 || got.Relation[0] != "page-aaa" || got.Relation[1] != "page-bbb" {
		t.Errorf("Relation = %v, want %v", got.Relation, ids)
	}
}

func TestProperty_Files(t *testing.T) {
	editable := map[string]any{
		"Attachments": []any{"https://example.com/file.pdf"},
	}
	schema := map[string]string{"Attachments": "files"}
	got, err := convert.YAMLToProperties(editable, nil, schema)
	if err != nil {
		t.Fatalf("YAMLToProperties: %v", err)
	}
	prop := got["Attachments"]
	if len(prop.Files) != 1 || prop.Files[0].ExternalURL != "https://example.com/file.pdf" {
		t.Errorf("Files = %+v", prop.Files)
	}
}

func TestPropertiesToYAML_SplitsComputedFromEditable(t *testing.T) {
	props := map[string]notion.PropertyValue{
		"Status":     {Type: "select", Select: "Done"},
		"LastEdited": {Type: "last_edited_time", Text: "2026-05-07T00:00:00Z", Computed: true},
	}
	editable, computed := convert.PropertiesToYAML(props)

	if _, ok := editable["Status"]; !ok {
		t.Error("Status should be in editable")
	}
	if _, ok := computed["LastEdited"]; !ok {
		t.Error("LastEdited should be in computed")
	}
	if _, ok := editable["LastEdited"]; ok {
		t.Error("LastEdited should NOT be in editable")
	}
}

func TestProperty_InvalidType_ReturnsError(t *testing.T) {
	editable := map[string]any{"prop": 123} // number where string is expected
	schema := map[string]string{"prop": "rich_text"}
	_, err := convert.YAMLToProperties(editable, nil, schema)
	if err == nil {
		t.Error("expected error for type mismatch, got nil")
	}
}
