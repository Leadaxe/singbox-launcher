// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл generator.go содержит функции генерации финальной конфигурации из шаблона и модели визарда:
//   - BuildTemplateConfig - собирает финальную конфигурацию из шаблона, ParserConfig и выбранных секций
//   - BuildParserOutboundsBlock - формирует блок outbounds из сгенерированных outbounds
//   - MergeRouteSection - объединяет правила маршрутизации из шаблона и пользовательские правила
//   - FormatSectionJSON, IndentMultiline - вспомогательные функции форматирования JSON
//
// Эти функции работают только с данными (WizardModel, template, JSON), без зависимостей от GUI.
// Они возвращают готовый текст конфигурации для сохранения или preview.
//
// Генерация конфигурации - это отдельная ответственность от парсинга (parser.go) и валидации (validator.go).
// Содержит сложную логику слияния правил маршрутизации и форматирования JSON секций.
// Используется как презентером (presenter_save.go, presenter_async.go), так и напрямую в бизнес-логике.
//
// Используется в:
//   - presenter_save.go - для генерации конфигурации при сохранении
//   - presenter_async.go - для генерации preview конфигурации
package business

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// BuildTemplateConfig builds the final configuration from template and wizard model.
// It processes all selected sections, merges route rules, and generates outbounds block.
func BuildTemplateConfig(model *wizardmodels.WizardModel, forPreview bool) (string, error) {
	startTime := time.Now()
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: START at %s", startTime.Format("15:04:05.000"))

	if model.TemplateData == nil {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: TemplateData is nil, returning error")
		return "", fmt.Errorf("template data not available")
	}
	parserConfigText := strings.TrimSpace(model.ParserConfigJSON)
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: ParserConfig text length: %d bytes", len(parserConfigText))
	if parserConfigText == "" {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: ParserConfig is empty, returning error")
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	// Parse ParserConfig JSON to ensure it has version 2 and parser object
	parseStartTime := time.Now()
	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigText), &parserConfig); err != nil {
		// If parsing fails, use text as-is (might be invalid JSON, but let user fix it)
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Failed to parse ParserConfig JSON (took %v): %v", time.Since(parseStartTime), err)
	} else {
		// Normalize ParserConfig (migrate version, set defaults, update last_updated)
		normalizeStartTime := time.Now()
		config.NormalizeParserConfig(&parserConfig, true)
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Normalized ParserConfig in %v", time.Since(normalizeStartTime))

		// Serialize back to JSON with proper formatting (always version 2 format)
		serializeStartTime := time.Now()
		configToSerialize := map[string]interface{}{
			"ParserConfig": parserConfig.ParserConfig,
		}
		serialized, err := json.MarshalIndent(configToSerialize, "", IndentBase)
		if err == nil {
			parserConfigText = string(serialized)
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Serialized ParserConfig in %v (new length: %d bytes)",
				time.Since(serializeStartTime), len(parserConfigText))
		} else {
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Failed to serialize ParserConfig (took %v): %v",
				time.Since(serializeStartTime), err)
		}
	}
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: ParserConfig processing took %v total", time.Since(parseStartTime))

	sectionsStartTime := time.Now()
	sections := make([]string, 0)
	sectionCount := 0
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Processing %d sections", len(model.TemplateData.SectionOrder))
	for _, key := range model.TemplateData.SectionOrder {
		sectionStartTime := time.Now()
		if selected, ok := model.TemplateSectionSelections[key]; !ok || !selected {
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Section '%s' not selected, skipping", key)
			continue
		}
		raw := model.TemplateData.Sections[key]
		var formatted string
		var err error
		if key == "outbounds" && model.TemplateData.HasParserOutboundsBlock {
			// If template had @PARSER_OUTBOUNDS_BLOCK marker, replace entire outbounds array
			// with generated content
			outboundsStartTime := time.Now()
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Building outbounds block (generated outbounds: %d)",
				len(model.GeneratedOutbounds))
			content := BuildParserOutboundsBlock(model, model.TemplateData, forPreview)
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Built outbounds block in %v (content length: %d bytes)",
				time.Since(outboundsStartTime), len(content))

			// Add elements after marker if they exist (any elements, not just direct-out)
			if model.TemplateData.OutboundsAfterMarker != "" {
				// Remove extra spaces and commas
				cleaned := strings.TrimSpace(model.TemplateData.OutboundsAfterMarker)
				cleaned = strings.TrimRight(cleaned, ",")
				if cleaned != "" {
					indented := IndentMultiline(cleaned, Indent(2))
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
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Merging route section (template rules: %d, custom rules: %d)",
				len(model.SelectableRuleStates), len(model.CustomRules))
			merged, err := MergeRouteSection(raw, model.SelectableRuleStates, model.CustomRules, model.SelectedFinalOutbound)
			if err != nil {
				debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Route merge failed (took %v): %v",
					time.Since(routeStartTime), err)
				return "", fmt.Errorf("route merge failed: %w", err)
			}
			raw = merged
			formatStartTime := time.Now()
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Formatted route section in %v (total route processing: %v)",
				time.Since(formatStartTime), time.Since(routeStartTime))
		} else {
			formatStartTime := time.Now()
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Formatted section '%s' in %v", key, time.Since(formatStartTime))
		}
		sections = append(sections, fmt.Sprintf(`  "%s": %s`, key, formatted))
		sectionCount++
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Processed section '%s' in %v (total sections processed: %d)",
			key, time.Since(sectionStartTime), sectionCount)
	}
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Processed all sections in %v (total: %d)",
		time.Since(sectionsStartTime), sectionCount)

	if len(sections) == 0 {
		debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: No sections selected, returning error")
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
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: Built final config in %v (result length: %d bytes)",
		time.Since(buildStartTime), len(result))
	debuglog.Log("DEBUG", debuglog.LevelVerbose, debuglog.UseGlobal, "buildTemplateConfig: END (total duration: %v)", time.Since(startTime))
	return result, nil
}

// BuildParserOutboundsBlock builds the outbounds block content from generated outbounds.
// If forPreview is true and nodes count exceeds MaxNodesForFullPreview, shows statistics instead.
func BuildParserOutboundsBlock(model *wizardmodels.WizardModel, templateData *wizardtemplate.TemplateData, forPreview bool) string {
	indent := Indent(2)
	var builder strings.Builder
	builder.WriteString(indent + "/** @ParserSTART */\n")

	// If this is for preview window (forPreview == true) AND nodes count exceeds MaxNodesForFullPreview, show statistics
	if forPreview && model.OutboundStats.NodesCount > wizardutils.MaxNodesForFullPreview {
		statsComment := fmt.Sprintf(`%s// Generated: %d nodes, %d local selectors, %d global selectors
%s// Total outbounds: %d
`,
			indent,
			model.OutboundStats.NodesCount,
			model.OutboundStats.LocalSelectorsCount,
			model.OutboundStats.GlobalSelectorsCount,
			indent,
			len(model.GeneratedOutbounds))
		builder.WriteString(statsComment)
	} else {
		// For file (forPreview == false) or for small number of nodes - show full list
		count := len(model.GeneratedOutbounds)
		// Check if there are elements after marker (any elements, not just direct-out)
		hasAfterMarker := templateData != nil &&
			strings.TrimSpace(templateData.OutboundsAfterMarker) != ""

		for idx, entry := range model.GeneratedOutbounds {
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
func MergeRouteSection(raw json.RawMessage, states []*wizardmodels.RuleState, customRules []*wizardmodels.RuleState, finalOutbound string) (json.RawMessage, error) {
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
		if outbound == wizardmodels.RejectActionName {
			// User selected reject - set action: reject without method, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = wizardmodels.RejectActionName
			delete(cloned, "method")
		} else if outbound == "drop" {
			// User selected drop - set action: reject with method: drop, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = wizardmodels.RejectActionName
			cloned["method"] = wizardmodels.RejectActionMethod
		} else if outbound != "" {
			// User selected regular outbound - set outbound, remove action and method
			cloned["outbound"] = outbound
			delete(cloned, "action")
			delete(cloned, "method")
		}
	}

	// Process template and custom rules with unified logic
	processRule := func(ruleState *wizardmodels.RuleState) {
		if !ruleState.Enabled {
			return
		}
		cloned := cloneRule(ruleState.Rule)
		outbound := wizardmodels.GetEffectiveOutbound(ruleState)
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
// It normalizes process_name fields based on the platform (removes .exe on macOS/Linux).
func cloneRule(rule wizardtemplate.TemplateSelectableRule) map[string]interface{} {
	cloned := make(map[string]interface{}, len(rule.Raw))
	for key, value := range rule.Raw {
		// Normalize process_name field for non-Windows platforms
		if key == "process_name" && runtime.GOOS != "windows" {
			normalized := normalizeProcessNames(value)
			cloned[key] = normalized
		} else {
			cloned[key] = value
		}
	}
	return cloned
}

// normalizeProcessNames removes .exe extensions from process names on non-Windows platforms.
// It handles both single string values and arrays of strings.
func normalizeProcessNames(value interface{}) interface{} {
	switch v := value.(type) {
	case []interface{}:
		// Array of process names
		normalized := make([]interface{}, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				normalizedName := strings.TrimSuffix(str, ".exe")
				normalized = append(normalized, normalizedName)
			} else {
				normalized = append(normalized, item)
			}
		}
		return normalized
	case []string:
		// Array of strings (direct type)
		normalized := make([]string, 0, len(v))
		for _, str := range v {
			normalizedName := strings.TrimSuffix(str, ".exe")
			normalized = append(normalized, normalizedName)
		}
		return normalized
	case string:
		// Single string value
		return strings.TrimSuffix(v, ".exe")
	default:
		// Unknown type, return as-is
		return value
	}
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

// TriggerParseForPreview and UpdateTemplatePreviewAsync were moved to presentation layer.
// They directly manipulate GUI widgets and should be in presenter, not business logic.
