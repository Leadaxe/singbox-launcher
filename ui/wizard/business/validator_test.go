package business

import (
	"strings"
	"testing"

	"singbox-launcher/core/config"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// TestValidateParserConfig tests ValidateParserConfig function
func TestValidateParserConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.ParserConfig
		expectError bool
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
		},
		{
			name:        "Nil ParserConfig",
			config:      nil,
			expectError: true,
		},
		{
			name: "ParserConfig with nil Proxies",
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
					Proxies: nil,
				},
			},
			expectError: true,
		},
		{
			name: "ParserConfig with invalid URL",
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
					Proxies: []config.ProxySource{
						{
							Source: "invalid-url",
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "ParserConfig with invalid URI",
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
					Proxies: []config.ProxySource{
						{
							Connections: []string{"invalid-uri"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "ParserConfig with invalid outbound",
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
					Proxies: []config.ProxySource{},
					Outbounds: []config.OutboundConfig{
						{
							Tag:  "", // Empty tag should fail
							Type: "selector",
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParserConfig(tt.config)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateURL tests ValidateURL function
func TestValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{"Valid HTTPS URL", "https://example.com/subscription", false},
		{"Valid HTTP URL", "http://example.com/subscription", false},
		{"Empty URL", "", true},
		{"URL too long", "https://example.com/" + strings.Repeat("a", wizardutils.MaxURILength), true},
		{"URL too short", "http://a", true},
		{"URL without scheme", "example.com/subscription", true},
		{"URL without host", "https://", true},
		{"Invalid URL format", "not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for URL %q, got nil", tt.url)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for URL %q: %v", tt.url, err)
				}
			}
		})
	}
}

// TestValidateURI tests ValidateURI function
func TestValidateURI(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
	}{
		{"Valid VLESS URI", "vless://uuid@server:443", false},
		{"Valid VMess URI", "vmess://base64", false},
		{"Valid Trojan URI", "trojan://password@server:443", false},
		{"Empty URI", "", true},
		{"URI too long", "vless://" + strings.Repeat("a", wizardutils.MaxURILength), true},
		{"URI too short", "vless://", true},
		{"URI without protocol", "uuid@server:443", true},
		{"Invalid URI format", "not-a-uri", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURI(tt.uri)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for URI %q, got nil", tt.uri)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for URI %q: %v", tt.uri, err)
				}
			}
		})
	}
}

// TestValidateOutbound tests ValidateOutbound function
func TestValidateOutbound(t *testing.T) {
	tests := []struct {
		name        string
		outbound    *config.OutboundConfig
		expectError bool
	}{
		{
			name: "Valid outbound",
			outbound: &config.OutboundConfig{
				Tag:  "proxy-out",
				Type: "selector",
			},
			expectError: false,
		},
		{
			name:        "Nil outbound",
			outbound:    nil,
			expectError: true,
		},
		{
			name: "Empty tag",
			outbound: &config.OutboundConfig{
				Tag:  "",
				Type: "selector",
			},
			expectError: true,
		},
		{
			name: "Empty type",
			outbound: &config.OutboundConfig{
				Tag:  "proxy-out",
				Type: "",
			},
			expectError: true,
		},
		{
			name: "Tag too long",
			outbound: &config.OutboundConfig{
				Tag:  strings.Repeat("a", 257),
				Type: "selector",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutbound(tt.outbound)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateJSONSize tests ValidateJSONSize function
func TestValidateJSONSize(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "Valid size",
			data:        []byte(strings.Repeat("a", 1000)),
			expectError: false,
		},
		{
			name:        "Size at limit",
			data:        []byte(strings.Repeat("a", int(wizardutils.MaxJSONConfigSize))),
			expectError: false,
		},
		{
			name:        "Size exceeds limit",
			data:        []byte(strings.Repeat("a", int(wizardutils.MaxJSONConfigSize)+1)),
			expectError: true,
		},
		{
			name:        "Empty data",
			data:        []byte{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJSONSize(tt.data)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateJSON tests ValidateJSON function
func TestValidateJSON(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
	}{
		{
			name:        "Valid JSON",
			data:        []byte(`{"key": "value"}`),
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			data:        []byte(`{"key": "value"`),
			expectError: true,
		},
		{
			name:        "JSON too large",
			data:        []byte(strings.Repeat("a", int(wizardutils.MaxJSONConfigSize)+1)),
			expectError: true,
		},
		{
			name:        "Empty JSON",
			data:        []byte(`{}`),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJSON(tt.data)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateHTTPResponseSize tests ValidateHTTPResponseSize function
func TestValidateHTTPResponseSize(t *testing.T) {
	tests := []struct {
		name        string
		size        int64
		expectError bool
	}{
		{
			name:        "Valid size",
			size:        1000,
			expectError: false,
		},
		{
			name:        "Size at limit",
			size:        wizardutils.MaxSubscriptionSize,
			expectError: false,
		},
		{
			name:        "Size exceeds limit",
			size:        wizardutils.MaxSubscriptionSize + 1,
			expectError: true,
		},
		{
			name:        "Zero size",
			size:        0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHTTPResponseSize(tt.size)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateParserConfigJSON tests ValidateParserConfigJSON function
func TestValidateParserConfigJSON(t *testing.T) {
	validConfig := `{
  "ParserConfig": {
    "version": 2,
    "proxies": [
      {
        "source": "https://example.com/subscription",
        "connections": ["vless://uuid@server:443"]
      }
    ],
    "outbounds": [
      {
        "tag": "proxy-out",
        "type": "selector"
      }
    ]
  }
}`

	tests := []struct {
		name        string
		jsonText    string
		expectError bool
	}{
		{
			name:        "Valid ParserConfig JSON",
			jsonText:    validConfig,
			expectError: false,
		},
		{
			name:        "Empty JSON",
			jsonText:    "",
			expectError: true,
		},
		{
			name:        "Invalid JSON",
			jsonText:    `{"ParserConfig": {`,
			expectError: true,
		},
		{
			name:        "JSON too large",
			jsonText:    `{"ParserConfig": {` + strings.Repeat("a", int(wizardutils.MaxJSONConfigSize)) + `}}`,
			expectError: true,
		},
		{
			name:        "Invalid ParserConfig structure",
			jsonText:    `{"ParserConfig": {"proxies": null}}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParserConfigJSON(tt.jsonText)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateRule tests ValidateRule function
func TestValidateRule(t *testing.T) {
	tests := []struct {
		name        string
		rule        map[string]interface{}
		expectError bool
	}{
		{
			name: "Valid rule with domain",
			rule: map[string]interface{}{
				"domain":   []string{"example.com"},
				"outbound": "proxy-out",
			},
			expectError: false,
		},
		{
			name: "Valid rule with ip_cidr",
			rule: map[string]interface{}{
				"ip_cidr":  []string{"192.168.1.0/24"},
				"outbound": "proxy-out",
			},
			expectError: false,
		},
		{
			name:        "Nil rule",
			rule:        nil,
			expectError: true,
		},
		{
			name:        "Empty rule",
			rule:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRule(tt.rule)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
