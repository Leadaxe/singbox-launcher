package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestNormalizeParserConfig tests the NormalizeParserConfig function
func TestNormalizeParserConfig(t *testing.T) {
	t.Run("Version 1 to version 2 migration", func(t *testing.T) {
		config := &ParserConfig{
			Version: 1,
			ParserConfig: struct {
				Version   int              `json:"version,omitempty"`
				Proxies   []ProxySource    `json:"proxies"`
				Outbounds []OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{},
		}
		NormalizeParserConfig(config, false)
		if config.Version != 0 {
			t.Errorf("Expected Version to be 0 after migration, got %d", config.Version)
		}
		if config.ParserConfig.Version != ParserConfigVersion {
			t.Errorf("Expected ParserConfig.Version to be %d, got %d", ParserConfigVersion, config.ParserConfig.Version)
		}
	})

	t.Run("Set default reload", func(t *testing.T) {
		config := &ParserConfig{
			ParserConfig: struct {
				Version   int              `json:"version,omitempty"`
				Proxies   []ProxySource    `json:"proxies"`
				Outbounds []OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{},
		}
		NormalizeParserConfig(config, false)
		if config.ParserConfig.Parser.Reload != "4h" {
			t.Errorf("Expected default reload '4h', got '%s'", config.ParserConfig.Parser.Reload)
		}
	})

	t.Run("Update last_updated", func(t *testing.T) {
		config := &ParserConfig{
			ParserConfig: struct {
				Version   int              `json:"version,omitempty"`
				Proxies   []ProxySource    `json:"proxies"`
				Outbounds []OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{},
		}
		before := time.Now()
		NormalizeParserConfig(config, true)
		after := time.Now()
		if config.ParserConfig.Parser.LastUpdated == "" {
			t.Error("Expected last_updated to be set")
		}
		parsedTime, err := time.Parse(time.RFC3339, config.ParserConfig.Parser.LastUpdated)
		if err != nil {
			t.Fatalf("Failed to parse last_updated: %v", err)
		}
		if parsedTime.Before(before) || parsedTime.After(after) {
			t.Errorf("last_updated time %v is not within expected range [%v, %v]", parsedTime, before, after)
		}
	})

	t.Run("Nil config", func(t *testing.T) {
		NormalizeParserConfig(nil, false)
		// Should not panic
	})
}

// TestExtractParserConfig tests the ExtractParserConfig function
func TestExtractParserConfig(t *testing.T) {
	// Create a temporary config file with @ParserConfig block
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	parserConfigJSON := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [
      {
        "source": "https://example.com/subscription",
        "connections": []
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector"
      }
    ],
    "parser": {
      "reload": "4h"
    }
  }
}`

	configContent := `{
  "log": {},
  "inbounds": [],
  "outbounds": [],
  /** @ParserConfig
` + parserConfigJSON + `
*/
  "route": {}
}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	t.Run("Extract valid ParserConfig", func(t *testing.T) {
		config, err := ExtractParserConfig(configPath)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if config == nil {
			t.Fatal("Expected config, got nil")
		}
		if config.ParserConfig.Version != 2 {
			t.Errorf("Expected version 2, got %d", config.ParserConfig.Version)
		}
		if len(config.ParserConfig.Proxies) != 1 {
			t.Errorf("Expected 1 proxy source, got %d", len(config.ParserConfig.Proxies))
		}
		if config.ParserConfig.Proxies[0].Source != "https://example.com/subscription" {
			t.Errorf("Expected source 'https://example.com/subscription', got '%s'", config.ParserConfig.Proxies[0].Source)
		}
	})

	t.Run("Config file not found", func(t *testing.T) {
		_, err := ExtractParserConfig("/nonexistent/config.json")
		if err == nil {
			t.Error("Expected error for nonexistent file, got nil")
		}
	})

	t.Run("Config without @ParserConfig block", func(t *testing.T) {
		invalidConfigPath := filepath.Join(tempDir, "invalid_config.json")
		invalidContent := `{
  "log": {},
  "inbounds": [],
  "outbounds": []
}`
		if err := os.WriteFile(invalidConfigPath, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to create invalid config file: %v", err)
		}
		_, err := ExtractParserConfig(invalidConfigPath)
		if err == nil {
			t.Error("Expected error for config without @ParserConfig block, got nil")
		}
	})
}

// TestUpdateLastUpdatedInConfig tests the UpdateLastUpdatedInConfig function
func TestUpdateLastUpdatedInConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	parserConfigJSON := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [],
    "outbounds": [],
    "parser": {
      "reload": "4h"
    }
  }
}`

	configContent := `{
  "log": {},
  /** @ParserConfig
` + parserConfigJSON + `
*/
  "outbounds": []
}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	t.Run("Update last_updated", func(t *testing.T) {
		updateTime := time.Now().UTC()
		err := UpdateLastUpdatedInConfig(configPath, updateTime)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Verify the update
		config, err := ExtractParserConfig(configPath)
		if err != nil {
			t.Fatalf("Failed to extract config after update: %v", err)
		}
		if config.ParserConfig.Parser.LastUpdated == "" {
			t.Error("Expected last_updated to be set")
		}
		parsedTime, err := time.Parse(time.RFC3339, config.ParserConfig.Parser.LastUpdated)
		if err != nil {
			t.Fatalf("Failed to parse last_updated: %v", err)
		}
		// Allow 1 second tolerance
		diff := parsedTime.Sub(updateTime)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("last_updated time %v differs from expected %v by more than 1 second", parsedTime, updateTime)
		}
	})

	t.Run("Update nonexistent config", func(t *testing.T) {
		err := UpdateLastUpdatedInConfig("/nonexistent/config.json", time.Now())
		if err == nil {
			t.Error("Expected error for nonexistent file, got nil")
		}
	})
}

// TestIsSubscriptionURL tests the IsSubscriptionURL function
func TestIsSubscriptionURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"HTTP URL", "http://example.com/subscription", true},
		{"HTTPS URL", "https://example.com/subscription", true},
		{"HTTPS URL with path", "https://example.com/path/to/subscription.txt", true},
		{"VLESS link", "vless://uuid@server:443", false},
		{"VMess link", "vmess://base64", false},
		{"Empty string", "", false},
		{"Whitespace HTTP", "  http://example.com  ", true},
		{"Invalid URL", "not-a-url", false},
		{"File path", "/path/to/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSubscriptionURL(tt.input)
			if result != tt.expected {
				t.Errorf("IsSubscriptionURL(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

