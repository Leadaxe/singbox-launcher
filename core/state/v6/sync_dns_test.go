package v6

import (
	"encoding/json"
	"testing"
)

// fakePreset реализует PresetLite для тестов sync функции.
type fakePreset struct {
	id         string
	serverTags []string
	hasRule    bool
}

func (p *fakePreset) PresetID() string                  { return p.id }
func (p *fakePreset) PresetDNSServerTags() []string     { return p.serverTags }
func (p *fakePreset) PresetHasDNSRule() bool            { return p.hasRule }

func presetMap(presets ...*fakePreset) map[string]PresetLite {
	out := make(map[string]PresetLite, len(presets))
	for _, p := range presets {
		out[p.id] = p
	}
	return out
}

// TestSync_EnableAddsEntries — enable preset → DNS получает kind=preset entries.
func TestSync_EnableAddsEntries(t *testing.T) {
	rules := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	dns := &DNSOptions{}
	presets := presetMap(&fakePreset{
		id: "russian", serverTags: []string{"yandex_udp", "yandex_doh"}, hasRule: true,
	})

	SyncDNSOptionsWithActivePresets(rules, dns, presets)

	if len(dns.Servers) != 2 {
		t.Fatalf("expected 2 server entries, got %d: %+v", len(dns.Servers), dns.Servers)
	}
	expectedRefs := map[string]bool{"russian:yandex_udp": true, "russian:yandex_doh": true}
	for _, s := range dns.Servers {
		if s.Kind != DNSServerKindPreset {
			t.Errorf("entry should be kind=preset: %+v", s)
		}
		if !expectedRefs[s.Ref] {
			t.Errorf("unexpected ref: %s", s.Ref)
		}
		if !s.Enabled {
			t.Errorf("entry should default to Enabled=true: %+v", s)
		}
	}
	if len(dns.Rules) != 1 || dns.Rules[0].Ref != "russian" {
		t.Errorf("expected 1 rule entry ref=russian: %+v", dns.Rules)
	}
}

// TestSync_DisableRemovesEntries — disable preset → entries исчезают.
func TestSync_DisableRemovesEntries(t *testing.T) {
	rules := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: false, Body: json.RawMessage(`{"vars":{}}`)},
	}
	dns := &DNSOptions{
		Servers: []DNSServer{
			{Kind: DNSServerKindPreset, Ref: "russian:yandex_udp", Enabled: true},
			{Kind: DNSServerKindUser, Tag: "my-pihole", Enabled: true, Body: map[string]interface{}{"server": "192.168.1.5"}},
		},
		Rules: []DNSRule{
			{Kind: DNSRuleKindPreset, Ref: "russian", Enabled: true},
		},
	}
	presets := presetMap(&fakePreset{
		id: "russian", serverTags: []string{"yandex_udp"}, hasRule: true,
	})

	SyncDNSOptionsWithActivePresets(rules, dns, presets)

	if len(dns.Servers) != 1 || dns.Servers[0].Kind != DNSServerKindUser {
		t.Errorf("user entry should remain, preset entry removed: %+v", dns.Servers)
	}
	if len(dns.Rules) != 0 {
		t.Errorf("preset rule should be removed: %+v", dns.Rules)
	}
}

// TestSync_PreserveUserToggle — если preset entry уже была с Enabled=false,
// sync сохраняет этот toggle (пока preset активен).
func TestSync_PreserveUserToggle(t *testing.T) {
	rules := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	dns := &DNSOptions{
		Servers: []DNSServer{
			{Kind: DNSServerKindPreset, Ref: "russian:yandex_udp", Enabled: false}, // user выключил
		},
	}
	presets := presetMap(&fakePreset{
		id: "russian", serverTags: []string{"yandex_udp", "yandex_doh"}, hasRule: false,
	})

	SyncDNSOptionsWithActivePresets(rules, dns, presets)

	if len(dns.Servers) != 2 {
		t.Fatalf("expected 2 entries: %+v", dns.Servers)
	}
	var first, second *DNSServer
	for i := range dns.Servers {
		s := &dns.Servers[i]
		if s.Ref == "russian:yandex_udp" {
			first = s
		}
		if s.Ref == "russian:yandex_doh" {
			second = s
		}
	}
	if first == nil || first.Enabled {
		t.Errorf("user toggle Enabled=false should be preserved: %+v", first)
	}
	if second == nil || !second.Enabled {
		t.Errorf("new entry should default to Enabled=true: %+v", second)
	}
}

// TestSync_Idempotent — повторный вызов не меняет состояние.
func TestSync_Idempotent(t *testing.T) {
	rules := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	dns := &DNSOptions{}
	presets := presetMap(&fakePreset{
		id: "russian", serverTags: []string{"yandex_udp"}, hasRule: true,
	})

	SyncDNSOptionsWithActivePresets(rules, dns, presets)
	snapshot, _ := json.Marshal(dns)
	SyncDNSOptionsWithActivePresets(rules, dns, presets)
	after, _ := json.Marshal(dns)
	if string(snapshot) != string(after) {
		t.Errorf("sync should be idempotent:\nbefore: %s\nafter:  %s", snapshot, after)
	}
}

// TestSync_DroppedMissingPreset — entry с ref на preset которого нет в template
// удаляется (broken ref cleanup).
func TestSync_DroppedMissingPreset(t *testing.T) {
	rules := []Rule{} // no active presets
	dns := &DNSOptions{
		Servers: []DNSServer{
			{Kind: DNSServerKindPreset, Ref: "deleted:yandex_udp", Enabled: true},
			{Kind: DNSServerKindTemplate, Tag: "google_doh", Enabled: true},
		},
	}
	presets := map[string]PresetLite{} // empty

	SyncDNSOptionsWithActivePresets(rules, dns, presets)

	if len(dns.Servers) != 1 || dns.Servers[0].Kind != DNSServerKindTemplate {
		t.Errorf("template entry should remain, broken preset ref removed: %+v", dns.Servers)
	}
}

// TestSync_DisableEnableRoundtrip — disable → re-enable preset = default state.
func TestSync_DisableEnableRoundtrip(t *testing.T) {
	presets := presetMap(&fakePreset{
		id: "russian", serverTags: []string{"yandex_udp"}, hasRule: false,
	})
	rulesEnabled := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
	}
	rulesDisabled := []Rule{
		{Kind: RuleKindPreset, Ref: "russian", Enabled: false, Body: json.RawMessage(`{"vars":{}}`)},
	}

	dns := &DNSOptions{}
	SyncDNSOptionsWithActivePresets(rulesEnabled, dns, presets)
	// Юзер кликнул выкл на entry.
	dns.Servers[0].Enabled = false

	// Юзер disable'ит preset.
	SyncDNSOptionsWithActivePresets(rulesDisabled, dns, presets)
	if len(dns.Servers) != 0 {
		t.Errorf("disable preset → entry removed: %+v", dns.Servers)
	}

	// Юзер re-enable'ит — entry создаётся ЗАНОВО с дефолтом Enabled=true
	// (toggle юзера потерян — это by design).
	SyncDNSOptionsWithActivePresets(rulesEnabled, dns, presets)
	if len(dns.Servers) != 1 || !dns.Servers[0].Enabled {
		t.Errorf("re-enable preset → fresh entry with Enabled=true: %+v", dns.Servers)
	}
}

// TestPresetIDFromServerRef / LocalTagFromServerRef — helpers.
func TestPresetRefHelpers(t *testing.T) {
	cases := []struct {
		ref       string
		presetID  string
		localTag  string
	}{
		{"russian:yandex_udp", "russian", "yandex_udp"},
		{"ru-direct:domains", "ru-direct", "domains"},
		{"no_colon", "", ""},
		{":empty_id", "", ""},
		{"trailing:", "trailing", ""},
	}
	for _, c := range cases {
		if got := PresetIDFromServerRef(c.ref); got != c.presetID {
			t.Errorf("PresetIDFromServerRef(%q): got %q, want %q", c.ref, got, c.presetID)
		}
		if got := LocalTagFromServerRef(c.ref); got != c.localTag {
			t.Errorf("LocalTagFromServerRef(%q): got %q, want %q", c.ref, got, c.localTag)
		}
	}
}
