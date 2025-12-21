package business

import (
	"encoding/json"
	"strings"
	"testing"

	wizardstate "singbox-launcher/ui/wizard/state"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// TestMergeRouteSection tests MergeRouteSection function
func TestMergeRouteSection(t *testing.T) {
	rawRoute := json.RawMessage(`{
  "rules": [
    {
      "domain": ["example.com"],
      "outbound": "proxy-out"
    }
  ],
  "final": "direct-out"
}`)

	// Create selectable rule states
	selectableRules := []*wizardstate.SelectableRuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Test Rule",
				Raw: map[string]interface{}{
					"domain": []string{"test.com"},
					"outbound": "proxy-out",
				},
				HasOutbound:     true,
				DefaultOutbound: "proxy-out",
			},
			Enabled:          true,
			SelectedOutbound: "proxy-out",
		},
	}

	// Create custom rules
	customRules := []*wizardstate.SelectableRuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Custom Rule",
				Raw: map[string]interface{}{
					"ip_cidr": []string{"192.168.1.0/24"},
					"outbound": "direct-out",
				},
				HasOutbound:     true,
				DefaultOutbound: "direct-out",
			},
			Enabled:          true,
			SelectedOutbound: "direct-out",
		},
	}

	result, err := MergeRouteSection(rawRoute, selectableRules, customRules, "final-out")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var route map[string]interface{}
	if err := json.Unmarshal(result, &route); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Verify rules were merged
	rules, ok := route["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules array")
	}

	// Should have original rule + selectable rule + custom rule = 3 rules
	if len(rules) != 3 {
		t.Errorf("Expected 3 rules, got %d", len(rules))
	}

	// Verify final outbound was set
	if route["final"] != "final-out" {
		t.Errorf("Expected final outbound 'final-out', got %v", route["final"])
	}
}

// TestMergeRouteSection_RejectAction tests that reject action is handled correctly
func TestMergeRouteSection_RejectAction(t *testing.T) {
	rawRoute := json.RawMessage(`{
  "rules": [],
  "final": "direct-out"
}`)

	selectableRules := []*wizardstate.SelectableRuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Reject Rule",
				Raw: map[string]interface{}{
					"domain": []string{"blocked.com"},
				},
				HasOutbound:     true,
				DefaultOutbound: "proxy-out",
			},
			Enabled:          true,
			SelectedOutbound: wizardstate.RejectActionName, // User selected reject
		},
	}

	result, err := MergeRouteSection(rawRoute, selectableRules, nil, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var route map[string]interface{}
	if err := json.Unmarshal(result, &route); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	rules, ok := route["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		t.Fatal("Expected at least one rule")
	}

	rule, ok := rules[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected rule to be a map")
	}

	// Verify action is set to reject and outbound is removed
	if rule["action"] != wizardstate.RejectActionName {
		t.Errorf("Expected action 'reject', got %v", rule["action"])
	}
	if _, hasOutbound := rule["outbound"]; hasOutbound {
		t.Error("Expected outbound to be removed for reject action")
	}
}

// TestMergeRouteSection_DisabledRules tests that disabled rules are not included
func TestMergeRouteSection_DisabledRules(t *testing.T) {
	rawRoute := json.RawMessage(`{
  "rules": [],
  "final": "direct-out"
}`)

	selectableRules := []*wizardstate.SelectableRuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Disabled Rule",
				Raw: map[string]interface{}{
					"domain": []string{"test.com"},
					"outbound": "proxy-out",
				},
				HasOutbound:     true,
				DefaultOutbound: "proxy-out",
			},
			Enabled:          false, // Disabled
			SelectedOutbound: "proxy-out",
		},
	}

	result, err := MergeRouteSection(rawRoute, selectableRules, nil, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var route map[string]interface{}
	if err := json.Unmarshal(result, &route); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	rules, ok := route["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules array")
	}

	// Disabled rule should not be included
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules (disabled rule should be excluded), got %d", len(rules))
	}
}

// TestFormatSectionJSON tests FormatSectionJSON function
func TestFormatSectionJSON(t *testing.T) {
	tests := []struct {
		name        string
		raw         json.RawMessage
		indentLevel int
		expectError bool
		checkResult func(*testing.T, string)
	}{
		{
			name:        "Valid JSON with indent level 2",
			raw:         json.RawMessage(`{"key": "value"}`),
			indentLevel: 2,
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				if !strings.Contains(result, "key") {
					t.Error("Expected result to contain 'key'")
				}
				if !strings.Contains(result, "value") {
					t.Error("Expected result to contain 'value'")
				}
			},
		},
		{
			name:        "Valid JSON with indent level 4",
			raw:         json.RawMessage(`{"key": "value"}`),
			indentLevel: 4,
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				if result == "" {
					t.Error("Expected non-empty result")
				}
			},
		},
		{
			name:        "Invalid JSON",
			raw:         json.RawMessage(`{"key": "value"`),
			indentLevel: 2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatSectionJSON(tt.raw, tt.indentLevel)
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

// TestIndentMultiline tests IndentMultiline function
func TestIndentMultiline(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		indent   string
		expected string
	}{
		{
			name:     "Single line",
			text:     "line1",
			indent:   "  ",
			expected: "  line1",
		},
		{
			name:     "Multiple lines",
			text:     "line1\nline2\nline3",
			indent:   "  ",
			expected: "  line1\n  line2\n  line3",
		},
		{
			name:     "Empty text",
			text:     "",
			indent:   "  ",
			expected: "  ",
		},
		{
			name:     "Text with trailing newline",
			text:     "line1\nline2\n",
			indent:   "  ",
			expected: "  line1\n  line2\n  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IndentMultiline(tt.text, tt.indent)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestGenerateTagPrefix tests GenerateTagPrefix function
func TestGenerateTagPrefix(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{"Index 1", 1, "1:"},
		{"Index 2", 2, "2:"},
		{"Index 10", 10, "10:"},
		{"Index 0", 0, "0:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTagPrefix(tt.index)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

