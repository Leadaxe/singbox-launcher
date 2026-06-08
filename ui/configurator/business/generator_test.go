package business

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/build"
)

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
			result, err := build.FormatSectionJSON(tt.raw, tt.indentLevel)
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
			result := build.IndentMultiline(tt.text, tt.indent)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
