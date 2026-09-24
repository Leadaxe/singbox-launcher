// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл template_loader.go определяет интерфейс TemplateLoader для загрузки TemplateData.
//
// TemplateLoader позволяет презентеру загружать TemplateData без прямой зависимости
// от реализации загрузки, что делает код тестируемым (можно использовать моки).
// Реализация по умолчанию (DefaultTemplateLoader) использует wizardtemplate.LoadTemplateData.
//
// Используется в:
//   - presentation/presenter.go - WizardPresenter использует TemplateLoader для загрузки шаблона при инициализации
package business

import (
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/paths"
)

// TemplateLoader загружает TemplateData.
type TemplateLoader interface {
	LoadTemplateData(layout paths.Layout) (*wizardtemplate.TemplateData, error)
}

// DefaultTemplateLoader - реализация TemplateLoader по умолчанию.
type DefaultTemplateLoader struct{}

// LoadTemplateData загружает TemplateData из файла.
func (*DefaultTemplateLoader) LoadTemplateData(layout paths.Layout) (*wizardtemplate.TemplateData, error) {
	return wizardtemplate.LoadTemplateData(layout)
}
