package models

import (
	"testing"

	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// TestGetEffectiveOutbound tests GetEffectiveOutbound function
func TestGetEffectiveOutbound(t *testing.T) {
	tests := []struct {
		name      string
		ruleState *RuleState
		expected  string
	}{
		{
			name: "Has SelectedOutbound",
			ruleState: &RuleState{
				SelectedOutbound: "selected-outbound",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "default-outbound",
				},
			},
			expected: "selected-outbound",
		},
		{
			name: "No SelectedOutbound, uses DefaultOutbound",
			ruleState: &RuleState{
				SelectedOutbound: "",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "default-outbound",
				},
			},
			expected: "default-outbound",
		},
		{
			name: "No SelectedOutbound and no DefaultOutbound",
			ruleState: &RuleState{
				SelectedOutbound: "",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEffectiveOutbound(tt.ruleState)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestEnsureDefaultOutbound tests EnsureDefaultOutbound function
func TestEnsureDefaultOutbound(t *testing.T) {
	tests := []struct {
		name               string
		ruleState          *RuleState
		availableOutbounds []string
		expectedSelected   string
	}{
		{
			name: "Already has SelectedOutbound",
			ruleState: &RuleState{
				SelectedOutbound: "already-selected",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "default-outbound",
				},
			},
			availableOutbounds: []string{"default-outbound", "other"},
			expectedSelected:   "already-selected",
		},
		{
			name: "No SelectedOutbound, uses DefaultOutbound from Rule",
			ruleState: &RuleState{
				SelectedOutbound: "",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "default-outbound",
				},
			},
			availableOutbounds: []string{"default-outbound", "other"},
			expectedSelected:   "default-outbound",
		},
		{
			name: "No SelectedOutbound, no DefaultOutbound, uses first available",
			ruleState: &RuleState{
				SelectedOutbound: "",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "",
				},
			},
			availableOutbounds: []string{"first-outbound", "second-outbound"},
			expectedSelected:   "first-outbound",
		},
		{
			name: "No SelectedOutbound, no available outbounds",
			ruleState: &RuleState{
				SelectedOutbound: "",
				Rule: wizardtemplate.TemplateSelectableRule{
					DefaultOutbound: "",
				},
			},
			availableOutbounds: []string{},
			expectedSelected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnsureDefaultOutbound(tt.ruleState, tt.availableOutbounds)
			if tt.ruleState.SelectedOutbound != tt.expectedSelected {
				t.Errorf("Expected SelectedOutbound %q, got %q", tt.expectedSelected, tt.ruleState.SelectedOutbound)
			}
		})
	}
}
