// Package business содержит бизнес-логику визарда конфигурации.
//
// Файл ui_updater.go определяет интерфейс UIUpdater для обновления GUI из бизнес-логики.
//
// UIUpdater позволяет бизнес-логике обновлять GUI без прямой зависимости от Fyne виджетов.
// Реализация UIUpdater находится в presentation.WizardPresenter, который обновляет виджеты GUIState.
//
// Этот интерфейс используется в функциях бизнес-логики (parser.go, generator.go и т.д.),
// которые выполняют асинхронные операции и должны обновлять GUI прогресс, статусы и preview.
//
// Используется в:
//   - business/parser.go - CheckURL и ParseAndPreview используют UIUpdater для обновления GUI
//   - presentation/presenter_ui_updater.go - WizardPresenter реализует интерфейс UIUpdater
package business

// UIUpdater предоставляет методы для обновления GUI из бизнес-логики.
type UIUpdater interface {
	// UpdateURLStatus обновляет статус проверки URL
	UpdateURLStatus(status string)
	// UpdateCheckURLProgress обновляет прогресс проверки URL (0.0-1.0, -1 для скрытия)
	UpdateCheckURLProgress(progress float64)
	// UpdateCheckURLButtonText обновляет текст кнопки Check (пустая строка для скрытия)
	UpdateCheckURLButtonText(text string)
	// UpdateOutboundsPreview обновляет текст preview outbounds
	UpdateOutboundsPreview(text string)
	// UpdateParserConfig обновляет текст ParserConfig
	UpdateParserConfig(text string)
	// UpdateTemplatePreview обновляет текст preview шаблона
	UpdateTemplatePreview(text string)
	// UpdateSaveProgress обновляет прогресс сохранения (0.0-1.0, -1 для скрытия)
	UpdateSaveProgress(progress float64)
	// UpdateSaveButtonText обновляет текст кнопки Save (пустая строка для скрытия)
	UpdateSaveButtonText(text string)
}
