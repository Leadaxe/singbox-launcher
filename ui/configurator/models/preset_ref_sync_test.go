package models

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
)

// TestSyncAllRulesToStateRulesV6_PresetOnly — только preset-ref'ы в model.
func TestSyncAllRulesToStateRulesV6_PresetOnly(t *testing.T) {
	prs := []*PresetRefState{
		{Ref: "ru-direct", Enabled: true, Vars: map[string]string{"dns_ip": "77.88.8.7"}},
	}
	out := SyncAllRulesToStateRulesV6(prs, nil)
	if len(out) != 1 || out[0].Kind != state.RuleKindPreset || out[0].Ref != "ru-direct" {
		t.Errorf("preset sync: %+v", out)
	}
}

// TestSyncAllRulesToStateRulesV6_InlineFromCustomRule — kind=inline из legacy CustomRule.
func TestSyncAllRulesToStateRulesV6_InlineFromCustomRule(t *testing.T) {
	cr := []*RuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Firefox VPN",
				Rule: map[string]interface{}{
					"domain_suffix": []interface{}{"example.com"},
					"outbound":      "proxy-out", // должно быть stripped
				},
			},
			Enabled:          true,
			SelectedOutbound: "proxy-out",
		},
	}
	out := SyncAllRulesToStateRulesV6(nil, cr)
	if len(out) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out))
	}
	r := out[0]
	if r.Kind != state.RuleKindInline {
		t.Errorf("kind: %q", r.Kind)
	}
	// SPEC 063: identity вычислима через StableRuleID; ID-поля в struct больше нет.
	if got := state.StableRuleID(r); got != "Firefox-VPN" {
		t.Errorf("StableRuleID: %q (want sanitize of label)", got)
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ib := body.(*state.InlineBody)
	if ib.Name != "Firefox VPN" || ib.Outbound != "proxy-out" {
		t.Errorf("inline body: %+v", ib)
	}
	if _, has := ib.Match["outbound"]; has {
		t.Errorf("outbound should be stripped from match: %+v", ib.Match)
	}
}

// TestSyncAllRulesToStateRulesV6_SrsFromCustomRule — kind=srs детектится по remote rule_set.
func TestSyncAllRulesToStateRulesV6_SrsFromCustomRule(t *testing.T) {
	rsRaw := json.RawMessage(`{"type":"remote","url":"https://example.com/list.srs"}`)
	cr := []*RuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label:    "Block list",
				Rule:     map[string]interface{}{"outbound": "reject"},
				RuleSets: []json.RawMessage{rsRaw},
			},
			Enabled:          true,
			SelectedOutbound: "reject",
		},
	}
	out := SyncAllRulesToStateRulesV6(nil, cr)
	if len(out) != 1 || out[0].Kind != state.RuleKindSrs {
		t.Errorf("kind: %+v", out)
	}
	body, _ := out[0].DecodeBody()
	sb := body.(*state.SrsBody)
	if sb.SrsURL != "https://example.com/list.srs" {
		t.Errorf("srs url: %q", sb.SrsURL)
	}
	if sb.Outbound != "reject" {
		t.Errorf("outbound: %q", sb.Outbound)
	}
}

// TestSyncAllRulesToStateRulesV6_Mixed — preset + inline + srs одновременно.
func TestSyncAllRulesToStateRulesV6_Mixed(t *testing.T) {
	prs := []*PresetRefState{{Ref: "x", Enabled: true, Vars: map[string]string{}}}
	cr := []*RuleState{
		{
			Rule:             wizardtemplate.TemplateSelectableRule{Label: "I", Rule: map[string]interface{}{"port": []interface{}{443}}},
			Enabled:          true,
			SelectedOutbound: "direct-out",
		},
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label:    "S",
				Rule:     map[string]interface{}{},
				RuleSets: []json.RawMessage{json.RawMessage(`{"type":"remote","url":"https://x"}`)},
			},
			Enabled:          true,
			SelectedOutbound: "proxy-out",
		},
	}
	out := SyncAllRulesToStateRulesV6(prs, cr)
	if len(out) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(out))
	}
	kinds := []state.RuleKind{out[0].Kind, out[1].Kind, out[2].Kind}
	want := []state.RuleKind{state.RuleKindPreset, state.RuleKindInline, state.RuleKindSrs}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kind[%d]: got %q want %q", i, kinds[i], want[i])
		}
	}
}

// TestStableRuleID_FromLegacyRuleState — SPEC 063: identity конвертированного
// legacy RuleState вычислима через `state.StableRuleID` от resulting state.Rule.
// Прежняя локальная `stableRuleID` (с префиксом "rule-") удалена; identity
// теперь чистая sanitize(body.name).
func TestStableRuleID_FromLegacyRuleState(t *testing.T) {
	cases := map[string]string{
		"Hello World":            "Hello-World",
		"Firefox через VPN":      "Firefox--VPN", // не-ASCII strip'нуто
		"name with !@# symbols!": "name-with--symbols",
	}
	for label, want := range cases {
		rs := &RuleState{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: label,
				Rule:  map[string]interface{}{"port": []interface{}{443}},
			},
			SelectedOutbound: "direct-out",
		}
		r := customRuleStateToV6Rule(rs)
		if r == nil {
			t.Fatalf("label %q: conversion returned nil", label)
		}
		if got := state.StableRuleID(*r); got != want {
			t.Errorf("label %q: StableRuleID got %q want %q", label, got, want)
		}
	}
}
