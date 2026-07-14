package subscription

import (
	"net/url"
	"strings"
	"testing"
)

func urlEncode(s string) string { return url.QueryEscape(s) }

// Real P-256 keys (SEC1 DER private / PKIX DER public, base64) — test fixtures,
// not a live account.
const (
	masqueTestPriv = "MHcCAQEEIB5oxGzgOdLvTY2aAbRsyJslxnlvPpOzLR076h3cgsncoAoGCCqGSM49AwEHoUQDQgAEDQBTbtpEikpJDklVHdnMhgIR8YatYDJLUILDQWGdwBbqaLiKKiuawVQz6MIaHr0I/4mNM/TfUUnoENKv9qZEWw=="
	masqueTestPub  = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEDQBTbtpEikpJDklVHdnMhgIR8YatYDJLUILDQWGdwBbqaLiKKiuawVQz6MIaHr0I/4mNM/TfUUnoENKv9qZEWw=="
)

func TestParseNode_Masque_Canonical(t *testing.T) {
	uri := "masque://" + masqueTestPriv + "@162.159.198.1:443?publickey=" + urlEncode(masqueTestPub) +
		"&address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8db9%3A%3A%2F128&profile=cloudflare&network=h3" +
		"&sni=consumer-masque.cloudflareclient.com&mtu=1280&idle_timeout=5m#MASQUE-smoke"

	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode error: %v", err)
	}
	ob := node.Outbound
	assertEq(t, ob["type"], "masque")
	assertEq(t, ob["server"], "162.159.198.1")
	assertEq(t, ob["server_port"], 443)
	assertEq(t, ob["profile"], "cloudflare")
	assertEq(t, ob["network"], "h3")
	assertEq(t, ob["private_key"], masqueTestPriv)
	assertEq(t, ob["public_key"], masqueTestPub)
	assertEq(t, ob["ip"], "172.16.0.2/32")
	assertEq(t, ob["ipv6"], "2606:4700:110:8db9::/128")
	assertEq(t, ob["sni"], "consumer-masque.cloudflareclient.com")
	assertEq(t, ob["idle_timeout"], "5m")
}

func TestMasque_MissingKeysRejected(t *testing.T) {
	// no publickey
	if _, err := ParseNode("masque://"+masqueTestPriv+"@1.2.3.4:443?address=172.16.0.2%2F32#x", nil); err == nil {
		t.Fatal("expected error without publickey")
	}
	// no address
	if _, err := ParseNode("masque://"+masqueTestPriv+"@1.2.3.4:443?publickey="+urlEncode(masqueTestPub)+"#x", nil); err == nil {
		t.Fatal("expected error without address")
	}
}

func TestMasque_NetworkDefaultAndValidation(t *testing.T) {
	base := "masque://" + masqueTestPriv + "@1.2.3.4:443?publickey=" + urlEncode(masqueTestPub) + "&address=172.16.0.2%2F32"
	// default h3
	n, _ := ParseNode(base+"#x", nil)
	assertEq(t, n.Outbound["network"], "h3")
	// invalid network forced to h3
	n2, _ := ParseNode(base+"&network=tcp#x", nil)
	assertEq(t, n2.Outbound["network"], "h3")
	// h2 honored
	n3, _ := ParseNode(base+"&network=h2#x", nil)
	assertEq(t, n3.Outbound["network"], "h2")
}

func TestMasque_ShareURIRoundTrip(t *testing.T) {
	uri := "masque://" + masqueTestPriv + "@162.159.198.1:443?publickey=" + urlEncode(masqueTestPub) +
		"&address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8db9%3A%3A%2F128&network=h3&sni=x.example#node"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse: %v", err)
	}
	share, err := ShareURIFromOutbound(node.Outbound)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if !strings.HasPrefix(share, "masque://") {
		t.Fatalf("bad share scheme: %s", share)
	}
	node2, err := ParseNode(share, nil)
	if err != nil || node2 == nil {
		t.Fatalf("reparse %q: %v", share, err)
	}
	for _, k := range []string{"type", "server", "server_port", "private_key", "public_key", "ip", "ipv6", "network", "sni"} {
		if node.Outbound[k] != node2.Outbound[k] {
			t.Errorf("round-trip %s: %v != %v", k, node.Outbound[k], node2.Outbound[k])
		}
	}
}
