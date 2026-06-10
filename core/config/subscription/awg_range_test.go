package subscription

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// SPEC 073.2: AWG 2.0 header randomization ranges (H1-H4 = "lo-hi") pass
// through to the endpoint as normalized strings — the sing-box-lx core
// (>= 1.13.13-lx.6) accepts "h1": "N-M" and picks an in-range value per
// handshake. Plain values stay int64 (JSON numbers), as before.
func TestParseWireGuardURI_AWGHeaderRanges(t *testing.T) {
	e := url.Values{}
	e.Set("h1", "43613244-384550127")
	e.Set("h2", "300-200") // reversed — normalized to lo-hi
	e.Set("h3", "10-x")    // garbage — skipped, node survives
	e.Set("h4", "992706287")
	e.Set("jc", "10-20") // ranges are h-only; non-header numeric range is skipped
	node, err := parseWireGuardURI(awgTestURI("wireguard", e), nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v", err)
	}
	if v, _ := node.Outbound["h1"].(string); v != "43613244-384550127" {
		t.Errorf("h1 = %v (%T), want range string", node.Outbound["h1"], node.Outbound["h1"])
	}
	if v, _ := node.Outbound["h2"].(string); v != "200-300" {
		t.Errorf("h2 = %v, want normalized 200-300", node.Outbound["h2"])
	}
	if _, ok := node.Outbound["h3"]; ok {
		t.Error("garbage h3 must be skipped, not stored")
	}
	if v, _ := node.Outbound["h4"].(int64); v != 992706287 {
		t.Errorf("h4 = %v (%T), want plain int64", node.Outbound["h4"], node.Outbound["h4"])
	}
	if _, ok := node.Outbound["jc"]; ok {
		t.Error("range on jc (non-header field) must be skipped")
	}
}

// Ranged h-values must survive endpoint → share-URI → endpoint untouched and
// marshal as JSON strings (the core's range form), plain ones as numbers.
func TestShareURI_AWGHeaderRanges_RoundTrip(t *testing.T) {
	e := url.Values{}
	e.Set("h1", "43613244-384550127")
	e.Set("h4", "992706287")
	n1, err := parseWireGuardURI(awgTestURI("wireguard", e), nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	shareURI, err := ShareURIFromWireGuardEndpoint(n1.Outbound)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if !strings.Contains(shareURI, "h1=43613244-384550127") {
		t.Errorf("share URI lost the h1 range: %s", shareURI)
	}
	n2, err := parseWireGuardURI(shareURI, nil)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if n1.Outbound["h1"] != n2.Outbound["h1"] || n1.Outbound["h4"] != n2.Outbound["h4"] {
		t.Errorf("round-trip drift: h1 %v -> %v, h4 %v -> %v",
			n1.Outbound["h1"], n2.Outbound["h1"], n1.Outbound["h4"], n2.Outbound["h4"])
	}
	b, err := json.Marshal(n1.Outbound)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["h1"].(string); !ok {
		t.Errorf("ranged h1 must marshal as a JSON string, got %T", m["h1"])
	}
	if _, ok := m["h4"].(float64); !ok {
		t.Errorf("plain h4 must marshal as a JSON number, got %T", m["h4"])
	}
}

// Real-world AmneziaWG 2.0 .conf shape (synthetic keys): H ranges, an i1 blob,
// empty I2-I5 lines — pasted as text it must yield a complete AWG endpoint.
func TestConvertWGConfText_AWG2RangesAndEmptyI(t *testing.T) {
	conf := `[Interface]
Address = 10.8.1.25/32
DNS = 172.29.172.254, 1.0.0.1
PrivateKey = UFJJVkFURUtFWTAwMDAwMDAwMDAwMDAwMDAwMA=
Jc = 5
Jmin = 10
Jmax = 50
S1 = 28
S2 = 121
S3 = 25
S4 = 9
H1 = 43613244-384550127
H2 = 826869626-2105069164
H3 = 2124774725-2141151992
H4 = 2144594503-2146278491
I1 = <b 0x084481800001000300000000077469636b657473>
I2 =
I3 =
I4 =
I5 =

[Peer]
PublicKey = QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=
PresharedKey = UFNLMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.7:44733
PersistentKeepalive = 25
`
	uri, err := ConvertWGConfText(conf)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	wantNum := map[string]int64{"jc": 5, "jmin": 10, "jmax": 50, "s1": 28, "s2": 121, "s3": 25, "s4": 9}
	for k, w := range wantNum {
		if v, _ := node.Outbound[k].(int64); v != w {
			t.Errorf("%s = %v, want %d", k, node.Outbound[k], w)
		}
	}
	wantRange := map[string]string{
		"h1": "43613244-384550127",
		"h2": "826869626-2105069164",
		"h3": "2124774725-2141151992",
		"h4": "2144594503-2146278491",
	}
	for k, w := range wantRange {
		if v, _ := node.Outbound[k].(string); v != w {
			t.Errorf("%s = %v (%T), want range string %q", k, node.Outbound[k], node.Outbound[k], w)
		}
	}
	if s, _ := node.Outbound["i1"].(string); !strings.HasPrefix(s, "<b 0x0844") {
		t.Errorf("i1 lost or mangled: %q", s)
	}
	for _, k := range []string{"i2", "i3", "i4", "i5"} {
		if _, ok := node.Outbound[k]; ok {
			t.Errorf("empty %s line must not produce a key", k)
		}
	}
	if v, _ := node.Outbound["mtu"].(int); v != 1280 {
		t.Errorf("mtu = %v, want AWG default 1280", node.Outbound["mtu"])
	}
}

// Overlap detection mirrors the core contract: unset header = WG default
// (h1=1 … h4=4), single = [v,v], range = [lo,hi].
func TestAWGHeaderOverlap(t *testing.T) {
	cases := []struct {
		name     string
		ep       map[string]interface{}
		wantPair bool
	}{
		{"real-world disjoint ranges", map[string]interface{}{
			"h1": "43613244-384550127", "h2": "826869626-2105069164",
			"h3": "2124774725-2141151992", "h4": "2144594503-2146278491",
		}, false},
		{"distinct singles", map[string]interface{}{
			"h1": int64(100), "h2": int64(200), "h3": int64(300), "h4": int64(400),
		}, false},
		{"range covers default of unset h2", map[string]interface{}{
			"h1": "1-100",
		}, true},
		{"single inside range", map[string]interface{}{
			"h1": int64(500), "h2": "400-600", "h3": int64(700), "h4": int64(800),
		}, true},
		{"no awg headers at all", map[string]interface{}{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := awgHeaderOverlap(c.ep)
			if got := a != ""; got != c.wantPair {
				t.Errorf("overlap = %v (%s/%s), want %v", got, a, b, c.wantPair)
			}
		})
	}
}
