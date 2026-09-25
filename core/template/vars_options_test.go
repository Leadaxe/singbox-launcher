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

// `options` ортогональны `type` (SPEC 143 Т8–Т10): объектная форма тип не
// меняет, `enum` читается как `text` с закрытым списком, `options_open`
// разрешает своё значение.

func TestTemplateVarOptionsObjectFormKeepsType(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"name":"urltest_interval","type":"text","options":[{"title":"5m (default)","value":"5m"}]}`, "text"},
		{`{"name":"tol","type":"int","options":[{"title":"Low","value":"50"}]}`, "int"},
		{`{"name":"mix","type":"text","options":["plain",{"title":"Fancy","value":"fancy"}]}`, "text"},
		{`{"name":"log_level","type":"enum","options":[{"title":"Info","value":"info"}]}`, "text"},
		{`{"name":"urltest_url","type":"text","options":["https://a","https://b"]}`, "text"},
	} {
		var v TemplateVar
		if err := json.Unmarshal([]byte(tc.raw), &v); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.raw, err)
		}
		if v.Type != tc.want {
			t.Errorf("%s: Type = %q, want %q", tc.raw, v.Type, tc.want)
		}
		if v.OptionsOpen {
			t.Errorf("%s: OptionsOpen = true without options_open", tc.raw)
		}
	}
	var v TemplateVar
	if err := json.Unmarshal([]byte(`{"name":"mtu","type":"int","options":["1280"],"options_open":true}`), &v); err != nil {
		t.Fatal(err)
	}
	if !v.OptionsOpen {
		t.Errorf("options_open: true not read")
	}
}

func TestValidateTemplateVarOptionsShape(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		reject bool
	}{
		{`{"name":"flag","type":"bool","options":[{"title":"On","value":"true"}]}`, true},
		{`{"name":"mtu","type":"int","options_open":true}`, false},
		{`{"name":"mtu","type":"int","options":["1280","1492"],"options_open":true}`, false},
		{`{"name":"flag","type":"bool"}`, false},
	} {
		var v TemplateVar
		if err := json.Unmarshal([]byte(tc.raw), &v); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.raw, err)
		}
		err := ValidateWizardTemplate([]TemplateVar{v}, nil, nil)
		if (err != nil) != tc.reject {
			t.Errorf("%s: err = %v, want reject=%v", tc.raw, err, tc.reject)
		}
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
