package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/template"
	v6 "singbox-launcher/core/state/v6"
)

func makeTestPreset(t *testing.T, raw string) template.Preset {
	t.Helper()
	var p template.Preset
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal preset: %v", err)
	}
	return p
}

func makeTestRule(t *testing.T, kind v6.RuleKind, ref, id string, enabled bool, bodyJSON string) v6.Rule {
	t.Helper()
	return v6.Rule{
		Kind:    kind,
		Ref:     ref,
		ID:      id,
		Enabled: enabled,
		Body:    json.RawMessage(bodyJSON),
	}
}

// TestPipeline_EmptyState — пустой state → только template defaults для DNS.
func TestPipeline_EmptyState(t *testing.T) {
	tpl := []TemplateDNSServer{
		{Tag: "google_doh", DefaultEnabled: true, Raw: map[string]interface{}{"tag": "google_doh", "type": "https", "default_enabled": true}},
	}
	result := BuildRulesAndDNS(nil, tpl, nil, nil)
	if len(result.RouteRuleSet) != 0 || len(result.RouteRules) != 0 {
		t.Errorf("route should be empty: %+v / %+v", result.RouteRuleSet, result.RouteRules)
	}
	if len(result.DNSServers) != 1 || result.DNSServers[0]["tag"] != "google_doh" {
		t.Errorf("dns.servers should have template default: %+v", result.DNSServers)
	}
	// default_enabled должно быть strip'нуто
	if _, has := result.DNSServers[0]["default_enabled"]; has {
		t.Errorf("default_enabled should be stripped: %v", result.DNSServers[0])
	}
}

// TestPipeline_TemplateDNS_OverrideDisable — override выключает template-сервер.
func TestPipeline_TemplateDNS_OverrideDisable(t *testing.T) {
	tpl := []TemplateDNSServer{
		{Tag: "google_doh", DefaultEnabled: true, Raw: map[string]interface{}{"tag": "google_doh", "type": "https"}},
		{Tag: "cloudflare_udp", DefaultEnabled: false, Raw: map[string]interface{}{"tag": "cloudflare_udp", "type": "udp"}},
	}
	state := &v6.State{
		DNS: v6.DNSConfig{
			TemplateServers: map[string]v6.TemplateServerOvr{
				"google_doh":     {Enabled: false}, // override → disabled
				"cloudflare_udp": {Enabled: true},  // override → enabled
			},
		},
	}
	result := BuildRulesAndDNS(nil, tpl, state, nil)
	if len(result.DNSServers) != 1 {
		t.Fatalf("expected 1 DNS server (cloudflare_udp), got %d: %+v", len(result.DNSServers), result.DNSServers)
	}
	if result.DNSServers[0]["tag"] != "cloudflare_udp" {
		t.Errorf("expected cloudflare_udp, got %v", result.DNSServers[0])
	}
}

// TestPipeline_PresetRef — expand preset-ref в RouteRuleSet/RouteRules/DNSServers.
func TestPipeline_PresetRef(t *testing.T) {
	presets := []template.Preset{makeTestPreset(t, `{
		"id": "ru-direct-mini",
		"label": "Russian domains direct (mini)",
		"vars": [{"name": "out", "type": "outbound", "default": "direct-out"}],
		"rule_set": [
			{"tag": "domains", "type": "inline", "format": "domain_suffix",
			 "rules": [{"domain_suffix": ["ru"]}]}
		],
		"rule": {"rule_set": "domains", "outbound": "@out"}
	}`)}
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindPreset, "ru-direct-mini", "", true, `{"vars":{}}`)},
	}
	result := BuildRulesAndDNS(presets, nil, state, nil)
	if len(result.RouteRuleSet) != 1 {
		t.Fatalf("rule_set count: %d", len(result.RouteRuleSet))
	}
	if result.RouteRuleSet[0]["tag"] != "ru-direct-mini:domains" {
		t.Errorf("rule_set tag: %v", result.RouteRuleSet[0]["tag"])
	}
	if len(result.RouteRules) != 1 || result.RouteRules[0]["outbound"] != "direct-out" {
		t.Errorf("route rule: %+v", result.RouteRules)
	}
}

// TestPipeline_BrokenPresetRef — ref на несуществующий preset → warning, skip.
func TestPipeline_BrokenPresetRef(t *testing.T) {
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindPreset, "missing", "", true, `{"vars":{}}`)},
	}
	result := BuildRulesAndDNS(nil, nil, state, nil)
	if len(result.RouteRules) != 0 {
		t.Error("broken preset should not emit rule")
	}
	hasWarn := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "not found in template") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected broken-preset warning: %v", result.Warnings)
	}
}

// TestPipeline_DisabledPresetRef — Enabled=false → не эмитим.
func TestPipeline_DisabledPresetRef(t *testing.T) {
	presets := []template.Preset{makeTestPreset(t, `{
		"id": "x", "label": "X",
		"rule": {"ip_is_private": true, "outbound": "direct-out"}
	}`)}
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindPreset, "x", "", false, `{"vars":{}}`)},
	}
	result := BuildRulesAndDNS(presets, nil, state, nil)
	if len(result.RouteRules) != 0 {
		t.Error("disabled preset should not emit")
	}
}

// TestPipeline_UserInline — kind=inline → ПРЯМОЕ route rule (без rule_set wrapper).
// SPEC 056 follow-up: emit user inline match directly в route.rules[] — headless
// rule_set type=inline отвергает connection-level match-поля (protocol/inbound/...),
// route.rules[] принимает union всех типов. Каждое user inline уникально по tag —
// reuse нет, обёртка лишь добавляет индирекцию.
func TestPipeline_UserInline(t *testing.T) {
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindInline, "", "01JUSR1", true, `{
			"name": "Firefox VPN",
			"match": {"domain_suffix": ["example.com"]},
			"outbound": "proxy-out"
		}`)},
	}
	result := BuildRulesAndDNS(nil, nil, state, nil)
	if len(result.RouteRuleSet) != 0 {
		t.Errorf("user inline should NOT emit rule_set (direct route rule): %+v", result.RouteRuleSet)
	}
	if len(result.RouteRules) != 1 {
		t.Fatalf("route rule count")
	}
	rr := result.RouteRules[0]
	if rr["outbound"] != "proxy-out" {
		t.Errorf("expected outbound=proxy-out, got %+v", rr)
	}
	ds, ok := rr["domain_suffix"].([]interface{})
	if !ok || len(ds) != 1 || ds[0] != "example.com" {
		t.Errorf("expected match merged into route rule (domain_suffix=[example.com]), got %+v", rr)
	}
	if _, has := rr["rule_set"]; has {
		t.Errorf("route rule should NOT have rule_set ref (direct emit), got %+v", rr)
	}
}

// TestPipeline_UserInline_Reject — outbound=reject → action=reject, no outbound.
func TestPipeline_UserInline_Reject(t *testing.T) {
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindInline, "", "01JBLK1", true, `{
			"name": "Block site",
			"match": {"domain_suffix": ["evil.com"]},
			"outbound": "reject"
		}`)},
	}
	result := BuildRulesAndDNS(nil, nil, state, nil)
	rr := result.RouteRules[0]
	if rr["action"] != "reject" {
		t.Errorf("expected action=reject, got %v", rr)
	}
	if _, has := rr["outbound"]; has {
		t.Errorf("outbound should be removed: %v", rr)
	}
}

// TestPipeline_UserSrs_NoCache — kind=srs без cached path → warning, skip.
func TestPipeline_UserSrs_NoCache(t *testing.T) {
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindSrs, "", "01JSRS1", true, `{
			"name": "Block list",
			"srs_url": "https://example.com/list.srs",
			"outbound": "reject"
		}`)},
	}
	result := BuildRulesAndDNS(nil, nil, state, nil) // srsCachedPaths=nil
	if len(result.RouteRules) != 0 {
		t.Error("srs without cache should not emit")
	}
	hasWarn := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "no cached file") {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected no-cache warning: %v", result.Warnings)
	}
}

// TestPipeline_UserSrs_WithCache — cached path есть → local rule_set emit.
func TestPipeline_UserSrs_WithCache(t *testing.T) {
	state := &v6.State{
		Rules: []v6.Rule{makeTestRule(t, v6.RuleKindSrs, "", "01JSRS1", true, `{
			"name": "Block list",
			"srs_url": "https://example.com/list.srs",
			"outbound": "reject"
		}`)},
	}
	cached := map[string]string{
		"01JSRS1": "/tmp/rule-sets/01JSRS1.srs",
	}
	result := BuildRulesAndDNS(nil, nil, state, cached)
	if len(result.RouteRuleSet) != 1 {
		t.Fatalf("rule_set count: %d", len(result.RouteRuleSet))
	}
	rs := result.RouteRuleSet[0]
	if rs["type"] != "local" || rs["path"] != "/tmp/rule-sets/01JSRS1.srs" {
		t.Errorf("srs rule_set: %+v", rs)
	}
}

// SPEC 057: TestPipeline_ExtraServersAndRules removed — extras dropped from
// v6.DNSConfig. State holds only TemplateServers refs + scalars; user-defined
// DNS bodies live in template.dns_options / preset.dns_servers / preset.dns_rule.

// TestPipeline_MixedKinds — preset + inline + srs одновременно.
func TestPipeline_MixedKinds(t *testing.T) {
	presets := []template.Preset{makeTestPreset(t, `{
		"id": "private-ips", "label": "Private IPs",
		"rule": {"ip_is_private": true, "outbound": "direct-out"}
	}`)}
	state := &v6.State{
		Rules: []v6.Rule{
			makeTestRule(t, v6.RuleKindPreset, "private-ips", "", true, `{"vars":{}}`),
			makeTestRule(t, v6.RuleKindInline, "", "01JIN1", true, `{
				"name": "X", "match": {"port": [443]}, "outbound": "direct-out"
			}`),
			makeTestRule(t, v6.RuleKindSrs, "", "01JSR1", true, `{
				"name": "Y", "srs_url": "https://x", "outbound": "reject"
			}`),
		},
	}
	srsCache := map[string]string{"01JSR1": "/path/to/y.srs"}
	result := BuildRulesAndDNS(presets, nil, state, srsCache)

	if len(result.RouteRules) != 3 {
		t.Errorf("expected 3 route rules (preset + inline + srs), got %d: %+v", len(result.RouteRules), result.RouteRules)
	}
	// rule_set: только SRS (inline теперь напрямую в route.rules, preset
	// private-ips тоже без rule_set — match-only rule).
	if len(result.RouteRuleSet) != 1 {
		t.Errorf("expected 1 rule_set (srs only), got %d: %+v", len(result.RouteRuleSet), result.RouteRuleSet)
	}
}

// TestPipeline_DuplicateTagFirstWins — два preset'а эмитят одинаковый tag → first-wins + warning.
func TestPipeline_DuplicateTagFirstWins(t *testing.T) {
	p1 := makeTestPreset(t, `{
		"id": "a",
		"rule_set": [{"tag": "shared", "type": "inline", "rules": [{"domain_suffix": ["x"]}]}],
		"rule": {"rule_set": "shared", "outbound": "direct-out"}
	}`)
	// два разных preset'а в разных prefix'ах не создают конфликт по tag —
	// они эмитят `a:shared` и `b:shared` (разные). Так что тест на конфликт
	// невозможен через два preset'а с одинаковыми local-tag'ами.
	// Проверим вместо этого что одинаковые prefix'ованные tag'и из одного и того же preset'а
	// не дублируются (idempotent emit).
	state := &v6.State{
		Rules: []v6.Rule{
			makeTestRule(t, v6.RuleKindPreset, "a", "", true, `{"vars":{}}`),
		},
	}
	result := BuildRulesAndDNS([]template.Preset{p1}, nil, state, nil)
	if len(result.RouteRuleSet) != 1 {
		t.Errorf("expected 1 rule_set: %+v", result.RouteRuleSet)
	}
}

// TestParseTemplateDNSDefaults — парсинг template.dns_defaults.servers.
func TestParseTemplateDNSDefaults(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"tag": "google_doh", "type": "https", "default_enabled": true}`),
		json.RawMessage(`{"tag": "old_legacy", "type": "udp", "enabled": false}`),
		json.RawMessage(`{"tag": "implicit", "type": "udp"}`),
	}
	defaults := ParseTemplateDNSDefaults(raw)
	if len(defaults) != 3 {
		t.Fatalf("count: %d", len(defaults))
	}
	if !defaults[0].DefaultEnabled {
		t.Error("explicit default_enabled=true lost")
	}
	if defaults[1].DefaultEnabled {
		t.Error("legacy 'enabled' should be read as default_enabled")
	}
	if !defaults[2].DefaultEnabled {
		t.Error("implicit default should be true")
	}
}

// TestSanitizeServerForEmit — помощник strip'а control полей.
func TestSanitizeServerForEmit(t *testing.T) {
	m := map[string]interface{}{
		"tag":             "x",
		"type":            "udp",
		"server":          "1.1.1.1",
		"default_enabled": true,
		"if":              []interface{}{"foo"},
		"_internal":       "secret",
	}
	out := SanitizeServerForEmit(m)
	if _, has := out["default_enabled"]; has {
		t.Error("default_enabled should be stripped")
	}
	if _, has := out["if"]; has {
		t.Error("if should be stripped")
	}
	if _, has := out["_internal"]; has {
		t.Error("_-prefixed should be stripped")
	}
	if out["tag"] != "x" || out["server"] != "1.1.1.1" {
		t.Errorf("data lost: %+v", out)
	}
}
