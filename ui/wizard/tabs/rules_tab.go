// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл rules_tab.go содержит функцию CreateRulesTab, которая создает UI второго таба визарда:
//   - Отображение правил маршрутизации из шаблона (SelectableRuleStates)
//   - Выбор outbound для каждого правила через Select виджеты
//   - Отображение пользовательских правил (CustomRules)
//   - Кнопки добавления, редактирования и удаления правил
//   - Выбор финального outbound (FinalOutboundSelect)
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
// Содержит сложную логику управления виджетами правил (RuleWidget) и их синхронизации с моделью.
//
// Используется в:
//   - wizard.go - при создании окна визарда, вызывается CreateRulesTab(presenter, showAddRuleDialog)
//   - presenter_rules.go - RefreshRulesTab вызывает CreateRulesTab для обновления содержимого таба
//
// Взаимодействует с:
//   - presenter - все действия пользователя обрабатываются через методы presenter
//   - dialogs/add_rule_dialog.go - вызывает ShowAddRuleDialog для добавления/редактирования правил
package tabs

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowAddRuleDialogFunc is a function type for showing the add rule dialog.
type ShowAddRuleDialogFunc func(p *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int)

// CreateRulesTab creates the Rules tab UI.
// showAddRuleDialog is a function that will be called to show the add rule dialog.
func CreateRulesTab(presenter *wizardpresentation.WizardPresenter, showAddRuleDialog ShowAddRuleDialogFunc) fyne.CanvasObject {
	model := presenter.Model()
	guiState := presenter.GUIState()

	if model.TemplateData == nil {
		templateFileName := wizardtemplate.GetTemplateFileName()
		return container.NewVBox(
			widget.NewLabel(fmt.Sprintf("Template file bin/%s not found.", templateFileName)),
			widget.NewLabel("Create the template file to enable this tab."),
		)
	}

	presenter.InitializeTemplateState()

	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(model))

	// Set flag to block callbacks during initialization
	guiState.UpdatingOutboundOptions = true

	// Initialize CustomRules if needed
	if model.CustomRules == nil {
		model.CustomRules = make([]*wizardmodels.RuleState, 0)
	}

	// Create RuleWidgets for selectable rules
	rulesBox := container.NewVBox()
	if len(model.SelectableRuleStates) == 0 {
		rulesBox.Add(widget.NewLabel("No selectable rules defined in template."))
	} else {
		for i := range model.SelectableRuleStates {
			ruleState := model.SelectableRuleStates[i]
			idx := i

			// Only show outbound selector if rule has "outbound" field
			var outboundSelect *widget.Select
			var outboundRow fyne.CanvasObject
			if ruleState.Rule.HasOutbound {
				wizardmodels.EnsureDefaultOutbound(ruleState, availableOutbounds)
				outboundSelect = widget.NewSelect(availableOutbounds, func(value string) {
					// Ignore callback during programmatic update
					if guiState.UpdatingOutboundOptions {
						return
					}
					model.SelectableRuleStates[idx].SelectedOutbound = value
					model.TemplatePreviewNeedsUpdate = true
				})
				outboundSelect.SetSelected(ruleState.SelectedOutbound)
				if !ruleState.Enabled {
					outboundSelect.Disable()
				}
				outboundRow = container.NewHBox(
					widget.NewLabel("Outbound:"),
					outboundSelect,
				)
			}

			// Create RuleWidget and add to GUIState
			ruleWidget := &wizardpresentation.RuleWidget{
				Select:    outboundSelect,
				RuleState: ruleState,
			}
			guiState.RuleOutboundSelects = append(guiState.RuleOutboundSelects, ruleWidget)

			checkbox := widget.NewCheck(ruleState.Rule.Label, func(val bool) {
				model.SelectableRuleStates[idx].Enabled = val
				model.TemplatePreviewNeedsUpdate = true
				if outboundSelect != nil {
					if val {
						outboundSelect.Enable()
					} else {
						outboundSelect.Disable()
					}
				}
			})
			checkbox.SetChecked(ruleState.Enabled)

			// Create checkbox container with optional info button for description
			checkboxContainer := container.NewHBox(checkbox)
			if ruleState.Rule.Description != "" {
				infoButton := widget.NewButton("?", func() {
					dialog.ShowInformation(ruleState.Rule.Label, ruleState.Rule.Description, guiState.Window)
				})
				infoButton.Importance = widget.LowImportance
				checkboxContainer.Add(infoButton)
			}

			rowContent := []fyne.CanvasObject{checkboxContainer, layout.NewSpacer()}
			if outboundRow != nil {
				rowContent = append(rowContent, outboundRow)
			}
			rulesBox.Add(container.NewHBox(rowContent...))
		}
	}

	// Display custom rules
	for i := range model.CustomRules {
		customRule := model.CustomRules[i]
		idx := i

		wizardmodels.EnsureDefaultOutbound(customRule, availableOutbounds)

		outboundSelect := widget.NewSelect(availableOutbounds, func(value string) {
			if guiState.UpdatingOutboundOptions {
				return
			}
			model.CustomRules[idx].SelectedOutbound = value
			model.TemplatePreviewNeedsUpdate = true
		})
		outboundSelect.SetSelected(customRule.SelectedOutbound)
		if !customRule.Enabled {
			outboundSelect.Disable()
		}

		// Create RuleWidget for custom rule
		customRuleWidget := &wizardpresentation.RuleWidget{
			Select:    outboundSelect,
			RuleState: customRule,
		}
		guiState.RuleOutboundSelects = append(guiState.RuleOutboundSelects, customRuleWidget)

		// Edit button
		editButton := widget.NewButton("✏️", func() {
			showAddRuleDialog(presenter, customRule, idx)
		})
		editButton.Importance = widget.LowImportance

		// Delete button
		deleteButton := widget.NewButton("❌", func() {
			// Create copy of index for closure
			deleteIdx := idx
			// Delete rule from model
			model.CustomRules = append(model.CustomRules[:deleteIdx], model.CustomRules[deleteIdx+1:]...)
			// Remove from GUIState
			newRuleWidgets := make([]*wizardpresentation.RuleWidget, 0, len(guiState.RuleOutboundSelects)-1)
			for _, rw := range guiState.RuleOutboundSelects {
				if r, ok := rw.RuleState.(*wizardmodels.RuleState); ok && r != customRule {
					newRuleWidgets = append(newRuleWidgets, rw)
				}
			}
			guiState.RuleOutboundSelects = newRuleWidgets
			model.TemplatePreviewNeedsUpdate = true
			// Recreate tab content
			refreshWrapper := func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject {
				return CreateRulesTab(p, showAddRuleDialog)
			}
			presenter.RefreshRulesTab(refreshWrapper)
		})
		deleteButton.Importance = widget.LowImportance

		checkbox := widget.NewCheck(customRule.Rule.Label, func(val bool) {
			model.CustomRules[idx].Enabled = val
			model.TemplatePreviewNeedsUpdate = true
			if val {
				outboundSelect.Enable()
			} else {
				outboundSelect.Disable()
			}
		})
		checkbox.SetChecked(customRule.Enabled)

		rowContent := []fyne.CanvasObject{
			checkbox,
			editButton,
			deleteButton,
			layout.NewSpacer(),
			container.NewHBox(
				widget.NewLabel("Outbound:"),
				outboundSelect,
			),
		}
		rulesBox.Add(container.NewHBox(rowContent...))
	}

	wizardbusiness.EnsureFinalSelected(model, availableOutbounds)
	finalSelect := widget.NewSelect(availableOutbounds, func(value string) {
		// Ignore callback during programmatic update
		if guiState.UpdatingOutboundOptions {
			return
		}
		model.SelectedFinalOutbound = value
		model.TemplatePreviewNeedsUpdate = true
	})
	finalSelect.SetSelected(model.SelectedFinalOutbound)
	guiState.FinalOutboundSelect = finalSelect

	// Add Rule button - add inside rulesBox
	addRuleButton := widget.NewButton("➕ Add Rule", func() {
		showAddRuleDialog(presenter, nil, -1)
	})
	addRuleButton.Importance = widget.LowImportance
	rulesBox.Add(addRuleButton)

	rulesScroll := CreateRulesScroll(guiState, rulesBox)

	// Reset flag before refreshOutboundOptions, as it will set it if needed
	guiState.UpdatingOutboundOptions = false
	presenter.RefreshOutboundOptions()

	return container.NewVBox(
		widget.NewLabel("Selectable rules"),
		rulesScroll,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Final outbound:"),
			finalSelect,
			layout.NewSpacer(),
		),
	)
}

// CreateRulesScroll creates a scrollable container for rules content.
func CreateRulesScroll(guiState *wizardpresentation.GUIState, content fyne.CanvasObject) fyne.CanvasObject {
	maxHeight := guiState.Window.Canvas().Size().Height * 0.65
	if maxHeight <= 0 {
		maxHeight = 430
	}
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(0, maxHeight))
	return scroll
}
