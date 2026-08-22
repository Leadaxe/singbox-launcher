package subscription

import (
	"testing"
)

// --- ParseNode for proxy-http(s):// / proxy+http(s):// URIs --------------
//
// Fixtures mirror the shared contract corpus (contract/corpus/uri/http/) and
// LxBox's http_proxy_test.dart, so both parsers stay behaviorally aligned.

func TestParseNode_HTTPProxy_Table(t *testing.T) {
	cases := []struct {
		name         string
		uri          string
		wantErr      bool
		wantServer   string
		wantPort     int
		wantUsername string
		wantPassword string
		wantPath     string
		wantHeaders  map[string]string
		wantTLS      bool
		wantSNI      string
		wantFP       string
		wantAlpn     []string
		wantInsecure bool
		wantLabel    string
	}{
		{
			name:       "plain, no port -> default 80",
			uri:        "proxy-http://p.example-1.com",
			wantServer: "p.example-1.com",
			wantPort:   80,
		},
		{
			name:       "plain with explicit port and label",
			uri:        "proxy-http://10.0.0.1:8080#Corp%20proxy",
			wantServer: "10.0.0.1",
			wantPort:   8080,
			wantLabel:  "Corp proxy",
		},
		{
			name:         "userinfo user only",
			uri:          "proxy-http://user1@h.example-1.com:8080#u",
			wantServer:   "h.example-1.com",
			wantPort:     8080,
			wantUsername: "user1",
			wantLabel:    "u",
		},
		{
			name:         "userinfo password-only (:pass)",
			uri:          "proxy-http://:pass123@h.example-1.com:3128#pass-only",
			wantServer:   "h.example-1.com",
			wantPort:     3128,
			wantPassword: "pass123",
			wantLabel:    "pass-only",
		},
		{
			name:         "userinfo percent-encoded decodes",
			uri:          "proxy-http://user%40dom:p%3Ass@h.example-1.com:8080#pct-userinfo",
			wantServer:   "h.example-1.com",
			wantPort:     8080,
			wantUsername: "user@dom",
			wantPassword: "p:ss",
			wantLabel:    "pct-userinfo",
		},
		{
			name:        "path and headers from query",
			uri:         "proxy-http://h.example-1.com:8080?path=%2Ftunnel&headers=X-Token%3A%20abc%0D%0AUser-Agent%3A%20curl#hdrs",
			wantServer:  "h.example-1.com",
			wantPort:    8080,
			wantPath:    "/tunnel",
			wantHeaders: map[string]string{"X-Token": "abc", "User-Agent": "curl"},
			wantLabel:   "hdrs",
		},
		{
			name:       "https, no port -> default 443, TLS on with server as sni",
			uri:        "proxy-https://p.example-1.com",
			wantServer: "p.example-1.com",
			wantPort:   443,
			wantTLS:    true,
			wantSNI:    "p.example-1.com",
		},
		{
			name:         "https full TLS: sni/fp/alpn/insecure",
			uri:          "proxy-https://h.example-1.com:8443?sni=proxy.example-corp.com&fp=chrome&alpn=h2,http/1.1&allowInsecure=1#full-tls",
			wantServer:   "h.example-1.com",
			wantPort:     8443,
			wantTLS:      true,
			wantSNI:      "proxy.example-corp.com",
			wantFP:       "chrome",
			wantAlpn:     []string{"h2", "http/1.1"},
			wantInsecure: true,
			wantLabel:    "full-tls",
		},
		{
			name:         "plus-form alias proxy+http equivalent to proxy-http",
			uri:          "proxy+http://alice:pass123@h.example-1.com:8080#plus-http",
			wantServer:   "h.example-1.com",
			wantPort:     8080,
			wantUsername: "alice",
			wantPassword: "pass123",
			wantLabel:    "plus-http",
		},
		{
			name:         "plus-form alias proxy+https equivalent to proxy-https",
			uri:          "proxy+https://alice:pass123@h.example-1.com:8443?sni=proxy.example-corp.com#plus-https",
			wantServer:   "h.example-1.com",
			wantPort:     8443,
			wantUsername: "alice",
			wantPassword: "pass123",
			wantTLS:      true,
			wantSNI:      "proxy.example-corp.com",
			wantLabel:    "plus-https",
		},
		{
			name:    "missing hostname rejected",
			uri:     "proxy-http://",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseNode(tc.uri, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseNode(%q) expected error, got none (node=%+v)", tc.uri, node)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNode(%q) unexpected error: %v", tc.uri, err)
			}
			if node == nil {
				t.Fatalf("ParseNode(%q) returned nil node with nil error", tc.uri)
			}
			if node.Scheme != "http" {
				t.Errorf("Scheme = %q, want %q", node.Scheme, "http")
			}
			if node.Server != tc.wantServer {
				t.Errorf("Server = %q, want %q", node.Server, tc.wantServer)
			}
			if node.Port != tc.wantPort {
				t.Errorf("Port = %d, want %d", node.Port, tc.wantPort)
			}
			if node.Label != tc.wantLabel {
				t.Errorf("Label = %q, want %q", node.Label, tc.wantLabel)
			}
			if node.Outbound == nil {
				t.Fatalf("Outbound is nil")
			}
			if got, _ := node.Outbound["type"].(string); got != "http" {
				t.Errorf(`Outbound["type"] = %q, want "http"`, got)
			}
			if got, _ := node.Outbound["username"].(string); got != tc.wantUsername {
				t.Errorf(`Outbound["username"] = %q, want %q`, got, tc.wantUsername)
			}
			if got, _ := node.Outbound["password"].(string); got != tc.wantPassword {
				t.Errorf(`Outbound["password"] = %q, want %q`, got, tc.wantPassword)
			}
			if got, _ := node.Outbound["path"].(string); got != tc.wantPath {
				t.Errorf(`Outbound["path"] = %q, want %q`, got, tc.wantPath)
			}
			if tc.wantHeaders != nil {
				hdrs, ok := node.Outbound["headers"].(map[string]interface{})
				if !ok {
					t.Fatalf(`Outbound["headers"] missing or wrong type: %#v`, node.Outbound["headers"])
				}
				if len(hdrs) != len(tc.wantHeaders) {
					t.Errorf("headers = %v, want %v", hdrs, tc.wantHeaders)
				}
				for k, v := range tc.wantHeaders {
					if got, _ := hdrs[k].(string); got != v {
						t.Errorf("headers[%q] = %q, want %q", k, got, v)
					}
				}
			} else if _, ok := node.Outbound["headers"]; ok {
				t.Errorf(`Outbound["headers"] present, want absent: %#v`, node.Outbound["headers"])
			}

			tlsData, hasTLS := node.Outbound["tls"].(map[string]interface{})
			if hasTLS != tc.wantTLS {
				t.Errorf("has tls block = %v, want %v (tls=%#v)", hasTLS, tc.wantTLS, tlsData)
			}
			if tc.wantTLS {
				if sni, _ := tlsData["server_name"].(string); sni != tc.wantSNI {
					t.Errorf("tls.server_name = %q, want %q", sni, tc.wantSNI)
				}
				if tc.wantFP != "" {
					utls, _ := tlsData["utls"].(map[string]interface{})
					if fp, _ := utls["fingerprint"].(string); fp != tc.wantFP {
						t.Errorf("tls.utls.fingerprint = %q, want %q", fp, tc.wantFP)
					}
				}
				if len(tc.wantAlpn) > 0 {
					alpn, _ := tlsData["alpn"].([]string)
					if len(alpn) != len(tc.wantAlpn) {
						t.Errorf("tls.alpn = %v, want %v", alpn, tc.wantAlpn)
					} else {
						for i, v := range tc.wantAlpn {
							if alpn[i] != v {
								t.Errorf("tls.alpn[%d] = %q, want %q", i, alpn[i], v)
							}
						}
					}
				}
				gotInsecure, _ := tlsData["insecure"].(bool)
				if gotInsecure != tc.wantInsecure {
					t.Errorf("tls.insecure = %v, want %v", gotInsecure, tc.wantInsecure)
				}
			}
		})
	}
}

// --- IsDirectLink recognizes all four schemes -----------------------------

func TestIsDirectLink_HTTPProxySchemes(t *testing.T) {
	uris := []string{
		"proxy-http://host.tld",
		"proxy-https://host.tld",
		"proxy+http://host.tld",
		"proxy+https://host.tld",
	}
	for _, u := range uris {
		if !IsDirectLink(u) {
			t.Errorf("IsDirectLink(%q) = false, want true", u)
		}
	}
}

// --- GenerateNodeJSON emits the http-specific fields -----------------------
//
// Guards against the emitter-parser-pairing trap (SPEC 103 §9.B6 note): the
// per-scheme switch in GenerateNodeJSON must have its own "http" branch, or
// username/password/path/headers silently vanish from the emitted outbound.

func TestParseNode_HTTPProxy_IPv6Host(t *testing.T) {
	node, err := ParseNode("proxy-http://user1@[2001:db8::1]:8080#http-v6", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if node.Server != "2001:db8::1" {
		t.Errorf("Server = %q, want %q", node.Server, "2001:db8::1")
	}
	if node.Port != 8080 {
		t.Errorf("Port = %d, want 8080", node.Port)
	}
	if got, _ := node.Outbound["username"].(string); got != "user1" {
		t.Errorf(`Outbound["username"] = %q, want "user1"`, got)
	}
}

func TestParseNode_HTTPProxy_SecurityNoneDisablesTLS(t *testing.T) {
	// trojan-convention security=none must switch TLS off even on the https-form.
	node, err := ParseNode("proxy-https://h.example-1.com?security=none", nil)
	if err != nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	if _, ok := node.Outbound["tls"]; ok {
		t.Errorf("tls block present despite security=none: %#v", node.Outbound["tls"])
	}
}
