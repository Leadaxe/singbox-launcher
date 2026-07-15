package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func rawOutbound(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func decodeOutbound(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestApplyTLSTransforms_FragmentOnFirstHop(t *testing.T) {
	obs := []json.RawMessage{
		rawOutbound(t, map[string]interface{}{
			"type": "vless", "tag": "v", "server": "a.example",
			"tls": map[string]interface{}{"enabled": true, "server_name": "cover.example.com"},
		}),
	}
	out := ApplyTLSTransforms(obs, TLSTransformOptions{Fragment: true, FragmentFallbackDelay: "500ms"})
	tls := decodeOutbound(t, out[0])["tls"].(map[string]interface{})
	if tls["fragment"] != true {
		t.Errorf("fragment not set: %#v", tls)
	}
	if tls["fragment_fallback_delay"] != "500ms" {
		t.Errorf("fallback delay = %v", tls["fragment_fallback_delay"])
	}
}

func TestApplyTLSTransforms_SkipsDetourAndUtility(t *testing.T) {
	obs := []json.RawMessage{
		// detour (inner hop) — must be skipped
		rawOutbound(t, map[string]interface{}{
			"type": "trojan", "tag": "inner", "detour": "hop",
			"tls": map[string]interface{}{"enabled": true, "server_name": "x.example"},
		}),
		// direct — utility, skipped
		rawOutbound(t, map[string]interface{}{"type": "direct", "tag": "direct-out"}),
		// naive — manages own stack, skipped
		rawOutbound(t, map[string]interface{}{
			"type": "naive", "tag": "n",
			"tls": map[string]interface{}{"enabled": true, "server_name": "n.example"},
		}),
		// tls disabled — skipped
		rawOutbound(t, map[string]interface{}{
			"type": "vmess", "tag": "notls",
			"tls": map[string]interface{}{"enabled": false},
		}),
	}
	out := ApplyTLSTransforms(obs, TLSTransformOptions{Fragment: true})
	for i, raw := range out {
		m := decodeOutbound(t, raw)
		if tls, ok := m["tls"].(map[string]interface{}); ok {
			if _, has := tls["fragment"]; has {
				t.Errorf("outbound %d (%v) should not have been fragmented", i, m["tag"])
			}
		}
	}
}

func TestApplyTLSTransforms_NoopWhenDisabled(t *testing.T) {
	obs := []json.RawMessage{
		rawOutbound(t, map[string]interface{}{
			"type": "vless", "tag": "v",
			"tls": map[string]interface{}{"enabled": true, "server_name": "x.example"},
		}),
	}
	out := ApplyTLSTransforms(obs, TLSTransformOptions{})
	// identical slice returned (no-op)
	if &out[0] != &obs[0] && string(out[0]) != string(obs[0]) {
		t.Errorf("expected unchanged outbound when no transform enabled")
	}
}

func TestMixedCaseSNI_Deterministic_PunycodeSafe(t *testing.T) {
	// deterministic: same input → same output across calls
	a := mixedCaseSNI("www.google.com")
	b := mixedCaseSNI("www.google.com")
	if a != b {
		t.Errorf("mixedCaseSNI not deterministic: %q vs %q", a, b)
	}
	// case-insensitive equal to the original (only case changed)
	if !strings.EqualFold(a, "www.google.com") {
		t.Errorf("mixedCaseSNI changed more than case: %q", a)
	}
	// actually mixed (not all-lower) for a hostname with letters
	if a == "www.google.com" {
		t.Errorf("expected case to change for %q", a)
	}
	// Punycode label left intact
	got := mixedCaseSNI("xn--80akhbyknj4f.example")
	labels := strings.Split(got, ".")
	if labels[0] != "xn--80akhbyknj4f" {
		t.Errorf("Punycode label must be untouched, got %q", labels[0])
	}
}

func TestTLSTransformOptionsFromVars(t *testing.T) {
	opts := TLSTransformOptionsFromVars(map[string]string{
		"tls_fragment":        "true",
		"tls_record_fragment": "1",
		"tls_mixed_case_sni":  "on",
	})
	if !opts.Fragment || !opts.RecordFragment || !opts.MixedCaseSNI {
		t.Errorf("vars not parsed: %+v", opts)
	}
	if TLSTransformOptionsFromVars(map[string]string{"tls_fragment": "false"}).Fragment {
		t.Errorf("false should be off")
	}
}
