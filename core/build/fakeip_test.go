package build

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// The FakeIP preset must emit a type:"fakeip" DNS server carrying inet4_range /
// inet6_range (proving the SPEC 085 field passthrough works end to end), plus
// an A/AAAA DNS rule pointing at it. Mirrors the bundled bin/wizard_template.json
// fakeip preset shape.
func TestResolveDNS_FakeIPPreset(t *testing.T) {
	presetsJSON := `[{
		"id": "fakeip",
		"label": "FakeIP",
		"vars": [
			{"name": "dns_server", "type": "dns_server", "default": "fakeip"},
			{"name": "inet4_range", "type": "text", "default": "198.18.0.0/15"},
			{"name": "inet6_range", "type": "text", "default": "fc00::/18"}
		],
		"dns_servers": [
			{"tag": "fakeip", "type": "fakeip", "inet4_range": "@inet4_range", "inet6_range": "@inet6_range"}
		],
		"dns_rule": {"query_type": ["A","AAAA"], "server": "@dns_server"}
	}]`
	td := makeTestTD(t, presetsJSON)
	st := &state.State{
		Rules: []state.Rule{
			{Kind: state.RuleKindPreset, Ref: "fakeip", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	got := ResolveDNS(st, td, nil)

	// fakeip server with both ranges, prefixed tag fakeip:fakeip.
	var fakeip *ResolvedDNSServer
	for i := range got.Servers {
		if got.Servers[i].Source == DNSSourcePreset && got.Servers[i].LocalTag == "fakeip" {
			fakeip = &got.Servers[i]
			break
		}
	}
	if fakeip == nil {
		t.Fatalf("fakeip server not emitted; servers=%+v", got.Servers)
	}
	if fakeip.Tag != "fakeip:fakeip" {
		t.Errorf("fakeip tag = %q, want fakeip:fakeip", fakeip.Tag)
	}
	if got := fakeip.Body["type"]; got != "fakeip" {
		t.Errorf("type = %v, want fakeip", got)
	}
	if got := fakeip.Body["inet4_range"]; got != "198.18.0.0/15" {
		t.Errorf("inet4_range = %v, want 198.18.0.0/15", got)
	}
	if got := fakeip.Body["inet6_range"]; got != "fc00::/18" {
		t.Errorf("inet6_range = %v, want fc00::/18", got)
	}

	// A/AAAA rule pointing at the prefixed fakeip server.
	var found bool
	for _, r := range got.Rules {
		if r.Source != DNSSourcePreset {
			continue
		}
		srv, _ := r.Body["server"].(string)
		if srv == "fakeip:fakeip" {
			qt, _ := json.Marshal(r.Body["query_type"])
			if strings.Contains(string(qt), "A") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("A/AAAA→fakeip DNS rule not emitted; rules=%+v", got.Rules)
	}
}
