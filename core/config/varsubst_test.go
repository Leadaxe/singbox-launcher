package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// newPCWithAutoProxyOptions builds a ParserConfig containing one auto-proxy-out
// outbound with the given options. ParserConfig.ParserConfig is an anonymous
// struct, so we build via JSON round-trip rather than struct literal.
func newPCWithAutoProxyOptions(options map[string]interface{}) *ParserConfig {
	pc := &ParserConfig{}
	pc.ParserConfig.Outbounds = []OutboundConfig{
		{Tag: "auto-proxy-out", Type: "urltest", Options: options},
	}
	return pc
}

func TestSubstituteParserConfigPlaceholders_NilSafety(t *testing.T) {
	// Should not panic on nil ParserConfig.
	SubstituteParserConfigPlaceholders(nil, nil)
}

func TestSubstituteParserConfigPlaceholders_HardcodedFallback(t *testing.T) {
	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"url":       "@urltest_url",
		"interval":  "@urltest_interval",
		"tolerance": "@urltest_tolerance",
	})
	SubstituteParserConfigPlaceholders(pc, nil)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["url"]; got != "https://cp.cloudflare.com/generate_204" {
		t.Errorf("url: %v, want hardcoded default", got)
	}
	if got := opts["interval"]; got != "5m" {
		t.Errorf("interval: %v, want %q", got, "5m")
	}
	// tolerance must be int (sing-box expects a number, not a string).
	if got, ok := opts["tolerance"].(int); !ok || got != 100 {
		t.Errorf("tolerance: %v (%T), want int 100", opts["tolerance"], opts["tolerance"])
	}
}

func TestSubstituteParserConfigPlaceholders_CustomSubstituter(t *testing.T) {
	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"interval": "@urltest_interval",
		"url":      "@urltest_url",
	})
	subst := func(name string) (interface{}, bool) {
		switch name {
		case "urltest_interval":
			return "10m", true
		case "urltest_url":
			return "https://www.google.com/generate_204", true
		}
		return nil, false
	}
	SubstituteParserConfigPlaceholders(pc, subst)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["interval"]; got != "10m" {
		t.Errorf("interval: %v, want %q", got, "10m")
	}
	if got := opts["url"]; got != "https://www.google.com/generate_204" {
		t.Errorf("url: %v, want google", got)
	}
}

func TestSubstituteParserConfigPlaceholders_SubstFalseFallsBackToHardcoded(t *testing.T) {
	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"interval": "@urltest_interval",
	})
	// Substituter that knows nothing — falls through to hardcoded fallback.
	subst := func(name string) (interface{}, bool) { return nil, false }
	SubstituteParserConfigPlaceholders(pc, subst)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["interval"]; got != "5m" {
		t.Errorf("interval: %v, want hardcoded 5m fallback", got)
	}
}

func TestSubstituteParserConfigPlaceholders_UnknownPlaceholderLeftAlone(t *testing.T) {
	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"interrupt_exist_connections": "@some_unknown_var",
		"interval":                    "5m", // already resolved, not a placeholder
	})
	SubstituteParserConfigPlaceholders(pc, nil)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["interrupt_exist_connections"]; got != "@some_unknown_var" {
		t.Errorf("unknown placeholder: %v, want literal preserved", got)
	}
	if got := opts["interval"]; got != "5m" {
		t.Errorf("non-placeholder string mutated: %v", got)
	}
}

func TestSubstituteParserConfigPlaceholders_ProxySourceOutbounds(t *testing.T) {
	// Placeholders also live in parser_config.proxies[].outbounds[].options
	// (local selectors). Ensure those are walked too.
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		{
			Source: "https://example.com/sub",
			Outbounds: []OutboundConfig{
				{Tag: "vpn-1-auto", Type: "urltest", Options: map[string]interface{}{"interval": "@urltest_interval"}},
			},
		},
	}
	SubstituteParserConfigPlaceholders(pc, nil)

	if got := pc.ParserConfig.Proxies[0].Outbounds[0].Options["interval"]; got != "5m" {
		t.Errorf("local selector interval: %v, want 5m fallback", got)
	}
}

func TestSubstituteParserConfigPlaceholders_NoOptionsMapNoCrash(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Outbounds = []OutboundConfig{
		{Tag: "raw", Type: "direct", Options: nil},
	}
	// Nil Options must not panic.
	SubstituteParserConfigPlaceholders(pc, nil)
}

func TestSubstituteParserConfigPlaceholders_NonStringValuesUntouched(t *testing.T) {
	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"interrupt_exist_connections": true,
		"max_outbound_count":          float64(5),
		"tags":                        []interface{}{"a", "b"},
	})
	SubstituteParserConfigPlaceholders(pc, nil)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["interrupt_exist_connections"]; got != true {
		t.Errorf("bool mutated: %v", got)
	}
	if got := opts["max_outbound_count"]; got != float64(5) {
		t.Errorf("number mutated: %v", got)
	}
	if got := opts["tags"]; len(got.([]interface{})) != 2 {
		t.Errorf("slice mutated: %v", got)
	}
}

func TestKnownPlaceholderFallback_ToleranceIsInt(t *testing.T) {
	val, ok := knownPlaceholderFallback("urltest_tolerance")
	if !ok {
		t.Fatal("urltest_tolerance: ok=false, want true")
	}
	if _, isInt := val.(int); !isInt {
		t.Errorf("urltest_tolerance type %T, want int (sing-box rejects strings here)", val)
	}
}

func TestKnownPlaceholderFallback_UnknownReturnsFalse(t *testing.T) {
	if val, ok := knownPlaceholderFallback("unknown_var"); ok {
		t.Errorf("unknown var: ok=true, val=%v; want ok=false", val)
	}
}

func TestBuildVarSubstituterFromDisk_TemplateDefaults(t *testing.T) {
	execDir := newTestLayout(t)
	template := map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "10m"},
			map[string]interface{}{"name": "urltest_tolerance", "type": "enum", "default_value": "200"},
			map[string]interface{}{"name": "tun_stack", "type": "enum", "default_value": "system"},
		},
	}
	writeTemplateFile(t, execDir, template)

	subst := BuildVarSubstituterFromDisk(execDir)

	if val, ok := subst("urltest_interval"); !ok || val != "10m" {
		t.Errorf("urltest_interval: %v ok=%v, want %q", val, ok, "10m")
	}
	if val, ok := subst("urltest_tolerance"); !ok || val != 200 {
		t.Errorf("urltest_tolerance: %v (%T) ok=%v, want int 200", val, val, ok)
	}
	if val, ok := subst("tun_stack"); !ok || val != "system" {
		t.Errorf("tun_stack: %v ok=%v, want %q", val, ok, "system")
	}
}

func TestBuildVarSubstituterFromDisk_StateOverridesTemplate(t *testing.T) {
	execDir := newTestLayout(t)
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
		},
	})
	// Real state.json schema: settings_vars is an array of {name, value}, not a map.
	writeStateFile(t, execDir, map[string]interface{}{
		// On-disk key is "vars" — see WizardStateFile.Vars in
		// ui/wizard/models/wizard_state_file.go (json:"vars").
		"vars": []interface{}{
			map[string]string{"name": "urltest_interval", "value": "30m"},
		},
	})

	subst := BuildVarSubstituterFromDisk(execDir)
	if val, ok := subst("urltest_interval"); !ok || val != "30m" {
		t.Errorf("urltest_interval: %v ok=%v, want user override %q", val, ok, "30m")
	}
}

func TestBuildVarSubstituterFromDisk_StateOverrideRealSchema(t *testing.T) {
	// Belt-and-braces: the *exact* on-disk schema seen in production state.json.
	// If this test passes, the v0.8.8.1 path-typo regression cannot reappear.
	execDir := newTestLayout(t)
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
			map[string]interface{}{"name": "urltest_url", "type": "text", "default_value": "https://cp.cloudflare.com/generate_204"},
		},
	})
	writeStateFile(t, execDir, map[string]interface{}{
		"version": 4,
		"id":      "test",
		"vars": []interface{}{
			map[string]string{"name": "urltest_interval", "value": "1m"},
			map[string]string{"name": "urltest_url", "value": "https://www.google.com/generate_204"},
		},
	})

	subst := BuildVarSubstituterFromDisk(execDir)
	if val, ok := subst("urltest_interval"); !ok || val != "1m" {
		t.Errorf("interval override: %v ok=%v, want %q", val, ok, "1m")
	}
	if val, ok := subst("urltest_url"); !ok || val != "https://www.google.com/generate_204" {
		t.Errorf("url override: %v ok=%v", val, ok)
	}
}

func TestBuildVarSubstituterFromDisk_BoolCoercion(t *testing.T) {
	execDir := newTestLayout(t)
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "tun_enabled", "type": "bool", "default_value": "true"},
			map[string]interface{}{"name": "tun_disabled", "type": "bool", "default_value": "false"},
		},
	})

	subst := BuildVarSubstituterFromDisk(execDir)
	if val, ok := subst("tun_enabled"); !ok || val != true {
		t.Errorf("tun_enabled: %v (%T) ok=%v, want bool true", val, val, ok)
	}
	if val, ok := subst("tun_disabled"); !ok || val != false {
		t.Errorf("tun_disabled: %v (%T) ok=%v, want bool false", val, val, ok)
	}
}

func TestBuildVarSubstituterFromDisk_MissingFiles(t *testing.T) {
	execDir := newTestLayout(t)
	// No template, no state. Should not panic; returns ok=false for everything.
	subst := BuildVarSubstituterFromDisk(execDir)
	if val, ok := subst("urltest_interval"); ok {
		t.Errorf("missing files: ok=true val=%v, want ok=false", val)
	}
}

func TestBuildVarSubstituterFromDisk_PlatformObjectDefault(t *testing.T) {
	execDir := newTestLayout(t)
	// `default_value` as object (per-platform) — pick "default" key.
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{
				"name":          "tun_stack",
				"type":          "enum",
				"default_value": map[string]interface{}{"win7": "gvisor", "default": "system"},
			},
		},
	})

	subst := BuildVarSubstituterFromDisk(execDir)
	if val, ok := subst("tun_stack"); !ok || val != "system" {
		t.Errorf("tun_stack: %v ok=%v, want %q (default key)", val, ok, "system")
	}
}

func TestSubstituteParserConfigPlaceholders_EndToEndWithDiskSubstituter(t *testing.T) {
	// Verifies the full path: template + state on disk → BuildVarSubstituterFromDisk
	// → SubstituteParserConfigPlaceholders → resolved options.
	execDir := newTestLayout(t)
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_url", "type": "text", "default_value": "https://1.1.1.1"},
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
			map[string]interface{}{"name": "urltest_tolerance", "type": "enum", "default_value": "100"},
		},
	})
	writeStateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]string{"name": "urltest_interval", "value": "1m"},
		},
	})

	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"url":       "@urltest_url",
		"interval":  "@urltest_interval",
		"tolerance": "@urltest_tolerance",
	})
	subst := BuildVarSubstituterFromDisk(execDir)
	SubstituteParserConfigPlaceholders(pc, subst)

	opts := pc.ParserConfig.Outbounds[0].Options
	if got := opts["url"]; got != "https://1.1.1.1" {
		t.Errorf("url: %v, want template default", got)
	}
	if got := opts["interval"]; got != "1m" {
		t.Errorf("interval: %v, want state override 1m", got)
	}
	if got := opts["tolerance"]; got != 100 {
		t.Errorf("tolerance: %v (%T), want int 100", got, got)
	}
}

// TestBuildVarSubstituterFromDisk_RealFixture loads a fixture captured from
// an actual user's state.json and asserts that values defined under the
// real on-disk JSON key (`vars`) are picked up. If somebody refactors
// loadStateSettingsVars and gets the JSON tag wrong AGAIN (this happened
// twice during the v0.8.8.1/.2 cycle), this test fails loudly.
//
// Fixture lives in testdata/state-real.json — derived from a real install,
// edited only to anonymize ID. To regenerate from a live launcher:
//
//   python3 -c '...' > core/config/testdata/state-real.json
func TestBuildVarSubstituterFromDisk_RealFixture(t *testing.T) {
	fixtureRaw, err := os.ReadFile("testdata/state-real.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	execDir := newTestLayout(t)
	statePath := filepath.Join(execDir, "bin", "wizard_states", "state.json")
	if err := os.WriteFile(statePath, fixtureRaw, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	// Minimal template so var lookups don't trigger template fallback —
	// we want to verify the OVERRIDE path explicitly.
	writeTemplateFile(t, execDir, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "tun", "type": "bool", "default_value": "false"},
			map[string]interface{}{"name": "tun_stack", "type": "enum", "default_value": "mixed"},
		},
	})

	subst := BuildVarSubstituterFromDisk(execDir)

	// Fixture has tun=true (overrides template default false).
	if val, ok := subst("tun"); !ok || val != true {
		t.Errorf("tun: %v (%T) ok=%v, want bool true from real state.json", val, val, ok)
	}
	// Fixture has tun_stack=system (overrides template default mixed).
	if val, ok := subst("tun_stack"); !ok || val != "system" {
		t.Errorf("tun_stack: %v ok=%v, want %q from real state.json", val, ok, "system")
	}
}

// newTestLayout creates a fake exec directory with the on-disk layout
// expected by BuildVarSubstituterFromDisk: bin/ and bin/wizard_states/.
// Returns the execDir; both subdirs are created.
func newTestLayout(t *testing.T) string {
	t.Helper()
	execDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(execDir, "bin", "wizard_states"), 0755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	return execDir
}

func writeTemplateFile(t *testing.T, execDir string, v interface{}) {
	t.Helper()
	writeJSON(t, filepath.Join(execDir, "bin", "wizard_template.json"), v)
}

func writeStateFile(t *testing.T, execDir string, v interface{}) {
	t.Helper()
	writeJSON(t, filepath.Join(execDir, "bin", "wizard_states", "state.json"), v)
}

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
