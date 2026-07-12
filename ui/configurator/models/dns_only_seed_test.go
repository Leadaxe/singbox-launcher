package models

import (
	"testing"

	"singbox-launcher/core/template"
)

// dns-only preset (dns_rules, no route Rules) vs a route preset (has Rules).
func tdWithMixedPresets() *template.TemplateData {
	return &template.TemplateData{
		Presets: []template.Preset{
			{ID: "fakeip", DNSRules: []map[string]interface{}{{"query_type": []string{"A"}, "server": "fakeip"}}},
			{ID: "russian", Rules: []map[string]interface{}{{"rule_set": "ru", "outbound": "@out"}},
				DNSRule: map[string]interface{}{"rule_set": "ru", "server": "y"}},
			{ID: "block-ads", Rules: []map[string]interface{}{{"rule_set": "ads", "outbound": "reject"}}},
		},
	}
}

func hasRef(m *WizardModel, ref string) *PresetRefState {
	for _, pr := range m.PresetRefs {
		if pr != nil && pr.Ref == ref {
			return pr
		}
	}
	return nil
}

func TestEnsureDNSOnlyPresetsSeeded_AddsMissingEnabled(t *testing.T) {
	m := &WizardModel{TemplateData: tdWithMixedPresets()}
	EnsureDNSOnlyPresetsSeeded(m)

	fakeip := hasRef(m, "fakeip")
	if fakeip == nil {
		t.Fatal("fakeip (dns-only) should be seeded")
	}
	if !fakeip.Enabled {
		t.Error("seeded ref must be Enabled=true (route level)")
	}
	if fakeip.DNSRuleEnabled != nil {
		t.Error("DNSRuleEnabled must be nil (default enabled) on fresh seed")
	}
	// route presets must NOT be seeded
	if hasRef(m, "russian") != nil || hasRef(m, "block-ads") != nil {
		t.Error("route presets must not be auto-seeded")
	}
}

func TestEnsureDNSOnlyPresetsSeeded_Idempotent(t *testing.T) {
	m := &WizardModel{TemplateData: tdWithMixedPresets()}
	EnsureDNSOnlyPresetsSeeded(m)
	EnsureDNSOnlyPresetsSeeded(m)
	EnsureDNSOnlyPresetsSeeded(m)
	n := 0
	for _, pr := range m.PresetRefs {
		if pr != nil && pr.Ref == "fakeip" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("fakeip seeded %d times, want exactly 1 (idempotent)", n)
	}
}

// The critical round-trip invariant: a user who disabled fakeip (DNSRuleEnabled
// = false) must NOT have it re-enabled by a re-seed.
func TestEnsureDNSOnlyPresetsSeeded_PreservesToggleOff(t *testing.T) {
	off := false
	m := &WizardModel{
		TemplateData: tdWithMixedPresets(),
		PresetRefs:   []*PresetRefState{{Ref: "fakeip", Enabled: true, DNSRuleEnabled: &off}},
	}
	EnsureDNSOnlyPresetsSeeded(m)

	fakeip := hasRef(m, "fakeip")
	if fakeip == nil || fakeip.DNSRuleEnabled == nil || *fakeip.DNSRuleEnabled != false {
		t.Fatalf("re-seed must preserve user's disabled state, got %+v", fakeip)
	}
	if fakeip.IsDNSRuleEnabled() {
		t.Error("disabled fakeip must stay disabled after re-seed")
	}
}

func TestEnsureDNSOnlyPresetsSeeded_NilSafe(t *testing.T) {
	EnsureDNSOnlyPresetsSeeded(nil)
	EnsureDNSOnlyPresetsSeeded(&WizardModel{}) // nil TemplateData
}

// IsDNSOnly classification sanity (drives library/rules-tab hiding).
func TestPresetIsDNSOnly(t *testing.T) {
	td := tdWithMixedPresets()
	if !td.Presets[0].IsDNSOnly() {
		t.Error("fakeip must be DNS-only")
	}
	if td.Presets[1].IsDNSOnly() {
		t.Error("russian (has route + dns) must NOT be DNS-only")
	}
	if td.Presets[2].IsDNSOnly() {
		t.Error("block-ads (route only) must NOT be DNS-only")
	}
}
