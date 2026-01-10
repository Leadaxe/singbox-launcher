// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл interfaces.go определяет интерфейсы и адаптеры, которые могут использоваться
// без зависимостей от GUI (для тестирования).
//
// Интерфейсы определены здесь без build constraints, чтобы тесты могли их использовать.
package business

import (
	"singbox-launcher/core/config"
)

// ConfigService предоставляет доступ к генерации outbounds из ParserConfig.
// Интерфейс определен здесь для использования в тестах без зависимости от core.
type ConfigService interface {
	GenerateOutboundsFromParserConfig(parserConfig *config.ParserConfig, tagCounts map[string]int, progressCallback func(float64, string)) (*config.OutboundGenerationResult, error)
}

// FileServiceInterface предоставляет доступ к путям конфигурации и файлам.
// Интерфейс определен здесь для использования в тестах без зависимости от core/services.
type FileServiceInterface interface {
	ConfigPath() string
	ExecDir() string
}
