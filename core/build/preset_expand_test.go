package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/template"
)

// ruDirectPreset — real-world фикстура из SPEC 053 §«Real-world example».
// Используется во всех expansion test'ах.
func ruDirectPreset(t *testing.T) *template.Preset {
	t.Helper()
	raw := []byte(`{
		"id": "ru-direct",
		"label": "Russian domains & IPs",
		"default_enabled": true,
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"},
			{"name": "use_dns_override", "type": "bool", "default": "true"},
			{"name": "dns_server", "type": "dns_server", "default": "yandex_udp",
			 "if": ["use_dns_override"], "select": "local"},
			{"name": "dns_ip", "type": "enum", "default": "77.88.8.8",
			 "if": ["use_dns_override"], "options": [
				{"title": "Base", "value": "77.88.8.8"},
				{"title": "Safe", "value": "77.88.8.88"},
				{"title": "Family", "value": "77.88.8.7"}
			 ]},
			{"name": "geoip_enabled", "type": "bool", "default": "true"}
		],
		"rule_set": [
			{"tag": "ru-domains", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["ru","su"]}]},
			{"tag": "ru-services", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["yandex.com"]}]},
			{"tag": "geoip-ru", "type": "remote", "format": "binary",
			 "url": "https://example.com/geoip-ru.srs",
			 "if": ["geoip_enabled"]}
		],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "@dns_ip",
			 "server_port": 53, "detour": "@out",
			 "if": ["use_dns_override"]},
			{"tag": "yandex_doh", "type": "https", "server": "77.88.8.88",
			 "server_port": 443, "path": "/dns-query", "detour": "@out",
			 "if": ["use_dns_override"]},
			{"tag": "yandex_dot", "type": "tls", "server": "77.88.8.88",
			 "server_port": 853, "detour": "@out",
			 "if": ["use_dns_override"]}
		],
		"rule":     {"rule_set": ["ru-domains","ru-services","geoip-ru"], "outbound": "@out"},
		"dns_rule": {"rule_set": ["ru-domains","ru-services"], "server": "@dns_server",
		             "if": ["use_dns_override"]}
	}`)
	var p template.Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal ru-direct fixture: %v", err)
	}
	return &p
}

// TestExpand_RuDirect_Default — default varsValues, всё включено.
// Ожидание: 3 rule_set (с префиксами), rule на все 3, dns_rule с yandex_udp,
// в dns.servers только yandex_udp (yandex_doh/dot — мёртвый код), detour
// удалён т.к. @out=direct-out.
func TestExpand_RuDirect_Default(t *testing.T) {
	p := ruDirectPreset(t)
	frags, warns, ok := ExpandPreset(p, nil)
	if !ok {
		t.Fatalf("expand failed: %v", warns)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}

	// rule_set: 3 элемента
	if len(frags.RuleSets) != 3 {
		t.Fatalf("expected 3 rule_sets, got %d", len(frags.RuleSets))
	}
	expectedTags := []string{"ru-direct:ru-domains", "ru-direct:ru-services", "ru-direct:geoip-ru"}
	for i, want := range expectedTags {
		if got := frags.RuleSets[i]["tag"]; got != want {
			t.Errorf("rule_set[%d].tag: got %q want %q", i, got, want)
		}
	}

	// routing rule
	if frags.RoutingRule == nil {
		t.Fatal("RoutingRule should not be nil")
	}
	if got := frags.RoutingRule["outbound"]; got != "direct-out" {
		t.Errorf("rule.outbound: got %v want direct-out", got)
	}
	ruleSetRefs, _ := frags.RoutingRule["rule_set"].([]interface{})
	if len(ruleSetRefs) != 3 {
		t.Errorf("rule.rule_set should have 3 refs, got %v", ruleSetRefs)
	}
	if ruleSetRefs[0] != "ru-direct:ru-domains" {
		t.Errorf("rule.rule_set[0] should be prefixed: %v", ruleSetRefs[0])
	}

	// dns_rule
	if frags.DNSRule == nil {
		t.Fatal("DNSRule should not be nil")
	}
	if srv := frags.DNSRule["server"]; srv != "ru-direct:yandex_udp" {
		t.Errorf("dns_rule.server: got %v want ru-direct:yandex_udp", srv)
	}
	dnsRefs, _ := frags.DNSRule["rule_set"].([]interface{})
	if len(dnsRefs) != 2 {
		t.Errorf("dns_rule.rule_set should have 2 refs (no geoip-ru), got %v", dnsRefs)
	}

	// dns_servers: ВСЕ 3 yandex_* (SPEC 056-R-N follow-up: consumption-filter
	// удалён, per-server enable управляется через state.DNS.Servers).
	if len(frags.DNSServers) != 3 {
		t.Fatalf("expected 3 dns_servers (yandex_udp/doh/dot), got %d: %v", len(frags.DNSServers), frags.DNSServers)
	}
	// Все должны иметь preset-prefix tag.
	tagSet := map[string]bool{}
	for _, ds := range frags.DNSServers {
		if t, _ := ds["tag"].(string); t != "" {
			tagSet[t] = true
		}
	}
	for _, want := range []string{"ru-direct:yandex_udp", "ru-direct:yandex_doh", "ru-direct:yandex_dot"} {
		if !tagSet[want] {
			t.Errorf("missing tag: %s (got: %v)", want, tagSet)
		}
	}
	// Найдём yandex_udp для substitute проверки.
	var udp map[string]interface{}
	for _, ds := range frags.DNSServers {
		if ds["tag"] == "ru-direct:yandex_udp" {
			udp = ds
			break
		}
	}
	if udp == nil {
		t.Fatal("ru-direct:yandex_udp missing")
	}
	if udp["server"] != "77.88.8.8" {
		t.Errorf("dns_server.server should be substituted to default dns_ip 77.88.8.8: %v", udp["server"])
	}
	// detour должен быть удалён (@out=direct-out)
	if _, hasDetour := udp["detour"]; hasDetour {
		t.Errorf("detour should be stripped when @out=direct-out: %v", udp)
	}
}

// TestExpand_RuDirect_NoDNSOverride — use_dns_override=false → DNS bundle выкидывается.
func TestExpand_RuDirect_NoDNSOverride(t *testing.T) {
	p := ruDirectPreset(t)
	frags, _, ok := ExpandPreset(p, map[string]string{
		"use_dns_override": "false",
	})
	if !ok {
		t.Fatal("expand failed")
	}

	// rule_set: 3 элемента (geoip_enabled=true по умолчанию)
	if len(frags.RuleSets) != 3 {
		t.Errorf("rule_sets count: %d", len(frags.RuleSets))
	}

	// routing rule — есть
	if frags.RoutingRule == nil {
		t.Error("routing rule should be present")
	}

	// dns_rule — НЕТ (if=false)
	if frags.DNSRule != nil {
		t.Errorf("dns_rule should be nil: %v", frags.DNSRule)
	}

	// dns_servers — ВСЕ выкинуты (if=false)
	if len(frags.DNSServers) != 0 {
		t.Errorf("dns_servers should be empty: %v", frags.DNSServers)
	}
}

// TestExpand_RuDirect_NoGeoip — geoip_enabled=false → rule_set[2] выкинут,
// ссылка в rule.rule_set автоматически вычищена.
func TestExpand_RuDirect_NoGeoip(t *testing.T) {
	p := ruDirectPreset(t)
	frags, _, ok := ExpandPreset(p, map[string]string{
		"geoip_enabled": "false",
	})
	if !ok {
		t.Fatal("expand failed")
	}

	// rule_set: только 2 элемента (geoip-ru выкинут)
	if len(frags.RuleSets) != 2 {
		t.Errorf("expected 2 rule_sets, got %d", len(frags.RuleSets))
	}
	for _, rs := range frags.RuleSets {
		if rs["tag"] == "ru-direct:geoip-ru" {
			t.Error("geoip-ru should not be emitted")
		}
	}

	// rule.rule_set: только 2 ссылки (без geoip-ru)
	refs, _ := frags.RoutingRule["rule_set"].([]interface{})
	if len(refs) != 2 {
		t.Errorf("rule.rule_set should have 2 refs after dangling clean, got %v", refs)
	}
	for _, r := range refs {
		if r == "ru-direct:geoip-ru" {
			t.Error("dangling ref to geoip-ru should be cleaned")
		}
	}
}

// TestExpand_RuDirect_DifferentDNSServer — юзер выбрал yandex_doh для dns_rule.server.
// SPEC 056-R-N: все 3 yandex_* в frags.DNSServers; @dns_server влияет только на dns_rule.server.
func TestExpand_RuDirect_DifferentDNSServer(t *testing.T) {
	p := ruDirectPreset(t)
	frags, _, ok := ExpandPreset(p, map[string]string{
		"dns_server": "yandex_doh",
	})
	if !ok {
		t.Fatal("expand failed")
	}

	if len(frags.DNSServers) != 3 {
		t.Fatalf("expected 3 dns_servers (all bundled), got %d", len(frags.DNSServers))
	}
	// dns_rule.server подставился из @dns_server
	if srv := frags.DNSRule["server"]; srv != "ru-direct:yandex_doh" {
		t.Errorf("dns_rule.server: %v", srv)
	}
}

// TestExpand_RuDirect_OutboundOverride — юзер сменил outbound на proxy-out.
// Detour в dns_server теперь != direct-out → ключ должен остаться.
func TestExpand_RuDirect_OutboundOverride(t *testing.T) {
	p := ruDirectPreset(t)
	frags, _, ok := ExpandPreset(p, map[string]string{
		"out": "proxy-out",
	})
	if !ok {
		t.Fatal("expand failed")
	}
	if frags.RoutingRule["outbound"] != "proxy-out" {
		t.Errorf("rule.outbound: %v", frags.RoutingRule["outbound"])
	}
	// detour в yandex_udp = "proxy-out" → ключ ОСТАЁТСЯ (SPEC 056-R-N: все 3 yandex_* в frags)
	if len(frags.DNSServers) != 3 {
		t.Fatalf("expected 3 dns_servers, got %d", len(frags.DNSServers))
	}
	var udp map[string]interface{}
	for _, ds := range frags.DNSServers {
		if ds["tag"] == "ru-direct:yandex_udp" {
			udp = ds
			break
		}
	}
	if udp == nil {
		t.Fatal("yandex_udp missing")
	}
	if det := udp["detour"]; det != "proxy-out" {
		t.Errorf("detour should be 'proxy-out', not stripped: %v", det)
	}
}

// TestExpand_BlockAds_Reject — preset с default outbound="reject".
// Ожидание: rule.action=reject, outbound удалён.
func TestExpand_BlockAds_Reject(t *testing.T) {
	raw := []byte(`{
		"id": "block-ads",
		"label": "Block ads",
		"vars": [
			{"name": "out", "type": "outbound", "default": "reject"}
		],
		"rule_set": [
			{"tag": "ads", "type": "remote", "format": "binary", "url": "https://x/ads.srs"}
		],
		"rule": {"rule_set": "ads", "outbound": "@out"}
	}`)
	var p template.Preset
	_ = json.Unmarshal(raw, &p)

	frags, _, ok := ExpandPreset(&p, nil)
	if !ok {
		t.Fatal("expand failed")
	}
	if frags.RoutingRule["action"] != "reject" {
		t.Errorf("rule.action: %v", frags.RoutingRule["action"])
	}
	if _, has := frags.RoutingRule["outbound"]; has {
		t.Errorf("rule.outbound should be removed for reject: %v", frags.RoutingRule)
	}
	if frags.RoutingRule["rule_set"] != "ru-direct:ads" && frags.RoutingRule["rule_set"] != "block-ads:ads" {
		t.Errorf("rule.rule_set should be prefixed: %v", frags.RoutingRule["rule_set"])
	}
}

// TestExpand_PrivateIPs_NoRuleSet — preset Form 1 (inline match без rule_set).
func TestExpand_PrivateIPs_NoRuleSet(t *testing.T) {
	raw := []byte(`{
		"id": "private-ips-direct",
		"label": "Private IPs direct",
		"vars": [
			{"name": "out", "type": "outbound", "default": "direct-out"}
		],
		"rule": {"ip_is_private": true, "outbound": "@out"}
	}`)
	var p template.Preset
	_ = json.Unmarshal(raw, &p)

	frags, _, ok := ExpandPreset(&p, nil)
	if !ok {
		t.Fatal("expand failed")
	}
	if len(frags.RuleSets) != 0 {
		t.Errorf("expected no rule_sets: %v", frags.RuleSets)
	}
	if frags.RoutingRule == nil {
		t.Fatal("rule should be present")
	}
	if frags.RoutingRule["ip_is_private"] != true {
		t.Errorf("ip_is_private should be preserved: %v", frags.RoutingRule)
	}
	if frags.RoutingRule["outbound"] != "direct-out" {
		t.Errorf("outbound: %v", frags.RoutingRule["outbound"])
	}
}

// TestExpand_UnresolvedVar — @unknown_var → unresolved warning, preset skip.
func TestExpand_UnresolvedVar(t *testing.T) {
	raw := []byte(`{
		"id": "broken",
		"label": "X",
		"vars": [{"name": "x", "type": "text", "default": "y"}],
		"rule": {"outbound": "@nonexistent"}
	}`)
	var p template.Preset
	_ = json.Unmarshal(raw, &p)

	_, warns, ok := ExpandPreset(&p, nil)
	if ok {
		t.Error("expand should fail with unresolved var")
	}
	if len(warns) == 0 {
		t.Error("expected warning")
	}
	hasUnresolved := false
	for _, w := range warns {
		if strings.Contains(w.Message, "unresolved") {
			hasUnresolved = true
		}
	}
	if !hasUnresolved {
		t.Errorf("expected unresolved warning: %v", warns)
	}
}

// TestExpand_UserVarsOverride — userVars перебивают template default.
func TestExpand_UserVarsOverride(t *testing.T) {
	p := ruDirectPreset(t)
	frags, _, ok := ExpandPreset(p, map[string]string{"dns_ip": "77.88.8.7"})
	if !ok {
		t.Fatal("expand failed")
	}
	if len(frags.DNSServers) != 3 {
		t.Fatalf("expected 3 dns_servers, got %d", len(frags.DNSServers))
	}
	// yandex_udp.server = @dns_ip — должен подставиться в user value.
	var udp map[string]interface{}
	for _, ds := range frags.DNSServers {
		if ds["tag"] == "ru-direct:yandex_udp" {
			udp = ds
			break
		}
	}
	if udp == nil {
		t.Fatal("yandex_udp missing")
	}
	if udp["server"] != "77.88.8.7" {
		t.Errorf("user override didn't apply: %v", udp["server"])
	}
}

// TestExpand_IfOr_FragmentDropped — `if_or` со всеми false → фрагмент drop.
func TestExpand_IfOr_FragmentDropped(t *testing.T) {
	raw := []byte(`{
		"id": "x",
		"label": "X",
		"vars": [
			{"name": "a", "type": "bool", "default": "false"},
			{"name": "b", "type": "bool", "default": "false"}
		],
		"rule_set": [
			{"tag": "main", "type": "inline", "rules": [{"domain_suffix": ["x"]}],
			 "if_or": ["a", "b"]}
		],
		"rule": {"rule_set": "main", "outbound": "direct-out"}
	}`)
	var p template.Preset
	_ = json.Unmarshal(raw, &p)

	frags, _, ok := ExpandPreset(&p, nil)
	if !ok {
		t.Fatal("expand failed")
	}
	if len(frags.RuleSets) != 0 {
		t.Errorf("rule_set should be dropped (if_or both false): %v", frags.RuleSets)
	}
	// rule остаётся? rule_set ссылка стерта, остаётся только outbound → rule пустой → dropped
	if frags.RoutingRule != nil {
		t.Errorf("rule should be dropped (no valid rule_set ref + no other match): %v", frags.RoutingRule)
	}
}

// TestExpand_NilPreset — guard.
func TestExpand_NilPreset(t *testing.T) {
	_, _, ok := ExpandPreset(nil, nil)
	if ok {
		t.Error("nil preset should return ok=false")
	}
}

// TestEvalIf_AllCombinations — sanity на evalIf.
func TestEvalIf_AllCombinations(t *testing.T) {
	vars := map[string]string{"a": "true", "b": "false", "c": "true"}

	cases := []struct {
		name   string
		ifL    []string
		ifOrL  []string
		want   bool
	}{
		{"empty empty", nil, nil, true},
		{"if a true", []string{"a"}, nil, true},
		{"if b false", []string{"b"}, nil, false},
		{"if a+c true", []string{"a", "c"}, nil, true},
		{"if a+b false", []string{"a", "b"}, nil, false},
		{"if_or b false only", nil, []string{"b"}, false},
		{"if_or b+c true", nil, []string{"b", "c"}, true},
		{"if a + if_or b+c true", []string{"a"}, []string{"b", "c"}, true},
		{"if b + if_or anything false", []string{"b"}, []string{"a", "c"}, false},
		{"if a + if_or all false", []string{"a"}, []string{"b"}, false},
	}
	for _, tc := range cases {
		if got := evalIf(tc.ifL, tc.ifOrL, vars); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
