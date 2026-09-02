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
		"&address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8db9%3A%3A%2F128&profile=cloudflare&vhttp=h3" +
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
	// auto — ядро с lx.27 (h3 с откатом на h2): не деградируется
	n4, _ := ParseNode(base+"&vhttp=auto#x", nil)
	assertEq(t, n4.Outbound["vhttp"], "auto")
	if len(n4.Warnings) != 0 {
		t.Errorf("vhttp=auto must not warn, got %v", n4.Warnings)
	}
}

// Legacy-алиасы ?network= / ?server_name= больше не читаются (0.8.0, D-078):
// игнорируются как любой неизвестный query-параметр. Фикстура корпуса —
// masque/legacy_params_ignored.
func TestMasque_LegacyParamsIgnored(t *testing.T) {
	base := "masque://" + masqueTestPriv + "@1.2.3.4:443?publickey=" + urlEncode(masqueTestPub) + "&address=172.16.0.2%2F32"
	n, _ := ParseNode(base+"&network=h2&server_name=legacy.example#x", nil)
	assertEq(t, n.Outbound["vhttp"], "h3") // network=h2 игнорируется → parse-дефолт
	if _, flat := n.Outbound["network"]; flat {
		t.Error("ignored network must not be carried through")
	}
	if _, has := n.Outbound["tls"]; has {
		t.Errorf("server_name= must be ignored, got tls block: %v", n.Outbound["tls"])
	}
}

func TestMasque_ShareURIRoundTrip(t *testing.T) {
	uri := "masque://" + masqueTestPriv + "@162.159.198.1:443?publickey=" + urlEncode(masqueTestPub) +
		"&address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8db9%3A%3A%2F128&vhttp=h3&sni=x.example#node"
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

// Плоские network/sni/skip_cert_verify sing-box-импорта СТРИПАЮТСЯ без
// переноса значений (0.8.0, D-078). Удаление обязательно: плоский sni рядом
// с tls.server_name — два источника имени, ядро падает fail-fast'ом
// (SPEC 062 §2); а legacy-значения больше не читаются.
func TestSanitizeSingboxOutboundMap_MasqueLegacyStripped(t *testing.T) {
	ob := map[string]interface{}{
		"type":             "masque",
		"network":          "h2",
		"sni":              "x.example",
		"skip_cert_verify": true,
	}
	SanitizeSingboxOutboundMap(ob, "imported")

	for _, k := range []string{"network", "sni", "skip_cert_verify"} {
		if _, has := ob[k]; has {
			t.Errorf("legacy flat %q must be removed", k)
		}
	}
	if _, has := ob["vhttp"]; has {
		t.Errorf("network value must NOT fold into vhttp anymore, got %v", ob["vhttp"])
	}
	if _, has := ob["tls"]; has {
		t.Errorf("flat sni/skip_cert_verify must NOT fold into tls anymore, got %v", ob["tls"])
	}
}

// Канонические поля стрип не задевает.
func TestSanitizeSingboxOutboundMap_MasqueCanonicalSurvives(t *testing.T) {
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

// Метка узла НИКОГДА не берётся из userinfo: там лежат учётные данные
// (vless/tuic — UUID, wireguard/masque — приватный ключ, ss/trojan — пароль).
// Прежде URI без `#fragment` получал в Label собственный секрет, а имя узла
// едет в UI, логи и бэкап. Ожидание: Label пуст, Tag — scheme-server-port.
func TestNodeLabel_NeverFromUserinfo(t *testing.T) {
	cases := map[string]string{
		"vless uuid":     "vless://11111111-1111-1111-1111-111111111111@example-1.com:443?security=tls",
		"tuic uuid":      "tuic://u:p@example-1.com:443",
		"trojan pass":    "trojan://secretpass@example-1.com:443",
		"masque privkey": "masque://" + masqueTestPriv + "@192.0.2.44:443?publickey=" + urlEncode(masqueTestPub) + "&address=172.16.0.2%2F32",
	}
	for name, uri := range cases {
		t.Run(name, func(t *testing.T) {
			node, err := ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("parse: %v", err)
			}
			if node.Label != "" {
				t.Errorf("Label = %q — учётные данные утекли в имя узла", node.Label)
			}
			if strings.Contains(node.Tag, "11111111") || strings.Contains(node.Tag, "secretpass") ||
				strings.Contains(node.Tag, masqueTestPriv) {
				t.Errorf("Tag = %q — учётные данные утекли в тег", node.Tag)
			}
		})
	}
}
