package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// TestMergePresets_NoopWhenEmpty — пустой ctx → routeRaw возвращается как есть.
func TestMergePresets_NoopWhenEmpty(t *testing.T) {
	raw := json.RawMessage(`{"rules":[{"protocol":"dns","action":"hijack-dns"}]}`)
	out, err := MergePresetsIntoRoute(raw, PresetMergeContext{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if string(out) != string(raw) {
		t.Errorf("noop expected, got %s", out)
	}
}

// TestMergePresets_AppendsActivePresetRule — active preset-ref добавляет fragments.
func TestMergePresets_AppendsActivePresetRule(t *testing.T) {
	presetJSON := `{
		"id": "private-ips",
		"label": "Private IPs",
		"rule": {"ip_is_private": true, "outbound": "direct-out"}
	}`
	var p template.Preset
	if err := json.Unmarshal([]byte(presetJSON), &p); err != nil {
		t.Fatalf("unmarshal preset: %v", err)
	}

	raw := json.RawMessage(`{"rules":[{"protocol":"dns","action":"hijack-dns"}],"rule_set":[]}`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{p},
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "private-ips", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	out, err := MergePresetsIntoRoute(raw, ctx)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var route map[string]interface{}
	if err := json.Unmarshal(out, &route); err != nil {
		t.Fatalf("unmarshal route: %v", err)
	}
	rules := route["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules (hijack + preset), got %d", len(rules))
	}
	// hijack-dns rule preserved
	first := rules[0].(map[string]interface{})
	if first["protocol"] != "dns" {
		t.Errorf("first rule should be hijack-dns: %+v", first)
	}
	// preset rule appended
	second := rules[1].(map[string]interface{})
	if second["outbound"] != "direct-out" {
		t.Errorf("preset rule outbound: %+v", second)
	}
	if second["ip_is_private"] != true {
		t.Errorf("preset rule match: %+v", second)
	}
}

// TestMergePresets_DisabledPresetRefSkipped — Enabled=false → не эмитим.
func TestMergePresets_DisabledPresetRefSkipped(t *testing.T) {
	raw := json.RawMessage(`{"rules":[],"rule_set":[]}`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{{ID: "x", Label: "X", Rule: map[string]interface{}{"ip_is_private": true, "outbound": "direct-out"}}},
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "x", Enabled: false, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	out, _ := MergePresetsIntoRoute(raw, ctx)
	// noop — preset disabled, hasAnyPresetRef false.
	if string(out) != string(raw) {
		t.Errorf("disabled preset should be noop, got %s", out)
	}
}

// TestMergePresets_BrokenRefWarningSkip — ref не найден → skip, route не падает.
func TestMergePresets_BrokenRefWarningSkip(t *testing.T) {
	raw := json.RawMessage(`{"rules":[],"rule_set":[]}`)
	ctx := PresetMergeContext{
		Presets: nil, // нет presets — все refs broken
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "nonexistent", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	out, err := MergePresetsIntoRoute(raw, ctx)
	if err != nil {
		t.Fatalf("should not fail on broken ref: %v", err)
	}
	var route map[string]interface{}
	if err := json.Unmarshal(out, &route); err != nil {
		t.Fatalf("unmarshal route: %v", err)
	}
	rules, _ := route["rules"].([]interface{})
	if len(rules) != 0 {
		t.Errorf("broken ref should not emit: %+v", rules)
	}
}

// TestMergePresets_DNSBundledServer — bundled DNS из active preset попадает в dns.servers.
func TestMergePresets_DNSBundledServer(t *testing.T) {
	presetJSON := `{
		"id": "ru-direct",
		"vars": [
			{"name": "out", "type": "outbound", "default": "proxy-out"},
			{"name": "dns_server", "type": "dns_server", "default": "yandex_udp", "select": "local"}
		],
		"rule_set": [{"tag": "domains", "type": "inline", "rules": [{"domain_suffix": ["ru"]}]}],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.8", "server_port": 53, "detour": "@out"}
		],
		"rule": {"rule_set": "domains", "outbound": "@out"},
		"dns_rule": {"rule_set": "domains", "server": "@dns_server"}
	}`
	var p template.Preset
	if err := json.Unmarshal([]byte(presetJSON), &p); err != nil {
		t.Fatalf("unmarshal preset: %v", err)
	}

	dnsRaw := json.RawMessage(`{"servers":[{"tag":"google_doh","type":"https"}],"rules":[]}`)
	ctx := PresetMergeContext{
		Presets: []template.Preset{p},
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "ru-direct", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
		// SPEC 056-R-N: kind=preset entry в DNSOptions.Servers (sync должен был создать).
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindPreset, Ref: "ru-direct:yandex_udp", Enabled: true},
			},
		},
	}
	out, err := MergePresetsIntoDNS(dnsRaw, ctx)
	if err != nil {
		t.Fatalf("dns merge: %v", err)
	}
	var dns map[string]interface{}
	if err := json.Unmarshal(out, &dns); err != nil {
		t.Fatalf("unmarshal dns: %v", err)
	}
	servers := dns["servers"].([]interface{})
	if len(servers) != 2 { // google_doh + ru-direct:yandex_udp
		t.Fatalf("expected 2 servers, got %d: %+v", len(servers), servers)
	}
	// Yandex bundled должен быть с префиксованным tag и detour=proxy-out
	bundled := servers[1].(map[string]interface{})
	if bundled["tag"] != "ru-direct:yandex_udp" {
		t.Errorf("bundled tag prefix: %v", bundled["tag"])
	}
	if bundled["detour"] != "proxy-out" {
		t.Errorf("detour substitution: %v", bundled["detour"])
	}
}

// TestMergePresets_DNSTemplateServerDisabled — SPEC 056-R-N: kind=template
// entry с Enabled=false → server отфильтрован, остальные template-серверы
// материализуются через TemplateDNSDefaults.
func TestMergePresets_DNSTemplateServerDisabled(t *testing.T) {
	dnsRaw := json.RawMessage(`{"servers":[]}`)
	ctx := PresetMergeContext{
		TemplateDNSDefaults: []TemplateDNSServer{
			{Tag: "google_doh", Enabled: true, Raw: map[string]interface{}{
				"tag": "google_doh", "type": "https", "server": "dns.google",
			}},
			{Tag: "cloudflare_udp", Enabled: true, Raw: map[string]interface{}{
				"tag": "cloudflare_udp", "type": "udp", "server": "1.1.1.1",
			}},
		},
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindTemplate, Tag: "google_doh", Enabled: true},
				{Kind: state.DNSServerKindTemplate, Tag: "cloudflare_udp", Enabled: false},
			},
		},
	}
	out, err := MergePresetsIntoDNS(dnsRaw, ctx)
	if err != nil {
		t.Fatalf("dns merge: %v", err)
	}
	if !strings.Contains(string(out), "google_doh") {
		t.Error("google_doh should remain")
	}
	if strings.Contains(string(out), "cloudflare_udp") {
		t.Error("cloudflare_udp should be filtered out (Enabled=false)")
	}
}

// TestMergePresets_DNSUserServers — kind=user entries в state.DNSOptions.Servers
// эмитятся в config через stripDNSWizardOnlyFields. kind=user rules — то же
// через cleanDanglingDNSRule.
func TestMergePresets_DNSUserServers(t *testing.T) {
	dnsRaw := json.RawMessage(`{"servers":[]}`)
	ctx := PresetMergeContext{
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindUser, Tag: "my-pihole", Enabled: true, Body: map[string]interface{}{
					"tag": "my-pihole", "type": "udp", "server": "192.168.1.5",
				}},
			},
			Rules: []state.DNSRule{
				{Kind: state.DNSRuleKindUser, Enabled: true, Body: map[string]interface{}{
					"server": "my-pihole", "domain_suffix": []interface{}{"internal.local"},
				}},
			},
		},
	}
	out, err := MergePresetsIntoDNS(dnsRaw, ctx)
	if err != nil {
		t.Fatalf("dns merge: %v", err)
	}
	if !strings.Contains(string(out), "my-pihole") {
		t.Errorf("user server should appear: %s", out)
	}
	if !strings.Contains(string(out), "internal.local") {
		t.Errorf("user rule should appear: %s", out)
	}
}

// TestHasAnyPresetRef — sanity на helper.
func TestHasAnyPresetRef(t *testing.T) {
	if hasAnyPresetRef(nil) {
		t.Error("nil should be false")
	}
	if hasAnyPresetRef([]state.Rule{{Kind: state.RuleKindInline, ID: "x", Enabled: true}}) {
		t.Error("inline should not count")
	}
	if hasAnyPresetRef([]state.Rule{{Kind: state.RuleKindPreset, Ref: "x", Enabled: false}}) {
		t.Error("disabled preset should not count")
	}
	if !hasAnyPresetRef([]state.Rule{{Kind: state.RuleKindPreset, Ref: "x", Enabled: true}}) {
		t.Error("enabled preset should count")
	}
}
