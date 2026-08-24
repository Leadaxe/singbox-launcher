package locale

import (
	"encoding/json"
	"fmt"
)

// Entry is a single catalog record (SPEC 111).
//
// Three JSON shapes are accepted:
//
//	"Save": "Сохранить"                          // bare string (legacy flat
//	                                             // format and _display_name)
//	"Save": { "value": "Сохранить" }             // plain string
//	"%d nodes": { "value": {"one": …, "few": …}} // plural forms
//
// plus an optional "special" map for collisions — the same English text
// translated differently depending on the call site:
//
//	"Copy": { "value": "Копировать", "special": { "1": { "value": "Скопировать" } } }
type Entry struct {
	Value   Value
	Special map[string]Entry
}

// Value holds either a plain string or a set of plural forms (never both).
type Value struct {
	Text  string            // non-empty for a plain string
	Forms map[string]string // non-empty for a plural entry
}

// IsZero reports whether the value carries neither text nor plural forms.
func (v Value) IsZero() bool {
	return v.Text == "" && len(v.Forms) == 0
}

// UnmarshalJSON accepts a bare string, {"value": string} or
// {"value": {form: string}} with an optional "special" map.
func (e *Entry) UnmarshalJSON(data []byte) error {
	// Bare string — the legacy flat format ("key": "translation") and the
	// _display_name meta key. Tolerating it also lets old-format catalogs
	// load without errors and degrade gracefully (keys simply don't match).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		e.Value = Value{Text: s}
		e.Special = nil
		return nil
	}

	var raw struct {
		Value   json.RawMessage  `json:"value"`
		Special map[string]Entry `json:"special"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw.Value) == 0 {
		return fmt.Errorf("entry has no value")
	}
	val, err := parseValue(raw.Value)
	if err != nil {
		return err
	}
	e.Value = val
	e.Special = raw.Special
	return nil
}

func parseValue(data json.RawMessage) (Value, error) {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return Value{Text: s}, nil
	}
	var forms map[string]string
	if err := json.Unmarshal(data, &forms); err != nil {
		return Value{}, fmt.Errorf("value is neither a string nor a form map: %w", err)
	}
	if len(forms) == 0 {
		return Value{}, fmt.Errorf("empty form map")
	}
	return Value{Forms: forms}, nil
}

// parseCatalog decodes a whole catalog file. A malformed top-level document
// is an error; a malformed individual entry is skipped (reported in skipped)
// so that one bad record cannot take the whole language down.
func parseCatalog(data []byte) (entries map[string]Entry, skipped []string, err error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	entries = make(map[string]Entry, len(raw))
	for key, msg := range raw {
		var e Entry
		if err := e.UnmarshalJSON(msg); err != nil {
			skipped = append(skipped, key)
			continue
		}
		entries[key] = e
	}
	return entries, skipped, nil
}
