package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/state"
)

// TestParseNodeDocument — приём документа узла (SPEC 121 §5.1) на четырёх
// входах: годный документ, два узла, лишний верхний ключ, правило без цели.
func TestParseNodeDocument(t *testing.T) {
	const okDoc = `{
  "endpoints": [
    {"type": "wireguard", "tag": "ts-node", "address": ["10.0.0.2/32"]}
  ],
  "dns": {
    "servers": [{"type": "udp", "tag": "ts-dns", "server": "100.100.100.100", "detour": "ts-node"}],
    "rules":   [{"domain_suffix": [".ts.net"], "server": "ts-dns"}]
  },
  "route": {
    "rules": [{"ip_cidr": ["100.64.0.0/10"], "outbound": "ts-node"}]
  }
}`

	tests := []struct {
		name    string
		in      string
		wantErr string // подстрока; "" = разбор обязан пройти
		check   func(t *testing.T, body json.RawMessage, sections *state.NodeSections)
	}{
		{
			name: "document with one endpoint and its bundle",
			in:   okDoc,
			check: func(t *testing.T, body json.RawMessage, sections *state.NodeSections) {
				var ob map[string]interface{}
				if err := json.Unmarshal(body, &ob); err != nil {
					t.Fatalf("body: %v", err)
				}
				if ob["type"] != "wireguard" || ob["tag"] != "ts-node" {
					t.Fatalf("body = %v", ob)
				}
				if sections == nil {
					t.Fatal("sections are nil")
				}
				if len(sections.DNSServers) != 1 || len(sections.DNSRules) != 1 || len(sections.Rules) != 1 {
					t.Fatalf("sections = %d/%d/%d, want 1/1/1",
						len(sections.DNSServers), len(sections.DNSRules), len(sections.Rules))
				}
				// Реальный тег узла переписан в @self во ВСЕХ ссылках, а
				// «.ts.net» — не ссылка и остаётся собой.
				if got := string(sections.DNSServers[0]); !strings.Contains(got, `"detour":"@self"`) {
					t.Fatalf("dns server = %s, want detour @self", got)
				}
				if got := string(sections.Rules[0]); !strings.Contains(got, `"outbound":"@self"`) {
					t.Fatalf("route rule = %s, want outbound @self", got)
				}
				if got := string(sections.DNSRules[0]); !strings.Contains(got, `".ts.net"`) {
					t.Fatalf("dns rule = %s, want the domain suffix intact", got)
				}
			},
		},
		{
			name: "two nodes in one document",
			in: `{"outbounds":[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},` +
				`{"type":"socks","tag":"b","server":"1.1.1.2","server_port":1080}]}`,
			wantErr: "exactly one node",
		},
		{
			name: "foreign top-level key",
			in: `{"outbounds":[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}],` +
				`"inbounds":[{"type":"tun","tag":"tun-in"}]}`,
			wantErr: "inbounds",
		},
		{
			name: "route rule without a target gets @self",
			in: `{"outbounds":[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}],` +
				`"route":{"rules":[{"domain_suffix":[".example"]}]}}`,
			check: func(t *testing.T, _ json.RawMessage, sections *state.NodeSections) {
				if sections == nil || len(sections.Rules) != 1 {
					t.Fatalf("sections = %+v", sections)
				}
				if got := string(sections.Rules[0]); !strings.Contains(got, `"outbound":"@self"`) {
					t.Fatalf("route rule = %s, want outbound @self appended", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !IsNodeDocument([]byte(tc.in)) {
				t.Fatalf("IsNodeDocument = false; the input must be recognised as a document")
			}
			body, sections, err := ParseNodeDocument([]byte(tc.in))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseNodeDocument accepted the document; want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want it to name %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNodeDocument: %v", err)
			}
			tc.check(t, body, sections)
		})
	}
}
