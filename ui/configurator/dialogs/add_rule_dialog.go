// Package dialogs содержит диалоговые окна визарда конфигурации.
//
// Файл add_rule_dialog.go — окно добавления и правки пользовательского
// правила маршрутизации (SPEC 188 §6): имя, стопка условий (форма) или тело
// JSON, выход (Direction), настройки наборов, вкладки DNS и Preview.
//
// Раскладка окна по файлам:
//   - rule_cond_stack.go, rule_cond_frame.go, rule_cond_group.go,
//     rule_cond_refs.go — форма: стопка условий-рамок по таблице реестра,
//     группы правил, поле наборов;
//   - rule_dialog_dns.go — вкладка DNS; rule_dialog_preview.go — Preview;
//   - rule_process_picker.go — выбор процессов;
//   - ui/configurator/models/rule_conditions.go — тело ⇄ стопка (без Fyne).
//
// Окно правит запись модели (RuleState): тело правила с целью и `refs`
// внутри, объект dns, настройки наборов. Используется в tabs/rules_tab.go.
package dialogs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/outboundutil"

	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	addRuleArrayPastedText  = "Paste one rule at a time: the editor keeps one sing-box rule per entry"
	addRuleNestedTargetText = "Only the rule itself has a target (outbound or action): remove it from the rules inside groups."
	addRuleNoConditionText  = "Add at least one condition."
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
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	app := controller.UIService.Application

	isEdit := editRule != nil
	dialogTitle := locale.T("Add Rule")
	if isEdit {
		dialogTitle = locale.T("Edit Rule")
	}

	// Ensure only one rule dialog is open at a time
	openDialogs := presenter.OpenRuleDialogs()
	for key, existingDialog := range openDialogs {
		existingDialog.Close()
		delete(openDialogs, key)
	}
	presenter.UpdateChildOverlay()
	updateChildOverlay := func() { presenter.UpdateChildOverlay() }
	dialogKey := ruleIndex
	if !isEdit {
		dialogKey = -1
	}
	updateChildOverlay()

	table, err := wizardmodels.LauncherRuleCondTable()
	if err != nil {
		debuglog.WarnLog("rule dialog: rule conditions table: %v", err)
	}

	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(locale.T("Rule name"))

	// Выход (Direction) — цель корня тела.
	availableOutbounds := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(model))
	if len(availableOutbounds) == 0 {
		availableOutbounds = []string{wizardmodels.DefaultOutboundTag, wizardmodels.RejectActionName}
	}
	outboundMap := make(map[string]bool, len(availableOutbounds))
	for _, opt := range availableOutbounds {
		outboundMap[opt] = true
	}
	outboundSelect := widget.NewSelect(availableOutbounds, nil)
	outboundSelect.SetSelected(availableOutbounds[0])
	selectedOutbound := func() string {
		if outboundSelect.Selected != "" {
			return outboundSelect.Selected
		}
		return availableOutbounds[0]
	}

	// SPEC 185: настройки наборов правила (режим, период, выход загрузки) —
	// видны, пока в дереве есть ссылки `refs`.
	var srsOptionsInitial corestate.RuleSetOptions
	var initialDNS *corestate.RuleDNS
	var initialBody map[string]interface{}
	if isEdit {
		srsOptionsInitial = editRule.SrsOptions
		initialDNS = editRule.DNS
		initialBody = editRule.Rule.Rule
		labelEntry.SetText(editRule.Rule.Label)
		if editRule.SelectedOutbound != "" && outboundMap[editRule.SelectedOutbound] {
			outboundSelect.SetSelected(editRule.SelectedOutbound)
		}
	}
	srsOptionsForm := NewRuleSetOptionsForm(srsOptionsInitial, availableOutbounds, nil)

	// Открытие: тело из ключей таблицы — формой; иначе — вкладкой JSON.
	startStack := wizardmodels.RuleCondStack{}
	formOK := true
	if isEdit {
		if st, ok := table.StackFromBody(initialBody); ok {
			startStack = st.ForForm()
		} else {
			formOK = false
		}
	}
	activeTabIsRaw := !formOK

	var dialogWindow fyne.Window
	var refreshState func()
	ctx := &ruleFormCtx{
		table: table,
		app:   app,
		goos:  model.Target.Normalized().GOOS,
		wheel: components.NewWheelThrough(),
		onChange: func() {
			if refreshState != nil {
				refreshState()
			}
		},
	}

	// Снимок формы: правило, которое открыли и не меняли, сохраняется
	// исходным телом (порядок ключей и вид значений не переписываются).
	// snapshotJSON — тело формы на момент загрузки, snapshotMatch — тело,
	// из которого форма загружена.
	var editor *condStackEditor
	var snapshotJSON string
	var snapshotMatch map[string]interface{}
	formHolder := container.NewVBox()
	loadForm := func(s wizardmodels.RuleCondStack, match map[string]interface{}) {
		editor = newCondStackEditor(ctx, s)
		formHolder.Objects = []fyne.CanvasObject{editor.object()}
		formHolder.Refresh()
		snapshotMatch = cloneRuleMap(match)
		if read, rerr := editor.read(false); rerr == nil {
			snapshotJSON = table.RuleCondBodyJSON(read)
		}
	}
	if formOK {
		loadForm(startStack, ruleMatchOf(initialBody))
	} else {
		loadForm(wizardmodels.RuleCondStack{}, nil)
	}

	// formMatch — тело формы без цели; nil — условий нет.
	formMatch := func(strict bool) (map[string]interface{}, error) {
		lenient, rerr := editor.read(false)
		if rerr == nil && snapshotMatch != nil && table.RuleCondBodyJSON(lenient) == snapshotJSON {
			return cloneRuleMap(snapshotMatch), nil
		}
		s, rerr := editor.read(strict)
		if rerr != nil {
			return nil, rerr
		}
		return table.StackBody(s), nil
	}

	rawTabEntry := fynewidget.NewJSONEditor("")
	// rawMatch — тело с вкладки JSON без цели корня.
	rawMatch := func() (map[string]interface{}, error) {
		trimmed := strings.TrimSpace(rawTabEntry.Text())
		if trimmed == "" {
			return nil, errors.New(locale.T("Raw JSON is empty"))
		}
		if err := ruleArrayPastedError(trimmed); err != nil {
			return nil, err
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		if obj == nil {
			return nil, errors.New(locale.T("rule must be a JSON object"))
		}
		if ruleHasNestedTarget(obj) {
			return nil, errors.New(locale.T(addRuleNestedTargetText))
		}
		return ruleMatchOf(obj), nil
	}
	currentMatch := func(strict bool) (map[string]interface{}, error) {
		if activeTabIsRaw {
			return rawMatch()
		}
		return formMatch(strict)
	}
	// currentBody — тело с целью, каким его увидит сборка; nil — не заполнено.
	currentBody := func() map[string]interface{} {
		m, err := currentMatch(false)
		if err != nil || len(m) == 0 {
			return nil
		}
		return ruleWithTarget(m, selectedOutbound())
	}

	dnsWheel := components.NewWheelThrough()
	dnsTab := newRuleDNSTab(model, initialDNS, initialBody, dnsWheel, ctx.changed)
	// Блок наборов — выбор режима, период, detour: своей прокрутки в нём нет,
	// колесо над ним целиком отдаётся форме; прячется обёртка вместе со слоем.
	srsOptionsRow := ctx.wheel.Wrap(srsOptionsForm.Box)

	var confirmButton *widget.Button
	confirmHint := widget.NewLabel("")
	confirmHint.Importance = widget.WarningImportance
	confirmHint.Truncation = fyne.TextTruncateEllipsis
	refreshState = func() {
		if editor == nil {
			return
		}
		body := currentBody()
		dnsTab.update(body)
		refs := corestate.BodyRefs(body)
		if len(refs) > 0 {
			srsOptionsRow.Show()
			srsOptionsForm.SetModeLocked(ruleRefsHaveFile(refs))
		} else {
			srsOptionsRow.Hide()
		}
		if confirmButton == nil {
			return
		}
		// Почему кнопка неактивна — строкой слева от кнопок: имя, условия и
		// ответ DNS лежат на разных вкладках, по серой кнопке причину не угадать.
		reason := ""
		switch {
		case strings.TrimSpace(labelEntry.Text) == "":
			reason = locale.T("Rule name is required")
		case body == nil:
			reason = locale.T(addRuleNoConditionText)
			if _, err := currentMatch(false); err != nil {
				reason = err.Error()
			}
		default:
			if e := dnsTab.answerError(); e != "" {
				reason = locale.T("DNS") + ": " + e
			}
		}
		confirmHint.SetText(reason)
		if reason == "" {
			confirmButton.Enable()
		} else {
			confirmButton.Disable()
		}
	}
	labelEntry.OnChanged = func(string) { refreshState() }
	outboundSelect.OnChanged = func(string) { refreshState() }
	rawTabEntry.SetOnChanged(func(string) { refreshState() })

	saveRule := func() {
		label := strings.TrimSpace(labelEntry.Text)
		if label == "" {
			dialog.ShowError(errors.New(locale.T("Rule name is required")), dialogWindow)
			return
		}
		// Имя — источник идентификатора правила (state.StableRuleID): на нём
		// держатся порядок и сопоставление слотов. Два правила с одним
		// идентификатором — отказ. Имя, которое не меняли, не проверяется:
		// старое состояние с дублем остаётся редактируемым.
		if editRule == nil || label != strings.TrimSpace(editRule.Rule.Label) {
			if other := conflictingRuleName(model.CustomRules, editRule, label); other != "" {
				dialog.ShowError(errors.New(locale.Tf("The name conflicts with the rule %q — choose another one.", other)), dialogWindow)
				return
			}
		}
		match, err := currentMatch(true)
		if err != nil {
			dialog.ShowError(err, dialogWindow)
			return
		}
		if len(match) == 0 {
			dialog.ShowError(errors.New(locale.T(addRuleNoConditionText)), dialogWindow)
			return
		}
		outbound := selectedOutbound()
		ruleRaw := ruleWithTarget(match, outbound)
		ruleSets := wizardmodels.RuleSetsFromBody(ruleRaw)
		var srsOptions corestate.RuleSetOptions
		if len(ruleSets) > 0 {
			srsOptions = srsOptionsForm.Value()
		}
		ruleDNS := dnsTab.value()

		if isEdit {
			editRule.DNS = ruleDNS
			editRule.SrsOptions = srsOptions
			editRule.Rule.Label = label
			editRule.Rule.Rule = ruleRaw
			editRule.Rule.HasOutbound = true
			editRule.Rule.DefaultOutbound = outbound
			editRule.Rule.Params = nil
			editRule.Rule.RuleSets = ruleSets
			editRule.SelectedOutbound = outbound
		} else {
			newRule := &wizardmodels.RuleState{
				Rule: wizardtemplate.TemplateSelectableRule{
					Label:           label,
					Rule:            ruleRaw,
					RuleSets:        ruleSets,
					HasOutbound:     true,
					DefaultOutbound: outbound,
					IsDefault:       true,
				},
				Enabled:          true,
				SelectedOutbound: outbound,
				// SPEC 106: номер оси — конец занятой пользовательской зоны.
				Num:        wizardmodels.NextRuleNum(model),
				DNS:        ruleDNS,
				SrsOptions: srsOptions,
			}
			model.CustomRules = append(model.CustomRules, newRule)
		}

		presenter.MarkAsChanged()
		if createRulesTab != nil {
			presenter.RefreshRulesTab(createRulesTab)
		}
		delete(openDialogs, dialogKey)
		updateChildOverlay()
		dialogWindow.Close()
	}

	confirmBtnText := locale.T("Add")
	if isEdit {
		confirmBtnText = locale.T("Save")
	}
	confirmButton = widget.NewButton(confirmBtnText, saveRule)
	confirmButton.Importance = widget.HighImportance
	cancelButton := widget.NewButton(locale.T("Cancel"), func() {
		delete(openDialogs, dialogKey)
		updateChildOverlay()
		dialogWindow.Close()
	})

	ruleNameBlock := container.NewVBox(widget.NewLabel(locale.T("Rule Name:")), wheelEntry(ctx.wheel, labelEntry))
	formContent := container.NewVBox(
		formHolder,
		widget.NewSeparator(),
		widget.NewLabel(locale.T("Direction:")),
		outboundSelect,
		srsOptionsRow,
	)
	formScroll := components.WrapInScrollWithGutter(formContent)
	ctx.wheel.SetScroll(formScroll)
	dnsScroll := components.WrapInScrollWithGutter(dnsTab.box)
	dnsWheel.SetScroll(dnsScroll)
	formTabItem := container.NewTabItem(locale.T("Form"), formScroll)
	rawTabItem := container.NewTabItem(locale.T("JSON"), components.WrapInScrollWithGutter(rawTabEntry.Object()))
	dnsTabItem := container.NewTabItem(locale.T("DNS"), dnsScroll)

	// Preview — что правило даёт в конфиге, тем же кодом, что сборка.
	previewView := fynewidget.NewJSONView("")
	previewContent := container.NewBorder(
		container.NewHBox(layout.NewSpacer(), widget.NewLabel("🔒")),
		nil, nil, nil,
		container.NewScroll(previewView.Object()),
	)
	previewTabItem := container.NewTabItem(locale.T("Preview"), previewContent)
	refreshPreview := func() {
		previewView.SetText(rulePreviewText(model, rulePreviewInput{
			label:    strings.TrimSpace(labelEntry.Text),
			outbound: selectedOutbound(),
			body:     currentBody(),
			dns:      dnsTab.value(),
			srs:      srsOptionsForm.Value(),
			editing:  editRule,
		}))
	}

	tabs := container.NewAppTabs(formTabItem, dnsTabItem, rawTabItem, previewTabItem)
	// JSON-вкладка показывает тело записи (с `refs`); теги наборов — в Preview.
	setRawText := func(match map[string]interface{}) {
		if b, e := json.MarshalIndent(ruleWithTarget(match, selectedOutbound()), "", "  "); e == nil {
			rawTabEntry.SetText(string(b))
		}
	}
	syncFormToRaw := func() {
		if m, e := formMatch(false); e == nil {
			setRawText(m)
		}
	}
	// syncRawToForm — тело JSON в форму; false — форма его не выражает или
	// JSON не разбирается: окно остаётся на вкладке JSON.
	// askDiscardRaw — с вкладки JSON не уйти (ошибка или форма не выражает):
	// выбор «отбросить правки JSON» (тело — снова из формы, переход на
	// вкладку target) или «остаться в редакторе JSON».
	askDiscardRaw := func(title, msg string, target *container.TabItem) {
		text := widget.NewLabel(msg + "\n\n" + locale.T("Discard the JSON changes and go back to the form?"))
		text.Wrapping = fyne.TextWrapWord
		d := dialog.NewCustomConfirm(title, locale.T("Discard"), locale.T("Keep editing"), text, func(discard bool) {
			if !discard {
				return
			}
			activeTabIsRaw = false
			syncFormToRaw()
			tabs.Select(target)
		}, dialogWindow)
		d.Resize(fyne.NewSize(440, 220))
		d.Show()
	}
	syncRawToForm := func() bool {
		match, e := rawMatch()
		if e != nil {
			askDiscardRaw(locale.T("JSON error"), e.Error(), formTabItem)
			return false
		}
		st, ok := table.StackFromBody(match)
		if !ok {
			askDiscardRaw(locale.T("Rule not recognized"), locale.T("The form cannot show this rule."), formTabItem)
			return false
		}
		loadForm(st.ForForm(), match)
		var obj map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimSpace(rawTabEntry.Text())), &obj) == nil {
			if ob := ruleTargetName(obj); ob != "" && outboundMap[ob] {
				outboundSelect.SetSelected(ob)
			}
		}
		return true
	}
	// activeTabIsRaw — последняя из вкладок Form/JSON: вкладка DNS и Preview
	// её не меняют, тело правила берётся оттуда. Синхронизация — только при
	// переходе между Form и JSON.
	tabs.OnSelected = func(t *container.TabItem) {
		switch t {
		case rawTabItem:
			if !activeTabIsRaw {
				activeTabIsRaw = true
				syncFormToRaw()
			}
		case formTabItem:
			if activeTabIsRaw {
				if syncRawToForm() {
					activeTabIsRaw = false
				} else {
					tabs.Select(rawTabItem)
					return
				}
			}
		case dnsTabItem, previewTabItem:
			// Тело для DNS и Preview берётся из JSON — с ошибкой туда не пускаем.
			if activeTabIsRaw {
				if _, e := rawMatch(); e != nil {
					askDiscardRaw(locale.T("JSON error"), e.Error(), t)
					tabs.Select(rawTabItem)
					return
				}
			}
			if t != previewTabItem {
				break
			}
			refreshPreview()
		}
		refreshState()
	}

	if formOK {
		syncFormToRaw()
	} else if b, e := json.MarshalIndent(initialBody, "", "  "); e == nil {
		rawTabEntry.SetText(string(b))
	}

	mainContent := container.NewBorder(
		ruleNameBlock,
		container.NewBorder(nil, nil, nil, container.NewHBox(cancelButton, confirmButton), confirmHint),
		nil, nil,
		tabs,
	)

	dialogWindow = app.NewWindow(dialogTitle)
	ctx.win = dialogWindow
	dialogWindow.Resize(fyne.NewSize(560, 700))
	fynewidget.CenterOnScreen(dialogWindow)
	dialogWindow.SetContent(mainContent)
	if activeTabIsRaw {
		tabs.Select(rawTabItem)
	}

	openDialogs[dialogKey] = dialogWindow
	updateChildOverlay()
	dialogWindow.SetCloseIntercept(func() {
		delete(openDialogs, dialogKey)
		updateChildOverlay()
		dialogWindow.Close()
	})

	refreshState()
	dialogWindow.Show()
}

// ruleMatchOf — тело без цели корня (`outbound`, `action`, `method`), копией
// верхнего уровня.
func ruleMatchOf(body map[string]interface{}) map[string]interface{} {
	if body == nil {
		return nil
	}
	out := make(map[string]interface{}, len(body))
	for k, v := range body {
		if !corestate.IsRuleTargetKey(k) {
			out[k] = v
		}
	}
	return out
}

// ruleWithTarget — копия тела с целью корня по выбору Direction.
func ruleWithTarget(match map[string]interface{}, outbound string) map[string]interface{} {
	out := make(map[string]interface{}, len(match)+1)
	for k, v := range match {
		out[k] = v
	}
	return outboundutil.ApplyOutboundToRule(out, outbound)
}

// ruleTargetName — выбор Direction по цели тела: outbound, иначе действие
// (reject; reject + method drop — drop).
func ruleTargetName(body map[string]interface{}) string {
	if ob, _ := body["outbound"].(string); ob != "" {
		return ob
	}
	if m, _ := body["method"].(string); m != "" {
		return m
	}
	a, _ := body["action"].(string)
	return a
}

// ruleHasNestedTarget — цель (outbound / action / method) стоит в
// под-правиле группы: цель — только у корня (SPEC 188 §2).
func ruleHasNestedTarget(body map[string]interface{}) bool {
	found := false
	corestate.WalkRuleTree(body, func(path []int, node map[string]interface{}, _ bool) bool {
		if len(path) == 0 {
			return true
		}
		for k := range node {
			if corestate.IsRuleTargetKey(k) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// cloneRuleMap — глубокая копия тела (значения JSON).
func cloneRuleMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// ruleArrayPastedError — вставлен МАССИВ правил, а запись правила держит
// ровно одно правило sing-box (норма «одно правило — одно тело», D-111,
// contract/docs/BACKUP.md §2). Раскладывать массив на записи умеет только
// импорт бэкапа; редактор одну запись в несколько не превращает, а без этой
// проверки массив доходил до разбора в объект и отвечал сырой ошибкой
// encoding/json. nil — вставлено не массивом.
func ruleArrayPastedError(trimmed string) error {
	if !strings.HasPrefix(trimmed, "[") {
		return nil
	}
	return errors.New(locale.T(addRuleArrayPastedText))
}

// conflictingRuleName — имя другого пользовательского правила с тем же
// идентификатором (state.StableRuleID), что дало бы имя label; пусто — конфликта
// нет. self — редактируемое правило, оно не сравнивается с собой.
func conflictingRuleName(rules []*wizardmodels.RuleState, self *wizardmodels.RuleState, label string) string {
	id := corestate.StableRuleID(corestate.Rule{Kind: corestate.RuleKindInline, Name: label})
	for _, rs := range rules {
		if rs == nil || rs == self {
			continue
		}
		other := strings.TrimSpace(rs.Rule.Label)
		if other != "" && corestate.StableRuleID(corestate.Rule{Kind: corestate.RuleKindInline, Name: other}) == id {
			return other
		}
	}
	return ""
}
