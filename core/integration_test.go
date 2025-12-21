package core

import (
	"encoding/base64"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
)

// TestIntegration_RealWorldSubscription tests parsing real-world subscription data
// Uses examples from BLACK_VLESS_RUS.txt
func TestIntegration_RealWorldSubscription(t *testing.T) {
	// Real-world examples from the subscription file
	realWorldLinks := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates [black lists]",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria [black lists]",
		"vless://eb6a085c-437a-4539-bb43-19168d50bb10@46.250.240.80:443?encryption=none&security=reality&sni=www.microsoft.com&fp=safari&pbk=lDOVN5z1ZfaBqfUWJ9yNnonzAjW3ypLr_rJLMgm5BQQ&sid=b65b6d0bcb4cd8b8&allowInsecure=1&type=grpc&authority=&serviceName=647e311eb70230db731bd4b1&mode=gun#🇦🇺 Australia [black lists]",
		"vless://2ee2a715-d541-416a-8713-d66567448c2e@91.98.155.240:443?encryption=none&security=none&type=grpc#🇩🇪 Germany [black lists]",
		"vless://22eb7060-d314-49d6-9d24-5a3ee72488cd@185.121.134.113:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=torretobarbershop.de&fp=random&pbk=ebCoS9P5o6dm9v8Dxbe-lEuEzS-R9rPH0IaimuAIKDg&sid=25f8f93d&allowInsecure=1&type=tcp&headerType=none#🇩🇪 Germany [black lists]",
		"vless://0f8e2cfc-fdf2-46c7-bfcd-ce8273d5ff13@94.156.114.200:38395?encryption=none&flow=xtls-rprx-vision&security=reality&sni=vk.com&fp=chrome&pbk=DvKMFkm82Jkp6Jk_7pXYb0fMXq1MsLMNxDeQfcCSbEA&sid=ab12cd34&allowInsecure=1&type=tcp&headerType=none#🇩🇪 Germany [black lists]",
		"vless://a91e64f0-9295-499d-bf4a-661ad99d4938@193.233.254.154:8443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=eh.vk.com&fp=chrome&pbk=8OsJx6xuHcpL_5e1w0U4bMBa-icevDgvvzNwPwZbORQ&sid=5540e44a53c3d01c&allowInsecure=1&type=tcp&headerType=none#🇩🇪 Germany [black lists]",
		"vless://6c5984fb-3549-4067-93d0-e1fd568f8a8a@109.122.197.74:443?encryption=none&security=reality&sni=de.fasssst.online&fp=chrome&pbk=qkceRBCQxyjNn_t556P-Ia4HWlLK0l4Mxn08MpFGYDw&sid=7e59d497&allowInsecure=1&type=grpc#🇩🇪 Germany [black lists]",
	}

	t.Run("Parse all real-world links", func(t *testing.T) {
		parsedNodes := make([]*config.ParsedNode, 0)
		errors := make([]error, 0)

		for i, link := range realWorldLinks {
			node, err := subscription.ParseNode(link, nil)
			if err != nil {
				errors = append(errors, err)
				t.Logf("Failed to parse link %d: %v", i+1, err)
				continue
			}
			if node != nil {
				parsedNodes = append(parsedNodes, node)
			}
		}

		if len(errors) > 0 {
			t.Errorf("Failed to parse %d out of %d links", len(errors), len(realWorldLinks))
		}

		if len(parsedNodes) == 0 {
			t.Fatal("No nodes were parsed successfully")
		}

		// Verify all parsed nodes have valid outbounds
		for i, node := range parsedNodes {
			if node.Outbound == nil {
				t.Errorf("Node %d: Expected outbound to be generated", i+1)
				continue
			}
			if node.Outbound["tag"] == nil {
				t.Errorf("Node %d: Expected outbound to have 'tag' field", i+1)
			}
			if node.Outbound["type"] == nil {
				t.Errorf("Node %d: Expected outbound to have 'type' field", i+1)
			}
			if node.Outbound["server"] == nil {
				t.Errorf("Node %d: Expected outbound to have 'server' field", i+1)
			}
		}

		t.Logf("Successfully parsed %d out of %d real-world links", len(parsedNodes), len(realWorldLinks))
	})

	t.Run("Process through ConfigService", func(t *testing.T) {
		ac := &AppController{
			ConfigPath: "/tmp/test-config.json",
		}
		svc := NewConfigService(ac)

		proxySource := ProxySource{
			Source:      "",
			Connections: realWorldLinks,
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Failed to process real-world links: %v", err)
		}

		if len(nodes) != len(realWorldLinks) {
			t.Errorf("Expected %d nodes, got %d", len(realWorldLinks), len(nodes))
		}

		// Verify tag uniqueness
		tags := make(map[string]bool)
		for _, node := range nodes {
			if tags[node.Tag] {
				t.Errorf("Duplicate tag found: %s", node.Tag)
			}
			tags[node.Tag] = true
		}
	})

	t.Run("Test skip filters with real-world data", func(t *testing.T) {
		skipFilters := []map[string]string{
			{"tag": "/Germany/i"},
		}

		germanyCount := 0
		otherCount := 0

		for _, link := range realWorldLinks {
			node, err := subscription.ParseNode(link, skipFilters)
			if err != nil {
				continue
			}
			if node == nil {
				// Node was skipped
				if strings.Contains(link, "Germany") {
					germanyCount++
				}
			} else {
				// Node was not skipped
				if !strings.Contains(link, "Germany") {
					otherCount++
				}
			}
		}

		if germanyCount == 0 {
			t.Error("Expected some Germany nodes to be skipped")
		}
		if otherCount == 0 {
			t.Error("Expected some non-Germany nodes to be parsed")
		}
	})
}

// TestIntegration_SubscriptionDecoding tests decoding subscription content
func TestIntegration_SubscriptionDecoding(t *testing.T) {
	// Create a subscription content similar to what would be fetched
	subscriptionLines := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria",
		"vless://eb6a085c-437a-4539-bb43-19168d50bb10@46.250.240.80:443?encryption=none&security=reality&sni=www.microsoft.com&fp=safari&pbk=lDOVN5z1ZfaBqfUWJ9yNnonzAjW3ypLr_rJLMgm5BQQ&sid=b65b6d0bcb4cd8b8&allowInsecure=1&type=grpc&authority=&serviceName=647e311eb70230db731bd4b1&mode=gun#🇦🇺 Australia",
	}

	subscriptionContent := strings.Join(subscriptionLines, "\n")

	t.Run("Decode base64 subscription", func(t *testing.T) {
		// Encode as base64 (URL encoding)
		encoded := base64.URLEncoding.EncodeToString([]byte(subscriptionContent))

		decoded, err := DecodeSubscriptionContent([]byte(encoded))
		if err != nil {
			t.Fatalf("Failed to decode subscription: %v", err)
		}

		decodedStr := string(decoded)
		if !strings.Contains(decodedStr, "vless://") {
			t.Error("Decoded content should contain vless:// links")
		}

		// Verify all original lines are present
		for _, line := range subscriptionLines {
			if !strings.Contains(decodedStr, line) {
				t.Errorf("Decoded content missing line: %s", line)
			}
		}
	})

	t.Run("Decode plain text subscription", func(t *testing.T) {
		decoded, err := DecodeSubscriptionContent([]byte(subscriptionContent))
		if err != nil {
			t.Fatalf("Failed to decode plain text subscription: %v", err)
		}

		decodedStr := string(decoded)
		if decodedStr != subscriptionContent {
			t.Error("Plain text content should be returned as-is")
		}
	})

	t.Run("Parse decoded subscription lines", func(t *testing.T) {
		lines := strings.Split(subscriptionContent, "\n")
		parsedCount := 0

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if subscription.IsDirectLink(line) {
				node, err := subscription.ParseNode(line, nil)
				if err == nil && node != nil {
					parsedCount++
				}
			}
		}

		if parsedCount != len(subscriptionLines) {
			t.Errorf("Expected to parse %d nodes, got %d", len(subscriptionLines), parsedCount)
		}
	})
}

// TestIntegration_ParserConfigFlow tests the full flow from subscription to ParserConfig
func TestIntegration_ParserConfigFlow(t *testing.T) {
	realLinks := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria",
	}

	t.Run("Create ParserConfig with real links", func(t *testing.T) {
		parserConfig := &ParserConfig{
			ParserConfig: struct {
				Version   int              `json:"version,omitempty"`
				Proxies   []ProxySource    `json:"proxies"`
				Outbounds []OutboundConfig `json:"outbounds"`
				Parser    struct {
					Reload      string `json:"reload,omitempty"`
					LastUpdated string `json:"last_updated,omitempty"`
				} `json:"parser,omitempty"`
			}{
				Version: 3,
				Proxies: []ProxySource{
					{
						Source:      "",
						Connections: realLinks,
					},
				},
				Outbounds: []OutboundConfig{
					{
						Tag:  "proxy-out",
						Type: "selector",
					},
				},
			},
		}

		// Normalize
		NormalizeParserConfig(parserConfig, false)

		// Verify normalization
		if parserConfig.ParserConfig.Version != ParserConfigVersion {
			t.Errorf("Expected version %d, got %d", ParserConfigVersion, parserConfig.ParserConfig.Version)
		}
		if parserConfig.ParserConfig.Parser.Reload != "4h" {
			t.Errorf("Expected default reload '4h', got '%s'", parserConfig.ParserConfig.Parser.Reload)
		}

		// Process through ConfigService
		ac := &AppController{
			ConfigPath: "/tmp/test-config.json",
		}
		svc := NewConfigService(ac)

		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(parserConfig.ParserConfig.Proxies[0], tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Failed to process proxy source: %v", err)
		}

		if len(nodes) != len(realLinks) {
			t.Errorf("Expected %d nodes, got %d", len(realLinks), len(nodes))
		}

		// Verify all nodes have valid outbounds
		for i, node := range nodes {
			if node.Outbound == nil {
				t.Errorf("Node %d: Expected outbound to be generated", i+1)
			}
		}
	})
}
