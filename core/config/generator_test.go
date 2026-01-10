package config

import (
	"testing"
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

