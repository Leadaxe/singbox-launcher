// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл rules_tab.go — вкладка Rules: единый список CustomRules (027), библиотека пресетов шаблона в library_rules_dialog.
//   - Над скроллом: Add Rule, Add from library и подпись столбца Outbound в одной строке (Border); в скролле — строки без «Outbound:» на каждой строке
//   - Final outbound; TUN для macOS — вкладка Settings (vars.tun)
//
// RuleWidget в GUIState связывает виджеты с *RuleState при пересоздании вкладки.
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
	"image/color"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowAddRuleDialogFunc is a function type for showing the add rule dialog.
type ShowAddRuleDialogFunc func(p *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int)

const (
	srsGroupDownloadTimeout = 90 * time.Second

	// rulesOutboundColumnRightGutter — отступ справа только у подписи «Outbound:» в шапке над скроллом.
	rulesOutboundColumnRightGutter float32 = 40
)

func srsBtnDownload() string { return locale.T("wizard.rules.button_srs_download") }
func srsBtnLoading() string  { return locale.T("wizard.rules.button_srs_loading") }
func srsBtnDone() string     { return locale.T("wizard.rules.button_srs_done") }

// srsEntriesTooltip возвращает строку URL для tooltip кнопки SRS.
// customRuleSRSEntries возвращает записи SRS для строки правила, если это пресет с rule_sets; иначе ok=false.
func customRuleSRSEntries(customRule *wizardmodels.RuleState) (entries []services.SRSEntry, ok bool) {
	if customRule == nil {
		return nil, false
	}
	if wizardmodels.DetermineRuleType(customRule.Rule.Rule) != wizardmodels.RuleTypeSRS || len(customRule.Rule.RuleSets) == 0 {
		return nil, false
	}
	return services.GetSRSEntries(customRule.Rule.RuleSets), true
}

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
	btn.SetText(srsBtnLoading())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), srsGroupDownloadTimeout)
		defer cancel()
		err := services.DownloadSRSGroup(ctx, model.ExecDir, srsEntries)
		presenter.UpdateUI(func() {
			btn.Enable()
			if err != nil {
				btn.SetText(srsBtnDownload())
				ruleSetsDir := filepath.Join(model.ExecDir, constants.BinDirName, constants.RuleSetsDirName)
				downloadURL := ""
				if len(srsEntries) > 0 {
					downloadURL = srsEntries[0].URL
				}
				debuglog.WarnLog("rules_tab: SRS download failed: %v", err)
				dialogs.ShowDownloadFailedManual(guiState.Window, locale.T("wizard.rules.error_srs_failed"), downloadURL, ruleSetsDir)
				return
			}
			btn.SetText(srsBtnDone())
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

	initializeRulesTabState(presenter, model, guiState)
	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(model))

	rulesBox := container.NewVBox()
	if len(model.CustomRules) == 0 {
		rulesBox.Add(createRulesEmptyState())
	} else {
		buildCustomRuleRows(presenter, model, guiState, availableOutbounds, showAddRuleDialog, rulesBox)
	}

	finalSelect := createFinalOutboundSelect(presenter, model, guiState, availableOutbounds)
	headerRow := createRulesToolbarOutboundHeaderRow(presenter, showAddRuleDialog)
	rulesScroll := CreateRulesScroll(guiState, rulesBox)

	// RefreshOutboundOptions will reset UpdatingOutboundOptions flag and hasChanges after all SetSelected() calls
	presenter.RefreshOutboundOptions()

	// Build final container
	return buildRulesTabContainer(headerRow, rulesScroll, finalSelect)
}

// createTemplateNotFoundMessage создает сообщение об отсутствии шаблона.
func createTemplateNotFoundMessage() fyne.CanvasObject {
	templateFileName := wizardtemplate.GetTemplateFileName()
	return container.NewVBox(
		widget.NewLabel(locale.Tf("wizard.rules.template_not_found", templateFileName)),
		widget.NewLabel(locale.T("wizard.rules.template_create_hint")),
	)
}

// initializeRulesTabState инициализирует состояние таба правил.
func initializeRulesTabState(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, guiState *wizardpresentation.GUIState) {
	presenter.InitializeTemplateState()

	// Очищаем старые виджеты перед созданием новых (важно при пересоздании вкладки)
	guiState.RuleOutboundSelects = make([]*wizardpresentation.RuleWidget, 0)

	guiState.UpdatingOutboundOptions = true

	if model.CustomRules == nil {
		model.CustomRules = make([]*wizardmodels.RuleState, 0)
	}
}

// refreshRulesTabFromPresenter пересоздаёт вкладку Rules (общий путь после правок списка).
func refreshRulesTabFromPresenter(presenter *wizardpresentation.WizardPresenter, showAddRuleDialog ShowAddRuleDialogFunc) {
	presenter.RefreshRulesTab(func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject {
		return CreateRulesTab(p, showAddRuleDialog)
	})
}

func rulesToolbarButtons(
	p *wizardpresentation.WizardPresenter,
	showAddRuleDialog ShowAddRuleDialogFunc,
) fyne.CanvasObject {
	addRule := widget.NewButton(locale.T("wizard.rules.button_add_rule"), func() {
		showAddRuleDialog(p, nil, -1)
	})
	addRule.Importance = widget.LowImportance
	addLib := widget.NewButton(locale.T("wizard.rules.button_add_from_library"), func() {
		ShowRulesLibraryDialog(p, showAddRuleDialog)
	})
	addLib.Importance = widget.LowImportance
	setTooltip(addLib, locale.T("wizard.rules.tooltip_add_from_library"))
	return container.NewHBox(addRule, addLib)
}

func rulesOutboundRightEdgeGutter() fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(rulesOutboundColumnRightGutter, 1))
	return r
}

// rulesRowEditDeleteLeadWidth — ширина блока Edit+Delete в строке правила (для выравнивания заголовка «Outbound:» над селектами).
func rulesRowEditDeleteLeadWidth() float32 {
	e := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {})
	d := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {})
	e.Importance = widget.LowImportance
	d.Importance = widget.LowImportance
	return container.NewHBox(e, d).MinSize().Width
}

// createRulesToolbarOutboundHeaderRow — одна строка над скроллом: кнопки слева, подпись столбца Outbound справа над селектами.
func createRulesToolbarOutboundHeaderRow(
	p *wizardpresentation.WizardPresenter,
	showAddRuleDialog ShowAddRuleDialogFunc,
) fyne.CanvasObject {
	lead := canvas.NewRectangle(color.Transparent)
	lead.SetMinSize(fyne.NewSize(rulesRowEditDeleteLeadWidth(), 1))
	outLbl := widget.NewLabel(locale.T("wizard.rules.label_outbound"))
	right := container.NewHBox(lead, outLbl, rulesOutboundRightEdgeGutter())
	return container.NewBorder(nil, nil, rulesToolbarButtons(p, showAddRuleDialog), right, layout.NewSpacer())
}

func createRulesEmptyState() fyne.CanvasObject {
	msg := widget.NewLabel(locale.T("wizard.rules.empty_state"))
	msg.Wrapping = fyne.TextWrapWord
	return msg
}

// buildCustomRuleRows строит строки правил: чекбокс, ↑↓, подпись (центр Border), SRS, справа Edit, Del, Select (подпись Outbound — в шапке над скроллом).
func buildCustomRuleRows(
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
		srsEntries, isSRSRule := customRuleSRSEntries(customRule)

		var row *fynewidget.HoverRow
		rowGetter := func() *fynewidget.HoverRow { return row }

		outboundWidget := createOutboundSelectorForCustomRule(
			presenter, model, guiState, customRule, idx, availableOutbounds, rowGetter,
		)
		outboundSelect := &outboundWidget.Select
		if isSRSRule && len(srsEntries) > 0 && !services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
			outboundSelect.Disable()
		}

		var srsButton *ttwidget.Button
		enableRuleOnSRSSuccess := new(bool)
		checkbox := createRuleEnableCheckbox(presenter, model, guiState, customRule, idx, outboundSelect, &srsButton, enableRuleOnSRSSuccess)
		label := ttwidget.NewLabel(customRule.Rule.Label)
		label.Wrapping = fyne.TextWrapOff
		label.Truncation = fyne.TextTruncateEllipsis
		if d := strings.TrimSpace(customRule.Rule.Description); d != "" {
			label.SetToolTip(d)
		}
		var srsHF *fynewidget.HoverForwardTTButton
		if isSRSRule && len(srsEntries) > 0 {
			srsBtn := createCustomRuleSRSButton(presenter, model, guiState, customRule, idx, srsEntries, checkbox, outboundSelect, enableRuleOnSRSSuccess, rowGetter)
			srsButton = srsBtn.TTWidget()
			srsHF = srsBtn
		}

		moveUpButton, moveDownButton, editButton, deleteButton := createCustomRuleActionButtons(
			presenter, model, guiState, customRule, idx, showAddRuleDialog, rowGetter,
		)

		customRuleWidget := &wizardpresentation.RuleWidget{
			Select:    outboundSelect,
			Checkbox:  checkbox,
			SRSButton: srsButton,
			RuleState: customRule,
		}
		guiState.RuleOutboundSelects = append(guiState.RuleOutboundSelects, customRuleWidget)

		// Border: центр под подпись (HBox(left, Spacer, …) давал left только MinSize → «…» у label).
		leftLead := container.NewHBox(checkbox, moveUpButton, moveDownButton)
		rightCluster := container.NewHBox(editButton, deleteButton, outboundWidget)

		labelTap := fynewidget.NewTapWrap(label, func() {
			if checkbox.Disabled() {
				return
			}
			checkbox.SetChecked(!checkbox.Checked)
		})
		var center fyne.CanvasObject = labelTap
		if srsHF != nil {
			center = container.NewBorder(nil, nil, nil, srsHF, labelTap)
		}
		rowInner := container.NewBorder(nil, nil, leftLead, rightCluster, center)
		row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
		row.WireTooltipLabelHover(label)
		rulesBox.Add(row)
	}
}

// createRuleEnableCheckbox — чекбокс вкл/выкл; подпись правила обёрнута в TapWrap и тоже переключает состояние.
func createRuleEnableCheckbox(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	outboundSelect *widget.Select,
	srsButtonRef **ttwidget.Button,
	enableRuleOnSRSSuccess *bool,
) *widget.Check {
	var ch *widget.Check
	ch = widget.NewCheck("", func(val bool) {
		if val {
			entries, isSRS := customRuleSRSEntries(customRule)
			if isSRS && len(entries) > 0 && !services.AllSRSDownloadedForEntries(model.ExecDir, entries) {
				if !guiState.UpdatingOutboundOptions && *srsButtonRef != nil {
					*enableRuleOnSRSSuccess = true
					(*srsButtonRef).OnTapped()
				}
				ch.SetChecked(false)
				return
			}
		}

		model.CustomRules[idx].Enabled = val
		model.TemplatePreviewNeedsUpdate = true

		if val {
			outboundSelect.Enable()
		} else {
			outboundSelect.Disable()
		}

		if !guiState.UpdatingOutboundOptions {
			presenter.MarkAsChanged()
		}
	})
	ch.SetChecked(customRule.Enabled)
	setTooltip(ch, locale.T("wizard.rules.tooltip_rule_enabled"))
	return ch
}

// createOutboundSelectorForCustomRule создает селектор outbound для custom rule.
func createOutboundSelectorForCustomRule(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	availableOutbounds []string,
	rowGetter fynewidget.RowHoverGetter,
) *fynewidget.HoverForwardSelect {
	wizardmodels.EnsureDefaultOutbound(customRule, availableOutbounds)

	sel := fynewidget.NewHoverForwardSelect(availableOutbounds, func(value string) {
		if guiState.UpdatingOutboundOptions {
			return
		}
		model.CustomRules[idx].SelectedOutbound = value
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	}, rowGetter)
	sel.SetSelected(customRule.SelectedOutbound)
	if !customRule.Enabled {
		sel.Disable()
	}

	return sel
}

// createCustomRuleActionButtons создает кнопки редактирования и удаления для custom rule.
func createCustomRuleActionButtons(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	customRule *wizardmodels.RuleState,
	idx int,
	showAddRuleDialog ShowAddRuleDialogFunc,
	rowGetter fynewidget.RowHoverGetter,
) (*fynewidget.HoverForwardButton, *fynewidget.HoverForwardButton, *fynewidget.HoverForwardButton, *fynewidget.HoverForwardButton) {
	moveUpButton := fynewidget.NewHoverForwardButton("↑", func() {
		moveCustomRuleUp(presenter, model, guiState, idx, showAddRuleDialog)
	}, rowGetter)
	moveUpButton.Importance = widget.LowImportance
	if idx <= 0 {
		moveUpButton.Disable()
		setTooltip(moveUpButton, locale.T("wizard.rules.tooltip_move_up_off"))
	} else {
		setTooltip(moveUpButton, locale.T("wizard.rules.tooltip_move_up"))
	}

	moveDownButton := fynewidget.NewHoverForwardButton("↓", func() {
		moveCustomRuleDown(presenter, model, guiState, idx, showAddRuleDialog)
	}, rowGetter)
	moveDownButton.Importance = widget.LowImportance
	if idx >= len(model.CustomRules)-1 {
		moveDownButton.Disable()
		setTooltip(moveDownButton, locale.T("wizard.rules.tooltip_move_down_off"))
	} else {
		setTooltip(moveDownButton, locale.T("wizard.rules.tooltip_move_down"))
	}

	// Edit — только иконка (подпись в tooltip, как у удаления)
	editButton := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		showAddRuleDialog(presenter, customRule, idx)
	}, rowGetter)
	editButton.Importance = widget.LowImportance
	setTooltip(editButton, locale.T("wizard.shared.button_edit"))

	// Delete button (standard trash icon; confirmation before removal)
	deleteButton := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
		ruleLabel := strings.TrimSpace(customRule.Rule.Label)
		if ruleLabel == "" {
			ruleLabel = locale.T("wizard.rules.dialog_delete_unnamed")
		}
		dialog.ShowConfirm(
			locale.T("wizard.dialog_confirmation"),
			locale.Tf("wizard.rules.dialog_delete_confirm", ruleLabel),
			func(ok bool) {
				if !ok {
					return
				}
				deleteCustomRule(presenter, model, guiState, customRule, showAddRuleDialog)
			},
			guiState.Window,
		)
	}, rowGetter)
	deleteButton.Importance = widget.LowImportance
	setTooltip(deleteButton, locale.T("wizard.rules.button_delete"))

	return moveUpButton, moveDownButton, editButton, deleteButton
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

	newRuleWidgets := make([]*wizardpresentation.RuleWidget, 0, len(guiState.RuleOutboundSelects)-1)
	for _, rw := range guiState.RuleOutboundSelects {
		if rw.RuleState != customRule {
			newRuleWidgets = append(newRuleWidgets, rw)
		}
	}
	guiState.RuleOutboundSelects = newRuleWidgets

	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()

	refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
}

func moveCustomRuleUp(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	idx int,
	showAddRuleDialog ShowAddRuleDialogFunc,
) {
	if idx <= 0 || idx >= len(model.CustomRules) {
		return
	}
	if guiState.RulesScroll != nil {
		guiState.RulesScrollOffset = guiState.RulesScroll.Offset
	}
	model.CustomRules[idx], model.CustomRules[idx-1] = model.CustomRules[idx-1], model.CustomRules[idx]
	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()

	refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
}

func moveCustomRuleDown(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	idx int,
	showAddRuleDialog ShowAddRuleDialogFunc,
) {
	if idx < 0 || idx >= len(model.CustomRules)-1 {
		return
	}
	if guiState.RulesScroll != nil {
		guiState.RulesScrollOffset = guiState.RulesScroll.Offset
	}
	model.CustomRules[idx], model.CustomRules[idx+1] = model.CustomRules[idx+1], model.CustomRules[idx]
	model.TemplatePreviewNeedsUpdate = true
	presenter.MarkAsChanged()

	refreshRulesTabFromPresenter(presenter, showAddRuleDialog)
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
	rowGetter fynewidget.RowHoverGetter,
) *fynewidget.HoverForwardTTButton {
	initialText := srsBtnDownload()
	if services.AllSRSDownloadedForEntries(model.ExecDir, srsEntries) {
		initialText = srsBtnDone()
	}
	btn := fynewidget.NewHoverForwardTTButton(initialText, nil, rowGetter)
	btn.Importance = widget.LowImportance
	if t := srsEntriesTooltip(srsEntries); t != "" {
		btn.SetToolTip(t)
	}
	btn.OnTapped = func() {
		runSRSDownloadAsync(presenter, model, guiState, srsEntries, btn.TTWidget(), outboundSelect, func() {
			if *enableRuleOnSRSSuccess {
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

// createFinalOutboundSelect создает селектор финального outbound.
func createFinalOutboundSelect(
	presenter *wizardpresentation.WizardPresenter,
	model *wizardmodels.WizardModel,
	guiState *wizardpresentation.GUIState,
	availableOutbounds []string,
) *widget.Select {
	guiState.UpdatingOutboundOptions = true

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
func buildRulesTabContainer(headerRow, rulesScroll fyne.CanvasObject, finalSelect *widget.Select) fyne.CanvasObject {
	row := container.NewHBox(
		widget.NewLabel(locale.T("wizard.rules.label_final_outbound")),
		finalSelect,
		layout.NewSpacer(),
	)
	return container.NewVBox(
		headerRow,
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
	scrollGutter := canvas.NewRectangle(color.Transparent)
	scrollGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	contentWithGutter := container.NewBorder(nil, nil, nil, scrollGutter, content)
	scroll := container.NewVScroll(contentWithGutter)
	scroll.SetMinSize(fyne.NewSize(0, maxHeight))
	scroll.Offset = guiState.RulesScrollOffset
	guiState.RulesScroll = scroll
	return scroll
}
