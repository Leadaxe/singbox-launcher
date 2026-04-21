package tabs

import (
	"image/color"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

func settingsVarVisible(v wizardtemplate.TemplateVar, goos string) bool {
	ui := strings.ToLower(strings.TrimSpace(v.WizardUI))
	if ui == "hidden" || ui == "fix" {
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

func enumListContains(opts []string, v string) bool {
	for _, o := range opts {
		if o == v {
			return true
		}
	}
	return false
}

// clashSecretSecretVar — единственный поддерживаемый в Settings тип "secret": отдельная строка и кнопка перегенерации.
func clashSecretSecretVar(v wizardtemplate.TemplateVar) bool {
	return strings.EqualFold(strings.TrimSpace(v.Type), "secret") && v.Name == "clash_secret"
}

// templateVarUsedInAnotherVarConditional: имя bool-переменной в if/if_or другой var — после её смены нужно пересобрать Settings.
func templateVarUsedInAnotherVarConditional(td *wizardtemplate.TemplateData, name string) bool {
	if td == nil {
		return false
	}
	for _, v := range td.Vars {
		for _, x := range v.If {
			if x == name {
				return true
			}
		}
		for _, x := range v.IfOr {
			if x == name {
				return true
			}
		}
	}
	return false
}

func maybeRefreshSettingsAfterVarChange(gs *wizardpresentation.GUIState, td *wizardtemplate.TemplateData, changedName string) {
	if templateVarUsedInAnotherVarConditional(td, changedName) && gs.RefreshSettingsFromModel != nil {
		gs.RefreshSettingsFromModel()
	}
}

func applySettingsRowDisabled(rowEnabled bool, resetBtn *ttwidget.Button, extras ...fyne.Disableable) {
	if rowEnabled {
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

func newSettingsTitleLabel(text string) *ttwidget.Label {
	l := ttwidget.NewLabel(text)
	// В container.NewBorder лейбл в позиции leading получает свою MinSize; при TextWrapWord
	// при узкой колонке MinWidth схлопывается, текст уезжает столбиком по символам.
	l.Wrapping = fyne.TextWrapOff
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
		if tb, ok := interface{}(o).(interface{ SetToolTip(string) }); ok {
			tb.SetToolTip(tip)
		}
	}
}

// CreateSettingsTab строит вкладку Settings из wizard_template.json vars.
func CreateSettingsTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	model := presenter.Model()
	gs := presenter.GUIState()
	box := container.NewVBox()
	goos := runtime.GOOS

	refresh := func() {
		box.RemoveAll()
		if model.TemplateData == nil || len(model.TemplateData.Vars) == 0 {
			box.Add(widget.NewLabel(locale.T("wizard.settings.no_vars")))
			box.Refresh()
			return
		}
		td := model.TemplateData
		vi := wizardtemplate.VarIndex(td.Vars)
		resolved := wizardtemplate.ResolveTemplateVars(td.Vars, model.SettingsVars, td.RawTemplate)
		for _, vd := range td.Vars {
			if !settingsVarVisible(vd, goos) {
				continue
			}
			if vd.Separator {
				box.Add(settingsSeparatorBlock())
				continue
			}
			if strings.EqualFold(strings.TrimSpace(vd.Type), "secret") && !clashSecretSecretVar(vd) {
				continue
			}
			title := wizardtemplate.VarDisplayTitle(vd)
			toolTip := wizardtemplate.VarDisplayTooltip(vd)
			rowEnabled := wizardtemplate.VarUISatisfied(vd, vi, resolved, goos)
			row := buildSettingsVarRow(presenter, model, td, vd, title, toolTip, rowEnabled, gs)
			box.Add(row)
		}
		box.Refresh()
	}
	gs.RefreshSettingsFromModel = refresh
	refresh()

	scroll := container.NewVScroll(box)
	scroll.SetMinSize(fyne.NewSize(0, 400))
	return scroll
}

func buildSettingsVarRow(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, vd wizardtemplate.TemplateVar, title, toolTip string, rowEnabled bool, gs *wizardpresentation.GUIState) fyne.CanvasObject {
	name := vd.Name
	typ := vd.Type
	options := vd.Options
	viewMode := strings.EqualFold(strings.TrimSpace(vd.WizardUI), "view")

	st := model.SettingsVars
	raw := td.RawTemplate
	vars := td.Vars

	if clashSecretSecretVar(vd) {
		return buildClashSecretSecretRow(presenter, model, td, vd, title, toolTip, viewMode, rowEnabled)
	}

	reset := func() {
		delete(model.SettingsVars, name)
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
		if presenter.GUIState().RefreshSettingsFromModel != nil {
			presenter.GUIState().RefreshSettingsFromModel()
		}
	}

	resetBtn := ttwidget.NewButtonWithIcon("", theme.ContentUndoIcon(), reset)
	resetBtn.Importance = widget.LowImportance
	resetBtn.SetToolTip(locale.T("wizard.settings.reset_tooltip"))

	if viewMode {
		disp := strings.TrimSpace(wizardtemplate.DisplaySettingValue(vars, st, raw, name))
		if typ == "bool" {
			if disp != "true" && disp != "false" {
				disp = "false"
			}
		}
		valLab := ttwidget.NewLabel(disp)
		valLab.Wrapping = fyne.TextWrapWord
		titleLab := newSettingsTitleLabel(title)
		row := container.NewBorder(nil, nil, titleLab, resetBtn, valLab)
		setVarFieldToolTip(toolTip, titleLab, valLab)
		applySettingsRowDisabled(rowEnabled, resetBtn)
		return row
	}

	switch typ {
	case "bool":
		var prog bool
		var chkForDarwin *widget.Check
		titleLbl := newSettingsTitleLabel(title)
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
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
			maybeRefreshSettingsAfterVarChange(gs, td, name)
		}
		cwc := fynewidget.NewCheckWithContent(onChanged, titleLbl, fynewidget.CheckWithContentConfig{})
		chk := cwc.Check
		chkForDarwin = chk
		prog = true
		v, overridden := model.SettingsVars[name]
		checked := strings.TrimSpace(wizardtemplate.DisplaySettingValue(vars, st, raw, name)) == "true"
		if overridden {
			checked = v == "true"
		}
		chk.SetChecked(checked)
		prog = false
		row := container.NewBorder(nil, nil, cwc.CheckLeading, resetBtn, cwc.Content)
		setVarFieldToolTip(toolTip, titleLbl, chk)
		applySettingsRowDisabled(rowEnabled, resetBtn, chk)
		return row

	case "enum":
		titleLab := newSettingsTitleLabel(title)
		sel := widget.NewSelect(options, func(val string) {
			model.SettingsVars[name] = val
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
			maybeRefreshSettingsAfterVarChange(gs, td, name)
		})
		disp := wizardtemplate.DisplaySettingValue(vars, st, raw, name)
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
		sel.SetSelected(disp)
		row := container.NewBorder(nil, nil, titleLab, resetBtn, sel)
		setVarFieldToolTip(toolTip, titleLab, sel)
		applySettingsRowDisabled(rowEnabled, resetBtn, sel)
		return row

	case "text_list":
		titleLab := newSettingsTitleLabel(title)
		e := widget.NewMultiLineEntry()
		e.SetMinRowsVisible(3)
		disp := wizardtemplate.DisplaySettingValue(vars, st, raw, name)
		if v, ok := model.SettingsVars[name]; ok {
			disp = v
		}
		e.SetText(disp)
		e.OnChanged = func(s string) {
			model.SettingsVars[name] = s
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
		row := container.NewBorder(nil, nil, titleLab, resetBtn, e)
		setVarFieldToolTip(toolTip, titleLab, e)
		applySettingsRowDisabled(rowEnabled, resetBtn, e)
		return row

	default: // text
		titleLab := newSettingsTitleLabel(title)
		disp := wizardtemplate.DisplaySettingValue(vars, st, raw, name)
		if v, ok := model.SettingsVars[name]; ok {
			disp = v
		}
		onChanged := func(s string) {
			model.SettingsVars[name] = s
			model.TemplatePreviewNeedsUpdate = true
			presenter.MarkAsChanged()
		}
		// If the var declares `options` on a text type, render a combo-dropdown
		// (free text + preset suffix menu) instead of a plain entry. Ported from
		// LxBox — turns "Test URL" / "Test interval" / "Tolerance" from a typo
		// minefield into tap-to-pick-or-edit.
		if len(options) > 0 {
			se := widget.NewSelectEntry(options)
			se.SetText(disp)
			se.OnChanged = onChanged
			row := container.NewBorder(nil, nil, titleLab, resetBtn, se)
			setVarFieldToolTip(toolTip, titleLab, se)
			applySettingsRowDisabled(rowEnabled, resetBtn, se)
			return row
		}
		e := widget.NewEntry()
		e.SetText(disp)
		e.OnChanged = onChanged
		row := container.NewBorder(nil, nil, titleLab, resetBtn, e)
		setVarFieldToolTip(toolTip, titleLab, e)
		applySettingsRowDisabled(rowEnabled, resetBtn, e)
		return row
	}
}

func buildClashSecretSecretRow(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, vd wizardtemplate.TemplateVar, title, toolTip string, viewMode bool, rowEnabled bool) fyne.CanvasObject {
	name := vd.Name
	st := model.SettingsVars
	raw := td.RawTemplate
	vars := td.Vars

	regenerate := func() {
		if model.SettingsVars == nil {
			model.SettingsVars = make(map[string]string)
		}
		gen, err := wizardtemplate.GenerateClashSecret()
		if err != nil {
			debuglog.WarnLog("settings_tab: GenerateClashSecret: %v", err)
			delete(model.SettingsVars, name)
		} else {
			model.SettingsVars[name] = gen
		}
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
		if presenter.GUIState().RefreshSettingsFromModel != nil {
			presenter.GUIState().RefreshSettingsFromModel()
		}
	}

	regenBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), regenerate)
	regenBtn.Importance = widget.LowImportance
	regenBtn.SetToolTip(locale.T("wizard.settings.clash_secret_regenerate_tooltip"))

	if viewMode {
		disp := strings.TrimSpace(wizardtemplate.DisplaySettingValue(vars, st, raw, name))
		valLab := ttwidget.NewLabel(disp)
		valLab.Wrapping = fyne.TextWrapWord
		titleLab := newSettingsTitleLabel(title)
		row := container.NewBorder(nil, nil, titleLab, regenBtn, valLab)
		setVarFieldToolTip(toolTip, titleLab, valLab)
		applySettingsRowDisabled(rowEnabled, regenBtn)
		return row
	}

	titleLab := newSettingsTitleLabel(title)
	e := widget.NewEntry()
	disp := wizardtemplate.DisplaySettingValue(vars, st, raw, name)
	if v, ok := model.SettingsVars[name]; ok {
		disp = v
	}
	e.SetText(disp)
	e.OnChanged = func(s string) {
		model.SettingsVars[name] = s
		model.TemplatePreviewNeedsUpdate = true
		presenter.MarkAsChanged()
	}
	row := container.NewBorder(nil, nil, titleLab, regenBtn, e)
	setVarFieldToolTip(toolTip, titleLab, e)
	applySettingsRowDisabled(rowEnabled, regenBtn, e)
	return row
}

