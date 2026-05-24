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
	"fmt"
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

	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/process"

	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardtemplate "singbox-launcher/core/template"
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
	dialogTitle := locale.T("wizard.add_rule.title_add")
	if isEdit {
		dialogTitle = locale.T("wizard.add_rule.title_edit")
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
	var activeTabIsRaw bool

	// Input field height
	inputFieldHeight := float32(90)

	// Input fields
	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_name"))

	ipEntry := widget.NewMultiLineEntry()
	ipEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_ip"))
	ipEntry.Wrapping = fyne.TextWrapWord

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_url"))
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
	processesLabel := widget.NewLabel(locale.T("wizard.add_rule.label_processes"))
	selectProcessesButton := widget.NewButton(locale.T("wizard.add_rule.button_select_processes"), func() {})

	// Match by path: checkbox, Simple/Regex radio, path patterns multiline
	matchByPathCheck := widget.NewCheck(locale.T("wizard.add_rule.check_match_by_path"), func(bool) {})
	pathModeRadio := widget.NewRadioGroup([]string{locale.T("wizard.add_rule.radio_simple"), locale.T("wizard.add_rule.radio_regex")}, func(string) {})
	pathPatternsEntry := widget.NewMultiLineEntry()
	pathPatternsEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_path_simple"))
	pathPatternsEntry.Wrapping = fyne.TextWrapWord
	pathPatternsScroll := container.NewScroll(pathPatternsEntry)
	pathPatternsSizeRect := canvas.NewRectangle(color.Transparent)
	pathPatternsSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	pathPatternsContainer := container.NewMax(pathPatternsSizeRect, pathPatternsScroll)
	pathPatternsLabel := widget.NewLabel(locale.T("wizard.add_rule.label_path_patterns"))

	// Custom JSON field (initialised early so it can be loaded when editing)
	customEntry := widget.NewMultiLineEntry()
	customEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_custom"))
	customEntry.SetText("{}")
	customScroll := container.NewScroll(customEntry)
	customSizeRect := canvas.NewRectangle(color.Transparent)
	customSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	customContainer := container.NewMax(customSizeRect, customScroll)
	customLabel := widget.NewLabel(locale.T("wizard.add_rule.label_custom_json"))

	// SRS: manual URLs (one per line)
	srsURLsEntry := widget.NewMultiLineEntry()
	srsURLsEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_srs_urls"))
	srsURLsEntry.Wrapping = fyne.TextWrapWord
	srsURLsScroll := container.NewScroll(srsURLsEntry)
	srsURLsSizeRect := canvas.NewRectangle(color.Transparent)
	srsURLsSizeRect.SetMinSize(fyne.NewSize(0, inputFieldHeight))
	srsURLsContainer := container.NewMax(srsURLsSizeRect, srsURLsScroll)
	srsURLsLabel := widget.NewLabel(locale.T("wizard.add_rule.label_srs_urls"))
	const runetfreedomSRSURL = "https://github.com/runetfreedom/russia-v2ray-rules-dat/tree/release/sing-box"
	srsHintButton := widget.NewButton("?", nil)
	srsLabelRow := container.NewHBox(srsURLsLabel, layout.NewSpacer(), srsHintButton)

	// Raw tab: JSON правила (синхронизация с формой при переключении вкладок)
	rawTabEntry := widget.NewMultiLineEntry()
	rawTabEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_raw"))
	rawTabEntry.Wrapping = fyne.TextWrapWord

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

	// Determine initial rule type and load data (для нового правила — первая позиция: IP)
	pathPatternsInitial := ""
	matchByPathInitial := false
	pathModeInitial := locale.T("wizard.add_rule.radio_regex") // по умолчанию Regex, если не в params
	srsURLsInitial := []string{}
	domainModeInitial := ""   // "Exact domains"|"Suffix"|"Keyword"|"Regex"
	domainListInitial := ""   // многострочный список для exact/suffix/keyword
	domainRegexInitial := ""  // строка для Regex
	ruleType := wizardmodels.RuleTypeIPS
	if isEdit {
		labelEntry.SetText(editRule.Rule.Label)
		if editRule.SelectedOutbound != "" && outboundMap[editRule.SelectedOutbound] {
			outboundSelect.SetSelected(editRule.SelectedOutbound)
		}
		ruleData := editRule.Rule.Rule
		ruleType = wizardmodels.DetermineRuleType(ruleData)
		params := editRule.Rule.Params

		if ruleData != nil {
			switch ruleType {
			case wizardmodels.RuleTypeIPS:
				if ips := ExtractStringArray(ruleData["ip_cidr"]); len(ips) > 0 {
					ipEntry.SetText(strings.Join(ips, "\n"))
				}
			case wizardmodels.RuleTypeURLs:
				if arr := ExtractStringArray(ruleData["domain_suffix"]); len(arr) > 0 {
				domainModeInitial = locale.T("wizard.add_rule.domain_suffix")
				domainListInitial = strings.Join(arr, "\n")
			} else if arr := ExtractStringArray(ruleData["domain_keyword"]); len(arr) > 0 {
				domainModeInitial = locale.T("wizard.add_rule.domain_keyword")
				domainListInitial = strings.Join(arr, "\n")
			} else if re, ok := ruleData["domain_regex"].(string); ok && re != "" {
				domainModeInitial = locale.T("wizard.add_rule.domain_regex")
				domainRegexInitial = re
			} else if domains := ExtractStringArray(ruleData["domain"]); len(domains) > 0 {
				domainModeInitial = locale.T("wizard.add_rule.domain_exact")
					domainListInitial = strings.Join(domains, "\n")
				}
				if params != nil {
					if mode, ok := params["domain_mode"].(string); ok {
						domainModeInitial = mode
					}
				}
			case wizardmodels.RuleTypeProcesses:
				if procs := ExtractStringArray(ruleData[ProcessKey]); len(procs) > 0 {
					processesSelected = dedupeProcessStrings(procs)
					sortProcessStrings(processesSelected)
				}
				if pathVal, ok := ruleData[ProcessPathRegexKey]; ok {
					matchByPathInitial = true
					if arr := ExtractStringArray(pathVal); len(arr) > 0 {
						pathPatternsInitial = strings.Join(arr, "\n")
					}
				}
				if params != nil {
					if v, ok := params["match_by_path"].(bool); ok {
						matchByPathInitial = v
					}
				if s, ok := params["path_mode"].(string); ok && (s == locale.T("wizard.add_rule.radio_simple") || s == locale.T("wizard.add_rule.radio_regex")) {
					pathModeInitial = s
				}
				}
			case wizardmodels.RuleTypeSRS:
				for _, rs := range editRule.Rule.RuleSets {
					var m map[string]interface{}
					if err := json.Unmarshal(rs, &m); err == nil {
						if u, ok := m["url"].(string); ok && u != "" {
							srsURLsInitial = append(srsURLsInitial, u)
						}
					}
				}
			case wizardmodels.RuleTypeRaw:
				fallthrough
			default:
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
		if ruleType == wizardmodels.RuleTypeSRS && len(srsURLsInitial) > 0 {
			srsURLsEntry.SetText(strings.Join(srsURLsInitial, "\n"))
		}
	}
	if isEdit && ruleType != "" {
		if rd := editRule.Rule.Rule; rd != nil {
			if b, err := json.MarshalIndent(rd, "", "  "); err == nil {
				rawTabEntry.SetText(string(b))
			}
		}
	} else {
		rawTabEntry.SetText(`{
  "ip_cidr": [],
  "outbound": "proxy-out"
}`)
	}

	// Rule type selection: микро-модель + 5 типов (подписи человекочитаемые, значения — константы)
	ruleSel := NewRuleTypeSelection(ruleType)
	var syncingRuleType bool
	// Single RadioGroup для всех 5 типов — настоящий radio с круглыми маркерами
	// (раньше были 5 NewCheck с mutex-логикой через OnChanged → визуально галки).
	typeLabelIP := locale.T("wizard.add_rule.type_ip")
	typeLabelDomain := locale.T("wizard.add_rule.type_domain")
	typeLabelProcess := locale.T("wizard.add_rule.type_process")
	typeLabelSRS := locale.T("wizard.add_rule.type_srs")
	typeLabelCustom := locale.T("wizard.add_rule.type_custom")

	ruleTypeRadio := widget.NewRadioGroup(
		[]string{typeLabelIP, typeLabelDomain, typeLabelProcess, typeLabelSRS, typeLabelCustom},
		func(sel string) {
			if syncingRuleType {
				return
			}
			switch sel {
			case typeLabelIP:
				ruleSel.SetType(wizardmodels.RuleTypeIPS)
			case typeLabelDomain:
				ruleSel.SetType(wizardmodels.RuleTypeURLs)
			case typeLabelProcess:
				ruleSel.SetType(wizardmodels.RuleTypeProcesses)
			case typeLabelSRS:
				ruleSel.SetType(wizardmodels.RuleTypeSRS)
			case typeLabelCustom:
				ruleSel.SetType(wizardmodels.RuleTypeRaw)
			}
		},
	)

	// Domains/URLs: domainModeSelect (exact / suffix / keyword / regex).
	// Раньше был справа от Domain checkbox; теперь — отдельная строка под radio,
	// показывается только когда выбран Domain тип (видимость управляется в
	// updateFieldsVisibility — см. ниже legacy syncFormToRaw path).
	domainModeOptions := []string{locale.T("wizard.add_rule.domain_exact"), locale.T("wizard.add_rule.domain_suffix"), locale.T("wizard.add_rule.domain_keyword"), locale.T("wizard.add_rule.domain_regex")}
	domainModeSelect := widget.NewSelect(domainModeOptions, nil)
	domainModeRow := container.NewHBox(widget.NewLabel(locale.T("wizard.add_rule.domain_mode_label")), domainModeSelect)

	// matchByPathCheck (для Process) — аналогично, под radio, не inline.
	processOptionsRow := container.NewHBox(matchByPathCheck)

	ruleTypeContainer := container.NewVBox(ruleTypeRadio, domainModeRow, processOptionsRow)

	// Manage field visibility
	ipLabel := widget.NewLabel(locale.T("wizard.add_rule.label_ip"))
	urlLabel := widget.NewLabel(locale.T("wizard.add_rule.label_domains"))
	domainRegexEntry := widget.NewEntry()
	domainRegexEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_domain_regex"))
	updateDomainLabel := func() {
		switch domainModeSelect.Selected {
		case locale.T("wizard.add_rule.domain_suffix"):
			urlLabel.SetText(locale.T("wizard.add_rule.label_suffixes"))
		case locale.T("wizard.add_rule.domain_keyword"):
			urlLabel.SetText(locale.T("wizard.add_rule.label_keywords"))
		case locale.T("wizard.add_rule.domain_regex"):
			urlLabel.SetText(locale.T("wizard.add_rule.label_domain_regex"))
		default:
			urlLabel.SetText(locale.T("wizard.add_rule.label_domains"))
		}
	}
	domainModeSelect.SetSelected(locale.T("wizard.add_rule.domain_exact"))
	if domainModeInitial != "" {
		domainModeSelect.SetSelected(domainModeInitial)
		if domainModeInitial == locale.T("wizard.add_rule.domain_regex") {
			domainRegexEntry.SetText(domainRegexInitial)
		} else {
			urlEntry.SetText(domainListInitial)
		}
	}

	updateVisibility := func(selectedType string) {
		hideAllFormTypeSpecific := func() {
			ipLabel.Hide()
			ipContainer.Hide()
			urlLabel.Hide()
			urlContainer.Hide()
			domainRegexEntry.Hide()
			domainModeSelect.Hide() // показываем только при выборе Domains/URLs
			processesLabel.Hide()
			processesContainerWrap.Hide()
			selectProcessesButton.Hide()
			matchByPathCheck.Hide()
			pathPatternsLabel.Hide()
			pathPatternsContainer.Hide()
			pathModeRadio.Hide()
			srsLabelRow.Hide()
			srsURLsContainer.Hide()
			customContainer.Hide()
			customLabel.Hide()
		}
		showIP := func() {
			hideAllFormTypeSpecific()
			ipLabel.Show()
			ipContainer.Show()
		}
		updateProcessModeVisibility := func() {
			if ruleSel.Type() != wizardmodels.RuleTypeProcesses {
				return
			}
			if matchByPathCheck.Checked {
				processesLabel.Hide()
				processesContainerWrap.Hide()
				selectProcessesButton.Hide()
				pathPatternsLabel.Show()
				pathPatternsContainer.Show()
				pathModeRadio.Show()
			} else {
				processesLabel.Show()
				processesContainerWrap.Show()
				selectProcessesButton.Show()
				pathPatternsLabel.Hide()
				pathPatternsContainer.Hide()
				pathModeRadio.Hide()
			}
		}
		showProcess := func() {
			hideAllFormTypeSpecific()
			matchByPathCheck.Show()
			updateProcessModeVisibility()
		}
		showDomain := func() {
			hideAllFormTypeSpecific()
			domainModeSelect.Show()
			urlLabel.Show()
			updateDomainLabel()
			if domainModeSelect.Selected == locale.T("wizard.add_rule.domain_regex") {
				domainRegexEntry.Show()
				urlContainer.Hide()
			} else {
				urlContainer.Show()
				domainRegexEntry.Hide()
			}
		}
		showSRS := func() {
			hideAllFormTypeSpecific()
			srsLabelRow.Show()
			srsURLsContainer.Show()
		}
		showCustom := func() {
			hideAllFormTypeSpecific()
			customContainer.Show()
			customLabel.Show()
		}

		switch selectedType {
		case wizardmodels.RuleTypeIPS:
			showIP()
		case wizardmodels.RuleTypeProcesses:
			showProcess()
		case wizardmodels.RuleTypeSRS:
			showSRS()
		case wizardmodels.RuleTypeRaw:
			showCustom()
		default:
			showDomain()
		}
	}

	// Save button and validation functions
	var confirmButton *widget.Button
	var saveRule func()
	var updateButtonState func()
	var dialogWindow fyne.Window

	parseCustomJSON := func() (map[string]interface{}, error) {
		trimmed := strings.TrimSpace(customEntry.Text)
		if trimmed == "" {
			return nil, errors.New(locale.T("wizard.add_rule.error_custom_empty"))
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, errors.New(locale.T("wizard.add_rule.error_custom_object"))
		}
		return obj, nil
	}

	buildSRSRuleSetsAndTags := func() (ruleSets []json.RawMessage, tags []string, err error) {
		lines := ParseLines(strings.TrimSpace(srsURLsEntry.Text), false)
		if len(lines) == 0 {
			return nil, nil, errors.New(locale.T("wizard.add_rule.error_srs_required"))
		}
		seen := make(map[string]bool)
		for _, rawURL := range lines {
			u := strings.TrimSpace(rawURL)
			if u == "" {
				continue
			}
			tag := srsTagFromURL(u)
			if tag == "" {
				continue
			}
			// Дедуп: один и тот же URL дважды → один rule_set entry.
			// Content-addressed tag это гарантирует автоматически (same URL
			// → same tag), просто скипаем второй раз.
			if seen[tag] {
				continue
			}
			seen[tag] = true
			entry := map[string]interface{}{
				"tag":    tag,
				"type":   "remote",
				"format": "binary",
				"url":    u,
			}
			raw, _ := json.Marshal(entry)
			ruleSets = append(ruleSets, raw)
			tags = append(tags, tag)
		}
		if len(ruleSets) == 0 {
			return nil, nil, errors.New(locale.T("wizard.add_rule.error_srs_valid"))
		}
		return ruleSets, tags, nil
	}

	// buildRuleRaw возвращает (rule, ruleSets для SRS или nil, error).
	buildRuleRaw := func(selectedType string, selectedOutbound string) (rule map[string]interface{}, ruleSets []json.RawMessage, err error) {
		switch selectedType {
		case wizardmodels.RuleTypeIPS:
			ipText := strings.TrimSpace(ipEntry.Text)
			items := ParseLines(ipText, false)
			return map[string]interface{}{
				"ip_cidr":  items,
				"outbound": selectedOutbound,
			}, nil, nil
		case wizardmodels.RuleTypeProcesses:
			if matchByPathCheck.Checked {
				lines := ParseLines(pathPatternsEntry.Text, false)
				if len(lines) == 0 {
					return nil, nil, errors.New(locale.T("wizard.add_rule.error_path_required"))
				}
				regexList := make([]string, 0, len(lines))
				isSimple := pathModeRadio.Selected != locale.T("wizard.add_rule.radio_regex")
				for _, line := range lines {
					var re string
					if isSimple {
						var e error
						re, e = SimplePatternToRegex(line)
						if e != nil {
							return nil, nil, e
						}
					} else {
						if _, e := regexp.Compile(line); e != nil {
							return nil, nil, e
						}
						re = line
					}
					regexList = append(regexList, re)
				}
				return map[string]interface{}{
					ProcessPathRegexKey: regexList,
					"outbound":          selectedOutbound,
				}, nil, nil
			}
			items := make([]string, len(processesSelected))
			copy(items, processesSelected)
			return map[string]interface{}{
				ProcessKey: items,
				"outbound": selectedOutbound,
			}, nil, nil
		case wizardmodels.RuleTypeSRS:
			sets, tags, e := buildSRSRuleSetsAndTags()
			if e != nil {
				return nil, nil, e
			}
			var ruleSetVal interface{} = tags
			if len(tags) == 1 {
				ruleSetVal = tags[0]
			}
			return map[string]interface{}{
				"rule_set": ruleSetVal,
				"outbound": selectedOutbound,
			}, sets, nil
		case wizardmodels.RuleTypeRaw:
			obj, e := parseCustomJSON()
			if e != nil {
				return nil, nil, e
			}
			obj["outbound"] = selectedOutbound
			return obj, nil, nil
		default:
			items := ParseLines(strings.TrimSpace(urlEntry.Text), false)
		switch domainModeSelect.Selected {
		case locale.T("wizard.add_rule.domain_regex"):
			re := strings.TrimSpace(domainRegexEntry.Text)
			return map[string]interface{}{
				"domain_regex": re,
				"outbound":     selectedOutbound,
			}, nil, nil
		case locale.T("wizard.add_rule.domain_suffix"):
			return map[string]interface{}{
				"domain_suffix": items,
				"outbound":      selectedOutbound,
			}, nil, nil
		case locale.T("wizard.add_rule.domain_keyword"):
			return map[string]interface{}{
				"domain_keyword": items,
				"outbound":       selectedOutbound,
			}, nil, nil
			default:
				return map[string]interface{}{
					"domain":   items,
					"outbound": selectedOutbound,
				}, nil, nil
			}
		}
	}

	validateFields := func() bool {
		if strings.TrimSpace(labelEntry.Text) == "" {
			return false
		}
		switch ruleSel.Type() {
		case wizardmodels.RuleTypeIPS:
			return strings.TrimSpace(ipEntry.Text) != ""
		case wizardmodels.RuleTypeProcesses:
			if matchByPathCheck.Checked {
				lines := ParseLines(pathPatternsEntry.Text, false)
				if len(lines) == 0 {
					return false
				}
				isSimple := pathModeRadio.Selected != locale.T("wizard.add_rule.radio_regex")
				for _, line := range lines {
					if isSimple {
						if _, err := SimplePatternToRegex(line); err != nil {
							return false
						}
					} else {
						if _, err := regexp.Compile(line); err != nil {
							return false
						}
					}
				}
				return true
			}
			return len(processesSelected) > 0
		case wizardmodels.RuleTypeSRS:
			return len(ParseLines(strings.TrimSpace(srsURLsEntry.Text), false)) > 0
		case wizardmodels.RuleTypeRaw:
			return strings.TrimSpace(customEntry.Text) != ""
		default:
		if domainModeSelect.Selected == locale.T("wizard.add_rule.domain_regex") {
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

	onRuleTypeChange := func(s string) {
		syncingRuleType = true
		defer func() { syncingRuleType = false }()
		// Map model rule-type → radio label.
		switch s {
		case wizardmodels.RuleTypeIPS:
			ruleTypeRadio.SetSelected(typeLabelIP)
		case wizardmodels.RuleTypeURLs:
			ruleTypeRadio.SetSelected(typeLabelDomain)
		case wizardmodels.RuleTypeProcesses:
			ruleTypeRadio.SetSelected(typeLabelProcess)
		case wizardmodels.RuleTypeSRS:
			ruleTypeRadio.SetSelected(typeLabelSRS)
		case wizardmodels.RuleTypeRaw:
			ruleTypeRadio.SetSelected(typeLabelCustom)
		}
		updateVisibility(s)
		if updateButtonState != nil {
			updateButtonState()
		}
	}
	ruleSel.SetOnChange(onRuleTypeChange)
	onRuleTypeChange(ruleSel.Type()) // начальная синхронизация при открытии (SetType не дергает OnChange, т.к. тип уже тот же)

	// When Match by path is toggled, refresh Process UI (name vs path) and validation
	matchByPathCheck.OnChanged = func(bool) {
		updateVisibility(ruleSel.Type())
		if updateButtonState != nil {
			updateButtonState()
		}
	}
	pathModeRadio.OnChanged = func(selected string) {
		if selected == locale.T("wizard.add_rule.radio_regex") {
			pathPatternsEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_path_regex"))
		} else {
			pathPatternsEntry.SetPlaceHolder(locale.T("wizard.add_rule.placeholder_path_simple"))
		}
		if updateButtonState != nil {
			updateButtonState()
		}
	}

	pathModeRadio.SetSelected(locale.T("wizard.add_rule.radio_simple"))
	if matchByPathInitial {
		matchByPathCheck.SetChecked(true)
		pathPatternsEntry.SetText(pathPatternsInitial)
		pathModeRadio.SetSelected(pathModeInitial)
		updateVisibility(ruleSel.Type())
	}

	saveRule = func() {
		label := strings.TrimSpace(labelEntry.Text)
		if label == "" {
			dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_name_required")), dialogWindow)
			return
		}
		var ruleRaw map[string]interface{}
		var srsRuleSets []json.RawMessage
		selectedType := ruleSel.Type()
		selectedOutbound := outboundSelect.Selected
		if selectedOutbound == "" {
			selectedOutbound = availableOutbounds[0]
		}

		if activeTabIsRaw {
			trimmed := strings.TrimSpace(rawTabEntry.Text)
			if trimmed == "" {
				dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_raw_empty")), dialogWindow)
				return
			}
			if err := json.Unmarshal([]byte(trimmed), &ruleRaw); err != nil {
				dialog.ShowError(fmt.Errorf("invalid JSON: %w", err), dialogWindow)
				return
			}
			if ruleRaw == nil {
				dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_must_be_object")), dialogWindow)
				return
			}
			if _, hasOut := ruleRaw["outbound"]; !hasOut {
				if _, hasAction := ruleRaw["action"]; !hasAction {
					dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_must_have_outbound")), dialogWindow)
					return
				}
			}
			if selectedOutbound == wizardmodels.RejectActionName || selectedOutbound == "drop" {
				ruleRaw["action"] = selectedOutbound
				delete(ruleRaw, "outbound")
			} else {
				ruleRaw["outbound"] = selectedOutbound
			}
			selectedType = wizardmodels.RuleTypeRaw
		} else {
			var err error
			ruleRaw, srsRuleSets, err = buildRuleRaw(selectedType, selectedOutbound)
			if err != nil {
				dialog.ShowError(err, dialogWindow)
				return
			}
		}

		params := make(map[string]interface{})
		if selectedType == wizardmodels.RuleTypeProcesses {
			params["match_by_path"] = matchByPathCheck.Checked
			if matchByPathCheck.Checked {
		if pathModeRadio.Selected == locale.T("wizard.add_rule.radio_simple") {
				params["path_mode"] = locale.T("wizard.add_rule.radio_simple")
			} else {
				params["path_mode"] = locale.T("wizard.add_rule.radio_regex")
			}
			}
		}
		if selectedType == wizardmodels.RuleTypeURLs {
			params["domain_mode"] = domainModeSelect.Selected
		}

		if isEdit {
			editRule.Rule.Label = label
			editRule.Rule.Rule = ruleRaw
			editRule.Rule.HasOutbound = true
			editRule.Rule.DefaultOutbound = selectedOutbound
			editRule.Rule.Params = params
			if len(srsRuleSets) > 0 {
				editRule.Rule.RuleSets = srsRuleSets
			} else if selectedType != wizardmodels.RuleTypeSRS {
				editRule.Rule.RuleSets = nil
			}
			editRule.SelectedOutbound = selectedOutbound
		} else {
			tsr := wizardtemplate.TemplateSelectableRule{
				Label:           label,
				Rule:            ruleRaw,
				HasOutbound:     true,
				DefaultOutbound: selectedOutbound,
				IsDefault:       true,
				Params:          params,
			}
			if len(srsRuleSets) > 0 {
				tsr.RuleSets = srsRuleSets
			}
			newRule := &wizardmodels.RuleState{
				Rule:             tsr,
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

	confirmBtnText := locale.T("wizard.add_rule.button_add")
	if isEdit {
		confirmBtnText = locale.T("wizard.add_rule.button_save")
	}
	confirmButton = widget.NewButton(confirmBtnText, saveRule)
	confirmButton.Importance = widget.HighImportance

	cancelButton := widget.NewButton(locale.T("wizard.add_rule.button_cancel"), func() {
		delete(openDialogs, dialogKey)
		updateChildOverlay()
		dialogWindow.Close()
	})

	// Field change handlers for validation
	labelEntry.OnChanged = func(string) { updateButtonState() }
	ipEntry.OnChanged = func(string) { updateButtonState() }
	urlEntry.OnChanged = func(string) { updateButtonState() }
	domainRegexEntry.OnChanged = func(string) { updateButtonState() }
	domainModeSelect.OnChanged = func(string) {
		updateDomainLabel()
		updateVisibility(ruleSel.Type())
		updateButtonState()
	}
	pathPatternsEntry.OnChanged = func(string) { updateButtonState() }
	srsURLsEntry.OnChanged = func(string) { updateButtonState() }

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
			removeBtn := widget.NewButton(locale.T("wizard.add_rule.button_remove"), func() {
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
		w := controller.UIService.Application.NewWindow(locale.T("wizard.add_rule.window_select_processes"))
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

		addBtn := widget.NewButton(locale.T("wizard.add_rule.button_add_process"), func() {
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

		refreshBtn := widget.NewButton(locale.T("wizard.add_rule.button_refresh"), func() {
			listData = getProcesses()
			procList.Refresh()
		})

		closeBtn := widget.NewButton(locale.T("wizard.add_rule.button_close"), func() { w.Close() })

		content := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), refreshBtn, addBtn, closeBtn), nil, nil, container.NewScroll(procList))
		w.SetContent(content)
		w.Show()
	}

	// wire selector button
	selectProcessesButton.OnTapped = func() { openProcessSelector() }

	// Rule name над вкладками Form/Raw
	ruleNameBlock := container.NewVBox(widget.NewLabel(locale.T("wizard.add_rule.label_name")), labelEntry)
	// Контент формы: тип правила и поля по типу
	inputContainer := container.NewVBox(
		widget.NewLabel(locale.T("wizard.add_rule.label_type")),
		ruleTypeContainer,
		widget.NewSeparator(),
		ipLabel,
		ipContainer,
		urlLabel,
		urlContainer,
		domainRegexEntry,
		processesLabel,
		processesContainerWrap,
		selectProcessesButton,
		pathPatternsLabel,
		pathPatternsContainer,
		pathModeRadio,
		srsLabelRow,
		srsURLsContainer,
		customLabel,
		customContainer,
		widget.NewSeparator(),
		widget.NewLabel(locale.T("wizard.add_rule.label_outbound")),
		outboundSelect,
	)

	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		cancelButton,
		confirmButton,
	)

	formScroll := container.NewScroll(inputContainer)
	rawScroll := container.NewScroll(rawTabEntry)
	formTabItem := container.NewTabItem(locale.T("wizard.add_rule.tab_form"), formScroll)
	rawTabItem := container.NewTabItem(locale.T("wizard.add_rule.tab_raw"), rawScroll)
	tabs := container.NewAppTabs(formTabItem, rawTabItem)
	syncFormToRaw := func() {
		ob := outboundSelect.Selected
		if ob == "" {
			ob = availableOutbounds[0]
		}
		ruleRaw, _, err := buildRuleRaw(ruleSel.Type(), ob)
		if err == nil && ruleRaw != nil {
			if b, e := json.MarshalIndent(ruleRaw, "", "  "); e == nil {
				rawTabEntry.SetText(string(b))
			}
		}
	}
	syncRawToForm := func() {
		trimmed := strings.TrimSpace(rawTabEntry.Text)
		if trimmed == "" {
			dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_raw_empty")), dialogWindow)
			tabs.SelectTab(rawTabItem)
			ruleSel.SetType(wizardmodels.RuleTypeRaw)
			return
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			dialog.ShowError(fmt.Errorf("invalid JSON: %w", err), dialogWindow)
			tabs.SelectTab(rawTabItem)
			ruleSel.SetType(wizardmodels.RuleTypeRaw)
			return
		}
		if obj == nil {
			dialog.ShowError(errors.New(locale.T("wizard.add_rule.error_must_be_object")), dialogWindow)
			tabs.SelectTab(rawTabItem)
			ruleSel.SetType(wizardmodels.RuleTypeRaw)
			return
		}
		detected := wizardmodels.DetermineRuleType(obj)
		if detected == wizardmodels.RuleTypeRaw {
			dialog.ShowInformation(locale.T("wizard.add_rule.dialog_not_recognized_title"), locale.T("wizard.add_rule.dialog_not_recognized_msg"), dialogWindow)
			tabs.SelectTab(rawTabItem)
			ruleSel.SetType(wizardmodels.RuleTypeRaw)
			activeTabIsRaw = true
			return
		}
		ruleSel.SetType(detected)
		switch detected {
		case wizardmodels.RuleTypeIPS:
			if ips := ExtractStringArray(obj["ip_cidr"]); len(ips) > 0 {
				ipEntry.SetText(strings.Join(ips, "\n"))
			}
		case wizardmodels.RuleTypeURLs:
		if arr := ExtractStringArray(obj["domain_suffix"]); len(arr) > 0 {
			domainModeSelect.SetSelected(locale.T("wizard.add_rule.domain_suffix"))
			urlEntry.SetText(strings.Join(arr, "\n"))
		} else if arr := ExtractStringArray(obj["domain_keyword"]); len(arr) > 0 {
			domainModeSelect.SetSelected(locale.T("wizard.add_rule.domain_keyword"))
			urlEntry.SetText(strings.Join(arr, "\n"))
		} else if re, ok := obj["domain_regex"].(string); ok && re != "" {
			domainModeSelect.SetSelected(locale.T("wizard.add_rule.domain_regex"))
			domainRegexEntry.SetText(re)
		} else if domains := ExtractStringArray(obj["domain"]); len(domains) > 0 {
			domainModeSelect.SetSelected(locale.T("wizard.add_rule.domain_exact"))
			urlEntry.SetText(strings.Join(domains, "\n"))
			}
			updateDomainLabel()
			updateVisibility(ruleSel.Type())
		case wizardmodels.RuleTypeProcesses:
			if procs := ExtractStringArray(obj[ProcessKey]); len(procs) > 0 {
				processesSelected = dedupeProcessStrings(procs)
				sortProcessStrings(processesSelected)
				refreshSelectedProcessesUI()
			} else if arr := ExtractStringArray(obj[ProcessPathRegexKey]); len(arr) > 0 {
				matchByPathCheck.SetChecked(true)
				pathPatternsEntry.SetText(strings.Join(arr, "\n"))
			}
		}
		// Восстанавливаем outbound в форме из rule
		if ob, ok := obj["outbound"].(string); ok && ob != "" && outboundMap[ob] {
			outboundSelect.SetSelected(ob)
		} else if action, ok := obj["action"].(string); ok && action != "" {
			if outboundMap[action] {
				outboundSelect.SetSelected(action)
			}
		}
	}
	tabs.OnSelected = func(t *container.TabItem) {
		if t == rawTabItem {
			activeTabIsRaw = true
			syncFormToRaw()
		} else {
			activeTabIsRaw = false
			syncRawToForm()
		}
	}

	// Border: сверху Rule name, снизу кнопки, центр — вкладки на всю оставшуюся высоту
	mainContent := container.NewBorder(
		ruleNameBlock,
		buttonsContainer,
		nil,
		nil,
		tabs,
	)

	// Create window - get Application from presenter's controller
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	dialogWindow = controller.UIService.Application.NewWindow(dialogTitle)
	srsHintButton.OnTapped = func() {
		msg := widget.NewLabel(locale.T("wizard.add_rule.srs_hint"))
		openBtn := widget.NewButton(locale.T("wizard.add_rule.button_open"), func() {
			_ = platform.OpenURL(runetfreedomSRSURL)
		})
		content := container.NewVBox(msg, openBtn)
		dialog.ShowCustom(locale.T("wizard.add_rule.srs_dialog_title"), locale.T("wizard.add_rule.button_close"), content, dialogWindow)
	}
	dialogWindow.Resize(fyne.NewSize(500, 640))
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
