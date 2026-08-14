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
	assertEq(t, ob["vhttp"], "h3")
	assertEq(t, ob["private_key"], masqueTestPriv)
	assertEq(t, ob["public_key"], masqueTestPub)
	assertEq(t, ob["ip"], "172.16.0.2/32")
	assertEq(t, ob["ipv6"], "2606:4700:110:8db9::/128")
	if _, flat := ob["sni"]; flat {
		t.Error("flat sni must not survive: the core deprecated it (SPEC 062)")
	}
	tls, ok := ob["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls block missing: %v", ob["tls"])
	}
	assertEq(t, tls["server_name"], "consumer-masque.cloudflareclient.com")
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

// Legacy `network=` is still accepted on input (foreign subscriptions ship it)
// and normalized to `vhttp`.
func TestMasque_VHTTPDefaultAndValidation(t *testing.T) {
	base := "masque://" + masqueTestPriv + "@1.2.3.4:443?publickey=" + urlEncode(masqueTestPub) + "&address=172.16.0.2%2F32"
	// default h3
	n, _ := ParseNode(base+"#x", nil)
	assertEq(t, n.Outbound["vhttp"], "h3")
	// invalid value forced to h3
	n2, _ := ParseNode(base+"&vhttp=tcp#x", nil)
	assertEq(t, n2.Outbound["vhttp"], "h3")
	// h2 honored
	n3, _ := ParseNode(base+"&vhttp=h2#x", nil)
	assertEq(t, n3.Outbound["vhttp"], "h2")
	// legacy spelling still understood
	n4, _ := ParseNode(base+"&network=h2#x", nil)
	assertEq(t, n4.Outbound["vhttp"], "h2")
	if _, flat := n4.Outbound["network"]; flat {
		t.Error("legacy network must be normalized away, not carried through")
	}
	// vhttp wins over a contradicting legacy network
	n5, _ := ParseNode(base+"&vhttp=h3&network=h2#x", nil)
	assertEq(t, n5.Outbound["vhttp"], "h3")
}

func TestMasque_ShareURIRoundTrip(t *testing.T) {
	uri := "masque://" + masqueTestPriv + "@162.159.198.1:443?publickey=" + urlEncode(masqueTestPub) +
		"&address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8db9%3A%3A%2F128&network=h3&sni=x.example#node"
	// note: the source URI uses the legacy spellings on purpose — the round-trip
	// must land on the current schema and stay stable across a second parse.
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
	for _, k := range []string{"type", "server", "server_port", "private_key", "public_key", "ip", "ipv6", "vhttp"} {
		if node.Outbound[k] != node2.Outbound[k] {
			t.Errorf("round-trip %s: %v != %v", k, node.Outbound[k], node2.Outbound[k])
		}
	}
	tls1, _ := node.Outbound["tls"].(map[string]interface{})
	tls2, _ := node2.Outbound["tls"].(map[string]interface{})
	if tls1 == nil || tls2 == nil || tls1["server_name"] != tls2["server_name"] {
		t.Errorf("round-trip tls.server_name: %v != %v", tls1, tls2)
	}
}

// Импортированный masque из чужого/старого конфига несёт плоские network/sni.
// Ядро принимает их до lx.30, но плоский sni рядом с tls.server_name — два
// источника имени, и при расхождении ядро падает fail-fast'ом (SPEC 062 §2).
func TestSanitizeSingboxOutboundMap_MasqueLegacyFolded(t *testing.T) {
	ob := map[string]interface{}{
		"type":             "masque",
		"network":          "h2",
		"sni":              "x.example",
		"skip_cert_verify": true,
	}
	SanitizeSingboxOutboundMap(ob, "imported")

	assertEq(t, ob["vhttp"], "h2")
	if _, has := ob["network"]; has {
		t.Error("legacy network must be removed")
	}
	if _, has := ob["sni"]; has {
		t.Error("flat sni must be removed")
	}
	if _, has := ob["skip_cert_verify"]; has {
		t.Error("flat skip_cert_verify must be removed")
	}
	tls, ok := ob["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("tls block missing: %v", ob["tls"])
	}
	assertEq(t, tls["server_name"], "x.example")
	assertEq(t, tls["insecure"], true)
}

// Новое имя выигрывает: legacy-дубликат снимается, а не перетирает его.
func TestSanitizeSingboxOutboundMap_MasqueNewWins(t *testing.T) {
	ob := map[string]interface{}{
		"type":    "masque",
		"vhttp":   "h3",
		"network": "h2",
		"sni":     "legacy.example",
		"tls":     map[string]interface{}{"server_name": "current.example"},
	}
	SanitizeSingboxOutboundMap(ob, "imported")

	assertEq(t, ob["vhttp"], "h3")
	tls, _ := ob["tls"].(map[string]interface{})
	assertEq(t, tls["server_name"], "current.example")
}

// masque идёт поверх QUIC — utls/reality ядро для него игнорирует, снимаем.
func TestSanitizeSingboxOutboundMap_MasqueStripsUTLS(t *testing.T) {
	ob := map[string]interface{}{
		"type": "masque",
		"tls": map[string]interface{}{
			"server_name": "x.example",
			"utls":        map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
		},
	}
	SanitizeSingboxOutboundMap(ob, "imported")

	tls, _ := ob["tls"].(map[string]interface{})
	if _, has := tls["utls"]; has {
		t.Error("utls must be stripped on QUIC-based masque")
	}
}
