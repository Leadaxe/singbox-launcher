// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл loader.go: загрузка ParserConfig для инициализации визарда.
//
// LoadConfigFromFile теперь читает только из template (SPEC 045 cleanup).
// state.json — canonical, читается отдельно через `state.Load` в presenter,
// здесь нужен только fallback для свежей установки (state.json ещё нет).
//
// До SPEC 045 был ещё один путь — извлечение `@ParserConfig` из config.json
// через `parser.ExtractParserConfig`. Удалён вместе с самим блоком в
// `core/build` (нет дубликата parser_config'а в config.json — только в state).
//
// Используется в:
//   - configurator.go — LoadConfigFromFile вызывается на инициализации,
//     если state.json отсутствует (presenter сначала пробует state.Load).
package business

import (
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	wizardtemplate "singbox-launcher/core/template"
)

// LoadConfigFromFile теперь — pure template loader. Сигнатура сохранена
// для совместимости с существующими callsite'ами в configurator.go;
// `fileService` остался в параметрах но не используется (раньше нужен был
// для config.json fallback'а — выпилен).
//
// Returns (loaded bool, parserConfigJSON string, sourceURLs string, error):
//   - state.json'а здесь нет: его читает presenter перед этим вызовом;
//     loader зовётся только если state.Load вернул error / no-state;
//   - templateData != nil + ParserConfig непустой → копия template parser_config;
//     sourceURLs всегда "" — у template нет user-specific URL'ов;
//   - templateData nil или ParserConfig пустой → false / "" / "" / nil
//     (визард работает в режиме «всё дефолт»).
func LoadConfigFromFile(_ FileServiceInterface, templateData *wizardtemplate.TemplateData) (bool, string, string, error) {
	if templateData == nil || templateData.ParserConfig == "" {
		debuglog.InfoLog("ConfigWizard: no template parser_config, using built-in defaults")
		return false, "", "", nil
	}

	debuglog.InfoLog("ConfigWizard: loading parser_config from template (no state.json yet)")
	var templateParserConfig config.ParserConfig
	if err := json.Unmarshal([]byte(templateData.ParserConfig), &templateParserConfig); err != nil {
		debuglog.ErrorLog("ConfigWizard: failed to parse template parser_config: %v", err)
		return false, "", "", nil
	}

	parserConfigJSON, err := SerializeParserConfig(&templateParserConfig)
	if err != nil {
		debuglog.ErrorLog("ConfigWizard: failed to serialize template parser_config: %v", err)
		return false, "", "", fmt.Errorf("failed to serialize parser_config: %w", err)
	}
	return true, parserConfigJSON, "", nil
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
