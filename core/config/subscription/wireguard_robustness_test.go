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
	node, err := parseWireGuardURI(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	if got := node.Outbound["private_key"]; got != rawKey {
		t.Errorf("private_key = %v, want %q (slashes preserved)", got, rawKey)
	}

	// An already-encoded key (%2F) must still round-trip to the same raw key.
	uriEnc := "wireguard://JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL%2FGzdTb4unw9%2F4=@1.2.3.4:51820?publickey=" +
		wgTestPub + "&address=10.0.0.2/32&allowedips=0.0.0.0/0#node"
	node2, err := parseWireGuardURI(uriEnc, nil)
	if err != nil || node2 == nil {
		t.Fatalf("encoded parse failed: err=%v node=%v", err, node2)
	}
	if got := node2.Outbound["private_key"]; got != rawKey {
		t.Errorf("encoded private_key = %v, want %q (no double-encoding)", got, rawKey)
	}
}

// A key that is not a 32-byte base64 value must reject the node at parse time:
// emitted as-is it fails `sing-box check` and kills the whole config. Seen in
// the wild as Proton's masked "PrivateKey = *****" placeholder pasted from the
// web UI without revealing the key.
func TestParseWireGuardURI_InvalidKeysRejected(t *testing.T) {
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
			if node, err := parseWireGuardURI(uri, nil); err == nil {
				t.Errorf("expected error, got node %v", node)
			}
		})
	}
}

// URL-safe base64 keys (seen in some exports) are converted to the std form the
// core requires, not passed through verbatim.
func TestParseWireGuardURI_URLSafeKeyNormalized(t *testing.T) {
	// std "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL/GzdTb4unw9/4=" in URL-safe alphabet:
	urlSafe := "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL_GzdTb4unw9_4="
	uri := "wireguard://" + urlSafe + "@1.2.3.4:51820?publickey=" + wgTestPub +
		"&address=10.0.0.2/32&allowedips=0.0.0.0/0#node"
	node, err := parseWireGuardURI(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("parse failed: err=%v node=%v", err, node)
	}
	want := "JSwzOkFIT1ZdZGtyeYCHjpWco6qxuL/GzdTb4unw9/4="
	if got := node.Outbound["private_key"]; got != want {
		t.Errorf("private_key = %v, want std-base64 %q", got, want)
	}
}

// Bug B (v1.1.1): a bare address/allowed_ip (no /N), common in AmneziaWG/.conf
// exports, made sing-box fail at load — `netip.ParsePrefix("172.16.0.2"): no '/'`.
// The parser now defaults a bare IP to /32 (IPv4) or /128 (IPv6).
func TestParseWireGuardURI_BareAddressGetsPrefix(t *testing.T) {
	cases := []struct {
		name, address, allowedips string
		wantAddr, wantAllowed     []string
	}{
		{"bare ipv4 address", "172.16.0.2", "0.0.0.0/0", []string{"172.16.0.2/32"}, []string{"0.0.0.0/0"}},
		{"bare ipv6 address", "fd00::2", "::/0", []string{"fd00::2/128"}, []string{"::/0"}},
		{"bare allowed ip", "10.0.0.2/32", "10.0.0.5", []string{"10.0.0.2/32"}, []string{"10.0.0.5/32"}},
		{"already-prefixed unchanged", "10.0.0.2/32", "0.0.0.0/0,::/0", []string{"10.0.0.2/32"}, []string{"0.0.0.0/0", "::/0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uri := "wireguard://" + wgTestPub + "@1.2.3.4:51820?publickey=" + wgTestPub +
				"&address=" + c.address + "&allowedips=" + c.allowedips + "#n"
			node, err := parseWireGuardURI(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("parse failed: err=%v node=%v", err, node)
			}
			gotAddr, _ := node.Outbound["address"].([]string)
			if !eqStrs(gotAddr, c.wantAddr) {
				t.Errorf("address = %v, want %v", gotAddr, c.wantAddr)
			}
			peers, _ := node.Outbound["peers"].([]map[string]interface{})
			if len(peers) != 1 {
				t.Fatalf("want 1 peer, got %d", len(peers))
			}
			gotAllowed, _ := peers[0]["allowed_ips"].([]string)
			if !eqStrs(gotAllowed, c.wantAllowed) {
				t.Errorf("allowed_ips = %v, want %v", gotAllowed, c.wantAllowed)
			}
		})
	}
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
