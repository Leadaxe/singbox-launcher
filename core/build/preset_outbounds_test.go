package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// SPEC 057-R-N: ApplyPresetOutboundsToParserConfig removed. Fixtures
// makeParserCfg / makePresetRule + TestApply_* tests deleted. Lifecycle
// coverage moved to sync_outbounds_test.go (preset add/remove + Updates
// stack) and resolve_outbounds_test.go (merge of base+updates).

// === applyOutboundUpdate (typed field-merge) ================================

func TestApplyOutboundUpdate_FiltersReplace(t *testing.T) {
	target := configtypes.OutboundConfig{
		Tag: "x", Type: "selector",
		Filters: map[string]interface{}{"old": true},
	}
	patch := configtypes.OutboundConfig{
		Filters: map[string]interface{}{"new": true},
	}
	out := applyOutboundUpdate(target, patch)
	if _, hasOld := out.Filters["old"]; hasOld {
		t.Fatalf("expected old filter replaced, got %+v", out.Filters)
	}
	if out.Filters["new"] != true {
		t.Fatalf("expected new filter present, got %+v", out.Filters)
	}
}

func TestApplyOutboundUpdate_OptionsPerKeyReplace(t *testing.T) {
	target := configtypes.OutboundConfig{
		Tag: "x", Type: "selector",
		Options: map[string]interface{}{
			"default":  "a",
			"interval": "5m",
		},
	}
	patch := configtypes.OutboundConfig{
		Options: map[string]interface{}{"default": "b"},
	}
	out := applyOutboundUpdate(target, patch)
	if out.Options["default"] != "b" {
		t.Fatalf("expected default=b, got %v", out.Options["default"])
	}
	if out.Options["interval"] != "5m" {
		t.Fatalf("expected interval preserved, got %v", out.Options["interval"])
	}
}

func TestApplyOutboundUpdate_AddOutboundsUnion(t *testing.T) {
	target := configtypes.OutboundConfig{
		Tag: "x", AddOutbounds: []string{"a", "b"},
	}
	patch := configtypes.OutboundConfig{AddOutbounds: []string{"b", "c"}}
	out := applyOutboundUpdate(target, patch)
	want := []string{"a", "b", "c"}
	if len(out.AddOutbounds) != len(want) {
		t.Fatalf("expected %v got %v", want, out.AddOutbounds)
	}
	for i := range want {
		if out.AddOutbounds[i] != want[i] {
			t.Fatalf("expected %v got %v", want, out.AddOutbounds)
		}
	}
}

func TestApplyOutboundUpdate_TagAndTypeImmutable(t *testing.T) {
	target := configtypes.OutboundConfig{Tag: "original", Type: "selector"}
	// patch.Type ignored (loader would have already cleared it for update).
	patch := configtypes.OutboundConfig{Tag: "overridden", Type: "urltest"}
	out := applyOutboundUpdate(target, patch)
	if out.Tag != "original" {
		t.Fatalf("Tag should be immutable, got %q", out.Tag)
	}
	if out.Type != "selector" {
		t.Fatalf("Type should be immutable, got %q", out.Type)
	}
}

// === CleanDanglingOutboundsInRouteRules ===================================

func TestClean_DanglingFallback(t *testing.T) {
	routeRaw := json.RawMessage(`{
		"rules": [
			{"domain": "example.com", "outbound": "missing"}
		],
		"final": "direct-out"
	}`)
	finalTags := map[string]bool{"direct-out": true}
	out, warns, err := CleanDanglingOutboundsInRouteRules(routeRaw, finalTags, "direct-out")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "replaced with fallback") {
		t.Fatalf("expected fallback warning, got %v", warns)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rules := got["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule kept, got %d", len(rules))
	}
	if rules[0].(map[string]interface{})["outbound"] != "direct-out" {
		t.Fatalf("expected outbound rewritten to direct-out, got %+v", rules[0])
	}
}

func TestClean_DanglingDropWhenNoFallback(t *testing.T) {
	routeRaw := json.RawMessage(`{
		"rules": [
			{"domain": "example.com", "outbound": "missing"},
			{"domain": "good.com",    "outbound": "direct-out"}
		]
	}`)
	finalTags := map[string]bool{"direct-out": true}
	out, warns, _ := CleanDanglingOutboundsInRouteRules(routeRaw, finalTags, "")
	if len(warns) != 1 || !strings.Contains(warns[0], "rule dropped") {
		t.Fatalf("expected drop warning, got %v", warns)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(out, &got)
	rules := got["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule kept (only good one), got %d", len(rules))
	}
}

func TestClean_SentinelPreserved(t *testing.T) {
	routeRaw := json.RawMessage(`{
		"rules": [
			{"domain": "ads.example", "outbound": "reject"},
			{"domain": "drop.example", "outbound": "block"},
			{"protocol": "dns",        "outbound": "dns-out"}
		]
	}`)
	finalTags := map[string]bool{"direct-out": true} // none of the sentinels included
	out, warns, _ := CleanDanglingOutboundsInRouteRules(routeRaw, finalTags, "direct-out")
	if len(warns) != 0 {
		t.Fatalf("sentinel-tagged rules should not warn, got %v", warns)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(out, &got)
	rules := got["rules"].([]interface{})
	if len(rules) != 3 {
		t.Fatalf("expected all 3 sentinel rules kept, got %d", len(rules))
	}
}

func TestClean_RuleWithoutOutbound_KeptUntouched(t *testing.T) {
	// Action-based rule (no outbound field) — e.g. {"action": "reject", ...}.
	routeRaw := json.RawMessage(`{
		"rules": [
			{"domain": "x.example", "action": "reject"}
		]
	}`)
	finalTags := map[string]bool{"direct-out": true}
	out, warns, _ := CleanDanglingOutboundsInRouteRules(routeRaw, finalTags, "direct-out")
	if len(warns) != 0 {
		t.Fatalf("rules without outbound should be untouched, got warnings %v", warns)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(out, &got)
	rules := got["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected rule kept, got %d", len(rules))
	}
}

// === ExpandPresetOutbounds (vars + if/if_or) ===============================

func TestExpand_IfFiltersEntries(t *testing.T) {
	preset := &template.Preset{
		ID: "p",
		Vars: []template.PresetVar{
			{Name: "geoip", Type: "bool", Default: "false"},
		},
		Outbounds: []template.PresetOutbound{
			{Mode: "add", Tag: "always", Type: "selector"},
			{Mode: "add", Tag: "guarded", Type: "selector", If: []string{"geoip"}},
		},
	}
	// Default geoip=false → second entry filtered.
	entries, _ := ExpandPresetOutbounds(preset, nil)
	if len(entries) != 1 || entries[0].Config.Tag != "always" {
		t.Fatalf("expected only 'always' entry, got %+v", entries)
	}
	// User overrides geoip=true → both entries.
	entries2, _ := ExpandPresetOutbounds(preset, map[string]string{"geoip": "true"})
	if len(entries2) != 2 {
		t.Fatalf("expected 2 entries with geoip=true, got %d", len(entries2))
	}
}

func TestExpand_VarSubstitutionInOptions(t *testing.T) {
	preset := &template.Preset{
		ID: "p",
		Vars: []template.PresetVar{
			{Name: "out", Type: "outbound", Default: "direct-out"},
		},
		Outbounds: []template.PresetOutbound{
			{Mode: "add", Tag: "x", Type: "selector",
				Options: map[string]interface{}{"default": "@out"}},
		},
	}
	entries, warns := ExpandPresetOutbounds(preset, map[string]string{"out": "proxy-out"})
	if len(warns) != 0 || len(entries) != 1 {
		t.Fatalf("expected 1 entry no warnings, got warns=%v entries=%+v", warns, entries)
	}
	if entries[0].Config.Options["default"] != "proxy-out" {
		t.Fatalf("expected default=proxy-out after substitution, got %v",
			entries[0].Config.Options["default"])
	}
}
