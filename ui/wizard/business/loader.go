// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл loader.go содержит функции для загрузки и инициализации конфигурации визарда:
//   - LoadConfigFromFile - загружает ParserConfig из config.json (приоритет) или из template (fallback)
//   - EnsureRequiredOutbounds - обеспечивает наличие требуемых outbounds из template в загруженной конфигурации
//   - CloneOutbound - создает глубокую копию OutboundConfig для безопасного изменения
//
// LoadConfigFromFile выполняет:
//   1. Проверку размера файла config.json (не должен превышать MaxJSONConfigSize)
//   2. Извлечение @ParserConfig блока из config.json через parser.ExtractParserConfig()
//   3. Если @ParserConfig не найден в config.json, использует ParserConfig из template
//   4. Извлечение source URLs из @ParserConfig (если есть)
//   5. Возврат загруженных данных или ошибки
//
// Файл является оркестратором, который координирует загрузку конфигурации:
// использует parser.ExtractParserConfig() из core/config/parser для извлечения @ParserConfig блока,
// работает с WizardModel (чистые данные без GUI зависимостей),
// возвращает ошибки для обработки презентером (показ диалогов, обновление GUI),
// обеспечивает логику слияния конфигурации из файла и template.
//
// Загрузка конфигурации - это отдельная ответственность от парсинга URL и генерации.
// Содержит логику работы с файлами и шаблонами.
// Координирует использование parser из core/config/parser.
//
// Реальная логика парсинга @ParserConfig блоков находится в core/config/parser,
// который используется через parser.ExtractParserConfig().
//
// Используется в:
//   - wizard.go - LoadConfigFromFile вызывается при инициализации визарда для загрузки существующей конфигурации
package business

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/parser"
	"singbox-launcher/internal/debuglog"
	wizardtemplate "singbox-launcher/ui/wizard/template"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// LoadConfigFromFile loads configuration data from existing config.json.
// It prioritizes loading ParserConfig from config.json, falling back to template if not available.
// Returns (loaded bool, parserConfigJSON string, sourceURLs string, error).
// ParserConfigJSON and sourceURLs are empty strings if not loaded.
func LoadConfigFromFile(fileService FileServiceInterface, templateData *wizardtemplate.TemplateData) (bool, string, string, error) {
	configPath := fileService.ConfigPath()
	fileInfo, err := os.Stat(configPath)
	if err == nil {
		// Validate file size before loading
		if fileInfo.Size() > wizardutils.MaxJSONConfigSize {
			debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: config.json file size (%d bytes) exceeds maximum (%d bytes)", fileInfo.Size(), wizardutils.MaxJSONConfigSize)
			return false, "", "", fmt.Errorf("config.json file is too large (%d bytes, maximum %d bytes). Please check the file size and content", fileInfo.Size(), wizardutils.MaxJSONConfigSize)
		}

		// config.json exists - try to extract ParserConfig from it
		parserConfig, err := parser.ExtractParserConfig(configPath)
		if err == nil {
			// Successfully extracted ParserConfig from config.json - use it fully
			debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Using ParserConfig from config.json")

			// Check and add/update required outbounds from template
			if templateData != nil && templateData.ParserConfig != "" {
				EnsureRequiredOutbounds(parserConfig, templateData.ParserConfig)
			}

			// Serialize ParserConfig
			parserConfigJSON, err := SerializeParserConfig(parserConfig)
			if err != nil {
				debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to serialize ParserConfig: %v", err)
				return false, "", "", fmt.Errorf("failed to serialize ParserConfig: %w", err)
			}

			// Extract URLs from parserConfig
			var sourceURLs string
			if len(parserConfig.ParserConfig.Proxies) > 0 {
				lines := make([]string, 0)
				for _, proxySource := range parserConfig.ParserConfig.Proxies {
					if proxySource.Source != "" {
						lines = append(lines, proxySource.Source)
					}
					lines = append(lines, proxySource.Connections...)
				}
				sourceURLs = strings.Join(lines, "\n")
			}

			debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Successfully loaded config from file")
			return true, parserConfigJSON, sourceURLs, nil
		}
		// If failed to extract ParserConfig from config.json, return error (presenter will show dialog)
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to extract ParserConfig from config.json: %v", err)
		return false, "", "", fmt.Errorf("error in @ParserConfig block in config.json:\n\n%v\n\nCheck JSON syntax in @ParserConfig block (e.g., trailing commas, invalid quotes, unclosed brackets). Default template will be used", err)
	}

	// Fallback: If config.json doesn't exist or doesn't contain ParserConfig, use template
	if templateData != nil && templateData.ParserConfig != "" {
		debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Using ParserConfig from template (config.json not found or invalid)")
		// Parse ParserConfig from template
		var templateParserConfig config.ParserConfig
		if err := json.Unmarshal([]byte(templateData.ParserConfig), &templateParserConfig); err != nil {
			debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to parse ParserConfig from template: %v", err)
			return false, "", "", nil
		}

		parserConfigJSON, err := SerializeParserConfig(&templateParserConfig)
		if err != nil {
			debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to serialize ParserConfig: %v", err)
			return false, "", "", fmt.Errorf("failed to serialize ParserConfig: %w", err)
		}

		return true, parserConfigJSON, "", nil
	}

	// Config doesn't exist and no template - leave default values
	debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: config.json not found and no template available, using default values")
	return false, "", "", nil
}

// EnsureRequiredOutbounds checks and adds/updates required outbounds from template.
//
// LOGIC:
// 1. Parse template from templateParserConfigJSON to get list of outbounds with wizard.required > 0
// 2. Create map of existing outbounds from parserConfig (loaded from config.json) by tag for fast lookup
// 3. Go through all outbounds from template:
//   - If wizard.required == 0 or missing → skip (don't check)
//   - If wizard.required == 1:
//   - Check only tag presence in config.json
//   - If missing → add from template
//   - If present → keep existing version from config.json (don't touch)
//   - If wizard.required > 1 (e.g., 2):
//   - Always overwrite from template, regardless of presence in config.json
//   - If present → replace via pointer (*existingOutbound = *cloned)
//   - If missing → add to list (append)
//
// IMPORTANT:
// - For required > 1 don't check match - always overwrite from template
// - For required == 1 check only tag presence, don't touch content
// - Cloning is done once before exists check for optimization
func EnsureRequiredOutbounds(parserConfig *config.ParserConfig, templateParserConfigJSON string) {
	// Validate template JSON size
	if err := ValidateJSONSize([]byte(templateParserConfigJSON)); err != nil {
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Template ParserConfig JSON size validation failed: %v", err)
		return
	}

	// STEP 1: Parse template to get list of outbounds with wizard.required > 0
	var templateParserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(templateParserConfigJSON), &templateParserConfig); err != nil {
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to parse template ParserConfig for required outbounds: %v", err)
		return
	}

	// STEP 2: Create map of existing outbounds from parserConfig (loaded from config.json) by tag
	// This allows fast check of outbound presence by tag without full iteration
	existingOutbounds := make(map[string]*config.OutboundConfig)
	for i := range parserConfig.ParserConfig.Outbounds {
		tag := parserConfig.ParserConfig.Outbounds[i].Tag
		if tag != "" {
			existingOutbounds[tag] = &parserConfig.ParserConfig.Outbounds[i]
		}
	}

	// STEP 3: Go through all outbounds from template and check required ones
	for _, templateOutbound := range templateParserConfig.ParserConfig.Outbounds {
		// Extract required value from wizard.required (new format) or ignore if missing
		required := templateOutbound.GetWizardRequired()
		if required <= 0 {
			continue // Skip non-required outbounds (required == 0 or missing)
		}

		tag := templateOutbound.Tag
		if tag == "" {
			continue // Skip outbounds without tag (can't check presence)
		}

		// Check outbound presence in current ParserConfig (from config.json)
		existingOutbound, exists := existingOutbounds[tag]

		if required == 1 {
			// LOGIC for required == 1: check only tag presence
			if !exists {
				// Outbound missing in config.json → add from template
				cloned := CloneOutbound(&templateOutbound)
				parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *cloned)
				debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Added required outbound '%s' (required=1) from template", tag)
			} else {
				// Outbound present in config.json → keep existing version (don't touch)
				debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Required outbound '%s' (required=1) already exists, keeping existing", tag)
			}
		} else if required > 1 {
			// LOGIC for required > 1: always overwrite from template, regardless of presence in config.json
			// Clone once before exists check for optimization
			cloned := CloneOutbound(&templateOutbound)
			if exists {
				// Outbound exists in config.json → replace via pointer with version from template
				*existingOutbound = *cloned
				debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Replaced outbound '%s' (required=%d) with template version (always overwrite)", tag, required)
			} else {
				// Outbound missing in config.json → add from template
				parserConfig.ParserConfig.Outbounds = append(parserConfig.ParserConfig.Outbounds, *cloned)
				debuglog.Log("INFO", debuglog.LevelInfo, debuglog.UseGlobal, "ConfigWizard: Added required outbound '%s' (required=%d) from template", tag, required)
			}
		}
	}
}

// CloneOutbound creates a deep copy of OutboundConfig.
func CloneOutbound(src *config.OutboundConfig) *config.OutboundConfig {
	dst := &config.OutboundConfig{
		Tag:          src.Tag,
		Type:         src.Type,
		Comment:      src.Comment,
		AddOutbounds: make([]string, len(src.AddOutbounds)),
	}

	// Copy Wizard (support both formats)
	if src.Wizard != nil {
		// If it's a map, create deep copy
		if wizardMap, ok := src.Wizard.(map[string]interface{}); ok {
			dst.Wizard = deepCopyValue(wizardMap)
		} else {
			// If it's a string, just copy
			dst.Wizard = src.Wizard
		}
	}
	copy(dst.AddOutbounds, src.AddOutbounds)

	// Copy Options
	if src.Options != nil {
		dst.Options = make(map[string]interface{})
		for k, v := range src.Options {
			dst.Options[k] = deepCopyValue(v)
		}
	}

	// Copy Filters
	if src.Filters != nil {
		dst.Filters = make(map[string]interface{})
		for k, v := range src.Filters {
			dst.Filters[k] = deepCopyValue(v)
		}
	}

	// Copy PreferredDefault
	if src.PreferredDefault != nil {
		dst.PreferredDefault = make(map[string]interface{})
		for k, v := range src.PreferredDefault {
			dst.PreferredDefault[k] = deepCopyValue(v)
		}
	}

	return dst
}

// deepCopyValue creates a deep copy of a value (for map and slice).
func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, vv := range val {
			result[k] = deepCopyValue(vv)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, vv := range val {
			result[i] = deepCopyValue(vv)
		}
		return result
	default:
		return v
	}
}
