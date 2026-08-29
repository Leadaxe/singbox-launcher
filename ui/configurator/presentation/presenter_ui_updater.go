// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_ui_updater.go содержит реализацию UIUpdater интерфейса в WizardPresenter.
//
// Методы UIUpdater:
//   - RefreshOutboundsConfiguratorList - пересобирает список конфигуратора Направлений
//   - UpdateTemplatePreview - обновляет preview шаблона (с обработкой больших текстов)
//   - UpdateSaveProgress, UpdateSaveButtonText - управление прогрессом и кнопкой Save
//
// UIUpdater позволяет бизнес-логике обновлять GUI без прямой зависимости от Fyne виджетов.
// Большинство методов шлют работу в UI через SafeFyneDo (presenter.go). Исключение:
// RefreshOutboundsConfiguratorList работает синхронно на потоке вызывающего кода (см. комментарий у метода).
//
// Реализация UIUpdater - это отдельная ответственность от других методов презентера.
// Содержит много однотипных методов обновления разных виджетов.
// Является мостом между бизнес-логикой (business) и GUI (Fyne виджеты).
//
// Используется в:
//   - business/parser.go - вызывает методы UIUpdater для обновления GUI при парсинге
//   - business/loader.go - вызывает методы UIUpdater при загрузке конфигурации
package presentation

// RefreshOutboundsConfiguratorList пересобирает список конфигуратора
// Направлений (SPEC 117 W5: полезный остаток снесённого транспорта
// UpdateParserConfig(text) — сам текстовый транспорт мёртв с SPEC 104).
//
// Выполняется синхронно на потоке вызывающего кода; все текущие вызовы идут
// из обработчиков UI Fyne (главный поток).
func (p *WizardPresenter) RefreshOutboundsConfiguratorList() {
	if p.guiState == nil {
		return
	}
	if p.guiState.RefreshOutboundsConfiguratorList != nil {
		p.guiState.RefreshOutboundsConfiguratorList()
	}
}

// UpdateSaveProgress обновляет прогресс сохранения (0.0-1.0, -1 для скрытия).
func (p *WizardPresenter) UpdateSaveProgress(progress float64) {
	p.UpdateUI(func() {
		if p.guiState.SaveProgress == nil {
			return
		}
		if progress < 0 {
			p.guiState.SaveProgress.Hide()
			p.guiState.SaveProgress.SetValue(0)
			p.guiState.SaveInProgress = false
		} else {
			p.guiState.SaveProgress.SetValue(progress)
			p.guiState.SaveProgress.Show()
			p.guiState.SaveInProgress = true
		}
	})
}

// UpdateSaveStatusText sets the status label (left of Prev). Empty string hides it.
func (p *WizardPresenter) UpdateSaveStatusText(text string) {
	p.UpdateUI(func() {
		if p.guiState.SaveStatusLabel == nil {
			return
		}
		if text == "" {
			p.guiState.SaveStatusLabel.SetText("")
			p.guiState.SaveStatusLabel.Hide()
		} else {
			p.guiState.SaveStatusLabel.SetText(text)
			p.guiState.SaveStatusLabel.Show()
		}
	})
}

// UpdateSaveButtonText обновляет текст кнопки Save (пустая строка для скрытия).
func (p *WizardPresenter) UpdateSaveButtonText(text string) {
	p.UpdateUI(func() {
		if p.guiState.SaveButton == nil {
			return
		}
		if text == "" {
			p.guiState.SaveButton.Hide()
			p.guiState.SaveButton.Disable()
		} else {
			p.guiState.SaveButton.SetText(text)
			p.guiState.SaveButton.Show()
			p.guiState.SaveButton.Enable()
		}
	})
}
