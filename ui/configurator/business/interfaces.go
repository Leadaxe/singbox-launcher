// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл interfaces.go определяет интерфейсы и адаптеры, которые могут использоваться
// без зависимостей от GUI (для тестирования).
//
// Интерфейсы определены здесь без build constraints, чтобы тесты могли их использовать.
package business

import (
	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
)

// ConfigService предоставляет доступ к генерации outbounds из ParserConfig
// и per-source refresh.
// Интерфейс определен здесь для использования в тестах без зависимости от core.
type ConfigService interface {
	GenerateOutboundsFromParserConfig(parserConfig *config.ParserConfig, tagCounts map[string]int, progressCallback func(float64, string)) (*config.OutboundGenerationResult, error)

	// RefreshSingleSubscription — SPEC 052 phase 7: триггерит fetch+meta+raw
	// для одного source по ID. Делает state.Load+Save — требует
	// существующего state.json. Используется auto-update heartbeat'ом и
	// VPN-event retry'ями (там state всегда уже есть).
	RefreshSingleSubscription(sourceID string) (*corestate.Source, error)

	// RefreshSourceInPlace — cold-start path: fetch+raw+meta для in-memory
	// Source pointer'а, без обращения к state.json. Используется UI Refresh
	// button'ом, чтобы работало на свежей инсталляции (без state.json) или
	// на не-сохранённых правках в визарде.
	RefreshSourceInPlace(src *corestate.Source) (bool, error)
}

// FileServiceInterface предоставляет доступ к путям конфигурации и sing-box.
// Интерфейс определен здесь для использования в тестах без зависимости от core/services.
type FileServiceInterface interface {
	ConfigPath() string
	ExecDir() string
	SingboxPath() string
}
