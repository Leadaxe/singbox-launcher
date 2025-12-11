package parsers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
)

// TestIsDirectLink tests the IsDirectLink function
func TestIsDirectLink(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"VLESS link", "vless://uuid@server:443", true},
		{"VMess link", "vmess://base64", true},
		{"Trojan link", "trojan://password@server:443", true},
		{"Shadowsocks link", "ss://method:password@server:443", true},
		{"HTTP URL", "https://example.com/subscription", false},
		{"Empty string", "", false},
		{"Whitespace VLESS", "  vless://uuid@server:443  ", true},
		{"Invalid scheme", "http://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDirectLink(tt.input)
			if result != tt.expected {
				t.Errorf("IsDirectLink(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseNode_VLESS tests parsing VLESS nodes
func TestParseNode_VLESS(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
		checkFields func(*testing.T, *ParsedNode)
	}{
		{
			name:        "Basic VLESS with Reality",
			uri:         "vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates",
			expectError: false,
			checkFields: func(t *testing.T, node *ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Scheme != "vless" {
					t.Errorf("Expected scheme 'vless', got '%s'", node.Scheme)
				}
				if node.Server != "31.57.228.19" {
					t.Errorf("Expected server '31.57.228.19', got '%s'", node.Server)
				}
				if node.Port != 443 {
					t.Errorf("Expected port 443, got %d", node.Port)
				}
				if node.UUID != "4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3" {
					t.Errorf("Expected UUID '4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3', got '%s'", node.UUID)
				}
				if node.Flow != "xtls-rprx-vision" {
					t.Errorf("Expected flow 'xtls-rprx-vision', got '%s'", node.Flow)
				}
				if node.Query.Get("sni") != "hls-svod.itunes.apple.com" {
					t.Errorf("Expected SNI 'hls-svod.itunes.apple.com', got '%s'", node.Query.Get("sni"))
				}
			},
		},
		{
			name:        "VLESS with default port",
			uri:         "vless://uuid@example.com#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Port != 443 {
					t.Errorf("Expected default port 443, got %d", node.Port)
				}
			},
		},
		{
			name:        "VLESS with custom port",
			uri:         "vless://uuid@example.com:8443#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Port != 8443 {
					t.Errorf("Expected port 8443, got %d", node.Port)
				}
			},
		},
		{
			name:        "Invalid VLESS URI",
			uri:         "vless://invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := ParseNode(tt.uri, nil)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if tt.checkFields != nil {
				tt.checkFields(t, node)
			}
		})
	}
}

// TestParseNode_VMess tests parsing VMess nodes
func TestParseNode_VMess(t *testing.T) {
	// Create a valid VMess JSON config
	vmessConfig := map[string]interface{}{
		"v":    "2",
		"ps":   "Test VMess",
		"add":  "example.com",
		"port": "443",
		"id":   "12345678-1234-1234-1234-123456789abc",
		"net":  "tcp",
		"type": "none",
		"tls":  "tls",
		"sni":  "example.com",
	}
	vmessJSON, _ := json.Marshal(vmessConfig)
	vmessBase64 := base64.URLEncoding.EncodeToString(vmessJSON)
	vmessURI := "vmess://" + vmessBase64

	t.Run("Valid VMess node", func(t *testing.T) {
		node, err := ParseNode(vmessURI, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node == nil {
			t.Fatal("Expected node, got nil")
		}
		if node.Scheme != "vmess" {
			t.Errorf("Expected scheme 'vmess', got '%s'", node.Scheme)
		}
		if node.Server != "example.com" {
			t.Errorf("Expected server 'example.com', got '%s'", node.Server)
		}
		if node.Port != 443 {
			t.Errorf("Expected port 443, got %d", node.Port)
		}
		if node.UUID != "12345678-1234-1234-1234-123456789abc" {
			t.Errorf("Expected UUID '12345678-1234-1234-1234-123456789abc', got '%s'", node.UUID)
		}
	})

	t.Run("Invalid VMess base64", func(t *testing.T) {
		_, err := ParseNode("vmess://invalid-base64!!!", nil)
		if err == nil {
			t.Error("Expected error for invalid base64, got nil")
		}
	})
}

// TestParseNode_Trojan tests parsing Trojan nodes
func TestParseNode_Trojan(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
		checkFields func(*testing.T, *ParsedNode)
	}{
		{
			name:        "Basic Trojan",
			uri:         "trojan://password123@example.com:443#Trojan Server",
			expectError: false,
			checkFields: func(t *testing.T, node *ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Scheme != "trojan" {
					t.Errorf("Expected scheme 'trojan', got '%s'", node.Scheme)
				}
				if node.UUID != "password123" {
					t.Errorf("Expected password 'password123', got '%s'", node.UUID)
				}
			},
		},
		{
			name:        "Trojan with default port",
			uri:         "trojan://password@example.com#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Port != 443 {
					t.Errorf("Expected default port 443, got %d", node.Port)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := ParseNode(tt.uri, nil)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if tt.checkFields != nil {
				tt.checkFields(t, node)
			}
		})
	}
}

// TestParseNode_Shadowsocks tests parsing Shadowsocks nodes
func TestParseNode_Shadowsocks(t *testing.T) {
	// SIP002 format: ss://base64(method:password)@server:port#tag
	method := "aes-256-gcm"
	password := "test-password"
	userinfo := method + ":" + password
	encodedUserinfo := base64.URLEncoding.EncodeToString([]byte(userinfo))
	ssURI := "ss://" + encodedUserinfo + "@example.com:443#Shadowsocks Server"

	t.Run("Valid Shadowsocks SIP002", func(t *testing.T) {
		node, err := ParseNode(ssURI, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node == nil {
			t.Fatal("Expected node, got nil")
		}
		if node.Scheme != "ss" {
			t.Errorf("Expected scheme 'ss', got '%s'", node.Scheme)
		}
		if node.Query.Get("method") != method {
			t.Errorf("Expected method '%s', got '%s'", method, node.Query.Get("method"))
		}
		if node.Query.Get("password") != password {
			t.Errorf("Expected password '%s', got '%s'", password, node.Query.Get("password"))
		}
	})

	t.Run("Invalid Shadowsocks missing credentials", func(t *testing.T) {
		_, err := ParseNode("ss://@example.com:443", nil)
		if err == nil {
			t.Error("Expected error for missing credentials, got nil")
		}
	})
}

// TestParseNode_SkipFilters tests skip filter functionality
func TestParseNode_SkipFilters(t *testing.T) {
	uri := "vless://uuid@example.com:443#🇩🇪 Germany [black lists]"

	t.Run("Skip by tag", func(t *testing.T) {
		skipFilters := []map[string]string{
			{"tag": "🇩🇪 Germany"},
		}
		node, err := ParseNode(uri, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node != nil {
			t.Error("Expected node to be skipped, but got node")
		}
	})

	t.Run("Skip by host", func(t *testing.T) {
		skipFilters := []map[string]string{
			{"host": "example.com"},
		}
		node, err := ParseNode(uri, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node != nil {
			t.Error("Expected node to be skipped, but got node")
		}
	})

	t.Run("Skip by regex", func(t *testing.T) {
		skipFilters := []map[string]string{
			{"tag": "/Germany/i"},
		}
		node, err := ParseNode(uri, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node != nil {
			t.Error("Expected node to be skipped, but got node")
		}
	})

	t.Run("No skip - node should be parsed", func(t *testing.T) {
		skipFilters := []map[string]string{
			{"tag": "🇺🇸 USA"},
		}
		node, err := ParseNode(uri, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node == nil {
			t.Error("Expected node to be parsed, but got nil")
		}
	})
}

// TestParseNode_RealWorldExamples tests with real-world examples from subscription
func TestParseNode_RealWorldExamples(t *testing.T) {
	realExamples := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates [black lists]",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria [black lists]",
		"vless://eb6a085c-437a-4539-bb43-19168d50bb10@46.250.240.80:443?encryption=none&security=reality&sni=www.microsoft.com&fp=safari&pbk=lDOVN5z1ZfaBqfUWJ9yNnonzAjW3ypLr_rJLMgm5BQQ&sid=b65b6d0bcb4cd8b8&allowInsecure=1&type=grpc&authority=&serviceName=647e311eb70230db731bd4b1&mode=gun#🇦🇺 Australia [black lists]",
		"vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#🇩🇪 Germany [black lists]",
	}

	for i, uri := range realExamples {
		t.Run(fmt.Sprintf("Real example %d", i+1), func(t *testing.T) {
			node, err := ParseNode(uri, nil)
			if err != nil {
				t.Fatalf("Failed to parse real-world example: %v", err)
			}
			if node == nil {
				t.Fatal("Expected node, got nil")
			}
			if node.Outbound == nil {
				t.Error("Expected outbound to be generated")
			}
			// Verify outbound has required fields
			if node.Outbound["tag"] == nil {
				t.Error("Expected outbound to have 'tag' field")
			}
			if node.Outbound["type"] == nil {
				t.Error("Expected outbound to have 'type' field")
			}
			if node.Outbound["server"] == nil {
				t.Error("Expected outbound to have 'server' field")
			}
		})
	}
}

// TestBuildOutbound tests outbound generation
func TestBuildOutbound(t *testing.T) {
	t.Run("VLESS with Reality", func(t *testing.T) {
		node := &ParsedNode{
			Tag:    "test-vless",
			Scheme: "vless",
			Server: "example.com",
			Port:   443,
			UUID:   "test-uuid",
			Flow:   "xtls-rprx-vision",
			Query:  make(map[string][]string),
		}
		node.Query.Set("sni", "example.com")
		node.Query.Set("fp", "chrome")
		node.Query.Set("pbk", "test-public-key")
		node.Query.Set("sid", "test-short-id")

		outbound := buildOutbound(node)
		if outbound["type"] != "vless" {
			t.Errorf("Expected type 'vless', got '%v'", outbound["type"])
		}
		if outbound["uuid"] != "test-uuid" {
			t.Errorf("Expected uuid 'test-uuid', got '%v'", outbound["uuid"])
		}
		if outbound["flow"] != "xtls-rprx-vision" {
			t.Errorf("Expected flow 'xtls-rprx-vision', got '%v'", outbound["flow"])
		}
		tls, ok := outbound["tls"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected TLS configuration")
		}
		reality, ok := tls["reality"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected Reality configuration")
		}
		if reality["public_key"] != "test-public-key" {
			t.Errorf("Expected public_key 'test-public-key', got '%v'", reality["public_key"])
		}
	})

	t.Run("Shadowsocks type conversion", func(t *testing.T) {
		node := &ParsedNode{
			Tag:    "test-ss",
			Scheme: "ss",
			Server: "example.com",
			Port:   443,
			Query:  make(map[string][]string),
		}
		node.Query.Set("method", "aes-256-gcm")
		node.Query.Set("password", "test-password")

		outbound := buildOutbound(node)
		if outbound["type"] != "shadowsocks" {
			t.Errorf("Expected type 'shadowsocks', got '%v'", outbound["type"])
		}
		if outbound["method"] != "aes-256-gcm" {
			t.Errorf("Expected method 'aes-256-gcm', got '%v'", outbound["method"])
		}
		if outbound["password"] != "test-password" {
			t.Errorf("Expected password 'test-password', got '%v'", outbound["password"])
		}
	})
}
