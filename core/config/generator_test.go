package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// TestOutboundInfo_ThreePassAlgorithm tests the three-pass algorithm for handling dynamic addOutbounds
func TestOutboundInfo_ThreePassAlgorithm(t *testing.T) {
	// This test verifies the logic of the three-pass algorithm conceptually
	// Full integration tests would require setting up ParserConfig with nodes and selectors

	t.Run("Empty dynamic selector should not be added to addOutbounds", func(t *testing.T) {
		// Test case: Selector A has addOutbounds=["B"], but B is empty (no nodes)
		// Expected: A should not include B in its addOutbounds list

		// Create outboundsInfo
		outboundsInfo := make(map[string]*outboundInfo)

		// Selector B: empty (no filtered nodes)
		outboundsInfo["B"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "B",
				Type:         "selector",
				Filters:      map[string]interface{}{"tag": "/🇷🇺/i"},
				AddOutbounds: []string{},
			},
			filteredNodes: []*ParsedNode{}, // Empty - no nodes match filter
			outboundCount: 0,               // Pass 1: only nodes count
			isValid:       false,
		}

		// Selector A: has B in addOutbounds
		outboundsInfo["A"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "A",
				Type:         "selector",
				Filters:      map[string]interface{}{"tag": "!/🇷🇺/i"},
				AddOutbounds: []string{"B", "direct-out"},
			},
			filteredNodes: []*ParsedNode{ // Has some nodes
				{Tag: "node1"},
			},
			outboundCount: 1, // Pass 1: only nodes count
			isValid:       false,
		}

		// Pass 2: Calculate total outboundCount (simulate topological sort)
		// Process B first (no dependencies)
		bInfo := outboundsInfo["B"]
		bInfo.outboundCount = len(bInfo.filteredNodes) // 0
		bInfo.isValid = (bInfo.outboundCount > 0)      // false

		// Process A (depends on B)
		aInfo := outboundsInfo["A"]
		totalCount := len(aInfo.filteredNodes) // Start with nodes: 1

		for _, addTag := range aInfo.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				// Dynamic outbound B - check if valid
				if addInfo.outboundCount > 0 {
					totalCount++
				}
			} else {
				// Constant "direct-out" - always add
				totalCount++
			}
		}

		// Expected: 1 (nodes) + 1 (direct-out constant) = 2
		// B should not be added because it's empty (outboundCount == 0)
		expectedCount := 2
		if totalCount != expectedCount {
			t.Errorf("Expected outboundCount %d, got %d. Empty selector B should not be counted.", expectedCount, totalCount)
		}

		// Verify B is not valid
		if bInfo.isValid {
			t.Error("Empty selector B should not be valid")
		}

		// Verify A is valid (has nodes + constant)
		if totalCount == 0 {
			t.Error("Selector A should be valid (has nodes + constant)")
		}
	})

	t.Run("Valid dynamic selector should be added to addOutbounds", func(t *testing.T) {
		// Test case: Selector A has addOutbounds=["B"], and B has nodes
		// Expected: A should include B in its addOutbounds list

		outboundsInfo := make(map[string]*outboundInfo)

		// Selector B: has nodes
		outboundsInfo["B"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "B",
				Type:         "selector",
				Filters:      map[string]interface{}{"tag": "/🇷🇺/i"},
				AddOutbounds: []string{},
			},
			filteredNodes: []*ParsedNode{
				{Tag: "node-ru-1"},
			},
			outboundCount: 1,
			isValid:       false,
		}

		// Selector A: has B in addOutbounds
		outboundsInfo["A"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "A",
				Type:         "selector",
				Filters:      map[string]interface{}{"tag": "!/🇷🇺/i"},
				AddOutbounds: []string{"B"},
			},
			filteredNodes: []*ParsedNode{
				{Tag: "node-int-1"},
			},
			outboundCount: 1,
			isValid:       false,
		}

		// Pass 2: Calculate total outboundCount
		bInfo := outboundsInfo["B"]
		bInfo.outboundCount = len(bInfo.filteredNodes) // 1
		bInfo.isValid = (bInfo.outboundCount > 0)      // true

		aInfo := outboundsInfo["A"]
		totalCount := len(aInfo.filteredNodes) // Start with nodes: 1

		for _, addTag := range aInfo.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				if addInfo.outboundCount > 0 {
					totalCount++ // B is valid, add it
				}
			}
		}

		// Expected: 1 (nodes) + 1 (B) = 2
		expectedCount := 2
		if totalCount != expectedCount {
			t.Errorf("Expected outboundCount %d, got %d. Valid selector B should be counted.", expectedCount, totalCount)
		}
	})

	t.Run("Constants should always be added regardless of outboundsInfo", func(t *testing.T) {
		// Test case: Selector A has addOutbounds=["direct-out", "auto-proxy-out"]
		// These are constants (not in outboundsInfo), should always be added

		outboundsInfo := make(map[string]*outboundInfo)

		outboundsInfo["A"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "A",
				Type:         "selector",
				AddOutbounds: []string{"direct-out", "auto-proxy-out"},
			},
			filteredNodes: []*ParsedNode{},
			outboundCount: 0,
			isValid:       false,
		}

		// Pass 2: Calculate total outboundCount
		aInfo := outboundsInfo["A"]
		totalCount := len(aInfo.filteredNodes) // Start with nodes: 0

		for _, addTag := range aInfo.config.AddOutbounds {
			if _, exists := outboundsInfo[addTag]; exists {
				// Dynamic - would check validity
				totalCount++
			} else {
				// Constants - always add
				totalCount++
			}
		}

		// Expected: 0 (nodes) + 2 (constants) = 2
		expectedCount := 2
		if totalCount != expectedCount {
			t.Errorf("Expected outboundCount %d, got %d. Constants should always be added.", expectedCount, totalCount)
		}
	})

	t.Run("Chain of dependencies should be processed in correct order", func(t *testing.T) {
		// Test case: A depends on B, B depends on C
		// C should be processed first, then B, then A (topological order)

		outboundsInfo := make(map[string]*outboundInfo)

		// C: has nodes
		outboundsInfo["C"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "C",
				Type:         "selector",
				AddOutbounds: []string{},
			},
			filteredNodes: []*ParsedNode{{Tag: "node-c-1"}},
			outboundCount: 1,
			isValid:       false,
		}

		// B: depends on C
		outboundsInfo["B"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "B",
				Type:         "selector",
				AddOutbounds: []string{"C"},
			},
			filteredNodes: []*ParsedNode{},
			outboundCount: 0,
			isValid:       false,
		}

		// A: depends on B
		outboundsInfo["A"] = &outboundInfo{
			config: OutboundConfig{
				Tag:          "A",
				Type:         "selector",
				AddOutbounds: []string{"B"},
			},
			filteredNodes: []*ParsedNode{{Tag: "node-a-1"}},
			outboundCount: 1,
			isValid:       false,
		}

		// Simulate topological sort processing order: C -> B -> A

		// Process C (no dependencies)
		cInfo := outboundsInfo["C"]
		cInfo.outboundCount = len(cInfo.filteredNodes) // 1
		cInfo.isValid = true

		// Process B (depends on C, which is now processed)
		bInfo := outboundsInfo["B"]
		bTotalCount := len(bInfo.filteredNodes) // 0
		for _, addTag := range bInfo.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				if addInfo.outboundCount > 0 {
					bTotalCount++ // C is valid
				}
			}
		}
		bInfo.outboundCount = bTotalCount // 1
		bInfo.isValid = true

		// Process A (depends on B, which is now processed)
		aInfo := outboundsInfo["A"]
		aTotalCount := len(aInfo.filteredNodes) // 1
		for _, addTag := range aInfo.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				if addInfo.outboundCount > 0 {
					aTotalCount++ // B is valid
				}
			}
		}
		aInfo.outboundCount = aTotalCount // 2

		// Verify counts
		if cInfo.outboundCount != 1 {
			t.Errorf("C: expected outboundCount 1, got %d", cInfo.outboundCount)
		}
		if bInfo.outboundCount != 1 {
			t.Errorf("B: expected outboundCount 1, got %d", bInfo.outboundCount)
		}
		if aInfo.outboundCount != 2 {
			t.Errorf("A: expected outboundCount 2, got %d", aInfo.outboundCount)
		}
	})
}

// Sing-box rejects Xray-only flow xtls-rprx-vision-udp443; buildOutbound maps it to vision + xudp.
// GenerateNodeJSON must emit the converted flow from node.Outbound, not the original node.Flow (used for skip filters).
func TestGenerateNodeJSON_VLESS_XtlsVisionUDP443(t *testing.T) {
	uri := "vless://729764b1-149c-49f2-b170-322544df7b5b@144.31.130.245:443?encryption=none&flow=xtls-rprx-vision-udp443&sni=144.31.130.245&fp=chrome#Cyprus"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if node == nil {
		t.Fatal("ParseNode returned nil")
	}
	if node.Flow != "xtls-rprx-vision-udp443" {
		t.Fatalf("node.Flow should stay original for filters, got %q", node.Flow)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	if !strings.Contains(jsonStr, `"flow":"xtls-rprx-vision"`) {
		t.Fatalf("expected sing-box flow in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"packet_encoding":"xudp"`) {
		t.Fatalf("expected packet_encoding in JSON:\n%s", jsonStr)
	}
	if strings.Contains(jsonStr, "xtls-rprx-vision-udp443") {
		t.Fatalf("must not emit unsupported Xray flow in JSON:\n%s", jsonStr)
	}
}

// GenerateNodeJSON must emit transport for VLESS ws and omit tls when security=none.
func TestGenerateNodeJSON_VLESS_WSTransportNoTLS(t *testing.T) {
	uri := "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@cdn.example.com:8880?encryption=none&type=ws&path=%2F&host=h.cdn&security=none#t"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	if !strings.Contains(jsonStr, `"transport":`) || !strings.Contains(jsonStr, `"type":"ws"`) {
		t.Fatalf("expected ws transport in JSON:\n%s", jsonStr)
	}
	if strings.Contains(jsonStr, `"tls":`) {
		t.Fatalf("unexpected tls for security=none:\n%s", jsonStr)
	}
}

// sing-box rejects uTLS fingerprints with wrong casing (e.g. "QQ"); emit lowercase (issue #45).
func TestGenerateNodeJSON_UTLSFingerprintLowercase(t *testing.T) {
	node := &ParsedNode{
		Scheme: "vless",
		Tag:    "t-fp",
		Server: "example.com",
		Port:   443,
		UUID:   "a0ee37a5-1844-4087-bc5c-1db6f416d38c",
		Outbound: map[string]interface{}{
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "QQ",
				},
			},
		},
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	if strings.Contains(jsonStr, `"QQ"`) {
		t.Fatalf("expected no uppercase QQ in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"fingerprint":"qq"`) {
		t.Fatalf("expected lowercase qq fingerprint:\n%s", jsonStr)
	}
}

// VLESS URI may use fingerprint= instead of fp=; same normalization as hysteria2.
func TestParseNode_VLESS_FingerprintQueryAlias(t *testing.T) {
	uri := "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@example.com:443?encryption=none&security=tls&sni=example.com&fingerprint=QQ#t"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	tls := node.Outbound["tls"].(map[string]interface{})
	ut := tls["utls"].(map[string]interface{})
	if ut["fingerprint"] != "qq" {
		t.Fatalf("fingerprint: %+v", ut)
	}
}

// GenerateNodeJSON must emit username and password for SOCKS5 (from node.Outbound / URI userinfo).
func TestGenerateNodeJSON_SOCKS5_WithAuth(t *testing.T) {
	uri := "socks5://myuser:mypass@proxy.example.com:1080#Office"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	if !strings.Contains(jsonStr, `"type":"socks"`) {
		t.Fatalf("expected socks type in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"version":"5"`) {
		t.Fatalf("expected SOCKS version 5 in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"server":"proxy.example.com"`) {
		t.Fatalf("expected server in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"server_port":1080`) {
		t.Fatalf("expected server_port in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"username":"myuser"`) {
		t.Fatalf("expected username in JSON:\n%s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"password":"mypass"`) {
		t.Fatalf("expected password in JSON:\n%s", jsonStr)
	}
}

// Large subscription lists may carry invalid UTF-8 in query/path and line breaks in #fragment labels.
// fmt %q is not JSON-safe; sing-box decode must accept the object line.
func TestGenerateNodeJSON_InvalidUTF8PathAndNewlineLabelStillValidJSON(t *testing.T) {
	node := &ParsedNode{
		Scheme: "vless",
		Tag:    "t-invalid-utf8",
		Server: "1.2.3.4",
		Port:   443,
		UUID:   "00000000-0000-0000-0000-000000000001",
		Label:  "line1\nline2",
		Outbound: map[string]interface{}{
			"transport": map[string]interface{}{
				"type": "ws",
				"path": "/prefix\xff\xfe/suffix",
			},
		},
	}
	s, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "// line1 line2") {
		t.Fatalf("expected newline sanitized in comment, got:\n%s", s)
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected comment + JSON line: %q", s)
	}
	jsonLine := strings.TrimSuffix(strings.TrimSpace(lines[len(lines)-1]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLine), &obj); err != nil {
		t.Fatalf("object line must be valid JSON: %v\n%s", err, jsonLine)
	}
}

// WireGuard endpoints use a leading // comment; subscription metadata must not break JSONC structure or UTF-8.
func TestGenerateEndpointJSON_CommentSanitized(t *testing.T) {
	node := &ParsedNode{
		Scheme:  "wireguard",
		Tag:     "wg-test-tag",
		Comment: "line1\nline2\xff\xfe",
		Outbound: map[string]interface{}{
			"private_key": "YFabc1234567890123456789012345678901234567890=",
			"address":     []string{"10.0.0.2/32"},
			"peers": []interface{}{
				map[string]interface{}{
					"address":     "203.0.113.1:51820",
					"public_key":  "YFpeerpub9876543210987654321098765432109876543210=",
					"allowed_ips": []string{"0.0.0.0/0"},
				},
			},
		},
	}
	s, err := GenerateEndpointJSON(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "// line1 line2") {
		t.Fatalf("expected newline/invalid UTF-8 sanitized in comment prefix, got prefix:\n%.80q", s)
	}
	idx := strings.Index(s, "\n")
	if idx < 0 {
		t.Fatalf("expected newline after comment: %q", s)
	}
	jsonPart := strings.TrimSpace(s[idx+1:])
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPart), &obj); err != nil {
		t.Fatalf("endpoint JSON must parse: %v\n%s", err, jsonPart)
	}
}

func TestGenerateNodeJSON_DetourEmitted(t *testing.T) {
	node := &ParsedNode{
		Scheme: "vless",
		Tag:    "main-tag",
		Server: "example.com",
		Port:   443,
		UUID:   "00000000-0000-0000-0000-000000000099",
		Flow:   "xtls-rprx-vision",
		Label:  "chain",
		Outbound: map[string]interface{}{
			"detour": "jump-tag",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "sni.test",
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
					"short_id":   "0123456789abcdef",
				},
			},
		},
	}
	s, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"detour":"jump-tag"`) {
		t.Fatalf("expected detour in JSON: %s", s)
	}
}
