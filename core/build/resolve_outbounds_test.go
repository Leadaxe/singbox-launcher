package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// TestResolveOutbounds_GlobalNoUpdates — global outbound без updates остаётся as-is.
func TestResolveOutbounds_GlobalNoUpdates(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{
			Tag:  "proxy-out",
			Type: "selector",
			Options: map[string]interface{}{
				"default": "auto-proxy-out",
			},
			AddOutbounds: []string{"direct-out", "auto-proxy-out"},
		},
	}
	got := ResolveOutbounds(outbounds, nil, nil)
	if len(got.Globals) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Globals))
	}
	r := got.Globals[0]
	if r.Source != OutboundSourceGlobal {
		t.Errorf("expected Source=global, got %q", r.Source)
	}
	if r.Ref != "" || r.HasPresetUpdates {
		t.Errorf("expected no preset metadata, got %+v", r)
	}
	if r.Body.Tag != "proxy-out" || r.Body.Options["default"] != "auto-proxy-out" {
		t.Errorf("body lost: %+v", r.Body)
	}
}

// TestResolveOutbounds_PresetAddRef — entry с ref → Source=preset, PresetLabel.
func TestResolveOutbounds_PresetAddRef(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{
			Tag:  "ru VPN 🇷🇺",
			Type: "selector",
			Ref:  "russian",
		},
	}
	presets := []template.Preset{
		{ID: "russian", Label: "Russian domains & IPs"},
	}
	got := ResolveOutbounds(outbounds, presets, nil)
	if len(got.Globals) != 1 {
		t.Fatalf("count: %d", len(got.Globals))
	}
	r := got.Globals[0]
	if r.Source != OutboundSourcePreset {
		t.Errorf("expected Source=preset, got %q", r.Source)
	}
	if r.Ref != "russian" || r.PresetLabel != "Russian domains & IPs" {
		t.Errorf("preset metadata: ref=%q label=%q", r.Ref, r.PresetLabel)
	}
}

// TestResolveOutbounds_UpdatesMerged — updates стек применяется к base.
func TestResolveOutbounds_UpdatesMerged(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{
			Tag:          "proxy-out",
			Type:         "selector",
			AddOutbounds: []string{"direct-out"},
			Updates: []configtypes.OutboundUpdate{
				{Ref: "russian", Patch: map[string]interface{}{
					"filters":      map[string]interface{}{"tag": "!/(🇷🇺)/i"},
					"addOutbounds": []interface{}{"extra-ru"},
				}},
			},
		},
	}
	got := ResolveOutbounds(outbounds, nil, nil)
	r := got.Globals[0]
	if !r.HasPresetUpdates {
		t.Error("HasPresetUpdates should be true")
	}
	if r.Body.Filters == nil || r.Body.Filters["tag"] != "!/(🇷🇺)/i" {
		t.Errorf("filters not patched: %+v", r.Body.Filters)
	}
	// addOutbounds = union (direct-out base + extra-ru patch).
	wantSet := map[string]bool{"direct-out": true, "extra-ru": true}
	gotSet := map[string]bool{}
	for _, s := range r.Body.AddOutbounds {
		gotSet[s] = true
	}
	for k := range wantSet {
		if !gotSet[k] {
			t.Errorf("addOutbounds union missing %q: got %v", k, r.Body.AddOutbounds)
		}
	}
}

// TestResolveOutbounds_UpdatesOrderMatters — multiple updates apply in order.
func TestResolveOutbounds_UpdatesOrderMatters(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{
			Tag:  "proxy-out",
			Type: "selector",
			Updates: []configtypes.OutboundUpdate{
				{Ref: "russian", Patch: map[string]interface{}{
					"filters": map[string]interface{}{"tag": "first"},
				}},
				{Ref: "ru-inside", Patch: map[string]interface{}{
					"filters": map[string]interface{}{"tag": "second"},
				}},
			},
		},
	}
	got := ResolveOutbounds(outbounds, nil, nil)
	// last-wins для replace-полей (filters = replace целиком в applyOutboundUpdate)
	if got.Globals[0].Body.Filters["tag"] != "second" {
		t.Errorf("expected last update to win: %v", got.Globals[0].Body.Filters)
	}
}

// TestResolveOutbounds_RequiredFromTemplate — Required field из requiredTags map.
func TestResolveOutbounds_RequiredFromTemplate(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
		{Tag: "vpn ①", Type: "selector"},
	}
	requiredTags := map[string]bool{"proxy-out": true}
	got := ResolveOutbounds(outbounds, nil, requiredTags)
	if !got.Globals[0].Required {
		t.Error("proxy-out should be Required=true")
	}
	if got.Globals[1].Required {
		t.Error("vpn ① should not be Required")
	}
}

// TestResolveOutbounds_DanglingRef — ref на удалённый preset → PresetLabel = ref.
func TestResolveOutbounds_DanglingRef(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{Tag: "leftover", Ref: "deleted-preset"},
	}
	got := ResolveOutbounds(outbounds, nil, nil)
	r := got.Globals[0]
	if r.Source != OutboundSourcePreset || r.PresetLabel != "deleted-preset" {
		t.Errorf("dangling ref handling: %+v", r)
	}
}

// TestApplyOutboundUpdatePatch_Empty — пустой patch = noop.
func TestApplyOutboundUpdatePatch_Empty(t *testing.T) {
	target := configtypes.OutboundConfig{Tag: "x", Type: "selector"}
	out := applyOutboundUpdatePatch(target, nil)
	if out.Tag != "x" || out.Type != "selector" {
		t.Errorf("noop patch changed target: %+v", out)
	}
}
