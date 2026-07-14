package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// SPEC 002 v2: the full XHTTP field set must be emitted into config.json under
// the core's snake_case keys, and the result must be valid JSON. This is the
// emission counterpart to the subscription-package parse/round-trip tests.
func TestGenerateNodeJSON_XHTTPv2Emission(t *testing.T) {
	uri := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@www.example.com:443?type=xhttp&security=tls&sni=www.example.com&fp=chrome&encryption=none&host=www.example.com&path=%2Fxhttp&mode=packet-up&xPaddingBytes=100-1000&noGRPCHeader=true&sessionPlacement=header&sessionKey=X-Session&seqPlacement=query&seqKey=x_seq&uplinkDataPlacement=header&uplinkDataKey=X-Data&uplinkChunkSize=3000-4000&uplinkHTTPMethod=POST&xPaddingObfsMode=true&xPaddingKey=x_padding&xPaddingHeader=X-Padding&xPaddingPlacement=header&xPaddingMethod=tokenish&scMaxEachPostBytes=1000000&scMinPostsIntervalMs=30#golden"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	js, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	// GenerateNodeJSON emits an array-element fragment (leading // comment,
	// trailing comma), so it is not a standalone JSON document. Sanity-check
	// that the embedded outbound object parses on its own — this catches a
	// broken string-concat builder (unbalanced braces, missing commas).
	if obj := extractFirstJSONObject(js); obj != "" {
		var probe map[string]interface{}
		if err := json.Unmarshal([]byte(obj), &probe); err != nil {
			t.Fatalf("embedded outbound object is not valid JSON: %v\n%s", err, obj)
		}
	} else {
		t.Fatalf("could not locate outbound JSON object in:\n%s", js)
	}
	wantSubstrings := []string{
		`"type":"xhttp"`,
		`"mode":"packet-up"`,
		`"x_padding_bytes":"100-1000"`,
		`"no_grpc_header":true`,
		`"x_padding_obfs_mode":true`,
		`"session_placement":"header"`,
		`"seq_placement":"query"`,
		`"uplink_data_placement":"header"`,
		`"uplink_chunk_size":"3000-4000"`,
		`"uplink_http_method":"POST"`,
		`"x_padding_placement":"header"`,
		`"x_padding_method":"tokenish"`,
		`"sc_max_each_post_bytes":"1000000"`,
		`"sc_min_posts_interval_ms":"30"`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(js, want) {
			t.Errorf("config.json missing %s\nJSON: %s", want, js)
		}
	}
	// camelCase must never reach the core JSON.
	for _, leak := range []string{"sessionPlacement", "xPaddingObfsMode", "scMaxEachPostBytes", "noGRPCHeader"} {
		if strings.Contains(js, leak) {
			t.Errorf("camelCase key %q leaked into config.json\nJSON: %s", leak, js)
		}
	}
}

// extractFirstJSONObject returns the first balanced {...} object in s (the
// builder wraps it in a // comment and a trailing comma). Brace counting is
// fine here: the generated outbound has no braces inside string values.
func extractFirstJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// VLESS flow (xtls-rprx-vision) is valid only over bare TLS/Reality. When a
// v2ray transport is present sing-box rejects the combo, so the generator must
// drop flow. Conversely, flow over bare Reality (no transport) must survive.
func TestGenerateNodeJSON_FlowDroppedWithTransport(t *testing.T) {
	cases := []struct {
		name     string
		uri      string
		wantFlow bool
	}{
		{
			name:     "flow + xhttp transport → flow dropped",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=xtls-rprx-vision&type=xhttp&path=%2Fx&host=h.test&security=reality&sni=h.test&pbk=BBBB&sid=64f4#t",
			wantFlow: false,
		},
		{
			name:     "flow + ws transport → flow dropped",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=xtls-rprx-vision&type=ws&path=%2Fw&security=tls&sni=h.test#t",
			wantFlow: false,
		},
		{
			name:     "flow + bare Reality (no transport) → flow kept",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=xtls-rprx-vision&type=tcp&security=reality&sni=h.test&pbk=BBBB&sid=64f4#t",
			wantFlow: true,
		},
		{
			name:     "flow=none bare TCP → dropped (x3-ui literal none)",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=none&type=tcp&security=reality&sni=h.test&pbk=BBBB&sid=64f4#t",
			wantFlow: false,
		},
		{
			name:     "flow=xtls-rprx-direct (deprecated) → dropped",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=xtls-rprx-direct&type=tcp&security=reality&sni=h.test&pbk=BBBB&sid=64f4#t",
			wantFlow: false,
		},
		{
			name:     "flow=xtls-rprx-vision-udp443 bare TCP → normalized vision kept",
			uri:      "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@h.test:443?encryption=none&flow=xtls-rprx-vision-udp443&type=tcp&security=reality&sni=h.test&pbk=BBBB&sid=64f4#t",
			wantFlow: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node, err := subscription.ParseNode(c.uri, nil)
			if err != nil || node == nil {
				t.Fatalf("parse: %v", err)
			}
			js, err := GenerateNodeJSON(node)
			if err != nil {
				t.Fatalf("gen: %v", err)
			}
			hasFlow := strings.Contains(js, `"flow"`)
			if hasFlow != c.wantFlow {
				t.Errorf("flow present = %v, want %v\nJSON: %s", hasFlow, c.wantFlow, js)
			}
			// The only emitted flow value sing-box accepts is xtls-rprx-vision.
			if hasFlow && !strings.Contains(js, `"flow":"xtls-rprx-vision"`) {
				t.Errorf("emitted flow must be exactly xtls-rprx-vision\nJSON: %s", js)
			}
		})
	}
}

// Guard: outboundHasTransport recognizes a real transport but not an empty/absent one.
func TestOutboundHasTransport(t *testing.T) {
	if outboundHasTransport(nil) {
		t.Error("nil outbound has no transport")
	}
	if outboundHasTransport(map[string]interface{}{"type": "vless"}) {
		t.Error("no transport key → false")
	}
	if outboundHasTransport(map[string]interface{}{"transport": map[string]interface{}{}}) {
		t.Error("empty transport map → false")
	}
	if !outboundHasTransport(map[string]interface{}{"transport": map[string]interface{}{"type": "xhttp"}}) {
		t.Error("xhttp transport → true")
	}
}
