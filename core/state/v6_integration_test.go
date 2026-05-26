package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParseV6_MetaAndConnections — базовый v6 файл с новым dns_options shape.
func TestParseV6_MetaAndConnections(t *testing.T) {
	raw := []byte(`{
		"meta": {
			"version": 6,
			"schema": "presets_v1",
			"created_at": "2026-05-01T00:00:00Z",
			"updated_at": "2026-05-12T10:00:00Z"
		},
		"connections": {
			"sources":   [{"id": "src1", "type": "subscription", "enabled": true, "url": "https://x"}],
			"outbounds": [],
			"defaults":  {"max_nodes": 3000}
		},
		"rules": [
			{"kind": "preset", "ref": "ru-direct", "enabled": true, "body": {"vars": {}}},
			{"kind": "inline", "id": "u1", "enabled": true, "body": {
				"name": "Firefox", "match": {"domain_suffix": ["x.com"]}, "outbound": "proxy-out"
			}}
		],
		"vars": [{"name": "cert_store", "value": "mozilla"}],
		"dns_options": {
			"strategy": "prefer_ipv4",
			"final":    "google_doh",
			"servers": [
				{"kind":"template", "tag":"cloudflare_udp", "enabled":true}
			]
		}
	}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse v6: %v", err)
	}
	if s.Version != 6 {
		t.Errorf("Version: %d", s.Version)
	}
	if len(s.Rules) != 2 {
		t.Errorf("Rules count: %d", len(s.Rules))
	}
	if len(s.Connections.Sources) != 1 || s.Connections.Sources[0].ID != "src1" {
		t.Errorf("connections lost: %+v", s.Connections)
	}
	if len(s.Vars) != 1 || s.Vars[0].Name != "cert_store" {
		t.Errorf("vars lost: %+v", s.Vars)
	}
	if s.DNS.Strategy != "prefer_ipv4" || s.DNS.Final != "google_doh" {
		t.Errorf("DNS scalars: %+v", s.DNS)
	}
	if len(s.DNS.Servers) != 1 || s.DNS.Servers[0].Tag != "cloudflare_udp" || !s.DNS.Servers[0].Enabled {
		t.Errorf("dns_options.servers lost: %+v", s.DNS.Servers)
	}

	// Legacy view: inline видна в CustomRules, preset-ref скрыт
	if len(s.CustomRules) != 1 {
		t.Errorf("legacy CustomRules should contain only inline (preset-ref skipped): %+v", s.CustomRules)
	}
	if s.CustomRules[0].Label != "Firefox" {
		t.Errorf("legacy CustomRule label: %q", s.CustomRules[0].Label)
	}
	if s.CustomRules[0].SelectedOutbound != "proxy-out" {
		t.Errorf("legacy outbound: %q", s.CustomRules[0].SelectedOutbound)
	}
}

// TestParseV6_LegacyDevShapeConversion — старый дев-shape `dns` со SPEC 053
// (template_servers / extra_servers / extra_rules) конвертится в новый
// flat dns_options при parseCurrent.
func TestParseV6_LegacyDevShapeConversion(t *testing.T) {
	raw := []byte(`{
		"meta": {"version": 6, "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		"connections": {"sources": [], "outbounds": [], "defaults": {}},
		"rules": [],
		"dns": {
			"strategy": "prefer_ipv4",
			"template_servers": {"google_doh": {"enabled": true}, "cloudflare_udp": {"enabled": false}},
			"extra_servers":    [{"tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": 53}],
			"extra_rules":      [{"server": "my-pihole", "domain_suffix": ["internal.local"]}]
		}
	}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.DNS.Strategy != "prefer_ipv4" {
		t.Errorf("strategy lost: %+v", s.DNS)
	}
	if len(s.DNS.Servers) != 3 {
		t.Errorf("expected 3 servers (2 template + 1 user), got: %+v", s.DNS.Servers)
	}
	// Spot-check каждого entry: ровно один user-server с tag=my-pihole.
	foundUserPihole := false
	for _, srv := range s.DNS.Servers {
		if srv.Kind == DNSServerKindUser && srv.Tag == "my-pihole" {
			foundUserPihole = true
			if srv.Body["server"] != "192.168.1.5" {
				t.Errorf("user body lost: %+v", srv.Body)
			}
		}
	}
	if !foundUserPihole {
		t.Error("user my-pihole entry not converted")
	}
	if len(s.DNS.Rules) != 1 {
		t.Errorf("rules count: %+v", s.DNS.Rules)
	}
	if s.DNS.Rules[0].Kind != DNSRuleKindUser {
		t.Errorf("rule kind: %v", s.DNS.Rules[0].Kind)
	}
}

// TestSave_AlwaysWritesV6 — SPEC 060: single write path, всегда canonical (v6).
// Раньше TestSave_V5_WhenNoPresetRefs ожидал v5 если нет preset-ref; теперь
// независимо от содержимого пишем v6.
func TestSave_AlwaysWritesV6(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Pure inline rules — раньше шло v5, теперь должно идти v6.
	s := New()
	s.CustomRules = []CustomRule{
		{Label: "Test inline", Enabled: true, SelectedOutbound: "direct-out",
			Rule: map[string]interface{}{"ip_is_private": true}},
	}

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Meta struct {
			Version int    `json:"version"`
			Schema  string `json:"schema"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if probe.Meta.Version != 6 {
		t.Errorf("expected v6 save (single write path), got version=%d", probe.Meta.Version)
	}
	if probe.Meta.Schema != "presets_v1" {
		t.Errorf("expected schema presets_v1, got %q", probe.Meta.Schema)
	}
}

// TestSave_V6_WhenHasPresetRef — preset-ref в state → Save пишет v6.
func TestSave_V6_WhenHasPresetRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := New()
	s.Rules = []Rule{
		{
			Kind:    RuleKindPreset,
			Ref:     "ru-direct",
			Enabled: true,
			Body:    json.RawMessage(`{"vars":{}}`),
		},
	}

	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, _ := os.ReadFile(path)
	var probe struct {
		Meta struct {
			Version int    `json:"version"`
			Schema  string `json:"schema"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if probe.Meta.Version != 6 {
		t.Errorf("expected v6 save, got %d", probe.Meta.Version)
	}
	if probe.Meta.Schema != "presets_v1" {
		t.Errorf("expected schema presets_v1, got %q", probe.Meta.Schema)
	}
}

// TestRoundTrip_V6_LoadSaveLoad — Save v6 → Load → Save → identical (SPEC 056-R-N).
func TestRoundTrip_V6_LoadSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := New()
	original.Rules = []Rule{
		{Kind: RuleKindPreset, Ref: "ru-direct", Enabled: true,
			Body: json.RawMessage(`{"vars":{"dns_ip":"77.88.8.7"}}`)},
		// SPEC 063: identity для inline = sanitize(body.name). Поле Rule.ID удалено.
		{Kind: RuleKindInline, Enabled: true,
			Body: json.RawMessage(`{"name":"X","match":{"port":[443]},"outbound":"proxy-out"}`)},
	}
	original.DNS = DNSOptions{
		Strategy: "prefer_ipv4",
		Final:    "google_doh",
		Servers: []DNSServer{
			{Kind: DNSServerKindTemplate, Tag: "cloudflare_udp", Enabled: true},
			{Kind: DNSServerKindUser, Tag: "my-pihole", Enabled: true, Body: map[string]interface{}{
				"tag": "my-pihole", "type": "udp", "server": "192.168.1.5", "server_port": float64(53),
			}},
		},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("save 1: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.Rules) != 2 {
		t.Errorf("Rules round-trip: %d", len(loaded.Rules))
	}
	if loaded.Rules[0].Ref != "ru-direct" {
		t.Errorf("ref lost: %+v", loaded.Rules[0])
	}
	if loaded.DNS.Final != "google_doh" {
		t.Errorf("DNS round-trip lost: %+v", loaded.DNS)
	}
	if len(loaded.DNS.Servers) != 2 {
		t.Errorf("servers round-trip: %+v", loaded.DNS.Servers)
	}
	if loaded.DNS.Servers[0].Kind != DNSServerKindTemplate ||
		loaded.DNS.Servers[0].Tag != "cloudflare_udp" ||
		!loaded.DNS.Servers[0].Enabled {
		t.Errorf("template entry lost: %+v", loaded.DNS.Servers[0])
	}
	if loaded.DNS.Servers[1].Kind != DNSServerKindUser ||
		loaded.DNS.Servers[1].Body["server"] != "192.168.1.5" {
		t.Errorf("user entry body lost: %+v", loaded.DNS.Servers[1])
	}

	// Re-save и проверка что Version остаётся v6.
	if err := loaded.Save(path); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !isV6(raw) {
		t.Error("should remain v6 after re-save")
	}
}

// TestParseV6_LegacyInlineConversion — kind=inline в v6 → legacy CustomRule с outbound в Rule.
func TestParseV6_LegacyInlineConversion(t *testing.T) {
	raw := []byte(`{
		"meta": {"version": 6, "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		"connections": {"sources": [], "outbounds": [], "defaults": {}},
		"rules": [{"kind": "inline", "id": "u1", "enabled": true, "body": {
			"name": "X",
			"match": {"domain_suffix": ["example.com"]},
			"outbound": "proxy-out"
		}}],
		"dns": {}
	}`)
	s, _ := Parse(raw)
	if len(s.CustomRules) != 1 {
		t.Fatalf("expected 1 CustomRule, got %d", len(s.CustomRules))
	}
	cr := s.CustomRules[0]
	if cr.Label != "X" || cr.SelectedOutbound != "proxy-out" {
		t.Errorf("legacy CustomRule: %+v", cr)
	}
	if cr.Rule["outbound"] != "proxy-out" {
		t.Error("Rule.outbound should be set for build pipeline")
	}
}

// TestParseV6_LegacySRSConversion — SPEC 063 follow-up: kind=srs в v6 →
// legacy CustomRule с Type=srs И rule_set placeholder в Rule (для
// DetermineRuleType, иначе edit dialog показывает Custom JSON).
func TestParseV6_LegacySRSConversion(t *testing.T) {
	raw := []byte(`{
		"meta": {"version": 6, "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"},
		"connections": {"sources": [], "outbounds": [], "defaults": {}},
		"rules": [{"kind": "srs", "enabled": true, "body": {
			"name": "YT",
			"srs_url": "https://example.com/list.srs",
			"outbound": "direct-out"
		}}],
		"dns": {}
	}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.CustomRules) != 1 {
		t.Fatalf("expected 1 CustomRule, got %d", len(s.CustomRules))
	}
	cr := s.CustomRules[0]
	if cr.Type != RuleTypeSRS {
		t.Errorf("Type: %q want srs", cr.Type)
	}
	// rule_set ключ обязан быть в Rule — иначе DetermineRuleType вернёт raw.
	if _, has := cr.Rule["rule_set"]; !has {
		t.Errorf("Rule must contain rule_set hint for SRS detection: %+v", cr.Rule)
	}
	if len(cr.RuleSet) != 1 {
		t.Errorf("RuleSet count: %d", len(cr.RuleSet))
	}
}
