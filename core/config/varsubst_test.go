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
	binDir := t.TempDir()
	template := map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "10m"},
			map[string]interface{}{"name": "urltest_tolerance", "type": "enum", "default_value": "200"},
			map[string]interface{}{"name": "tun_stack", "type": "enum", "default_value": "system"},
		},
	}
	writeJSON(t, filepath.Join(binDir, "wizard_template.json"), template)

	subst := BuildVarSubstituterFromDisk(binDir)

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
	binDir := t.TempDir()
	writeJSON(t, filepath.Join(binDir, "wizard_template.json"), map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
		},
	})
	writeJSON(t, filepath.Join(binDir, "state.json"), map[string]interface{}{
		"settings_vars": map[string]string{
			"urltest_interval": "30m",
		},
	})

	subst := BuildVarSubstituterFromDisk(binDir)
	if val, ok := subst("urltest_interval"); !ok || val != "30m" {
		t.Errorf("urltest_interval: %v ok=%v, want user override %q", val, ok, "30m")
	}
}

func TestBuildVarSubstituterFromDisk_BoolCoercion(t *testing.T) {
	binDir := t.TempDir()
	writeJSON(t, filepath.Join(binDir, "wizard_template.json"), map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "tun_enabled", "type": "bool", "default_value": "true"},
			map[string]interface{}{"name": "tun_disabled", "type": "bool", "default_value": "false"},
		},
	})

	subst := BuildVarSubstituterFromDisk(binDir)
	if val, ok := subst("tun_enabled"); !ok || val != true {
		t.Errorf("tun_enabled: %v (%T) ok=%v, want bool true", val, val, ok)
	}
	if val, ok := subst("tun_disabled"); !ok || val != false {
		t.Errorf("tun_disabled: %v (%T) ok=%v, want bool false", val, val, ok)
	}
}

func TestBuildVarSubstituterFromDisk_MissingFiles(t *testing.T) {
	binDir := t.TempDir()
	// No template, no state. Should not panic; returns ok=false for everything.
	subst := BuildVarSubstituterFromDisk(binDir)
	if val, ok := subst("urltest_interval"); ok {
		t.Errorf("missing files: ok=true val=%v, want ok=false", val)
	}
}

func TestBuildVarSubstituterFromDisk_PlatformObjectDefault(t *testing.T) {
	binDir := t.TempDir()
	// `default_value` as object (per-platform) — pick "default" key.
	writeJSON(t, filepath.Join(binDir, "wizard_template.json"), map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{
				"name":          "tun_stack",
				"type":          "enum",
				"default_value": map[string]interface{}{"win7": "gvisor", "default": "system"},
			},
		},
	})

	subst := BuildVarSubstituterFromDisk(binDir)
	if val, ok := subst("tun_stack"); !ok || val != "system" {
		t.Errorf("tun_stack: %v ok=%v, want %q (default key)", val, ok, "system")
	}
}

func TestSubstituteParserConfigPlaceholders_EndToEndWithDiskSubstituter(t *testing.T) {
	// Verifies the full path: template + state on disk → BuildVarSubstituterFromDisk
	// → SubstituteParserConfigPlaceholders → resolved options.
	binDir := t.TempDir()
	writeJSON(t, filepath.Join(binDir, "wizard_template.json"), map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "urltest_url", "type": "text", "default_value": "https://1.1.1.1"},
			map[string]interface{}{"name": "urltest_interval", "type": "enum", "default_value": "5m"},
			map[string]interface{}{"name": "urltest_tolerance", "type": "enum", "default_value": "100"},
		},
	})
	writeJSON(t, filepath.Join(binDir, "state.json"), map[string]interface{}{
		"settings_vars": map[string]string{
			"urltest_interval": "1m",
		},
	})

	pc := newPCWithAutoProxyOptions(map[string]interface{}{
		"url":       "@urltest_url",
		"interval":  "@urltest_interval",
		"tolerance": "@urltest_tolerance",
	})
	subst := BuildVarSubstituterFromDisk(binDir)
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
