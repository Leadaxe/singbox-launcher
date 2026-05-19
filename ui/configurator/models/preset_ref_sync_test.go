package models

import (
	"encoding/json"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	v6 "singbox-launcher/core/state/v6"
)

// TestSyncAllRulesToStateRulesV6_PresetOnly — только preset-ref'ы в model.
func TestSyncAllRulesToStateRulesV6_PresetOnly(t *testing.T) {
	prs := []*PresetRefState{
		{Ref: "ru-direct", Enabled: true, Vars: map[string]string{"dns_ip": "77.88.8.7"}},
	}
	out := SyncAllRulesToStateRulesV6(prs, nil)
	if len(out) != 1 || out[0].Kind != v6.RuleKindPreset || out[0].Ref != "ru-direct" {
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
	if r.Kind != v6.RuleKindInline {
		t.Errorf("kind: %q", r.Kind)
	}
	if r.ID == "" {
		t.Error("ID should be generated")
	}
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ib := body.(*v6.InlineBody)
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
	if len(out) != 1 || out[0].Kind != v6.RuleKindSrs {
		t.Errorf("kind: %+v", out)
	}
	body, _ := out[0].DecodeBody()
	sb := body.(*v6.SrsBody)
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
	kinds := []v6.RuleKind{out[0].Kind, out[1].Kind, out[2].Kind}
	want := []v6.RuleKind{v6.RuleKindPreset, v6.RuleKindInline, v6.RuleKindSrs}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("kind[%d]: got %q want %q", i, kinds[i], want[i])
		}
	}
}

// TestSyncDNSFullToStateV6_TemplateOnly — SPEC 057: template servers → overrides,
// user-added servers тихо игнорируются (state хранит только refs, не body).
func TestSyncDNSFullToStateV6_TemplateOnly(t *testing.T) {
	servers := []json.RawMessage{
		json.RawMessage(`{"tag":"google_doh","type":"https","enabled":true}`),
		json.RawMessage(`{"tag":"my-pihole","type":"udp","server":"192.168.1.5","enabled":true}`),
	}
	templateTags := map[string]bool{"google_doh": true}
	cfg := SyncDNSFullToStateV6(servers, "", nil, templateTags)

	if len(cfg.TemplateServers) != 1 {
		t.Errorf("template_servers count: %+v", cfg.TemplateServers)
	}
	if !cfg.TemplateServers["google_doh"].Enabled {
		t.Error("google_doh override should be true")
	}
	// SPEC 057: my-pihole (non-template) больше не попадает в state.
	// State поле ExtraServers удалено; такие серверы должны жить в template
	// или preset.dns_servers, иначе их вообще нет.
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
	if !cfg.TemplateServers["google_doh"].Enabled {
		t.Errorf("explicit override should win: %+v", cfg.TemplateServers)
	}
}

// TestSyncDNSFullToStateV6_RulesTextIgnored — SPEC 057: dnsRulesText аргумент
// сохранён для backward-compat, но игнорируется (state не хранит user DNS rules).
func TestSyncDNSFullToStateV6_RulesTextIgnored(t *testing.T) {
	rulesText := `{"rules":[{"server":"x","domain_suffix":["a.com"]}]}`
	cfg := SyncDNSFullToStateV6(nil, rulesText, nil, nil)
	// Никаких полей в v6.DNSConfig для DNS rules больше нет.
	// Проверяем что функция не паникует и возвращает пустой config.
	if len(cfg.TemplateServers) != 0 {
		t.Errorf("no template servers expected when only rulesText: %+v", cfg.TemplateServers)
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
