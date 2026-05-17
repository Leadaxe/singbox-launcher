// SPEC 055 — preset.outbounds expand + merge tests.
package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state/v6"
	"singbox-launcher/core/template"
)

// === Expand tests ===

func TestExpandPreset_OutboundsAdd(t *testing.T) {
	preset := &template.Preset{
		ID: "ru-inside",
		Outbounds: []template.PresetOutbound{
			{
				Tag:  "ru VPN 🇷🇺",
				Type: "selector",
				Options: map[string]interface{}{
					"default": "direct-out",
				},
				Filters:      map[string]interface{}{"tag": "/(🇷🇺)/i"},
				AddOutbounds: []string{"direct-out"},
			},
		},
	}
	frags, _, ok := ExpandPreset(preset, nil)
	if !ok {
		t.Fatalf("ExpandPreset failed")
	}
	if len(frags.Outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(frags.Outbounds))
	}
	ob := frags.Outbounds[0]
	if ob.Mode != "add" {
		t.Errorf("expected mode=add (default), got %q", ob.Mode)
	}
	if ob.Tag != "ru VPN 🇷🇺" {
		t.Errorf("expected tag preserved (not prefixed), got %q", ob.Tag)
	}
	// Body не должен содержать control fields.
	if _, has := ob.Body["mode"]; has {
		t.Error("expected mode stripped from Body")
	}
	if _, has := ob.Body["if"]; has {
		t.Error("expected if stripped from Body")
	}
}

func TestExpandPreset_OutboundsUpdateStripsType(t *testing.T) {
	preset := &template.Preset{
		ID: "p1",
		Outbounds: []template.PresetOutbound{
			{
				Mode: "update",
				Tag:  "proxy-out",
				Type: "shadowsocks", // forbidden — should be dropped at expand
				Filters: map[string]interface{}{
					"tag": "!/RU/i",
				},
			},
		},
	}
	frags, _, ok := ExpandPreset(preset, nil)
	if !ok {
		t.Fatalf("ExpandPreset failed")
	}
	ob := frags.Outbounds[0]
	if ob.Mode != "update" {
		t.Errorf("expected mode=update, got %q", ob.Mode)
	}
	if _, has := ob.Body["type"]; has {
		t.Error("expected type stripped from update Body")
	}
	if _, has := ob.Body["filters"]; !has {
		t.Error("expected filters preserved")
	}
}

func TestExpandPreset_OutboundsVarSubstitution(t *testing.T) {
	preset := &template.Preset{
		ID: "p1",
		Vars: []template.PresetVar{
			{Name: "default_node", Type: "text", Default: "node-1"},
		},
		Outbounds: []template.PresetOutbound{
			{
				Tag:  "my-selector",
				Type: "selector",
				Options: map[string]interface{}{
					"default": "@default_node",
				},
			},
		},
	}
	frags, _, ok := ExpandPreset(preset, nil)
	if !ok {
		t.Fatalf("ExpandPreset failed")
	}
	opts := frags.Outbounds[0].Body["options"].(map[string]interface{})
	if opts["default"] != "node-1" {
		t.Errorf("expected @default_node→'node-1', got %v", opts["default"])
	}
}

func TestExpandPreset_OutboundsIfFilter(t *testing.T) {
	preset := &template.Preset{
		ID: "p1",
		Vars: []template.PresetVar{
			{Name: "use_extra", Type: "bool", Default: "false"},
		},
		Outbounds: []template.PresetOutbound{
			{Tag: "always", Type: "selector"},
			{Tag: "conditional", Type: "selector", If: []string{"use_extra"}},
		},
	}
	// use_extra=false → conditional dropped
	frags, _, _ := ExpandPreset(preset, nil)
	if len(frags.Outbounds) != 1 || frags.Outbounds[0].Tag != "always" {
		t.Errorf("expected only 'always', got %+v", frags.Outbounds)
	}
	// use_extra=true → both
	frags, _, _ = ExpandPreset(preset, map[string]string{"use_extra": "true"})
	if len(frags.Outbounds) != 2 {
		t.Errorf("expected 2 outbounds, got %d", len(frags.Outbounds))
	}
}

// === Merge tests ===

func TestMergePresetsIntoOutbounds_Add(t *testing.T) {
	base := []byte(`[{"tag":"proxy-out","type":"selector"}]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "ru-inside", Outbounds: []template.PresetOutbound{
				{Tag: "ru VPN 🇷🇺", Type: "selector"},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "ru-inside", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, err := MergePresetsIntoOutbounds(base, ctx)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	if len(arr) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(arr))
	}
	if arr[0]["tag"] != "proxy-out" || arr[1]["tag"] != "ru VPN 🇷🇺" {
		t.Errorf("unexpected order/tags: %+v", arr)
	}
}

func TestMergePresetsIntoOutbounds_UpdateFilters(t *testing.T) {
	base := []byte(`[{"tag":"proxy-out","type":"selector","options":{"default":"x"}}]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "ru-inside", Outbounds: []template.PresetOutbound{
				{Mode: "update", Tag: "proxy-out",
					Filters: map[string]interface{}{"tag": "!/RU/i"}},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "ru-inside", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, _ := MergePresetsIntoOutbounds(base, ctx)
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	if len(arr) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(arr))
	}
	po := arr[0]
	// options.default preserved
	opts := po["options"].(map[string]interface{})
	if opts["default"] != "x" {
		t.Errorf("options.default lost: %+v", opts)
	}
	// filters added
	f := po["filters"].(map[string]interface{})
	if f["tag"] != "!/RU/i" {
		t.Errorf("filters not applied: %+v", f)
	}
}

func TestMergePresetsIntoOutbounds_UpdateAddOutboundsUnion(t *testing.T) {
	base := []byte(`[{"tag":"proxy-out","type":"selector","addOutbounds":["a","b"]}]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "p1", Outbounds: []template.PresetOutbound{
				{Mode: "update", Tag: "proxy-out", AddOutbounds: []string{"b", "c"}},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "p1", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, _ := MergePresetsIntoOutbounds(base, ctx)
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	add, _ := arr[0]["addOutbounds"].([]interface{})
	got := make([]string, 0, len(add))
	for _, x := range add {
		got = append(got, x.(string))
	}
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("union expected %v, got %v", want, got)
	}
}

func TestMergePresetsIntoOutbounds_UpdateMissingTargetSkipped(t *testing.T) {
	base := []byte(`[{"tag":"proxy-out","type":"selector"}]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "p1", Outbounds: []template.PresetOutbound{
				{Mode: "update", Tag: "nonexistent",
					Filters: map[string]interface{}{"tag": "/x/"}},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "p1", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, _ := MergePresetsIntoOutbounds(base, ctx)
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	if len(arr) != 1 || arr[0]["tag"] != "proxy-out" {
		t.Errorf("base outbound should be untouched: %+v", arr)
	}
}

func TestMergePresetsIntoOutbounds_AddCollisionFirstWins(t *testing.T) {
	// Two presets both add "shared" — first (by RuleOrder) wins.
	base := []byte(`[]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "p1", Outbounds: []template.PresetOutbound{
				{Tag: "shared", Type: "selector",
					Options: map[string]interface{}{"default": "first"}},
			}},
			{ID: "p2", Outbounds: []template.PresetOutbound{
				{Tag: "shared", Type: "urltest",
					Options: map[string]interface{}{"default": "second"}},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "p1", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
			{Kind: v6.RuleKindPreset, Ref: "p2", Enabled: true,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, _ := MergePresetsIntoOutbounds(base, ctx)
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	if len(arr) != 1 {
		t.Fatalf("expected 1 outbound (collision dedup), got %d", len(arr))
	}
	if arr[0]["type"] != "selector" {
		t.Errorf("expected first-wins (selector), got: %v", arr[0])
	}
}

func TestMergePresetsIntoOutbounds_DisabledPresetIgnored(t *testing.T) {
	base := []byte(`[{"tag":"proxy-out","type":"selector"}]`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{
			{ID: "p1", Outbounds: []template.PresetOutbound{
				{Tag: "new", Type: "selector"},
			}},
		},
		RulesV6: []v6.Rule{
			{Kind: v6.RuleKindPreset, Ref: "p1", Enabled: false,
				Body: mustMarshal(t, v6.PresetBody{})},
		},
	}
	out, _ := MergePresetsIntoOutbounds(base, ctx)
	var arr []map[string]interface{}
	json.Unmarshal(out, &arr)
	if len(arr) != 1 {
		t.Errorf("disabled preset should not add outbounds, got %+v", arr)
	}
}

// === Dangling cleanup tests ===

func TestCleanDanglingOutboundRefInRule_KeepsValid(t *testing.T) {
	rule := map[string]interface{}{"outbound": "proxy-out", "domain": "x"}
	emitted := map[string]bool{"proxy-out": true}
	out := cleanDanglingOutboundRefInRule(rule, emitted, "")
	if out["outbound"] != "proxy-out" {
		t.Errorf("valid outbound should be kept: %+v", out)
	}
}

func TestCleanDanglingOutboundRefInRule_FallbackToFinal(t *testing.T) {
	rule := map[string]interface{}{"outbound": "ghost", "domain": "x"}
	emitted := map[string]bool{"proxy-out": true}
	out := cleanDanglingOutboundRefInRule(rule, emitted, "proxy-out")
	if out["outbound"] != "proxy-out" {
		t.Errorf("dangling ref should fallback to final 'proxy-out', got: %+v", out)
	}
}

func TestCleanDanglingOutboundRefInRule_DropWhenNoFinal(t *testing.T) {
	rule := map[string]interface{}{"outbound": "ghost", "domain": "x"}
	emitted := map[string]bool{}
	out := cleanDanglingOutboundRefInRule(rule, emitted, "")
	if out != nil {
		t.Errorf("expected nil (drop), got: %+v", out)
	}
}

func TestCleanDanglingOutboundRefInRule_SentinelsPreserved(t *testing.T) {
	for _, sent := range []string{"reject", "drop"} {
		rule := map[string]interface{}{"outbound": sent, "domain": "x"}
		out := cleanDanglingOutboundRefInRule(rule, map[string]bool{}, "fallback")
		if out["outbound"] != sent {
			t.Errorf("sentinel %q should be preserved, got: %+v", sent, out)
		}
	}
}

// === Helper ===

func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
