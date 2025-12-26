package state

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

const (
	DefaultOutboundTag = "direct-out"
	RejectActionName   = "reject"
	RejectActionMethod = "drop"
)

// WizardState stores the configuration wizard state.
type WizardState struct {
	Controller *core.AppController
	Window     fyne.Window

	// Tab 1: Sources & ParserConfig
	SourceURLEntry       *widget.Entry
	URLStatusLabel       *widget.Label
	ParserConfigEntry    *widget.Entry
	OutboundsPreview     *widget.Entry
	OutboundsPreviewText string // Store text for read-only mode
	CheckURLButton       *widget.Button
	CheckURLProgress     *widget.ProgressBar
	CheckURLPlaceholder  *canvas.Rectangle
	CheckURLContainer    fyne.CanvasObject
	CheckURLInProgress   bool
	ParseButton          *widget.Button
	ParserConfigUpdating bool

	// Parsed data
	ParserConfig       *config.ParserConfig
	GeneratedOutbounds []string
	// Statistics for preview (used when nodes > maxNodesForFullPreview)
	OutboundStats struct {
		NodesCount           int
		LocalSelectorsCount  int
		GlobalSelectorsCount int
	}

	// Template data for second tab
	TemplateData                *wizardtemplate.TemplateData
	TemplateSectionSelections   map[string]bool
	SelectableRuleStates        []*SelectableRuleState
	CustomRules                 []*SelectableRuleState // User-defined custom rules
	TemplatePreviewEntry        *widget.Entry
	TemplatePreviewText         string
	TemplatePreviewStatusLabel  *widget.Label
	ShowPreviewButton           *widget.Button
	FinalOutboundSelect         *widget.Select
	SelectedFinalOutbound       string
	PreviewNeedsParse           bool
	TemplatePreviewNeedsUpdate  bool // Flag for recalculating preview when Rules tab changes
	AutoParseInProgress         bool
	PreviewGenerationInProgress bool

	// Flag to prevent callbacks during programmatic updates
	UpdatingOutboundOptions bool

	// Navigation buttons
	CloseButton      *widget.Button
	PrevButton       *widget.Button
	NextButton       *widget.Button
	SaveButton       *widget.Button
	SaveProgress     *widget.ProgressBar
	SavePlaceholder  *canvas.Rectangle
	SaveInProgress   bool
	ButtonsContainer fyne.CanvasObject
	Tabs             *container.AppTabs

	// Track open rule edit dialogs to prevent multiple dialogs for the same rule
	OpenRuleDialogs map[int]fyne.Window // ruleIndex -> dialog window
}

// SelectableRuleState describes the state of a selectable rule.
type SelectableRuleState struct {
	Rule             wizardtemplate.TemplateSelectableRule
	Enabled          bool
	SelectedOutbound string
	OutboundSelect   *widget.Select
}

// SaveConfigWithBackup saves the configuration with backup.
func (state *WizardState) SaveConfigWithBackup(text string) (string, error) {
	// Validate JSON before saving (support JSONC with comments)
	jsonBytes := jsonc.ToJSON([]byte(text))
	var configJSON map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &configJSON); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	// Generate random secret
	randomSecret := generateRandomSecret(24)

	// Try to replace secret in original text, preserving comments
	// Look for secret inside clash_api block
	finalText := text
	secretReplaced := false

	// Try to find and replace secret using regex
	simpleSecretPattern := regexp.MustCompile(`("secret"\s*:\s*)"[^"]*"`)
	if simpleSecretPattern.MatchString(text) && strings.Contains(text, "clash_api") {
		// Replace existing secret (assume it's in clash_api)
		finalText = simpleSecretPattern.ReplaceAllString(text, fmt.Sprintf(`$1"%s"`, randomSecret))
		secretReplaced = true
	}

	if !secretReplaced {
		// Secret not found, need to add it via JSON parsing
		if experimental, ok := configJSON["experimental"].(map[string]interface{}); ok {
			if clashAPI, ok := experimental["clash_api"].(map[string]interface{}); ok {
				clashAPI["secret"] = randomSecret
			} else {
				// If clash_api doesn't exist, create it
				experimental["clash_api"] = map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				}
			}
		} else {
			// If experimental doesn't exist, create it
			configJSON["experimental"] = map[string]interface{}{
				"clash_api": map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				},
			}
		}

		// Serialize back to JSON with formatting
		finalJSONBytes, err := json.MarshalIndent(configJSON, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal config: %w", err)
		}
		finalText = string(finalJSONBytes)
	}

	configPath := state.Controller.FileService.ConfigPath
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
		backup := state.NextBackupPath(configPath)
		if err := os.Rename(configPath, backup); err != nil {
			return "", err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(configPath, []byte(finalText), 0o644); err != nil {
		return "", err
	}
	// Update config status in Core Dashboard if callback is set
	if state.Controller != nil && state.Controller.UIService != nil && state.Controller.UIService.UpdateConfigStatusFunc != nil {
		state.Controller.UIService.UpdateConfigStatusFunc()
	}
	return configPath, nil
}

// NextBackupPath generates the next backup path for a file.
func (state *WizardState) NextBackupPath(path string) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	candidate := filepath.Join(dir, fmt.Sprintf("%s-old%s", base, ext))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-old-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// SetCheckURLState manages the state of the Check button and progress bar.
func (state *WizardState) SetCheckURLState(statusText string, buttonText string, progress float64) {
	if statusText != "" && state.URLStatusLabel != nil {
		state.URLStatusLabel.SetText(statusText)
	}

	progressVisible := false
	if progress < 0 {
		// Hide progress
		if state.CheckURLProgress != nil {
			state.CheckURLProgress.Hide()
			state.CheckURLProgress.SetValue(0)
		}
	} else {
		// Show progress
		if state.CheckURLProgress != nil {
			state.CheckURLProgress.SetValue(progress)
			state.CheckURLProgress.Show()
		}
		progressVisible = true
	}

	buttonVisible := false
	if progressVisible {
		// If showing progress, button is hidden
		if state.CheckURLButton != nil {
			state.CheckURLButton.Hide()
		}
	} else if buttonText == "" {
		// Hide button
		if state.CheckURLButton != nil {
			state.CheckURLButton.Hide()
		}
	} else {
		// Show button
		if state.CheckURLButton != nil {
			state.CheckURLButton.SetText(buttonText)
			state.CheckURLButton.Show()
			state.CheckURLButton.Enable()
		}
		buttonVisible = true
	}

	// Manage placeholder
	if state.CheckURLPlaceholder != nil {
		if buttonVisible || progressVisible {
			state.CheckURLPlaceholder.Show()
		} else {
			state.CheckURLPlaceholder.Hide()
		}
	}
}

// SetSaveState manages the state of the Save button and progress bar.
func (state *WizardState) SetSaveState(buttonText string, progress float64) {
	progressVisible := false
	if progress < 0 {
		// Hide progress
		if state.SaveProgress != nil {
			state.SaveProgress.Hide()
			state.SaveProgress.SetValue(0)
		}
		state.SaveInProgress = false
	} else {
		// Show progress
		if state.SaveProgress != nil {
			state.SaveProgress.SetValue(progress)
			state.SaveProgress.Show()
		}
		progressVisible = true
		state.SaveInProgress = true
	}

	buttonVisible := false
	if progressVisible {
		// If showing progress, button is hidden
		if state.SaveButton != nil {
			state.SaveButton.Hide()
			state.SaveButton.Disable()
		}
	} else if buttonText == "" {
		// Hide button
		if state.SaveButton != nil {
			state.SaveButton.Hide()
			state.SaveButton.Disable()
		}
	} else {
		// Show button
		if state.SaveButton != nil {
			state.SaveButton.SetText(buttonText)
			state.SaveButton.Show()
			state.SaveButton.Enable()
		}
		buttonVisible = true
	}

	// Manage placeholder
	if state.SavePlaceholder != nil {
		if buttonVisible || progressVisible {
			state.SavePlaceholder.Show()
		} else {
			state.SavePlaceholder.Hide()
		}
	}
}

// SetTemplatePreviewText sets the preview text for the template.
func (state *WizardState) SetTemplatePreviewText(text string) {
	// Optimization: don't update if text hasn't changed
	if state.TemplatePreviewText == text {
		// If text hasn't changed, but preview was successfully set earlier,
		// reset flag (text is already correct, no need to recalculate)
		if state.TemplatePreviewNeedsUpdate && state.TemplatePreviewEntry != nil && state.TemplatePreviewEntry.Text == text {
			state.TemplatePreviewNeedsUpdate = false
		}
		return
	}

	state.TemplatePreviewText = text
	if state.TemplatePreviewEntry == nil {
		// If Entry hasn't been created yet, reset flag (will be set later)
		state.TemplatePreviewNeedsUpdate = false
		return
	}

	// Check if text in Entry has changed
	if state.TemplatePreviewEntry.Text == text {
		// Text is already set, reset flag
		state.TemplatePreviewNeedsUpdate = false
		return
	}

	DebugLog("setTemplatePreviewText: Setting preview text (length: %d bytes)", len(text))

	// For large texts (>50KB) show loading message before insertion
	if len(text) > 50000 {
		SafeFyneDo(state.Window, func() {
			state.TemplatePreviewEntry.SetText("Loading large preview...")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("⏳ Loading large preview...")
			}
		})

		// Insert large text asynchronously
		go func() {
			SafeFyneDo(state.Window, func() {
				insertStartTime := time.Now()
				state.TemplatePreviewEntry.SetText(text)
				// Reset flag after successful insertion of large text
				state.TemplatePreviewNeedsUpdate = false
				DebugLog("setTemplatePreviewText: Large text inserted in %v", time.Since(insertStartTime))
			})
		}()
	} else {
		// For normal texts use synchronous insertion
		SafeFyneDo(state.Window, func() {
			state.TemplatePreviewEntry.SetText(text)
			// Reset flag after successful text setting
			state.TemplatePreviewNeedsUpdate = false
		})
	}
}

// RefreshOutboundOptions refreshes the outbound options for all rules.
func (state *WizardState) RefreshOutboundOptions() {
	startTime := time.Now()
	DebugLog("refreshOutboundOptions: START at %s", startTime.Format("15:04:05.000"))

	if len(state.SelectableRuleStates) == 0 && state.FinalOutboundSelect == nil {
		DebugLog("refreshOutboundOptions: No rule states and no final select, returning early")
		return
	}

	getOptionsStartTime := time.Now()
	options := EnsureDefaultAvailableOutbounds(state.GetAvailableOutbounds())
	DebugLog("refreshOutboundOptions: getAvailableOutbounds took %v (found %d options)",
		time.Since(getOptionsStartTime), len(options))

	// Create map for fast lookup (O(1) instead of O(n))
	optionsMap := make(map[string]bool, len(options))
	for _, opt := range options {
		optionsMap[opt] = true
	}

	ensureSelected := func(ruleState *SelectableRuleState) {
		if !ruleState.Rule.HasOutbound {
			return
		}
		if ruleState.SelectedOutbound != "" && optionsMap[ruleState.SelectedOutbound] {
			return
		}
		candidate := ruleState.Rule.DefaultOutbound
		if candidate == "" || !optionsMap[candidate] {
			candidate = options[0]
		}
		ruleState.SelectedOutbound = candidate
	}

	state.EnsureFinalSelected(options)

	// Set flag to block callbacks during programmatic updates
	state.UpdatingOutboundOptions = true
	defer func() {
		state.UpdatingOutboundOptions = false
	}()

	uiUpdateStartTime := time.Now()
	SafeFyneDo(state.Window, func() {
		// Update outbound selectors for all rules with unified logic
		updateOutboundSelect := func(ruleState *SelectableRuleState) {
			if !ruleState.Rule.HasOutbound || ruleState.OutboundSelect == nil {
				return
			}
			ensureSelected(ruleState)
			ruleState.OutboundSelect.Options = options
			ruleState.OutboundSelect.SetSelected(ruleState.SelectedOutbound)
			ruleState.OutboundSelect.Refresh()
		}

		for _, ruleState := range state.SelectableRuleStates {
			updateOutboundSelect(ruleState)
		}
		for _, customRule := range state.CustomRules {
			updateOutboundSelect(customRule)
		}
		if state.FinalOutboundSelect != nil {
			state.FinalOutboundSelect.Options = options
			state.FinalOutboundSelect.SetSelected(state.SelectedFinalOutbound)
			state.FinalOutboundSelect.Refresh()
		}
	})
	DebugLog("refreshOutboundOptions: UI update took %v", time.Since(uiUpdateStartTime))
	DebugLog("refreshOutboundOptions: END (total duration: %v)", time.Since(startTime))
}

// EnsureFinalSelected ensures that the final outbound is selected from available options.
func (state *WizardState) EnsureFinalSelected(options []string) {
	options = EnsureDefaultAvailableOutbounds(options)
	preferred := state.SelectedFinalOutbound
	if preferred == "" && state.TemplateData != nil && state.TemplateData.DefaultFinal != "" {
		preferred = state.TemplateData.DefaultFinal
	}
	if preferred == "" {
		preferred = DefaultOutboundTag
	}
	if !ContainsString(options, preferred) {
		if state.TemplateData != nil && state.TemplateData.DefaultFinal != "" && ContainsString(options, state.TemplateData.DefaultFinal) {
			preferred = state.TemplateData.DefaultFinal
		} else if ContainsString(options, DefaultOutboundTag) {
			preferred = DefaultOutboundTag
		} else {
			preferred = options[0]
		}
	}
	state.SelectedFinalOutbound = preferred
}

// InitializeTemplateState initializes the template state.
func (state *WizardState) InitializeTemplateState() {
	if state.TemplateData == nil {
		return
	}
	if state.TemplateSectionSelections == nil {
		state.TemplateSectionSelections = make(map[string]bool)
	}
	for _, key := range state.TemplateData.SectionOrder {
		if _, ok := state.TemplateSectionSelections[key]; !ok {
			state.TemplateSectionSelections[key] = true
		}
	}
	options := EnsureDefaultAvailableOutbounds(state.GetAvailableOutbounds())

	if len(state.SelectableRuleStates) == 0 {
		for _, rule := range state.TemplateData.SelectableRules {
			outbound := rule.DefaultOutbound
			if outbound == "" {
				outbound = options[0]
			}
			state.SelectableRuleStates = append(state.SelectableRuleStates, &SelectableRuleState{
				Rule:             rule,
				SelectedOutbound: outbound,
				Enabled:          rule.IsDefault, // Enable rule if @default directive is present
			})
		}
	} else {
		for _, ruleState := range state.SelectableRuleStates {
			EnsureDefaultOutbound(ruleState, options)
		}
	}

	state.EnsureFinalSelected(options)
	// Don't call updateTemplatePreview here - it will be called after creating all tabs
}

// GetAvailableOutbounds returns a list of available outbound tags.
func (state *WizardState) GetAvailableOutbounds() []string {
	tags := map[string]struct{}{
		DefaultOutboundTag: {},
		RejectActionName:   {},
		"drop":             {}, // Always include "drop" in available options
	}

	var parserCfg *config.ParserConfig
	if state.ParserConfig != nil {
		parserCfg = state.ParserConfig
	} else if state.ParserConfigEntry != nil && state.ParserConfigEntry.Text != "" {
		var parsed config.ParserConfig
		if err := json.Unmarshal([]byte(state.ParserConfigEntry.Text), &parsed); err == nil {
			parserCfg = &parsed
		}
	}
	if parserCfg != nil {
		// Add global outbounds
		for _, outbound := range parserCfg.ParserConfig.Outbounds {
			// Skip outbounds with "wizard":"hide" or "wizard":{"hide":true}
			if outbound.IsWizardHidden() {
				continue
			}
			if outbound.Tag != "" {
				tags[outbound.Tag] = struct{}{}
			}
			for _, extra := range outbound.AddOutbounds {
				tags[extra] = struct{}{}
			}
		}
		// Add local outbounds from all ProxySource
		for _, proxySource := range parserCfg.ParserConfig.Proxies {
			for _, outbound := range proxySource.Outbounds {
				// Skip outbounds with "wizard":"hide" or "wizard":{"hide":true}
				if outbound.IsWizardHidden() {
					continue
				}
				if outbound.Tag != "" {
					tags[outbound.Tag] = struct{}{}
				}
				for _, extra := range outbound.AddOutbounds {
					tags[extra] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// generateRandomSecret generates a random secret string.
func generateRandomSecret(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based secret if random generation fails
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:length]
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}

// RefreshRulesTab refreshes the Rules tab content.
// createRulesTab is a function that creates the Rules tab UI.
// It can accept either a function with one parameter (state) or two parameters (state, showAddRuleDialog).
func (state *WizardState) RefreshRulesTab(createRulesTab interface{}) {
	if state.Tabs == nil {
		return
	}

	for _, tab := range state.Tabs.Items {
		if tab.Text == "Rules" {
			var newContent fyne.CanvasObject
			// Try to call with one parameter (state)
			if fn, ok := createRulesTab.(func(*WizardState) fyne.CanvasObject); ok {
				newContent = fn(state)
			} else {
				return
			}
			tab.Content = newContent
			state.Tabs.Refresh()
			break
		}
	}
}

// Helper functions (moved from utils to avoid circular imports)

// ContainsString checks if a string slice contains a target string.
func ContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// EnsureDefaultAvailableOutbounds sets default outbounds if the list is empty.
func EnsureDefaultAvailableOutbounds(outbounds []string) []string {
	if len(outbounds) == 0 {
		return []string{DefaultOutboundTag, RejectActionName}
	}
	return outbounds
}

// EnsureDefaultOutbound ensures that a rule state has a selected outbound.
func EnsureDefaultOutbound(ruleState *SelectableRuleState, availableOutbounds []string) {
	if ruleState.SelectedOutbound == "" {
		if ruleState.Rule.DefaultOutbound != "" {
			ruleState.SelectedOutbound = ruleState.Rule.DefaultOutbound
		} else if len(availableOutbounds) > 0 {
			ruleState.SelectedOutbound = availableOutbounds[0]
		}
	}
}

// GetEffectiveOutbound returns the effective outbound for a rule (SelectedOutbound or DefaultOutbound).
func GetEffectiveOutbound(ruleState *SelectableRuleState) string {
	if ruleState.SelectedOutbound != "" {
		return ruleState.SelectedOutbound
	}
	return ruleState.Rule.DefaultOutbound
}
