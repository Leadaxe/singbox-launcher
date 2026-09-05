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
				if len(sections.DNSServers()) != 1 || len(sections.DNSRules()) != 1 || len(sections.Rules) != 1 {
					t.Fatalf("sections = %d/%d/%d, want 1/1/1",
						len(sections.DNSServers()), len(sections.DNSRules()), len(sections.Rules))
				}
				// Записи хранимой формы: DNS — вид user, правило — inline.
				srv := sections.DNSServers()[0]
				if srv.Kind != state.DNSServerKindUser || srv.Tag != "ts-dns" {
					t.Fatalf("dns server = %+v, want a user entry tagged ts-dns", srv)
				}
				// Реальный тег узла переписан в @self во ВСЕХ ссылках, а
				// «.ts.net» — не ссылка и остаётся собой.
				if got, _ := srv.Body["detour"].(string); got != state.SelfPlaceholder {
					t.Fatalf("dns server detour = %q, want %s", got, state.SelfPlaceholder)
				}
				rule := sections.Rules[0]
				if rule.Kind != state.RuleKindInline {
					t.Fatalf("route rule kind = %q, want inline", rule.Kind)
				}
				decoded, err := rule.DecodeBody()
				if err != nil {
					t.Fatalf("route rule body: %v", err)
				}
				if got := decoded.(*state.InlineBody).Outbound; got != state.SelfPlaceholder {
					t.Fatalf("route rule outbound = %q, want %s", got, state.SelfPlaceholder)
				}
				if got := sections.DNSRules()[0].Body["domain_suffix"]; got == nil {
					t.Fatalf("dns rule = %v, want the domain suffix intact", sections.DNSRules()[0].Body)
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
				body, err := sections.Rules[0].DecodeBody()
				if err != nil {
					t.Fatalf("route rule body: %v", err)
				}
				if got := body.(*state.InlineBody).Outbound; got != state.SelfPlaceholder {
					t.Fatalf("route rule outbound = %q, want %s to be filled in", got, state.SelfPlaceholder)
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
