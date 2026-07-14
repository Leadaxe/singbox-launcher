package subscription

import (
	"testing"
)

// A WARP wireguard:// URI carries reserved=b0,b1,b2; the parser must land it on
// the peer, and the share-URI encoder must re-emit it (lossless round-trip).
func TestWireGuardReservedRoundTrip(t *testing.T) {
	uri := "wireguard://8HwVtYZaYEecHZ6MqEzHDaOySDpOEDqDEGk0XjFGNHc=@162.159.192.10:2408" +
		"?publickey=bmXOC%2BF1FxEMF9dyiK2H5%2F1SUtzH0JuVo51h2wPfgyo%3D" +
		"&address=172.16.0.2%2F32&allowedips=0.0.0.0%2F0%2C%3A%3A%2F0&reserved=29%2C172%2C92#WARP"

	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	ep := node.Outbound
	peers, ok := ep["peers"].([]map[string]interface{})
	if !ok || len(peers) == 0 {
		t.Fatalf("no peers in endpoint: %#v", ep["peers"])
	}
	res, ok := peers[0]["reserved"].([]int)
	if !ok {
		t.Fatalf("reserved wrong type: %#v", peers[0]["reserved"])
	}
	if len(res) != 3 || res[0] != 29 || res[1] != 172 || res[2] != 92 {
		t.Fatalf("reserved = %v, want [29 172 92]", res)
	}

	// round-trip through the share-URI encoder
	back, err := ShareURIFromWireGuardEndpoint(ep)
	if err != nil {
		t.Fatalf("share uri: %v", err)
	}
	node2, err := ParseNode(back, nil)
	if err != nil || node2 == nil {
		t.Fatalf("reparse: %v", err)
	}
	peers2 := node2.Outbound["peers"].([]map[string]interface{})
	res2 := peers2[0]["reserved"].([]int)
	if len(res2) != 3 || res2[0] != 29 || res2[1] != 172 || res2[2] != 92 {
		t.Fatalf("reserved lost in round-trip: %v", res2)
	}
}

// An AmneziaWG/WARP node carrying masquerade sugar ip/id/ib must promote those
// onto the endpoint and re-emit them on encode (SPEC 009 / SPEC 084).
func TestWireGuardMasqueradeRoundTrip(t *testing.T) {
	uri := "wireguard://8HwVtYZaYEecHZ6MqEzHDaOySDpOEDqDEGk0XjFGNHc=@188.114.96.7:955" +
		"?publickey=bmXOC%2BF1FxEMF9dyiK2H5%2F1SUtzH0JuVo51h2wPfgyo%3D" +
		"&address=172.16.0.2%2F32&allowedips=0.0.0.0%2F0" +
		"&jc=4&jmin=40&jmax=70&s1=0&s2=0&h1=1&h2=2&h3=3&h4=4" +
		"&ip=quic&id=www.google.com&ib=chrome#WARP"

	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	ep := node.Outbound
	for k, want := range map[string]string{"ip": "quic", "id": "www.google.com", "ib": "chrome"} {
		if got, _ := ep[k].(string); got != want {
			t.Errorf("endpoint[%q] = %q, want %q", k, got, want)
		}
	}
	back, err := ShareURIFromWireGuardEndpoint(ep)
	if err != nil {
		t.Fatalf("share uri: %v", err)
	}
	node2, err := ParseNode(back, nil)
	if err != nil || node2 == nil {
		t.Fatalf("reparse: %v", err)
	}
	for k, want := range map[string]string{"ip": "quic", "id": "www.google.com", "ib": "chrome"} {
		if got, _ := node2.Outbound[k].(string); got != want {
			t.Errorf("round-trip endpoint[%q] = %q, want %q", k, got, want)
		}
	}
}

// An explicit i1 must suppress id/ip/ib promotion (the core rejects both).
func TestWireGuardExplicitI1SuppressesMasquerade(t *testing.T) {
	uri := "wireguard://8HwVtYZaYEecHZ6MqEzHDaOySDpOEDqDEGk0XjFGNHc=@188.114.96.7:955" +
		"?publickey=bmXOC%2BF1FxEMF9dyiK2H5%2F1SUtzH0JuVo51h2wPfgyo%3D" +
		"&address=172.16.0.2%2F32&allowedips=0.0.0.0%2F0" +
		"&i1=%3Cb%200x11%3E&ip=quic&id=example.com#WARP"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := node.Outbound["ip"]; ok {
		t.Errorf("ip must be dropped when i1 is explicit")
	}
	if _, ok := node.Outbound["i1"]; !ok {
		t.Errorf("explicit i1 must be preserved")
	}
}

func TestReservedTripletParsing(t *testing.T) {
	cases := map[string][]int{
		"29,172,92":   {29, 172, 92},
		" 1 , 2 , 3 ": {1, 2, 3},
		"":            nil,
		"1,2":         nil,
		"1,2,3,4":     nil,
		"1,2,999":     nil, // out of byte range
		"a,b,c":       nil,
		"-1,2,3":      nil,
	}
	for in, want := range cases {
		got := parseReservedTriplet(in)
		if len(got) != len(want) {
			t.Errorf("parseReservedTriplet(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("parseReservedTriplet(%q) = %v, want %v", in, got, want)
				break
			}
		}
	}
}
