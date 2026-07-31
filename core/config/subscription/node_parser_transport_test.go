package subscription

import "testing"

func TestNormalizeRealityShortID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"48720c", "48720c"},
		{" 9083951b754b4254 ", "9083951b754b4254"},
		{"ABCDEF01", "abcdef01"},
		{"48\xC2\xA7ab", "48ab"},                         // § (UTF-8) between hex — strip non-hex
		{"\xC2\xA0", ""},                                 // NBSP only
		{"9083951b754b4254deadbeef", "9083951b754b4254"}, // truncate to 16 hex
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeRealityShortID(tt.in); got != tt.want {
			t.Errorf("normalizeRealityShortID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseNode_VLESS_RealityShortIDSanitized(t *testing.T) {
	// NBSP (U+00A0) inside sid — sing-box hex decode fails without sanitization
	uri := "vless://a1b2c3d4-e5f6-7890-abcd-ef1234567890@example.com:443?encryption=none&security=reality&type=tcp&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48%C2%A0ab12"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tls, ok := node.Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing tls")
	}
	rel, ok := tls["reality"].(map[string]interface{})
	if !ok {
		t.Fatal("missing reality")
	}
	if got, _ := rel["short_id"].(string); got != "48ab12" {
		t.Fatalf("short_id got %q want 48ab12", got)
	}
}

func TestIsValidRealityPublicKey(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg", true},   // base64url, 43 chars
		{" mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg ", true}, // surrounding space
		{"mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg=", true},  // stray pad
		{"enabled", false}, // junk from broken public lists
		{"true", false},
		{"", false},
		{"short", false},
		{"!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", false}, // 43 non-base64 chars
	}
	for _, tt := range tests {
		if got := isValidRealityPublicKey(tt.in); got != tt.want {
			t.Errorf("isValidRealityPublicKey(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeUTLSFingerprint(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// sing-box names pass through, lowercased (issue #45).
		{"chrome", "chrome"},
		{"QQ", "qq"},
		{" Firefox ", "firefox"},
		{"randomized", "randomized"},
		{"android", "android"},
		{"360", "360"},
		// Chrome ClientHello variants sing-box accepts verbatim.
		{"chrome_psk_shuffle", "chrome_psk_shuffle"},
		{"chrome_pq", "chrome_pq"},
		// Raw uTLS ClientHelloID identifiers → browser family.
		{"HelloChrome_120", "chrome"},
		{"hellochrome_auto", "chrome"},
		{"HelloChrome-106", "chrome"},
		{"HelloFirefox_Auto", "firefox"},
		{"HelloSafari_16_0", "safari"},
		{"HelloIOS_14", "ios"},
		{"HelloEdge_85", "edge"},
		{"HelloAndroid_11_OkHttp", "android"},
		{"HelloRandomized", "randomized"}, // must not match the "hellorandom" prefix
		{"HelloRandom", "random"},
		// Junk from broken lists — dropped, not passed through.
		{"enabled", ""},
		{"true", ""},
		{"HelloGolang", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := NormalizeUTLSFingerprint(tt.in); got != tt.want {
			t.Errorf("NormalizeUTLSFingerprint(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Regression: a node carrying a raw uTLS identifier (fp=HelloChrome_120) made
// sing-box abort the entire config with "initialize outbound[N]: unknown uTLS
// fingerprint", so no node started. It must map onto the chrome family.
func TestParseNode_VLESS_RawUTLSIdentifierFingerprint(t *testing.T) {
	uri := "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@example.com:443?encryption=none&security=tls&sni=example.com&fp=HelloChrome_120#t"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tls, ok := node.Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing tls")
	}
	ut, ok := tls["utls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing utls")
	}
	if ut["fingerprint"] != "chrome" {
		t.Fatalf("fingerprint = %#v, want chrome", ut["fingerprint"])
	}
}

// A fingerprint sing-box cannot map at all must not reach the config: the node
// falls back to a valid name rather than emitting the junk value.
func TestParseNode_VLESS_JunkFingerprintDropped(t *testing.T) {
	uri := "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@example.com:443?encryption=none&security=tls&sni=example.com&fp=enabled#t"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tls := node.Outbound["tls"].(map[string]interface{})
	ut, ok := tls["utls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing utls")
	}
	if got := ut["fingerprint"]; got != "random" {
		t.Fatalf("fingerprint = %#v, want random fallback", got)
	}
}

// Regression: a broken public list attaches pbk=enabled to a plain security=tls
// node. Emitting that as reality.public_key made sing-box reject the entire
// config ("initialize outbound[N]: invalid public_key") so the VPN never started.
// The node must degrade to plain TLS, with no reality block.
func TestParseNode_VLESS_JunkPbkOnTLSNode(t *testing.T) {
	uri := "vless://35aead44-22e1-d2ef-01a8-ab8c508222ec@172.67.204.176:2053?security=tls&sni=ez.example.workers.dev&type=ws&host=ez.example.workers.dev&path=%2Fsync&fp=chrome&pbk=enabled&allowInsecure=0"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tls, ok := node.Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("expected plain TLS to remain enabled")
	}
	if _, hasReality := tls["reality"]; hasReality {
		t.Fatalf("junk pbk=enabled must NOT produce a reality block; tls=%v", tls)
	}
}
