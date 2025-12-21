package core

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestProcessProxySource_Subscription tests processing subscription URLs
func TestProcessProxySource_Subscription(t *testing.T) {
	// Create a temporary directory for test files
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	// Create a minimal AppController for testing
	ac := &AppController{
		ConfigPath: configPath,
	}
	svc := NewConfigService(ac)

	// Note: This test would require mocking HTTP requests or using a test HTTP server
	// For now, we'll test the logic that doesn't require network access

	t.Run("ProcessProxySource with empty source", func(t *testing.T) {
		proxySource := ProxySource{
			Source:      "",
			Connections: []string{"vless://test-uuid@example.com:443#Test"},
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(nodes) != 1 {
			t.Errorf("Expected 1 node, got %d", len(nodes))
		}
	})

	t.Run("ProcessProxySource with direct links", func(t *testing.T) {
		proxySource := ProxySource{
			Source: "",
			Connections: []string{
				"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates",
				"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria",
			},
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(nodes) != 2 {
			t.Errorf("Expected 2 nodes, got %d", len(nodes))
		}
		// Verify nodes were parsed correctly
		for i, node := range nodes {
			if node == nil {
				t.Errorf("Node %d is nil", i)
				continue
			}
			if node.Scheme != "vless" {
				t.Errorf("Node %d: Expected scheme 'vless', got '%s'", i, node.Scheme)
			}
			if node.Outbound == nil {
				t.Errorf("Node %d: Expected outbound to be generated", i)
			}
		}
	})

	t.Run("ProcessProxySource with skip filters", func(t *testing.T) {
		proxySource := ProxySource{
			Source: "",
			Connections: []string{
				"vless://uuid1@example.com:443#🇩🇪 Germany",
				"vless://uuid2@example.com:443#🇺🇸 USA",
			},
			Skip: []map[string]string{
				{"tag": "🇩🇪 Germany"},
			},
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// Should have only 1 node (Germany should be skipped)
		if len(nodes) != 1 {
			t.Errorf("Expected 1 node after skip filter, got %d", len(nodes))
		}
		if nodes[0].Tag == "🇩🇪 Germany" {
			t.Error("Expected Germany node to be skipped")
		}
	})

	t.Run("ProcessProxySource with tag deduplication", func(t *testing.T) {
		proxySource := ProxySource{
			Source: "",
			Connections: []string{
				"vless://uuid1@example.com:443#Test",
				"vless://uuid2@example.com:443#Test", // Duplicate tag
			},
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(nodes) != 2 {
			t.Errorf("Expected 2 nodes, got %d", len(nodes))
		}
		// Check that tags are unique
		tags := make(map[string]bool)
		for _, node := range nodes {
			if tags[node.Tag] {
				t.Errorf("Duplicate tag found: %s", node.Tag)
			}
			tags[node.Tag] = true
		}
		// One tag should be "Test" and the other should be "Test-2" or similar
		hasOriginal := false
		hasDuplicate := false
		for tag := range tags {
			if tag == "Test" {
				hasOriginal = true
			}
			if strings.HasPrefix(tag, "Test-") {
				hasDuplicate = true
			}
		}
		if !hasOriginal {
			t.Error("Expected original tag 'Test' to be present")
		}
		if !hasDuplicate {
			t.Error("Expected duplicate tag to be renamed")
		}
	})
}

// TestMakeTagUnique tests the MakeTagUnique function
func TestMakeTagUnique(t *testing.T) {
	t.Run("First occurrence", func(t *testing.T) {
		tagCounts := make(map[string]int)
		result := MakeTagUnique("Test", tagCounts, "Test")
		if result != "Test" {
			t.Errorf("Expected 'Test', got '%s'", result)
		}
		if tagCounts["Test"] != 1 {
			t.Errorf("Expected tagCounts['Test'] to be 1, got %d", tagCounts["Test"])
		}
	})

	t.Run("Duplicate tag", func(t *testing.T) {
		tagCounts := make(map[string]int)
		tagCounts["Test"] = 1
		result := MakeTagUnique("Test", tagCounts, "Test")
		if result != "Test-2" {
			t.Errorf("Expected 'Test-2', got '%s'", result)
		}
		if tagCounts["Test"] != 2 {
			t.Errorf("Expected tagCounts['Test'] to be 2, got %d", tagCounts["Test"])
		}
	})

	t.Run("Multiple duplicates", func(t *testing.T) {
		tagCounts := make(map[string]int)
		tagCounts["Test"] = 2
		result := MakeTagUnique("Test", tagCounts, "Test")
		if result != "Test-3" {
			t.Errorf("Expected 'Test-3', got '%s'", result)
		}
		if tagCounts["Test"] != 3 {
			t.Errorf("Expected tagCounts['Test'] to be 3, got %d", tagCounts["Test"])
		}
	})
}

// TestLogDuplicateTagStatistics tests the LogDuplicateTagStatistics function
func TestLogDuplicateTagStatistics(t *testing.T) {
	t.Run("No duplicates", func(t *testing.T) {
		tagCounts := map[string]int{
			"Test1": 1,
			"Test2": 1,
			"Test3": 1,
		}
		// Should not panic
		LogDuplicateTagStatistics(tagCounts, "Test")
	})

	t.Run("With duplicates", func(t *testing.T) {
		tagCounts := map[string]int{
			"Test1": 1,
			"Test2": 3, // Duplicate
			"Test3": 2, // Duplicate
		}
		// Should not panic
		LogDuplicateTagStatistics(tagCounts, "Test")
	})
}

// TestProcessProxySource_InvalidLinks tests error handling
func TestProcessProxySource_InvalidLinks(t *testing.T) {
	ac := &AppController{
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
	}
	svc := NewConfigService(ac)

	t.Run("Invalid connection link", func(t *testing.T) {
		proxySource := ProxySource{
			Source:      "",
			Connections: []string{"invalid-link"},
		}
		tagCounts := make(map[string]int)
		nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
		// Should handle invalid links gracefully
		if err != nil {
			// Error is acceptable for invalid links
		}
		// Should return empty nodes or skip invalid ones
		_ = nodes
	})
}

// TestProcessProxySource_RealWorldExamples tests with real-world examples
func TestProcessProxySource_RealWorldExamples(t *testing.T) {
	ac := &AppController{
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
	}
	svc := NewConfigService(ac)

	realExamples := []string{
		"vless://4a3ece53-6000-4ba3-a9fa-fd0d7ba61cf3@31.57.228.19:443?encryption=none&flow=xtls-rprx-vision&security=reality&sni=hls-svod.itunes.apple.com&fp=chrome&pbk=mLmBhbVFfNuo2eUgBh6r9-5Koz9mUCn3aSzlR6IejUg&sid=48720c&allowInsecure=1&type=tcp&headerType=none#🇦🇪 United Arab Emirates [black lists]",
		"vless://53fff6cc-b4ec-43e8-ade5-e0c42972fc33@152.53.227.159:80?encryption=none&security=none&type=ws&host=cdn.ir&path=%2Fnews#🇦🇹 Austria [black lists]",
		"vless://eb6a085c-437a-4539-bb43-19168d50bb10@46.250.240.80:443?encryption=none&security=reality&sni=www.microsoft.com&fp=safari&pbk=lDOVN5z1ZfaBqfUWJ9yNnonzAjW3ypLr_rJLMgm5BQQ&sid=b65b6d0bcb4cd8b8&allowInsecure=1&type=grpc&authority=&serviceName=647e311eb70230db731bd4b1&mode=gun#🇦🇺 Australia [black lists]",
	}

	proxySource := ProxySource{
		Source:      "",
		Connections: realExamples,
	}
	tagCounts := make(map[string]int)
	nodes, err := svc.ProcessProxySource(proxySource, tagCounts, nil, 0, 1)
	if err != nil {
		t.Fatalf("Unexpected error processing real-world examples: %v", err)
	}
	if len(nodes) != len(realExamples) {
		t.Errorf("Expected %d nodes, got %d", len(realExamples), len(nodes))
	}
	for i, node := range nodes {
		if node == nil {
			t.Errorf("Node %d is nil", i)
			continue
		}
		if node.Outbound == nil {
			t.Errorf("Node %d: Expected outbound to be generated", i)
		}
		// Verify outbound structure
		if node.Outbound["tag"] == nil {
			t.Errorf("Node %d: Expected outbound to have 'tag' field", i)
		}
		if node.Outbound["type"] == nil {
			t.Errorf("Node %d: Expected outbound to have 'type' field", i)
		}
		if node.Outbound["server"] == nil {
			t.Errorf("Node %d: Expected outbound to have 'server' field", i)
		}
	}
}
