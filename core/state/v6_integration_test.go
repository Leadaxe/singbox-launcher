package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	v6 "singbox-launcher/core/state/v6"
)

// TestParseV6_MetaAndConnections — базовый v6 файл парсится корректно.
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
		"dns": {
			"strategy": "prefer_ipv4",
			"final":    "google_doh",
			"template_servers": {"cloudflare_udp": {"enabled": true}},
			"extra_servers":    [],
			"extra_rules":      []
		}
	}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse v6: %v", err)
	}
	if s.Version != 6 {
		t.Errorf("Version: %d", s.Version)
	}
	if len(s.RulesV6) != 2 {
		t.Errorf("RulesV6 count: %d", len(s.RulesV6))
	}
	if len(s.Connections.Sources) != 1 || s.Connections.Sources[0].ID != "src1" {
		t.Errorf("connections lost: %+v", s.Connections)
	}
	if len(s.Vars) != 1 || s.Vars[0].Name != "cert_store" {
		t.Errorf("vars lost: %+v", s.Vars)
	}
	// DNSV6
	if s.DNSV6.Strategy != "prefer_ipv4" || s.DNSV6.Final != "google_doh" {
		t.Errorf("DNSV6 scalars: %+v", s.DNSV6)
	}
	if !s.DNSV6.TemplateServers["cloudflare_udp"].Enabled {
		t.Error("template_servers override lost")
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

// TestParseV6_DNSLegacyView — legacy DNSOptions view содержит ТОЛЬКО
// template_servers overrides (SPEC 057: extras dropped из state schema).
// Старые state.json с extras: поля тихо игнорируются на load — Go JSON
// unmarshal не падает на unknown fields в struct.
func TestParseV6_DNSLegacyView(t *testing.T) {
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
	if s.DNSOptions == nil {
		t.Fatal("legacy DNSOptions view should be present")
	}
	// SPEC 057: legacy view содержит только override-маркеры от template_servers.
	if len(s.DNSOptions.Servers) != 2 {
		t.Errorf("legacy servers count: %d (want 2 = template overrides only, extras dropped)", len(s.DNSOptions.Servers))
	}
	if len(s.DNSOptions.Rules) != 0 {
		t.Errorf("legacy rules count: %d (want 0, extras dropped per SPEC 057)", len(s.DNSOptions.Rules))
	}
}

// TestSave_V5_WhenNoPresetRefs — без preset-ref'ов Save пишет v5.
func TestSave_V5_WhenNoPresetRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

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
			Version int `json:"version"`
		} `json:"meta"`
	}
	json.Unmarshal(raw, &probe)
	if probe.Meta.Version != 5 {
		t.Errorf("expected v5 save (no preset-refs), got version=%d", probe.Meta.Version)
	}

	// Backup НЕ должен быть создан (мы не переходим с v5 на v6).
	if _, err := os.Stat(path + ".v5.bak"); !os.IsNotExist(err) {
		t.Error("backup should NOT exist for v5 save")
	}
}

// TestSave_V6_WhenHasPresetRef — preset-ref в state → Save пишет v6.
func TestSave_V6_WhenHasPresetRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := New()
	s.RulesV6 = []v6.Rule{
		{
			Kind:    v6.RuleKindPreset,
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
	json.Unmarshal(raw, &probe)
	if probe.Meta.Version != 6 {
		t.Errorf("expected v6 save, got %d", probe.Meta.Version)
	}
	if probe.Meta.Schema != "presets_v1" {
		t.Errorf("expected schema presets_v1, got %q", probe.Meta.Schema)
	}
}

// TestSave_BackupV5OnFirstUpgrade — при первом v5→v6 upgrade создаётся backup.
func TestSave_BackupV5OnFirstUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Step 1: первый Save — v5 (без preset-ref'ов)
	s := New()
	s.CustomRules = []CustomRule{
		{Label: "Inline", Enabled: true, SelectedOutbound: "direct-out",
			Rule: map[string]interface{}{"ip_is_private": true}},
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Step 2: добавляем preset-ref → второй Save должен переключиться на v6 + создать backup.
	s.RulesV6 = []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "ru-direct", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("upgrade save: %v", err)
	}

	// Backup должен существовать.
	if _, err := os.Stat(path + ".v5.bak"); err != nil {
		t.Errorf("backup should exist after v5→v6 upgrade: %v", err)
	}

	// Главный файл — теперь v6.
	raw, _ := os.ReadFile(path)
	if !v6.IsV6(raw) {
		t.Errorf("main file should be v6 after upgrade")
	}

	// Step 3: третий Save (всё ещё с preset-ref) — backup НЕ перезаписывается (идемпотентно).
	backupBefore, _ := os.ReadFile(path + ".v5.bak")
	if err := s.Save(path); err != nil {
		t.Fatalf("third save: %v", err)
	}
	backupAfter, _ := os.ReadFile(path + ".v5.bak")
	if string(backupBefore) != string(backupAfter) {
		t.Error("backup should NOT be overwritten on subsequent saves")
	}
}

// TestRoundTrip_V6_LoadSaveLoad — Save v6 → Load → Save → identical.
func TestRoundTrip_V6_LoadSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := New()
	original.RulesV6 = []v6.Rule{
		{Kind: v6.RuleKindPreset, Ref: "ru-direct", Enabled: true,
			Body: json.RawMessage(`{"vars":{"dns_ip":"77.88.8.7"}}`)},
		{Kind: v6.RuleKindInline, ID: "u1", Enabled: true,
			Body: json.RawMessage(`{"name":"X","match":{"port":[443]},"outbound":"proxy-out"}`)},
	}
	original.DNSV6 = v6.DNSConfig{
		Strategy: "prefer_ipv4",
		Final:    "google_doh",
		TemplateServers: map[string]v6.TemplateServerOvr{
			"cloudflare_udp": {Enabled: true},
		},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("save 1: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.RulesV6) != 2 {
		t.Errorf("RulesV6 round-trip: %d", len(loaded.RulesV6))
	}
	if loaded.RulesV6[0].Ref != "ru-direct" {
		t.Errorf("ref lost: %+v", loaded.RulesV6[0])
	}
	if loaded.DNSV6.Final != "google_doh" {
		t.Errorf("DNS round-trip lost: %+v", loaded.DNSV6)
	}
	if !loaded.DNSV6.TemplateServers["cloudflare_udp"].Enabled {
		t.Error("template_servers override lost")
	}

	// Re-save и проверка что Version остаётся v6.
	if err := loaded.Save(path); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !v6.IsV6(raw) {
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
