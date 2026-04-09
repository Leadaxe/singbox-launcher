package tabs

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	internaldialogs "singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// rowHighlight is a subtle tint for rows marked for adding (similar to list selection).
var rowHighlight = color.NRGBA{R: 0x33, G: 0x88, B: 0xee, A: 0x2a}

func boolSliceAnyTrue(values []bool) bool {
	for _, v := range values {
		if v {
			return true
		}
	}
	return false
}

// ShowRulesLibraryDialog shows presets from the template; user checks rows and taps Add selected.
func ShowRulesLibraryDialog(p *wizardpresentation.WizardPresenter, showAddRuleDialog ShowAddRuleDialogFunc) {
	guiState := p.GUIState()
	model := p.Model()
	win := guiState.Window
	if win == nil || model.TemplateData == nil {
		debuglog.DebugLog("library_rules_dialog: skip (nil window or template)")
		return
	}
	rules := model.TemplateData.SelectableRules
	if len(rules) == 0 {
		debuglog.DebugLog("library_rules_dialog: no selectable_rules in template")
		return
	}

	picked := make([]bool, len(rules))
	listBox := container.NewVBox()

	addBtn := widget.NewButton(locale.T("wizard.rules.library_add_selected"), nil)
	addBtn.Importance = widget.HighImportance
	addBtn.Disable()

	refreshAddBtn := func(b *widget.Button) {
		if boolSliceAnyTrue(picked) {
			b.Enable()
		} else {
			b.Disable()
		}
	}

	for i := range rules {
		i, tr := i, &rules[i]

		lbl := ttwidget.NewLabel(tr.Label)
		lbl.Wrapping = fyne.TextWrapOff
		lbl.Truncation = fyne.TextTruncateEllipsis
		if d := strings.TrimSpace(tr.Description); d != "" {
			lbl.SetToolTip(d)
		}

		var row *fynewidget.HoverRow
		chk := widget.NewCheck("", func(on bool) {
			picked[i] = on
			if row != nil {
				row.Refresh()
			}
			refreshAddBtn(addBtn)
		})

		labelTap := fynewidget.NewTapWrap(lbl, func() {
			if chk.Disabled() {
				return
			}
			chk.SetChecked(!chk.Checked)
		})

		// Border: чекбокс слева, подпись в центре получает оставшуюся ширину (HBox даёт лейблу ~0 → только «…»).
		rowLeft := container.NewBorder(nil, nil, fynewidget.CheckLeadingWrap(chk), nil, labelTap)
		padded := container.NewPadded(rowLeft)
		minH := canvas.NewRectangle(color.Transparent)
		minH.SetMinSize(fyne.NewSize(0, 36))
		paddedWithMin := container.NewMax(minH, padded)
		row = fynewidget.NewHoverRow(paddedWithMin, fynewidget.HoverRowConfig{
			IsSelected:   func() bool { return picked[i] },
			SelectedFill: &rowHighlight,
		})
		row.WireTooltipLabelHover(lbl)
		listBox.Add(row)
	}

	scrollGutter := canvas.NewRectangle(color.Transparent)
	scrollGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	scrollInner := container.NewBorder(nil, nil, nil, scrollGutter, listBox)
	scroll := container.NewVScroll(scrollInner)
	minH := canvas.NewRectangle(color.Transparent)
	minH.SetMinSize(fyne.NewSize(0, 300))
	scrollBlock := container.NewMax(minH, scroll)

	hint := widget.NewLabel(locale.T("wizard.rules.library_hint"))
	hint.Wrapping = fyne.TextWrapWord

	buttons := container.NewHBox(layout.NewSpacer(), addBtn)
	main := container.NewVBox(hint, scrollBlock)
	var dlg dialog.Dialog
	addBtn.OnTapped = func() {
		added := wizardbusiness.AppendClonedPresetsToCustomRules(model, rules, picked)
		if added == 0 {
			debuglog.WarnLog("library_rules_dialog: no rules appended (selection present but clone failed?)")
		}
		model.TemplatePreviewNeedsUpdate = true
		p.MarkAsChanged()
		dlg.Hide()
		refreshRulesTabFromPresenter(p, showAddRuleDialog)
		p.RefreshOutboundOptions()
	}

	dlg = internaldialogs.NewCustom(locale.T("wizard.rules.library_title"), main, buttons, locale.T("wizard.rules.library_cancel"), win)
	dlg.Resize(fyne.NewSize(520, 440))
	dlg.Show()
}
