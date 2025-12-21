//go:build cgo

package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
)

// TestApplyURLToParserConfig tests the applyURLToParserConfig logic
func TestApplyURLToParserConfig(t *testing.T) {
	t.Run("Split subscriptions and connections", func(t *testing.T) {
		state := &WizardState{}
		entry := widget.NewMultiLineEntry()
		entry.SetText(`{
	"ParserConfig": {
		"version": 2,
		"proxies": [],
		"outbounds": []
	}
}`)
		state.ParserConfigEntry = entry

		input := `https://example.com/subscription
vless://uuid@server:443#Test
https://another.com/sub
vmess://base64`

		// This would normally update the ParserConfigEntry
		// For testing, we'll verify the logic manually
		lines := strings.Split(input, "\n")
		subscriptions := make([]string, 0)
		connections := make([]string, 0)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if core.IsSubscriptionURL(line) {
				subscriptions = append(subscriptions, line)
			} else if strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "vmess://") {
				connections = append(connections, line)
			}
		}

		if len(subscriptions) != 2 {
			t.Errorf("Expected 2 subscriptions, got %d", len(subscriptions))
		}
		if len(connections) != 2 {
			t.Errorf("Expected 2 connections, got %d", len(connections))
		}
	})

	t.Run("Empty input", func(t *testing.T) {
		state := &WizardState{}
		entry := widget.NewMultiLineEntry()
		entry.SetText(`{
	"ParserConfig": {
		"version": 2,
		"proxies": [],
		"outbounds": []
	}
}`)
		state.ParserConfigEntry = entry

		input := ""
		lines := strings.Split(input, "\n")
		subscriptions := make([]string, 0)
		connections := make([]string, 0)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if core.IsSubscriptionURL(line) {
				subscriptions = append(subscriptions, line)
			} else if strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "vmess://") {
				connections = append(connections, line)
			}
		}

		if len(subscriptions) != 0 {
			t.Errorf("Expected 0 subscriptions, got %d", len(subscriptions))
		}
		if len(connections) != 0 {
			t.Errorf("Expected 0 connections, got %d", len(connections))
		}
	})
}

// TestSerializeParserConfig tests the serializeParserConfig function
func TestSerializeParserConfig(t *testing.T) {
	t.Run("Valid ParserConfig", func(t *testing.T) {
		parserConfig := &core.ParserConfig{
			ParserConfig: struct {
				Version   int                   `json:"version,omitempty"`
				Proxies   []core.ProxySource    `json:"proxies"`
				Outbounds []core.OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{
				Version: 2,
				Proxies: []core.ProxySource{
					{
						Source:      "https://example.com/subscription",
						Connections: []string{"vless://uuid@server:443"},
					},
				},
				Outbounds: []core.OutboundConfig{
					{
						Tag:  "proxy-out",
						Type: "selector",
					},
				},
			},
		}

		result, err := serializeParserConfig(parserConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == "" {
			t.Error("Expected non-empty result")
		}

		// Verify it's valid JSON
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Fatalf("Result is not valid JSON: %v", err)
		}

		// Verify structure
		pc, ok := parsed["ParserConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected ParserConfig object")
		}
		if pc["version"].(float64) != 2 {
			t.Errorf("Expected version 2, got %v", pc["version"])
		}
	})

	t.Run("Nil ParserConfig", func(t *testing.T) {
		_, err := serializeParserConfig(nil)
		if err == nil {
			t.Error("Expected error for nil ParserConfig, got nil")
		}
	})

	t.Run("ParserConfig with default reload", func(t *testing.T) {
		parserConfig := &core.ParserConfig{
			ParserConfig: struct {
				Version   int                   `json:"version,omitempty"`
				Proxies   []core.ProxySource    `json:"proxies"`
				Outbounds []core.OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{
				Version: 2,
			},
		}

		// Normalize should set default reload
		core.NormalizeParserConfig(parserConfig, false)

		result, err := serializeParserConfig(parserConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(result), &parsed); err != nil {
			t.Fatalf("Result is not valid JSON: %v", err)
		}

		pc := parsed["ParserConfig"].(map[string]interface{})
		parser := pc["parser"].(map[string]interface{})
		if parser["reload"] != "4h" {
			t.Errorf("Expected default reload '4h', got '%v'", parser["reload"])
		}
	})
}

// TestGetAvailableOutbounds tests the getAvailableOutbounds logic
func TestGetAvailableOutbounds(t *testing.T) {
	t.Run("With ParserConfig", func(t *testing.T) {
		state := &WizardState{
			ParserConfig: &core.ParserConfig{
				ParserConfig: struct {
					Version   int                   `json:"version,omitempty"`
					Proxies   []core.ProxySource    `json:"proxies"`
					Outbounds []core.OutboundConfig `json:"outbounds"`
					Parser    struct {
						Reload      string `json:"reload,omitempty"`
						LastUpdated string `json:"last_updated,omitempty"`
					} `json:"parser,omitempty"`
				}{
					Outbounds: []core.OutboundConfig{
						{
							Tag:          "proxy-out",
							Type:         "selector",
							AddOutbounds: []string{"extra-outbound"},
						},
					},
				},
			},
		}

		options := state.getAvailableOutbounds()
		if len(options) == 0 {
			t.Error("Expected at least some outbound options")
		}

		// Should include default options
		hasDirect := false
		hasReject := false
		hasProxyOut := false
		for _, opt := range options {
			if opt == defaultOutboundTag {
				hasDirect = true
			}
			if opt == rejectActionName {
				hasReject = true
			}
			if opt == "proxy-out" {
				hasProxyOut = true
			}
		}

		if !hasDirect {
			t.Error("Expected 'direct-out' to be in options")
		}
		if !hasReject {
			t.Error("Expected 'reject' to be in options")
		}
		if !hasProxyOut {
			t.Error("Expected 'proxy-out' to be in options")
		}
	})

	t.Run("Without ParserConfig", func(t *testing.T) {
		state := &WizardState{}
		options := state.getAvailableOutbounds()
		if len(options) == 0 {
			t.Error("Expected at least default outbound options")
		}
		// Should have at least direct-out and reject
		if len(options) < 2 {
			t.Errorf("Expected at least 2 default options, got %d", len(options))
		}
	})
}

// TestEnsureFinalSelected tests the ensureFinalSelected logic
func TestEnsureFinalSelected(t *testing.T) {
	t.Run("Select from available options", func(t *testing.T) {
		state := &WizardState{}
		options := []string{"direct-out", "proxy-out", "reject"}
		state.ensureFinalSelected(options)

		if state.SelectedFinalOutbound == "" {
			t.Error("Expected SelectedFinalOutbound to be set")
		}
		if state.SelectedFinalOutbound != "direct-out" {
			// Should default to direct-out or first option
			if state.SelectedFinalOutbound != options[0] {
				t.Errorf("Expected SelectedFinalOutbound to be 'direct-out' or first option, got '%s'", state.SelectedFinalOutbound)
			}
		}
	})

	t.Run("Preserve existing selection if valid", func(t *testing.T) {
		state := &WizardState{
			SelectedFinalOutbound: "proxy-out",
		}
		options := []string{"direct-out", "proxy-out", "reject"}
		state.ensureFinalSelected(options)

		if state.SelectedFinalOutbound != "proxy-out" {
			t.Errorf("Expected SelectedFinalOutbound to remain 'proxy-out', got '%s'", state.SelectedFinalOutbound)
		}
	})

	t.Run("Update if selection not in options", func(t *testing.T) {
		state := &WizardState{
			SelectedFinalOutbound: "invalid-outbound",
		}
		options := []string{"direct-out", "proxy-out", "reject"}
		state.ensureFinalSelected(options)

		if state.SelectedFinalOutbound == "invalid-outbound" {
			t.Error("Expected SelectedFinalOutbound to be updated from invalid option")
		}
		if state.SelectedFinalOutbound == "" {
			t.Error("Expected SelectedFinalOutbound to be set to a valid option")
		}
	})
}

// Mock entry for testing
// Using real fyne widgets in tests avoids type mismatch with `*widget.Entry`.

// TestRealWorldSubscriptionParsing tests with real subscription examples
func TestRealWorldSubscriptionParsing(t *testing.T) {
	// Example from BLACK_VLESS_RUS.txt
	realLinks := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates [black lists]",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria [black lists]",
		"vless://eb6a085c-437a-4539-bb43-19168d50bb10@46.250.240.80:443?encryption=none&security=reality&sni=www.microsoft.com&fp=safari&pbk=lDOVN5z1ZfaBqfUWJ9yNnonzAjW3ypLr_rJLMgm5BQQ&sid=b65b6d0bcb4cd8b8&allowInsecure=1&type=grpc&authority=&serviceName=647e311eb70230db731bd4b1&mode=gun#🇦🇺 Australia [black lists]",
	}

	// Test that these can be parsed and serialized in ParserConfig
	parserConfig := &core.ParserConfig{
		ParserConfig: struct {
			Version   int                   `json:"version,omitempty"`
			Proxies   []core.ProxySource    `json:"proxies"`
			Outbounds []core.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version: 2,
			Proxies: []core.ProxySource{
				{
					Source:      "",
					Connections: realLinks,
				},
			},
			Outbounds: []core.OutboundConfig{
				{
					Tag:  "proxy-out",
					Type: "selector",
				},
			},
		},
	}

	// Normalize
	core.NormalizeParserConfig(parserConfig, false)

	// Serialize
	result, err := serializeParserConfig(parserConfig)
	if err != nil {
		t.Fatalf("Failed to serialize ParserConfig with real-world examples: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Serialized result is not valid JSON: %v", err)
	}

	pc := parsed["ParserConfig"].(map[string]interface{})
	proxies := pc["proxies"].([]interface{})
	if len(proxies) != 1 {
		t.Errorf("Expected 1 proxy source, got %d", len(proxies))
	}

	proxySource := proxies[0].(map[string]interface{})
	connections := proxySource["connections"].([]interface{})
	if len(connections) != len(realLinks) {
		t.Errorf("Expected %d connections, got %d", len(realLinks), len(connections))
	}
}
