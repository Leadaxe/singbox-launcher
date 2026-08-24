package subscription

import "testing"

// Round-trip формы Add server: URI, который собирает addServerForm.buildURI,
// обязан разобраться парсером и дать полный outbound. Форма строит URI ровно
// так же (url.URL + UserPassword), поэтому проверяем сами строки.
func TestAddServerFormURIRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		uri      string
		wantType string
		wantUser string
		wantPass string
		wantTLS  bool
	}{
		{"socks5 with auth", "socks5://alice:s3cret@10.0.0.1:1080#my-socks", "socks", "alice", "s3cret", false},
		{"socks5 no auth", "socks5://10.0.0.1:1080#plain", "socks", "", "", false},
		{"http plain", "proxy-http://bob:pw@proxy.example:8080#h", "http", "bob", "pw", false},
		{"https tls", "proxy-https://bob:pw@proxy.example:443#hs", "http", "bob", "pw", true},
		{"http no auth", "proxy-http://proxy.example:8080#hn", "http", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseNode(tc.uri, nil)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if node.Outbound == nil {
				t.Fatal("nil outbound")
			}
			if got, _ := node.Outbound["type"].(string); got != tc.wantType {
				t.Errorf("type: want %q got %q", tc.wantType, got)
			}
			if got, _ := node.Outbound["username"].(string); got != tc.wantUser {
				t.Errorf("username: want %q got %q", tc.wantUser, got)
			}
			if got, _ := node.Outbound["password"].(string); got != tc.wantPass {
				t.Errorf("password: want %q got %q", tc.wantPass, got)
			}
			tlsRaw, hasTLS := node.Outbound["tls"]
			if tc.wantTLS {
				if !hasTLS {
					t.Fatal("tls block missing for https scheme")
				}
				if m, ok := tlsRaw.(map[string]interface{}); ok {
					if en, _ := m["enabled"].(bool); !en {
						t.Error("tls present but not enabled")
					}
				}
			} else if hasTLS {
				if m, ok := tlsRaw.(map[string]interface{}); ok {
					if en, _ := m["enabled"].(bool); en {
						t.Error("tls enabled for plain scheme")
					}
				}
			}
		})
	}
}
