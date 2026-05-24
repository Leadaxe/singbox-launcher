package template

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPreset_RoundTrip_Form1_InlineMatch — простой preset без rule_set'ов.
// Match-поля inline в rule (ip_is_private). Один outbound var.
func TestPreset_RoundTrip_Form1_InlineMatch(t *testing.T) {
	raw := []byte(`{
		"id": "private-ips-direct",
		"label": "Private IPs direct",
		"description": "Route LAN traffic directly.",
		"default_enabled": true,
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out", "title": "Outbound"}
		],
		"rule": {"ip_is_private": true, "outbound": "@out"}
	}`)
	var p Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.ID != "private-ips-direct" || p.Label != "Private IPs direct" {
		t.Errorf("id/label mismatch: got id=%q label=%q", p.ID, p.Label)
	}
	if !p.DefaultEnabled {
		t.Error("DefaultEnabled should be true")
	}
	if len(p.Vars) != 1 || p.Vars[0].Name != "out" || p.Vars[0].Type != "outbound" {
		t.Errorf("vars mismatch: %+v", p.Vars)
	}
	if len(p.RuleSet) != 0 {
		t.Errorf("rule_set should be empty: %+v", p.RuleSet)
	}
	if p.Rule["ip_is_private"] != true {
		t.Errorf("rule.ip_is_private should be true: %+v", p.Rule)
	}
}

// TestPreset_RoundTrip_Form3_MultiRuleSet — multi rule_set с OR-композицией.
func TestPreset_RoundTrip_Form3_MultiRuleSet(t *testing.T) {
	raw := []byte(`{
		"id": "ru-blocked",
		"label": "Russian blocked resources",
		"vars": [
			{"name": "out", "type": "outbound", "default": "proxy-out"}
		],
		"rule_set": [
			{"tag": "main",      "type": "remote", "format": "binary", "url": "https://example.com/main.srs"},
			{"tag": "community", "type": "remote", "format": "binary", "url": "https://example.com/community.srs"}
		],
		"rule": {"rule_set": ["main", "community"], "network": ["tcp","udp"], "outbound": "@out"}
	}`)
	var p Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(p.RuleSet) != 2 {
		t.Errorf("expected 2 rule_set entries, got %d", len(p.RuleSet))
	}
	if p.RuleSet[0].Tag != "main" || p.RuleSet[0].Type != "remote" {
		t.Errorf("rule_set[0] mismatch: %+v", p.RuleSet[0])
	}
	if p.RuleSet[1].URL != "https://example.com/community.srs" {
		t.Errorf("rule_set[1] URL mismatch: %+v", p.RuleSet[1])
	}
	// rule.rule_set должно остаться массивом
	ruleSet, ok := p.Rule["rule_set"].([]interface{})
	if !ok || len(ruleSet) != 2 {
		t.Errorf("rule.rule_set should be array of 2: %+v", p.Rule["rule_set"])
	}
}

// TestPreset_RoundTrip_Form2_BundledDNS — preset с bundled DNS-сервером под условием.
func TestPreset_RoundTrip_Form2_BundledDNS(t *testing.T) {
	raw := []byte(`{
		"id": "ru-direct-mini",
		"label": "Russian domains direct (mini)",
		"default_enabled": true,
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"},
			{"name": "use_yandex_dns", "type": "bool", "default": "true",
			 "title": "Use Yandex DNS"},
			{"name": "dns_ip", "type": "enum", "default": "77.88.8.88",
			 "if": ["use_yandex_dns"], "options": [
				{"title": "Safe (77.88.8.88)", "value": "77.88.8.88"},
				{"title": "Family (77.88.8.7)", "value": "77.88.8.7"}
			 ]}
		],
		"rule_set": [
			{"tag": "domains", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["ru","xn--p1ai"]}]}
		],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "@dns_ip",
			 "server_port": 53, "detour": "@out",
			 "if": ["use_yandex_dns"]}
		],
		"rule":     {"rule_set": "domains", "outbound": "@out"},
		"dns_rule": {"rule_set": "domains", "server": "yandex_udp",
		             "if": ["use_yandex_dns"]}
	}`)
	var p Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Vars
	if len(p.Vars) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(p.Vars))
	}
	// dns_ip имеет if + options
	dnsIPVar := p.Vars[2]
	if dnsIPVar.Name != "dns_ip" || dnsIPVar.Type != "enum" {
		t.Errorf("dns_ip var mismatch: %+v", dnsIPVar)
	}
	if len(dnsIPVar.If) != 1 || dnsIPVar.If[0] != "use_yandex_dns" {
		t.Errorf("dns_ip.If mismatch: %+v", dnsIPVar.If)
	}
	// Options decode
	enum, tags, ok := dnsIPVar.DecodeOptions()
	if !ok || len(tags) != 0 {
		t.Errorf("DecodeOptions ok=%v tags=%v (expected enum non-nil)", ok, tags)
	}
	if len(enum) != 2 || enum[0].Title != "Safe (77.88.8.88)" || enum[0].Value != "77.88.8.88" {
		t.Errorf("enum decode mismatch: %+v", enum)
	}

	// rule_set
	if len(p.RuleSet) != 1 || p.RuleSet[0].Tag != "domains" || p.RuleSet[0].Type != "inline" {
		t.Errorf("rule_set mismatch: %+v", p.RuleSet)
	}

	// dns_servers
	if len(p.DNSServers) != 1 {
		t.Fatalf("expected 1 dns_server, got %d", len(p.DNSServers))
	}
	ds := p.DNSServers[0]
	if ds.Tag != "yandex_udp" || ds.Type != "udp" || ds.Server != "@dns_ip" || ds.Detour != "@out" {
		t.Errorf("dns_server mismatch: %+v", ds)
	}
	if len(ds.If) != 1 || ds.If[0] != "use_yandex_dns" {
		t.Errorf("dns_server.If mismatch: %+v", ds.If)
	}

	// dns_rule
	if p.DNSRule["server"] != "yandex_udp" {
		t.Errorf("dns_rule.server mismatch: %+v", p.DNSRule)
	}
	dnsRuleIf, _ := p.DNSRule["if"].([]interface{})
	if len(dnsRuleIf) != 1 {
		t.Errorf("dns_rule.if mismatch: %+v", p.DNSRule["if"])
	}
}

// TestPreset_DecodeOptions_DnsServerWhitelist — explicit []string whitelist для dns_server.
func TestPreset_DecodeOptions_DnsServerWhitelist(t *testing.T) {
	raw := []byte(`{
		"name": "dns_server", "type": "dns_server", "default": "yandex_udp",
		"options": ["yandex_udp", "yandex_doh", "yandex_dot"]
	}`)
	var v PresetVar
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	enum, tags, ok := v.DecodeOptions()
	if !ok {
		t.Fatal("DecodeOptions failed")
	}
	if enum != nil {
		t.Errorf("enum should be nil for dns_server type: %+v", enum)
	}
	if len(tags) != 3 || tags[0] != "yandex_udp" || tags[2] != "yandex_dot" {
		t.Errorf("tags mismatch: %+v", tags)
	}
}

// TestPreset_DecodeOptions_Empty — options пропущен → ok=true, оба nil.
func TestPreset_DecodeOptions_Empty(t *testing.T) {
	v := PresetVar{Name: "out", Type: "outbound", Default: "direct-out"}
	enum, tags, ok := v.DecodeOptions()
	if !ok {
		t.Fatal("DecodeOptions should succeed on empty Options")
	}
	if enum != nil || tags != nil {
		t.Errorf("both should be nil: enum=%v tags=%v", enum, tags)
	}
}

// TestPreset_DecodeOptions_Malformed — невалидный JSON в options → ok=false.
func TestPreset_DecodeOptions_Malformed(t *testing.T) {
	v := PresetVar{
		Name:    "x",
		Type:    "enum",
		Options: json.RawMessage(`{"this":"is not an array"}`),
	}
	_, _, ok := v.DecodeOptions()
	if ok {
		t.Error("expected ok=false on malformed options")
	}
}

// TestPreset_RoundTrip_RuDirect — real-world preset из SPEC §«Real-world example».
// Демонстрирует все фичи одновременно: 4 vars, 3 rule_set (multi), bundled DNS с
// `select: "local"`, conditional fragments, selective dns_rule.
func TestPreset_RoundTrip_RuDirect(t *testing.T) {
	raw := []byte(`{
		"id": "ru-direct",
		"label": "Russian domains & IPs",
		"description": "Route Russian TLDs (.ru/.su/IDN), service CDNs and Russian IP ranges directly.",
		"default_enabled": true,
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out", "title": "Outbound"},
			{"name": "use_dns_override", "type": "bool", "default": "true",
			 "title": "Use Yandex DNS for these domains"},
			{"name": "dns_server", "type": "dns_server", "default": "yandex_udp",
			 "if": ["use_dns_override"], "select": "local", "title": "DNS server"},
			{"name": "dns_ip", "type": "enum", "default": "77.88.8.8",
			 "if": ["use_dns_override"], "options": [
				{"title": "77.88.8.8 · Base", "value": "77.88.8.8"},
				{"title": "77.88.8.88 · Safe", "value": "77.88.8.88"},
				{"title": "77.88.8.7 · Family", "value": "77.88.8.7"}
			 ]},
			{"name": "geoip_enabled", "type": "bool", "default": "true",
			 "title": "GeoIP IP-range fallback"}
		],
		"rule_set": [
			{"tag": "ru-domains", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["ru","su","xn--p1ai"]}]},
			{"tag": "ru-services", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["yandex.com","vk.com","sberbank.com"]}]},
			{"tag": "geoip-ru", "type": "remote", "format": "binary",
			 "url": "https://example.com/geoip-ru.srs",
			 "if": ["geoip_enabled"]}
		],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "@dns_ip",
			 "server_port": 53, "detour": "@out",
			 "title": "Yandex UDP", "description": "Yandex public DNS over UDP.",
			 "if": ["use_dns_override"]},
			{"tag": "yandex_doh", "type": "https", "server": "77.88.8.88",
			 "server_port": 443, "path": "/dns-query",
			 "tls": {"enabled": true, "server_name": "safe.dot.dns.yandex.net"},
			 "detour": "@out",
			 "title": "Yandex DoH", "description": "Yandex Safe DoH.",
			 "if": ["use_dns_override"]}
		],
		"rule":     {"rule_set": ["ru-domains","ru-services","geoip-ru"], "outbound": "@out"},
		"dns_rule": {"rule_set": ["ru-domains","ru-services"], "server": "@dns_server",
		             "if": ["use_dns_override"]}
	}`)
	var p Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.ID != "ru-direct" {
		t.Errorf("id mismatch: %q", p.ID)
	}
	if len(p.Vars) != 5 {
		t.Errorf("expected 5 vars, got %d", len(p.Vars))
	}
	if len(p.RuleSet) != 3 {
		t.Errorf("expected 3 rule_set, got %d", len(p.RuleSet))
	}
	if len(p.DNSServers) != 2 {
		t.Errorf("expected 2 dns_servers, got %d", len(p.DNSServers))
	}

	// Select на dns_server var
	dnsServerVar := p.Vars[2]
	if dnsServerVar.Select != "local" {
		t.Errorf("dns_server var.select mismatch: %q", dnsServerVar.Select)
	}

	// geoip-ru имеет if
	geoipRS := p.RuleSet[2]
	if geoipRS.Tag != "geoip-ru" || len(geoipRS.If) != 1 || geoipRS.If[0] != "geoip_enabled" {
		t.Errorf("geoip-ru rule_set mismatch: %+v", geoipRS)
	}

	// dns_servers имеют title + description раздельно
	yandexUDP := p.DNSServers[0]
	if yandexUDP.Title != "Yandex UDP" || yandexUDP.Description != "Yandex public DNS over UDP." {
		t.Errorf("title/description split mismatch: %+v", yandexUDP)
	}

	// Round-trip: marshal обратно и проверить что critical-поля сохранились
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outStr := string(out)
	for _, mustContain := range []string{
		`"id":"ru-direct"`,
		`"select":"local"`,
		`"title":"Yandex UDP"`,
		`"description":"Yandex public DNS over UDP."`,
		`"if":["geoip_enabled"]`,
	} {
		if !strings.Contains(outStr, mustContain) {
			t.Errorf("round-trip lost: %q not in output", mustContain)
		}
	}
}

// TestPreset_OmitEmpty — пустые поля не должны попадать в JSON.
func TestPreset_OmitEmpty(t *testing.T) {
	p := Preset{
		ID:    "minimal",
		Label: "Minimal preset",
		Rule:  map[string]interface{}{"outbound": "direct-out"},
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outStr := string(out)
	for _, mustNotContain := range []string{
		`"description"`, `"default_enabled"`, `"vars"`, `"rule_set"`, `"dns_servers"`, `"dns_rule"`,
	} {
		if strings.Contains(outStr, mustNotContain) {
			t.Errorf("expected omit on empty: %q present in %s", mustNotContain, outStr)
		}
	}
}
