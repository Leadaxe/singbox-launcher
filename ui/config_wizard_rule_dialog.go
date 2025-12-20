package ui

import (
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

const (
	ruleTypeIP     = "IP Addresses (CIDR)"
	ruleTypeDomain = "Domains/URLs"
)

// extractStringArray извлекает []string из interface{} (поддерживает []interface{} и []string)
func extractStringArray(val interface{}) []string {
	if arr, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	if arr, ok := val.([]string); ok {
		return arr
	}
	return nil
}

// parseLines парсит многострочный текст, удаляя пустые строки
func parseLines(text string, preserveOriginal bool) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if preserveOriginal {
				result = append(result, line) // Сохраняем оригинал (с пробелами)
			} else {
				result = append(result, trimmed) // Сохраняем обрезанную версию
			}
		}
	}
	return result
}

// showAddRuleDialog открывает диалог для добавления или редактирования пользовательского правила
func showAddRuleDialog(state *WizardState, editRule *SelectableRuleState, ruleIndex int) {
	if state.Window == nil {
		return
	}

	isEdit := editRule != nil
	dialogTitle := "Add Rule"
	if isEdit {
		dialogTitle = "Edit Rule"
	}

	// Проверяем, не открыт ли уже диалог для этого правила
	if state.openRuleDialogs == nil {
		state.openRuleDialogs = make(map[int]fyne.Window)
	}
	dialogKey := ruleIndex
	if !isEdit {
		dialogKey = -1
	}
	if existingDialog, exists := state.openRuleDialogs[dialogKey]; exists {
		existingDialog.Close()
		delete(state.openRuleDialogs, dialogKey)
	}

	// Высота полей ввода IP и URL
	inputFieldHeight := float32(90)

	// Поля ввода
	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder("Rule name")

	ipEntry := widget.NewMultiLineEntry()
	ipEntry.SetPlaceHolder("Enter IP addresses (CIDR format)\ne.g., 192.168.1.0/24")
	ipEntry.Wrapping = fyne.TextWrapWord

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetPlaceHolder("Enter domains or URLs (one per line)\ne.g., example.com")
	urlEntry.Wrapping = fyne.TextWrapWord

	// Ограничиваем высоту полей ввода
	ipScroll := container.NewScroll(ipEntry)
	ipSizeRect := canvas.NewRectangle(color.Transparent)
	ipSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	ipContainer := container.NewMax(ipSizeRect, ipScroll)

	urlScroll := container.NewScroll(urlEntry)
	urlSizeRect := canvas.NewRectangle(color.Transparent)
	urlSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	urlContainer := container.NewMax(urlSizeRect, urlScroll)

	// Outbound selector
	availableOutbounds := state.getAvailableOutbounds()
	if len(availableOutbounds) == 0 {
		availableOutbounds = []string{defaultOutboundTag, rejectActionName}
	}
	outboundSelect := widget.NewSelect(availableOutbounds, func(string) {})
	if len(availableOutbounds) > 0 {
		outboundSelect.SetSelected(availableOutbounds[0])
	}

	// Создаем map для быстрого поиска outbound (O(1) вместо O(n))
	outboundMap := make(map[string]bool, len(availableOutbounds))
	for _, opt := range availableOutbounds {
		outboundMap[opt] = true
	}

	// Определяем начальный тип правила и загружаем данные
	ruleType := ruleTypeDomain
	if isEdit {
		labelEntry.SetText(editRule.Rule.Label)
		if editRule.SelectedOutbound != "" && outboundMap[editRule.SelectedOutbound] {
			outboundSelect.SetSelected(editRule.SelectedOutbound)
		}

		// Загружаем IP или домены
		if ipVal, hasIP := editRule.Rule.Raw["ip_cidr"]; hasIP {
			ruleType = ruleTypeIP
			if ips := extractStringArray(ipVal); len(ips) > 0 {
				ipEntry.SetText(strings.Join(ips, "\n"))
			}
		} else if domainVal, hasDomain := editRule.Rule.Raw["domain"]; hasDomain {
			ruleType = ruleTypeDomain
			if domains := extractStringArray(domainVal); len(domains) > 0 {
				urlEntry.SetText(strings.Join(domains, "\n"))
			}
		}
	}

	// Управление видимостью полей
	ipLabel := widget.NewLabel("IP Addresses (one per line, CIDR format):")
	urlLabel := widget.NewLabel("Domains/URLs (one per line):")
	updateVisibility := func(selectedType string) {
		isIP := selectedType == ruleTypeIP
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

	// Кнопка сохранения и функции валидации
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
		if selectedType == ruleTypeIP {
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

	// RadioGroup для выбора типа правила
	ruleTypeRadio = widget.NewRadioGroup([]string{ruleTypeIP, ruleTypeDomain}, func(selected string) {
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
		// Fallback: если outbound не выбран (например, при редактировании старого правила с несуществующим outbound)
		if selectedOutbound == "" {
			selectedOutbound = availableOutbounds[0] // availableOutbounds всегда не пустой (см. строки 107-109)
		}

		var ruleRaw map[string]interface{}
		var items []string
		var ruleKey string

		if selectedType == ruleTypeIP {
			ipText := strings.TrimSpace(ipEntry.Text)
			items = parseLines(ipText, false) // Обрезаем пробелы
			ruleKey = "ip_cidr"
		} else {
			urlText := strings.TrimSpace(urlEntry.Text)
			items = parseLines(urlText, false) // Обрезаем пробелы
			ruleKey = "domain"
		}

		ruleRaw = map[string]interface{}{
			ruleKey:    items,
			"outbound": selectedOutbound,
		}

		// Сохраняем или обновляем правило
		if isEdit {
			editRule.Rule.Label = label
			editRule.Rule.Raw = ruleRaw
			editRule.Rule.HasOutbound = true
			editRule.Rule.DefaultOutbound = selectedOutbound
			editRule.SelectedOutbound = selectedOutbound
		} else {
			newRule := &SelectableRuleState{
				Rule: TemplateSelectableRule{
					Label:           label,
					Raw:             ruleRaw,
					HasOutbound:     true,
					DefaultOutbound: selectedOutbound,
					IsDefault:       true,
				},
				Enabled:          true,
				SelectedOutbound: selectedOutbound,
			}
			if state.CustomRules == nil {
				state.CustomRules = make([]*SelectableRuleState, 0)
			}
			state.CustomRules = append(state.CustomRules, newRule)
		}

		// Устанавливаем флаг для пересчета превью
		state.templatePreviewNeedsUpdate = true
		state.refreshRulesTab()
		delete(state.openRuleDialogs, dialogKey)
		dialogWindow.Close()
	}

	confirmBtnText := "Add"
	if isEdit {
		confirmBtnText = "Save"
	}
	confirmButton = widget.NewButton(confirmBtnText, saveRule)
	confirmButton.Importance = widget.HighImportance

	cancelButton := widget.NewButton("Cancel", func() {
		delete(state.openRuleDialogs, dialogKey)
		dialogWindow.Close()
	})

	// Обработчики изменений полей для валидации
	labelEntry.OnChanged = func(string) { updateButtonState() }
	ipEntry.OnChanged = func(string) { updateButtonState() }
	urlEntry.OnChanged = func(string) { updateButtonState() }

	// Контейнер с содержимым
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

	// Создаем окно
	dialogWindow = state.Controller.Application.NewWindow(dialogTitle)
	dialogWindow.Resize(fyne.NewSize(500, 600))
	dialogWindow.CenterOnScreen()
	dialogWindow.SetContent(mainContent)

	// Регистрируем диалог
	state.openRuleDialogs[dialogKey] = dialogWindow

	dialogWindow.SetCloseIntercept(func() {
		delete(state.openRuleDialogs, dialogKey)
		dialogWindow.Close()
	})

	updateButtonState()
	dialogWindow.Show()
}

// refreshRulesTab обновляет вкладку с правилами
func (state *WizardState) refreshRulesTab() {
	if state.tabs == nil {
		return
	}

	for _, tab := range state.tabs.Items {
		if tab.Text == "Rules" {
			newContent := createTemplateTab(state)
			tab.Content = newContent
			state.tabs.Refresh()
			break
		}
	}
}
