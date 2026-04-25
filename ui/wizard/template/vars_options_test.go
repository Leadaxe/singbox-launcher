package template

import (
	"encoding/json"
	"testing"
)

func TestTemplateVarOptionsLegacyStringList(t *testing.T) {
	raw := `{"name":"log_level","type":"enum","options":["debug","info","warn"]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := v.Options, []string{"debug", "info", "warn"}; !sliceEq(got, want) {
		t.Errorf("Options = %v, want %v", got, want)
	}
	if v.OptionTitles != nil {
		t.Errorf("OptionTitles = %v, want nil (no titles when legacy form)", v.OptionTitles)
	}
	if v.OptionTitle(1) != "info" {
		t.Errorf("OptionTitle(1) = %q, want fallback to value %q", v.OptionTitle(1), "info")
	}
}

func TestTemplateVarOptionsObjectList(t *testing.T) {
	raw := `{"name":"urltest_interval","type":"text","options":[
		{"title":"5m (default)","value":"5m"},
		{"title":"30m (battery)","value":"30m"}
	]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := v.Options, []string{"5m", "30m"}; !sliceEq(got, want) {
		t.Errorf("Options = %v, want %v", got, want)
	}
	if got, want := v.OptionTitles, []string{"5m (default)", "30m (battery)"}; !sliceEq(got, want) {
		t.Errorf("OptionTitles = %v, want %v", got, want)
	}
	if v.OptionTitle(0) != "5m (default)" {
		t.Errorf("OptionTitle(0) = %q, want %q", v.OptionTitle(0), "5m (default)")
	}
}

func TestTemplateVarOptionsMixedList(t *testing.T) {
	// string among objects — each element is parsed independently.
	raw := `{"name":"mix","type":"text","options":["plain",{"title":"Fancy","value":"fancy"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, want := v.Options, []string{"plain", "fancy"}; !sliceEq(got, want) {
		t.Errorf("Options = %v, want %v", got, want)
	}
	if got, want := v.OptionTitles, []string{"plain", "Fancy"}; !sliceEq(got, want) {
		t.Errorf("OptionTitles = %v, want %v", got, want)
	}
}

func TestTemplateVarOptionsEmptyTitleFallsBackToValue(t *testing.T) {
	raw := `{"name":"x","type":"text","options":[{"title":"","value":"ok"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.OptionTitle(0) != "ok" {
		t.Errorf("OptionTitle(0) = %q, want %q (title='' falls back to value)", v.OptionTitle(0), "ok")
	}
}

// Object-form options (`[{title, value}]`) carry display-only titles. Free-text
// combo (`type:"text"`) cannot safely round-trip them: user-typed text bypasses
// the title→value mapping. Normalize the declared type to "enum" whenever any
// option element is in object form, regardless of original type.

func TestTemplateVarOptionsTextWithObjectFormDegradesToEnum(t *testing.T) {
	raw := `{"name":"urltest_interval","type":"text","options":[
		{"title":"5m (default)","value":"5m"},
		{"title":"30m","value":"30m"}
	]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "enum" {
		t.Errorf("Type = %q, want %q (text+object-form options must degrade to enum)", v.Type, "enum")
	}
}

func TestTemplateVarOptionsBoolWithObjectFormDegradesToEnum(t *testing.T) {
	// Defensive: bool+options is nonsensical, but if a template author writes
	// it, normalize to enum rather than letting the renderer guess.
	raw := `{"name":"weird","type":"bool","options":[{"title":"On","value":"true"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "enum" {
		t.Errorf("Type = %q, want %q (any type + object-form options → enum)", v.Type, "enum")
	}
}

func TestTemplateVarOptionsEnumWithObjectFormStaysEnum(t *testing.T) {
	// Already enum — no-op.
	raw := `{"name":"log_level","type":"enum","options":[{"title":"Info","value":"info"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "enum" {
		t.Errorf("Type = %q, want %q (enum is idempotent under degradation)", v.Type, "enum")
	}
}

func TestTemplateVarOptionsTextWithLegacyStringListStaysText(t *testing.T) {
	// Legacy form (title==value implicitly) is safe for free-text combo —
	// no degradation, free typing remains useful.
	raw := `{"name":"urltest_url","type":"text","options":["https://a","https://b"]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "text" {
		t.Errorf("Type = %q, want %q (legacy string-list options must NOT degrade)", v.Type, "text")
	}
}

func TestTemplateVarOptionsTextWithMixedFormDegradesToEnum(t *testing.T) {
	// One string + one object → still triggers degradation; once any element
	// is object-form, semantics flip to closed-set.
	raw := `{"name":"mix","type":"text","options":["plain",{"title":"Fancy","value":"fancy"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "enum" {
		t.Errorf("Type = %q, want %q (mixed list with any object → enum)", v.Type, "enum")
	}
}

func TestTemplateVarOptionsTextWithEmptyTitleObjectDegradesToEnum(t *testing.T) {
	// Object form with empty title — still object form, so degrade.
	raw := `{"name":"x","type":"text","options":[{"title":"","value":"ok"}]}`
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.Type != "enum" {
		t.Errorf("Type = %q, want %q (object form with empty title is still object form)", v.Type, "enum")
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
