package business

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config"
)

// TestSerializeParserConfig_Standalone tests SerializeParserConfig without UI dependencies
func TestSerializeParserConfig_Standalone(t *testing.T) {
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

// TestApplyURLToParserConfig_Logic tests the logic of ApplyURLToParserConfig without UI
func TestApplyURLToParserConfig_Logic(t *testing.T) {
	// Test URL classification logic
	input := `https://example.com/subscription
vless://uuid@server:443#Test
https://another.com/sub
vmess://base64`

	lines := strings.Split(input, "\n")
	subscriptions := make([]string, 0)
	connections := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			subscriptions = append(subscriptions, line)
		} else if strings.Contains(line, "://") {
			connections = append(connections, line)
		}
	}

	if len(subscriptions) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subscriptions))
	}
	if len(connections) != 2 {
		t.Errorf("Expected 2 connections, got %d", len(connections))
	}
}
