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
	"sort"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/process"

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

	// Processes selector (selected items and popup)
	processesSelected := make([]string, 0)
	processesContainer := container.NewVBox()
	processesScroll := container.NewVScroll(processesContainer)
	// Make processes field display ~4 lines high
	processesSizeRect := canvas.NewRectangle(color.Transparent)
	processesSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	processesContainerWrap := container.NewMax(processesSizeRect, processesScroll)
	processesLabel := widget.NewLabel("Processes (select one or more via popup):")
	selectProcessesButton := widget.NewButton("Select Processes...", func() {})

	// Helper to normalize process name (strip legacy "PID: name" format)
	normalizeProcName := func(s string) string {
		parts := strings.SplitN(strings.TrimSpace(s), ": ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
		return strings.TrimSpace(s)
	}

	// Sort helper for process strings (by name)
	sortProcessStrings := func(items []string) {
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i]) < strings.ToLower(items[j])
		})
	}

	// Dedupe helper for process names (case-insensitive)
	dedupeProcessStrings := func(items []string) []string {
		seen := make(map[string]struct{}, len(items))
		out := make([]string, 0, len(items))
		for _, item := range items {
			n := normalizeProcName(item)
			key := strings.ToLower(n)
			if n == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, n)
		}
		return out
	}

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

		// Load IP, domains or processes
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
		} else if procVal, hasProc := editRule.Rule.Raw["process"]; hasProc {
			ruleType = RuleTypeProcess
			if procs := ExtractStringArray(procVal); len(procs) > 0 {
				processesSelected = dedupeProcessStrings(procs)
				sortProcessStrings(processesSelected)
			}
		}
	}

	// Manage field visibility
	ipLabel := widget.NewLabel("IP Addresses (one per line, CIDR format):")
	urlLabel := widget.NewLabel("Domains/URLs (one per line):")
	updateVisibility := func(selectedType string) {
		isIP := selectedType == RuleTypeIP
		isProcess := selectedType == RuleTypeProcess
		if isIP {
			ipLabel.Show()
			ipContainer.Show()
			urlLabel.Hide()
			urlContainer.Hide()
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
		} else if isProcess {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Hide()
			urlContainer.Hide()
			processesLabel.Show()
			processesContainerWrap.Show()
			selectProcessesButton.Show()
		} else {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Show()
			urlContainer.Show()
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
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
		if selectedType == RuleTypeProcess {
			return len(processesSelected) > 0
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
	ruleTypeRadio = widget.NewRadioGroup([]string{RuleTypeIP, RuleTypeDomain, RuleTypeProcess}, func(selected string) {
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
		} else if selectedType == RuleTypeProcess {
			// processesSelected already contains process names; store as-is
			items = make([]string, len(processesSelected))
			copy(items, processesSelected)
			ruleKey = "process"
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

	// Helper to refresh selected processes UI (sorted by name)
	var refreshSelectedProcessesUI func()
	refreshSelectedProcessesUI = func() {
		processesSelected = dedupeProcessStrings(processesSelected)
		// sort selected items by process name
		sortProcessStrings(processesSelected)
		processesContainer.Objects = nil
		for i := range processesSelected {
			idx := i
			p := processesSelected[i]
			lbl := widget.NewLabel(p)
			removeBtn := widget.NewButton("−", func() {
				// remove item at idx
				processesSelected = append(processesSelected[:idx], processesSelected[idx+1:]...)
				refreshSelectedProcessesUI()
				updateButtonState()
			})
			processesContainer.Add(container.NewHBox(lbl, layout.NewSpacer(), removeBtn))
		}
		processesContainer.Refresh()
	}

	// Open process selector popup
	openProcessSelector := func() {
		controller := presenter.Controller()
		if controller == nil || controller.UIService == nil {
			return
		}
		w := controller.UIService.Application.NewWindow("Select Processes")
		w.Resize(fyne.NewSize(500, 400))

		// Load process list using process package (names only, deduped)
		getProcesses := func() []string {
			procs, err := process.GetProcesses()
			if err != nil {
				return []string{}
			}
			items := make([]string, 0, len(procs))
			for _, p := range procs {
				items = append(items, p.Name)
			}
			items = dedupeProcessStrings(items)
			sortProcessStrings(items)
			return items
		}

		listData := getProcesses()
		selectedIdx := -1
		procList := widget.NewList(
			func() int { return len(listData) },
			func() fyne.CanvasObject { return container.NewHBox(widget.NewLabel(""), layout.NewSpacer()) },
			func(i widget.ListItemID, o fyne.CanvasObject) {
				lbl := o.(*fyne.Container).Objects[0].(*widget.Label)
				lbl.SetText(listData[i])
			},
		)
		procList.OnSelected = func(id widget.ListItemID) {
			selectedIdx = id
		}

		addBtn := widget.NewButton("+ Add", func() {
			if selectedIdx >= 0 && selectedIdx < len(listData) {
				item := normalizeProcName(listData[selectedIdx])
				// avoid duplicates (case-insensitive)
				found := false
				for _, s := range processesSelected {
					if strings.EqualFold(s, item) {
						found = true
						break
					}
				}
				if !found {
					processesSelected = append(processesSelected, item)
					refreshSelectedProcessesUI()
					updateButtonState()
				}
			}
		})

		refreshBtn := widget.NewButton("Refresh", func() {
			listData = getProcesses()
			procList.Refresh()
		})

		closeBtn := widget.NewButton("Close", func() { w.Close() })

		content := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), refreshBtn, addBtn, closeBtn), nil, nil, container.NewScroll(procList))
		w.SetContent(content)
		w.Show()
	}

	// wire selector button
	selectProcessesButton.OnTapped = func() { openProcessSelector() }

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
		processesLabel,
		processesContainerWrap,
		selectProcessesButton,
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

	// Refresh selected processes UI in case we loaded existing values
	refreshSelectedProcessesUI()
	updateButtonState()
	dialogWindow.Show()
}
