package models

import "testing"

func TestRebuildDNSRuleOrder_EmptyModel(t *testing.T) {
	m := &WizardModel{}
	RebuildDNSRuleOrder(m)
	if len(m.DNSRuleOrder) != 0 {
		t.Fatalf("expected empty DNSRuleOrder, got %d slots", len(m.DNSRuleOrder))
	}
}

func TestRebuildDNSRuleOrder_UsersThenPresets(t *testing.T) {
	m := &WizardModel{
		DNSUserRules: []DNSUserRule{
			{Enabled: true, Body: map[string]interface{}{"domain": "a"}},
			{Enabled: true, Body: map[string]interface{}{"domain": "b"}},
		},
		PresetRefs: []*PresetRefState{
			{Ref: "preset1"},
			{Ref: "preset2"},
		},
	}
	RebuildDNSRuleOrder(m)
	if len(m.DNSRuleOrder) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(m.DNSRuleOrder))
	}
	want := []DNSRuleSlot{
		{Kind: DNSSlotKindUser, Index: 0},
		{Kind: DNSSlotKindUser, Index: 1},
		{Kind: DNSSlotKindPresetRef, Index: 0},
		{Kind: DNSSlotKindPresetRef, Index: 1},
	}
	for i, s := range m.DNSRuleOrder {
		if s != want[i] {
			t.Errorf("slot[%d] = %+v, want %+v", i, s, want[i])
		}
	}
}

func TestReconcileDNSRuleOrder_DropsStaleSlots(t *testing.T) {
	m := &WizardModel{
		DNSUserRules: []DNSUserRule{
			{Enabled: true, Body: map[string]interface{}{"domain": "a"}},
		},
		PresetRefs: []*PresetRefState{{Ref: "p1"}},
		DNSRuleOrder: []DNSRuleSlot{
			{Kind: DNSSlotKindUser, Index: 0},
			{Kind: DNSSlotKindUser, Index: 99},     // stale — past end
			{Kind: DNSSlotKindPresetRef, Index: 0},
			{Kind: DNSSlotKindPresetRef, Index: 5}, // stale
		},
	}
	ReconcileDNSRuleOrder(m)
	if len(m.DNSRuleOrder) != 2 {
		t.Fatalf("expected 2 slots after reconcile, got %d", len(m.DNSRuleOrder))
	}
	if m.DNSRuleOrder[0].Kind != DNSSlotKindUser || m.DNSRuleOrder[0].Index != 0 {
		t.Errorf("slot[0] wrong: %+v", m.DNSRuleOrder[0])
	}
	if m.DNSRuleOrder[1].Kind != DNSSlotKindPresetRef || m.DNSRuleOrder[1].Index != 0 {
		t.Errorf("slot[1] wrong: %+v", m.DNSRuleOrder[1])
	}
}

func TestReconcileDNSRuleOrder_AppendsMissing(t *testing.T) {
	m := &WizardModel{
		DNSUserRules: []DNSUserRule{
			{Enabled: true, Body: map[string]interface{}{"domain": "a"}},
			{Enabled: true, Body: map[string]interface{}{"domain": "b"}}, // new
		},
		PresetRefs: []*PresetRefState{
			{Ref: "p1"},
			{Ref: "p2"}, // new
		},
		DNSRuleOrder: []DNSRuleSlot{
			{Kind: DNSSlotKindUser, Index: 0},
			{Kind: DNSSlotKindPresetRef, Index: 0},
		},
	}
	ReconcileDNSRuleOrder(m)
	if len(m.DNSRuleOrder) != 4 {
		t.Fatalf("expected 4 slots after reconcile, got %d", len(m.DNSRuleOrder))
	}
	// new ones appended at the end
	if m.DNSRuleOrder[2].Kind != DNSSlotKindUser || m.DNSRuleOrder[2].Index != 1 {
		t.Errorf("slot[2] wrong: %+v", m.DNSRuleOrder[2])
	}
	if m.DNSRuleOrder[3].Kind != DNSSlotKindPresetRef || m.DNSRuleOrder[3].Index != 1 {
		t.Errorf("slot[3] wrong: %+v", m.DNSRuleOrder[3])
	}
}

func TestCompactDNSRuleOrderIndices_ShiftsAfterRemoval(t *testing.T) {
	// Setup: 3 user rules, 2 presets, mixed order. Remove user rule at index 1.
	m := &WizardModel{
		DNSRuleOrder: []DNSRuleSlot{
			{Kind: DNSSlotKindUser, Index: 0},
			{Kind: DNSSlotKindPresetRef, Index: 0},
			{Kind: DNSSlotKindUser, Index: 1}, // to be removed
			{Kind: DNSSlotKindUser, Index: 2}, // shifts to 1
			{Kind: DNSSlotKindPresetRef, Index: 1},
		},
	}
	CompactDNSRuleOrderIndices(m, DNSSlotKindUser, 1)
	want := []DNSRuleSlot{
		{Kind: DNSSlotKindUser, Index: 0},
		{Kind: DNSSlotKindPresetRef, Index: 0},
		{Kind: DNSSlotKindUser, Index: 1}, // was 2, shifted -1
		{Kind: DNSSlotKindPresetRef, Index: 1},
	}
	if len(m.DNSRuleOrder) != len(want) {
		t.Fatalf("expected %d slots, got %d", len(want), len(m.DNSRuleOrder))
	}
	for i, s := range m.DNSRuleOrder {
		if s != want[i] {
			t.Errorf("slot[%d] = %+v, want %+v", i, s, want[i])
		}
	}
}

func TestCompactDNSRuleOrderIndices_LeavesOtherKindAlone(t *testing.T) {
	m := &WizardModel{
		DNSRuleOrder: []DNSRuleSlot{
			{Kind: DNSSlotKindPresetRef, Index: 0},
			{Kind: DNSSlotKindUser, Index: 0},
			{Kind: DNSSlotKindPresetRef, Index: 1}, // not affected
		},
	}
	CompactDNSRuleOrderIndices(m, DNSSlotKindUser, 0)
	// User slot dropped; preset indices unchanged.
	want := []DNSRuleSlot{
		{Kind: DNSSlotKindPresetRef, Index: 0},
		{Kind: DNSSlotKindPresetRef, Index: 1},
	}
	if len(m.DNSRuleOrder) != len(want) {
		t.Fatalf("expected %d slots, got %d", len(want), len(m.DNSRuleOrder))
	}
	for i, s := range m.DNSRuleOrder {
		if s != want[i] {
			t.Errorf("slot[%d] = %+v, want %+v", i, s, want[i])
		}
	}
}

func TestCompactDNSRuleOrderIndices_NilSafe(t *testing.T) {
	// Should not panic.
	CompactDNSRuleOrderIndices(nil, DNSSlotKindUser, 0)
}

func TestDNSUserRulesFromText_RoundTrip(t *testing.T) {
	text := `{"rules":[{"domain":"example.com","server":"my-dns"},{"domain_suffix":["ru"],"server":"direct_dns"}]}`
	rules := DNSUserRulesFromText(text)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Body["domain"] != "example.com" {
		t.Errorf("rule[0].domain = %v, want example.com", rules[0].Body["domain"])
	}
	if !rules[0].Enabled {
		t.Errorf("rule[0] should be Enabled by default")
	}

	// Serialize back; should preserve order + bodies.
	out := DNSUserRulesToText(rules)
	if out == "" {
		t.Fatalf("DNSUserRulesToText returned empty")
	}
	// Round-trip parse.
	rules2 := DNSUserRulesFromText(out)
	if len(rules2) != 2 {
		t.Fatalf("after round-trip: expected 2 rules, got %d", len(rules2))
	}
	if rules2[0].Body["domain"] != "example.com" {
		t.Errorf("round-trip: rule[0].domain = %v", rules2[0].Body["domain"])
	}
}

func TestDNSUserRulesFromText_EmptyReturnsNil(t *testing.T) {
	if rules := DNSUserRulesFromText(""); rules != nil {
		t.Errorf("expected nil for empty text, got %d rules", len(rules))
	}
	if rules := DNSUserRulesFromText("   \n   "); rules != nil {
		t.Errorf("expected nil for whitespace, got %d rules", len(rules))
	}
}

func TestDNSUserRulesFromText_StripsTopLevelKindFields(t *testing.T) {
	text := `{"rules":[{"kind":"user","ref":"foo","enabled":false,"domain":"x"}]}`
	rules := DNSUserRulesFromText(text)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if _, has := rules[0].Body["kind"]; has {
		t.Errorf("kind should be stripped from Body")
	}
	if _, has := rules[0].Body["ref"]; has {
		t.Errorf("ref should be stripped from Body")
	}
	if _, has := rules[0].Body["enabled"]; has {
		t.Errorf("enabled should be stripped from Body")
	}
	if rules[0].Body["domain"] != "x" {
		t.Errorf("domain should be preserved")
	}
}
