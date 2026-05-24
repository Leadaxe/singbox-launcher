package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// makeTemplate builds a TemplateData with given parser_config.outbounds + presets.
// requiredTags — set tag'ов которые получают `required: true` в template JSON.
func makeTemplate(t *testing.T, globals []configtypes.OutboundConfig, presets []template.Preset, requiredTags map[string]bool) *template.TemplateData {
	t.Helper()
	// Build raw outbounds with required flag injected.
	rawOutbounds := make([]map[string]interface{}, 0, len(globals))
	for _, g := range globals {
		// Marshal/unmarshal через JSON чтобы получить map (потом инжектим required).
		b, _ := json.Marshal(g)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		if requiredTags != nil && requiredTags[g.Tag] {
			m["required"] = true
		}
		rawOutbounds = append(rawOutbounds, m)
	}
	wrapped := map[string]interface{}{
		"ParserConfig": map[string]interface{}{
			"outbounds": rawOutbounds,
		},
	}
	pcStr, _ := json.Marshal(wrapped)
	return &template.TemplateData{
		ParserConfig: string(pcStr),
		Presets:      presets,
	}
}

// TestResolveOutbounds_DirectNoUpdates — direct outbound без updates остаётся as-is.
func TestResolveOutbounds_DirectNoUpdates(t *testing.T) {
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
	got := ResolveOutbounds(outbounds, nil)
	if len(got.Globals) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Globals))
	}
	r := got.Globals[0]
	if r.Source != OutboundSourceDirect {
		t.Errorf("expected Source=direct, got %q", r.Source)
	}
	if r.Ref != "" || r.HasPresetUpdates {
		t.Errorf("expected no ref/preset metadata, got %+v", r)
	}
	if r.Body.Tag != "proxy-out" || r.Body.Options["default"] != "auto-proxy-out" {
		t.Errorf("body lost: %+v", r.Body)
	}
}

// TestResolveOutbounds_PresetAddRef — entry с ref → Source=preset, PresetLabel.
func TestResolveOutbounds_PresetAddRef(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{Tag: "ru VPN 🇷🇺", Ref: "russian"},
	}
	td := &template.TemplateData{
		Presets: []template.Preset{
			{ID: "russian", Label: "Russian domains & IPs"},
		},
	}
	got := ResolveOutbounds(outbounds, td)
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

// TestResolveOutbounds_TemplateRef — referenced template entry резолвится из template body.
func TestResolveOutbounds_TemplateRef(t *testing.T) {
	td := makeTemplate(t, []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector", AddOutbounds: []string{"direct-out"}, Comment: "from template"},
	}, nil, nil)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Ref: configtypes.RefTemplate}, // thin
	}
	got := ResolveOutbounds(outbounds, td)
	r := got.Globals[0]
	if r.Source != OutboundSourceTemplate {
		t.Errorf("expected Source=template, got %q", r.Source)
	}
	if r.Body.Type != "selector" || r.Body.Comment != "from template" {
		t.Errorf("template body not resolved: %+v", r.Body)
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
	got := ResolveOutbounds(outbounds, nil)
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
	got := ResolveOutbounds(outbounds, nil)
	// last-wins для replace-полей (filters = replace целиком в applyOutboundUpdate)
	if got.Globals[0].Body.Filters["tag"] != "second" {
		t.Errorf("expected last update to win: %v", got.Globals[0].Body.Filters)
	}
}

// TestResolveOutbounds_HasUserPatch — Updates с RefUser маркируется HasUserPatch.
func TestResolveOutbounds_HasUserPatch(t *testing.T) {
	td := makeTemplate(t, []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector", Comment: "base"},
	}, nil, nil)
	outbounds := []configtypes.OutboundConfig{
		{
			Tag: "proxy-out", Ref: configtypes.RefTemplate,
			Updates: []configtypes.OutboundUpdate{
				{Ref: configtypes.RefUser, Patch: map[string]interface{}{"comment": "my edit"}},
			},
		},
	}
	got := ResolveOutbounds(outbounds, td)
	r := got.Globals[0]
	if !r.HasUserPatch {
		t.Error("HasUserPatch should be true")
	}
	if r.HasPresetUpdates {
		t.Error("HasPresetUpdates should be false (только USER patch)")
	}
	if r.Body.Comment != "my edit" {
		t.Errorf("USER patch not applied: %q", r.Body.Comment)
	}
}

// TestResolveOutbounds_RequiredFromTemplate — Required derived из td.RequiredOutboundTags.
func TestResolveOutbounds_RequiredFromTemplate(t *testing.T) {
	td := makeTemplate(t, []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
		{Tag: "vpn ①", Type: "selector"},
	}, nil, map[string]bool{"proxy-out": true})

	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Ref: configtypes.RefTemplate},
		{Tag: "vpn ①", Ref: configtypes.RefTemplate},
	}
	got := ResolveOutbounds(outbounds, td)
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
	got := ResolveOutbounds(outbounds, nil)
	r := got.Globals[0]
	if r.Source != OutboundSourcePreset || r.PresetLabel != "deleted-preset" {
		t.Errorf("dangling ref handling: %+v", r)
	}
	if r.Resolved {
		t.Error("Resolved should be false for dangling ref")
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
