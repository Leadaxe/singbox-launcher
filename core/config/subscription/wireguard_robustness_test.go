package subscription

import "testing"

const wgTestPub = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU="

// Bug A (v1.1.1): a base64 private key with a raw '/' broke url.Parse — the '/'
// was read as the start of the path, the userinfo was dropped, and the node
// failed with "missing private key" (it was added to Sources but vanished from
// Preview). The parser now percent-encodes a raw '/' in the userinfo first.
func TestParseWireGuardURI_SlashInPrivateKey(t *testing.T) {
	rawKey := "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL/GzdTb4unw9/4=" // two raw slashes
	uri := "wireguard://" + rawKey + "@1.2.3.4:51820?publickey=" + wgTestPub +
		"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	if got := node.Outbound["private_key"]; got != rawKey {
		t.Errorf("private_key = %v, want %q (slashes preserved)", got, rawKey)
	}

	// An already-encoded key (%2F) must still round-trip to the same raw key.
	uriEnc := "wireguard://JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL%2FGzdTb4unw9%2F4=@1.2.3.4:51820?publickey=" +
		wgTestPub + "&address=10.0.0.2/32&allowedips=0.0.0.0/0#node"
	node2, err := ParseNode(uriEnc, nil)
	if err != nil || node2 == nil {
		t.Fatalf("encoded parse failed: err=%v node=%v", err, node2)
	}
	if got := node2.Outbound["private_key"]; got != rawKey {
		t.Errorf("encoded private_key = %v, want %q (no double-encoding)", got, rawKey)
	}
}

// Негодный ключ (не 32 байта base64) роняет УЗЕЛ — но роняет его РЕЕСТР, а не
// парсер (контракт 1.1.11, решение владельца 19.09.2026): маппер переносит
// значение как есть, а wireguard.body.private_key / peers[].public_key /
// peers[].pre_shared_key снимают его с кодом wg_key_invalid. Пока проверка
// стояла здесь, узел пропадал МОЛЧА, и то же значение телом sing-box проверок
// не проходило вовсе (находка №4 LEGACY_AUDIT).
//
// Исход проверяется кейсами корпуса — uri/wireguard/{masked_private_key_rejected,
// short_private_key_rejected, junk_publickey_rejected, junk_presharedkey_rejected,
// wg_private_key_31_bytes, wg_private_key_33_bytes, wg_public_key_31_bytes,
// wg_psk_33_bytes, wg_psk_not_base64} и парными телами
// body/singbox/{endpoints_wg_private_key_31_bytes, endpoints_wg_psk_junk}.
// Здесь остаётся ровно обязанность МАППЕРА: не решать судьбу узла самому.
func TestParseWireGuardURI_InvalidKeysReachTheRegistry(t *testing.T) {
	cases := map[string]string{
		"masked private key": "wireguard://*****@1.2.3.4:51820?publickey=" + wgTestPub +
			"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node",
		"short private key": "wireguard://c2hvcnQ=@1.2.3.4:51820?publickey=" + wgTestPub +
			"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node",
		"junk publickey": "wireguard://" + wgTestPub + "@1.2.3.4:51820?publickey=enabled" +
			"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node",
		"junk presharedkey": "wireguard://" + wgTestPub + "@1.2.3.4:51820?publickey=" + wgTestPub +
			"&presharedkey=*****&address=10.0.0.2/32&allowedips=0.0.0.0/0#node",
	}
	for name, uri := range cases {
		t.Run(name, func(t *testing.T) {
			node, err := ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("маппер сам отверг узел (err=%v) — судить ключ обязан реестр", err)
			}
		})
	}
}

// URL-safe base64 keys (seen in some exports) are converted to the std form the
// core requires — но делает это САНИТАЙЗЕР по правилу тела
// (body.private_key, normalize base64_std), а не разбор ссылки: перевод
// написания правит ЗНАЧЕНИЕ и обязан работать на всех входах, включая тело
// sing-box (D133-22). Здесь, на выходе маппера, ключ ещё в том написании,
// в каком его прислали, и это нормально.
//
// Сквозную проверку несёт корпус: uri/wireguard/urlsafe_key_normalized
// ждёт в теле ровно std-base64 с паддингом.
func TestParseWireGuardURI_URLSafeKeyAccepted(t *testing.T) {
	// std "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL/GzdTb4unw9/4=" в url-safe алфавите:
	urlSafe := "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL_GzdTb4unw9_4="
	uri := "wireguard://" + urlSafe + "@1.2.3.4:51820?publickey=" + wgTestPub +
		"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	// Ключ доехал целиком и не потерялся на разборе authority — это и есть
	// предмет теста; канонизацию написания проверяет корпус.
	if got, _ := node.Outbound["private_key"].(string); got != urlSafe {
		t.Errorf("private_key = %q, ожидался доехавший целиком ключ %q", got, urlSafe)
	}
}

// Bug B (v1.1.1): голый адрес без /N (обычное дело у экспортов
// AmneziaWG/.conf) валил загрузку sing-box — `netip.ParsePrefix("172.16.0.2"):
// no '/'`. Достраивает маску ТЕЛО (normalize cidr_prefix у address и
// peers[].allowed_ips), а не разбор ссылки: это правило ЗНАЧЕНИЯ и работает на
// всех входах, включая тело sing-box и .conf (D133-22).
//
// Здесь, на выходе маппера, проверяется только то, за что отвечает маппер:
// список разрезан по запятой и доехал целиком, ни один элемент не потерян.
// Саму достройку маски проверяет корпус — uri/wireguard/bare_ip_to_cidr,
// bare_ipv4_address_prefix, bare_ipv6_address_prefix, bare_allowed_ip_prefix.
func TestParseWireGuardURI_BareAddressListSurvives(t *testing.T) {
	cases := []struct {
		name, address, allowedips string
		wantAddr, wantAllowed     []string
	}{
		{"bare ipv4 address", "172.16.0.2", "0.0.0.0/0", []string{"172.16.0.2"}, []string{"0.0.0.0/0"}},
		{"bare ipv6 address", "fd00::2", "::/0", []string{"fd00::2"}, []string{"::/0"}},
		{"bare allowed ip", "10.0.0.2/32", "10.0.0.5", []string{"10.0.0.2/32"}, []string{"10.0.0.5"}},
		{"список из двух", "10.0.0.2/32", "0.0.0.0/0,::/0", []string{"10.0.0.2/32"}, []string{"0.0.0.0/0", "::/0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uri := "wireguard://" + wgTestPub + "@1.2.3.4:51820?publickey=" + wgTestPub +
				"&address=" + c.address + "&allowedips=" + c.allowedips + "#n"
			node, err := ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("parse failed: err=%v node=%v", err, node)
			}
			if got := strList(node.Outbound["address"]); !eqStrs(got, c.wantAddr) {
				t.Errorf("address = %v, want %v", got, c.wantAddr)
			}
			peers, _ := wireGuardPeerMaps(node.Outbound)
			if len(peers) != 1 {
				t.Fatalf("want 1 peer, got %d", len(peers))
			}
			if got := strList(peers[0]["allowed_ips"]); !eqStrs(got, c.wantAllowed) {
				t.Errorf("allowed_ips = %v, want %v", got, c.wantAllowed)
			}
		})
	}
}

// strList — список строк тела в любой из форм, в которых он приезжает:
// []interface{} от движка (и после round-trip через JSON) либо []string.
func strList(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
