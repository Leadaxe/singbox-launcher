package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	v6 "singbox-launcher/core/state/v6"
	"singbox-launcher/core/template"
)

// === Test fixtures ============================================================

// makeParserCfg — helper: ParserConfig с заданными global outbounds.
func makeParserCfg(outbounds ...configtypes.OutboundConfig) *configtypes.ParserConfig {
	pc := &configtypes.ParserConfig{}
	pc.ParserConfig.Outbounds = append([]configtypes.OutboundConfig(nil), outbounds...)
	return pc
}

// makePresetRule — helper: v6.Rule kind=preset c заданными vars.
func makePresetRule(ref string, enabled bool, vars map[string]string) v6.Rule {
	body := v6.PresetBody{Vars: vars}
	if body.Vars == nil {
		body.Vars = map[string]string{}
	}
	raw, _ := json.Marshal(body)
	return v6.Rule{
		Kind:    v6.RuleKindPreset,
		Ref:     ref,
		Enabled: enabled,
		Body:    raw,
	}
}

// === ApplyPresetOutboundsToParserConfig ======================================

func TestApply_AddBasic(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "direct-out", Type: "direct"},
	)
	presets := []template.Preset{
		{
			ID: "p1",
			Outbounds: []template.PresetOutbound{
				{Mode: "add", Tag: "ru VPN", Type: "selector",
					Filters: map[string]interface{}{"tag": "/RU/i"}},
			},
		},
	}
	rules := []v6.Rule{makePresetRule("p1", true, nil)}

	out, warns, err := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
	if len(out.ParserConfig.Outbounds) != 2 {
		t.Fatalf("expected 2 outbounds (direct + ru VPN), got %d", len(out.ParserConfig.Outbounds))
	}
	if out.ParserConfig.Outbounds[1].Tag != "ru VPN" {
		t.Fatalf("expected appended ru VPN, got %q", out.ParserConfig.Outbounds[1].Tag)
	}
}

func TestApply_AddCollisionWithGlobal_FirstWins(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "proxy-out", Type: "selector",
			Filters: map[string]interface{}{"tag": "X"}},
	)
	presets := []template.Preset{
		{
			ID: "p1",
			Outbounds: []template.PresetOutbound{
				{Mode: "add", Tag: "proxy-out", Type: "selector",
					Filters: map[string]interface{}{"tag": "Y"}},
			},
		},
	}
	rules := []v6.Rule{makePresetRule("p1", true, nil)}

	out, warns, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if len(warns) != 1 || !strings.Contains(warns[0], "template global") {
		t.Fatalf("expected 'template global' collision warning, got %v", warns)
	}
	if len(out.ParserConfig.Outbounds) != 1 {
		t.Fatalf("collision should not append; got %d outbounds", len(out.ParserConfig.Outbounds))
	}
	if got := out.ParserConfig.Outbounds[0].Filters["tag"]; got != "X" {
		t.Fatalf("global filters should win, got %v", got)
	}
}

func TestApply_AddCollisionAcrossPresets_FirstByRuleOrder(t *testing.T) {
	parser := makeParserCfg()
	presets := []template.Preset{
		{ID: "first", Outbounds: []template.PresetOutbound{
			{Mode: "add", Tag: "shared", Type: "selector",
				Filters: map[string]interface{}{"src": "first"}},
		}},
		{ID: "second", Outbounds: []template.PresetOutbound{
			{Mode: "add", Tag: "shared", Type: "selector",
				Filters: map[string]interface{}{"src": "second"}},
		}},
	}
	rules := []v6.Rule{
		makePresetRule("first", true, nil),
		makePresetRule("second", true, nil),
	}

	out, warns, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if len(warns) != 1 || !strings.Contains(warns[0], "earlier preset") {
		t.Fatalf("expected 'earlier preset' collision warning, got %v", warns)
	}
	if got := out.ParserConfig.Outbounds[0].Filters["src"]; got != "first" {
		t.Fatalf("first preset should win, got %v", got)
	}
}

func TestApply_AddIdenticalBody_SilentSkip(t *testing.T) {
	parser := makeParserCfg()
	identical := template.PresetOutbound{
		Mode: "add", Tag: "ru VPN", Type: "selector",
		Filters:      map[string]interface{}{"tag": "/RU/"},
		AddOutbounds: []string{"direct-out"},
	}
	presets := []template.Preset{
		{ID: "a", Outbounds: []template.PresetOutbound{identical}},
		{ID: "b", Outbounds: []template.PresetOutbound{identical}},
	}
	rules := []v6.Rule{
		makePresetRule("a", true, nil),
		makePresetRule("b", true, nil),
	}

	out, warns, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if len(warns) != 0 {
		t.Fatalf("identical body should be silent skip (no warning), got %v", warns)
	}
	if len(out.ParserConfig.Outbounds) != 1 {
		t.Fatalf("identical body collision should not duplicate; got %d outbounds",
			len(out.ParserConfig.Outbounds))
	}
}

func TestApply_DisabledPreset_NoOp(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "direct-out", Type: "direct"},
	)
	presets := []template.Preset{
		{ID: "p", Outbounds: []template.PresetOutbound{
			{Mode: "add", Tag: "ru VPN", Type: "selector"},
		}},
	}
	rules := []v6.Rule{makePresetRule("p", false /* disabled */, nil)}

	out, warns, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if len(warns) != 0 {
		t.Fatalf("disabled preset should be silent, got %v", warns)
	}
	if len(out.ParserConfig.Outbounds) != 1 {
		t.Fatalf("disabled preset should not affect outbounds; got %d", len(out.ParserConfig.Outbounds))
	}
}

func TestApply_UpdateBasic(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "proxy-out", Type: "selector",
			Options: map[string]interface{}{"default": "auto-proxy-out"}},
	)
	presets := []template.Preset{
		{ID: "p", Outbounds: []template.PresetOutbound{
			{Mode: "update", Tag: "proxy-out",
				Filters: map[string]interface{}{"tag": "!/(RU)/i"}},
		}},
	}
	rules := []v6.Rule{makePresetRule("p", true, nil)}

	out, _, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	target := out.ParserConfig.Outbounds[0]
	if target.Filters == nil || target.Filters["tag"] != "!/(RU)/i" {
		t.Fatalf("expected filters patched on proxy-out, got %+v", target.Filters)
	}
	// Options should be preserved (per-key replace; nothing in patch.Options).
	if target.Options["default"] != "auto-proxy-out" {
		t.Fatalf("expected options preserved, got %+v", target.Options)
	}
}

func TestApply_UpdateMissingTarget_Warns(t *testing.T) {
	parser := makeParserCfg()
	presets := []template.Preset{
		{ID: "p", Outbounds: []template.PresetOutbound{
			{Mode: "update", Tag: "does-not-exist",
				Filters: map[string]interface{}{"tag": "X"}},
		}},
	}
	rules := []v6.Rule{makePresetRule("p", true, nil)}

	out, warns, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	if len(warns) != 1 || !strings.Contains(warns[0], "not found") {
		t.Fatalf("expected target-not-found warning, got %v", warns)
	}
	if len(out.ParserConfig.Outbounds) != 0 {
		t.Fatalf("update on missing should be no-op (no auto-create), got %+v",
			out.ParserConfig.Outbounds)
	}
}

func TestApply_UpdateMultipleInRuleOrder(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "proxy-out", Type: "selector"},
	)
	presets := []template.Preset{
		{ID: "first", Outbounds: []template.PresetOutbound{
			{Mode: "update", Tag: "proxy-out",
				AddOutbounds: []string{"a", "b"}},
		}},
		{ID: "second", Outbounds: []template.PresetOutbound{
			{Mode: "update", Tag: "proxy-out",
				AddOutbounds: []string{"b", "c"}},
		}},
	}
	rules := []v6.Rule{
		makePresetRule("first", true, nil),
		makePresetRule("second", true, nil),
	}

	out, _, _ := ApplyPresetOutboundsToParserConfig(parser, presets, rules)
	got := out.ParserConfig.Outbounds[0].AddOutbounds
	// First adds a, b; second unions in c (b deduped) → [a, b, c].
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("expected addOutbounds=%v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected addOutbounds=%v, got %v", want, got)
		}
	}
}

func TestApply_OriginalParserCfgImmutable(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "proxy-out", Type: "selector",
			Filters: map[string]interface{}{"tag": "ORIGINAL"}},
	)
	presets := []template.Preset{
		{ID: "p", Outbounds: []template.PresetOutbound{
			{Mode: "update", Tag: "proxy-out",
				Filters: map[string]interface{}{"tag": "PATCHED"}},
		}},
	}
	rules := []v6.Rule{makePresetRule("p", true, nil)}

	_, _, _ = ApplyPresetOutboundsToParserConfig(parser, presets, rules)

	if parser.ParserConfig.Outbounds[0].Filters["tag"] != "ORIGINAL" {
		t.Fatalf("original parser_config was mutated; tag = %v",
			parser.ParserConfig.Outbounds[0].Filters["tag"])
	}
}

func TestApply_EmptyRules_ReturnsClone(t *testing.T) {
	parser := makeParserCfg(
		configtypes.OutboundConfig{Tag: "direct-out", Type: "direct"},
	)
	out, warns, err := ApplyPresetOutboundsToParserConfig(parser, nil, nil)
	if err != nil || len(warns) != 0 {
		t.Fatalf("expected silent success, got err=%v warns=%v", err, warns)
	}
	if out == parser {
		t.Fatalf("expected fresh clone, got identical pointer")
	}
	if len(out.ParserConfig.Outbounds) != 1 || out.ParserConfig.Outbounds[0].Tag != "direct-out" {
		t.Fatalf("clone content mismatch")
	}
}

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
