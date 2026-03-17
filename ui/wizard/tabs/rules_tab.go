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
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowAddRuleDialogFunc is a function type for showing the add rule dialog.
type ShowAddRuleDialogFunc func(p *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int)

// SRS button labels (managed in one place)
const (
	srsBtnDownload       = "⬇ srs"
	srsBtnLoading        = "🔄 srs"
	srsBtnDone           = "✔️ srs"
	srsGroupDownloadTimeout = 90 * time.Second
)

// srsEntriesTooltip возвращает строку URL для tooltip кнопки SRS.
func srsEntriesTooltip(entries []services.SRSEntry) string {
	if len(entries) == 0 {
		return ""
	}
	urls := make([]string, len(entries))
	for i, e := range entries {
		urls[i] = e.URL
	}
	return strings.Join(urls, "\n")
}

// runSRSDownloadAsync запускает скачивание SRS в горутине и по завершении обновляет UI (кнопка, outbound, onSuccess).
func runSRSDownloadAsync(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	srsEntries []services.SRSEntry,
	btn *ttwidget.Button,
	outboundSelect *widget.Select,
	onSuccess func(),
) {
	if model.ExecDir == "" {
		return
	}
	btn.Disable()
	btn.SetText(srsBtnLoading)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), srsGroupDownloadTimeout)
		defer cancel()
		err := services.DownloadSRSGroup(ctx, model.ExecDir, srsEntries)
		presenter.UpdateUI(func() {
			btn.Enable()
			if err != nil {
				btn.SetText(srsBtnDownload)
				ruleSetsDir := filepath.Join(model.ExecDir, constants.BinDirName, constants.RuleSetsDirName)
				downloadURL := ""
				if len(srsEntries) > 0 {
					downloadURL = srsEntries[0].URL
				}
				debuglog.DebugLog("rules_tab: SRS download failed")
				dialogs.ShowDownloadFailedManual(guiState.Window, "Rule-set (SRS) download failed", downloadURL, ruleSetsDir)
				return
			}
			btn.SetText(srsBtnDone)
			if outboundSelect != nil {
				outboundSelect.Enable()
			}
			onSuccess()
		})
	}()
}

// CreateRulesTab creates the Rules tab UI.
// showAddRuleDialog is a function that will be called to show the add rule dialog.
func CreateRulesTab(presenter *wizardpresentation.WizardPresenter, showAddRuleDialog ShowAddRuleDialogFunc) fyne.CanvasObject {
	model := presenter.Model()
	guiState := presenter.GUIState()

	// Validate template data
	if model.TemplateData == nil {
		return createTemplateNotFoundMessage()
	}

	// Initialize state
	initializeRulesTabState(presenter, model, guiState)
	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(model))

	// Create UI components
	rulesBox := createSelectableRulesUI(presenter, model, guiState, availableOutbounds)
	createCustomRulesUI(presenter, model, guiState, availableOutbounds, showAddRuleDialog, rulesBox)
	createAddRuleButton(presenter, showAddRuleDialog, rulesBox)
	finalSelect := createFinalOutboundSelect(presenter, model, guiState, availableOutbounds)

	// Create scrollable container
	rulesScroll := CreateRulesScroll(guiState, rulesBox)

	// RefreshOutboundOptions will reset UpdatingOutboundOptions flag and hasChanges after all SetSelected() calls
	presenter.RefreshOutboundOptions()

	// Build final container
	return buildRulesTabContainer(presenter, rulesScroll, finalSelect)
}

// createTemplateNotFoundMessage создает сообщение об отсутствии шаблона.
func createTemplateNotFoundMessage() fyne.CanvasObject {
	templateFileName := wizardtemplate.GetTemplateFileName()
	return container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Template file bin/%s not found.", templateFileName)),
		widget.NewLabel("Create the template file to enable this tab."),
	)
}

// initializeRulesTabState инициализирует состояние таба правил.
func initializeRulesTabState(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, guiState *wizardpresentation.GUIState) {
	presenter.InitializeTemplateState()

	// Очищаем старые виджеты перед созданием новых (важно при пересоздании вкладки)
	guiState.RuleOutboundSelects = make([]*wizardpresentation.RuleWidget, 0)

	// Set flag to block callbacks during initialization
	guiState.UpdatingOutboundOptions = true
	debuglog.DebugLog("rules_tab: UpdatingOutboundOptions set to true before creating widgets")

	// Initialize CustomRules if needed
	if model.CustomRules == nil {
		model.CustomRules = make([]*wizardmodels.RuleState, 0)
	}
}

// createSelectableRulesUI создает UI для selectable rules из шаблона.
// Возвращает VBox контейнер для добавления custom rules и кнопки Add Rule.
func createSelectableRulesUI(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, guiState *wizardpresentation.GUIState, availableOutbounds []string) *fyne.Container {
	rulesBox := container.NewVBox()

	if len(model.SelectableRuleStates) == 0 {
		rulesBox.Add(widget.NewLabel("No selectable rules defined in template."))
		return rulesBox
	}

	for i := range model.SelectableRuleStates {
		ruleState := model.SelectableRuleStates[i]
		idx := i
		srsEntries := services.GetSRSEntries(ruleState.Rule.RuleSets)
		srsDownloaded := services.AllSRSDownloaded(model.ExecDir, ruleState.Rule.RuleSets)

		outboundSelect, outboundRow := createOutboundSelectorForSelectableRule(
			presenter, model, guiState, ruleState, idx, availableOutbounds, srsDownloaded,
		)

		var srsButton *ttwidget.Button
		enableRuleOnSRSSuccess := new(bool)
		checkbox := createSelectableRuleCheckbox(presenter, model, guiState, ruleState, idx, outboundSelect, &srsButton, enableRuleOnSRSSuccess)
		if len(srsEntries) > 0 {
			srsButton = createSRSButton(presenter, model, guiState, ruleState, idx, srsEntries, checkbox, outboundSelect, enableRuleOnSRSSuccess)
		}

		ruleWidget := &wizardpresentation.RuleWidget{
			Select:    outboundSelect,
			Checkbox:  checkbox,
			SRSButton: srsButton,
			RuleState: ruleState,
		}
		guiState.RuleOutboundSelects = append(guiState.RuleOutboundSelects, ruleWidget)
		rowContent := createSelectableRuleRowContent(ruleState, guiState, checkbox, outboundRow, srsButton)
		rulesBox.Add(container.NewHBox(rowContent...))
	}

	return rulesBox
}

// createOutboundSelectorForSelectableRule создает селектор outbound для selectable rule.
func createOutboundSelectorForSelectableRule(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	ruleState *wizardmodels.RuleState,
	idx int,
	availableOutbounds []string,
	srsDownloaded bool,
) (*widget.Select, fyne.CanvasObject) {
	if !ruleState.Rule.HasOutbound {
		return nil, nil
	}

	wizardmodels.EnsureDefaultOutbound(ruleState, availableOutbounds)
	outboundSelect := widget.NewSelect(availableOutbounds, func(value string) {
		// Ignore callback during programmatic update
		if guiState.UpdatingOutboundOptions {
			return
		}
		model.SelectableRuleStates[idx].SelectedOutbound = value
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	})
	outboundSelect.SetSelected(ruleState.SelectedOutbound)
	if !ruleState.Enabled {
		outboundSelect.Disable()
	}
	if !srsDownloaded {
		outboundSelect.Disable()
	}

	outboundRow := container.NewHBox(
		widget.NewLabel("Outbound:"),
		outboundSelect,
	)

	return outboundSelect, outboundRow
}

// createSelectableRuleCheckbox создает checkbox для selectable rule.
// When user checks the box and SRS is not downloaded, we start download and set enableRuleOnSRSSuccess
// so that on success the rule is enabled (checkbox stays checked).
func createSelectableRuleCheckbox(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	ruleState *wizardmodels.RuleState,
	idx int,
	outboundSelect *widget.Select,
	srsButtonRef **ttwidget.Button,
	enableRuleOnSRSSuccess *bool,
) *widget.Check {
	var checkbox *widget.Check
	checkbox = widget.NewCheck(ruleState.Rule.Label, func(val bool) {
		if val && !services.AllSRSDownloaded(model.ExecDir, ruleState.Rule.RuleSets) {
			if !guiState.UpdatingOutboundOptions && srsButtonRef != nil && *srsButtonRef != nil {
				*enableRuleOnSRSSuccess = true // on 🔄→✔️ success we will set the checkbox
				(*srsButtonRef).OnTapped()
			}
			checkbox.SetChecked(false)
			return
		}
		// Always update model and UI state to keep them in sync
		model.SelectableRuleStates[idx].Enabled = val
		model.TemplatePreviewNeedsUpdate = true

		if outboundSelect != nil {
			if val {
				outboundSelect.Enable()
			} else {
				outboundSelect.Disable()
			}
		}

		// Only mark as changed if not during programmatic update
		if !guiState.UpdatingOutboundOptions {
			presenter.MarkAsChanged()
		}
	})
	checkbox.SetChecked(ruleState.Enabled)
	checkbox.Refresh() // Обновляем визуальное состояние чекбокса
	return checkbox
}

// createSRSButton создает кнопку ⬇/🔄/✔️ для скачивания SRS (selectable rules).
func createSRSButton(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	ruleState *wizardmodels.RuleState,
	idx int,
	srsEntries []services.SRSEntry,
	checkbox *widget.Check,
	outboundSelect *widget.Select,
	enableRuleOnSRSSuccess *bool,
) *ttwidget.Button {
	initialText := srsBtnDownload
	if services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
		initialText = srsBtnDone
	}
	btn := ttwidget.NewButton(initialText, nil)
	btn.Importance = widget.LowImportance
	if t := srsEntriesTooltip(srsEntries); t != "" {
		btn.SetToolTip(t)
	}
	btn.OnTapped = func() {
		runSRSDownloadAsync(presenter, model, guiState, srsEntries, btn, outboundSelect, func() {
			if enableRuleOnSRSSuccess != nil && *enableRuleOnSRSSuccess {
				*enableRuleOnSRSSuccess = false
				guiState.UpdatingOutboundOptions = true
				model.SelectableRuleStates[idx].Enabled = true
				checkbox.SetChecked(true)
				guiState.UpdatingOutboundOptions = false
			} else if ruleState.Rule.IsDefault && !ruleState.Enabled {
				guiState.UpdatingOutboundOptions = true
				model.SelectableRuleStates[idx].Enabled = true
				checkbox.SetChecked(true)
				guiState.UpdatingOutboundOptions = false
			}
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		})
	}
	return btn
}

// createSelectableRuleRowContent создает содержимое строки для selectable rule.
func createSelectableRuleRowContent(
	ruleState *wizardmodels.RuleState,
	guiState *wizardpresentation.GUIState,
	checkbox *widget.Check,
	outboundRow fyne.CanvasObject,
	srsButton *ttwidget.Button,
) []fyne.CanvasObject {
	checkboxContainer := container.NewHBox(checkbox)
	if ruleState.Rule.Description != "" {
		infoButton := widget.NewButton("?", func() {
			dialog.ShowInformation(ruleState.Rule.Label, ruleState.Rule.Description, guiState.Window)
		})
		infoButton.Importance = widget.LowImportance
		checkboxContainer.Add(infoButton)
	}
	if srsButton != nil {
		checkboxContainer.Add(srsButton)
	}

	rowContent := []fyne.CanvasObject{checkboxContainer, layout.NewSpacer()}
	if outboundRow != nil {
		rowContent = append(rowContent, outboundRow)
	}

	return rowContent
}

// createCustomRulesUI создает UI для пользовательских правил.
func createCustomRulesUI(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	availableOutbounds []string,
	showAddRuleDialog ShowAddRuleDialogFunc,
	rulesBox *fyne.Container,
) {
	for i := range model.CustomRules {
		customRule := model.CustomRules[i]
		idx := i
		isSRSRule := wizardmodels.DetermineRuleType(customRule.Rule.Rule) == wizardmodels.RuleTypeSRS && len(customRule.Rule.RuleSets) > 0
		var srsEntries []services.SRSEntry
		if isSRSRule {
			srsEntries = services.GetSRSEntries(customRule.Rule.RuleSets)
		}

		outboundSelect := createOutboundSelectorForCustomRule(
			presenter, model, guiState, customRule, idx, availableOutbounds,
		)
		if isSRSRule && len(srsEntries) > 0 && !services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) && outboundSelect != nil {
			outboundSelect.Disable()
		}

		var srsButton *ttwidget.Button
		enableRuleOnSRSSuccess := new(bool)
		checkbox := createCustomRuleCheckbox(presenter, model, guiState, customRule, idx, outboundSelect, &srsButton, enableRuleOnSRSSuccess)
		if isSRSRule && len(srsEntries) > 0 {
			srsButton = createCustomRuleSRSButton(presenter, model, guiState, customRule, idx, srsEntries, checkbox, outboundSelect, enableRuleOnSRSSuccess)
		}

		// Create action buttons
		editButton, deleteButton := createCustomRuleActionButtons(
			presenter, model, guiState, customRule, idx, showAddRuleDialog,
		)

		// Create RuleWidget for custom rule
		customRuleWidget := &wizardpresentation.RuleWidget{
			Select:    outboundSelect,
			Checkbox:  checkbox,
			SRSButton: srsButton,
			RuleState: customRule,
		}
		guiState.RuleOutboundSelects = append(guiState.RuleOutboundSelects, customRuleWidget)

		// Create row content
		rowContent := createCustomRuleRowContent(checkbox, srsButton, editButton, deleteButton, outboundSelect)
		rulesBox.Add(container.NewHBox(rowContent...))
	}
}

// createOutboundSelectorForCustomRule создает селектор outbound для custom rule.
func createOutboundSelectorForCustomRule(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	availableOutbounds []string,
) *widget.Select {
	wizardmodels.EnsureDefaultOutbound(customRule, availableOutbounds)

	outboundSelect := widget.NewSelect(availableOutbounds, func(value string) {
		if guiState.UpdatingOutboundOptions {
			return
		}
		model.CustomRules[idx].SelectedOutbound = value
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	})
	outboundSelect.SetSelected(customRule.SelectedOutbound)
	if !customRule.Enabled {
		outboundSelect.Disable()
	}

	return outboundSelect
}

// createCustomRuleActionButtons создает кнопки редактирования и удаления для custom rule.
func createCustomRuleActionButtons(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	showAddRuleDialog ShowAddRuleDialogFunc,
) (*widget.Button, *widget.Button) {
	// Edit button
	editButton := widget.NewButton("✏️", func() {
		showAddRuleDialog(presenter, customRule, idx)
	})
	editButton.Importance = widget.LowImportance

	// Delete button
	deleteButton := widget.NewButton("❌", func() {
		deleteCustomRule(presenter, model, guiState, customRule, showAddRuleDialog)
	})
	deleteButton.Importance = widget.LowImportance

	return editButton, deleteButton
}

// deleteCustomRule удаляет пользовательское правило.
func deleteCustomRule(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	showAddRuleDialog ShowAddRuleDialogFunc,
) {
	// Find and remove rule from model
	for i, rule := range model.CustomRules {
		if rule == customRule {
			model.CustomRules = append(model.CustomRules[:i], model.CustomRules[i+1:]...)
			break
		}
	}

	// Remove from GUIState
	newRuleWidgets := make([]*wizardpresentation.RuleWidget, 0, len(guiState.RuleOutboundSelects)-1)
	for _, rw := range guiState.RuleOutboundSelects {
		if r, ok := rw.RuleState.(*wizardmodels.RuleState); ok && r != customRule {
			newRuleWidgets = append(newRuleWidgets, rw)
		}
	}
	guiState.RuleOutboundSelects = newRuleWidgets

	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()

	// Recreate tab content
	refreshWrapper := func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject {
		return CreateRulesTab(p, showAddRuleDialog)
	}
	presenter.RefreshRulesTab(refreshWrapper)
}

// createCustomRuleCheckbox создает checkbox для custom rule.
func createCustomRuleCheckbox(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	outboundSelect *widget.Select,
	srsButtonRef **ttwidget.Button,
	enableRuleOnSRSSuccess *bool,
) *widget.Check {
	checkbox := widget.NewCheck(customRule.Rule.Label, nil)
	checkbox.OnChanged = func(val bool) {
		// Для SRS-правил при включении и отсутствии локальных SRS запускаем скачивание (кнопка SRS создаётся всегда при isSRSRule и ненулевых entries).
		if val &&
			wizardmodels.DetermineRuleType(customRule.Rule.Rule) == wizardmodels.RuleTypeSRS &&
			len(customRule.Rule.RuleSets) > 0 {
			entries := services.GetSRSEntries(customRule.Rule.RuleSets)
			if len(entries) > 0 && !services.AllSRSDownloadedForEntries(model.ExecDir, entries) {
				// Важно: при инициализации UI (SetChecked ниже) кнопка SRS ещё может быть nil,
				// поэтому любые вызовы OnTapped должны быть защищены, иначе визард падает,
				// если пользователь удалил файлы из bin/rule-sets вручную.
				if !guiState.UpdatingOutboundOptions && srsButtonRef != nil && *srsButtonRef != nil {
					*enableRuleOnSRSSuccess = true
					(*srsButtonRef).OnTapped()
				}
				checkbox.SetChecked(false)
				return
			}
		}

		// Always update model and UI state to keep them in sync
		model.CustomRules[idx].Enabled = val
		model.TemplatePreviewNeedsUpdate = true

		if outboundSelect != nil {
			if val {
				outboundSelect.Enable()
			} else {
				outboundSelect.Disable()
			}
		}

		// Only mark as changed if not during programmatic update
		if !guiState.UpdatingOutboundOptions {
			presenter.MarkAsChanged()
		}
	}
	checkbox.SetChecked(customRule.Enabled)
	return checkbox
}

// createCustomRuleRowContent создает содержимое строки для custom rule.
func createCustomRuleRowContent(
	checkbox *widget.Check,
	srsButton *ttwidget.Button,
	editButton *widget.Button,
	deleteButton *widget.Button,
	outboundSelect *widget.Select,
) []fyne.CanvasObject {
	// Блок с чекбоксом и, опционально, кнопкой SRS.
	row := []fyne.CanvasObject{checkbox}

	if srsButton != nil {
		row = append(row, srsButton)
	}

	row = append(row,
		editButton,
		deleteButton,
		layout.NewSpacer(),
	)

	if outboundSelect != nil {
		row = append(row, container.NewHBox(
			widget.NewLabel("Outbound:"),
			outboundSelect,
		))
	}

	return row
}

// createCustomRuleSRSButton создает кнопку ⬇/🔄/✔️ для пользовательского SRS-правила.
func createCustomRuleSRSButton(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	_ *wizardmodels.RuleState,
	idx int,
	srsEntries []services.SRSEntry,
	checkbox *widget.Check,
	outboundSelect *widget.Select,
	enableRuleOnSRSSuccess *bool,
) *ttwidget.Button {
	initialText := srsBtnDownload
	if services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
		initialText = srsBtnDone
	}
	btn := ttwidget.NewButton(initialText, nil)
	btn.Importance = widget.LowImportance
	if t := srsEntriesTooltip(srsEntries); t != "" {
		btn.SetToolTip(t)
	}
	btn.OnTapped = func() {
		runSRSDownloadAsync(presenter, model, guiState, srsEntries, btn, outboundSelect, func() {
			if enableRuleOnSRSSuccess != nil && *enableRuleOnSRSSuccess {
				*enableRuleOnSRSSuccess = false
				guiState.UpdatingOutboundOptions = true
				model.CustomRules[idx].Enabled = true
				checkbox.SetChecked(true)
				guiState.UpdatingOutboundOptions = false
			}
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		})
	}
	return btn
}

// createAddRuleButton создает кнопку добавления правила.
func createAddRuleButton(
	presenter *wizardpresentation.WizardPresenter,
	showAddRuleDialog ShowAddRuleDialogFunc,
	rulesBox *fyne.Container,
) {
	addRuleButton := widget.NewButton("➕ Add Rule", func() {
		showAddRuleDialog(presenter, nil, -1)
	})
	addRuleButton.Importance = widget.LowImportance
	rulesBox.Add(addRuleButton)
}

// createFinalOutboundSelect создает селектор финального outbound.
func createFinalOutboundSelect(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	availableOutbounds []string,
) *widget.Select {
	// Set flag BEFORE creating finalSelect to prevent callback from firing during initialization
	guiState.UpdatingOutboundOptions = true
	debuglog.DebugLog("rules_tab: UpdatingOutboundOptions set to true before creating finalSelect")

	wizardbusiness.EnsureFinalSelected(model, availableOutbounds)
	finalSelect := widget.NewSelect(availableOutbounds, func(value string) {
		// Ignore callback during programmatic update
		if guiState.UpdatingOutboundOptions {
			return
		}
		model.SelectedFinalOutbound = value
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	})
	finalSelect.SetSelected(model.SelectedFinalOutbound)
	guiState.FinalOutboundSelect = finalSelect

	return finalSelect
}

// buildRulesTabContainer создает финальный контейнер таба правил.
func buildRulesTabContainer(presenter *wizardpresentation.WizardPresenter, rulesScroll fyne.CanvasObject, finalSelect *widget.Select) fyne.CanvasObject {
	model := presenter.Model()
	row := container.NewHBox(
		widget.NewLabel("Final outbound:"),
		finalSelect,
		layout.NewSpacer(),
	)
	if runtime.GOOS == "darwin" {
		tunCheck := widget.NewCheck("TUN", func(checked bool) {
			model.EnableTunForMacOS = checked
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		})
		tunCheck.SetChecked(model.EnableTunForMacOS)
		helpBtn := widget.NewButton("?", func() {
			dialog.ShowInformation("TUN", "Enabling TUN will require entering your password when starting or stopping the VPN.", presenter.GUIState().Window)
		})
		row.Add(tunCheck)
		row.Add(helpBtn)
	}
	return container.NewVBox(
		widget.NewLabel("Selectable rules"),
		rulesScroll,
		widget.NewSeparator(),
		row,
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
