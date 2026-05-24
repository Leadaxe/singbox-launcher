package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	v6 "singbox-launcher/core/state/v6"
	"singbox-launcher/core/template"
)

func makeSyncTestPreset(t *testing.T, id string, outboundsJSON string) template.Preset {
	t.Helper()
	var p template.Preset
	if err := json.Unmarshal([]byte(`{"id":"`+id+`","label":"Test"}`), &p); err != nil {
		t.Fatal(err)
	}
	if outboundsJSON != "" {
		if err := json.Unmarshal([]byte(outboundsJSON), &p.Outbounds); err != nil {
			t.Fatalf("parse outbounds: %v", err)
		}
	}
	return p
}

// TestSyncOutbounds_EnableAddEntry — enable preset → add entry с ref.
func TestSyncOutbounds_EnableAddEntry(t *testing.T) {
	preset := makeSyncTestPreset(t, "russian", `[
		{"mode":"add","tag":"ru VPN 🇷🇺","type":"selector",
		 "options":{"default":"direct-out"},"addOutbounds":["direct-out"]}
	]`)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
	}
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})

	if len(outbounds) != 2 {
		t.Fatalf("expected 2 entries (proxy-out + ru VPN), got %d", len(outbounds))
	}
	ru := outbounds[1]
	if ru.Tag != "ru VPN 🇷🇺" || ru.Ref != "russian" {
		t.Errorf("preset add entry: %+v", ru)
	}
}

// TestSyncOutbounds_DisableRemovesEntry — disable preset → entry удаляется.
func TestSyncOutbounds_DisableRemovesEntry(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
		{Tag: "ru VPN 🇷🇺", Type: "selector", Ref: "russian"},
	}
	// No active rules → preset entry должна исчезнуть.
	rules := []v6.Rule{}
	SyncOutboundsWithActivePresets(rules, &outbounds, nil)

	if len(outbounds) != 1 || outbounds[0].Tag != "proxy-out" {
		t.Errorf("expected only proxy-out, got %+v", outbounds)
	}
}

// TestSyncOutbounds_UpdateStack — mode=update → patch добавляется в updates стек.
func TestSyncOutbounds_UpdateStack(t *testing.T) {
	preset := makeSyncTestPreset(t, "russian", `[
		{"mode":"update","tag":"proxy-out","filters":{"tag":"!/(🇷🇺)/i"}}
	]`)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
	}
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})

	if len(outbounds[0].Updates) != 1 {
		t.Fatalf("expected 1 update: %+v", outbounds[0].Updates)
	}
	u := outbounds[0].Updates[0]
	if u.Ref != "russian" {
		t.Errorf("update ref: %q", u.Ref)
	}
	if filters, ok := u.Patch["filters"].(map[string]interface{}); !ok || filters["tag"] != "!/(🇷🇺)/i" {
		t.Errorf("update patch: %+v", u.Patch)
	}
}

// TestSyncOutbounds_DisableUpdateRemovesFromStack — disable preset → update удаляется из стека.
func TestSyncOutbounds_DisableUpdateRemovesFromStack(t *testing.T) {
	outbounds := []configtypes.OutboundConfig{
		{
			Tag:  "proxy-out",
			Type: "selector",
			Updates: []configtypes.OutboundUpdate{
				{Ref: "russian", Patch: map[string]interface{}{"filters": map[string]interface{}{"tag": "foo"}}},
				{Ref: "ru-inside", Patch: map[string]interface{}{"filters": map[string]interface{}{"tag": "bar"}}},
			},
		},
	}
	// Only ru-inside active — russian's update должна исчезнуть.
	russianPreset := makeSyncTestPreset(t, "russian", `[]`)
	ruInsidePreset := makeSyncTestPreset(t, "ru-inside", `[
		{"mode":"update","tag":"proxy-out","filters":{"tag":"bar"}}
	]`)
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "ru-inside", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{russianPreset, ruInsidePreset})

	if len(outbounds[0].Updates) != 1 {
		t.Fatalf("expected 1 update remaining (ru-inside): %+v", outbounds[0].Updates)
	}
	if outbounds[0].Updates[0].Ref != "ru-inside" {
		t.Errorf("expected ref=ru-inside, got %q", outbounds[0].Updates[0].Ref)
	}
}

// TestSyncOutbounds_Idempotent — повторный sync с тем же state не меняет ничего.
func TestSyncOutbounds_Idempotent(t *testing.T) {
	preset := makeSyncTestPreset(t, "russian", `[
		{"mode":"add","tag":"ru VPN 🇷🇺","type":"selector"},
		{"mode":"update","tag":"proxy-out","filters":{"tag":"!/(🇷🇺)/i"}}
	]`)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
	}
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})
	snapshot, _ := json.Marshal(outbounds)
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})
	after, _ := json.Marshal(outbounds)
	if string(snapshot) != string(after) {
		t.Errorf("sync not idempotent:\nbefore: %s\nafter: %s", snapshot, after)
	}
}

// TestSyncOutbounds_PreserveOrder — preset entry preserve позицию в slice при повторном sync.
func TestSyncOutbounds_PreserveOrder(t *testing.T) {
	preset := makeSyncTestPreset(t, "russian", `[
		{"mode":"add","tag":"ru VPN","type":"selector"}
	]`)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
	}
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	// Initial sync — preset entry appended at end.
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})
	if outbounds[1].Tag != "ru VPN" {
		t.Fatalf("expected ru VPN at index 1: %+v", outbounds)
	}

	// User reordered — moved preset entry to index 0.
	outbounds[0], outbounds[1] = outbounds[1], outbounds[0]
	if outbounds[0].Tag != "ru VPN" {
		t.Fatal("swap setup failed")
	}

	// Second sync — должен preserve user-chosen order.
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})
	if outbounds[0].Tag != "ru VPN" || outbounds[1].Tag != "proxy-out" {
		t.Errorf("user reorder lost: %+v", outbounds)
	}
}

// TestSyncOutbounds_AdoptLegacyGlobal — existing global без Ref с tag'ом
// совпадающим с preset add → entry adopt'ится (Ref ставится), не дублируется.
//
// Сценарий backward compat: state от pre-SPEC-057 имеет preset outbound'ы как
// обычные globals (без ref). На первом Sync они должны стать preset-bound,
// иначе UI продолжит рендерить их как Global без 🔒 badge и юзер потеряет
// связь с preset'ом (Edit/Del разблокированы, lifecycle сломан).
func TestSyncOutbounds_AdoptLegacyGlobal(t *testing.T) {
	preset := makeSyncTestPreset(t, "russian", `[
		{"mode":"add","tag":"ru VPN 🇷🇺","type":"selector",
		 "options":{"default":"direct-out"}}
	]`)
	outbounds := []configtypes.OutboundConfig{
		{Tag: "proxy-out", Type: "selector"},
		// Legacy: preset entry лежит как global без ref
		// (промоутнут старым "promote-to-global" подходом до SPEC 057).
		{Tag: "ru VPN 🇷🇺", Type: "selector", Options: map[string]interface{}{"default": "direct-out"}},
	}
	rules := []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	SyncOutboundsWithActivePresets(rules, &outbounds, []template.Preset{preset})

	if len(outbounds) != 2 {
		t.Fatalf("expected 2 entries (no duplicate from preset add): %+v", outbounds)
	}
	// ru VPN остался на своей позиции (index 1), но получил ref.
	if outbounds[1].Tag != "ru VPN 🇷🇺" {
		t.Fatalf("expected ru VPN at index 1: %+v", outbounds)
	}
	if outbounds[1].Ref != "russian" {
		t.Errorf("legacy global должен быть adopt'нут с ref=russian: %+v", outbounds[1])
	}
	// proxy-out НЕ должен получить ref (preset не добавлял этот tag).
	if outbounds[0].Ref != "" {
		t.Errorf("proxy-out не должен получить ref (не в preset.add): %+v", outbounds[0])
	}
}
