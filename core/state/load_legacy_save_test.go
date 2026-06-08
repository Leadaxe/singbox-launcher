package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadSaveV5_PreservesRulesAndDNS — regression for BUG1 (audit follow-up).
//
// A legacy v5 state loaded headlessly (no Configurator UI to re-emit from a
// model) must derive canonical v6 Rules/DNS at parse time. Otherwise the next
// Save — which serializes ONLY v6 fields (marshalDisk) — silently drops the
// user's custom rules + DNS config. Headless writers that hit this: Debug API
// PATCH, auto-save, log-level change, subscription refresh.
func TestLoadSaveV5_PreservesRulesAndDNS(t *testing.T) {
	old := diskStateV5{
		Meta: metaSectionV5{Version: 5, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		CustomRules: []CustomRule{{
			Label:            "Firefox via VPN",
			Enabled:          true,
			SelectedOutbound: "proxy-out",
			Rule: map[string]interface{}{
				"domain_suffix": []interface{}{"example.com"},
				"outbound":      "proxy-out",
			},
		}},
		DNSOptions: &LegacyDNSOptionsV5{
			Servers:  []json.RawMessage{json.RawMessage(`{"tag":"myudp","type":"udp","server":"1.2.3.4","enabled":true}`)},
			Final:    "myudp",
			Strategy: "prefer_ipv4",
		},
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Load v5 → must derive v6 Rules/DNS.
	s, err := Load(p)
	if err != nil {
		t.Fatalf("load v5: %v", err)
	}
	if len(s.Rules) != 1 {
		t.Fatalf("v5 load did not derive Rules from CustomRules: got %d", len(s.Rules))
	}
	if len(s.DNS.Servers) != 1 {
		t.Fatalf("v5 load did not derive DNS from DNSOptions: got %d servers", len(s.DNS.Servers))
	}

	// Headless Save (no Configurator), then reload as v6 — both must survive.
	if err := s.Save(p); err != nil {
		t.Fatalf("save: %v", err)
	}
	s2, err := Load(p)
	if err != nil {
		t.Fatalf("reload after save: %v", err)
	}
	if len(s2.Rules) != 1 {
		t.Fatalf("BUG1: headless Save dropped custom rules: got %d", len(s2.Rules))
	}
	if len(s2.DNS.Servers) != 1 {
		t.Fatalf("BUG1: headless Save dropped DNS servers: got %d", len(s2.DNS.Servers))
	}
}
