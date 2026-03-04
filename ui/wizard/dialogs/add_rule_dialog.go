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
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/process"

	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// CreateRulesTabFunc is a function type for creating the rules tab.
// This is used to avoid circular import between dialogs and tabs packages.
type CreateRulesTabFunc func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject

// ShowAddRuleDialog opens a dialog for adding or editing a custom rule.
// createRulesTab is a function that creates the rules tab content (used for RefreshRulesTab).
// This parameter is required to avoid circular import between dialogs and tabs packages.
func ShowAddRuleDialog(presenter *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int, createRulesTab CreateRulesTabFunc) {
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

	// Ensure only one rule dialog is open at a time
	openDialogs := presenter.OpenRuleDialogs()
	for key, existingDialog := range openDialogs {
		existingDialog.Close()
		delete(openDialogs, key)
	}
	presenter.UpdateChildOverlay() // Hide overlay immediately when all rule dialogs are closed
	// Use presenter's unified overlay update (rule dialogs, View, Outbound Edit)
	updateChildOverlay := func() { presenter.UpdateChildOverlay() }
	dialogKey := ruleIndex
	if !isEdit {
		dialogKey = -1
	}
	updateChildOverlay()

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

	// Custom JSON field (initialised early so it can be loaded when editing)
	customEntry := widget.NewMultiLineEntry()
	customEntry.SetPlaceHolder("Custom JSON (e.g., {})")
	customEntry.SetText("{}")
	customScroll := container.NewScroll(customEntry)
	customSizeRect := canvas.NewRectangle(color.Transparent)
	customSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	customContainer := container.NewMax(customSizeRect, customScroll)
	// Label for custom field (use variable so we can show/hide it with the field)
	customLabel := widget.NewLabel("Custom JSON:")

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
	domainRegexInitial := ""
	domainRegexInitialSet := false
	ruleType := RuleTypeDomain
	if isEdit {
		labelEntry.SetText(editRule.Rule.Label)
		if editRule.SelectedOutbound != "" && outboundMap[editRule.SelectedOutbound] {
			outboundSelect.SetSelected(editRule.SelectedOutbound)
		}

		// Load IP, domain (list/regex), process, or custom JSON
		ruleData := editRule.Rule.Rule
		hasIP := false
		hasDomain := false
		hasDomainRegex := false
		hasProc := false
		if ruleData != nil {
			if ipVal, ok := ruleData["ip_cidr"]; ok {
				hasIP = true
				ruleType = RuleTypeIP
				if ips := ExtractStringArray(ipVal); len(ips) > 0 {
					ipEntry.SetText(strings.Join(ips, "\n"))
				}
			} else if drVal, ok := ruleData["domain_regex"]; ok {
				hasDomainRegex = true
				ruleType = RuleTypeDomain
				if s, ok := drVal.(string); ok {
					domainRegexInitial = s
					domainRegexInitialSet = true
				}
			} else if domainVal, ok := ruleData["domain"]; ok {
				hasDomain = true
				ruleType = RuleTypeDomain
				if domains := ExtractStringArray(domainVal); len(domains) > 0 {
					urlEntry.SetText(strings.Join(domains, "\n"))
				}
			} else if procVal, ok := ruleData[ProcessKey]; ok {
				hasProc = true
				ruleType = RuleTypeProcess
				if procs := ExtractStringArray(procVal); len(procs) > 0 {
					processesSelected = dedupeProcessStrings(procs)
					sortProcessStrings(processesSelected)
				}
			}
		}

		if !hasIP && !hasDomain && !hasDomainRegex && !hasProc {
			// Custom rule: use Rule data (minus outbound) as JSON content
			ruleType = RuleTypeCustom
			if ruleData != nil {
				temp := make(map[string]interface{})
				for k, v := range ruleData {
					if k == "outbound" {
						continue
					}
					temp[k] = v
				}
				if b, err := json.MarshalIndent(temp, "", "  "); err == nil {
					customEntry.SetText(string(b))
				}
			}
		}
	}

	// Manage field visibility
	ipLabel := widget.NewLabel("IP Addresses (one per line, CIDR format):")
	urlLabel := widget.NewLabel("Domains/URLs (one per line):")
	// Regex mode switch for domains
	domainRegexCheck := widget.NewCheck("Regex", func(bool) {})
	// Entry for domain regex (single-line)
	domainRegexEntry := widget.NewEntry()
	domainRegexEntry.SetPlaceHolder("Enter regular expression")
	// If we loaded a domain_regex from existing rule, restore it
	if domainRegexInitialSet {
		domainRegexCheck.SetChecked(true)
		domainRegexEntry.SetText(domainRegexInitial)
	}

	updateVisibility := func(selectedType string) {
		showIP := func() {
			ipLabel.Show()
			ipContainer.Show()
			urlLabel.Hide()
			urlContainer.Hide()
			domainRegexCheck.Hide()
			domainRegexEntry.Hide()
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
			customContainer.Hide()
			customLabel.Hide()

		}
		showProcess := func() {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Hide()
			urlContainer.Hide()
			domainRegexCheck.Hide()
			domainRegexEntry.Hide()
			processesLabel.Show()
			processesContainerWrap.Show()
			selectProcessesButton.Show()
			customContainer.Hide()
			customLabel.Hide()
		}
		showDomain := func() {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Show()
			urlContainer.Show()
			domainRegexCheck.Show()
			if domainRegexCheck.Checked {
				domainRegexEntry.Show()
				urlContainer.Hide()
			} else {
				domainRegexEntry.Hide()
				urlContainer.Show()
			}
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
			customContainer.Hide()
			customLabel.Hide()
		}
		showCustom := func() {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Hide()
			urlContainer.Hide()
			domainRegexCheck.Hide()
			domainRegexEntry.Hide()
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
			customContainer.Show()
			customLabel.Show()
		}

		switch selectedType {
		case RuleTypeIP:
			showIP()
		case RuleTypeProcess:
			showProcess()
		case RuleTypeCustom:
			showCustom()
		default:
			showDomain()
		}
	}

	// Save button and validation functions
	var confirmButton *widget.Button
	var saveRule func()
	var updateButtonState func()
	var ruleTypeRadio *widget.RadioGroup
	var dialogWindow fyne.Window

	parseCustomJSON := func() (map[string]interface{}, error) {
		trimmed := strings.TrimSpace(customEntry.Text)
		if trimmed == "" {
			return nil, errors.New("Custom JSON is empty")
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, errors.New("Custom JSON must be an object")
		}
		return obj, nil
	}

	buildRuleRaw := func(selectedType string, selectedOutbound string) (map[string]interface{}, error) {
		switch selectedType {
		case RuleTypeIP:
			ipText := strings.TrimSpace(ipEntry.Text)
			items := ParseLines(ipText, false)
			return map[string]interface{}{
				"ip_cidr":  items,
				"outbound": selectedOutbound,
			}, nil
		case RuleTypeProcess:
			items := make([]string, len(processesSelected))
			copy(items, processesSelected)
			return map[string]interface{}{
				ProcessKey: items,
				"outbound": selectedOutbound,
			}, nil
		case RuleTypeCustom:
			obj, err := parseCustomJSON()
			if err != nil {
				return nil, err
			}
			obj["outbound"] = selectedOutbound
			return obj, nil
		default:
			if domainRegexCheck != nil && domainRegexCheck.Checked {
				re := strings.TrimSpace(domainRegexEntry.Text)
				return map[string]interface{}{
					"domain_regex": re,
					"outbound":     selectedOutbound,
				}, nil
			}
			urlText := strings.TrimSpace(urlEntry.Text)
			items := ParseLines(urlText, false)
			return map[string]interface{}{
				"domain":   items,
				"outbound": selectedOutbound,
			}, nil
		}
	}

	validateFields := func() bool {
		if strings.TrimSpace(labelEntry.Text) == "" {
			return false
		}
		if ruleTypeRadio == nil {
			return false
		}
		switch ruleTypeRadio.Selected {
		case RuleTypeIP:
			return strings.TrimSpace(ipEntry.Text) != ""
		case RuleTypeProcess:
			return len(processesSelected) > 0
		case RuleTypeCustom:
			return strings.TrimSpace(customEntry.Text) != ""
		default:
			// Domain mode: either domain list non-empty or regex provided and valid
			if domainRegexCheck.Checked {
				re := strings.TrimSpace(domainRegexEntry.Text)
				if re == "" {
					return false
				}
				if _, err := regexp.Compile(re); err != nil {
					return false
				}
				return true
			}
			return strings.TrimSpace(urlEntry.Text) != ""
		}
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
	ruleTypeRadio = widget.NewRadioGroup([]string{RuleTypeIP, RuleTypeDomain, RuleTypeProcess, RuleTypeCustom}, func(selected string) {
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

		ruleRaw, err := buildRuleRaw(selectedType, selectedOutbound)
		if err != nil {
			dialog.ShowError(err, dialogWindow)
			return
		}

		// Save or update rule
		if isEdit {
			editRule.Rule.Label = label
			editRule.Rule.Rule = ruleRaw
			editRule.Rule.HasOutbound = true
			editRule.Rule.DefaultOutbound = selectedOutbound
			editRule.SelectedOutbound = selectedOutbound
		} else {
			newRule := &wizardmodels.RuleState{
				Rule: wizardtemplate.TemplateSelectableRule{
					Label:           label,
					Rule:            ruleRaw,
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
		// Mark as changed
		presenter.MarkAsChanged()
		// Refresh rules tab
		if createRulesTab != nil {
			presenter.RefreshRulesTab(createRulesTab)
		}
		delete(openDialogs, dialogKey)
		updateChildOverlay()
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
		updateChildOverlay()
		dialogWindow.Close()
	})

	// Field change handlers for validation
	labelEntry.OnChanged = func(string) { updateButtonState() }
	ipEntry.OnChanged = func(string) { updateButtonState() }
	urlEntry.OnChanged = func(string) { updateButtonState() }
	domainRegexEntry.OnChanged = func(string) { updateButtonState() }
	domainRegexCheck.OnChanged = func(bool) { updateVisibility(ruleTypeRadio.Selected); updateButtonState() }

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
		widget.NewLabel("Rule Type:"),
		ruleTypeRadio,
		widget.NewSeparator(),
		ipLabel,
		ipContainer,
		container.NewHBox(urlLabel, layout.NewSpacer(), domainRegexCheck),
		urlContainer,
		domainRegexEntry,
		processesLabel,
		processesContainerWrap,
		selectProcessesButton,
		customLabel,
		customContainer,
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
	updateChildOverlay()

	dialogWindow.SetCloseIntercept(func() {
		delete(openDialogs, dialogKey)
		updateChildOverlay()
		dialogWindow.Close()
	})

	// Refresh selected processes UI in case we loaded existing values
	refreshSelectedProcessesUI()
	updateButtonState()
	dialogWindow.Show()
}
