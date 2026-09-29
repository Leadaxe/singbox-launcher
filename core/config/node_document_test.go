package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseNodeDocument — приём документа узла (SPEC 121 §5.1): из документа
// берётся только узел, `dns`/`route`/`sections` отбрасываются и называются
// (контракт 1.1.85); два узла и лишний верхний ключ — отказ.
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
  },
  "sections": {"rules": [{"kind": "inline", "enabled": true, "body": {"outbound": "@self"}}]}
}`

	tests := []struct {
		name        string
		in          string
		wantErr     string // подстрока; "" = разбор обязан пройти
		wantDropped []string
	}{
		{
			name:        "document with dns, route and sections keeps only the node",
			in:          okDoc,
			wantDropped: []string{"dns", "route", "sections"},
		},
		{
			name:        "bare envelope drops nothing",
			in:          `{"outbounds":[{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}],"dns":{}}`,
			wantDropped: nil,
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !IsNodeDocument([]byte(tc.in)) {
				t.Fatalf("IsNodeDocument = false; the input must be recognised as a document")
			}
			body, dropped, err := ParseNodeDocument([]byte(tc.in))
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
			var ob map[string]interface{}
			if err := json.Unmarshal(body, &ob); err != nil || ob["type"] == nil {
				t.Fatalf("body = %s (%v)", body, err)
			}
			if strings.Join(dropped, ",") != strings.Join(tc.wantDropped, ",") {
				t.Fatalf("dropped = %v, want %v", dropped, tc.wantDropped)
			}
		})
	}
}

// TestParseNodeDocumentFirstNode — разбор документа для правки одного узла
// (контракт 1.1.89, PARSING_PRINCIPLES §11 п.1): первый узел, не служебный и
// не группа; остальное отброшено и названо флагом; без узла — отказ.
// Импортный ParseNodeDocument при этом по-прежнему требует ровно один узел.
func TestParseNodeDocumentFirstNode(t *testing.T) {
	const twoNodes = `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"sel","outbounds":["a","b"]},` +
		`{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080},` +
		`{"type":"socks","tag":"b","server":"1.1.1.2","server_port":1080}]}`
	body, rest, err := ParseNodeDocumentFirstNode([]byte(twoNodes))
	if err != nil {
		t.Fatalf("two nodes: %v", err)
	}
	if want := `{"type":"socks","tag":"a","server":"1.1.1.1","server_port":1080}`; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
	if !rest {
		t.Error("two nodes: the rest must be reported as not kept")
	}
	if _, _, err := ParseNodeDocument([]byte(twoNodes)); err == nil || !strings.Contains(err.Error(), "exactly one node") {
		t.Errorf("import parse must still reject several nodes, got %v", err)
	}

	// Endpoints после outbounds: служебный outbound пропускается, узел берётся
	// из endpoints.
	body, rest, err = ParseNodeDocumentFirstNode([]byte(`{"outbounds":[{"type":"block","tag":"b"}],"endpoints":[{"type":"wireguard","tag":"w"}]}`))
	if err != nil || string(body) != `{"type":"wireguard","tag":"w"}` || !rest {
		t.Errorf("endpoint node: body=%s rest=%t err=%v", body, rest, err)
	}

	// Один узел без остатка — флага нет.
	if _, rest, err := ParseNodeDocumentFirstNode([]byte(`{"outbounds":[{"type":"socks","tag":"a"}]}`)); err != nil || rest {
		t.Errorf("single node: rest=%t err=%v", rest, err)
	}

	// Только служебные и группы — отказ.
	onlyService := `{"outbounds":[{"type":"direct","tag":"d"},{"type":"dns","tag":"dns-out"},{"type":"urltest","tag":"u","outbounds":["d"]}]}`
	if _, _, err := ParseNodeDocumentFirstNode([]byte(onlyService)); err == nil {
		t.Error("service outbounds and groups only: must be rejected")
	}
}
