package subscription

import "testing"

func TestNormalizeRealityShortID(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"48720c", "48720c"},
		{" 9083951b754b4254 ", "9083951b754b4254"},
		{"ABCDEF01", "abcdef01"},
		{"48\xC2\xA7ab", "48ab"}, // § (UTF-8) between hex — strip non-hex
		{"\xC2\xA0", ""},        // NBSP only
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
