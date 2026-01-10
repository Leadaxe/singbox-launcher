package business

import (
	"testing"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// TestGetAvailableOutbounds tests GetAvailableOutbounds function
func TestGetAvailableOutbounds(t *testing.T) {
	tests := []struct {
		name           string
		model          *wizardmodels.WizardModel
		expectedMinLen int
		expectedTags   []string
	}{
		{
			name: "Model with ParserConfig",
			model: &wizardmodels.WizardModel{
				ParserConfig: &config.ParserConfig{
					ParserConfig: struct {
						Version   int                     `json:"version,omitempty"`
						Proxies   []config.ProxySource    `json:"proxies"`
						Outbounds []config.OutboundConfig `json:"outbounds"`
						Parser    struct {
							Reload      string `json:"reload,omitempty"`
							LastUpdated string `json:"last_updated,omitempty"`
						} `json:"parser,omitempty"`
					}{
						Outbounds: []config.OutboundConfig{
							{Tag: "selector-1", Type: "selector"},
							{Tag: "selector-2", Type: "selector"},
						},
					},
				},
			},
			expectedMinLen: 5, // direct-out, reject, drop, selector-1, selector-2
			expectedTags:   []string{"direct-out", "reject", "drop", "selector-1", "selector-2"},
		},
		{
			name: "Model with ParserConfigJSON",
			model: &wizardmodels.WizardModel{
				ParserConfigJSON: `{
					"ParserConfig": {
						"outbounds": [
							{"tag": "test-outbound", "type": "selector"}
						]
					}
				}`,
			},
			expectedMinLen: 4, // direct-out, reject, drop, test-outbound
			expectedTags:   []string{"direct-out", "reject", "drop", "test-outbound"},
		},
		{
			name:           "Empty model",
			model:          &wizardmodels.WizardModel{},
			expectedMinLen: 3, // direct-out, reject, drop
			expectedTags:   []string{"direct-out", "reject", "drop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAvailableOutbounds(tt.model)
			if len(result) < tt.expectedMinLen {
				t.Errorf("Expected at least %d outbounds, got %d", tt.expectedMinLen, len(result))
			}
			// Check that all expected tags are present
			tagMap := make(map[string]bool)
			for _, tag := range result {
				tagMap[tag] = true
			}
			for _, expectedTag := range tt.expectedTags {
				if !tagMap[expectedTag] {
					t.Errorf("Expected tag %q to be in result", expectedTag)
				}
			}
		})
	}
}

// TestEnsureDefaultAvailableOutbounds tests EnsureDefaultAvailableOutbounds function
func TestEnsureDefaultAvailableOutbounds(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "Empty input returns defaults",
			input:    []string{},
			expected: []string{"direct-out", "reject"},
		},
		{
			name:     "Non-empty input preserved",
			input:    []string{"test1", "test2"},
			expected: []string{"test1", "test2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureDefaultAvailableOutbounds(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d", len(tt.expected), len(result))
			}
			for i, expected := range tt.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("Expected %q at index %d, got %q", expected, i, result[i])
				}
			}
		})
	}
}

// TestEnsureFinalSelected tests EnsureFinalSelected function
func TestEnsureFinalSelected(t *testing.T) {
	tests := []struct {
		name                string
		model               *wizardmodels.WizardModel
		options             []string
		expectedFinalOutbound string
	}{
		{
			name: "Model with selected final outbound in options",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "test-outbound",
			},
			options:             []string{"direct-out", "test-outbound", "reject"},
			expectedFinalOutbound: "test-outbound",
		},
		// Note: TemplateData is a pointer to template.TemplateData which we can't easily create in test
		// So we skip testing template default fallback here - it's covered in integration tests
		{
			name: "Model without selected final, uses direct-out",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "",
			},
			options:             []string{"direct-out", "test-outbound", "reject"},
			expectedFinalOutbound: "direct-out",
		},
		{
			name: "Selected final not in options, falls back to first option",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "not-in-options",
			},
			options:             []string{"direct-out", "test-outbound"},
			expectedFinalOutbound: "direct-out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnsureFinalSelected(tt.model, tt.options)
			if tt.model.SelectedFinalOutbound != tt.expectedFinalOutbound {
				t.Errorf("Expected final outbound %q, got %q", tt.expectedFinalOutbound, tt.model.SelectedFinalOutbound)
			}
		})
	}
}

