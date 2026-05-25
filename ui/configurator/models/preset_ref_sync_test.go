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
	if r.ID == "" {
		t.Error("ID should be generated")
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

// TestSyncDNSFullToStateV6_Split — SPEC 056-R-N: template tags → kind=template,
// user-added → kind=user в flat servers[].
func TestSyncDNSFullToStateV6_Split(t *testing.T) {
	servers := []json.RawMessage{
		json.RawMessage(`{"tag":"google_doh","type":"https","enabled":true}`),
		json.RawMessage(`{"tag":"my-pihole","type":"udp","server":"192.168.1.5","enabled":true}`),
	}
	templateTags := map[string]bool{"google_doh": true}
	cfg := SyncDNSFullToStateV6(servers, "", nil, templateTags)

	var tplGoogle, userPiHole *state.DNSServer
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if s.Kind == state.DNSServerKindTemplate && s.Tag == "google_doh" {
			tplGoogle = s
		}
		if s.Kind == state.DNSServerKindUser && s.Tag == "my-pihole" {
			userPiHole = s
		}
	}
	if tplGoogle == nil || !tplGoogle.Enabled {
		t.Errorf("google_doh template entry should be enabled: %+v", tplGoogle)
	}
	if userPiHole == nil {
		t.Fatalf("my-pihole user entry missing: %+v", cfg.Servers)
	}
	if userPiHole.Body["server"] != "192.168.1.5" {
		t.Errorf("user body lost server: %+v", userPiHole.Body)
	}
	if _, has := userPiHole.Body["enabled"]; has {
		t.Errorf("enabled should be stripped from user body: %v", userPiHole.Body)
	}
}

// TestSyncDNSFullToStateV6_ExplicitOverridesWin — DNSTemplateOverrides приоритетнее
// чем чтение enabled из raw server.
func TestSyncDNSFullToStateV6_ExplicitOverridesWin(t *testing.T) {
	servers := []json.RawMessage{
		json.RawMessage(`{"tag":"google_doh","type":"https","enabled":false}`),
	}
	templateTags := map[string]bool{"google_doh": true}
	overrides := map[string]bool{"google_doh": true} // явный override Enabled=true

	cfg := SyncDNSFullToStateV6(servers, "", overrides, templateTags)
	if len(cfg.Servers) != 1 || cfg.Servers[0].Kind != state.DNSServerKindTemplate || !cfg.Servers[0].Enabled {
		t.Errorf("explicit override should win: %+v", cfg.Servers)
	}
}

// TestSyncDNSFullToStateV6_RulesText — user rules парсятся из JSON text.
func TestSyncDNSFullToStateV6_RulesText(t *testing.T) {
	rulesText := `{"rules":[{"server":"x","domain_suffix":["a.com"]}]}`
	cfg := SyncDNSFullToStateV6(nil, rulesText, nil, nil)
	if len(cfg.Rules) != 1 {
		t.Fatalf("expected 1 user rule: %+v", cfg.Rules)
	}
	if cfg.Rules[0].Kind != state.DNSRuleKindUser || cfg.Rules[0].Body["server"] != "x" {
		t.Errorf("rule shape: %+v", cfg.Rules[0])
	}
}

// TestStableRuleID_Sanitize — sanity для генератора ID.
func TestStableRuleID_Sanitize(t *testing.T) {
	cases := map[string]string{
		"Hello World":            "rule-Hello-World",
		"Firefox через VPN":      "rule-Firefox--VPN", // не-ASCII strip'нуто
		"":                       "rule-unnamed",
		"name with !@# symbols!": "rule-name-with--symbols",
	}
	for label, want := range cases {
		rs := &RuleState{Rule: wizardtemplate.TemplateSelectableRule{Label: label}}
		if got := stableRuleID(rs); got != want {
			t.Errorf("label %q: got %q want %q", label, got, want)
		}
	}
}
