package presentation

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// helper: find DNS server entry in model.DNSServers by tag.
func findDNSServerByTag(t *testing.T, model *wizardmodels.WizardModel, tag string) map[string]interface{} {
	t.Helper()
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if got, _ := m["tag"].(string); got == tag {
			return m
		}
	}
	return nil
}

// TestPopulateUserDNSFromState_RestoresUserServerAndRule — SPEC 056 phase 7
// regression: после Save+reopen v6-state user-добавленный DNS-сервер
// (kind=user) и user DNS rule должны вернуться в model.DNSServers /
// model.DNSRulesText. До фикса restoreDNS читал только legacy
// sf.DNSOptions, который для v6 == nil.
func TestPopulateUserDNSFromState_RestoresUserServerAndRule(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	dns := state.DNSOptions{
		Servers: []state.DNSServer{
			{
				Kind:    state.DNSServerKindUser,
				Tag:     "my-pihole",
				Enabled: true,
				Body: map[string]interface{}{
					"type":        "udp",
					"server":      "192.168.1.5",
					"server_port": float64(53),
				},
			},
		},
		Rules: []state.DNSRule{
			{
				Kind:    state.DNSRuleKindUser,
				Enabled: true,
				Body: map[string]interface{}{
					"rule_set": "ru-domains",
					"server":   "my-pihole",
				},
			},
		},
	}

	populateUserDNSFromState(model, dns)

	srv := findDNSServerByTag(t, model, "my-pihole")
	if srv == nil {
		t.Fatalf("user DNS server my-pihole was not restored; DNSServers=%v", model.DNSServers)
	}
	if got, _ := srv["type"].(string); got != "udp" {
		t.Errorf("type: got %v want udp", srv["type"])
	}
	if got, _ := srv["server"].(string); got != "192.168.1.5" {
		t.Errorf("server: got %v want 192.168.1.5", srv["server"])
	}
	if got, _ := srv["server_port"].(float64); got != 53 {
		t.Errorf("server_port: got %v want 53", srv["server_port"])
	}
	if got, _ := srv["enabled"].(bool); !got {
		t.Errorf("enabled: got %v want true", srv["enabled"])
	}
	// kind/ref must NOT leak into wizard JSON shape.
	if _, has := srv["kind"]; has {
		t.Errorf("wizard server JSON must not contain kind field, got %v", srv["kind"])
	}
	if _, has := srv["ref"]; has {
		t.Errorf("wizard server JSON must not contain ref field, got %v", srv["ref"])
	}

	if strings.TrimSpace(model.DNSRulesText) == "" {
		t.Fatalf("DNSRulesText must contain user rule, got empty")
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(model.DNSRulesText), &root); err != nil {
		t.Fatalf("DNSRulesText is not valid JSON object: %v; text=%q", err, model.DNSRulesText)
	}
	rules, ok := root["rules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Fatalf("DNSRulesText.rules: got %v, want array of 1 element", root["rules"])
	}
	rule := rules[0].(map[string]interface{})
	if got, _ := rule["rule_set"].(string); got != "ru-domains" {
		t.Errorf("rule.rule_set: got %v want ru-domains", rule["rule_set"])
	}
	if got, _ := rule["server"].(string); got != "my-pihole" {
		t.Errorf("rule.server: got %v want my-pihole", rule["server"])
	}
}

// TestPopulateUserDNSFromState_EmptyStateNoCrash — defense against panics
// when sf.DNS is empty (fresh install / no user DNS entries).
func TestPopulateUserDNSFromState_EmptyStateNoCrash(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	model.DNSRulesText = ""
	populateUserDNSFromState(model, state.DNSOptions{})
	if len(model.DNSServers) != 0 {
		t.Errorf("empty state must not pollute DNSServers, got %v", model.DNSServers)
	}
	if model.DNSRulesText != "" {
		t.Errorf("empty state must not pollute DNSRulesText, got %q", model.DNSRulesText)
	}
}

// TestPopulateUserDNSFromState_NilModelNoCrash — defense.
func TestPopulateUserDNSFromState_NilModelNoCrash(t *testing.T) {
	populateUserDNSFromState(nil, state.DNSOptions{
		Servers: []state.DNSServer{{Kind: state.DNSServerKindUser, Tag: "x", Enabled: true}},
	})
}

// TestPopulateUserDNSFromState_SkipsTemplateAndPresetKinds — only kind=user
// touches model. Template/preset enabled-state lives in DNSTemplateOverrides
// and PresetRefState respectively.
func TestPopulateUserDNSFromState_SkipsTemplateAndPresetKinds(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	dns := state.DNSOptions{
		Servers: []state.DNSServer{
			{Kind: state.DNSServerKindTemplate, Tag: "cloudflare_udp", Enabled: true},
			{Kind: state.DNSServerKindPreset, Ref: "russian:yandex_udp", Enabled: true},
		},
		Rules: []state.DNSRule{
			{Kind: state.DNSRuleKindPreset, Ref: "russian", Enabled: true},
		},
	}
	populateUserDNSFromState(model, dns)
	if len(model.DNSServers) != 0 {
		t.Errorf("template/preset kinds must be skipped, got %v", model.DNSServers)
	}
	if model.DNSRulesText != "" {
		t.Errorf("preset rules must not touch DNSRulesText, got %q", model.DNSRulesText)
	}
}

// TestPopulateUserDNSFromState_IdempotentByTag — when legacy v5 path already
// populated DNSServers (v5→v6 migration round-trip), don't double-add the
// same tag.
func TestPopulateUserDNSFromState_IdempotentByTag(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	model.DNSServers = []json.RawMessage{
		json.RawMessage(`{"tag":"my-pihole","type":"udp","server":"10.0.0.1","enabled":true}`),
	}
	dns := state.DNSOptions{
		Servers: []state.DNSServer{
			{
				Kind: state.DNSServerKindUser, Tag: "my-pihole", Enabled: true,
				Body: map[string]interface{}{"type": "udp", "server": "192.168.1.5"},
			},
		},
	}
	populateUserDNSFromState(model, dns)
	if len(model.DNSServers) != 1 {
		t.Fatalf("duplicate tag must not be added, got %d entries", len(model.DNSServers))
	}
	// Existing entry wins (we skip on dup).
	srv := findDNSServerByTag(t, model, "my-pihole")
	if got, _ := srv["server"].(string); got != "10.0.0.1" {
		t.Errorf("existing entry should not be overwritten; got server=%v want 10.0.0.1", srv["server"])
	}
}

// TestPopulateUserDNSFromState_PreservesExistingRulesText — DNSRulesText
// already set (e.g. by legacy LoadPersistedWizardDNS) must NOT be clobbered
// by v6 user rules.
func TestPopulateUserDNSFromState_PreservesExistingRulesText(t *testing.T) {
	model := wizardmodels.NewWizardModel()
	model.DNSRulesText = `{"rules":[{"server":"existing"}]}`
	dns := state.DNSOptions{
		Rules: []state.DNSRule{
			{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{"server": "new"}},
		},
	}
	populateUserDNSFromState(model, dns)
	if !strings.Contains(model.DNSRulesText, "existing") {
		t.Errorf("existing DNSRulesText must be preserved, got %q", model.DNSRulesText)
	}
	if strings.Contains(model.DNSRulesText, "new") {
		t.Errorf("v6 user rules must not overwrite non-empty DNSRulesText, got %q", model.DNSRulesText)
	}
}
