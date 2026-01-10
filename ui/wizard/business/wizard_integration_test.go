package business

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// TestDefaultWizardFlow_NextNextFinish simulates the most common user flow:
// open wizard -> click next-next-finish with default settings (no manual edits).
//
// It mirrors the actual wizard initialization and generation flow.
// The key requirement: generated config must be valid JSON.
func TestDefaultWizardFlow_NextNextFinish(t *testing.T) {
	execDir := findProjectRoot(t)

	// Load template (as wizard does on initialization)
	templateData, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}
	if templateData == nil {
		t.Fatalf("Template data is nil")
	}

	// Initialize wizard model
	model := wizardmodels.NewWizardModel()
	model.TemplateData = templateData
	model.ParserConfigJSON = strings.TrimSpace(templateData.ParserConfig)

	// Emulate user entering subscription URL (Page 1 of wizard)
	model.SourceURLs = "https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt"

	// Enable all sections by default (as wizard does)
	for _, key := range templateData.SectionOrder {
		if _, ok := model.TemplateSectionSelections[key]; !ok {
			model.TemplateSectionSelections[key] = true
		}
	}

	// Build selectable rule states with defaults (as wizard does)
	options := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	if len(model.SelectableRuleStates) == 0 {
		for _, rule := range templateData.SelectableRules {
			outbound := rule.DefaultOutbound
			if outbound == "" {
				outbound = options[0]
			}
			model.SelectableRuleStates = append(model.SelectableRuleStates, &wizardmodels.RuleState{
				Rule:             rule,
				SelectedOutbound: outbound,
				Enabled:          rule.IsDefault,
			})
		}
	}

	EnsureFinalSelected(model, options)

	// Generate preview config (page 3 of wizard)
	previewText, err := BuildTemplateConfig(model, true)
	if err != nil {
		t.Fatalf("Preview generation failed: %v", err)
	}

	// Preview must be valid JSONC (JSON with comments)
	if !jsonc.Valid([]byte(previewText)) {
		t.Errorf("Preview config is not valid JSONC (len=%d)", len(previewText))
		t.Logf("First 500 chars: %s", safeSubstring(previewText, 0, 500))
		t.Logf("Last 500 chars: %s", safeSubstring(previewText, len(previewText)-500, 500))
	}

	// Generate save config (final generation)
	saveText, err := BuildTemplateConfig(model, false)
	if err != nil {
		t.Fatalf("Save generation failed: %v", err)
	}

	// Saved config must be valid JSONC
	if !jsonc.Valid([]byte(saveText)) {
		t.Errorf("Saved config is not valid JSONC (len=%d)", len(saveText))
		t.Logf("First 500 chars: %s", safeSubstring(saveText, 0, 500))
		t.Logf("Last 500 chars: %s", safeSubstring(saveText, len(saveText)-500, 500))
	}

	t.Logf("✅ Default wizard flow completed successfully")
	t.Logf("   Preview config: %d bytes, valid JSONC", len(previewText))
	t.Logf("   Save config: %d bytes, valid JSONC", len(saveText))
}

// TestWizardFlowWithCustomRules tests wizard flow with custom rules added by user.
func TestWizardFlowWithCustomRules(t *testing.T) {
	execDir := findProjectRoot(t)

	templateData, err := wizardtemplate.LoadTemplateData(execDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	model := wizardmodels.NewWizardModel()
	model.TemplateData = templateData
	model.ParserConfigJSON = strings.TrimSpace(templateData.ParserConfig)

	// Emulate user entering subscription URL
	model.SourceURLs = "https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt"

	// Enable all sections
	for _, key := range templateData.SectionOrder {
		model.TemplateSectionSelections[key] = true
	}

	// Initialize rule states
	options := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	for _, rule := range templateData.SelectableRules {
		outbound := rule.DefaultOutbound
		if outbound == "" {
			outbound = options[0]
		}
		model.SelectableRuleStates = append(model.SelectableRuleStates, &wizardmodels.RuleState{
			Rule:             rule,
			SelectedOutbound: outbound,
			Enabled:          rule.IsDefault,
		})
	}

	// Add custom rule (user action)
	customRule := &wizardmodels.RuleState{
		Rule: wizardtemplate.TemplateSelectableRule{
			Label:       "Custom Test Rule",
			Description: "Test custom rule",
			Raw: map[string]interface{}{
				"domain":   []interface{}{"example.com"},
				"outbound": "direct-out",
			},
			HasOutbound: true,
		},
		Enabled:          true,
		SelectedOutbound: "direct-out",
	}
	model.CustomRules = append(model.CustomRules, customRule)

	EnsureFinalSelected(model, options)

	// Generate config
	configText, err := BuildTemplateConfig(model, false)
	if err != nil {
		t.Fatalf("Config generation failed: %v", err)
	}

	// Config must be valid JSONC
	if !jsonc.Valid([]byte(configText)) {
		t.Fatalf("Config with custom rules is not valid JSONC")
	}

	// Verify custom rule is in the config
	if !strings.Contains(configText, "example.com") {
		t.Error("Custom rule is missing from generated config")
	}

	t.Logf("✅ Wizard flow with custom rules completed successfully")
}

// findProjectRoot walks up the directory tree to find project root.
// Returns path to directory containing go.mod and bin/config_template.json
func findProjectRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Walk up until we find bin/config_template.json and go.mod
	dir := wd
	for i := 0; i < 10; i++ {
		goModPath := filepath.Join(dir, "go.mod")
		templatePath := filepath.Join(dir, "bin", "config_template.json")

		if fileExists(goModPath) && fileExists(templatePath) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("Project root not found from wd=%s (expected go.mod and bin/config_template.json)", wd)
	return ""
}

// fileExists checks if file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// safeSubstring returns substring safely, handling out-of-bounds
func safeSubstring(text string, start, length int) string {
	if start < 0 {
		start = 0
	}
	if start >= len(text) {
		return ""
	}
	end := start + length
	if end > len(text) {
		end = len(text)
	}
	return text[start:end]
}
