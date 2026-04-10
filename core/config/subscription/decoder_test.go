package subscription

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNormalizeSubscriptionTextLine(t *testing.T) {
	in := "  vless://x@y?a=1&amp;b=2  "
	got := NormalizeSubscriptionTextLine(in)
	want := "vless://x@y?a=1&b=2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if NormalizeSubscriptionTextLine("") != "" {
		t.Fatal("empty in should be empty out")
	}
}

// TestDecodeSubscriptionContent tests the DecodeSubscriptionContent function
func TestDecodeSubscriptionContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		expectError bool
		checkResult func(*testing.T, []byte)
	}{
		{
			name:        "Base64 URL encoded content",
			content:     []byte(base64.URLEncoding.EncodeToString([]byte("vless://test\nvmess://test"))),
			expectError: false,
			checkResult: func(t *testing.T, decoded []byte) {
				if !strings.Contains(string(decoded), "vless://test") {
					t.Error("Expected decoded content to contain 'vless://test'")
				}
			},
		},
		{
			name:        "Base64 standard encoded content",
			content:     []byte(base64.StdEncoding.EncodeToString([]byte("vless://test\nvmess://test"))),
			expectError: false,
			checkResult: func(t *testing.T, decoded []byte) {
				if !strings.Contains(string(decoded), "vless://test") {
					t.Error("Expected decoded content to contain 'vless://test'")
				}
			},
		},
		{
			name:        "Plain text content",
			content:     []byte("vless://test\nvmess://test"),
			expectError: false,
			checkResult: func(t *testing.T, decoded []byte) {
				if !strings.Contains(string(decoded), "vless://test") {
					t.Error("Expected decoded content to contain 'vless://test'")
				}
			},
		},
		{
			name:        "Empty content",
			content:     []byte(""),
			expectError: true,
		},
		{
			name:        "Whitespace only",
			content:     []byte("   \n\t  "),
			expectError: false,
			checkResult: func(t *testing.T, decoded []byte) {
				if len(decoded) == 0 {
					t.Error("Expected decoded content to be returned even if whitespace")
				}
			},
		},
		{
			name:        "JSON array subscription (Xray-style)",
			content:     []byte(`[ {"remarks":"a","outbounds":[]} ]`),
			expectError: false,
			checkResult: func(t *testing.T, decoded []byte) {
				if !strings.HasPrefix(string(decoded), "[") {
					t.Errorf("expected JSON array, got %q", string(decoded))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := DecodeSubscriptionContent(tt.content)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, decoded)
			}
		})
	}
}
