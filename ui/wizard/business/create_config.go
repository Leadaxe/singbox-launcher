// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл create_config.go генерирует финальную конфигурацию sing-box из единого шаблона и модели визарда.
//
// BuildTemplateConfig собирает конфигурацию:
//  1. Нормализует ParserConfig (версия, last_updated)
//  2. Для каждой секции config из шаблона:
//     - outbounds: вставляет сгенерированные outbounds перед статическими
//     - route: база из шаблона (статические rules/rule_set, final из шаблона), затем правила и rule_set из custom_rules модели
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
	"runtime"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// MaterializeClashSecretIfNeeded один раз кладёт сгенерированный clash_secret в model.SettingsVars,
// пока ключа нет (после Сброса ключа снова нет — сгенерируется снова при следующем вызове).
// Иначе MaybeGenerateClashSecret при каждом GetEffectiveConfig выдавал бы новый секрет в превью.
func MaterializeClashSecretIfNeeded(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	td := model.TemplateData
	hasVar := false
	for _, v := range td.Vars {
		if v.Name == "clash_secret" {
			hasVar = true
			break
		}
	}
	if !hasVar {
		return
	}
	if model.SettingsVars == nil {
		model.SettingsVars = make(map[string]string)
	}
	if s, ok := model.SettingsVars["clash_secret"]; ok && !wizardtemplate.ClashSecretUnresolved(s) {
		return
	}
	resolved := wizardtemplate.ResolveTemplateVars(td.Vars, model.SettingsVars, td.RawTemplate)
	wizardtemplate.MaybeGenerateClashSecret(resolved)
	if rv, ok := resolved["clash_secret"]; ok {
		if s := strings.TrimSpace(rv.Scalar); s != "" {
			model.SettingsVars["clash_secret"] = s
		}
	}
}

// BuildTemplateConfig строит финальную конфигурацию из шаблона и модели визарда.
func BuildTemplateConfig(model *wizardmodels.WizardModel, forPreview bool) (string, error) {
	timing := debuglog.StartTiming("BuildTemplateConfig")
	defer timing.EndWithDefer()

	if model.TemplateData == nil {
		return "", fmt.Errorf("template data not available")
	}

	SyncDNSModelToSettingsVars(model)
	MaterializeClashSecretIfNeeded(model)

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

// effectiveTemplateConfig returns the merged top-level config map and key order (same rules as buildConfigSections).
func effectiveTemplateConfig(model *wizardmodels.WizardModel) (map[string]json.RawMessage, []string) {
	if model == nil || model.TemplateData == nil {
		return nil, nil
	}
	td := model.TemplateData
	config, order := td.Config, td.ConfigOrder
	if len(td.RawConfig) > 0 && (len(td.Params) > 0 || len(td.Vars) > 0) {
		effective, ord, err := wizardtemplate.GetEffectiveConfig(
			td.RawConfig,
			td.Params,
			runtime.GOOS,
			td.Vars,
			model.SettingsVars,
			td.RawTemplate,
		)
		if err == nil {
			return effective, ord
		}
		debuglog.WarnLog("effectiveTemplateConfig: GetEffectiveConfig: %v", err)
	}
	return config, order
}

// EffectiveConfigSection returns merged template JSON for one top-level config key (e.g. "experimental").
func EffectiveConfigSection(model *wizardmodels.WizardModel, sectionKey string) (json.RawMessage, bool, error) {
	if model == nil || model.TemplateData == nil {
		return nil, false, fmt.Errorf("no template data")
	}
	MaterializeClashSecretIfNeeded(model)
	config, _ := effectiveTemplateConfig(model)
	raw, ok := config[sectionKey]
	return raw, ok, nil
}

// buildConfigSections строит форматированные JSON-секции конфига.
func buildConfigSections(model *wizardmodels.WizardModel, forPreview bool, timing *debuglog.TimingContext) ([]string, error) {
	start := time.Now()
	var sections []string

	config, order := effectiveTemplateConfig(model)

	for _, key := range order {
		raw, ok := config[key]
		if !ok {
			continue
		}

		var formatted string
		var err error

		switch key {
		case "outbounds":
			formatted, err = buildOutboundsSection(model, raw, forPreview, timing)
		case "endpoints":
			formatted, err = buildEndpointsSection(model, raw, forPreview, timing)
		case "dns":
			formatted, err = buildDNSSection(model, raw, timing)
		case "route":
			formatted, err = buildRouteSection(model, raw, timing)
		default:
			formatted, err = FormatSectionJSON(raw, 2)
			if err != nil {
				debuglog.WarnLog("buildConfigSections: FormatSectionJSON failed for key %q, using raw: %v", key, err)
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

// buildOutboundsSection строит секцию outbounds: маркеры @ParserSTART/@ParserEND и статические из шаблона.
// При сохранении (forPreview == false) между маркерами ничего не пишется — заполнение
// динамической секции выполняет updater.WriteToConfig (Parser update) после сохранения.
// При preview динамические outbounds записываются для отображения пользователю.
func buildOutboundsSection(model *wizardmodels.WizardModel, templateOutbounds json.RawMessage, forPreview bool, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build outbounds", time.Since(start)) }()

	var staticOutbounds []json.RawMessage
	_ = json.Unmarshal(templateOutbounds, &staticOutbounds)

	indent := Indent(2)
	var builder strings.Builder
	builder.WriteString("[\n")

	hasDynamic := false
	builder.WriteString(indent + "/** @ParserSTART */\n")
	if forPreview {
		if model.OutboundStats.NodesCount > wizardutils.MaxNodesForFullPreview {
			builder.WriteString(fmt.Sprintf("%s// Generated: %d nodes, %d local selectors, %d global selectors\n",
				indent, model.OutboundStats.NodesCount, model.OutboundStats.LocalSelectorsCount, model.OutboundStats.GlobalSelectorsCount))
			builder.WriteString(fmt.Sprintf("%s// Total outbounds: %d\n", indent, len(model.GeneratedOutbounds)))
		} else {
			for idx, entry := range model.GeneratedOutbounds {
				cleaned := strings.TrimRight(entry, ",\n\r\t ")
				builder.WriteString(IndentMultiline(cleaned, indent))
				if idx < len(model.GeneratedOutbounds)-1 || len(staticOutbounds) > 0 {
					builder.WriteString(",")
				}
				builder.WriteString("\n")
				hasDynamic = true
			}
		}
	}
	builder.WriteString(indent + "/** @ParserEND */")

	if len(staticOutbounds) > 0 {
		for i, item := range staticOutbounds {
			if i > 0 || hasDynamic {
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

// buildEndpointsSection строит секцию endpoints (WireGuard): маркеры @ParserSTART_E/@ParserEND_E и статические из шаблона.
// При сохранении (forPreview == false) между маркерами ничего не пишется — заполнение
// динамической секции выполняет updater.WriteToConfig (Parser update) после сохранения.
// При preview динамические endpoints записываются для отображения пользователю.
func buildEndpointsSection(model *wizardmodels.WizardModel, templateEndpoints json.RawMessage, forPreview bool, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build endpoints", time.Since(start)) }()

	var staticEndpoints []json.RawMessage
	_ = json.Unmarshal(templateEndpoints, &staticEndpoints)

	indent := Indent(2)
	var builder strings.Builder
	builder.WriteString("[\n")

	hasDynamic := false
	builder.WriteString(indent + "/** @ParserSTART_E */\n")
	if forPreview {
		if model.OutboundStats.EndpointsCount > wizardutils.MaxNodesForFullPreview {
			builder.WriteString(fmt.Sprintf("%s// Generated: %d endpoints (WireGuard)\n", indent, model.OutboundStats.EndpointsCount))
		} else {
			for idx, entry := range model.GeneratedEndpoints {
				cleaned := strings.TrimRight(entry, ",\n\r\t ")
				builder.WriteString(IndentMultiline(cleaned, indent))
				if idx < len(model.GeneratedEndpoints)-1 || len(staticEndpoints) > 0 {
					builder.WriteString(",")
				}
				builder.WriteString("\n")
				hasDynamic = true
			}
		}
	}
	builder.WriteString(indent + "/** @ParserEND_E */")

	if len(staticEndpoints) > 0 {
		for i, item := range staticEndpoints {
			if i > 0 || hasDynamic {
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

// buildDNSSection merges wizard DNS model into the template dns section.
func buildDNSSection(model *wizardmodels.WizardModel, templateDNS json.RawMessage, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build dns", time.Since(start)) }()

	merged, err := MergeDNSSection(templateDNS, model)
	if err != nil {
		return "", fmt.Errorf("dns merge failed: %w", err)
	}
	formatted, err := FormatSectionJSON(merged, 2)
	if err != nil {
		return string(merged), nil
	}
	return formatted, nil
}

// buildRouteSection строит секцию route с объединением правил и rule_set.
func buildRouteSection(model *wizardmodels.WizardModel, raw json.RawMessage, timing *debuglog.TimingContext) (string, error) {
	start := time.Now()
	defer func() { timing.LogTiming("build route", time.Since(start)) }()

	merged, err := MergeRouteSection(raw, model.CustomRules, model.SelectedFinalOutbound, model.ExecDir,
		model.DefaultDomainResolver, model.DefaultDomainResolverUnset)
	if err != nil {
		return "", fmt.Errorf("route merge failed: %w", err)
	}

	formatted, err := FormatSectionJSON(merged, 2)
	if err != nil {
		return string(merged), nil
	}
	return formatted, nil
}

// MergeRouteSection объединяет custom rules (единый список) и rule_set в секцию route.
// execDir — директория исполняемого файла; для SRS rule-set при наличии локального файла
// подставляется type: local, path (для шаблонных правил и пользовательских SRS).
func MergeRouteSection(raw json.RawMessage, customRules []*wizardmodels.RuleState, finalOutbound string, execDir string, defaultDomainResolver string, omitDefaultDomainResolver bool) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}

	// Базовые rules из шаблона (например hijack-dns); далее append только из custom_rules.
	var rules []interface{}
	if existing, ok := route["rules"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			rules = arr
		}
	}

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

	processRule := func(ruleState *wizardmodels.RuleState) {
		if !ruleState.Enabled {
			return
		}
		outbound := wizardmodels.GetEffectiveOutbound(ruleState)

		// Добавляем rule_set от этого правила
		for _, rs := range ruleState.Rule.RuleSets {
			rsObj := convertRuleSetToLocalIfNeeded(rs, execDir)
			if rsObj != nil {
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

	if omitDefaultDomainResolver {
		delete(route, "default_domain_resolver")
	} else if strings.TrimSpace(defaultDomainResolver) != "" {
		route["default_domain_resolver"] = strings.TrimSpace(defaultDomainResolver)
	}

	return json.Marshal(route)
}

// convertRuleSetToLocalIfNeeded для remote SRS rule-set подставляет type: local + path,
// если локальный файл bin/rule-sets/{tag}.srs существует. Если файла нет (удалён вручную,
// ещё не скачан и т.п.), rule-set остаётся remote — sing-box загрузит его по URL при старте.
func convertRuleSetToLocalIfNeeded(rs json.RawMessage, execDir string) interface{} {
	var m map[string]interface{}
	if err := json.Unmarshal(rs, &m); err != nil {
		return nil
	}
	typ, _ := m["type"].(string)
	tag, _ := m["tag"].(string)
	if typ != "remote" || tag == "" || execDir == "" {
		return m
	}

	path := services.RuleSRSPath(execDir, tag)

	// Для всех SRS (как встроенных, так и пользовательских) генерируем local-вариант
	// только при наличии локального файла. Если файл был удалён вручную из bin/rule-sets/,
	// конфиг вернётся к remote-варианту и приложение не упадёт при запуске sing-box.
	if services.SRSFileExists(execDir, tag) {
		return map[string]interface{}{
			"tag":    tag,
			"type":   "local",
			"format": "binary",
			"path":   path,
		}
	}

	return m
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
