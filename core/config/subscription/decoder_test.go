package subscription

import (
	"encoding/base64"
	"strings"
	"testing"
)

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
