package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/template"
)

// TestBuildConfigPassesRootLXBlock pins the only channel through which the
// fork's root `lx` block (core >= lx.13, SPEC 138) reaches config.json: the
// template's `config` object. The builder has no dedicated handler for `lx`,
// so it must emit the block verbatim after @var substitution and must not
// leak the lx.12 spelling (`route.lx_idle_*`) into route.
// A future allowlist of root sections would drop the block silently; this
// test is what catches it.
//
// The template declares vars, so the build goes through
// template.GetEffectiveConfigFor — the same path as the shipped template.
func TestBuildConfigPassesRootLXBlock(t *testing.T) {
	raw := `{
  "vars": [
    {"name": "log_level", "type": "enum", "default_value": "warn", "options": ["warn", "info"]},
    {"name": "masque_idle", "type": "text", "default_value": "5m"}
  ],
  "config": {
    "log": {"level": "@log_level"},
    "route": {"rules": [], "final": "direct-out"},
    "lx": {"masque": {"idle_timeout": "@masque_idle"}},
    "experimental": {"cache_file": {"enabled": true}}
  }
}`
	td, err := template.ParseTemplateData([]byte(raw))
	if err != nil {
		t.Fatalf("ParseTemplateData: %v", err)
	}
	res, err := BuildConfig(BuildContext{
		Template: td,
		Vars:     map[string]string{"log_level": "info", "masque_idle": "10m"},
	})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(res.ConfigJSON, &got); err != nil {
		t.Fatalf("config.json is not plain JSON: %v\n%s", err, res.ConfigJSON)
	}
	var lx struct {
		MASQUE struct {
			IdleTimeout string `json:"idle_timeout"`
		} `json:"masque"`
	}
	if err := json.Unmarshal(got["lx"], &lx); err != nil {
		t.Fatalf("root lx block missing or malformed: %v\n%s", err, res.ConfigJSON)
	}
	if lx.MASQUE.IdleTimeout != "10m" {
		t.Errorf("lx.masque.idle_timeout = %q, want the state value 10m", lx.MASQUE.IdleTimeout)
	}

	if _, ok := got["experimental"]; !ok {
		t.Errorf("a neighbouring root section was lost:\n%s", res.ConfigJSON)
	}
	if strings.Contains(string(got["route"]), "lx_idle") {
		t.Errorf("route must not carry lx.12 idle keys:\n%s", got["route"])
	}
}
