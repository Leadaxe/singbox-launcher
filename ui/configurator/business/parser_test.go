package business

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config"
)

// TestSerializeParserConfig_Standalone tests SerializeParserConfig without UI dependencies.
// Кейс актуален для callsite'ов, которые ещё ходят через legacy ParserConfig view —
// JSON-editor вкладки, presenter_sync, loader fallback.
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
						{Tag: "proxy-out", Type: "selector"},
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

// TestTagPrefixFromSubscriptionFragment — utility-функция (используется в
// `business/sources.go::AppendURLsToSources`).
func TestTagPrefixFromSubscriptionFragment(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"https://xray.example/v/126.json#abvpn", "abvpn:"},
		{"https://xray.example/v/126.json#my%20label", "my label:"},
		{"https://xray.example/v/126.json", ""},
		{"https://xray.example/v/126.json#", ""},
		{"https://xray.example/v/126.json#%09trim%09", "trim:"},
		{"vless://uuid@host:443#nope", ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := tagPrefixFromSubscriptionFragment(tt.raw)
			if got != tt.want {
				t.Errorf("tagPrefixFromSubscriptionFragment(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
