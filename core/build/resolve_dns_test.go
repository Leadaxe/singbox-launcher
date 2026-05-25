package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// makeTestTD — TemplateData с минимальным config.dns + dns_options + presets.
func makeTestTD(t *testing.T, presetsJSON string) *template.TemplateData {
	t.Helper()
	td := &template.TemplateData{
		Config: map[string]json.RawMessage{
			"dns": json.RawMessage(`{"servers": []}`),
		},
		DNSOptionsRaw: json.RawMessage(`{
			"servers": [
				{"tag":"local_dns_resolver","type":"local","required":true,"enabled":true},
				{"tag":"direct_dns_resolver","type":"udp","server":"8.8.8.8","required":true,"enabled":true},
				{"tag":"cloudflare_udp","type":"udp","server":"1.1.1.1","enabled":true},
				{"tag":"google_doh","type":"https","server":"dns.google","enabled":false}
			]
		}`),
	}
	if presetsJSON != "" {
		if err := json.Unmarshal([]byte(presetsJSON), &td.Presets); err != nil {
			t.Fatalf("parse presets: %v", err)
		}
	}
	return td
}

// TestResolveDNS_Required — required=true entries из dns_options → Locked + Source=template.
func TestResolveDNS_Required(t *testing.T) {
	td := makeTestTD(t, "")
	state := &state.State{}
	got := ResolveDNS(state, td, nil)
	if len(got.Servers) < 2 {
		t.Fatalf("expected >=2 required servers, got %d", len(got.Servers))
	}
	required := findServer(got.Servers, "local_dns_resolver")
	if required == nil {
		t.Fatal("local_dns_resolver required entry missing")
	}
	if required.Source != DNSSourceTemplate || !required.Locked || !required.Enabled || !required.Active {
		t.Errorf("required entry flags wrong: %+v", required)
	}
}

// TestResolveDNS_TemplateOverride — state.DNS.Servers[kind=template] override default_enabled.
func TestResolveDNS_TemplateOverride(t *testing.T) {
	td := makeTestTD(t, "")
	state := &state.State{
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindTemplate, Tag: "google_doh", Enabled: true}, // template default false
			},
		},
	}
	got := ResolveDNS(state, td, nil)
	google := findServer(got.Servers, "google_doh")
	if google == nil {
		t.Fatal("google_doh missing")
	}
	if google.Source != DNSSourceTemplate || !google.Enabled {
		t.Errorf("expected enabled override: %+v", google)
	}
	// cloudflare_udp — нет в state, должен взять template default (true)
	cf := findServer(got.Servers, "cloudflare_udp")
	if cf == nil || !cf.Enabled {
		t.Errorf("cloudflare_udp should default to enabled=true: %+v", cf)
	}
}

// TestResolveDNS_Preset — preset bundled servers все попадают (no consumption filter).
func TestResolveDNS_Preset(t *testing.T) {
	presetsJSON := `[{
		"id": "russian",
		"label": "Russian domains",
		"vars": [
			{"name": "use_dns_override", "type": "bool", "default": "true"}
		],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "77.88.8.8", "if": ["use_dns_override"]},
			{"tag": "yandex_doh", "type": "https", "server": "common.dot.dns.yandex.net", "if": ["use_dns_override"]},
			{"tag": "yandex_dot", "type": "tls", "server": "common.dot.dns.yandex.net", "if": ["use_dns_override"]}
		]
	}]`
	td := makeTestTD(t, presetsJSON)
	state := &state.State{
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	got := ResolveDNS(state, td, nil)

	// Ожидаем 3 preset entry (no consumption filter — все bundled).
	presetEntries := 0
	for _, s := range got.Servers {
		if s.Source == DNSSourcePreset {
			presetEntries++
			if !s.Active {
				t.Errorf("preset entry should be Active (use_dns_override=true default): %+v", s)
			}
			if !s.Enabled {
				t.Errorf("preset entry should default Enabled=true: %+v", s)
			}
			if s.PresetID != "russian" || s.PresetLabel != "Russian domains" {
				t.Errorf("preset metadata: %+v", s)
			}
		}
	}
	if presetEntries != 3 {
		t.Errorf("expected 3 preset DNS server entries, got %d", presetEntries)
	}
}

// TestResolveDNS_PresetInactiveByIf — если var у preset'а false → Active=false + InactiveReason.
func TestResolveDNS_PresetInactiveByIf(t *testing.T) {
	presetsJSON := `[{
		"id": "russian",
		"label": "Russian",
		"vars": [{"name": "use_dns_override", "type": "bool", "default": "false"}],
		"dns_servers": [
			{"tag": "yandex_doh", "type": "https", "server": "x", "if": ["use_dns_override"]}
		]
	}]`
	td := makeTestTD(t, presetsJSON)
	state := &state.State{
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	got := ResolveDNS(state, td, nil)
	yandex := findServer(got.Servers, "russian:yandex_doh")
	if yandex == nil {
		t.Fatal("preset entry should be in result regardless of Active state")
	}
	if yandex.Active {
		t.Errorf("expected Active=false (use_dns_override=false): %+v", yandex)
	}
	if yandex.InactiveReason == "" {
		t.Errorf("InactiveReason should explain why: %+v", yandex)
	}
}

// TestResolveDNS_PresetUserToggle — state.DNS.Servers[kind=preset].Enabled override.
func TestResolveDNS_PresetUserToggle(t *testing.T) {
	presetsJSON := `[{
		"id": "russian", "label": "Russian",
		"dns_servers": [{"tag": "yandex_doh", "type": "https", "server": "x"}]
	}]`
	td := makeTestTD(t, presetsJSON)
	state := &state.State{
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindPreset, Ref: "russian:yandex_doh", Enabled: false},
			},
		},
	}
	got := ResolveDNS(state, td, nil)
	y := findServer(got.Servers, "russian:yandex_doh")
	if y == nil {
		t.Fatal("missing preset entry")
	}
	if y.Enabled {
		t.Errorf("user toggle Enabled=false should be preserved: %+v", y)
	}
	if !y.Active {
		t.Errorf("should still be Active (no if-block): %+v", y)
	}
}

// TestResolveDNS_User — user-defined server в state.DNS.Servers[kind=user].
func TestResolveDNS_User(t *testing.T) {
	td := makeTestTD(t, "")
	state := &state.State{
		DNS: state.DNSOptions{
			Servers: []state.DNSServer{
				{Kind: state.DNSServerKindUser, Tag: "my-pihole", Enabled: true, Body: map[string]interface{}{
					"tag": "my-pihole", "type": "udp", "server": "192.168.1.5",
				}},
			},
		},
	}
	got := ResolveDNS(state, td, nil)
	pi := findServer(got.Servers, "my-pihole")
	if pi == nil {
		t.Fatal("user entry missing")
	}
	if pi.Source != DNSSourceUser || !pi.Active || !pi.Enabled || pi.Locked {
		t.Errorf("user flags wrong: %+v", pi)
	}
	if pi.Body["server"] != "192.168.1.5" {
		t.Errorf("user body lost: %+v", pi.Body)
	}
}

// TestResolveDNS_NoConsumptionFilter — все 3 yandex_* в результате
// даже если @dns_server picks один (regression тест на SPEC 056-R-N).
func TestResolveDNS_NoConsumptionFilter(t *testing.T) {
	presetsJSON := `[{
		"id": "russian", "label": "Russian",
		"vars": [{"name": "dns_server", "type": "dns_server", "default": "yandex_doh"}],
		"dns_servers": [
			{"tag": "yandex_udp", "type": "udp", "server": "x"},
			{"tag": "yandex_doh", "type": "https", "server": "x"},
			{"tag": "yandex_dot", "type": "tls", "server": "x"}
		],
		"dns_rule": {"rule_set": "ru-domains", "server": "@dns_server"}
	}]`
	td := makeTestTD(t, presetsJSON)
	state := &state.State{
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	got := ResolveDNS(state, td, nil)
	tags := []string{}
	for _, s := range got.Servers {
		if s.Source == DNSSourcePreset {
			tags = append(tags, s.LocalTag)
		}
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 preset DNS servers (yandex_udp/doh/dot), got %d: %v", len(tags), tags)
	}
}

// findServer — helper для поиска по Tag или LocalTag.
func findServer(servers []ResolvedDNSServer, tag string) *ResolvedDNSServer {
	for i := range servers {
		if servers[i].Tag == tag || servers[i].LocalTag == tag {
			return &servers[i]
		}
	}
	return nil
}
