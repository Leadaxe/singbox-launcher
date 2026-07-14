package subscription

import (
	"strings"
	"testing"
)

// SPEC 002 v2: the launcher must parse the full XHTTP field set — flat camelCase
// query params, the `extra` URL-encoded JSON blob, placement/key fields, the
// x-padding obfs block, and packet-up tuning — and map every URL spelling onto
// the core's snake_case transport keys.

func tr(t *testing.T, uri string) map[string]interface{} {
	t.Helper()
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode(%q): err=%v", uri, err)
	}
	m, ok := n.Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("no transport map in %+v", n.Outbound)
	}
	return m
}

func TestXHTTPv2_FlatCamelCaseFields(t *testing.T) {
	// Golden fixture from SPEC 002 §8.2: every v2 field at a non-default value,
	// flat camelCase (what toUri writes). Verifies camelCase→snake_case mapping
	// for all 14 fields plus the base trio and bools.
	uri := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@www.example.com:443?type=xhttp&security=tls&sni=www.example.com&fp=chrome&encryption=none&host=www.example.com&path=%2Fxhttp&mode=packet-up&xPaddingBytes=100-1000&noGRPCHeader=true&sessionPlacement=header&sessionKey=X-Session&seqPlacement=query&seqKey=x_seq&uplinkDataPlacement=header&uplinkDataKey=X-Data&uplinkChunkSize=3000-4000&uplinkHTTPMethod=POST&xPaddingObfsMode=true&xPaddingKey=x_padding&xPaddingHeader=X-Padding&xPaddingPlacement=header&xPaddingMethod=tokenish&scMaxEachPostBytes=1000000&scMinPostsIntervalMs=30#golden"
	m := tr(t, uri)

	wantStr := map[string]string{
		"type":                  "xhttp",
		"host":                  "www.example.com",
		"path":                  "/xhttp",
		"mode":                  "packet-up",
		"x_padding_bytes":       "100-1000",
		"session_placement":     "header",
		"session_key":           "X-Session",
		"seq_placement":         "query",
		"seq_key":               "x_seq",
		"uplink_data_placement": "header",
		"uplink_data_key":       "X-Data",
		"uplink_chunk_size":     "3000-4000",
		"uplink_http_method":    "POST",
		"x_padding_key":         "x_padding",
		"x_padding_header":      "X-Padding",
		"x_padding_placement":   "header",
		"x_padding_method":      "tokenish",
		"sc_max_each_post_bytes":   "1000000",
		"sc_min_posts_interval_ms": "30",
	}
	for k, want := range wantStr {
		if got, _ := m[k].(string); got != want {
			t.Errorf("transport[%q] = %q, want %q", k, got, want)
		}
	}
	if v, _ := m["no_grpc_header"].(bool); !v {
		t.Errorf("no_grpc_header = %v, want true", m["no_grpc_header"])
	}
	if v, _ := m["x_padding_obfs_mode"].(bool); !v {
		t.Errorf("x_padding_obfs_mode = %v, want true", m["x_padding_obfs_mode"])
	}
	// camelCase keys must NOT leak into the sing-box transport.
	for _, leak := range []string{"sessionPlacement", "xPaddingObfsMode", "scMaxEachPostBytes", "noGRPCHeader"} {
		if _, ok := m[leak]; ok {
			t.Errorf("camelCase key %q leaked into transport", leak)
		}
	}
}

func TestXHTTPv2_ExtraJSON(t *testing.T) {
	// SPEC 002 §8.2 / example B: tuning fields arrive in `extra` as URL-encoded
	// JSON, with numbers (scMaxEachPostBytes:"1000000", scMinPostsIntervalMs:30.0)
	// and a float that must drop its ".0". scMaxConcurrentPosts is accept-but-
	// ignore (legacy) and must not surface.
	extra := `{"scMaxEachPostBytes":"1000000","scMaxConcurrentPosts":100.0,"scMinPostsIntervalMs":30.0,"xPaddingBytes":"100-1000","noGRPCHeader":false}`
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@199.232.244.214:443?type=xhttp&mode=packet-up&security=tls&sni=manage.fastly.com&host=oh6.global.ssl.fastly.net&path=%2F&fp=chrome&encryption=none&extra=" + urlEnc(extra) + "#France"
	m := tr(t, uri)

	if got, _ := m["sc_max_each_post_bytes"].(string); got != "1000000" {
		t.Errorf("sc_max_each_post_bytes = %q, want 1000000", got)
	}
	if got, _ := m["sc_min_posts_interval_ms"].(string); got != "30" {
		t.Errorf("sc_min_posts_interval_ms = %q (float .0 must be dropped), want 30", got)
	}
	if got, _ := m["x_padding_bytes"].(string); got != "100-1000" {
		t.Errorf("x_padding_bytes from extra = %q, want 100-1000", got)
	}
	// noGRPCHeader:false in extra → flag stays unset (we only emit when true).
	if _, ok := m["no_grpc_header"]; ok {
		t.Errorf("no_grpc_header should be absent when extra says false, got %v", m["no_grpc_header"])
	}
	// scMaxConcurrentPosts is legacy accept-but-ignore — must not appear.
	if _, ok := m["sc_max_concurrent_posts"]; ok {
		t.Errorf("legacy scMaxConcurrentPosts must be dropped")
	}
}

func TestXHTTPv2_ExtraWinsOverFlat(t *testing.T) {
	// SPEC 002 §1.5: extra has priority for its keys.
	extra := `{"xPaddingBytes":"500-600"}`
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@h.test:443?type=xhttp&xPaddingBytes=100-1000&security=tls&sni=h.test&extra=" + urlEnc(extra) + "#t"
	m := tr(t, uri)
	if got, _ := m["x_padding_bytes"].(string); got != "500-600" {
		t.Errorf("extra must win: x_padding_bytes = %q, want 500-600", got)
	}
}

func TestXHTTPv2_PathQueryTailTrimmed(t *testing.T) {
	// SPEC 002 §4.1: path=/GaMeOpTiMiZeR?ed=2048 → the ?-tail is not the path.
	uri := "vless://c59eb5ed-6324-4d53-ad4f-8cda48b30811@h.test:443?type=xhttp&path=" + urlEnc("/GaMeOpTiMiZeR?ed=2048") + "&security=tls&sni=h.test#t"
	m := tr(t, uri)
	if got, _ := m["path"].(string); got != "/GaMeOpTiMiZeR" {
		t.Errorf("path ?-tail not trimmed: got %q, want /GaMeOpTiMiZeR", got)
	}
}

// Full round-trip of the golden fixture: parseUri → ShareURIFromOutbound →
// parseUri. Every v2 field must survive identically. Catches a field that
// parses but isn't re-emitted (or vice versa).
func TestXHTTPv2_RoundTripGolden(t *testing.T) {
	uri := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@www.example.com:443?type=xhttp&security=tls&sni=www.example.com&fp=chrome&encryption=none&host=www.example.com&path=%2Fxhttp&mode=packet-up&xPaddingBytes=100-1000&noGRPCHeader=true&sessionPlacement=header&sessionKey=X-Session&seqPlacement=query&seqKey=x_seq&uplinkDataPlacement=header&uplinkDataKey=X-Data&uplinkChunkSize=3000-4000&uplinkHTTPMethod=POST&xPaddingObfsMode=true&xPaddingKey=x_padding&xPaddingHeader=X-Padding&xPaddingPlacement=header&xPaddingMethod=tokenish&scMaxEachPostBytes=1000000&scMinPostsIntervalMs=30#golden"
	n, err := ParseNode(uri, nil)
	if err != nil || n == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	got, err := ShareURIFromOutbound(n.Outbound)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	n2, err := ParseNode(got, nil)
	if err != nil || n2 == nil {
		t.Fatalf("re-parse: %v uri=%q", err, got)
	}
	before := n.Outbound["transport"].(map[string]interface{})
	after := n2.Outbound["transport"].(map[string]interface{})

	keys := []string{
		"type", "host", "path", "mode", "x_padding_bytes",
		"session_placement", "session_key", "seq_placement", "seq_key",
		"uplink_data_placement", "uplink_data_key", "uplink_chunk_size", "uplink_http_method",
		"x_padding_key", "x_padding_header", "x_padding_placement", "x_padding_method",
		"sc_max_each_post_bytes", "sc_min_posts_interval_ms",
	}
	for _, k := range keys {
		if before[k] != after[k] {
			t.Errorf("round-trip drift on %q: before=%v after=%v", k, before[k], after[k])
		}
	}
	for _, k := range []string{"no_grpc_header", "x_padding_obfs_mode"} {
		bv, _ := before[k].(bool)
		av, _ := after[k].(bool)
		if bv != av {
			t.Errorf("round-trip drift on bool %q: before=%v after=%v", k, bv, av)
		}
	}
}

// urlEnc percent-encodes a value for embedding in a query string the same way a
// real subscription would. Kept local so the tests read as literal URIs.
func urlEnc(s string) string {
	r := strings.NewReplacer(
		"{", "%7B", "}", "%7D", "\"", "%22", ":", "%3A", ",", "%2C",
		"/", "%2F", "?", "%3F", "=", "%3D", " ", "%20",
	)
	return r.Replace(s)
}
