// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл validator.go содержит функции валидации для различных структур данных визарда:
//   - ValidateParserConfig - валидация структуры и содержимого ParserConfig
//   - ValidateURL - валидация URL подписок (проверка формата, схемы, хоста)
//   - ValidateURI - валидация URI для прямых ссылок (vless://, vmess://, ssh:// и т.д.)
//   - ValidateOutbound - валидация конфигурации outbound (tag, type, длина)
//   - ValidateRule - валидация правил маршрутизации
//   - ValidateJSON - валидация JSON структуры и размера
//   - ValidateJSONSize, ValidateHTTPResponseSize - проверка размеров данных
//
// Эти функции являются чистыми функциями валидации (pure functions) - они не зависят от GUI
// и могут быть использованы для тестирования бизнес-логики без Fyne.
//
// Валидаторы находятся в wizard/business, а не в core/config:
// используют константы из wizard/utils (MaxURILength и т.д.) и core/config/parser (MaxConfigFileSize)
// которые специфичны для визарда (лимиты для UI операций),
// используются только в контексте визарда (parser.go, loader.go, create_config.go).
//
// Они используются в:
//   - parser.go - для валидации URL и ParserConfig перед обработкой
//   - loader.go - для валидации при загрузке конфигурации
//   - create_config.go - для валидации данных перед генерацией конфигурации
package business

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/parser"
	wizardutils "singbox-launcher/ui/configurator/utils"
)

// ValidateStringLength validates string length against min and max bounds.
func ValidateStringLength(s, fieldName string, min, max int) error {
	if len(s) > max {
		return fmt.Errorf("%s length (%d) exceeds maximum (%d)", fieldName, len(s), max)
	}
	if len(s) < min {
		return fmt.Errorf("%s length (%d) is less than minimum (%d)", fieldName, len(s), min)
	}
	return nil
}

// ValidateParserConfig validates ParserConfig structure and content.
func ValidateParserConfig(parserConfig *config.ParserConfig) error {
	if parserConfig == nil {
		return fmt.Errorf("ParserConfig is nil")
	}

	if parserConfig.ParserConfig.Proxies == nil {
		return fmt.Errorf("ParserConfig.Proxies is nil")
	}

	// Validate each proxy source
	for i, proxy := range parserConfig.ParserConfig.Proxies {
		if proxy.Source != "" {
			if err := ValidateURL(proxy.Source); err != nil {
				return fmt.Errorf("proxy source %d: invalid URL: %w", i, err)
			}
		}

		// Validate connections
		for j, conn := range proxy.Connections {
			if err := ValidateURI(conn); err != nil {
				return fmt.Errorf("proxy %d connection %d: invalid URI: %w", i, j, err)
			}
		}

		// Validate outbounds
		for j, outbound := range proxy.Outbounds {
			if err := ValidateOutbound(&outbound); err != nil {
				return fmt.Errorf("proxy %d outbound %d: %w", i, j, err)
			}
		}
	}

	// Validate global outbounds
	for i, outbound := range parserConfig.ParserConfig.Outbounds {
		if err := ValidateOutbound(&outbound); err != nil {
			return fmt.Errorf("global outbound %d: %w", i, err)
		}
	}

	return nil
}

// ValidateURL validates a URL string.
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL is empty")
	}

	if err := ValidateStringLength(urlStr, "URL", wizardutils.MinURILength, wizardutils.MaxURILength); err != nil {
		return err
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http, https, etc.)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// ValidateURI validates a URI string (for proxy connections).
func ValidateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI is empty")
	}

	if err := ValidateStringLength(uri, "URI", wizardutils.MinURILength, wizardutils.MaxURILength); err != nil {
		return err
	}

	// Basic URI format check (should start with protocol)
	if !strings.Contains(uri, "://") {
		return fmt.Errorf("URI must contain protocol (e.g., vless://, vmess://, ssh://)")
	}

	return nil
}

// ValidateOutbound validates an OutboundConfig.
func ValidateOutbound(outbound *config.OutboundConfig) error {
	if outbound == nil {
		return fmt.Errorf("outbound is nil")
	}

	if outbound.Tag == "" {
		return fmt.Errorf("outbound tag is empty")
	}

	if outbound.Type == "" {
		return fmt.Errorf("outbound type is empty")
	}

	// Validate tag length
	if err := ValidateStringLength(outbound.Tag, "outbound tag", 1, 256); err != nil {
		return err
	}

	return nil
}

// ValidateRule validates a rule structure.
func ValidateRule(rule map[string]interface{}) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}

	// Check for required fields based on rule type
	// This is a basic validation - more specific validation can be added
	if len(rule) == 0 {
		return fmt.Errorf("rule is empty")
	}

	return nil
}

// ValidateJSONSize validates that JSON data size is within limits.
func ValidateJSONSize(jsonData []byte) error {
	if len(jsonData) > parser.MaxConfigFileSize {
		return fmt.Errorf("JSON size (%d bytes) exceeds maximum (%d bytes)", len(jsonData), parser.MaxConfigFileSize)
	}
	return nil
}

// ValidateJSON validates JSON structure and size.
func ValidateJSON(jsonData []byte) error {
	if err := ValidateJSONSize(jsonData); err != nil {
		return err
	}

	if !json.Valid(jsonData) {
		return fmt.Errorf("invalid JSON structure")
	}

	return nil
}

// ValidateSize validates that size is within the specified maximum limit.
// This is a generic function for validating sizes of various entities (files, HTTP responses, etc.).
func ValidateSize(size int64, maxSize int64, entityName string) error {
	if size > maxSize {
		return fmt.Errorf("%s size (%d bytes) exceeds maximum (%d bytes)", entityName, size, maxSize)
	}
	return nil
}

// ValidateHTTPResponseSize validates HTTP response size.
// Uses ValidateSize internally for consistency.
func ValidateHTTPResponseSize(size int64) error {
	return ValidateSize(size, wizardutils.MaxSubscriptionSize, "HTTP response")
}

// ValidateParserConfigJSON validates ParserConfig JSON text.
func ValidateParserConfigJSON(jsonText string) error {
	if jsonText == "" {
		return fmt.Errorf("ParserConfig JSON is empty")
	}

	jsonBytes := []byte(jsonText)
	if err := ValidateJSON(jsonBytes); err != nil {
		return fmt.Errorf("invalid ParserConfig JSON: %w", err)
	}

	var parserConfig config.ParserConfig
	if err := json.Unmarshal(jsonBytes, &parserConfig); err != nil {
		return fmt.Errorf("failed to parse ParserConfig JSON: %w", err)
	}

	return ValidateParserConfig(&parserConfig)
}
