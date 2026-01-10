// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл add_rule_dialog.go содержит функцию ShowAddRuleDialog, которая создает диалоговое окно
// для добавления или редактирования пользовательского правила маршрутизации:
//   - Ввод домена, IP, порта и других критериев правила
//   - Выбор outbound для правила (включая reject/drop)
//   - Валидация введенных данных
//   - Сохранение правила в модель через presenter
//
// Диалог поддерживает два режима:
//   - Добавление нового правила (editRule == nil)
//   - Редактирование существующего правила (editRule != nil, ruleIndex указывает индекс)
//
// Диалоговые окна имеют отдельную ответственность от основных табов.
// Содержит сложную логику валидации и обработки ввода пользователя.
//
// Используется в:
//   - tabs/rules_tab.go - вызывается при нажатии кнопок "Add Rule" и "Edit" для правил
//
// Взаимодействует с:
//   - presenter - все действия пользователя обрабатываются через методы presenter
//   - models.RuleState - работает с данными правил из модели
//   - business - использует валидацию и утилиты из business пакета
package dialogs

import (
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtabs "singbox-launcher/ui/wizard/tabs"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowAddRuleDialog opens a dialog for adding or editing a custom rule.
func ShowAddRuleDialog(presenter *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int) {
	guiState := presenter.GUIState()
	model := presenter.Model()

	if guiState.Window == nil {
		return
	}

	isEdit := editRule != nil
	dialogTitle := "Add Rule"
	if isEdit {
		dialogTitle = "Edit Rule"
	}

	// Check if dialog is already open for this rule
	openDialogs := presenter.OpenRuleDialogs()
	dialogKey := ruleIndex
	if !isEdit {
		dialogKey = -1
	}
	if existingDialog, exists := openDialogs[dialogKey]; exists {
		existingDialog.Close()
		delete(openDialogs, dialogKey)
	}

	// Input field height
	inputFieldHeight := float32(90)

	// Input fields
	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder("Rule name")

	ipEntry := widget.NewMultiLineEntry()
	ipEntry.SetPlaceHolder("Enter IP addresses (CIDR format)\ne.g., 192.168.1.0/24")
	ipEntry.Wrapping = fyne.TextWrapWord

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetPlaceHolder("Enter domains or URLs (one per line)\ne.g., example.com")
	urlEntry.Wrapping = fyne.TextWrapWord

	// Limit input field height
	ipScroll := container.NewScroll(ipEntry)
	ipSizeRect := canvas.NewRectangle(color.Transparent)
	ipSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	ipContainer := container.NewMax(ipSizeRect, ipScroll)

	urlScroll := container.NewScroll(urlEntry)
	urlSizeRect := canvas.NewRectangle(color.Transparent)
	urlSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	urlContainer := container.NewMax(urlSizeRect, urlScroll)

	// Outbound selector
	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(model))
	if len(availableOutbounds) == 0 {
		availableOutbounds = []string{wizardmodels.DefaultOutboundTag, wizardmodels.RejectActionName}
	}
	outboundSelect := widget.NewSelect(availableOutbounds, func(string) {})
	if len(availableOutbounds) > 0 {
		outboundSelect.SetSelected(availableOutbounds[0])
	}

	// Create map for fast outbound lookup (O(1) instead of O(n))
	outboundMap := make(map[string]bool, len(availableOutbounds))
	for _, opt := range availableOutbounds {
		outboundMap[opt] = true
	}

	// Determine initial rule type and load data
	ruleType := RuleTypeDomain
	if isEdit {
		labelEntry.SetText(editRule.Rule.Label)
		if editRule.SelectedOutbound != "" && outboundMap[editRule.SelectedOutbound] {
			outboundSelect.SetSelected(editRule.SelectedOutbound)
		}

		// Load IP or domains
		if ipVal, hasIP := editRule.Rule.Raw["ip_cidr"]; hasIP {
			ruleType = RuleTypeIP
			if ips := ExtractStringArray(ipVal); len(ips) > 0 {
				ipEntry.SetText(strings.Join(ips, "\n"))
			}
		} else if domainVal, hasDomain := editRule.Rule.Raw["domain"]; hasDomain {
			ruleType = RuleTypeDomain
			if domains := ExtractStringArray(domainVal); len(domains) > 0 {
				urlEntry.SetText(strings.Join(domains, "\n"))
			}
		}
	}

	// Manage field visibility
	ipLabel := widget.NewLabel("IP Addresses (one per line, CIDR format):")
	urlLabel := widget.NewLabel("Domains/URLs (one per line):")
	updateVisibility := func(selectedType string) {
		isIP := selectedType == RuleTypeIP
		if isIP {
			ipLabel.Show()
			ipContainer.Show()
			urlLabel.Hide()
			urlContainer.Hide()
		} else {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Show()
			urlContainer.Show()
		}
	}

	// Save button and validation functions
	var confirmButton *widget.Button
	var saveRule func()
	var updateButtonState func()
	var ruleTypeRadio *widget.RadioGroup
	var dialogWindow fyne.Window

	validateFields := func() bool {
		if strings.TrimSpace(labelEntry.Text) == "" {
			return false
		}
		if ruleTypeRadio == nil {
			return false
		}
		selectedType := ruleTypeRadio.Selected
		if selectedType == RuleTypeIP {
			return strings.TrimSpace(ipEntry.Text) != ""
		}
		return strings.TrimSpace(urlEntry.Text) != ""
	}

	updateButtonState = func() {
		if confirmButton != nil {
			if validateFields() {
				confirmButton.Enable()
			} else {
				confirmButton.Disable()
			}
		}
	}

	// RadioGroup for rule type selection
	ruleTypeRadio = widget.NewRadioGroup([]string{RuleTypeIP, RuleTypeDomain}, func(selected string) {
		updateVisibility(selected)
		if updateButtonState != nil {
			updateButtonState()
		}
	})
	ruleTypeRadio.SetSelected(ruleType)
	updateVisibility(ruleType)

	saveRule = func() {
		label := strings.TrimSpace(labelEntry.Text)
		selectedType := ruleTypeRadio.Selected
		selectedOutbound := outboundSelect.Selected
		// Fallback: if outbound not selected (e.g., when editing old rule with non-existent outbound)
		if selectedOutbound == "" {
			selectedOutbound = availableOutbounds[0] // availableOutbounds is always non-empty (see lines 107-109)
		}

		var ruleRaw map[string]interface{}
		var items []string
		var ruleKey string

		if selectedType == RuleTypeIP {
			ipText := strings.TrimSpace(ipEntry.Text)
			items = ParseLines(ipText, false) // Trim spaces
			ruleKey = "ip_cidr"
		} else {
			urlText := strings.TrimSpace(urlEntry.Text)
			items = ParseLines(urlText, false) // Trim spaces
			ruleKey = "domain"
		}

		ruleRaw = map[string]interface{}{
			ruleKey:    items,
			"outbound": selectedOutbound,
		}

		// Save or update rule
		if isEdit {
			editRule.Rule.Label = label
			editRule.Rule.Raw = ruleRaw
			editRule.Rule.HasOutbound = true
			editRule.Rule.DefaultOutbound = selectedOutbound
			editRule.SelectedOutbound = selectedOutbound
		} else {
			newRule := &wizardmodels.RuleState{
				Rule: wizardtemplate.TemplateSelectableRule{
					Label:           label,
					Raw:             ruleRaw,
					HasOutbound:     true,
					DefaultOutbound: selectedOutbound,
					IsDefault:       true,
				},
				Enabled:          true,
				SelectedOutbound: selectedOutbound,
			}
			if model.CustomRules == nil {
				model.CustomRules = make([]*wizardmodels.RuleState, 0)
			}
			model.CustomRules = append(model.CustomRules, newRule)
		}

		// Set flag for preview recalculation
		model.TemplatePreviewNeedsUpdate = true
		// Refresh rules tab
		refreshWrapper := func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject {
			return wizardtabs.CreateRulesTab(p, ShowAddRuleDialog)
		}
		presenter.RefreshRulesTab(refreshWrapper)
		delete(openDialogs, dialogKey)
		dialogWindow.Close()
	}

	confirmBtnText := "Add"
	if isEdit {
		confirmBtnText = "Save"
	}
	confirmButton = widget.NewButton(confirmBtnText, saveRule)
	confirmButton.Importance = widget.HighImportance

	cancelButton := widget.NewButton("Cancel", func() {
		delete(openDialogs, dialogKey)
		dialogWindow.Close()
	})

	// Field change handlers for validation
	labelEntry.OnChanged = func(string) { updateButtonState() }
	ipEntry.OnChanged = func(string) { updateButtonState() }
	urlEntry.OnChanged = func(string) { updateButtonState() }

	// Content container
	inputContainer := container.NewVBox(
		widget.NewLabel("Rule Name:"),
		labelEntry,
		widget.NewSeparator(),
		widget.NewLabel("Rule Type:"),
		ruleTypeRadio,
		widget.NewSeparator(),
		ipLabel,
		ipContainer,
		urlLabel,
		urlContainer,
		widget.NewSeparator(),
		widget.NewLabel("Outbound:"),
		outboundSelect,
	)

	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		cancelButton,
		confirmButton,
	)

	mainContent := container.NewBorder(
		nil,
		buttonsContainer,
		nil,
		nil,
		container.NewScroll(inputContainer),
	)

	// Create window - get Application from presenter's controller
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	dialogWindow = controller.UIService.Application.NewWindow(dialogTitle)
	dialogWindow.Resize(fyne.NewSize(500, 600))
	dialogWindow.CenterOnScreen()
	dialogWindow.SetContent(mainContent)

	// Register dialog
	openDialogs[dialogKey] = dialogWindow

	dialogWindow.SetCloseIntercept(func() {
		delete(openDialogs, dialogKey)
		dialogWindow.Close()
	})

	updateButtonState()
	dialogWindow.Show()
}
