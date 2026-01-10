// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_ui_updater.go содержит реализацию UIUpdater интерфейса в WizardPresenter.
//
// Методы UIUpdater:
//   - UpdateURLStatus - обновляет статус проверки URL
//   - UpdateCheckURLProgress, UpdateCheckURLButtonText - управление прогрессом и кнопкой Check
//   - UpdateOutboundsPreview - обновляет preview сгенерированных outbounds
//   - UpdateParserConfig - обновляет текст ParserConfig
//   - UpdateTemplatePreview - обновляет preview шаблона (с обработкой больших текстов)
//   - UpdateSaveProgress, UpdateSaveButtonText - управление прогрессом и кнопкой Save
//
// UIUpdater позволяет бизнес-логике обновлять GUI без прямой зависимости от Fyne виджетов.
// Все методы обеспечивают безопасное обновление GUI из других горутин через SafeFyneDo
// (определена в presenter.go), что предотвращает паники при обновлении Fyne виджетов
// не из главного потока.
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

// UpdateURLStatus обновляет статус проверки URL.
func (p *WizardPresenter) UpdateURLStatus(status string) {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.URLStatusLabel != nil {
			p.guiState.URLStatusLabel.SetText(status)
		}
	})
}

// UpdateCheckURLProgress обновляет прогресс проверки URL (0.0-1.0, -1 для скрытия).
func (p *WizardPresenter) UpdateCheckURLProgress(progress float64) {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.CheckURLProgress == nil {
			return
		}
		if progress < 0 {
			p.guiState.CheckURLProgress.Hide()
			p.guiState.CheckURLProgress.SetValue(0)
		} else {
			p.guiState.CheckURLProgress.SetValue(progress)
			p.guiState.CheckURLProgress.Show()
		}
	})
}

// UpdateCheckURLButtonText обновляет текст кнопки Check (пустая строка для скрытия).
func (p *WizardPresenter) UpdateCheckURLButtonText(text string) {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.CheckURLButton == nil {
			return
		}
		if text == "" {
			p.guiState.CheckURLButton.Hide()
		} else {
			p.guiState.CheckURLButton.SetText(text)
			p.guiState.CheckURLButton.Show()
			p.guiState.CheckURLButton.Enable()
		}
	})
}

// UpdateOutboundsPreview обновляет текст preview outbounds.
func (p *WizardPresenter) UpdateOutboundsPreview(text string) {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.OutboundsPreview != nil {
			p.guiState.OutboundsPreview.SetText(text)
		}
	})
}

// UpdateParserConfig обновляет текст ParserConfig.
func (p *WizardPresenter) UpdateParserConfig(text string) {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.ParserConfigEntry != nil {
			p.guiState.ParserConfigUpdating = true
			p.guiState.ParserConfigEntry.SetText(text)
			p.guiState.ParserConfigUpdating = false
		}
	})
}

// UpdateTemplatePreview обновляет текст preview шаблона.
func (p *WizardPresenter) UpdateTemplatePreview(text string) {
	if p.guiState.TemplatePreviewEntry == nil {
		return
	}

	if len(text) > 50000 {
		SafeFyneDo(p.guiState.Window, func() {
			p.guiState.TemplatePreviewEntry.SetText("Loading large preview...")
			if p.guiState.TemplatePreviewStatusLabel != nil {
				p.guiState.TemplatePreviewStatusLabel.SetText("⏳ Loading large preview...")
			}
		})

		go func() {
			SafeFyneDo(p.guiState.Window, func() {
				p.guiState.TemplatePreviewEntry.SetText(text)
				p.model.TemplatePreviewNeedsUpdate = false
			})
		}()
	} else {
		SafeFyneDo(p.guiState.Window, func() {
			p.guiState.TemplatePreviewEntry.SetText(text)
			p.model.TemplatePreviewNeedsUpdate = false
		})
	}
}

// UpdateSaveProgress обновляет прогресс сохранения (0.0-1.0, -1 для скрытия).
func (p *WizardPresenter) UpdateSaveProgress(progress float64) {
	SafeFyneDo(p.guiState.Window, func() {
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

// UpdateSaveButtonText обновляет текст кнопки Save (пустая строка для скрытия).
func (p *WizardPresenter) UpdateSaveButtonText(text string) {
	SafeFyneDo(p.guiState.Window, func() {
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
