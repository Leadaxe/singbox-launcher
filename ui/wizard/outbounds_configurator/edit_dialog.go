// edit_dialog.go provides the Add/Edit outbound dialog for the configurator.
// The dialog is shown as a separate window (like the Add Rule dialog).
package outbounds_configurator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// ShowEditDialog opens a separate window to add or edit an outbound. existing may be nil for add.
// onSave is called with the new config, scopeKind ("global" or "source") and sourceIndex (when scope is source).
// editPresenter is optional; when set, only one Edit/Add window is allowed and it is registered for overlay/focus.
func ShowEditDialog(
	parent fyne.Window,
	editPresenter OutboundEditPresenter,
	parserConfig *config.ParserConfig,
	existing *config.OutboundConfig,
	isGlobal bool,
	sourceIndex int,
	existingTags []string,
	onSave func(updated *config.OutboundConfig, scopeKind string, sourceIndex int),
) {
	if editPresenter != nil {
		if w := editPresenter.OpenOutboundEditWindow(); w != nil {
			w.RequestFocus()
			return
		}
	}
	isAdd := existing == nil
	dialogTitle := "Edit Outbound"
	if isAdd {
		dialogTitle = "Add Outbound"
	}

	tagEntry := widget.NewEntry()
	if existing != nil {
		tagEntry.SetText(existing.Tag)
	}
	tagEntry.SetPlaceHolder("e.g. proxy-out")

	typeSelect := widget.NewSelect([]string{"manual (selector)", "auto (urltest)"}, nil)
	if existing != nil {
		if existing.Type == "urltest" {
			typeSelect.SetSelected("auto (urltest)")
		} else {
			typeSelect.SetSelected("manual (selector)")
		}
	} else {
		typeSelect.SetSelected("manual (selector)")
	}

	commentEntry := widget.NewEntry()
	if existing != nil {
		commentEntry.SetText(existing.Comment)
	}
	commentEntry.SetPlaceHolder("Optional comment")

	// Scope: For all | For source: ...
	scopeOptions := []string{"For all"}
	for i, p := range parserConfig.ParserConfig.Proxies {
		label := p.Source
		if label == "" {
			label = "Source " + strconv.Itoa(i+1)
		}
		if len(label) > 35 {
			label = label[:32] + "..."
		}
		scopeOptions = append(scopeOptions, "For source: "+label)
	}
	scopeSelect := widget.NewSelect(scopeOptions, nil)
	if isAdd {
		scopeSelect.SetSelected("For all")
	} else if isGlobal {
		scopeSelect.SetSelected("For all")
	} else {
		if sourceIndex >= 0 && sourceIndex < len(parserConfig.ParserConfig.Proxies) {
			scopeSelect.SetSelected(scopeOptions[sourceIndex+1])
		} else {
			scopeSelect.SetSelected(scopeOptions[0])
		}
	}

	// Filters: fixed key "tag", value editable
	filterKeyLabel := widget.NewLabel("tag")
	filterValEntry := widget.NewEntry()
	filterValEntry.SetPlaceHolder("e.g. /🇳🇱/i or !/(🇷🇺)/i")
	if existing != nil && existing.Filters != nil {
		if v, ok := existing.Filters["tag"]; ok {
			if s, ok := v.(string); ok {
				filterValEntry.SetText(s)
			}
		} else {
			for _, v := range existing.Filters {
				if s, ok := v.(string); ok {
					filterValEntry.SetText(s)
					break
				}
			}
		}
	}

	// Preferred default: fixed key "tag", value editable
	defKeyLabel := widget.NewLabel("tag")
	defValEntry := widget.NewEntry()
	defValEntry.SetPlaceHolder("e.g. /🇳🇱/i")
	if existing != nil && existing.PreferredDefault != nil {
		if v, ok := existing.PreferredDefault["tag"]; ok {
			if s, ok := v.(string); ok {
				defValEntry.SetText(s)
			}
		} else {
			for _, v := range existing.PreferredDefault {
				if s, ok := v.(string); ok {
					defValEntry.SetText(s)
					break
				}
			}
		}
	}

	// AddOutbounds: direct-out, reject checkboxes + checkboxes for other tags
	directCheck := widget.NewCheck("direct-out", nil)
	rejectCheck := widget.NewCheck("reject", nil)
	otherTagChecks := make([]*widget.Check, 0, len(existingTags))
	otherTagsMap := make(map[string]*widget.Check)
	for _, tag := range existingTags {
		c := widget.NewCheck(tag, nil)
		otherTagChecks = append(otherTagChecks, c)
		otherTagsMap[tag] = c
	}
	if existing != nil && len(existing.AddOutbounds) > 0 {
		for _, t := range existing.AddOutbounds {
			if t == "direct-out" {
				directCheck.SetChecked(true)
			} else if t == "reject" {
				rejectCheck.SetChecked(true)
			} else if c, ok := otherTagsMap[t]; ok {
				c.SetChecked(true)
			}
		}
	}

	otherTagsBox := container.NewVBox()
	for _, c := range otherTagChecks {
		otherTagsBox.Add(c)
	}
	scrollOther := container.NewScroll(otherTagsBox)
	scrollOther.SetMinSize(fyne.NewSize(0, 80))

	// Raw tab: editable JSON (valid outbound object)
	initialConfig := existing
	if initialConfig == nil {
		initialConfig = &config.OutboundConfig{
			Tag:           "",
			Type:          "selector",
			Comment:       "",
			Options:       map[string]interface{}{"interrupt_exist_connections": true},
			AddOutbounds:  nil,
		}
	}
	rawJSONBytes, _ := json.MarshalIndent(initialConfig, "", "  ")
	rawEntry := widget.NewMultiLineEntry()
	rawEntry.SetText(string(rawJSONBytes))
	rawEntry.Wrapping = fyne.TextWrapOff
	rawEntry.SetMinRowsVisible(16)
	rawScroll := container.NewScroll(rawEntry)
	rawScroll.SetMinSize(fyne.NewSize(400, 360))

	var currentTab string = "settings"

	var dialogWin fyne.Window
	getScopeFromForm := func() (scopeKind string, idx int) {
		scopeKind = "global"
		idx = -1
		if scopeSelect.Selected != "" && strings.HasPrefix(scopeSelect.Selected, "For source:") {
			scopeKind = "source"
			for i, opt := range scopeOptions {
				if i > 0 && opt == scopeSelect.Selected {
					idx = i - 1
					break
				}
			}
		}
		return scopeKind, idx
	}
	save := func() {
		if currentTab == "raw" {
			var cfg config.OutboundConfig
			if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
				dialog.ShowError(fmt.Errorf("invalid JSON: %w", err), dialogWin)
				return
			}
			if strings.TrimSpace(cfg.Tag) == "" {
				dialog.ShowError(fmt.Errorf("tag is required"), dialogWin)
				return
			}
			scopeKind, idx := getScopeFromForm()
			if existing != nil && existing.Wizard != nil {
				cfg.Wizard = wizardbusiness.CloneOutbound(existing).Wizard
			}
			onSave(&cfg, scopeKind, idx)
			if dialogWin != nil {
				dialogWin.Close()
			}
			return
		}
		tag := strings.TrimSpace(tagEntry.Text)
		if tag == "" {
			dialog.ShowError(fmt.Errorf("tag is required"), dialogWin)
			return
		}
		obType := "selector"
		if typeSelect.Selected == "auto (urltest)" {
			obType = "urltest"
		}
		scopeKind, idx := getScopeFromForm()

		cfg := &config.OutboundConfig{
			Tag:     tag,
			Type:    obType,
			Comment: strings.TrimSpace(commentEntry.Text),
		}
		if existing != nil && existing.Options != nil {
			cfg.Options = make(map[string]interface{})
			for k, v := range existing.Options {
				cfg.Options[k] = v
			}
		} else if obType == "selector" {
			cfg.Options = map[string]interface{}{"interrupt_exist_connections": true}
		} else {
			cfg.Options = map[string]interface{}{
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "5m", "tolerance": 100,
				"interrupt_exist_connections": true,
			}
		}

		filterVal := strings.TrimSpace(filterValEntry.Text)
		if filterVal != "" {
			cfg.Filters = map[string]interface{}{"tag": filterVal}
		}
		defVal := strings.TrimSpace(defValEntry.Text)
		if defVal != "" {
			cfg.PreferredDefault = map[string]interface{}{"tag": defVal}
		}

		var addOb []string
		if directCheck.Checked {
			addOb = append(addOb, "direct-out")
		}
		if rejectCheck.Checked {
			addOb = append(addOb, "reject")
		}
		for _, c := range otherTagChecks {
			if c.Checked {
				addOb = append(addOb, c.Text)
			}
		}
		cfg.AddOutbounds = addOb

		// Preserve wizard if editing
		if existing != nil && existing.Wizard != nil {
			cfg.Wizard = wizardbusiness.CloneOutbound(existing).Wizard
		}
		onSave(cfg, scopeKind, idx)
		if dialogWin != nil {
			dialogWin.Close()
		}
	}

	form := container.NewVBox(
		widget.NewLabel("Scope"),
		scopeSelect,
		widget.NewLabel("Tag"),
		tagEntry,
		widget.NewLabel("Type"),
		typeSelect,
		widget.NewLabel("Comment"),
		commentEntry,
		widget.NewLabel("Filters (key and value; use !/regex/i for negation)"),
		container.NewGridWithColumns(2, filterKeyLabel, filterValEntry),
		widget.NewLabel("Preferred default (filter for default node)"),
		container.NewGridWithColumns(2, defKeyLabel, defValEntry),
		widget.NewLabel("Add outbounds at start (direct-out, reject, others)"),
		container.NewHBox(directCheck, rejectCheck),
		scrollOther,
	)
	// Right margin inside scroll so the scrollbar does not overlap form elements
	const scrollbarGap = 20
	rightGap := canvas.NewRectangle(color.Transparent)
	rightGap.SetMinSize(fyne.NewSize(scrollbarGap, 0))
	formWithGap := container.NewBorder(nil, nil, nil, rightGap, form)
	widthSpacer := canvas.NewRectangle(color.Transparent)
	widthSpacer.SetMinSize(fyne.NewSize(400, 0))
	scrollContent := container.NewMax(widthSpacer, formWithGap)
	dialogScroll := container.NewScroll(scrollContent)
	dialogScroll.SetMinSize(fyne.NewSize(400, 400))

	tabs := container.NewAppTabs(
		container.NewTabItem("Settings", dialogScroll),
		container.NewTabItem("Raw", rawScroll),
	)
	tabs.OnSelected = func(t *container.TabItem) {
		if t.Text == "Raw" {
			currentTab = "raw"
		} else {
			currentTab = "settings"
		}
	}

	cancelBtn := widget.NewButton("Cancel", func() {
		if dialogWin != nil {
			dialogWin.Close()
		}
	})
	saveBtn := widget.NewButton("Save", func() { save() })

	buttonsContainer := container.NewHBox(
		layout.NewSpacer(),
		cancelBtn,
		saveBtn,
	)
	mainContent := container.NewBorder(
		nil,
		buttonsContainer,
		nil,
		nil,
		tabs,
	)

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	dialogWin = app.NewWindow(dialogTitle)
	if editPresenter != nil {
		editPresenter.SetOutboundEditWindow(dialogWin)
		dialogWin.SetOnClosed(func() {
			editPresenter.ClearOutboundEditWindow()
			editPresenter.UpdateChildOverlay()
		})
	}
	dialogWin.Resize(fyne.NewSize(440, 560))
	dialogWin.CenterOnScreen()
	dialogWin.SetContent(mainContent)
	dialogWin.Show()
	if editPresenter != nil {
		editPresenter.UpdateChildOverlay()
	}
}
