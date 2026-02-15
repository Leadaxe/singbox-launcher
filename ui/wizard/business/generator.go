// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл generator.go генерирует финальную конфигурацию sing-box из единого шаблона и модели визарда.
//
// BuildTemplateConfig собирает конфигурацию:
//  1. Нормализует ParserConfig (версия, last_updated)
//  2. Для каждой секции config из шаблона:
//     - outbounds: вставляет сгенерированные outbounds перед статическими
//     - route: добавляет включённые selectable rules, custom rules, rule_set и устанавливает final
//     - остальные секции: форматирует как есть
//  3. Оборачивает всё в JSONC с блоком @ParserConfig
//
// Используется в:
//   - presenter_save.go — для генерации конфигурации при сохранении
//   - presenter_async.go — для генерации preview конфигурации
package business

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// BuildTemplateConfig строит финальную конфигурацию из шаблона и модели визарда.
func BuildTemplateConfig(model *wizardmodels.WizardModel, forPreview bool) (string, error) {
	timing := debuglog.StartTiming("BuildTemplateConfig")
	defer timing.EndWithDefer()

	if model.TemplateData == nil {
		return "", fmt.Errorf("template data not available")
	}

	parserConfigText := strings.TrimSpace(model.ParserConfigJSON)
	if parserConfigText == "" {
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	// Нормализация ParserConfig (версия, last_updated)
	parserConfigText = normalizeParserConfig(parserConfigText, timing)

	// Сборка секций конфига
	sections, err := buildConfigSections(model, forPreview, timing)
	if err != nil {
		return "", err
	}

	if len(sections) == 0 {
		return "", fmt.Errorf("no config sections found")
	}

	// Финальная сборка: { @ParserConfig ... секции }
	var builder strings.Builder
	builder.WriteString("{\n")
	builder.WriteString("/** @ParserConfig\n")
	builder.WriteString(parserConfigText)
	builder.WriteString("\n*/\n")
	builder.WriteString(strings.Join(sections, ",\n"))
	builder.WriteString("\n}\n")

	return builder.String(), nil
}

// normalizeParserConfig нормализует ParserConfig JSON (версия, defaults, last_updated).
func normalizeParserConfig(text string, timing *debuglog.TimingContext) string {
	start := time.Now()
	var parserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(text), &parserConfig); err != nil {
		timing.LogTiming("parse ParserConfig", time.Since(start))
		return text
	}

	config.NormalizeParserConfig(&parserConfig, true)

	serialized, err := json.MarshalIndent(map[string]interface{}{
		"ParserConfig": parserConfig.ParserConfig,
	}, "", IndentBase)
	if err != nil {
		timing.LogTiming("serialize ParserConfig", time.Since(start))
		return text
	}

	timing.LogTiming("ParserConfig processing", time.Since(start))
	return string(serialized)
}

// buildConfigSections строит форматированные JSON-секции конфига.
func buildConfigSections(model *wizardmodels.WizardModel, forPreview bool, timing *debuglog.TimingContext) ([]string, error) {
	start := time.Now()
	var sections []string

	for _, key := range model.TemplateData.ConfigOrder {
		raw, ok := model.TemplateData.Config[key]
		if !ok {
			continue
		}

		var formatted string
		var err error

		switch key {
		case "outbounds":
			formatted, err = buildOutboundsSection(model, raw, forPreview, timing)
		case "route":
			formatted, err = buildRouteSection(model, raw, timing)
		default:
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
				err = nil
			}
		}

		if err != nil {
			return nil, err
		}

		sections = append(sections, fmt.Sprintf(`  "%s": %s`, key, formatted))
	}

	timing.LogTiming("build all sections", time.Since(start))
	return sections, nil
}

// buildOutboundsSection строит секцию outbounds: динамические между @ParserSTART/@ParserEND и статические из шаблона.
// Между блоками ставится запятая только если есть оба.
func buildOutboundsSection(model *wizardmodels.WizardModel, templateOutbounds json.RawMessage, forPreview bool, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build outbounds", time.Since(start)) }()

	indent := Indent(2)
	var builder strings.Builder
	builder.WriteString("[\n")

	// 1. Динамические outbounds между маркерами
	builder.WriteString(indent + "/** @ParserSTART */\n")
	if forPreview && model.OutboundStats.NodesCount > wizardutils.MaxNodesForFullPreview {
		builder.WriteString(fmt.Sprintf("%s// Generated: %d nodes, %d local selectors, %d global selectors\n",
			indent, model.OutboundStats.NodesCount, model.OutboundStats.LocalSelectorsCount, model.OutboundStats.GlobalSelectorsCount))
		builder.WriteString(fmt.Sprintf("%s// Total outbounds: %d\n", indent, len(model.GeneratedOutbounds)))
	} else {
		for idx, entry := range model.GeneratedOutbounds {
			cleaned := strings.TrimRight(entry, ",\n\r\t ")
			builder.WriteString(IndentMultiline(cleaned, indent))
			if idx < len(model.GeneratedOutbounds)-1 {
				builder.WriteString(",")
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString(indent + "/** @ParserEND */")

	// 2. Статические outbounds: запятая только если есть и динамические, и статические
	var staticOutbounds []json.RawMessage
	if err := json.Unmarshal(templateOutbounds, &staticOutbounds); err == nil && len(staticOutbounds) > 0 {
		needComma := len(model.GeneratedOutbounds) > 0
		for i, item := range staticOutbounds {
			if needComma || i > 0 {
				builder.WriteString(",\n")
			} else {
				builder.WriteString("\n")
			}
			formatted, err := formatCompactJSON(item, indent)
			if err != nil {
				formatted = string(item)
			}
			builder.WriteString(indent + formatted)
		}
	}

	builder.WriteString("\n  ]")
	return builder.String(), nil
}

// buildRouteSection строит секцию route с объединением правил и rule_set.
func buildRouteSection(model *wizardmodels.WizardModel, raw json.RawMessage, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build route", time.Since(start)) }()

	merged, err := MergeRouteSection(raw, model.SelectableRuleStates, model.CustomRules, model.SelectedFinalOutbound)
	if err != nil {
		return "", fmt.Errorf("route merge failed: %w", err)
	}

	formatted, err := FormatSectionJSON(merged, 2)
	if err != nil {
		return string(merged), nil
	}
	return formatted, nil
}

// MergeRouteSection объединяет selectable rules, custom rules и rule_set в секцию route.
func MergeRouteSection(raw json.RawMessage, states []*wizardmodels.RuleState, customRules []*wizardmodels.RuleState, finalOutbound string) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}

	// Существующие rules из шаблона
	var rules []interface{}
	if existing, ok := route["rules"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			rules = arr
		}
	}

	// Существующие rule_set из шаблона
	var ruleSets []interface{}
	if existing, ok := route["rule_set"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			ruleSets = arr
		}
	}

	// applyOutbound устанавливает outbound/action для правила
	applyOutbound := func(cloned map[string]interface{}, outbound string) {
		switch outbound {
		case wizardmodels.RejectActionName:
			delete(cloned, "outbound")
			cloned["action"] = wizardmodels.RejectActionName
			delete(cloned, "method")
		case "drop":
			delete(cloned, "outbound")
			cloned["action"] = wizardmodels.RejectActionName
			cloned["method"] = wizardmodels.RejectActionMethod
		default:
			if outbound != "" {
				cloned["outbound"] = outbound
				delete(cloned, "action")
				delete(cloned, "method")
			}
		}
	}

	// Обработка правил (selectable + custom)
	processRule := func(ruleState *wizardmodels.RuleState) {
		if !ruleState.Enabled {
			return
		}
		outbound := wizardmodels.GetEffectiveOutbound(ruleState)

		// Добавляем rule_set от этого правила
		for _, rs := range ruleState.Rule.RuleSets {
			var rsObj interface{}
			if err := json.Unmarshal(rs, &rsObj); err == nil {
				ruleSets = append(ruleSets, rsObj)
			}
		}

		// Добавляем правила маршрутизации
		if len(ruleState.Rule.Rules) > 0 {
			for _, r := range ruleState.Rule.Rules {
				cloned := copyMap(r)
				applyOutbound(cloned, outbound)
				rules = append(rules, cloned)
			}
		} else if ruleState.Rule.Rule != nil {
			cloned := copyMap(ruleState.Rule.Rule)
			applyOutbound(cloned, outbound)
			rules = append(rules, cloned)
		}
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
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}
	if finalOutbound != "" {
		route["final"] = finalOutbound
	}

	return json.Marshal(route)
}

// copyMap создаёт поверхностную копию map (достаточно для модификации outbound).
func copyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// formatCompactJSON форматирует JSON компактно с отступом.
func formatCompactJSON(raw json.RawMessage, indent string) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// IndentMultiline добавляет отступ к каждой строке многострочного текста.
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

// FormatSectionJSON форматирует JSON-секцию с указанным уровнем отступа.
func FormatSectionJSON(raw json.RawMessage, indentLevel int) (string, error) {
	var buf bytes.Buffer
	prefix := strings.Repeat(" ", indentLevel)
	if err := json.Indent(&buf, raw, prefix, "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}
