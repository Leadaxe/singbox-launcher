package tabs

import (
	"encoding/json"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// isRemote — конфиг готовится для другой машины. От этого зависит, где
// показывать target-переменные: у удалённой машины есть своя вкладка Target,
// у локальной её нет вовсе, и раздача в LAN настраивается здесь.
func settingsVarVisible(v wizardtemplate.TemplateVar, goos string, isRemote bool) bool {
	ui := strings.ToLower(strings.TrimSpace(v.WizardUI))
	if ui == "hidden" || ui == "fix" {
		return false
	}
	// SPEC 097: "target"-vars (gateway_mode и LAN-интерфейсы) рендерит вкладка
	// Target — но она есть только у удалённой машины. В локальном режиме
	// вкладки нет, и эти поля показываются здесь, иначе раздача в LAN стала бы
	// недоступна вовсе.
	//
	// Порядок при этом сохраняется: от gateway_mode зависят default_value
	// других полей, а дефолты резолвятся однопроходно сверху вниз — в шаблоне
	// эти переменные объявлены первыми, поэтому и в списке Settings они выше
	// зависимых.
	if ui == wizardtemplate.WizardUITarget && isRemote {
		return false
	}
	if len(v.Platforms) == 0 {
		return true
	}
	for _, p := range v.Platforms {
		if p == goos {
			return true
		}
	}
	return false
}

// noInboundConfigured — true when neither TUN nor mixed-proxy inbound is
// effectively enabled (after resolver applies state + template defaults).
// Triggers the SPEC 066 follow-up warning row in Settings tab.
//
// "false" / missing / non-"true" all count as off.
func noInboundConfigured(resolved map[string]wizardtemplate.ResolvedVar) bool {
	tunOn := resolved["tun"].Scalar == "true"
	proxyOn := resolved["enable_proxy_in"].Scalar == "true"
	return !tunOn && !proxyOn
}

// buildNoInboundWarningRow — orange ⚠ banner explaining the trap state.
// Standalone row (no Reset button, no value), so it slots after the last
// var-row regardless of layout. Locale key: wizard.settings.no_inbound_warning.
func buildNoInboundWarningRow() fyne.CanvasObject {
	lbl := widget.NewLabel(locale.T("wizard.settings.no_inbound_warning"))
	lbl.Wrapping = fyne.TextWrapWord
	lbl.Importance = widget.WarningImportance
	return container.NewPadded(lbl)
}

func enumListContains(opts []string, v string) bool {
	for _, o := range opts {
		if o == v {
			return true
		}
	}
	return false
}

// templateVarUsedInAnotherVarConditional: имя bool-переменной в if/if_or
// ИЛИ в default_value.#if другой var — после её смены нужно пересобрать
// Settings.
//
// SPEC 097 добавил вторую форму зависимости: default_value может ветвиться
// по другой var (gateway_mode → proxy_in_listen). Без учёта этого случая
// поле сохраняло дефолт, посчитанный до переключения галки.
func templateVarUsedInAnotherVarConditional(td *wizardtemplate.TemplateData, name string) bool {
	if td == nil {
		return false
	}
	ref := "@" + name
	for _, v := range td.Vars {
		if defaultValueMentionsVar(v.DefaultValue, ref) {
			return true
		}
		// If/IfOr entries are canonical "@name" (SPEC 067 Phase 3); changedName is
		// the bare var name. Strip the @ before comparing, else the refresh trigger
		// never fires and dependent rows stay frozen on toggle.
		for _, x := range v.If {
			if strings.TrimPrefix(x, "@") == name {
				return true
			}
		}
		for _, x := range v.IfOr {
			if strings.TrimPrefix(x, "@") == name {
				return true
			}
		}
	}
	return false
}

// defaultValueMentionsVar — ссылается ли default_value этой var на @ref
// (в любой ветке #if-дерева). Сравнение по сериализованному дереву: точность
// «ссылка есть где-то внутри» здесь достаточна — цена ложного срабатывания
// это одна лишняя перерисовка, цена пропуска — залипшее значение в UI.
func defaultValueMentionsVar(dv wizardtemplate.VarDefaultValue, ref string) bool {
	if len(dv.PerPlatform) == 0 {
		return false
	}
	raw, err := json.Marshal(dv.PerPlatform)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), `"`+ref+`"`)
}

func maybeRefreshSettingsAfterVarChange(gs *wizardpresentation.GUIState, td *wizardtemplate.TemplateData, changedName string) {
	if !templateVarUsedInAnotherVarConditional(td, changedName) {
		return
	}
	if gs.RefreshSettingsFromModel != nil {
		gs.RefreshSettingsFromModel()
	}
	// В удалённом режиме те же vars (gateway_mode + LAN-интерфейсы) живут на
	// вкладке Target — её тоже надо пересобрать, иначе поле include_interface
	// не разблокируется сразу после установки галки. В локальном режиме
	// вкладки нет, и хук пуст: поля уже обновлены выше, вместе с Settings.
	if gs.RefreshTargetTabFromModel != nil {
		gs.RefreshTargetTabFromModel()
	}
}

// dnsTabOwnedVar true для dns_* var, чьё «живое» значение во время сессии
// визарда держит не model.SettingsVars, а отдельное зеркальное поле модели
// (model.DNSStrategy / DNSFinal / DefaultDomainResolver), которое рисует
// DNS-вкладка и которое ПЕРЕЗАПИСЫВАЕТ SettingsVars перед сборкой
// (SyncDNSModelToSettingsVars, dns_settings_vars.go:97-110, вызывается из
// buildConfigFromModel). Если on_change поменял dns_strategy/dns_final/
// dns_default_domain_resolver только в SettingsVars, эта запись потеряется
// молча на первой же сборке конфига — правку обязаны увидеть И зеркальное
// поле, И DNS-вкладка.
func dnsTabOwnedVar(name string) bool {
	switch name {
	case wizardmodels.VarDNSStrategy, wizardmodels.VarDNSFinal, wizardmodels.VarDNSDefaultDomainResolver:
		return true
	default:
		return false
	}
}

// syncDNSMirrorFieldFromSettingsVars переносит on_change-запись из
// model.SettingsVars[name] в соответствующее зеркальное поле DNS-вкладки,
// иначе refreshDNSSelectsFromModel (presenter_sync.go:190) увидит старое
// model.DNSStrategy и не даст новому значению даже дойти до экрана.
func syncDNSMirrorFieldFromSettingsVars(model *wizardmodels.WizardModel, name string) {
	v, ok := model.SettingsVars[name]
	if !ok {
		return
	}
	switch name {
	case wizardmodels.VarDNSStrategy:
		model.DNSStrategy = strings.TrimSpace(v)
	case wizardmodels.VarDNSFinal:
		model.DNSFinal = strings.TrimSpace(v)
	case wizardmodels.VarDNSDefaultDomainResolver:
		model.DefaultDomainResolver = strings.TrimSpace(v)
		model.DefaultDomainResolverUnset = false
	}
}

// applyOnChangeAndRefresh — паритет с mobile _onVarChanged (settings_screen.dart:
// 247-250; SPEC 103). Вызывается ПОСЛЕ того, как виджет уже записал новое
// значение в model.SettingsVars[name]: применяет декларативный каскад
// on_change.set (ApplyOnChange, core/template/on_change.go) и, если он
// реально что-то поменял, форсит перерисовку Settings/Target/DNS — соседние
// поля (например dns_strategy/resolve_strategy от ipv6_enabled) могли
// поменять значение независимо от templateVarUsedInAnotherVarConditional
// (та проверка знает только про if/if_or/default_value-зависимости,
// on_change — отдельный, декларативный канал).
func applyOnChangeAndRefresh(presenter *wizardpresentation.WizardPresenter, td *wizardtemplate.TemplateData, model *wizardmodels.WizardModel, changedName string) {
	if td == nil || model == nil || presenter == nil {
		return
	}
	touched := wizardtemplate.ApplyOnChange(changedName, td.Vars, model.SettingsVars, model.Target.Normalized())

	// SPEC 107: батч = САМА изменённая переменная + цели каскада on_change.
	//
	// ApplyOnChange возвращает только цели, поэтому без явного добавления
	// changedName поля, гейт которых зависит от неё, не пересчитывались:
	// «Enable IPv6» включён, а «TUN IPv6 address» остаётся приглушённым.
	// Ранний выход при пустом каскаде имел тот же эффект для любой
	// переменной БЕЗ on_change — а таких гейтов большинство (@tun,
	// @gateway_mode).
	changed := append([]string{changedName}, touched...)
	needDNSRefresh := false
	for _, name := range touched {
		if dnsTabOwnedVar(name) {
			syncDNSMirrorFieldFromSettingsVars(model, name)
			needDNSRefresh = true
		}
	}
	gs := presenter.GUIState()
	if gs == nil {
		return
	}
	// SPEC 107 §8.3: пересчитываем ТОЛЬКО строки, подписанные на изменившиеся
	// переменные, и обновляем лишь те, у кого результат гейта реально
	// поменялся. Полная пересборка вкладки остаётся для смены шаблона или
	// таргета (RefreshSettingsFromModel) — там меняется сам набор строк.
	//
	// `touched` — уже батч (изменённая var + цели каскада on_change), поэтому
	// строка, зависящая от двух переменных каскада, пересчитается один раз.
	if !recomputeSettingsGates(gs, td, model, changed) && gs.RefreshSettingsFromModel != nil {
		// Индекса нет (вкладка ещё не собрана) — падаем на прежний путь.
		gs.RefreshSettingsFromModel()
	}
	if gs.RefreshTargetTabFromModel != nil {
		gs.RefreshTargetTabFromModel()
	}
	if needDNSRefresh {
		presenter.RefreshDNSDependentSelectsOnly()
	}
}

// labels — подписи строки: их приглушение и есть главный видимый признак
// того, что поле неактивно. Само по себе Disable() у Entry в тёмной теме
// почти не меняет вид пустого поля — строка выглядит рабочей, и связь
// «галка → зависимое поле» не читается (наблюдалось на паре
// gateway_mode → LAN interfaces).
func applySettingsRowDisabled(rowEnabled bool, resetBtn *ttwidget.Button, extras ...fyne.Disableable) {
	setRowEnabled(rowEnabled, resetBtn, extras...)
}

// setRowEnabled приводит виджеты строки к состоянию гейта. В отличие от
// прежней версии умеет и ВКЛЮЧАТЬ: реактивный пересчёт (SPEC 107) вызывает её
// в обе стороны, а не только при первичной сборке.
func setRowEnabled(rowEnabled bool, resetBtn *ttwidget.Button, extras ...fyne.Disableable) {
	if rowEnabled {
		if resetBtn != nil {
			resetBtn.Enable()
		}
		for _, x := range extras {
			if x != nil {
				x.Enable()
			}
		}
		return
	}
	if resetBtn != nil {
		resetBtn.Disable()
	}
	for _, x := range extras {
		if x != nil {
			x.Disable()
		}
	}
}

// bindRowGate подписывает строку на изменения переменных её гейта
// (SPEC 107 §8.2). Строка без гейта не подписывается.
//
// titleLab приглушается вместе с блокировкой (D-063): у пустого Entry в
// тёмной теме Disable() почти не виден, и связь «галка → зависимое поле»
// иначе не читается.
func bindRowGate(
	gs *wizardpresentation.GUIState,
	vd wizardtemplate.TemplateVar,
	rowEnabled bool,
	titleLab *ttwidget.Label,
	resetBtn *ttwidget.Button,
	extras ...fyne.Disableable,
) {
	if gs == nil || gs.SettingsGates == nil {
		return
	}
	idx, ok := gs.SettingsGates.(*gateIndex)
	if !ok {
		return
	}
	idx.subscribe(vd, rowEnabled, func(enabled bool) {
		setRowEnabled(enabled, resetBtn, extras...)
		if titleLab != nil {
			if enabled {
				titleLab.Importance = widget.MediumImportance
			} else {
				titleLab.Importance = widget.LowImportance
			}
			titleLab.Refresh()
		}
	})
}

func newSettingsTitleLabel(text string) *ttwidget.Label {
	l := ttwidget.NewLabel(text)
	// В container.NewBorder лейбл в позиции leading получает свою MinSize; при TextWrapWord
	// при узкой колонке MinWidth схлопывается, текст уезжает столбиком по символам.
	l.Wrapping = fyne.TextWrapOff
	return l
}

// newSettingsTitleLabelFor — подпись строки, приглушённая когда строка
// неактивна. Приглушение подписи и есть главный видимый признак блокировки:
// Disable() у пустого Entry в тёмной теме почти не меняет вид, строка
// выглядит рабочей, и связь «галка → зависимое поле» не читается
// (наблюдалось на паре gateway_mode → LAN interfaces).
func newSettingsTitleLabelFor(text string, rowEnabled bool) *ttwidget.Label {
	l := newSettingsTitleLabel(text)
	if !rowEnabled {
		l.Importance = widget.LowImportance
	}
	return l
}

// settingsSeparatorBlock — горизонтальная линия между строками Settings (vars.separator).
// Цвет InputBorder заметнее стандартного theme.Separator в тёмной теме; сверху/снизу — отступ.
func settingsSeparatorBlock() fyne.CanvasObject {
	gap := float32(theme.InnerPadding()) / 2
	if gap < 6 {
		gap = 6
	}
	top := canvas.NewRectangle(color.Transparent)
	top.SetMinSize(fyne.NewSize(1, gap))
	bot := canvas.NewRectangle(color.Transparent)
	bot.SetMinSize(fyne.NewSize(1, gap))

	var lineCol color.Color = color.Gray{Y: 0x55}
	if app := fyne.CurrentApp(); app != nil {
		lineCol = app.Settings().Theme().Color(theme.ColorNameInputBorder, app.Settings().ThemeVariant())
	}
	line := canvas.NewRectangle(lineCol)
	line.SetMinSize(fyne.NewSize(1, 2))
	return container.NewVBox(top, line, bot)
}

func setVarFieldToolTip(tip string, widgets ...fyne.CanvasObject) {
	tip = strings.TrimSpace(tip)
	if tip == "" {
		return
	}
	for _, o := range widgets {
		if o == nil {
			continue
		}
		fynewidget.SetToolTipSafe(o, tip)
	}
}

// CreateSettingsTab строит вкладку Settings из wizard_template.json vars.
func CreateSettingsTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	model := presenter.Model()
	gs := presenter.GUIState()
	box := container.NewVBox()

	refresh := func() {
		// SPEC 097: платформа ЦЕЛЕВОЙ машины, не той, где запущен лаунчер.
		// Для local Target нормализуется в runtime.GOOS, так что поведение
		// локального визарда не меняется.
		tgt := model.Target.Normalized()
		goos := tgt.GOOS
		box.RemoveAll()
		if model.TemplateData == nil || len(model.TemplateData.Vars) == 0 {
			box.Add(widget.NewLabel(locale.T("wizard.settings.no_vars")))
			box.Refresh()
			return
		}
		td := model.TemplateData
		vi := wizardtemplate.VarIndex(td.Vars)
		resolved := wizardtemplate.ResolveTemplateVarsFor(td.Vars, model.SettingsVars, td.RawTemplate, tgt)
		// SPEC 107: индекс гейтов пересобирается вместе со строками — он
		// производный от текущего набора виджетов и живёт ровно столько же.
		gs.SettingsGates = newGateIndex()
		for _, vd := range td.Vars {
			if !settingsVarVisible(vd, goos, tgt.IsRemote()) {
				continue
			}
			if vd.Separator {
				box.Add(settingsSeparatorBlock())
				continue
			}
			// Условие, зависящее ТОЛЬКО от таргета/платформы (без ссылок на
			// другие vars), означает «этого поля для такой машины не
			// существует» — строку не рисуем вовсе. Условие, зависящее от
			// других vars (@tun), лишь ГАСИТ строку: пользователь может
			// включить переключатель выше и разблокировать её.
			if wizardtemplate.VarConditionIsTargetOnly(vd) &&
				!wizardtemplate.VarUISatisfiedFor(vd, vi, resolved, tgt) {
				continue
			}
			title := wizardtemplate.VarDisplayTitle(vd)
			toolTip := wizardtemplate.VarDisplayTooltip(vd)
			rowEnabled := wizardtemplate.VarUISatisfiedFor(vd, vi, resolved, tgt)
			row := buildSettingsVarRow(presenter, model, td, vd, title, toolTip, rowEnabled, gs)
			box.Add(row)
		}
		// SPEC 066 follow-up: trap-state warning. After SPEC 066 made `tun`
		// user-disableable on Win/Linux, the combination tun=false +
		// enable_proxy_in=false is reachable from the UI → sing-box would
		// start with zero inbounds → no traffic ever reaches it, silently.
		// Show a soft warning row when both resolve to false (we don't hard-
		// block, so power users can still test weird configs).
		if noInboundConfigured(resolved) {
			box.Add(buildNoInboundWarningRow())
		}
		box.Refresh()
	}
	gs.RefreshSettingsFromModel = refresh
	refresh()

	scroll := container.NewVScroll(box)
	scroll.SetMinSize(adaptiveScrollSize(gs, 0.5, 400))

	// Бэкап переехал на вкладку «Generate»: прибитый к низу через Border, он
	// забирал свою высоту целиком, и прокрутке настроек доставался остаток —
	// нижние строки обрезались тем сильнее, чем уже окно.
	return scroll
}

func buildSettingsVarRow(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, vd wizardtemplate.TemplateVar, title, toolTip string, rowEnabled bool, gs *wizardpresentation.GUIState) fyne.CanvasObject {
	name := vd.Name
	typ := vd.Type
	// Options carry actual values for substitution. Object-form options
	// (`[{title, value}]`) are normalized to `type:"enum"` at unmarshal
	// time (see TemplateVar.UnmarshalJSON), so:
	//   - enum branch may have title != value and uses titleForValue /
	//     valueForTitle to map dropdown picks back to values;
	//   - text branch only ever sees plain-string options (title==value),
	//     so no title↔value mapping is needed there.
	options := vd.Options
	viewMode := strings.EqualFold(strings.TrimSpace(vd.WizardUI), "view")
	// SPEC 097: значения полей резолвятся для ТАРГЕТА модели — иначе строка
	// показывала бы local-дефолт (singbox-tun0) там, где в конфиг уедет
	// remote-значение (lxd-tun0).
	rowTarget := model.Target.Normalized()

	st := model.SettingsVars
	raw := td.RawTemplate
	vars := td.Vars

	if strings.EqualFold(strings.TrimSpace(typ), "secret") {
		return buildSettingsSecretRow(presenter, model, td, vd, title, toolTip, viewMode, rowEnabled, gs)
	}

	reset := func() {
		delete(model.SettingsVars, name)
		presenter.MarkAsChanged()
		if presenter.GUIState().RefreshSettingsFromModel != nil {
			presenter.GUIState().RefreshSettingsFromModel()
		}
	}

	resetBtn := ttwidget.NewButtonWithIcon("", theme.ContentUndoIcon(), reset)
	resetBtn.Importance = widget.LowImportance
	resetBtn.SetToolTip(locale.T("wizard.settings.reset_tooltip"))

	if viewMode {
		disp := strings.TrimSpace(wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget))
		if typ == "bool" {
			if disp != "true" && disp != "false" {
				disp = "false"
			}
		}
		valLab := ttwidget.NewLabel(disp)
		valLab.Wrapping = fyne.TextWrapWord
		titleLab := newSettingsTitleLabelFor(title, rowEnabled)
		row := container.NewBorder(nil, nil, titleLab, resetBtn, valLab)
		setVarFieldToolTip(toolTip, titleLab, valLab)
		applySettingsRowDisabled(rowEnabled, resetBtn)
		bindRowGate(gs, vd, rowEnabled, titleLab, resetBtn)
		return row
	}

	switch typ {
	case "bool":
		var prog bool
		var chkForDarwin *widget.Check
		titleLbl := newSettingsTitleLabelFor(title, rowEnabled)
		onChanged := func(checked bool) {
			if prog {
				return
			}
			if !checked {
				if maybeTunOffDarwin(presenter, model, td, name, chkForDarwin) {
					return
				}
			}
			if checked {
				model.SettingsVars[name] = "true"
			} else {
				model.SettingsVars[name] = "false"
			}
			presenter.MarkAsChanged()
			applyOnChangeAndRefresh(presenter, td, model, name)
			maybeRefreshSettingsAfterVarChange(gs, td, name)
		}
		cwc := fynewidget.NewCheckWithContent(onChanged, titleLbl, fynewidget.CheckWithContentConfig{})
		chk := cwc.Check
		chkForDarwin = chk
		prog = true
		v, overridden := model.SettingsVars[name]
		checked := strings.TrimSpace(wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget)) == "true"
		if overridden {
			checked = v == "true"
		}
		chk.SetChecked(checked)
		prog = false
		row := container.NewBorder(nil, nil, cwc.CheckLeading, resetBtn, cwc.Content)
		setVarFieldToolTip(toolTip, titleLbl, chk)
		applySettingsRowDisabled(rowEnabled, resetBtn, chk)
		bindRowGate(gs, vd, rowEnabled, titleLbl, resetBtn, chk)
		return row

	case "enum":
		titleLab := newSettingsTitleLabelFor(title, rowEnabled)
		// Object-form options surface display titles distinct from values;
		// legacy string-list form sets title == value. Map both directions
		// for the dropdown.
		optionTitles := make([]string, len(options))
		for i := range options {
			optionTitles[i] = vd.OptionTitle(i)
		}
		valueForTitle := func(t string) string {
			for i, ot := range optionTitles {
				if ot == t {
					return options[i]
				}
			}
			return t
		}
		titleForValue := func(val string) string {
			for i, v := range options {
				if v == val {
					return optionTitles[i]
				}
			}
			return val
		}
		sel := widget.NewSelect(optionTitles, func(pickedTitle string) {
			model.SettingsVars[name] = valueForTitle(pickedTitle)
			presenter.MarkAsChanged()
			applyOnChangeAndRefresh(presenter, td, model, name)
			maybeRefreshSettingsAfterVarChange(gs, td, name)
		})
		disp := wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget)
		if _, ok := model.SettingsVars[name]; ok {
			disp = model.SettingsVars[name]
		}
		if len(options) > 0 && !enumListContains(options, disp) {
			disp = options[0]
			if model.SettingsVars[name] != disp {
				model.SettingsVars[name] = disp
				presenter.MarkAsChanged()
			}
		}
		sel.SetSelected(titleForValue(disp))
		row := container.NewBorder(nil, nil, titleLab, resetBtn, sel)
		setVarFieldToolTip(toolTip, titleLab, sel)
		applySettingsRowDisabled(rowEnabled, resetBtn, sel)
		bindRowGate(gs, vd, rowEnabled, titleLab, resetBtn, sel)
		return row

	case "text_list":
		titleLab := newSettingsTitleLabelFor(title, rowEnabled)
		e := widget.NewMultiLineEntry()
		e.SetMinRowsVisible(3)
		disp := wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget)
		if v, ok := model.SettingsVars[name]; ok {
			disp = v
		}
		e.SetText(disp)
		e.OnChanged = func(s string) {
			model.SettingsVars[name] = s
			presenter.MarkAsChanged()
			applyOnChangeAndRefresh(presenter, td, model, name)
		}
		row := container.NewBorder(nil, nil, titleLab, resetBtn, e)
		setVarFieldToolTip(toolTip, titleLab, e)
		applySettingsRowDisabled(rowEnabled, resetBtn, e)
		bindRowGate(gs, vd, rowEnabled, titleLab, resetBtn, e)
		return row

	default: // text
		titleLab := newSettingsTitleLabelFor(title, rowEnabled)
		disp := wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget)
		if v, ok := model.SettingsVars[name]; ok {
			disp = v
		}
		onChanged := func(s string) {
			model.SettingsVars[name] = s
			presenter.MarkAsChanged()
			applyOnChangeAndRefresh(presenter, td, model, name)
		}
		// `type:"text"` + options always means plain-string options
		// (title==value): object-form options force the var to enum at
		// unmarshal time. So the SelectEntry combo can use options
		// directly without any title↔value mapping — what the user sees
		// in the dropdown is what gets substituted.
		if len(options) > 0 {
			se := widget.NewSelectEntry(options)
			se.SetText(disp)
			se.OnChanged = onChanged
			row := container.NewBorder(nil, nil, titleLab, resetBtn, se)
			setVarFieldToolTip(toolTip, titleLab, se)
			applySettingsRowDisabled(rowEnabled, resetBtn, se)
			bindRowGate(gs, vd, rowEnabled, titleLab, resetBtn, se)
			return row
		}
		e := widget.NewEntry()
		e.SetText(disp)
		e.OnChanged = onChanged
		row := container.NewBorder(nil, nil, titleLab, resetBtn, e)
		setVarFieldToolTip(toolTip, titleLab, e)
		applySettingsRowDisabled(rowEnabled, resetBtn, e)
		bindRowGate(gs, vd, rowEnabled, titleLab, resetBtn, e)
		return row
	}
}

// buildSettingsSecretRow renders any type:"secret" var uniformly: a masked
// password field (Fyne PasswordEntry → dots + built-in show/hide eye toggle),
// a regenerate button, and always-prefilled behaviour — when the row is active
// and the value is empty/placeholder, a random secret is generated and
// persisted (same generator as clash_secret). All secrets behave identically.
func buildSettingsSecretRow(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, vd wizardtemplate.TemplateVar, title, toolTip string, viewMode bool, rowEnabled bool, gs *wizardpresentation.GUIState) fyne.CanvasObject {
	name := vd.Name
	st := model.SettingsVars
	raw := td.RawTemplate
	vars := td.Vars
	rowTarget := model.Target.Normalized()

	titleLab := newSettingsTitleLabelFor(title, rowEnabled)

	disp := wizardtemplate.DisplaySettingValueFor(vars, st, raw, name, rowTarget)
	if v, ok := model.SettingsVars[name]; ok {
		disp = v
	}
	// Always pre-filled: generate + persist a value when the row is active and
	// the secret is empty/placeholder. Gated on rowEnabled so disabled (if-gated)
	// rows don't spawn secrets until their condition is met.
	if rowEnabled && !viewMode && wizardtemplate.SecretUnresolved(disp) {
		if gen, err := wizardtemplate.GenerateSecret(); err == nil {
			if model.SettingsVars == nil {
				model.SettingsVars = make(map[string]string)
			}
			model.SettingsVars[name] = gen
			disp = gen
			presenter.MarkAsChanged()
		} else {
			debuglog.WarnLog("settings_tab: GenerateSecret prefill %q: %v", name, err)
		}
	}

	e := widget.NewPasswordEntry() // masked dots + built-in reveal (eye) toggle
	e.SetText(disp)
	e.OnChanged = func(s string) {
		model.SettingsVars[name] = s
		presenter.MarkAsChanged()
		applyOnChangeAndRefresh(presenter, td, model, name)
	}
	if viewMode {
		e.Disable()
	}

	regenerate := func() {
		if model.SettingsVars == nil {
			model.SettingsVars = make(map[string]string)
		}
		gen, err := wizardtemplate.GenerateSecret()
		if err != nil {
			debuglog.WarnLog("settings_tab: GenerateSecret: %v", err)
			delete(model.SettingsVars, name)
		} else {
			model.SettingsVars[name] = gen
		}
		presenter.MarkAsChanged()
		applyOnChangeAndRefresh(presenter, td, model, name)
		if presenter.GUIState().RefreshSettingsFromModel != nil {
			presenter.GUIState().RefreshSettingsFromModel()
		}
	}
	regenBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), regenerate)
	regenBtn.Importance = widget.LowImportance
	regenBtn.SetToolTip(locale.T("wizard.settings.clash_secret_regenerate_tooltip"))

	row := container.NewBorder(nil, nil, titleLab, regenBtn, e)
	setVarFieldToolTip(toolTip, titleLab, e)
	applySettingsRowDisabled(rowEnabled, regenBtn, e)
	bindRowGate(gs, vd, rowEnabled, titleLab, regenBtn, e)
	return row
}
