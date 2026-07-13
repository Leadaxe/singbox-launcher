package tabs

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
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

// ShowRulesLibraryDialog shows preset list from the template; user checks rows and taps Add selected.
// One unified section — Library is just a catalog of presets from template.presets[].
// On Add: preset-ref `{kind: preset, ref: <id>, body: {vars: {}}}` is appended to state.rules[].
// If the same ref already exists in state — the checkbox is disabled and the row is marked.
//
// Использует widget.NewModalPopUp напрямую (вместо dialog.NewCustom) ради
// AddPopUpToolTipLayer — без этого tooltips на ttwidget.Label внутри dialog'а
// рендерятся в window-level tooltip layer'е, который находится **под**
// dialog-overlay'ем в z-order'е canvas.Overlays(). Tooltips создаются, но
// невидимы за модальным фоном. AddPopUpToolTipLayer добавляет tooltip layer
// **внутрь** PopUp'а — тогда они рисуются поверх content'а dialog'а.
func ShowRulesLibraryDialog(p *wizardpresentation.WizardPresenter, showAddRuleDialog ShowAddRuleDialogFunc) {
	guiState := p.GUIState()
	model := p.Model()
	win := guiState.Window
	if win == nil || model.TemplateData == nil {
		debuglog.DebugLog("library_rules_dialog: skip (nil window or template)")
		return
	}
	presets := model.TemplateData.Presets
	if len(presets) == 0 {
		debuglog.DebugLog("library_rules_dialog: no presets in template")
		return
	}

	picked := make([]bool, len(presets))
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

	existingRefs := make(map[string]bool, len(model.PresetRefs))
	for _, pr := range model.PresetRefs {
		if pr != nil {
			existingRefs[pr.Ref] = true
		}
	}

	for i := range presets {
		i, pr := i, &presets[i]
		labelText := pr.Label
		if labelText == "" {
			labelText = pr.ID
		}
		already := existingRefs[pr.ID]
		if already {
			labelText += "  · already added"
		}

		// Прямой ttwidget.Label.SetToolTip — тот же паттерн что работает на
		// Rules tab (rules_unified_rows.go: ttwidget.NewLabel + SetToolTip
		// без wrap'ов). NewCheckWithContent создавал промежуточный hoverable
		// wrap, через который Fyne не пробрасывал hover на label →
		// ToolTipWidgetExtend.MouseIn никогда не вызывался.
		lbl := ttwidget.NewLabel(labelText)
		lbl.Wrapping = fyne.TextWrapOff
		lbl.Truncation = fyne.TextTruncateEllipsis
		if d := strings.TrimSpace(pr.Description); d != "" {
			lbl.SetToolTip(d)
		}

		var row *fynewidget.HoverRow
		chk := widget.NewCheck("", func(on bool) {
			if already {
				picked[i] = false
				return
			}
			picked[i] = on
			if row != nil {
				row.Refresh()
			}
			refreshAddBtn(addBtn)
		})
		if already {
			chk.Checked = true
			chk.Disable()
		}

		// TapWrap делает label-область кликабельной для toggle чекбокса —
		// тот же паттерн что на Rules tab (rules_unified_rows.go:288).
		// TapWrap НЕ блокирует hover-события на дочернем label (он Tappable
		// + Cursorable, не Hoverable), поэтому ttwidget.Label.MouseIn
		// продолжает срабатывать → tooltip показывается.
		labelTap := fynewidget.NewTapWrap(lbl, func() {
			if chk.Disabled() {
				return
			}
			chk.SetChecked(!chk.Checked)
		})

		rowInner := container.NewBorder(nil, nil, fynewidget.CheckLeadingWrap(chk), nil, labelTap)
		row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{
			IsSelected:   func() bool { return picked[i] },
			SelectedFill: &rowHighlight,
		})
		row.WireTooltipLabelHover(lbl)
		listBox.Add(row)
	}

	scrollGutter := components.NewScrollGutter()
	scrollInner := container.NewBorder(nil, nil, nil, scrollGutter, listBox)
	scroll := container.NewVScroll(scrollInner)
	minH := canvas.NewRectangle(color.Transparent)
	minH.SetMinSize(fyne.NewSize(0, 300))
	scrollBlock := container.NewStack(minH, scroll)

	hint := widget.NewLabel(locale.T("wizard.rules.library_hint"))
	hint.Wrapping = fyne.TextWrapWord

	// PopUp + tooltip layer: строим dialog руками вместо dialog.NewCustom —
	// последний прячет underlying *widget.PopUp за интерфейс, не позволяя
	// прицепить AddPopUpToolTipLayer.
	titleLabel := widget.NewLabelWithStyle(locale.T("wizard.rules.library_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	separator := widget.NewSeparator()

	var popup *widget.PopUp
	cancelBtn := widget.NewButton(locale.T("wizard.rules.library_cancel"), func() {
		if popup != nil {
			popup.Hide()
		}
	})
	buttonsRow := container.NewBorder(nil, nil, cancelBtn, container.NewHBox(layout.NewSpacer(), addBtn), nil)

	main := container.NewVBox(hint, scrollBlock)
	body := container.NewBorder(
		container.NewVBox(titleLabel, separator), // top
		buttonsRow,                               // bottom
		nil, nil,
		main,
	)
	// Размер задаём через padded wrapper — PopUp растягивается под content.
	sizer := canvas.NewRectangle(color.Transparent)
	sizer.SetMinSize(fyne.NewSize(520, 440))
	content := container.NewStack(sizer, body)

	popup = widget.NewModalPopUp(content, win.Canvas())
	fynetooltip.AddPopUpToolTipLayer(popup)

	addBtn.OnTapped = func() {
		added := 0
		for i, pr := range presets {
			if !picked[i] || existingRefs[pr.ID] {
				continue
			}
			ref := wizardpresentation.PresetRefForUI{
				Ref:     pr.ID,
				Enabled: true,
				Vars:    map[string]string{},
			}
			ref.AppendTo(model)
			added++
		}
		if added == 0 {
			debuglog.WarnLog("library_rules_dialog: no presets added (all already present?)")
		}
		model.TemplatePreviewNeedsUpdate = true
		p.MarkAsChanged()
		fynetooltip.DestroyPopUpToolTipLayer(popup)
		popup.Hide()
		refreshRulesTabFromPresenter(p, showAddRuleDialog)
		p.RefreshOutboundOptions()
	}

	// ESC закрывает popup. Сохраняем предыдущий обработчик canvas для restore.
	prevKeyHandler := win.Canvas().OnTypedKey()
	win.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		if key.Name == fyne.KeyEscape {
			fynetooltip.DestroyPopUpToolTipLayer(popup)
			popup.Hide()
			win.Canvas().SetOnTypedKey(prevKeyHandler)
			return
		}
		if prevKeyHandler != nil {
			prevKeyHandler(key)
		}
	})
	// Hook на close cancel btn — restore key handler + destroy tooltip layer.
	origCancel := cancelBtn.OnTapped
	cancelBtn.OnTapped = func() {
		fynetooltip.DestroyPopUpToolTipLayer(popup)
		win.Canvas().SetOnTypedKey(prevKeyHandler)
		if origCancel != nil {
			origCancel()
		}
	}

	popup.Show()
}
