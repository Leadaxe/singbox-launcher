// Package template содержит функциональность загрузки и парсинга шаблонов конфигурации.
//
// Файл loader.go содержит функции для загрузки шаблона конфигурации из файла и извлечения его частей:
//   - LoadTemplateData - загружает шаблон из файла (config_template.json или config_template_macos.json)
//   - GetTemplateFileName - возвращает имя файла шаблона для текущей платформы
//   - GetTemplateURL - возвращает URL для загрузки шаблона с GitHub
//   - TemplateData - структура данных шаблона (ParserConfig, секции, правила, defaultFinal)
//   - TemplateSelectableRule - структура правила, которое может быть выбрано в визарде
//
// LoadTemplateData выполняет парсинг шаблона с извлечением специальных блоков:
//  1. Извлекает @ParserConfig блок из комментариев (для использования в визарде)
//  2. Извлекает @SelectableRule блоки (правила маршрутизации, которые можно настраивать)
//  3. Парсит JSON конфигурации с сохранением порядка секций
//  4. Определяет defaultFinal outbound из секции route
//  5. Проверяет наличие маркера @PARSER_OUTBOUNDS_BLOCK для вставки сгенерированных outbounds
//
// Шаблоны используются в визарде как основа для генерации финальной конфигурации:
// пользователь настраивает ParserConfig, выбирает секции и правила, и визард объединяет
// это с шаблоном для создания финальной конфигурации sing-box.
//
// Загрузка шаблонов - это отдельная ответственность от бизнес-логики визарда.
// Шаблоны содержат статическую структуру конфигурации и правила маршрутизации по умолчанию.
//
// Используется в:
//   - wizard.go - LoadTemplateData вызывается при инициализации визарда для загрузки шаблона
//   - business/template_loader.go - DefaultTemplateLoader использует LoadTemplateData
//   - business/generator.go - TemplateData используется при генерации финальной конфигурации
package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/internal/debuglog"
)

const templateLoaderLogLevel = debuglog.LevelOff

func tplLog(level debuglog.Level, format string, args ...interface{}) {
	debuglog.Log("TemplateLoader", level, templateLoaderLogLevel, format, args...)
}

// TemplateData represents the loaded template data.
type TemplateData struct {
	ParserConfig            string
	Sections                map[string]json.RawMessage
	SectionOrder            []string
	SelectableRules         []TemplateSelectableRule
	DefaultFinal            string
	HasParserOutboundsBlock bool   // true if @PARSER_OUTBOUNDS_BLOCK marker was found in template
	OutboundsAfterMarker    string // Elements after @PARSER_OUTBOUNDS_BLOCK marker (e.g., direct-out)
}

// TemplateSelectableRule represents a rule that can be selected in the template.
type TemplateSelectableRule struct {
	Label           string
	Description     string
	Raw             map[string]interface{}
	DefaultOutbound string
	HasOutbound     bool // true if rule has "outbound" field that can be selected
	IsDefault       bool // true if rule should be enabled by default
}

// GetTemplateFileName returns the template file name for the current platform.
// On macOS returns "config_template_macos.json", on other platforms returns "config_template.json".
func GetTemplateFileName() string {
	if runtime.GOOS == "darwin" {
		return "config_template_macos.json"
	}
	return "config_template.json"
}

// GetTemplateURL returns the GitHub URL for downloading the template for the current platform.
func GetTemplateURL() string {
	fileName := GetTemplateFileName()
	return fmt.Sprintf("https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/bin/%s", fileName)
}

// LoadTemplateData loads template data from file.
func LoadTemplateData(execDir string) (*TemplateData, error) {
	templateFileName := GetTemplateFileName()
	templatePath := filepath.Join(execDir, "bin", templateFileName)
	tplLog(debuglog.LevelInfo, "Starting to load template from: %s", templatePath)
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		tplLog(debuglog.LevelError, "Failed to read template file: %v", err)
		return nil, err
	}
	tplLog(debuglog.LevelVerbose, "Successfully read template file, size: %d bytes", len(raw))

	rawStr := string(raw)
	parserConfig, cleaned := extractCommentBlock(rawStr, "ParserConfig")
	tplLog(debuglog.LevelVerbose, "After extractCommentBlock, parserConfig length: %d, cleaned length: %d", len(parserConfig), len(cleaned))

	selectableBlocks, cleaned := extractAllSelectableBlocks(cleaned)
	tplLog(debuglog.LevelVerbose, "After extractAllSelectableBlocks, found %d blocks, cleaned length: %d", len(selectableBlocks), len(cleaned))
	if len(selectableBlocks) > 0 {
		for i, block := range selectableBlocks {
			tplLog(debuglog.LevelTrace, "Block %d (first 100 chars): %s", i+1, truncateString(block, 100))
		}
	}

	// Check for @PARSER_OUTBOUNDS_BLOCK marker before parsing JSON
	// (JSON parser will ignore comments, so we need to check the raw string)
	hasParserBlock := strings.Contains(cleaned, "@PARSER_OUTBOUNDS_BLOCK")
	tplLog(debuglog.LevelVerbose, "Has @PARSER_OUTBOUNDS_BLOCK marker: %v", hasParserBlock)

	// Extract elements after the marker (e.g., direct-out)
	var outboundsAfterMarker string
	if hasParserBlock {
		outboundsAfterMarker = extractOutboundsAfterMarker(cleaned)
		if outboundsAfterMarker != "" {
			tplLog(debuglog.LevelVerbose, "Extracted outbounds after marker (first 200 chars): %s", truncateString(outboundsAfterMarker, 200))
		}
	}

	// Validate JSON before parsing
	jsonBytes := jsonc.ToJSON([]byte(cleaned))
	tplLog(debuglog.LevelVerbose, "After jsonc.ToJSON, jsonBytes length: %d", len(jsonBytes))

	if !json.Valid(jsonBytes) {
		tplLog(debuglog.LevelWarn, "JSON validation failed. First 500 chars: %s", truncateString(string(jsonBytes), 500))
		return nil, fmt.Errorf("invalid JSON after removing @SelectableRule blocks. This may indicate a syntax error in %s", templateFileName)
	}

	tplLog(debuglog.LevelVerbose, "JSON is valid, proceeding to unmarshal")

	// Parse JSON while preserving key order from template
	sections, sectionOrder, err := parseJSONWithOrder(jsonBytes)
	if err != nil {
		tplLog(debuglog.LevelError, "JSON unmarshal failed: %v", err)
		return nil, fmt.Errorf("failed to parse %s: %w", templateFileName, err)
	}

	tplLog(debuglog.LevelVerbose, "Successfully unmarshaled %d sections", len(sections))
	tplLog(debuglog.LevelTrace, "Section order from template: %v", sectionOrder)

	defaultFinal := extractDefaultFinal(sections)
	if defaultFinal != "" {
		tplLog(debuglog.LevelVerbose, "Detected default final outbound: %s", defaultFinal)
	}

	selectableRules, err := parseSelectableRules(selectableBlocks)
	if err != nil {
		tplLog(debuglog.LevelError, "parseSelectableRules failed: %v", err)
		return nil, err
	}

	tplLog(debuglog.LevelVerbose, "Successfully parsed %d selectable rules", len(selectableRules))

	result := &TemplateData{
		ParserConfig:            strings.TrimSpace(parserConfig),
		Sections:                sections,
		SectionOrder:            sectionOrder,
		SelectableRules:         selectableRules,
		DefaultFinal:            defaultFinal,
		HasParserOutboundsBlock: hasParserBlock,
		OutboundsAfterMarker:    outboundsAfterMarker,
	}

	tplLog(debuglog.LevelInfo, "Successfully loaded template data with %d sections and %d selectable rules", len(sections), len(selectableRules))

	return result, nil
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractCommentBlock(src, marker string) (string, string) {
	pattern := regexp.MustCompile(`(?s)/\*\*\s*@` + marker + `\s*(.*?)\*/`)
	matches := pattern.FindStringSubmatch(src)
	if len(matches) < 2 {
		return "", src
	}
	cleaned := pattern.ReplaceAllString(src, "")
	return strings.TrimSpace(matches[1]), cleaned
}

func extractAllSelectableBlocks(src string) ([]string, string) {
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: input length: %d", len(src))
	// Only support @SelectableRule
	// Match the block including optional leading/trailing commas, whitespace, and empty lines
	pattern := regexp.MustCompile(`(?is)(\s*,?\s*)/\*\*\s*@selectablerule\s*(.*?)\*/(\s*,?\s*)`)
	matches := pattern.FindAllStringSubmatch(src, -1)
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: found %d matches", len(matches))
	if len(matches) == 0 {
		tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: no matches, returning original source")
		return nil, src
	}

	// Extract blocks first before removing
	var blocks []string
	for _, m := range matches {
		if len(m) >= 3 {
			blocks = append(blocks, strings.TrimSpace(m[2]))
		}
	}
	tplLog(debuglog.LevelVerbose, "extractAllSelectableBlocks: extracted %d blocks", len(blocks))

	// Remove the blocks, including surrounding commas and whitespace
	// Use a more aggressive pattern that also removes empty lines after blocks
	cleaned := pattern.ReplaceAllString(src, "")
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: after removing blocks, length: %d", len(cleaned))

	// Remove empty lines that might be left (lines with only whitespace)
	cleaned = regexp.MustCompile(`(?m)^\s*$\n?`).ReplaceAllString(cleaned, "")
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: after removing empty lines, length: %d", len(cleaned))

	// Clean up any double commas that might result
	cleaned = regexp.MustCompile(`,\s*,`).ReplaceAllString(cleaned, ",")
	// Clean up comma before closing bracket
	cleaned = regexp.MustCompile(`,\s*\]`).ReplaceAllString(cleaned, "]")
	// Clean up comma after opening bracket
	cleaned = regexp.MustCompile(`\[\s*,`).ReplaceAllString(cleaned, "[")
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: after cleaning commas, length: %d", len(cleaned))
	tplLog(debuglog.LevelTrace, "extractAllSelectableBlocks: first 200 chars of cleaned: %s", truncateString(cleaned, 200))

	return blocks, cleaned
}

func parseSelectableRules(blocks []string) ([]TemplateSelectableRule, error) {
	tplLog(debuglog.LevelVerbose, "parseSelectableRules: incoming blocks (%d total)", len(blocks))
	for i, block := range blocks {
		tplLog(debuglog.LevelTrace, "parseSelectableRules: incoming block %d raw (first 200 chars): %s", i+1, truncateString(block, 200))
	}

	if len(blocks) == 0 {
		tplLog(debuglog.LevelVerbose, "parseSelectableRules: no blocks provided, returning empty result")
		return nil, nil
	}

	var rules []TemplateSelectableRule
	for i, rawBlock := range blocks {
		tplLog(debuglog.LevelVerbose, "parseSelectableRules: processing block %d/%d", i+1, len(blocks))
		if strings.TrimSpace(rawBlock) == "" {
			tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d is empty after trimming, skipping", i+1)
			continue
		}

		label, description, isDefault, cleanedBlock := extractRuleMetadata(rawBlock, i+1)
		tplLog(debuglog.LevelVerbose, "parseSelectableRules: block %d label='%s', description='%s', isDefault=%v", i+1, label, description, isDefault)
		tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d cleaned body (first 200 chars): %s", i+1, truncateString(cleanedBlock, 200))

		if cleanedBlock == "" {
			return nil, fmt.Errorf("selectable rule block %d has no JSON content", i+1)
		}

		jsonStr, err := normalizeRuleJSON(cleanedBlock, i+1)
		if err != nil {
			return nil, fmt.Errorf("selectable rule block %d: %w", i+1, err)
		}
		tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d normalized JSON (first 200 chars): %s", i+1, truncateString(jsonStr, 200))

		jsonBytes := jsonc.ToJSON([]byte(jsonStr))
		if !json.Valid(jsonBytes) {
			tplLog(debuglog.LevelWarn, "parseSelectableRules: block %d JSON invalid after jsonc conversion (first 200 chars): %s", i+1, truncateString(string(jsonBytes), 200))
			return nil, fmt.Errorf("selectable rule block %d contains invalid JSON", i+1)
		}

		var items []map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &items); err != nil {
			tplLog(debuglog.LevelError, "parseSelectableRules: block %d JSON unmarshal failed: %v", i+1, err)
			return nil, fmt.Errorf("failed to parse selectable rule block %d: %w", i+1, err)
		}
		tplLog(debuglog.LevelVerbose, "parseSelectableRules: block %d parsed into %d item(s)", i+1, len(items))

		for _, item := range items {
			rule := TemplateSelectableRule{
				Raw:         make(map[string]interface{}),
				Label:       label,
				Description: description,
				IsDefault:   isDefault,
			}

			for key, value := range item {
				rule.Raw[key] = value
			}

			if rule.Label == "" {
				if labelVal, ok := item["label"]; ok {
					if labelStr, ok := labelVal.(string); ok {
						rule.Label = labelStr
					}
				}
			}

			// Check if rule has action: reject
			// If it does, ignore outbound field and set HasOutbound based on action: reject
			actionVal, hasAction := item["action"]
			if hasAction {
				if actionStr, ok := actionVal.(string); ok && actionStr == "reject" {
					// Rule has action: reject - ignore outbound field, set HasOutbound to true
					// Default outbound depends on method field
					rule.HasOutbound = true
					methodVal, hasMethod := item["method"]
					if hasMethod {
						if methodStr, ok := methodVal.(string); ok && methodStr == "drop" {
							// If method: drop, default outbound is "drop"
							rule.DefaultOutbound = "drop"
						} else {
							// If action: reject with method but not drop, default outbound is "reject"
							rule.DefaultOutbound = "reject"
						}
					} else {
						// If action: reject without method, default outbound is "reject"
						rule.DefaultOutbound = "reject"
					}
				} else {
					// Action is not reject - check for outbound field
					if outboundVal, hasOutbound := item["outbound"]; hasOutbound {
						rule.HasOutbound = true
						if outboundStr, ok := outboundVal.(string); ok {
							rule.DefaultOutbound = outboundStr
						}
					}
				}
			} else if outboundVal, hasOutbound := item["outbound"]; hasOutbound {
				// Rule has outbound field and no action field
				rule.HasOutbound = true
				if outboundStr, ok := outboundVal.(string); ok {
					rule.DefaultOutbound = outboundStr
				}
			}

			if rule.Label == "" {
				rule.Label = fmt.Sprintf("Rule %d", len(rules)+1)
			}

			rules = append(rules, rule)
		}
	}

	tplLog(debuglog.LevelVerbose, "parseSelectableRules: completed with %d rule(s)", len(rules))
	return rules, nil
}

func extractRuleMetadata(block string, blockIndex int) (string, string, bool, string) {
	const (
		labelDirective   = "@label"
		descDirective    = "@description"
		defaultDirective = "@default"
	)

	var builder strings.Builder
	var label string
	var description string
	var isDefault bool

	lines := strings.Split(block, "\n")
	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, labelDirective):
			value := strings.TrimSpace(trimmed[len(labelDirective):])
			if value != "" {
				label = value
				tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d line %d label parsed: %s", blockIndex, lineIdx+1, value)
			}
			continue
		case strings.HasPrefix(trimmed, descDirective):
			value := strings.TrimSpace(trimmed[len(descDirective):])
			if value != "" {
				description = value
				tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d line %d description parsed: %s", blockIndex, lineIdx+1, value)
			}
			continue
		case strings.HasPrefix(trimmed, defaultDirective):
			isDefault = true
			tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d line %d @default directive found", blockIndex, lineIdx+1)
			continue
		default:
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	cleaned := strings.TrimSpace(builder.String())
	tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d body length after removing directives: %d", blockIndex, len(cleaned))
	return label, description, isDefault, cleaned
}

func normalizeRuleJSON(body string, blockIndex int) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", fmt.Errorf("no JSON content after trimming block %d", blockIndex)
	}

	trimmed = strings.TrimRight(trimmed, " \t\r\n,")
	trimmed = strings.TrimSpace(trimmed)
	tplLog(debuglog.LevelTrace, "parseSelectableRules: block %d body after trimming trailing commas (first 200 chars): %s", blockIndex, truncateString(trimmed, 200))

	if trimmed == "" {
		return "", fmt.Errorf("no JSON content remains in block %d after trimming", blockIndex)
	}

	if strings.HasPrefix(trimmed, "[") {
		return trimmed, nil
	}

	normalized := fmt.Sprintf("[%s]", trimmed)
	return normalized, nil
}

// parseJSONWithOrder parses JSON while preserving the order of keys
func parseJSONWithOrder(jsonBytes []byte) (map[string]json.RawMessage, []string, error) {
	sections := make(map[string]json.RawMessage)
	var order []string

	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))

	// Skip the opening '{'
	token, err := decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, nil, fmt.Errorf("expected object start, got %v", token)
	}

	// Read keys in order
	for decoder.More() {
		// Read key
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, nil, fmt.Errorf("expected string key, got %v", keyToken)
		}

		// Read value as RawMessage
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("failed to decode value for key %s: %w", key, err)
		}

		sections[key] = value
		order = append(order, key)
	}

	// Skip the closing '}'
	token, err = decoder.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '}' {
		return nil, nil, fmt.Errorf("expected object end, got %v", token)
	}

	return sections, order, nil
}

// extractOutboundsAfterMarker extracts elements that come after @PARSER_OUTBOUNDS_BLOCK marker
// in the outbounds array (e.g., direct-out)
func extractOutboundsAfterMarker(src string) string {
	// Find the outbounds section
	outboundsPattern := regexp.MustCompile(`(?is)"outbounds"\s*:\s*\[(.*?)\]`)
	match := outboundsPattern.FindStringSubmatch(src)
	if len(match) < 2 {
		tplLog(debuglog.LevelTrace, "extractOutboundsAfterMarker: outbounds section not found")
		return ""
	}

	outboundsContent := match[1]
	tplLog(debuglog.LevelTrace, "extractOutboundsAfterMarker: found outbounds content (first 200 chars): %s", truncateString(outboundsContent, 200))

	// Find the marker
	markerPattern := regexp.MustCompile(`(?is)/\*\*\s*@PARSER_OUTBOUNDS_BLOCK\s*\*/(.*)`)
	markerMatch := markerPattern.FindStringSubmatch(outboundsContent)
	if len(markerMatch) < 2 {
		tplLog(debuglog.LevelTrace, "extractOutboundsAfterMarker: marker not found in outbounds content")
		return ""
	}

	// Extract content after marker
	afterMarker := strings.TrimSpace(markerMatch[1])
	tplLog(debuglog.LevelTrace, "extractOutboundsAfterMarker: content after marker (first 200 chars): %s", truncateString(afterMarker, 200))

	// Remove leading commas and whitespace
	afterMarker = strings.TrimLeft(afterMarker, ",\n\r\t ")

	if afterMarker == "" {
		tplLog(debuglog.LevelTrace, "extractOutboundsAfterMarker: no content after marker")
		return ""
	}

	// Remove trailing comma if present
	afterMarker = strings.TrimRight(afterMarker, ",\n\r\t ")

	tplLog(debuglog.LevelVerbose, "extractOutboundsAfterMarker: extracted %d chars", len(afterMarker))
	return afterMarker
}

func extractDefaultFinal(sections map[string]json.RawMessage) string {
	raw, ok := sections["route"]
	if !ok || len(raw) == 0 {
		return ""
	}
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		tplLog(debuglog.LevelWarn, "extractDefaultFinal: failed to unmarshal route section: %v", err)
		return ""
	}
	if finalVal, ok := route["final"]; ok {
		if finalStr, ok := finalVal.(string); ok {
			return finalStr
		}
	}
	return ""
}
