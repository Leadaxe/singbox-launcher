package business

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"singbox-launcher/core/config"
	wizardstate "singbox-launcher/ui/wizard/state"
	wizardtemplate "singbox-launcher/ui/wizard/template"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// BuildTemplateConfig builds the final configuration from template and wizard state.
// It processes all selected sections, merges route rules, and generates outbounds block.
func BuildTemplateConfig(state *wizardstate.WizardState, forPreview bool) (string, error) {
	startTime := time.Now()
	wizardstate.DebugLog("buildTemplateConfig: START at %s", startTime.Format("15:04:05.000"))

	if state.TemplateData == nil {
		wizardstate.DebugLog("buildTemplateConfig: TemplateData is nil, returning error")
		return "", fmt.Errorf("template data not available")
	}
	parserConfigText := strings.TrimSpace(state.ParserConfigEntry.Text)
	wizardstate.DebugLog("buildTemplateConfig: ParserConfig text length: %d bytes", len(parserConfigText))
	if parserConfigText == "" {
		wizardstate.DebugLog("buildTemplateConfig: ParserConfig is empty, returning error")
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	// Parse ParserConfig JSON to ensure it has version 2 and parser object
	parseStartTime := time.Now()
	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigText), &parserConfig); err != nil {
		// If parsing fails, use text as-is (might be invalid JSON, but let user fix it)
		wizardstate.DebugLog("buildTemplateConfig: Failed to parse ParserConfig JSON (took %v): %v", time.Since(parseStartTime), err)
	} else {
		// Normalize ParserConfig (migrate version, set defaults, update last_updated)
		normalizeStartTime := time.Now()
		config.NormalizeParserConfig(&parserConfig, true)
		wizardstate.DebugLog("buildTemplateConfig: Normalized ParserConfig in %v", time.Since(normalizeStartTime))

		// Serialize back to JSON with proper formatting (always version 2 format)
		serializeStartTime := time.Now()
		configToSerialize := map[string]interface{}{
			"ParserConfig": parserConfig.ParserConfig,
		}
		serialized, err := json.MarshalIndent(configToSerialize, "", "  ")
		if err == nil {
			parserConfigText = string(serialized)
			wizardstate.DebugLog("buildTemplateConfig: Serialized ParserConfig in %v (new length: %d bytes)",
				time.Since(serializeStartTime), len(parserConfigText))
		} else {
			wizardstate.DebugLog("buildTemplateConfig: Failed to serialize ParserConfig (took %v): %v",
				time.Since(serializeStartTime), err)
		}
	}
	wizardstate.DebugLog("buildTemplateConfig: ParserConfig processing took %v total", time.Since(parseStartTime))

	sectionsStartTime := time.Now()
	sections := make([]string, 0)
	sectionCount := 0
	wizardstate.DebugLog("buildTemplateConfig: Processing %d sections", len(state.TemplateData.SectionOrder))
	for _, key := range state.TemplateData.SectionOrder {
		sectionStartTime := time.Now()
		if selected, ok := state.TemplateSectionSelections[key]; !ok || !selected {
			wizardstate.DebugLog("buildTemplateConfig: Section '%s' not selected, skipping", key)
			continue
		}
		raw := state.TemplateData.Sections[key]
		var formatted string
		var err error
		if key == "outbounds" && state.TemplateData.HasParserOutboundsBlock {
			// If template had @PARSER_OUTBOUNDS_BLOCK marker, replace entire outbounds array
			// with generated content
			outboundsStartTime := time.Now()
			wizardstate.DebugLog("buildTemplateConfig: Building outbounds block (generated outbounds: %d)",
				len(state.GeneratedOutbounds))
			content := BuildParserOutboundsBlock(state, forPreview)
			wizardstate.DebugLog("buildTemplateConfig: Built outbounds block in %v (content length: %d bytes)",
				time.Since(outboundsStartTime), len(content))

			// Add elements after marker if they exist (any elements, not just direct-out)
			if state.TemplateData.OutboundsAfterMarker != "" {
				// Remove extra spaces and commas
				cleaned := strings.TrimSpace(state.TemplateData.OutboundsAfterMarker)
				cleaned = strings.TrimRight(cleaned, ",")
				if cleaned != "" {
					indented := IndentMultiline(cleaned, "    ")
					// Do NOT add comma before elements - it's already there after last element before @ParserEND
					content += "\n" + indented
				}
			}
			// Always add \n at the end of content before closing bracket
			content += "\n"

			// Wrap content in array brackets
			formatted = "[\n" + content + "\n  ]"
		} else if key == "route" {
			routeStartTime := time.Now()
			wizardstate.DebugLog("buildTemplateConfig: Merging route section (template rules: %d, custom rules: %d)",
				len(state.SelectableRuleStates), len(state.CustomRules))
			merged, err := MergeRouteSection(raw, state.SelectableRuleStates, state.CustomRules, state.SelectedFinalOutbound)
			if err != nil {
				wizardstate.DebugLog("buildTemplateConfig: Route merge failed (took %v): %v",
					time.Since(routeStartTime), err)
				return "", fmt.Errorf("route merge failed: %w", err)
			}
			raw = merged
			formatStartTime := time.Now()
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			wizardstate.DebugLog("buildTemplateConfig: Formatted route section in %v (total route processing: %v)",
				time.Since(formatStartTime), time.Since(routeStartTime))
		} else {
			formatStartTime := time.Now()
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			wizardstate.DebugLog("buildTemplateConfig: Formatted section '%s' in %v", key, time.Since(formatStartTime))
		}
		sections = append(sections, fmt.Sprintf(`  "%s": %s`, key, formatted))
		sectionCount++
		wizardstate.DebugLog("buildTemplateConfig: Processed section '%s' in %v (total sections processed: %d)",
			key, time.Since(sectionStartTime), sectionCount)
	}
	wizardstate.DebugLog("buildTemplateConfig: Processed all sections in %v (total: %d)",
		time.Since(sectionsStartTime), sectionCount)

	if len(sections) == 0 {
		wizardstate.DebugLog("buildTemplateConfig: No sections selected, returning error")
		return "", fmt.Errorf("no sections selected")
	}

	buildStartTime := time.Now()
	var builder strings.Builder
	builder.WriteString("{\n")
	builder.WriteString("/** @ParserConfig\n")
	builder.WriteString(parserConfigText)
	builder.WriteString("\n*/\n")
	builder.WriteString(strings.Join(sections, ",\n"))
	builder.WriteString("\n}\n")
	result := builder.String()
	wizardstate.DebugLog("buildTemplateConfig: Built final config in %v (result length: %d bytes)",
		time.Since(buildStartTime), len(result))
	wizardstate.DebugLog("buildTemplateConfig: END (total duration: %v)", time.Since(startTime))
	return result, nil
}

// BuildParserOutboundsBlock builds the outbounds block content from generated outbounds.
// If forPreview is true and nodes count exceeds MaxNodesForFullPreview, shows statistics instead.
func BuildParserOutboundsBlock(state *wizardstate.WizardState, forPreview bool) string {
	const indent = "    "
	var builder strings.Builder
	builder.WriteString(indent + "/** @ParserSTART */\n")

	// If this is for preview window (forPreview == true) AND nodes count exceeds MaxNodesForFullPreview, show statistics
	if forPreview && state.OutboundStats.NodesCount > wizardutils.MaxNodesForFullPreview {
		statsComment := fmt.Sprintf(`%s// Generated: %d nodes, %d local selectors, %d global selectors
%s// Total outbounds: %d
`,
			indent,
			state.OutboundStats.NodesCount,
			state.OutboundStats.LocalSelectorsCount,
			state.OutboundStats.GlobalSelectorsCount,
			indent,
			len(state.GeneratedOutbounds))
		builder.WriteString(statsComment)
	} else {
		// For file (forPreview == false) or for small number of nodes - show full list
		count := len(state.GeneratedOutbounds)
		// Check if there are elements after marker (any elements, not just direct-out)
		hasAfterMarker := state.TemplateData != nil &&
			strings.TrimSpace(state.TemplateData.OutboundsAfterMarker) != ""

		for idx, entry := range state.GeneratedOutbounds {
			// Remove commas and spaces at the end of line if present
			cleaned := strings.TrimRight(entry, ",\n\r\t ")
			indented := IndentMultiline(cleaned, indent)
			builder.WriteString(indented)
			// Add comma:
			// - if not last element (always)
			// - or if last element AND there are elements after marker
			if idx < count-1 || hasAfterMarker {
				builder.WriteString(",")
			}
			builder.WriteString("\n")
		}
	}

	endLine := indent + "/** @ParserEND */"
	builder.WriteString(endLine) // Without comma and without \n
	return builder.String()
}

// MergeRouteSection merges selectable rules and custom rules into route section.
// It applies outbound selections to rules and sets final outbound.
func MergeRouteSection(raw json.RawMessage, states []*wizardstate.SelectableRuleState, customRules []*wizardstate.SelectableRuleState, finalOutbound string) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}
	var rules []interface{}
	if existing, ok := route["rules"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			rules = arr
		} else {
			rules = []interface{}{existing}
		}
	}

	// applyOutboundToRule applies outbound to cloned rule (handles reject/drop)
	applyOutboundToRule := func(cloned map[string]interface{}, outbound string) {
		if outbound == wizardstate.RejectActionName {
			// User selected reject - set action: reject without method, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = wizardstate.RejectActionName
			delete(cloned, "method")
		} else if outbound == "drop" {
			// User selected drop - set action: reject with method: drop, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = wizardstate.RejectActionName
			cloned["method"] = wizardstate.RejectActionMethod
		} else if outbound != "" {
			// User selected regular outbound - set outbound, remove action and method
			cloned["outbound"] = outbound
			delete(cloned, "action")
			delete(cloned, "method")
		}
	}

	// Process template and custom rules with unified logic
	processRule := func(ruleState *wizardstate.SelectableRuleState) {
		if !ruleState.Enabled {
			return
		}
		cloned := cloneRule(ruleState.Rule)
		outbound := wizardstate.GetEffectiveOutbound(ruleState)
		applyOutboundToRule(cloned, outbound)
		rules = append(rules, cloned)
	}

	for _, state := range states {
		processRule(state)
	}

	for _, customRule := range customRules {
		processRule(customRule)
	}

	if len(rules) > 0 {
		route["rules"] = rules
	}
	if finalOutbound != "" {
		route["final"] = finalOutbound
	}
	return json.Marshal(route)
}

// cloneRule creates a deep copy of a rule from its raw data.
func cloneRule(rule wizardtemplate.TemplateSelectableRule) map[string]interface{} {
	cloned := make(map[string]interface{}, len(rule.Raw))
	for key, value := range rule.Raw {
		cloned[key] = value
	}
	return cloned
}

// IndentMultiline indents each line of multiline text with the specified indent string.
func IndentMultiline(text, indent string) string {
	if text == "" {
		return indent
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// FormatSectionJSON formats a JSON section with specified indentation level.
func FormatSectionJSON(raw json.RawMessage, indentLevel int) (string, error) {
	var buf bytes.Buffer
	prefix := strings.Repeat(" ", indentLevel)
	if err := json.Indent(&buf, raw, prefix, "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TriggerParseForPreview triggers parsing for preview.
func TriggerParseForPreview(state *wizardstate.WizardState) {
	if state.AutoParseInProgress {
		return
	}
	if !state.PreviewNeedsParse && len(state.GeneratedOutbounds) > 0 {
		return
	}
	if state.SourceURLEntry == nil || state.ParserConfigEntry == nil {
		return
	}
	if strings.TrimSpace(state.SourceURLEntry.Text) == "" || strings.TrimSpace(state.ParserConfigEntry.Text) == "" {
		return
	}
	state.AutoParseInProgress = true
	// Update status and disable Save button when parsing starts
	wizardstate.SafeFyneDo(state.Window, func() {
		if state.SaveButton != nil {
			state.SaveButton.Disable()
		}
		// Show parsing status on 3rd tab
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Parsing subscriptions and generating outbounds...")
		}
		if state.TemplatePreviewEntry != nil {
			state.SetTemplatePreviewText("Parsing configuration... Please wait.")
		}
	})
	// Call ParseAndPreview from parser.go (same package 'business')
	go ParseAndPreview(state)
}

// UpdateTemplatePreviewAsync updates the template preview asynchronously.
func UpdateTemplatePreviewAsync(state *wizardstate.WizardState) {
	startTime := time.Now()
	wizardstate.DebugLog("updateTemplatePreviewAsync: START at %s", startTime.Format("15:04:05.000"))

	// Prevent multiple simultaneous calls
	if state.PreviewGenerationInProgress {
		wizardstate.DebugLog("updateTemplatePreviewAsync: Preview generation already in progress, skipping")
		return
	}

	if state.TemplateData == nil || state.TemplatePreviewEntry == nil {
		wizardstate.DebugLog("updateTemplatePreviewAsync: TemplateData or TemplatePreviewEntry is nil, returning early")
		return
	}

	// Set generation flag and disable Save button
	state.PreviewGenerationInProgress = true
	wizardstate.SafeFyneDo(state.Window, func() {
		if state.TemplatePreviewEntry != nil {
			state.SetTemplatePreviewText("Building preview...")
		}
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Building preview configuration...")
		}
		// Disable Save button during generation
		if state.SaveButton != nil {
			state.SaveButton.Disable()
		}
	})

	// Build config asynchronously
	go func() {
		goroutineStartTime := time.Now()
		wizardstate.DebugLog("updateTemplatePreviewAsync: Goroutine START at %s", goroutineStartTime.Format("15:04:05.000"))

		defer func() {
			totalDuration := time.Since(goroutineStartTime)
			wizardstate.DebugLog("updateTemplatePreviewAsync: Goroutine END (duration: %v)", totalDuration)
			state.PreviewGenerationInProgress = false
			wizardstate.SafeFyneDo(state.Window, func() {
				// Enable Save button after completion
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
				// Enable Show Preview button
				if state.ShowPreviewButton != nil {
					state.ShowPreviewButton.Enable()
				}
			})
		}()

		// Update status: parsing ParserConfig
		wizardstate.SafeFyneDo(state.Window, func() {
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("⏳ Parsing ParserConfig...")
			}
		})

		buildStartTime := time.Now()
		wizardstate.DebugLog("updateTemplatePreviewAsync: Calling buildTemplateConfig")
		text, err := BuildTemplateConfig(state, true)
		buildDuration := time.Since(buildStartTime)
		if err != nil {
			wizardstate.DebugLog("updateTemplatePreviewAsync: buildTemplateConfig failed (took %v): %v", buildDuration, err)
			wizardstate.SafeFyneDo(state.Window, func() {
				errorText := fmt.Sprintf("Preview error: %v", err)
				state.SetTemplatePreviewText(errorText)
				// Reset flag even on error, to avoid repeated attempts on next Preview opening
				// (if user hasn't changed anything)
				state.TemplatePreviewNeedsUpdate = false
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: %v", err))
				}
			})
			return
		}
		wizardstate.DebugLog("updateTemplatePreviewAsync: buildTemplateConfig completed in %v (result size: %d bytes)",
			buildDuration, len(text))

		// Update preview text
		// SetTemplatePreviewText will reset templatePreviewNeedsUpdate flag after successful text setting
		isLargeText := len(text) > 50000
		wizardstate.SafeFyneDo(state.Window, func() {
			state.SetTemplatePreviewText(text)

			// Update status only for small texts
			// For large texts status will update after async insertion completes
			if !isLargeText {
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText("✅ Preview ready")
				}
				if state.ShowPreviewButton != nil {
					state.ShowPreviewButton.Enable()
				}
			}
		})
		if !isLargeText {
			wizardstate.DebugLog("updateTemplatePreviewAsync: Preview text inserted")
		} else {
			wizardstate.DebugLog("updateTemplatePreviewAsync: Large text insertion started (status will update when complete)")
		}
	}()
}
