package business

import (
	"reflect"
	"testing"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
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
			// SPEC 117: теги Направлений читаются из canonical GlobalOutbounds.
			name: "Model with GlobalOutbounds",
			model: &wizardmodels.WizardModel{
				GlobalOutbounds: []config.Direction{
					{Tag: "selector-1", Type: "selector"},
					{Tag: "selector-2", Type: "selector"},
				},
			},
			expectedMinLen: 5, // direct-out, reject, drop, selector-1, selector-2
			expectedTags:   []string{"direct-out", "reject", "drop", "selector-1", "selector-2"},
		},
		{
			// Выключенное Направление в цели не попадает.
			name: "Disabled direction skipped",
			model: &wizardmodels.WizardModel{
				GlobalOutbounds: []config.Direction{
					{Tag: "alive", Type: "selector"},
					{Tag: "paused", Type: "selector", Disabled: true},
				},
			},
			expectedMinLen: 4, // direct-out, reject, drop, alive
			expectedTags:   []string{"direct-out", "reject", "drop", "alive"},
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

func TestGetAvailableOutbounds_MemoByRevision(t *testing.T) {
	model := &wizardmodels.WizardModel{
		GlobalOutbounds: []config.Direction{{Tag: "memo-test-out", Type: "selector"}},
	}
	model.BumpRevision() // ревизия 0 = «мемо пусто», поэтому поднимаем
	a := GetAvailableOutbounds(model)
	b := GetAvailableOutbounds(model)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("second call should return same tags as memo hit: a=%v b=%v", a, b)
	}
	if model.AvailableOutboundsMemoRev != model.Revision {
		t.Fatalf("memo rev not set: got %d, want %d", model.AvailableOutboundsMemoRev, model.Revision)
	}
	InvalidatePreviewCache(model)
	if model.AvailableOutboundsMemoRev != 0 {
		t.Fatal("InvalidatePreviewCache should clear outbound memo")
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
		name                  string
		model                 *wizardmodels.WizardModel
		options               []string
		expectedFinalOutbound string
	}{
		{
			name: "Model with selected final outbound in options",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "test-outbound",
			},
			options:               []string{"direct-out", "test-outbound", "reject"},
			expectedFinalOutbound: "test-outbound",
		},
		// Note: TemplateData is a pointer to template.TemplateData which we can't easily create in test
		// So we skip testing template default fallback here - it's covered in integration tests
		{
			name: "Model without selected final, uses direct-out",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "",
			},
			options:               []string{"direct-out", "test-outbound", "reject"},
			expectedFinalOutbound: "direct-out",
		},
		{
			name: "Selected final not in options, falls back to first option",
			model: &wizardmodels.WizardModel{
				SelectedFinalOutbound: "not-in-options",
			},
			options:               []string{"direct-out", "test-outbound"},
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
