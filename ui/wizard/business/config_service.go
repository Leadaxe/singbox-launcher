//go:build cgo

// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл config_service.go содержит адаптер ConfigServiceAdapter для генерации outbounds:
//   - ConfigServiceAdapter - адаптер, который адаптирует core.ConfigService для использования в бизнес-логике
//
// ConfigService интерфейс определен в interfaces.go для использования в тестах без зависимости от core.
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

// ConfigServiceAdapter адаптирует core.ConfigService для использования в бизнес-логике.
type ConfigServiceAdapter struct {
	CoreConfigService *core.ConfigService
}

// GenerateOutboundsFromParserConfig вызывает соответствующий метод core.ConfigService.
func (a *ConfigServiceAdapter) GenerateOutboundsFromParserConfig(parserConfig *config.ParserConfig, tagCounts map[string]int, progressCallback func(float64, string)) (*config.OutboundGenerationResult, error) {
	return a.CoreConfigService.GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback)
}

