// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_methods.go содержит методы управления UI и инициализации:
//   - SetCheckURLState - управление состоянием кнопки Check и прогресс-бара проверки URL
//   - SetSaveState - управление состоянием кнопки Save и прогресс-бара сохранения
//   - RefreshOutboundOptions - обновление опций outbound для всех правил маршрутизации
//   - InitializeTemplateState - инициализация состояния шаблона (секции, правила, outbounds)
//   - SetTemplatePreviewText - установка текста preview с обработкой больших текстов
//
// Эти методы инкапсулируют логику управления виджетами и синхронизации с моделью.
// Методы управления UI и инициализации, отдельные от асинхронных операций.
// Содержат вспомогательные методы, используемые в разных частях презентера.
//
// Используется в:
//   - wizard.go - InitializeTemplateState вызывается при инициализации визарда
//   - tabs/rules_tab.go - RefreshOutboundOptions вызывается при обновлении правил
//   - presenter_async.go - SetTemplatePreviewText вызывается при обновлении preview
//   - presenter_save.go - SetSaveState вызывается для управления прогресс-баром сохранения
package presentation

import (
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// SetCheckURLState управляет состоянием кнопки Check и прогресс-бара.
func (p *WizardPresenter) SetCheckURLState(statusText string, buttonText string, progress float64) {
	SafeFyneDo(p.guiState.Window, func() {
		if statusText != "" && p.guiState.URLStatusLabel != nil {
			p.guiState.URLStatusLabel.SetText(statusText)
		}

		progressVisible := false
		if progress < 0 {
			if p.guiState.CheckURLProgress != nil {
				p.guiState.CheckURLProgress.Hide()
				p.guiState.CheckURLProgress.SetValue(0)
			}
		} else {
			if p.guiState.CheckURLProgress != nil {
				p.guiState.CheckURLProgress.SetValue(progress)
				p.guiState.CheckURLProgress.Show()
			}
			progressVisible = true
		}

		buttonVisible := false
		if progressVisible {
			if p.guiState.CheckURLButton != nil {
				p.guiState.CheckURLButton.Hide()
			}
		} else if buttonText == "" {
			if p.guiState.CheckURLButton != nil {
				p.guiState.CheckURLButton.Hide()
			}
		} else {
			if p.guiState.CheckURLButton != nil {
				p.guiState.CheckURLButton.SetText(buttonText)
				p.guiState.CheckURLButton.Show()
				p.guiState.CheckURLButton.Enable()
			}
			buttonVisible = true
		}

		if p.guiState.CheckURLPlaceholder != nil {
			if buttonVisible || progressVisible {
				p.guiState.CheckURLPlaceholder.Show()
			} else {
				p.guiState.CheckURLPlaceholder.Hide()
			}
		}
	})
}

// SetSaveState управляет состоянием кнопки Save и прогресс-бара.
func (p *WizardPresenter) SetSaveState(buttonText string, progress float64) {
	SafeFyneDo(p.guiState.Window, func() {
		progressVisible := false
		if progress < 0 {
			if p.guiState.SaveProgress != nil {
				p.guiState.SaveProgress.Hide()
				p.guiState.SaveProgress.SetValue(0)
			}
			p.guiState.SaveInProgress = false
		} else {
			if p.guiState.SaveProgress != nil {
				p.guiState.SaveProgress.SetValue(progress)
				p.guiState.SaveProgress.Show()
			}
			progressVisible = true
			p.guiState.SaveInProgress = true
		}

		buttonVisible := false
		if progressVisible {
			if p.guiState.SaveButton != nil {
				p.guiState.SaveButton.Hide()
				p.guiState.SaveButton.Disable()
			}
		} else if buttonText == "" {
			if p.guiState.SaveButton != nil {
				p.guiState.SaveButton.Hide()
				p.guiState.SaveButton.Disable()
			}
		} else {
			if p.guiState.SaveButton != nil {
				p.guiState.SaveButton.SetText(buttonText)
				p.guiState.SaveButton.Show()
				p.guiState.SaveButton.Enable()
			}
			buttonVisible = true
		}

		if p.guiState.SavePlaceholder != nil {
			if buttonVisible || progressVisible {
				p.guiState.SavePlaceholder.Show()
			} else {
				p.guiState.SavePlaceholder.Hide()
			}
		}
	})
}

// RefreshOutboundOptions обновляет опции outbound для всех правил.
func (p *WizardPresenter) RefreshOutboundOptions() {
	options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
	optionsMap := make(map[string]bool, len(options))
	for _, opt := range options {
		optionsMap[opt] = true
	}

	ensureSelected := func(ruleState *wizardmodels.RuleState) {
		if !ruleState.Rule.HasOutbound {
			return
		}
		if ruleState.SelectedOutbound != "" && optionsMap[ruleState.SelectedOutbound] {
			return
		}
		candidate := ruleState.Rule.DefaultOutbound
		if candidate == "" || !optionsMap[candidate] {
			candidate = options[0]
		}
		ruleState.SelectedOutbound = candidate
	}

	wizardbusiness.EnsureFinalSelected(p.model, options)

	p.guiState.UpdatingOutboundOptions = true
	defer func() {
		p.guiState.UpdatingOutboundOptions = false
	}()

	SafeFyneDo(p.guiState.Window, func() {
		for _, ruleWidget := range p.guiState.RuleOutboundSelects {
			if ruleWidget.RuleState == nil {
				continue
			}
			ruleState, ok := ruleWidget.RuleState.(*wizardmodels.RuleState)
			if !ok || !ruleState.Rule.HasOutbound || ruleWidget.Select == nil {
				continue
			}
			ensureSelected(ruleState)
			ruleWidget.Select.Options = options
			ruleWidget.Select.SetSelected(ruleState.SelectedOutbound)
			ruleWidget.Select.Refresh()
		}

		if p.guiState.FinalOutboundSelect != nil {
			p.guiState.FinalOutboundSelect.Options = options
			p.guiState.FinalOutboundSelect.SetSelected(p.model.SelectedFinalOutbound)
			p.guiState.FinalOutboundSelect.Refresh()
		}
	})
}

// InitializeTemplateState инициализирует состояние шаблона.
func (p *WizardPresenter) InitializeTemplateState() {
	if p.model.TemplateData == nil {
		return
	}
	if p.model.TemplateSectionSelections == nil {
		p.model.TemplateSectionSelections = make(map[string]bool)
	}
	for _, key := range p.model.TemplateData.SectionOrder {
		if _, ok := p.model.TemplateSectionSelections[key]; !ok {
			p.model.TemplateSectionSelections[key] = true
		}
	}

	options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))

	if len(p.model.SelectableRuleStates) == 0 {
		for _, rule := range p.model.TemplateData.SelectableRules {
			outbound := rule.DefaultOutbound
			if outbound == "" {
				outbound = options[0]
			}
			p.model.SelectableRuleStates = append(p.model.SelectableRuleStates, &wizardmodels.RuleState{
				Rule:             rule,
				SelectedOutbound: outbound,
				Enabled:          rule.IsDefault,
			})
		}
	} else {
		for _, ruleState := range p.model.SelectableRuleStates {
			wizardmodels.EnsureDefaultOutbound(ruleState, options)
		}
	}

	wizardbusiness.EnsureFinalSelected(p.model, options)
}

// SetTemplatePreviewText устанавливает текст предпросмотра шаблона.
func (p *WizardPresenter) SetTemplatePreviewText(text string) {
	// Optimization: don't update if text hasn't changed
	if p.model.TemplatePreviewText == text {
		if p.model.TemplatePreviewNeedsUpdate && p.guiState.TemplatePreviewEntry != nil && p.guiState.TemplatePreviewEntry.Text == text {
			p.model.TemplatePreviewNeedsUpdate = false
		}
		return
	}

	p.model.TemplatePreviewText = text
	if p.guiState.TemplatePreviewEntry == nil {
		p.model.TemplatePreviewNeedsUpdate = false
		return
	}

	if p.guiState.TemplatePreviewEntry.Text == text {
		p.model.TemplatePreviewNeedsUpdate = false
		return
	}

	// For large texts (>50KB) show loading message before insertion
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
