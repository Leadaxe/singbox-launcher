package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/config"
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
		{"Hysteria2 link", "hysteria2://password@server:443", true},
		{"Hysteria2 short form (hy2://)", "hy2://password@server:443", true},
		{"SSH link", "ssh://user@server:22", true},
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
		checkFields func(*testing.T, *config.ParsedNode)
	}{
		{
			name:        "Basic VLESS with Reality",
			uri:         "vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
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
			checkFields: func(t *testing.T, node *config.ParsedNode) {
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
			checkFields: func(t *testing.T, node *config.ParsedNode) {
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
		checkFields func(*testing.T, *config.ParsedNode)
	}{
		{
			name:        "Basic Trojan",
			uri:         "trojan://password123@example.com:443#Trojan Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
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
			checkFields: func(t *testing.T, node *config.ParsedNode) {
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
			{"tag": "/🇩🇪 Germany/i"},
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

	t.Run("Skip by flow - exact match", func(t *testing.T) {
		uriWithFlow := "vless://uuid@example.com:443?flow=xtls-rprx-vision-udp443#🇩🇪 Germany"
		skipFilters := []map[string]string{
			{"flow": "xtls-rprx-vision-udp443"},
		}
		node, err := ParseNode(uriWithFlow, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node != nil {
			t.Error("Expected node to be skipped, but got node")
		}
	})

	t.Run("Skip by flow - regex match", func(t *testing.T) {
		uriWithFlow := "vless://uuid@example.com:443?flow=xtls-rprx-vision-udp443#🇩🇪 Germany"
		skipFilters := []map[string]string{
			{"flow": "/xtls-rprx-vision-udp443/i"},
		}
		node, err := ParseNode(uriWithFlow, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node != nil {
			t.Error("Expected node to be skipped, but got node")
		}
	})

	t.Run("No skip by flow - different flow value", func(t *testing.T) {
		uriWithFlow := "vless://uuid@example.com:443?flow=xtls-rprx-vision#🇩🇪 Germany"
		skipFilters := []map[string]string{
			{"flow": "xtls-rprx-vision-udp443"},
		}
		node, err := ParseNode(uriWithFlow, skipFilters)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node == nil {
			t.Error("Expected node to be parsed, but got nil")
		}
		if node != nil && node.Flow != "xtls-rprx-vision" {
			t.Errorf("Expected flow 'xtls-rprx-vision', got '%s'", node.Flow)
		}
	})

	t.Run("Convert xtls-rprx-vision-udp443 to compatible format", func(t *testing.T) {
		uriWithFlow := "vless://uuid@example.com:443?flow=xtls-rprx-vision-udp443&sni=example.com&fp=chrome#🇩🇪 Germany"
		node, err := ParseNode(uriWithFlow, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if node == nil {
			t.Fatal("Expected node to be parsed, but got nil")
		}

		outbound := node.Outbound
		if outbound["flow"] != "xtls-rprx-vision" {
			t.Errorf("Expected flow 'xtls-rprx-vision', got '%v'", outbound["flow"])
		}
		if outbound["packet_encoding"] != "xudp" {
			t.Errorf("Expected packet_encoding 'xudp', got '%v'", outbound["packet_encoding"])
		}
		// Verify that original flow value is still stored in node.Flow for filtering
		if node.Flow != "xtls-rprx-vision-udp443" {
			t.Errorf("Expected node.Flow to be 'xtls-rprx-vision-udp443' (for filtering), got '%s'", node.Flow)
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
		node := &config.ParsedNode{
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
		node := &config.ParsedNode{
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

// TestParseNode_Hysteria2 tests parsing Hysteria2 nodes
func TestParseNode_Hysteria2(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
		checkFields func(*testing.T, *config.ParsedNode)
	}{
		{
			name:        "Basic Hysteria2 plain URL",
			uri:         "hysteria2://password123@example.com:443?sni=example.com#Test Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Scheme != "hysteria2" {
					t.Errorf("Expected scheme 'hysteria2', got '%s'", node.Scheme)
				}
				if node.Server != "example.com" {
					t.Errorf("Expected server 'example.com', got '%s'", node.Server)
				}
				if node.Port != 443 {
					t.Errorf("Expected port 443, got %d", node.Port)
				}
				if node.UUID != "password123" {
					t.Errorf("Expected password 'password123', got '%s'", node.UUID)
				}
				if node.Query.Get("sni") != "example.com" {
					t.Errorf("Expected SNI 'example.com', got '%s'", node.Query.Get("sni"))
				}
			},
		},
		{
			name:        "Hysteria2 with default port",
			uri:         "hysteria2://password@example.com#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Port != 443 {
					t.Errorf("Expected default port 443, got %d", node.Port)
				}
			},
		},
		{
			name:        "Hysteria2 base64-encoded URL",
			uri:         "hysteria2://NDdkYjM3M2ItZDIzYy00YWNiLWJmZDktZGFjZTM5YzRmMWU0QGhsLmthaXhpbmNsb3VkLnRvcDoyNzIwMC8/aW5zZWN1cmU9MCZzbmk9aGwua2FpeGluY2xvdWQudG9wJm1wb3J0PTI3MjAwLTI4MDAwIyVFNSU4OSVBOSVFNCVCRCU5OSVFNiVCNSU4MSVFOSU4NyU4RiVFRiVCQyU5QTkyLjcyJTIwR0INCg==",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Scheme != "hysteria2" {
					t.Errorf("Expected scheme 'hysteria2', got '%s'", node.Scheme)
				}
				if node.Server != "hl.kaixincloud.top" {
					t.Errorf("Expected server 'hl.kaixincloud.top', got '%s'", node.Server)
				}
				if node.Port != 27200 {
					t.Errorf("Expected port 27200, got %d", node.Port)
				}
				if node.UUID != "47db373b-d23c-4acb-bfd9-dace39c4f1e4" {
					t.Errorf("Expected password '47db373b-d23c-4acb-bfd9-dace39c4f1e4', got '%s'", node.UUID)
				}
				if node.Query.Get("sni") != "hl.kaixincloud.top" {
					t.Errorf("Expected SNI 'hl.kaixincloud.top', got '%s'", node.Query.Get("sni"))
				}
				if node.Query.Get("mport") != "27200-28000" {
					t.Errorf("Expected mport '27200-28000', got '%s'", node.Query.Get("mport"))
				}
				if node.Query.Get("insecure") != "0" {
					t.Errorf("Expected insecure '0', got '%s'", node.Query.Get("insecure"))
				}
			},
		},
		{
			name:        "Hysteria2 with server_ports and ALPN",
			uri:         "hysteria2://password@example.com:443?mport=10000-20000&sni=example.com&alpn=h3&insecure=0#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Query.Get("mport") != "10000-20000" {
					t.Errorf("Expected mport '10000-20000', got '%s'", node.Query.Get("mport"))
				}
				if node.Query.Get("alpn") != "h3" {
					t.Errorf("Expected alpn 'h3', got '%s'", node.Query.Get("alpn"))
				}
			},
		},
		{
			name:        "Hysteria2 with multiple ALPN values",
			uri:         "hysteria2://password@example.com:443?alpn=h3,h2#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Query.Get("alpn") != "h3,h2" {
					t.Errorf("Expected alpn 'h3,h2', got '%s'", node.Query.Get("alpn"))
				}
			},
		},
		{
			name:        "Hysteria2 with hy2:// scheme (short form)",
			uri:         "hy2://password123@example.com:443?sni=example.com#Test Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				if node.Scheme != "hysteria2" {
					t.Errorf("Expected scheme 'hysteria2', got '%s'", node.Scheme)
				}
				if node.Server != "example.com" {
					t.Errorf("Expected server 'example.com', got '%s'", node.Server)
				}
				if node.UUID != "password123" {
					t.Errorf("Expected password 'password123', got '%s'", node.UUID)
				}
			},
		},
		{
			name:        "Hysteria2 without password (warning but valid)",
			uri:         "hysteria2://@example.com:443#Test",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node == nil {
					t.Fatal("Expected node, got nil")
				}
				// Password is empty, but node is still parsed (with warning)
				if node.UUID != "" {
					t.Errorf("Expected empty password, got '%s'", node.UUID)
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

// TestBuildOutbound_Hysteria2 tests Hysteria2 outbound generation
func TestBuildOutbound_Hysteria2(t *testing.T) {
	t.Run("Hysteria2 with server_ports and ALPN", func(t *testing.T) {
		node := &config.ParsedNode{
			Tag:    "test-hysteria2",
			Scheme: "hysteria2",
			Server: "hl.kaixincloud.top",
			Port:   27200,
			UUID:   "47db373b-d23c-4acb-bfd9-dace39c4f1e4",
			Query:  make(map[string][]string),
		}
		node.Query.Set("sni", "hl.kaixincloud.top")
		node.Query.Set("mport", "27200-28000")
		node.Query.Set("insecure", "0")
		node.Query.Set("alpn", "h3")
		node.Query.Set("upmbps", "100")
		node.Query.Set("downmbps", "500")

		outbound := buildOutbound(node)
		if outbound["type"] != "hysteria2" {
			t.Errorf("Expected type 'hysteria2', got '%v'", outbound["type"])
		}
		if outbound["password"] != "47db373b-d23c-4acb-bfd9-dace39c4f1e4" {
			t.Errorf("Expected password '47db373b-d23c-4acb-bfd9-dace39c4f1e4', got '%v'", outbound["password"])
		}
		if outbound["server"] != "hl.kaixincloud.top" {
			t.Errorf("Expected server 'hl.kaixincloud.top', got '%v'", outbound["server"])
		}
		if outbound["server_port"] != 27200 {
			t.Errorf("Expected server_port 27200, got '%v'", outbound["server_port"])
		}
		// Check server_ports (array format for sing-box 1.9+)
		serverPorts, ok := outbound["server_ports"].([]string)
		if !ok {
			t.Errorf("Expected server_ports to be []string, got '%v'", outbound["server_ports"])
		} else if len(serverPorts) != 1 || serverPorts[0] != "27200:28000" {
			t.Errorf("Expected server_ports ['27200:28000'], got '%v'", serverPorts)
		}
		if outbound["up_mbps"] != 100 {
			t.Errorf("Expected up_mbps 100, got '%v'", outbound["up_mbps"])
		}
		if outbound["down_mbps"] != 500 {
			t.Errorf("Expected down_mbps 500, got '%v'", outbound["down_mbps"])
		}

		tls, ok := outbound["tls"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected TLS configuration")
		}
		if tls["enabled"] != true {
			t.Errorf("Expected TLS enabled true, got '%v'", tls["enabled"])
		}
		if tls["server_name"] != "hl.kaixincloud.top" {
			t.Errorf("Expected server_name 'hl.kaixincloud.top', got '%v'", tls["server_name"])
		}
		// insecure=0 means false, so insecure field should not be set (or be false)
		if insecureVal, ok := tls["insecure"]; ok && insecureVal != false {
			t.Errorf("Expected insecure false or not set, got '%v'", insecureVal)
		}

		alpn, ok := tls["alpn"].([]string)
		if !ok {
			t.Fatal("Expected ALPN array in TLS configuration")
		}
		if len(alpn) != 1 || alpn[0] != "h3" {
			t.Errorf("Expected ALPN ['h3'], got '%v'", alpn)
		}
	})

	t.Run("Hysteria2 with multiple ALPN values", func(t *testing.T) {
		node := &config.ParsedNode{
			Tag:    "test-hysteria2",
			Scheme: "hysteria2",
			Server: "example.com",
			Port:   443,
			UUID:   "password",
			Query:  make(map[string][]string),
		}
		node.Query.Set("sni", "example.com")
		node.Query.Set("alpn", "h3,h2")

		outbound := buildOutbound(node)
		tls, ok := outbound["tls"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected TLS configuration")
		}

		alpn, ok := tls["alpn"].([]string)
		if !ok {
			t.Fatal("Expected ALPN array in TLS configuration")
		}
		if len(alpn) != 2 || alpn[0] != "h3" || alpn[1] != "h2" {
			t.Errorf("Expected ALPN ['h3', 'h2'], got '%v'", alpn)
		}
	})

	t.Run("Hysteria2 with insecure=1", func(t *testing.T) {
		node := &config.ParsedNode{
			Tag:    "test-hysteria2",
			Scheme: "hysteria2",
			Server: "example.com",
			Port:   443,
			UUID:   "password",
			Query:  make(map[string][]string),
		}
		node.Query.Set("sni", "example.com")
		node.Query.Set("insecure", "1")

		outbound := buildOutbound(node)
		tls, ok := outbound["tls"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected TLS configuration")
		}
		if tls["insecure"] != true {
			t.Errorf("Expected insecure true, got '%v'", tls["insecure"])
		}
	})

	t.Run("Hysteria2 without password", func(t *testing.T) {
		node := &config.ParsedNode{
			Tag:    "test-hysteria2",
			Scheme: "hysteria2",
			Server: "example.com",
			Port:   443,
			UUID:   "",
			Query:  make(map[string][]string),
		}
		node.Query.Set("sni", "example.com")

		outbound := buildOutbound(node)
		// Should still generate outbound, but password will be empty
		if outbound["type"] != "hysteria2" {
			t.Errorf("Expected type 'hysteria2', got '%v'", outbound["type"])
		}
	})
}

// TestParseNode_SSH tests parsing SSH nodes
func TestParseNode_SSH(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
		checkFields func(*testing.T, *config.ParsedNode)
	}{
		{
			name:        "Basic SSH with user and password",
			uri:         "ssh://root:admin@127.0.0.1:22#Local SSH",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node.Scheme != "ssh" {
					t.Errorf("Expected scheme 'ssh', got '%s'", node.Scheme)
				}
				if node.Server != "127.0.0.1" {
					t.Errorf("Expected server '127.0.0.1', got '%s'", node.Server)
				}
				if node.Port != 22 {
					t.Errorf("Expected port 22, got %d", node.Port)
				}
				if node.UUID != "root" {
					t.Errorf("Expected user 'root', got '%s'", node.UUID)
				}
				if node.Query.Get("password") != "admin" {
					t.Errorf("Expected password 'admin', got '%s'", node.Query.Get("password"))
				}
				if node.Tag != "Local SSH" {
					t.Errorf("Expected tag 'Local SSH', got '%s'", node.Tag)
				}
			},
		},
		{
			name:        "SSH with user only (no password)",
			uri:         "ssh://user@example.com:2222#SSH Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node.UUID != "user" {
					t.Errorf("Expected user 'user', got '%s'", node.UUID)
				}
				if node.Port != 2222 {
					t.Errorf("Expected port 2222, got %d", node.Port)
				}
				if node.Query.Get("password") != "" {
					t.Errorf("Expected empty password, got '%s'", node.Query.Get("password"))
				}
			},
		},
		{
			name:        "SSH with private key path",
			uri:         "ssh://deploy@git.example.com:22?private_key_path=$HOME/.ssh/deploy_key#Git Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node.UUID != "deploy" {
					t.Errorf("Expected user 'deploy', got '%s'", node.UUID)
				}
				if node.Query.Get("private_key_path") != "$HOME/.ssh/deploy_key" {
					t.Errorf("Expected private_key_path '$HOME/.ssh/deploy_key', got '%s'", node.Query.Get("private_key_path"))
				}
			},
		},
		{
			name:        "SSH with full configuration",
			uri:         "ssh://root:password@192.168.1.1:22?private_key_path=/home/user/.ssh/id_rsa&private_key_passphrase=myphrase&host_key=ecdsa-sha2-nistp256%20AAAAE2VjZHNhLXNoYTItbmlzdH...&client_version=SSH-2.0-OpenSSH_7.4p1#My SSH Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node.Query.Get("password") != "password" {
					t.Errorf("Expected password 'password', got '%s'", node.Query.Get("password"))
				}
				if node.Query.Get("private_key_path") != "/home/user/.ssh/id_rsa" {
					t.Errorf("Expected private_key_path '/home/user/.ssh/id_rsa', got '%s'", node.Query.Get("private_key_path"))
				}
				if node.Query.Get("private_key_passphrase") != "myphrase" {
					t.Errorf("Expected private_key_passphrase 'myphrase', got '%s'", node.Query.Get("private_key_passphrase"))
				}
				if !strings.Contains(node.Query.Get("host_key"), "ecdsa-sha2-nistp256") {
					t.Errorf("Expected host_key to contain 'ecdsa-sha2-nistp256', got '%s'", node.Query.Get("host_key"))
				}
				if node.Query.Get("client_version") != "SSH-2.0-OpenSSH_7.4p1" {
					t.Errorf("Expected client_version 'SSH-2.0-OpenSSH_7.4p1', got '%s'", node.Query.Get("client_version"))
				}
			},
		},
		{
			name:        "SSH with multiple host keys",
			uri:         "ssh://user@server.com:22?host_key=key1,key2,key3#Multi Key Server",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				hostKey := node.Query.Get("host_key")
				if !strings.Contains(hostKey, "key1") || !strings.Contains(hostKey, "key2") || !strings.Contains(hostKey, "key3") {
					t.Errorf("Expected host_key to contain 'key1', 'key2', 'key3', got '%s'", hostKey)
				}
			},
		},
		{
			name:        "SSH with default port (22)",
			uri:         "ssh://admin@server.com#Default Port",
			expectError: false,
			checkFields: func(t *testing.T, node *config.ParsedNode) {
				if node.Port != 22 {
					t.Errorf("Expected default port 22, got %d", node.Port)
				}
			},
		},
		{
			name:        "SSH with invalid URI (missing hostname)",
			uri:         "ssh://user@",
			expectError: true,
		},
		{
			name:        "SSH with invalid URI (missing user)",
			uri:         "ssh://@server.com:22",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := ParseNode(tt.uri, nil)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for URI %q, but got none", tt.uri)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for URI %q: %v", tt.uri, err)
				return
			}

			if node == nil {
				t.Errorf("Expected node, got nil for URI %q", tt.uri)
				return
			}

			if tt.checkFields != nil {
				tt.checkFields(t, node)
			}

			// Verify outbound was built
			if node.Outbound == nil {
				t.Errorf("Expected outbound to be built, got nil")
				return
			}

			// Verify outbound type
			if outboundType, ok := node.Outbound["type"].(string); !ok || outboundType != "ssh" {
				t.Errorf("Expected outbound type 'ssh', got '%v'", node.Outbound["type"])
			}

			// Verify basic outbound fields
			if server, ok := node.Outbound["server"].(string); !ok || server != node.Server {
				t.Errorf("Expected outbound server '%s', got '%v'", node.Server, node.Outbound["server"])
			}

			if serverPort, ok := node.Outbound["server_port"].(int); !ok || serverPort != node.Port {
				t.Errorf("Expected outbound server_port %d, got '%v'", node.Port, node.Outbound["server_port"])
			}

			if user, ok := node.Outbound["user"].(string); !ok || user != node.UUID {
				t.Errorf("Expected outbound user '%s', got '%v'", node.UUID, node.Outbound["user"])
			}
		})
	}
}

// TestBuildOutbound_SSH tests SSH outbound building
func TestBuildOutbound_SSH(t *testing.T) {
	t.Run("SSH outbound with password", func(t *testing.T) {
		node := &config.ParsedNode{
			Scheme: "ssh",
			Server: "example.com",
			Port:   22,
			UUID:   "root",
			Tag:    "SSH Server",
			Query:  make(map[string][]string),
		}
		node.Query.Set("password", "secret123")

		outbound := buildOutbound(node)

		if outbound["type"] != "ssh" {
			t.Errorf("Expected type 'ssh', got '%v'", outbound["type"])
		}
		if outbound["server"] != "example.com" {
			t.Errorf("Expected server 'example.com', got '%v'", outbound["server"])
		}
		if outbound["server_port"] != 22 {
			t.Errorf("Expected server_port 22, got '%v'", outbound["server_port"])
		}
		if outbound["user"] != "root" {
			t.Errorf("Expected user 'root', got '%v'", outbound["user"])
		}
		if outbound["password"] != "secret123" {
			t.Errorf("Expected password 'secret123', got '%v'", outbound["password"])
		}
	})

	t.Run("SSH outbound with private key path", func(t *testing.T) {
		node := &config.ParsedNode{
			Scheme: "ssh",
			Server: "server.com",
			Port:   22,
			UUID:   "deploy",
			Tag:    "Deploy Server",
			Query:  make(map[string][]string),
		}
		node.Query.Set("private_key_path", "/home/user/.ssh/id_rsa")
		node.Query.Set("private_key_passphrase", "mypassphrase")

		outbound := buildOutbound(node)

		if outbound["private_key_path"] != "/home/user/.ssh/id_rsa" {
			t.Errorf("Expected private_key_path '/home/user/.ssh/id_rsa', got '%v'", outbound["private_key_path"])
		}
		if outbound["private_key_passphrase"] != "mypassphrase" {
			t.Errorf("Expected private_key_passphrase 'mypassphrase', got '%v'", outbound["private_key_passphrase"])
		}
	})

	t.Run("SSH outbound with host keys", func(t *testing.T) {
		node := &config.ParsedNode{
			Scheme: "ssh",
			Server: "server.com",
			Port:   22,
			UUID:   "user",
			Tag:    "Verified Server",
			Query:  make(map[string][]string),
		}
		node.Query.Set("host_key", "key1,key2,key3")

		outbound := buildOutbound(node)

		hostKeys, ok := outbound["host_key"].([]string)
		if !ok {
			t.Errorf("Expected host_key to be []string, got '%T'", outbound["host_key"])
			return
		}
		if len(hostKeys) != 3 {
			t.Errorf("Expected 3 host keys, got %d", len(hostKeys))
		}
		if hostKeys[0] != "key1" || hostKeys[1] != "key2" || hostKeys[2] != "key3" {
			t.Errorf("Expected host keys ['key1', 'key2', 'key3'], got %v", hostKeys)
		}
	})

	t.Run("SSH outbound with client version", func(t *testing.T) {
		node := &config.ParsedNode{
			Scheme: "ssh",
			Server: "server.com",
			Port:   22,
			UUID:   "user",
			Tag:    "Custom Client",
			Query:  make(map[string][]string),
		}
		node.Query.Set("client_version", "SSH-2.0-OpenSSH_7.4p1")

		outbound := buildOutbound(node)

		if outbound["client_version"] != "SSH-2.0-OpenSSH_7.4p1" {
			t.Errorf("Expected client_version 'SSH-2.0-OpenSSH_7.4p1', got '%v'", outbound["client_version"])
		}
	})

	t.Run("SSH outbound without user (should use default)", func(t *testing.T) {
		node := &config.ParsedNode{
			Scheme: "ssh",
			Server: "server.com",
			Port:   22,
			UUID:   "", // No user
			Tag:    "Default User",
			Query:  make(map[string][]string),
		}

		outbound := buildOutbound(node)

		if outbound["user"] != "root" {
			t.Errorf("Expected default user 'root', got '%v'", outbound["user"])
		}
	})
}
