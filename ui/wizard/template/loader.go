// Package template содержит функциональность загрузки и парсинга шаблонов конфигурации.
//
// Файл loader.go загружает единый шаблон конфигурации (wizard_template.json) и преобразует
// его в TemplateData для использования визардом.
//
// Шаблон содержит секции:
//   - parser_config — конфигурация парсера подписок (JSON-объект)
//   - config — основной конфиг sing-box (после применения params)
//   - dns_options (опционально) — дефолты вкладки DNS во визарде (не sing-box)
//   - selectable_rules — правила маршрутизации для визарда (с platforms и rule_set)
//   - params — платформозависимые параметры (применяются по runtime.GOOS)
//
// LoadTemplateData выполняет:
//  1. Чтение и валидацию JSON файла шаблона
//  2. Применение params для текущей платформы (replace/prepend/append)
//  3. Фильтрацию selectable_rules по platforms
//  4. Извлечение defaultFinal из config.route.final; default_domain_resolver из dns_options (default_domain_resolver или route.default_domain_resolver) или config.route
//  5. Парсинг config в упорядоченные секции для генератора
//
// Используется в:
//   - business/template_loader.go — DefaultTemplateLoader использует LoadTemplateData
//   - business/create_config.go — TemplateData используется при генерации финальной конфигурации
package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// TemplateFileName — единственный файл шаблона для всех платформ.
const TemplateFileName = "wizard_template.json"

// TemplateData — данные шаблона, подготовленные для визарда.
type TemplateData struct {
	// ParserConfig — JSON-текст блока parser_config (обернут в ParserConfig) для отображения и редактирования в визарде.
	//
	// Хранится как строка по следующим причинам:
	//   - Используется в UI (текстовое поле) — нужна строка для отображения и редактирования
	//   - Используется в бизнес-логике — парсится в структуру config.ParserConfig при необходимости
	//   - Хранение строки эффективнее, чем сериализация структуры каждый раз для UI
	//   - Строка уже содержит валидный JSON с оберткой {"ParserConfig": {...}}, готовый для парсинга
	ParserConfig string

	// Config — секции основного конфига (после применения params), сохраняя порядок из шаблона.
	Config map[string]json.RawMessage

	// ConfigOrder — порядок секций конфига (log, dns, inbounds, ...).
	ConfigOrder []string

	// RawConfig и Params — исходный config и params из шаблона; при сборке применяются заново с учётом vars / Settings.
	RawConfig json.RawMessage
	Params    []TemplateParam
	// Vars — объявления переменных шаблона; RawTemplate — полный JSON файла для default_node и ResolveTemplateVars.
	Vars        []TemplateVar
	RawTemplate json.RawMessage `json:"-"`

	// SelectableRules — правила маршрутизации для визарда (уже отфильтрованные по платформе).
	SelectableRules []TemplateSelectableRule

	// DefaultFinal — outbound по умолчанию из config.route.final.
	DefaultFinal string

	// DefaultDomainResolver — тег для route.default_domain_resolver: сначала из dns_options, иначе из config.route (после apply params).
	DefaultDomainResolver string

	// DNSOptionsRaw — секция dns_options из шаблона (не sing-box); визард читает отсюда список DNS-серверов и правила при наличии.
	DNSOptionsRaw json.RawMessage `json:"-"`
}

// TemplateSelectableRule — правило маршрутизации, управляемое пользователем в визарде.
type TemplateSelectableRule struct {
	// Label — название правила для отображения в UI.
	Label string

	// Description — описание правила (tooltip в визарде).
	Description string

	// IsDefault — включено по умолчанию при первом запуске визарда.
	IsDefault bool

	// Platforms — платформы, на которых правило доступно. Пустой = все.
	Platforms []string

	// RuleSets — определения rule_set, необходимые для работы правила.
	// Добавляются в config.route.rule_set только если правило включено.
	RuleSets []json.RawMessage

	// Rule — одиночное правило маршрутизации sing-box (map для модификации outbound).
	Rule map[string]interface{}

	// Rules — несколько правил маршрутизации (взаимоисключающее с Rule).
	Rules []map[string]interface{}

	// DefaultOutbound — outbound по умолчанию (извлекается из rule.outbound или action).
	DefaultOutbound string

	// HasOutbound — true если правило имеет outbound/action, который можно выбрать.
	HasOutbound bool

	// Params — состояние UI по типу правила (только для custom rules). Не сериализуется из шаблона.
	// Используется для сохранения/восстановления match_by_path, path_mode (processes), domain_regex (urls) и т.д.
	Params map[string]interface{}
}

// TemplateParam — платформозависимый параметр из секции params шаблона (name, platforms, value, mode).
type TemplateParam struct {
	Name      string          `json:"name"`
	Platforms []string        `json:"platforms"`
	Value     json.RawMessage `json:"value"`
	Mode      string          `json:"mode"` // "replace", "prepend", "append"
	If        []string        `json:"if,omitempty"`
	// IfOr: хотя бы одна перечисленная bool-переменная истинна на текущей ОС (см. ParamBoolVarTrue).
	IfOr []string `json:"if_or,omitempty"`
}

// jsonSelectableRule — промежуточная структура для десериализации selectable_rules из JSON.
type jsonSelectableRule struct {
	Label       string                   `json:"label"`
	Description string                   `json:"description"`
	Default     bool                     `json:"default"`
	Platforms   []string                 `json:"platforms,omitempty"`
	RuleSet     []json.RawMessage        `json:"rule_set,omitempty"`
	Rule        map[string]interface{}   `json:"rule,omitempty"`
	Rules       []map[string]interface{} `json:"rules,omitempty"`
}

// GetTemplateFileName возвращает имя файла шаблона. Один файл для всех платформ.
func GetTemplateFileName() string {
	return TemplateFileName
}

// GetTemplateURL возвращает URL для загрузки шаблона с GitHub (ветка main или develop в зависимости от версии приложения).
func GetTemplateURL() string {
	return fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/%s/bin/%s", constants.GetMyBranch(), TemplateFileName)
}

// LoadTemplateData загружает и обрабатывает шаблон конфигурации.
// Применяет params для текущей платформы, фильтрует selectable_rules.
func LoadTemplateData(execDir string) (*TemplateData, error) {
	templatePath := filepath.Join(execDir, "bin", TemplateFileName)
	debuglog.InfoLog("TemplateLoader: загрузка шаблона из: %s", templatePath)

	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", TemplateFileName, err)
	}

	// Удаление UTF-8 BOM если присутствует
	raw = stripUTF8BOM(raw)

	// Десериализация корневой структуры шаблона
	var root struct {
		ParserConfig    json.RawMessage      `json:"parser_config"`
		Config          json.RawMessage      `json:"config"`
		DNSOptions      json.RawMessage      `json:"dns_options,omitempty"`
		SelectableRules []jsonSelectableRule `json:"selectable_rules"`
		Params          []TemplateParam      `json:"params"`
		Vars            []TemplateVar        `json:"vars"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", TemplateFileName, err)
	}

	if err := ValidateWizardTemplate(root.Vars, root.Params, root.Config); err != nil {
		return nil, fmt.Errorf("%s: %w", TemplateFileName, err)
	}

	// 1. ParserConfig → оборачиваем содержимое parser_config в объект ParserConfig и форматируем
	parserConfigStr := ""
	if len(root.ParserConfig) > 0 {
		// Оборачиваем содержимое parser_config в объект ParserConfig для совместимости с config.ParserConfig
		// Форматируем для удобного отображения в UI
		var buf bytes.Buffer
		buf.WriteString("{\n  \"ParserConfig\": ")
		if err := json.Indent(&buf, root.ParserConfig, "  ", "  "); err == nil {
			// Успешно отформатировали содержимое
			buf.WriteString("\n}")
			parserConfigStr = buf.String()
		} else {
			// Fallback: просто оборачиваем без форматирования
			parserConfigStr = fmt.Sprintf("{\n  \"ParserConfig\": %s\n}", string(root.ParserConfig))
		}
	}
	debuglog.DebugLog("TemplateLoader: ParserConfig длина: %d", len(parserConfigStr))

	// 2. Сохраняем сырой config и params; применяем params + vars (дефолты шаблона, state пуст)
	rawConfig := root.Config
	rawFull := json.RawMessage(append([]byte(nil), raw...))
	configJSON, err := ApplyTemplateWithVars(root.Config, root.Params, runtime.GOOS, root.Vars, nil, rawFull)
	if err != nil {
		return nil, fmt.Errorf("error applying params: %w", err)
	}

	// 3. Парсинг config в упорядоченные секции
	configSections, configOrder, err := parseJSONWithOrder(configJSON)
	if err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	debuglog.DebugLog("TemplateLoader: секции конфига: %v", configOrder)

	// 4. Извлечение defaultFinal; default_domain_resolver — dns_options, затем route
	defaultFinal := extractDefaultFinal(configSections)
	defaultDomainResolver := extractDefaultDomainResolverFromDNSOptions(root.DNSOptions)
	if defaultDomainResolver == "" {
		defaultDomainResolver = extractDefaultDomainResolver(configSections)
	}
	debuglog.DebugLog("TemplateLoader: defaultFinal: %s", defaultFinal)
	debuglog.DebugLog("TemplateLoader: defaultDomainResolver: %s", defaultDomainResolver)

	// 5. Фильтрация selectable_rules по платформе и преобразование
	platform := runtime.GOOS
	selectableRules := filterAndConvertRules(root.SelectableRules, platform)
	debuglog.InfoLog("TemplateLoader: загружено %d selectable rules для платформы %s", len(selectableRules), platform)

	return &TemplateData{
		ParserConfig:            parserConfigStr,
		Config:                  configSections,
		ConfigOrder:             configOrder,
		RawConfig:               rawConfig,
		Params:                  root.Params,
		Vars:                    root.Vars,
		RawTemplate:             rawFull,
		SelectableRules:         selectableRules,
		DefaultFinal:            defaultFinal,
		DefaultDomainResolver:   defaultDomainResolver,
		DNSOptionsRaw:           root.DNSOptions,
	}, nil
}

// ApplyTemplateWithVars применяет params (платформа, if, if_or) и подставляет @name из vars.
func ApplyTemplateWithVars(configJSON json.RawMessage, params []TemplateParam, goos string, vars []TemplateVar, stateVars map[string]string, rawFull json.RawMessage) (json.RawMessage, error) {
	resolved := ResolveTemplateVars(vars, stateVars, rawFull)
	MaybeGenerateClashSecret(resolved)
	vi := VarIndex(vars)
	out, err := applyParamsFiltered(configJSON, params, goos, vi, resolved)
	if err != nil {
		return nil, err
	}
	if len(vars) == 0 {
		return out, nil
	}
	return SubstituteVarsInJSON(out, vars, resolved)
}

func applyParamsFiltered(configJSON json.RawMessage, params []TemplateParam, goos string, vi map[string]TemplateVar, resolved map[string]ResolvedVar) (json.RawMessage, error) {
	if len(params) == 0 {
		return configJSON, nil
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	for _, param := range params {
		if !matchesPlatform(param.Platforms, goos) {
			continue
		}
		if vi != nil && resolved != nil {
			if len(param.If) > 0 && len(param.IfOr) > 0 {
				return nil, fmt.Errorf("param %q: if and if_or cannot both be set", param.Name)
			}
			if len(param.If) > 0 && !ParamIfSatisfied(param.If, vi, resolved, goos) {
				continue
			}
			if len(param.IfOr) > 0 && !ParamIfOrSatisfied(param.IfOr, vi, resolved, goos) {
				continue
			}
		}
		mode := param.Mode
		if mode == "" {
			mode = "replace"
		}
		debuglog.DebugLog("TemplateLoader: применение param '%s' (mode=%s) для платформы %s", param.Name, mode, goos)

		if err := applyParam(config, param.Name, param.Value, mode); err != nil {
			return nil, fmt.Errorf("error applying param '%s': %w", param.Name, err)
		}
	}

	return json.Marshal(config)
}

// applyParam применяет один параметр к config.
// Поддерживает dot notation для вложенных путей (например, "route.rules").
func applyParam(config map[string]json.RawMessage, name string, value json.RawMessage, mode string) error {
	parts := strings.SplitN(name, ".", 2)
	key := parts[0]

	if len(parts) == 1 {
		// Простой ключ верхнего уровня
		return applyValue(config, key, value, mode)
	}

	// Вложенный путь — рекурсия
	subKey := parts[1]
	existing, ok := config[key]
	if !ok {
		existing = []byte("{}")
	}

	var subConfig map[string]json.RawMessage
	if err := json.Unmarshal(existing, &subConfig); err != nil {
		return fmt.Errorf("section '%s' is not an object: %w", key, err)
	}

	if err := applyParam(subConfig, subKey, value, mode); err != nil {
		return err
	}

	updated, err := json.Marshal(subConfig)
	if err != nil {
		return err
	}
	config[key] = updated
	return nil
}

// applyValue применяет значение к конкретному ключу с указанным режимом.
func applyValue(config map[string]json.RawMessage, key string, value json.RawMessage, mode string) error {
	switch mode {
	case "replace":
		config[key] = value
		return nil

	case "prepend":
		existing, ok := config[key]
		if !ok {
			config[key] = value
			return nil
		}
		return mergeArrays(config, key, value, existing)

	case "append":
		existing, ok := config[key]
		if !ok {
			config[key] = value
			return nil
		}
		return mergeArrays(config, key, existing, value)

	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

// mergeArrays объединяет два JSON-массива: first + second.
func mergeArrays(config map[string]json.RawMessage, key string, first, second json.RawMessage) error {
	var arr1, arr2 []json.RawMessage
	if err := json.Unmarshal(first, &arr1); err != nil {
		return fmt.Errorf("'%s' first array invalid: %w", key, err)
	}
	if err := json.Unmarshal(second, &arr2); err != nil {
		return fmt.Errorf("'%s' second array invalid: %w", key, err)
	}
	merged := append(arr1, arr2...)
	result, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	config[key] = result
	return nil
}

// GetEffectiveConfig применяет params и vars к rawConfig и возвращает секции и порядок ключей.
func GetEffectiveConfig(rawConfig json.RawMessage, params []TemplateParam, goos string, vars []TemplateVar, stateVars map[string]string, rawFull json.RawMessage) (map[string]json.RawMessage, []string, error) {
	if len(rawConfig) == 0 {
		return nil, nil, fmt.Errorf("raw config is empty")
	}
	applied, err := ApplyTemplateWithVars(rawConfig, params, goos, vars, stateVars, rawFull)
	if err != nil {
		return nil, nil, err
	}
	return parseJSONWithOrder(applied)
}

// matchesPlatform проверяет, подходит ли текущая платформа (GOOS; Win7-сборка — это windows/386).
func matchesPlatform(platforms []string, goos string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == goos {
			return true
		}
	}
	return false
}

// filterAndConvertRules фильтрует правила по платформе и конвертирует в TemplateSelectableRule.
func filterAndConvertRules(jsonRules []jsonSelectableRule, platform string) []TemplateSelectableRule {
	var result []TemplateSelectableRule
	for _, jr := range jsonRules {
		if !matchesPlatform(jr.Platforms, platform) {
			continue
		}
		rule := TemplateSelectableRule{
			Label:       jr.Label,
			Description: jr.Description,
			IsDefault:   jr.Default,
			Platforms:   jr.Platforms,
			RuleSets:    jr.RuleSet,
			Rule:        jr.Rule,
			Rules:       jr.Rules,
		}
		// Вычисление DefaultOutbound и HasOutbound
		computeOutboundInfo(&rule)

		if rule.Label == "" {
			rule.Label = fmt.Sprintf("Rule %d", len(result)+1)
		}
		result = append(result, rule)
	}
	return result
}

// computeOutboundInfo вычисляет DefaultOutbound и HasOutbound на основе содержимого правила.
func computeOutboundInfo(rule *TemplateSelectableRule) {
	// Определяем primary rule для анализа
	ruleData := rule.Rule
	if ruleData == nil && len(rule.Rules) > 0 {
		ruleData = rule.Rules[0]
	}
	if ruleData == nil {
		return
	}

	// Проверка action: reject
	if actionVal, ok := ruleData["action"]; ok {
		if actionStr, ok := actionVal.(string); ok && actionStr == "reject" {
			rule.HasOutbound = true
			if methodVal, ok := ruleData["method"]; ok {
				if methodStr, ok := methodVal.(string); ok && methodStr == "drop" {
					rule.DefaultOutbound = "drop"
				} else {
					rule.DefaultOutbound = "reject"
				}
			} else {
				rule.DefaultOutbound = "reject"
			}
			return
		}
	}

	// Проверка outbound
	if outboundVal, ok := ruleData["outbound"]; ok {
		rule.HasOutbound = true
		if outboundStr, ok := outboundVal.(string); ok {
			rule.DefaultOutbound = outboundStr
		}
	}
}

// parseJSONWithOrder парсит JSON-объект с сохранением порядка ключей.
func parseJSONWithOrder(jsonBytes []byte) (map[string]json.RawMessage, []string, error) {
	sections := make(map[string]json.RawMessage)
	var order []string

	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))

	token, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, nil, fmt.Errorf("expected '{', got %v", token)
	}

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected string key, got %v", keyToken)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("error decoding value for '%s': %w", key, err)
		}

		sections[key] = value
		order = append(order, key)
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '}' {
		return nil, nil, fmt.Errorf("expected '}', got %v", token)
	}

	return sections, order, nil
}

// extractDefaultFinal извлекает route.final из секций конфига.
func extractDefaultFinal(sections map[string]json.RawMessage) string {
	raw, ok := sections["route"]
	if !ok || len(raw) == 0 {
		return ""
	}
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return ""
	}
	if finalVal, ok := route["final"]; ok {
		if finalStr, ok := finalVal.(string); ok {
			return finalStr
		}
	}
	return ""
}

// extractDefaultDomainResolverFromDNSOptions reads dns_options (wizard-only): default_domain_resolver или route.default_domain_resolver.
func extractDefaultDomainResolverFromDNSOptions(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		debuglog.DebugLog("TemplateLoader: dns_options parse: %v", err)
		return ""
	}
	for _, key := range []string{"default_domain_resolver", "route.default_domain_resolver"} {
		if v, ok := m[key]; ok && len(v) > 0 {
			var s string
			if json.Unmarshal(v, &s) != nil {
				continue
			}
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
		}
	}
	return ""
}

// extractDefaultDomainResolver reads route.default_domain_resolver from config sections (after params).
func extractDefaultDomainResolver(sections map[string]json.RawMessage) string {
	raw, ok := sections["route"]
	if !ok || len(raw) == 0 {
		return ""
	}
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return ""
	}
	v, ok := route["default_domain_resolver"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// stripUTF8BOM удаляет UTF-8 BOM (EF BB BF) в начале файла, если присутствует.
func stripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
