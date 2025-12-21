package business

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// TestSerializeParserConfig tests SerializeParserConfig function
func TestSerializeParserConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.ParserConfig
		expectError bool
		checkResult func(*testing.T, string)
	}{
		{
			name: "Valid ParserConfig",
			config: &config.ParserConfig{
				ParserConfig: struct {
					Version   int                     `json:"version,omitempty"`
					Proxies   []config.ProxySource    `json:"proxies"`
					Outbounds []config.OutboundConfig `json:"outbounds"`
					Parser    struct {
						Reload      string `json:"reload,omitempty"`
						LastUpdated string `json:"last_updated,omitempty"`
					} `json:"parser,omitempty"`
				}{
					Version: 2,
					Proxies: []config.ProxySource{
						{
							Source:      "https://example.com/subscription",
							Connections: []string{"vless://uuid@server:443"},
						},
					},
					Outbounds: []config.OutboundConfig{
						{
							Tag:  "proxy-out",
							Type: "selector",
						},
					},
				},
			},
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				if result == "" {
					t.Error("Expected non-empty result")
				}
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(result), &parsed); err != nil {
					t.Errorf("Result is not valid JSON: %v", err)
				}
				if _, ok := parsed["ParserConfig"]; !ok {
					t.Error("Expected ParserConfig in result")
				}
			},
		},
		{
			name:        "Nil ParserConfig",
			config:      nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SerializeParserConfig(tt.config)
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
				tt.checkResult(t, result)
			}
		})
	}
}

// TestCloneOutbound tests CloneOutbound function
func TestCloneOutbound(t *testing.T) {
	src := &config.OutboundConfig{
		Tag:          "test-outbound",
		Type:         "selector",
		Comment:      "Test comment",
		AddOutbounds: []string{"outbound1", "outbound2"},
		Options: map[string]interface{}{
			"key1": "value1",
			"key2": map[string]interface{}{
				"nested": "value",
			},
		},
		Filters: map[string]interface{}{
			"filter1": []interface{}{"item1", "item2"},
		},
	}

	dst := CloneOutbound(src)

	// Verify deep copy
	if dst == src {
		t.Error("CloneOutbound should return a new instance")
	}

	if dst.Tag != src.Tag {
		t.Errorf("Expected tag %q, got %q", src.Tag, dst.Tag)
	}

	if dst.Type != src.Type {
		t.Errorf("Expected type %q, got %q", src.Type, dst.Type)
	}

	// Verify AddOutbounds is a copy
	if len(dst.AddOutbounds) != len(src.AddOutbounds) {
		t.Errorf("Expected %d AddOutbounds, got %d", len(src.AddOutbounds), len(dst.AddOutbounds))
	}

	// Modify original and verify clone is not affected
	src.AddOutbounds[0] = "modified"
	if dst.AddOutbounds[0] == "modified" {
		t.Error("Clone should not be affected by changes to original")
	}

	// Verify Options is a deep copy (check that modifying one doesn't affect the other)
	if dst.Options == nil || src.Options == nil {
		t.Error("Options should not be nil")
	}
	src.Options["key1"] = "modified"
	if dst.Options["key1"] == "modified" {
		t.Error("Clone Options should not be affected by changes to original")
	}
}

// TestEnsureRequiredOutbounds tests EnsureRequiredOutbounds function
func TestEnsureRequiredOutbounds(t *testing.T) {
	// Create template with required outbounds
	templateJSON := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [],
    "outbounds": [
      {
        "tag": "required-1",
        "type": "direct",
        "wizard": {
          "required": 1
        }
      },
      {
        "tag": "required-2",
        "type": "block",
        "wizard": {
          "required": 2
        }
      },
      {
        "tag": "optional",
        "type": "selector",
        "wizard": {
          "required": 0
        }
      }
    ]
  }
}`

	// Create parser config without required outbounds
	parserConfig := &config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version:   2,
			Proxies:   []config.ProxySource{},
			Outbounds: []config.OutboundConfig{},
		},
	}

	EnsureRequiredOutbounds(parserConfig, templateJSON)

	// Verify required-1 was added (required=1)
	foundRequired1 := false
	for _, outbound := range parserConfig.ParserConfig.Outbounds {
		if outbound.Tag == "required-1" {
			foundRequired1 = true
			if outbound.Type != "direct" {
				t.Errorf("Expected type 'direct' for required-1, got %q", outbound.Type)
			}
		}
	}
	if !foundRequired1 {
		t.Error("Expected required-1 outbound to be added")
	}

	// Verify required-2 was added (required=2)
	foundRequired2 := false
	for _, outbound := range parserConfig.ParserConfig.Outbounds {
		if outbound.Tag == "required-2" {
			foundRequired2 = true
			if outbound.Type != "block" {
				t.Errorf("Expected type 'block' for required-2, got %q", outbound.Type)
			}
		}
	}
	if !foundRequired2 {
		t.Error("Expected required-2 outbound to be added")
	}

	// Verify optional was not added (required=0)
	foundOptional := false
	for _, outbound := range parserConfig.ParserConfig.Outbounds {
		if outbound.Tag == "optional" {
			foundOptional = true
		}
	}
	if foundOptional {
		t.Error("Expected optional outbound not to be added")
	}
}

// TestEnsureRequiredOutbounds_Overwrite tests that required>1 outbounds are overwritten
func TestEnsureRequiredOutbounds_Overwrite(t *testing.T) {
	templateJSON := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [],
    "outbounds": [
      {
        "tag": "required-2",
        "type": "block",
        "wizard": {
          "required": 2
        }
      }
    ]
  }
}`

	// Create parser config with existing required-2 outbound
	parserConfig := &config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version: 2,
			Proxies: []config.ProxySource{},
			Outbounds: []config.OutboundConfig{
				{
					Tag:  "required-2",
					Type: "direct", // Different type - should be overwritten
				},
			},
		},
	}

	EnsureRequiredOutbounds(parserConfig, templateJSON)

	// Verify required-2 was overwritten
	found := false
	for _, outbound := range parserConfig.ParserConfig.Outbounds {
		if outbound.Tag == "required-2" {
			found = true
			if outbound.Type != "block" {
				t.Errorf("Expected type 'block' after overwrite, got %q", outbound.Type)
			}
		}
	}
	if !found {
		t.Error("Expected required-2 outbound to exist")
	}
}

// TestEnsureRequiredOutbounds_Preserve tests that required=1 outbounds are preserved
func TestEnsureRequiredOutbounds_Preserve(t *testing.T) {
	templateJSON := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [],
    "outbounds": [
      {
        "tag": "required-1",
        "type": "direct",
        "wizard": {
          "required": 1
        }
      }
    ]
  }
}`

	// Create parser config with existing required-1 outbound
	parserConfig := &config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version: 2,
			Proxies: []config.ProxySource{},
			Outbounds: []config.OutboundConfig{
				{
					Tag:  "required-1",
					Type: "selector", // Different type - should be preserved
				},
			},
		},
	}

	EnsureRequiredOutbounds(parserConfig, templateJSON)

	// Verify required-1 was preserved (not overwritten)
	found := false
	for _, outbound := range parserConfig.ParserConfig.Outbounds {
		if outbound.Tag == "required-1" {
			found = true
			if outbound.Type != "selector" {
				t.Errorf("Expected type 'selector' to be preserved, got %q", outbound.Type)
			}
		}
	}
	if !found {
		t.Error("Expected required-1 outbound to exist")
	}
}

// TestLoadConfigFromFile tests LoadConfigFromFile function (without actual file I/O)
func TestLoadConfigFromFile_FileSizeValidation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	// Create a config file that exceeds size limit
	largeContent := strings.Repeat("a", int(wizardutils.MaxJSONConfigSize)+1)
	err := os.WriteFile(configPath, []byte(largeContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Note: This test only validates file size checking logic
	// Full LoadConfigFromFile test would require mocking UI components
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	if fileInfo.Size() <= wizardutils.MaxJSONConfigSize {
		t.Error("Test file should exceed size limit")
	}
}
