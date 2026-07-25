package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// Issue #96: the generator must emit max_early_data / early_data_header_name into
// the sing-box transport object. appendOutboundTransportParts is an allow-list of
// keys, so without an explicit branch these fields are silently dropped — the fix
// would work on first import but be lost on every rebuild from state.json.

func TestGenerateNodeJSON_WS_EarlyDataEmission(t *testing.T) {
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@h.test:443?type=ws&security=tls&sni=h.test&host=h.test&path=" + urlEscape("/api/v2/channel?ed=2560") + "#ws-ed"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	js, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	obj := extractFirstJSONObject(js)
	if obj == "" {
		t.Fatalf("no outbound object in:\n%s", js)
	}
	var probe map[string]interface{}
	if err := json.Unmarshal([]byte(obj), &probe); err != nil {
		t.Fatalf("embedded outbound not valid JSON: %v\n%s", err, obj)
	}
	for _, want := range []string{
		`"path":"/api/v2/channel"`,
		`"max_early_data":2560`,
		`"early_data_header_name":"Sec-WebSocket-Protocol"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("emission missing %s\n%s", want, js)
		}
	}
	// The raw ?ed= tail must NOT leak into the emitted path.
	if strings.Contains(js, "ed=2560") {
		t.Errorf("raw ?ed= tail leaked into path:\n%s", js)
	}
}

// Round-trip: after a node is serialized to state.json and read back, numeric
// transport fields come back as float64. The generator must still emit them as
// an integer (not 2560 → "2560.0" or a dropped field).
func TestGenerateNodeJSON_WS_EarlyDataFloat64RoundTrip(t *testing.T) {
	node := &ParsedNode{
		Tag:    "rt",
		Scheme: "vless",
		Server: "h.test",
		Port:   443,
		UUID:   "c59eb5ed-6324-4d53-ad4f-8cda48b30811",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"tag":         "rt",
			"server":      "h.test",
			"server_port": 443,
			"uuid":        "c59eb5ed-6324-4d53-ad4f-8cda48b30811",
			"transport": map[string]interface{}{
				"type":                   "ws",
				"path":                   "/api/v2/channel",
				"max_early_data":         float64(2560), // as it comes back from state.json
				"early_data_header_name": "Sec-WebSocket-Protocol",
			},
		},
	}
	js, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if !strings.Contains(js, `"max_early_data":2560`) {
		t.Errorf("float64 max_early_data not emitted as int 2560:\n%s", js)
	}
	if strings.Contains(js, "2560.0") {
		t.Errorf("max_early_data emitted as float:\n%s", js)
	}
}

func urlEscape(s string) string {
	// tiny local percent-encoder for the one path we need, avoiding a net/url
	// import just for a test literal.
	r := strings.NewReplacer("/", "%2F", "?", "%3F", "=", "%3D")
	return r.Replace(s)
}
