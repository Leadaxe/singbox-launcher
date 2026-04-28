// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_ui_updater.go содержит реализацию UIUpdater интерфейса в WizardPresenter.
//
// Методы UIUpdater:
//   - UpdateParserConfig - обновляет текст ParserConfig
//   - UpdateTemplatePreview - обновляет preview шаблона (с обработкой больших текстов)
//   - UpdateSaveProgress, UpdateSaveButtonText - управление прогрессом и кнопкой Save
//
// UIUpdater позволяет бизнес-логике обновлять GUI без прямой зависимости от Fyne виджетов.
// Большинство методов шлют работу в UI через SafeFyneDo (presenter.go). Исключение:
// UpdateParserConfig обновляет entry синхронно на потоке вызывающего кода (см. комментарий у метода).
//
// Реализация UIUpdater - это отдельная ответственность от других методов презентера.
// Содержит много однотипных методов обновления разных виджетов.
// Является мостом между бизнес-логикой (business) и GUI (Fyne виджеты).
//
// Используется в:
//   - business/parser.go - вызывает методы UIUpdater для обновления GUI при парсинге
//   - business/loader.go - вызывает методы UIUpdater при загрузке конфигурации
//   - presenter_async.go - вызывает UpdateTemplatePreview при обновлении preview
package presentation

import "singbox-launcher/internal/locale"

// UpdateParserConfig обновляет текст поля ParserConfig и список конфигуратора outbounds.
//
// Выполняется синхронно на потоке вызывающего кода. Нельзя откладывать через fyne.Do без ожидания:
// иначе следующий MergeGUIToModel (например второе нажатие Add подряд) прочитает устаревший текст
// entry и затрёт model.ParserConfigJSON — append снова посчитает len(proxies) как до первого Add
// и выдаст тот же числовой tag_prefix.
//
// Все текущие вызовы идут из обработчиков UI Fyne (главный поток).
func (p *WizardPresenter) UpdateParserConfig(text string) {
	if p.guiState == nil {
		return
	}
	if p.guiState.ParserConfigEntry != nil {
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(text)
		p.guiState.ParserConfigUpdating = false
	}
	p.guiState.LastValidParserConfigJSON = text
	if p.guiState.RefreshOutboundsConfiguratorList != nil {
		p.guiState.RefreshOutboundsConfiguratorList()
	}
}

// UpdateTemplatePreview обновляет текст preview шаблона.
func (p *WizardPresenter) UpdateTemplatePreview(text string) {
	if p.guiState.TemplatePreviewEntry == nil {
		return
	}

	if len(text) > 50000 {
		p.UpdateUI(func() {
			p.guiState.TemplatePreviewEntry.SetText(locale.T("wizard.preview.loading_large"))
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText(locale.T("wizard.preview.status_loading_large"))
			}
		})

		go func() {
			p.UpdateUI(func() {
				p.guiState.TemplatePreviewEntry.SetText(text)
				p.model.TemplatePreviewNeedsUpdate = false
			})
		}()
	} else {
		p.UpdateUI(func() {
			p.guiState.TemplatePreviewEntry.SetText(text)
			p.model.TemplatePreviewNeedsUpdate = false
		})
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
