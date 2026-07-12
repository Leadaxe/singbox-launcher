package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Channels must survive a Save→Load round-trip and land under the "channels"
// disk key — NOT inside connections.outbounds (SPEC 087 invariant).
func TestChannels_RoundTripAndInvariant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := New()
	original.Channels = []Channel{
		{Tag: "vpn-1", Label: "VPN ①", Enabled: true, IncludeDirect: true, NodeFilter: "🇩🇪"},
		{
			Tag: "vpn-2", Label: "VPN ②", Enabled: true, NodeFilter: "ru", NodeFilterInvert: true,
			Auto: &ChannelAuto{URL: "https://x/y", Interval: "5m", Mode: "round_robin",
				Balancer: &ChannelBalancer{Pool: 3, PoolTolerance: 30, StickyHash: []string{"process"}}},
		},
	}
	if err := original.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Channels) != 2 {
		t.Fatalf("channels lost: got %d", len(loaded.Channels))
	}
	if loaded.Channels[1].Tag != "vpn-2" || !loaded.Channels[1].NodeFilterInvert {
		t.Errorf("channel fields lost: %+v", loaded.Channels[1])
	}
	if loaded.Channels[1].Auto == nil || loaded.Channels[1].Auto.Balancer == nil ||
		loaded.Channels[1].Auto.Balancer.Pool != 3 {
		t.Errorf("channel auto/balancer lost: %+v", loaded.Channels[1].Auto)
	}

	// Invariant: channels are NOT in connections.outbounds on disk.
	raw, _ := json.Marshal(mustReadDisk(t, path))
	var disk map[string]json.RawMessage
	_ = json.Unmarshal(raw, &disk)
	if _, ok := disk["channels"]; !ok {
		t.Error("expected top-level 'channels' key on disk")
	}
	conns, _ := json.Marshal(disk["connections"])
	if strings.Contains(string(conns), `"vpn-1"`) || strings.Contains(string(conns), `"vpn-2"`) {
		t.Errorf("channel tags must NOT appear in connections.outbounds: %s", conns)
	}
}

func mustReadDisk(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal disk: %v", err)
	}
	return m
}

func TestHealDanglingChannelRefs(t *testing.T) {
	s := New()
	s.Vars = []SettingVar{{Name: "route_final", Value: "vpn-3"}, {Name: "log_level", Value: "warn"}}
	s.Rules = []Rule{
		{Kind: RuleKindInline, Body: json.RawMessage(`{"name":"r1","match":{"domain":["a"]},"outbound":"vpn-3"}`)},
		{Kind: RuleKindInline, Body: json.RawMessage(`{"name":"r2","match":{"domain":["b"]},"outbound":"vpn-3-auto"}`)},
		{Kind: RuleKindInline, Body: json.RawMessage(`{"name":"r3","match":{"domain":["c"]},"outbound":"vpn-1"}`)},
	}

	n := HealDanglingChannelRefs(s, "vpn-3")
	if n < 3 {
		t.Errorf("expected >=3 heals (route_final + 2 rules), got %d", n)
	}
	// route_final → vpn-1
	for _, v := range s.Vars {
		if v.Name == "route_final" && v.Value != "vpn-1" {
			t.Errorf("route_final not healed: %q", v.Value)
		}
	}
	// rules referencing vpn-3 / vpn-3-auto → vpn-1; the vpn-1 rule untouched
	for _, r := range s.Rules {
		b, _ := r.DecodeBody()
		ib := b.(*InlineBody)
		if ib.Outbound != "vpn-1" {
			t.Errorf("rule %s outbound not healed: %q", ib.Name, ib.Outbound)
		}
	}
}

func TestHealDanglingChannelRefs_RequiredNoop(t *testing.T) {
	s := New()
	s.Vars = []SettingVar{{Name: "route_final", Value: "vpn-1"}}
	if n := HealDanglingChannelRefs(s, "vpn-1"); n != 0 {
		t.Errorf("healing the required vpn-1 must be a no-op, got %d", n)
	}
}
