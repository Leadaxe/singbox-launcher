// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл config_service.go определяет интерфейс ConfigService и его адаптер для генерации outbounds:
//   - ConfigService - интерфейс для генерации outbounds из ParserConfig
//   - ConfigServiceAdapter - адаптер, который адаптирует core.ConfigService для использования в бизнес-логике
//
// ConfigService позволяет бизнес-логике генерировать outbounds без прямой зависимости
// от core.ConfigService, что делает код тестируемым (можно использовать моки в тестах)
// и позволяет абстрагироваться от конкретной реализации core.ConfigService.
//
// Определение интерфейсов и адаптеров - это отдельная ответственность.
// Используется паттерн Adapter для инверсии зависимостей (Dependency Inversion Principle).
// Упрощает тестирование бизнес-логики путем подмены реализации ConfigService.
//
// Используется в:
//   - business/parser.go - GenerateOutboundsFromParserConfig вызывается для генерации outbounds
//   - presentation/presenter.go - ConfigServiceAdapter создается в презентере и передается в бизнес-логику
package business

import (
	"singbox-launcher/core"
	"singbox-launcher/core/config"
)

// ConfigService предоставляет доступ к генерации outbounds из ParserConfig.
type ConfigService interface {
	GenerateOutboundsFromParserConfig(parserConfig *config.ParserConfig, tagCounts map[string]int, progressCallback func(float64, string)) (*config.OutboundGenerationResult, error)
}

// ConfigServiceAdapter адаптирует core.ConfigService для использования в бизнес-логике.
type ConfigServiceAdapter struct {
	CoreConfigService *core.ConfigService
}

// GenerateOutboundsFromParserConfig вызывает соответствующий метод core.ConfigService.
func (a *ConfigServiceAdapter) GenerateOutboundsFromParserConfig(parserConfig *config.ParserConfig, tagCounts map[string]int, progressCallback func(float64, string)) (*config.OutboundGenerationResult, error) {
	return a.CoreConfigService.GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback)
}

