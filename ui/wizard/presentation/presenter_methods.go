// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_methods.go содержит методы управления UI и инициализации:
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
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// SetSaveState управляет состоянием кнопки Save и прогресс-бара.
func (p *WizardPresenter) SetSaveState(buttonText string, progress float64) {
	p.UpdateUI(func() {
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

// CancelSaveOperation отменяет текущую операцию сохранения.
// Устанавливает SaveInProgress в false, что позволяет горутине сохранения завершиться.
func (p *WizardPresenter) CancelSaveOperation() {
	p.UpdateUI(func() {
		p.guiState.SaveInProgress = false
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
	debuglog.DebugLog("RefreshOutboundOptions: UpdatingOutboundOptions set to true")

	p.UpdateUI(func() {
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

		// Reset flag AFTER all SetSelected() calls to prevent callbacks from firing
		// This must be done inside UpdateUI() because UpdateUI() executes asynchronously via fyne.Do
		p.guiState.UpdatingOutboundOptions = false
		debuglog.DebugLog("RefreshOutboundOptions: UpdatingOutboundOptions reset to false")
	})
}

// InitializeTemplateState инициализирует состояние шаблона.
// Создаёт RuleState для каждого selectable rule из шаблона (если ещё не загружены из state.json).
func (p *WizardPresenter) InitializeTemplateState() {
	if p.model.TemplateData == nil {
		return
	}

	options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))

	if len(p.model.SelectableRuleStates) == 0 {
		for _, rule := range p.model.TemplateData.SelectableRules {
			outbound := rule.DefaultOutbound
			if outbound == "" {
				outbound = options[0]
			}
			enabled := rule.IsDefault
			if !services.AllSRSDownloaded(p.model.ExecDir, rule.RuleSets) {
				enabled = false
			}
			p.model.SelectableRuleStates = append(p.model.SelectableRuleStates, &wizardmodels.RuleState{
				Rule:             rule,
				SelectedOutbound: outbound,
				Enabled:          enabled,
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
