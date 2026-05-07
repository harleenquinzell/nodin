package notion

import (
	"encoding/json"
	"fmt"
	"time"
)

// PropertyValue holds a single Notion database property value.
// Only one of the value fields is set, according to Type.
type PropertyValue struct {
	Type     string // e.g. "rich_text", "number", "select"
	Computed bool   // true for read-only computed properties

	// Exactly one of these is populated:
	Text     string          // rich_text, title, url, email, phone_number
	Number   *float64        // number
	Select   string          // select, status (option name)
	MultiSel []string        // multi_select
	Date     *DateValue      // date
	Checkbox *bool           // checkbox
	People   []string        // people (user IDs)
	Relation []string        // relation (page IDs)
	Files    []FilePropValue // files
	Formula  any             // formula (read-only)
	Rollup   any             // rollup (read-only)
}

// DateValue holds a Notion date property value.
type DateValue struct {
	Start string // ISO 8601
	End   string // optional; "" if point in time
	TZ    string // optional IANA timezone
}

// FilePropValue holds a file entry from a files property.
type FilePropValue struct {
	Name        string
	LocalPath   string
	ExternalURL string
}

// ParsePropertyValue parses a raw Notion property JSON into a PropertyValue.
func ParsePropertyValue(raw json.RawMessage) (PropertyValue, error) {
	var envelope struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return PropertyValue{}, fmt.Errorf("parse property type: %w", err)
	}

	pv := PropertyValue{Type: envelope.Type}

	switch envelope.Type {
	case "title":
		var v struct {
			Title []RichText `json:"title"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Text = richTextPlain(v.Title)

	case "rich_text":
		var v struct {
			RichText []RichText `json:"rich_text"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Text = richTextPlain(v.RichText)

	case "number":
		var v struct {
			Number *float64 `json:"number"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Number = v.Number

	case "select":
		var v struct {
			Select *struct {
				Name string `json:"name"`
			} `json:"select"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.Select != nil {
			pv.Select = v.Select.Name
		}

	case "status":
		var v struct {
			Status *struct {
				Name string `json:"name"`
			} `json:"status"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.Status != nil {
			pv.Select = v.Status.Name
		}

	case "multi_select":
		var v struct {
			MultiSelect []struct {
				Name string `json:"name"`
			} `json:"multi_select"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		for _, opt := range v.MultiSelect {
			pv.MultiSel = append(pv.MultiSel, opt.Name)
		}

	case "date":
		var v struct {
			Date *struct {
				Start    string  `json:"start"`
				End      *string `json:"end"`
				TimeZone *string `json:"time_zone"`
			} `json:"date"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.Date != nil {
			dv := &DateValue{Start: v.Date.Start}
			if v.Date.End != nil {
				dv.End = *v.Date.End
			}
			if v.Date.TimeZone != nil {
				dv.TZ = *v.Date.TimeZone
			}
			pv.Date = dv
		}

	case "checkbox":
		var v struct {
			Checkbox bool `json:"checkbox"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Checkbox = &v.Checkbox

	case "url":
		var v struct {
			URL *string `json:"url"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.URL != nil {
			pv.Text = *v.URL
		}

	case "email":
		var v struct {
			Email *string `json:"email"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.Email != nil {
			pv.Text = *v.Email
		}

	case "phone_number":
		var v struct {
			PhoneNumber *string `json:"phone_number"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if v.PhoneNumber != nil {
			pv.Text = *v.PhoneNumber
		}

	case "people":
		var v struct {
			People []struct {
				ID string `json:"id"`
			} `json:"people"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		for _, u := range v.People {
			pv.People = append(pv.People, u.ID)
		}

	case "relation":
		var v struct {
			Relation []struct {
				ID string `json:"id"`
			} `json:"relation"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		for _, r := range v.Relation {
			pv.Relation = append(pv.Relation, r.ID)
		}

	case "files":
		var v struct {
			Files []struct {
				Name     string `json:"name"`
				Type     string `json:"type"`
				External *struct {
					URL string `json:"url"`
				} `json:"external,omitempty"`
				File *struct {
					URL string `json:"url"`
				} `json:"file,omitempty"`
			} `json:"files"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		for _, f := range v.Files {
			fpv := FilePropValue{Name: f.Name}
			if f.External != nil {
				fpv.ExternalURL = f.External.URL
			} else if f.File != nil {
				fpv.ExternalURL = f.File.URL
			}
			pv.Files = append(pv.Files, fpv)
		}

	case "formula":
		var v struct {
			Formula any `json:"formula"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Computed = true
		pv.Formula = v.Formula

	case "rollup":
		var v struct {
			Rollup any `json:"rollup"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		pv.Computed = true
		pv.Rollup = v.Rollup

	case "created_time", "last_edited_time":
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if s, ok := v[envelope.Type].(string); ok {
			pv.Text = s
		}
		pv.Computed = true

	case "created_by", "last_edited_by":
		var v map[string]json.RawMessage
		if err := json.Unmarshal(raw, &v); err != nil {
			return pv, err
		}
		if u, ok := v[envelope.Type]; ok {
			var user struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(u, &user); err == nil {
				pv.Text = user.ID
			}
		}
		pv.Computed = true

	case "unique_id":
		pv.Computed = true
		// stored as raw formula value; convert layer handles YAML rendering

	default:
		pv.Computed = true
	}

	return pv, nil
}

// richTextPlain concatenates the PlainText of a []RichText slice.
func richTextPlain(rts []RichText) string {
	var s string
	for _, rt := range rts {
		s += rt.PlainText
	}
	return s
}

// Database represents a Notion database object.
type Database struct {
	Object         string                     `json:"object"`
	ID             string                     `json:"id"`
	Title          []RichText                 `json:"title"`
	CreatedTime    time.Time                  `json:"created_time"`
	LastEditedTime time.Time                  `json:"last_edited_time"`
	Archived       bool                       `json:"archived"`
	Parent         Parent                     `json:"parent"`
	Properties     map[string]json.RawMessage `json:"properties"` // schema
}

// TitleText returns the plain-text title of the database.
func (d *Database) TitleText() string {
	return richTextPlain(d.Title)
}

// Schema returns a map from property name → Notion property type ("select", "number", …).
// The "title" property is omitted since it is handled separately as the page title.
func (d *Database) Schema() map[string]string {
	schema := make(map[string]string, len(d.Properties))
	for name, raw := range d.Properties {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Type != "title" {
			schema[name] = envelope.Type
		}
	}
	return schema
}

// AsPage returns a Page that represents this database for use in pathmap lookups.
// Only the ID, Parent, and title properties are populated.
func (d *Database) AsPage() Page {
	titleRaw, _ := json.Marshal(map[string]any{
		"type": "title",
		"title": []map[string]any{{
			"type":      "text",
			"plain_text": d.TitleText(),
			"text":       map[string]any{"content": d.TitleText(), "link": nil},
			"annotations": map[string]bool{
				"bold": false, "italic": false,
				"strikethrough": false, "underline": false, "code": false,
			},
		}},
	})
	return Page{
		ID:     d.ID,
		Parent: d.Parent,
		Properties: map[string]json.RawMessage{
			"title": json.RawMessage(titleRaw),
		},
	}
}
