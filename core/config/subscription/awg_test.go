package subscription

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// awgTestURI builds a valid wireguard:// (or awg://) URI with the canonical WG
// query plus whatever AWG params are passed in `extra`.
func awgTestURI(scheme string, extra url.Values) string {
	q := url.Values{}
	q.Set("publickey", "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=")
	q.Set("address", "10.0.0.2/32")
	q.Set("allowedips", "0.0.0.0/0,::/0")
	q.Set("mtu", "1408")
	q.Set("keepalive", "25")
	for k, vs := range extra {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	return scheme + "://UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=@server.example.com:51821?" + q.Encode() + "#awg-server"
}

func awgFullExtra() url.Values {
	e := url.Values{}
	for k, v := range map[string]string{
		"jc": "10", "jmin": "50", "jmax": "100",
		"s1": "20", "s2": "20", "s3": "60", "s4": "60",
		"h1": "1234567890", "h2": "1234567891", "h3": "1234567892", "h4": "1234567893",
	} {
		e.Set(k, v)
	}
	e.Set("i1", "<b 0x000100002112a442><r 12>")
	e.Set("i2", "<b 0x010100002112a442><r 12>")
	e.Set("i3", "<r 24>")
	return e
}

func TestParseWireGuardURI_AWGFields(t *testing.T) {
	node, err := parseWireGuardURI(awgTestURI("wireguard", awgFullExtra()), nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	wantNum := map[string]int64{
		"jc": 10, "jmin": 50, "jmax": 100,
		"s1": 20, "s2": 20, "s3": 60, "s4": 60,
		"h1": 1234567890, "h2": 1234567891, "h3": 1234567892, "h4": 1234567893,
	}
	for k, want := range wantNum {
		got, ok := node.Outbound[k].(int64)
		if !ok {
			t.Errorf("%s: want int64, got %T (%v)", k, node.Outbound[k], node.Outbound[k])
			continue
		}
		if got != want {
			t.Errorf("%s: want %d, got %d", k, want, got)
		}
	}
	if got, _ := node.Outbound["i1"].(string); got != "<b 0x000100002112a442><r 12>" {
		t.Errorf("i1 mismatch: %q", got)
	}
	if got, _ := node.Outbound["i3"].(string); got != "<r 24>" {
		t.Errorf("i3 mismatch: %q", got)
	}
	// i4/i5 were not supplied → must be absent (no empty keys added).
	if _, ok := node.Outbound["i4"]; ok {
		t.Error("i4 should be absent")
	}
	if _, ok := node.Outbound["i5"]; ok {
		t.Error("i5 should be absent")
	}
}

func TestParseWireGuardURI_NoAWG_StaysClean(t *testing.T) {
	node, err := parseWireGuardURI(awgTestURI("wireguard", nil), nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	for _, k := range append(append([]string{}, awgNumericFields...), awgStringFields...) {
		if _, ok := node.Outbound[k]; ok {
			t.Errorf("plain WG node gained AWG key %q", k)
		}
	}
}

func TestParseWireGuardURI_BadNumeric_Skipped(t *testing.T) {
	e := url.Values{}
	e.Set("jc", "not-a-number")
	e.Set("jmin", "50")
	node, err := parseWireGuardURI(awgTestURI("wireguard", e), nil)
	if err != nil || node == nil {
		t.Fatalf("a bad numeric must not fail the whole node: err=%v", err)
	}
	if _, ok := node.Outbound["jc"]; ok {
		t.Error("invalid jc should be skipped, not stored")
	}
	if v, _ := node.Outbound["jmin"].(int64); v != 50 {
		t.Errorf("jmin should still parse: got %v", node.Outbound["jmin"])
	}
}

func TestIsDirectLink_AWG(t *testing.T) {
	if !IsDirectLink("awg://x@y:1?publickey=a&address=b&allowedips=c") {
		t.Error("awg:// should be a direct link")
	}
}

func TestParseNode_AWGScheme_RoutesToWireguard(t *testing.T) {
	e := url.Values{}
	e.Set("jc", "10")
	node, err := ParseNode(awgTestURI("awg", e), nil)
	if err != nil || node == nil {
		t.Fatalf("awg:// parse failed: err=%v", err)
	}
	if node.Scheme != "wireguard" {
		t.Errorf("awg:// node.Scheme = %q, want wireguard", node.Scheme)
	}
	if v, _ := node.Outbound["jc"].(int64); v != 10 {
		t.Errorf("awg:// jc not parsed: %v", node.Outbound["jc"])
	}
}

func TestShareURI_AWG_RoundTrip(t *testing.T) {
	n1, err := parseWireGuardURI(awgTestURI("wireguard", awgFullExtra()), nil)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	shareURI, err := ShareURIFromWireGuardEndpoint(n1.Outbound)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	n2, err := parseWireGuardURI(shareURI, nil)
	if err != nil {
		t.Fatalf("reparse: %v (uri=%s)", err, shareURI)
	}
	for _, k := range awgNumericFields {
		if n1.Outbound[k] != n2.Outbound[k] {
			t.Errorf("numeric %s drifted: %v(%T) -> %v(%T)", k, n1.Outbound[k], n1.Outbound[k], n2.Outbound[k], n2.Outbound[k])
		}
	}
	for _, k := range []string{"i1", "i2", "i3"} {
		if n1.Outbound[k] != n2.Outbound[k] {
			t.Errorf("string %s drifted (case must be preserved): %q -> %q", k, n1.Outbound[k], n2.Outbound[k])
		}
	}
}

func TestShareURI_AWG_ZeroJc_Preserved(t *testing.T) {
	e := url.Values{}
	e.Set("jc", "0") // explicit junk-off — must survive
	n1, err := parseWireGuardURI(awgTestURI("wireguard", e), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, ok := n1.Outbound["jc"].(int64); !ok || v != 0 {
		t.Fatalf("jc=0 should be stored as int64(0), got %T %v", n1.Outbound["jc"], n1.Outbound["jc"])
	}
	shareURI, err := ShareURIFromWireGuardEndpoint(n1.Outbound)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if !strings.Contains(shareURI, "jc=0") {
		t.Errorf("explicit jc=0 lost in share URI: %s", shareURI)
	}
	n2, _ := parseWireGuardURI(shareURI, nil)
	if v, ok := n2.Outbound["jc"].(int64); !ok || v != 0 {
		t.Errorf("jc=0 lost on round-trip: %T %v", n2.Outbound["jc"], n2.Outbound["jc"])
	}
}

// TestAWG_TypeFidelity_JSON guards the storage type: numeric AWG fields must
// marshal as JSON numbers (sing-box-lx rejects "jc":"10"), i* as strings.
func TestAWG_TypeFidelity_JSON(t *testing.T) {
	e := url.Values{}
	e.Set("jc", "10")
	e.Set("h1", "4000000000") // > int32 max — must not overflow / become string
	e.Set("i1", "<r 24>")
	n, err := parseWireGuardURI(awgTestURI("wireguard", e), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, err := json.Marshal(n.Outbound)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["jc"].(float64); !ok {
		t.Errorf("jc must decode as a JSON number (float64), got %T", m["jc"])
	}
	if f, ok := m["h1"].(float64); !ok || f != 4000000000 {
		t.Errorf("h1 must survive as number 4000000000, got %T %v", m["h1"], m["h1"])
	}
	if s, ok := m["i1"].(string); !ok || s != "<r 24>" {
		t.Errorf("i1 must be a string %q, got %T %v", "<r 24>", m["i1"], m["i1"])
	}
}

// TestParseWireGuardURI_MTUClamp verifies the AWG MTU policy (SPEC 073 follow-up):
// AmneziaWG endpoints default to / are clamped to awgMaxMTU (1280) because AWG's
// S3/S4 transport padding would otherwise push data packets past the path MTU and
// fail with EMSGSIZE (handshake OK, data silently stops). Plain WireGuard keeps
// the upstream 1420 default and honors the URI value verbatim.
func TestParseWireGuardURI_MTUClamp(t *testing.T) {
	const (
		pk   = "UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA="
		pub  = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="
		base = "publickey=" + pub + "&address=10.0.0.2/32&allowedips=0.0.0.0/0"
	)
	uri := func(extra string) string {
		return "wireguard://" + pk + "@server.example.com:51821?" + base + extra + "#n"
	}
	cases := []struct {
		name  string
		extra string
		want  int
	}{
		{"awg high mtu clamped", "&jc=10&mtu=1420", 1280},
		{"awg no mtu defaults low", "&jc=10", 1280},
		{"awg explicit lower honored", "&jc=10&mtu=1200", 1200},
		{"awg string-only field still AWG", "&i1=%3Cr+24%3E&mtu=1500", 1280},
		{"plain wg keeps high mtu", "&mtu=1500", 1500},
		{"plain wg default", "", 1420},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := parseWireGuardURI(uri(c.extra), nil)
			if err != nil || n == nil {
				t.Fatalf("parse: err=%v node=%v", err, n)
			}
			got, ok := n.Outbound["mtu"].(int)
			if !ok {
				t.Fatalf("mtu type = %T, want int", n.Outbound["mtu"])
			}
			if got != c.want {
				t.Errorf("mtu = %d, want %d", got, c.want)
			}
		})
	}
}
