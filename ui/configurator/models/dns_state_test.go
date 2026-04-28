package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPersistedDNSState_JSONMarshalServersRulesOnly(t *testing.T) {
	in := &PersistedDNSState{
		Servers: []json.RawMessage{json.RawMessage(`{"tag":"a","type":"udp","server":"1.1.1.1"}`)},
		Rules: []json.RawMessage{
			json.RawMessage(`{"server":"a"}`),
		},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"servers"`) || !strings.Contains(s, `"rules"`) {
		t.Fatalf("expected servers and rules in JSON, got: %s", s)
	}
	if strings.Contains(s, `"strategy"`) || strings.Contains(s, `"final"`) {
		t.Fatalf("new saves omit legacy dns_options scalars, got: %s", s)
	}
	var out PersistedDNSState
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Rules) != 1 || string(out.Rules[0]) != `{"server":"a"}` {
		t.Fatalf("Rules after unmarshal: %+v", out.Rules)
	}
}

func TestPersistedDNSState_UnmarshalLegacyScalars(t *testing.T) {
	legacy := `{"servers":[{"tag":"a","type":"udp","server":"1.1.1.1"}],"rules":[{"server":"a"}],"strategy":"prefer_ipv6","final":"a"}`
	var out PersistedDNSState
	if err := json.Unmarshal([]byte(legacy), &out); err != nil {
		t.Fatal(err)
	}
	if out.Strategy != "prefer_ipv6" || out.Final != "a" {
		t.Fatalf("legacy unmarshal: strategy=%q final=%q", out.Strategy, out.Final)
	}
}

func TestPersistedDNSState_StrategyOmitemptyWhenEmpty(t *testing.T) {
	in := &PersistedDNSState{
		Servers: []json.RawMessage{json.RawMessage(`{"tag":"a","type":"udp","server":"1.1.1.1"}`)},
		Final:   "a",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"strategy"`) {
		t.Fatalf("empty strategy should omit json key, got: %s", data)
	}
}
