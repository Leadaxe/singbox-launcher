package subscription

import (
	"net/url"
	"testing"
)

// All credentials below are fake placeholders.

func TestParseNode_Tuic_Canonical(t *testing.T) {
	uri := "tuic://00000000-0000-0000-0000-000000000001:testpass@tuic.example.test:443/?congestion_control=bbr&alpn=h3,spdy/3.1&udp_relay_mode=native&allow_insecure=1#TUIC-smoke"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode(%q) error: %v", uri, err)
	}
	assertEq(t, node.Scheme, "tuic")
	assertEq(t, node.Server, "tuic.example.test")
	assertEq(t, node.Port, 443)
	assertEq(t, node.UUID, "00000000-0000-0000-0000-000000000001")
	assertEq(t, node.Query.Get("password"), "testpass")
	assertEq(t, node.Label, "TUIC-smoke")
}

func TestBuildOutbound_Tuic(t *testing.T) {
	uri := "tuic://uuid-1:secret@tuic.example.test:443/?congestion_control=bbr&alpn=h3,spdy/3.1&udp_relay_mode=native&allow_insecure=1#node"
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "tuic-out"
	out := buildOutbound(node)

	assertEq(t, out["type"], "tuic")
	assertEq(t, out["tag"], "tuic-out")
	assertEq(t, out["server"], "tuic.example.test")
	assertEq(t, out["server_port"], 443)
	assertEq(t, out["uuid"], "uuid-1")
	assertEq(t, out["password"], "secret")
	assertEq(t, out["congestion_control"], "bbr")
	assertEq(t, out["udp_relay_mode"], "native")

	tls, ok := out["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls block missing or wrong type: %T", out["tls"])
	}
	assertEq(t, tls["enabled"], true)
	assertEq(t, tls["server_name"], "tuic.example.test")
	assertEq(t, tls["insecure"], true) // from allow_insecure=1
	alpn, ok := tls["alpn"].([]string)
	if !ok || len(alpn) != 2 || alpn[0] != "h3" || alpn[1] != "spdy/3.1" {
		t.Errorf("alpn = %v, want [h3 spdy/3.1]", tls["alpn"])
	}
}

func TestParseNode_Tuic_DefaultPort(t *testing.T) {
	node, err := ParseNode("tuic://u:p@host.tld", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	assertEq(t, node.Port, 443)
}

func TestParseNode_Tuic_MissingUserinfoRejected(t *testing.T) {
	if _, err := ParseNode("tuic://host.tld:443", nil); err == nil {
		t.Error("TUIC URI without uuid userinfo must be rejected")
	}
}

func TestBuildOutbound_Tuic_UnknownCongestionDropped(t *testing.T) {
	node, err := ParseNode("tuic://u:p@host.tld/?congestion_control=reno-xyz", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "t"
	out := buildOutbound(node)
	if _, has := out["congestion_control"]; has {
		t.Errorf("unknown congestion_control must be dropped, got %v", out["congestion_control"])
	}
}

func TestBuildOutbound_Tuic_HeartbeatSeconds(t *testing.T) {
	node, err := ParseNode("tuic://u:p@host.tld/?heartbeat=10", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "t"
	out := buildOutbound(node)
	assertEq(t, out["heartbeat"], "10s")
}

func TestBuildOutbound_Tuic_ZeroRTTAlias(t *testing.T) {
	node, err := ParseNode("tuic://u:p@host.tld/?reduce_rtt=1", nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	node.Tag = "t"
	out := buildOutbound(node)
	assertEq(t, out["zero_rtt_handshake"], true)
}

func TestShareURIRoundtrip_Tuic(t *testing.T) {
	input := "tuic://uuid-9:passw0rd@tuic.example.test:8443/?congestion_control=bbr&udp_relay_mode=native&alpn=h3&insecure=1#My%20TUIC"
	node, err := ParseNode(input, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if node.Tag == "" {
		node.Tag = "t"
	}
	out := buildOutbound(node)
	got, err := ShareURIFromOutbound(out)
	if err != nil {
		t.Fatalf("ShareURIFromOutbound: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("round-trip not parseable: %v", err)
	}
	assertEq(t, u.Scheme, "tuic")
	assertEq(t, u.Host, "tuic.example.test:8443")
	if u.User == nil || u.User.Username() != "uuid-9" {
		t.Errorf("uuid = %v, want uuid-9", u.User)
	}
	if pw, ok := u.User.Password(); !ok || pw != "passw0rd" {
		t.Errorf("password = %v ok=%v, want passw0rd", pw, ok)
	}
	q := u.Query()
	assertEq(t, q.Get("congestion_control"), "bbr")
	assertEq(t, q.Get("udp_relay_mode"), "native")
	assertEq(t, q.Get("alpn"), "h3")
	assertEq(t, q.Get("insecure"), "1")
	frag, _ := url.PathUnescape(u.Fragment)
	assertEq(t, frag, "My TUIC")
}

func TestShareURIFromTuic_MissingFields(t *testing.T) {
	_, err := ShareURIFromOutbound(map[string]interface{}{"type": "tuic", "uuid": "x"})
	if err == nil {
		t.Error("expected ErrShareURINotSupported for tuic missing password/server/port")
	}
}
