package subscription

import (
	"net/url"
	"testing"
)

// Issue #96: Xray encodes WebSocket Early Data as a `?ed=N` tail on the WS path
// (/api/v2/channel?ed=2560). sing-box treats that literally and the server 404s.
// Every WS transport builder (share-URI, Xray JSON, VMess) must split the tail
// into max_early_data + early_data_header_name=Sec-WebSocket-Protocol.

func TestSplitWSEarlyData(t *testing.T) {
	tests := []struct {
		in       string
		wantPath string
		wantED   int
	}{
		{"/api/v2/channel?ed=2560", "/api/v2/channel", 2560},
		{"/path?ed=2048&foo=bar", "/path", 2048}, // ed among other params
		{"/path?foo=bar&ed=100", "/path", 100},   // ed not first
		{"/plain", "/plain", 0},                  // no tail
		{"/plain?ed=", "/plain", 0},              // empty ed → strip tail, no ED
		{"/plain?ed=abc", "/plain", 0},           // non-numeric ed → no ED
		{"/plain?ed=0", "/plain", 0},             // ed=0 means disabled
		{"/plain?ed=-5", "/plain", 0},            // negative → ignored
		{"/plain?foo=bar", "/plain", 0},          // tail without ed → still stripped
		{"  /sp?ed=64  ", "/sp", 64},             // surrounding whitespace
		{"/p?ED=2560", "/p", 2560},               // uppercase key — folded like the rest of the parser
		{"/p?ed=2560&ed=99", "/p", 2560},         // duplicate ed → first wins
		{"", "", 0},                              // empty
		{"?ed=32", "", 32},                       // path is only the tail
	}
	for _, tt := range tests {
		gotPath, gotED := splitWSEarlyData(tt.in)
		if gotPath != tt.wantPath || gotED != tt.wantED {
			t.Errorf("splitWSEarlyData(%q) = (%q, %d), want (%q, %d)", tt.in, gotPath, gotED, tt.wantPath, tt.wantED)
		}
	}
}

func TestApplyWSEarlyData(t *testing.T) {
	// With ed: all three fields set.
	tr := map[string]interface{}{"type": "ws"}
	applyWSEarlyData(tr, "/api/v2/channel?ed=2560")
	if tr["path"] != "/api/v2/channel" {
		t.Fatalf("path = %v, want /api/v2/channel", tr["path"])
	}
	if tr["max_early_data"] != 2560 {
		t.Fatalf("max_early_data = %v, want 2560", tr["max_early_data"])
	}
	if tr["early_data_header_name"] != "Sec-WebSocket-Protocol" {
		t.Fatalf("early_data_header_name = %v, want Sec-WebSocket-Protocol", tr["early_data_header_name"])
	}

	// Without ed: only path, no early-data keys leak in.
	tr2 := map[string]interface{}{"type": "ws"}
	applyWSEarlyData(tr2, "/plain")
	if tr2["path"] != "/plain" {
		t.Fatalf("path = %v, want /plain", tr2["path"])
	}
	if _, ok := tr2["max_early_data"]; ok {
		t.Fatal("max_early_data must be absent when no ed")
	}
	if _, ok := tr2["early_data_header_name"]; ok {
		t.Fatal("early_data_header_name must be absent when no ed")
	}
}

func TestAppendEarlyDataToPath(t *testing.T) {
	tests := []struct {
		path string
		ed   int
		want string
	}{
		{"/api/v2/channel", 2560, "/api/v2/channel?ed=2560"},
		{"/p", 0, "/p"},                // no ed → untouched
		{"/p", -1, "/p"},               // invalid → untouched
		{"/p?x=1", 64, "/p?x=1&ed=64"}, // existing query → &ed
		{"", 32, "?ed=32"},             // empty path
	}
	for _, tt := range tests {
		if got := appendEarlyDataToPath(tt.path, tt.ed); got != tt.want {
			t.Errorf("appendEarlyDataToPath(%q, %d) = %q, want %q", tt.path, tt.ed, got, tt.want)
		}
	}
}

// --- Parser 1: share-URI (type=ws&path=...) ---

func TestParseNode_VLESS_WS_EarlyDataFromURI(t *testing.T) {
	// path=/api/v2/channel?ed=2560, percent-encoded in the query.
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@h.test:443?type=ws&security=tls&sni=h.test&host=h.test&path=" + url.QueryEscape("/api/v2/channel?ed=2560") + "#ws-ed"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tr, ok := node.Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing transport; outbound=%v", node.Outbound)
	}
	assertWSEarlyData(t, tr, "/api/v2/channel", 2560)
}

func TestParseNode_VLESS_WS_NoEarlyData(t *testing.T) {
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@h.test:443?type=ws&security=tls&sni=h.test&path=" + url.QueryEscape("/plain") + "#ws-plain"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tr, _ := node.Outbound["transport"].(map[string]interface{})
	if tr["path"] != "/plain" {
		t.Fatalf("path = %v want /plain", tr["path"])
	}
	if _, ok := tr["max_early_data"]; ok {
		t.Fatal("plain ws must not carry max_early_data")
	}
}

// --- Parser 2: Xray JSON array (wsSettings.path) — the issue's exact case ---

func TestParseNodesFromXrayJSONArray_WS_EarlyData(t *testing.T) {
	raw := `[
	  {
		"remarks": "ws-ed",
		"outbounds": [
		  {
			"protocol": "vless",
			"tag": "proxy",
			"settings": {
			  "vnext": [{ "address": "h.test", "port": 443, "users": [{ "id": "c59eb5ed-6324-4d53-ad4f-8cda48b30811", "encryption": "none" }] }]
			},
			"streamSettings": {
			  "network": "ws",
			  "security": "tls",
			  "tlsSettings": { "serverName": "h.test" },
			  "wsSettings": { "path": "/api/v2/channel?ed=2560", "host": "h.test" }
			}
		  }
		]
	  }
	]`
	nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	tr, ok := nodes[0].Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing transport; outbound=%v", nodes[0].Outbound)
	}
	assertWSEarlyData(t, tr, "/api/v2/channel", 2560)
	// Host header must still be preserved alongside the split.
	h, _ := tr["headers"].(map[string]string)
	if h["Host"] != "h.test" {
		t.Fatalf("Host header lost: %v", tr["headers"])
	}
}

// --- Parser 3: VMess JSON (net=ws, path=/x?ed=N) ---

func TestParseVMess_WS_EarlyData(t *testing.T) {
	// Legacy VMess JSON carries net=ws and the ed tail inside path.
	vmess := map[string]interface{}{
		"v":    "2",
		"ps":   "vmess-ed",
		"add":  "h.test",
		"port": float64(443), // JSON numbers decode to float64
		"id":   "c59eb5ed-6324-4d53-ad4f-8cda48b30811",
		"aid":  float64(0),
		"net":  "ws",
		"type": "none",
		"host": "h.test",
		"path": "/api/v2/channel?ed=2048",
		"tls":  "tls",
	}
	node, err := parseVMessJSON(vmess, nil)
	if err != nil || node == nil {
		t.Fatalf("parseVMessJSON: err=%v node=%v", err, node)
	}
	tr, ok := node.Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing transport; outbound=%v", node.Outbound)
	}
	assertWSEarlyData(t, tr, "/api/v2/channel", 2048)
}

// --- Exporter round-trip: share-URI (vless) preserves ?ed=N ---

func TestShareURIFromOutbound_VLESS_WS_EarlyDataRoundTrip(t *testing.T) {
	out := map[string]interface{}{
		"type":        "vless",
		"tag":         "rt",
		"server":      "h.test",
		"server_port": 443,
		"uuid":        "c59eb5ed-6324-4d53-ad4f-8cda48b30811",
		"tls":         map[string]interface{}{"enabled": true, "server_name": "h.test"},
		"transport": map[string]interface{}{
			"type":                   "ws",
			"path":                   "/api/v2/channel",
			"max_early_data":         2560,
			"early_data_header_name": "Sec-WebSocket-Protocol",
		},
	}
	uri, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	// Re-parse and confirm the early data survived the round trip.
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("re-parse: err=%v uri=%s", err, uri)
	}
	tr, ok := node.Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("re-parsed node lost transport: %v", node.Outbound)
	}
	assertWSEarlyData(t, tr, "/api/v2/channel", 2560)
}

func assertWSEarlyData(t *testing.T, tr map[string]interface{}, wantPath string, wantED int) {
	t.Helper()
	if tr["path"] != wantPath {
		t.Fatalf("path = %v, want %v", tr["path"], wantPath)
	}
	// max_early_data may be int (fresh parse); normalize via mapGetInt.
	if got := mapGetInt(tr, "max_early_data"); got != wantED {
		t.Fatalf("max_early_data = %v, want %d", tr["max_early_data"], wantED)
	}
	if tr["early_data_header_name"] != "Sec-WebSocket-Protocol" {
		t.Fatalf("early_data_header_name = %v, want Sec-WebSocket-Protocol", tr["early_data_header_name"])
	}
}
