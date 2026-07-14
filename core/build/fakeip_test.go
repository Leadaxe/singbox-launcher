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
		"dns_rules": [
			{"query_type": ["HTTPS","SVCB"], "action": "predefined", "rcode": "NOERROR", "if": ["@force"]},
			{"query_type": ["A","AAAA"], "server": "@dns_server"}
		]
	}]`
	presetsJSON = strings.Replace(presetsJSON,
		`{"name": "inet6_range", "type": "text", "default": "fc00::/18"}`,
		`{"name": "inet6_range", "type": "text", "default": "fc00::/18"},`+"\n\t\t\t"+`{"name": "force", "type": "bool", "default": "true"}`, 1)
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

	// Two preset DNS rules in order (SPEC 085.1): HTTPS/SVCB predefined block
	// first, then A/AAAA → fakeip:fakeip.
	var preset []ResolvedDNSRule
	for _, r := range got.Rules {
		if r.Source == DNSSourcePreset {
			preset = append(preset, r)
		}
	}
	if len(preset) != 2 {
		t.Fatalf("expected 2 preset DNS rules, got %d: %+v", len(preset), preset)
	}
	// rule 0: predefined block, no server
	if preset[0].Body["action"] != "predefined" || preset[0].Body["rcode"] != "NOERROR" {
		t.Errorf("rule 0 should be predefined NOERROR: %+v", preset[0].Body)
	}
	if _, hasServer := preset[0].Body["server"]; hasServer {
		t.Errorf("predefined block must NOT carry a server: %+v", preset[0].Body)
	}
	// rule 1: A/AAAA → fakeip:fakeip
	if preset[1].Body["server"] != "fakeip:fakeip" {
		t.Errorf("rule 1 server = %v, want fakeip:fakeip", preset[1].Body["server"])
	}

	// force=false drops the HTTPS/SVCB block → only the A/AAAA rule remains.
	st2 := &state.State{Rules: []state.Rule{
		{Kind: state.RuleKindPreset, Ref: "fakeip", Enabled: true, Body: json.RawMessage(`{"vars":{"force":"false"}}`)},
	}}
	got2 := ResolveDNS(st2, td, nil)
	presetCount := 0
	for _, r := range got2.Rules {
		if r.Source == DNSSourcePreset {
			presetCount++
			if r.Body["action"] == "predefined" {
				t.Errorf("force=false must drop the predefined block")
			}
		}
	}
	if presetCount != 1 {
		t.Errorf("force=false should leave 1 preset rule, got %d", presetCount)
	}
}
