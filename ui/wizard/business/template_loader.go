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
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// TemplateLoader загружает TemplateData.
type TemplateLoader interface {
	LoadTemplateData(execDir string) (*wizardtemplate.TemplateData, error)
}

// DefaultTemplateLoader - реализация TemplateLoader по умолчанию.
type DefaultTemplateLoader struct{}

// LoadTemplateData загружает TemplateData из файла.
func (l *DefaultTemplateLoader) LoadTemplateData(execDir string) (*wizardtemplate.TemplateData, error) {
	return wizardtemplate.LoadTemplateData(execDir)
}


