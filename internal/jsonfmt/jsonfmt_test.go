package jsonfmt

import (
	"os"
	"strings"
	"testing"
)

// TestWizardTemplateFormatted — штатный шаблон лежит в каноне tplfmt
// (`go run ./tools/tplfmt -w bin/wizard_template.json`), формат идемпотентен,
// и правила канона держатся на малых случаях.
func TestWizardTemplateFormatted(t *testing.T) {
	raw, err := os.ReadFile("../../bin/wizard_template.json")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	once, err := FormatCompact(raw)
	if err != nil {
		t.Fatalf("FormatCompact(template): %v", err)
	}
	if string(once) != string(raw) {
		t.Fatalf("bin/wizard_template.json is not in tplfmt form; run: go run ./tools/tplfmt -w bin/wizard_template.json")
	}
	twice, err := FormatCompact(once)
	if err != nil || string(twice) != string(once) {
		t.Fatalf("FormatCompact is not idempotent on the template (err=%v)", err)
	}

	long := strings.Repeat("x", 90)
	cases := []struct {
		name, in, want string
	}{
		{
			name: "inline up to 100 chars, key order kept, empty containers",
			in:   `{"z":1,"a":[ "x" , "y" ],"m":{},"e":[]}`,
			want: `{"z": 1, "a": ["x", "y"], "m": {}, "e": []}` + "\n",
		},
		{
			name: "over 100 chars breaks per element, children re-checked",
			in:   `{"k":["` + long + `"],"short":{"a":[1,2]}}`,
			want: "{\n  \"k\": [\"" + long + "\"],\n  \"short\": {\"a\": [1, 2]}\n}\n",
		},
		{
			name: "number literals kept as written",
			in:   `[1.0, 1e3, -0.50, 12345678901234567890]`,
			want: `[1.0, 1e3, -0.50, 12345678901234567890]` + "\n",
		},
		{
			name: "non-ASCII and <, & stay unescaped",
			in:   `{"tag":"vpn ①","t":"a<b & \u00e9"}`,
			want: `{"tag": "vpn ①", "t": "a<b & é"}` + "\n",
		},
	}
	for _, c := range cases {
		got, err := FormatCompact([]byte(c.in))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) != c.want {
			t.Fatalf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		again, _ := FormatCompact(got)
		if string(again) != string(got) {
			t.Fatalf("%s: not idempotent: %q", c.name, again)
		}
	}

	for _, bad := range []string{``, `{"a":}`, `{"a":1,}`, `[1] [2]`} {
		if _, err := FormatCompact([]byte(bad)); err == nil {
			t.Fatalf("FormatCompact(%q): want error", bad)
		}
	}
}
