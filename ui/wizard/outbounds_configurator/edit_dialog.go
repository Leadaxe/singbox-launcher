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
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// ShowEditDialog opens a separate window to add or edit an outbound. existing may be nil for add.
// ParserConfig is taken from the model (editPresenter.Model()) so the dialog always uses current sources.
// onSave is called with the new config, scopeKind ("global" or "source") and sourceIndex (when scope is source).
// editPresenter is required (Model() is used to get ParserConfig); when set, only one Edit/Add window is allowed.
func ShowEditDialog(
	parent fyne.Window,
	editPresenter OutboundEditPresenter,
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
	parserConfig := getParserConfig(editPresenter.Model())
	if parserConfig == nil {
		dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.outbound.error_config")), parent)
		return
	}
	isAdd := existing == nil
	dialogTitle := locale.T("wizard.outbound.title_edit")
	if isAdd {
		dialogTitle = locale.T("wizard.outbound.title_add")
	}

	tagEntry := widget.NewEntry()
	if existing != nil {
		tagEntry.SetText(existing.Tag)
	}
	tagEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_tag"))

	typeSelect := widget.NewSelect([]string{locale.T("wizard.outbound.type_manual"), locale.T("wizard.outbound.type_auto")}, nil)
	if existing != nil {
		if existing.Type == "urltest" {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_auto"))
		} else {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
		}
	} else {
		typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
	}

	commentEntry := widget.NewEntry()
	if existing != nil {
		commentEntry.SetText(existing.Comment)
	}
	commentEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_comment"))

	// Scope: For all | For source: ...
	scopeOptions := []string{locale.T("wizard.outbound.scope_all")}
	for i, p := range parserConfig.ParserConfig.Proxies {
		label := p.Source
		if label == "" {
			label = locale.T("wizard.outbound.label_source") + strconv.Itoa(i+1)
		}
		label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
		scopeOptions = append(scopeOptions, locale.T("wizard.outbound.scope_source")+label)
	}
	scopeSelect := widget.NewSelect(scopeOptions, nil)
	if isAdd {
		scopeSelect.SetSelected(locale.T("wizard.outbound.scope_all"))
	} else if isGlobal {
		scopeSelect.SetSelected(locale.T("wizard.outbound.scope_all"))
	} else {
		if sourceIndex >= 0 && sourceIndex < len(parserConfig.ParserConfig.Proxies) {
			scopeSelect.SetSelected(scopeOptions[sourceIndex+1])
		} else {
			scopeSelect.SetSelected(scopeOptions[0])
		}
	}

	// Filters: fixed key "tag", value editable
	filterKeyLabel := widget.NewLabel(locale.T("wizard.outbound.label_tag"))
	filterValEntry := widget.NewEntry()
	filterValEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_filter"))
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
	defKeyLabel := widget.NewLabel(locale.T("wizard.outbound.label_tag"))
	defValEntry := widget.NewEntry()
	defValEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_preferred"))
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
			Tag:          "",
			Type:         "selector",
			Comment:      "",
			Options:      map[string]interface{}{"interrupt_exist_connections": true},
			AddOutbounds: nil,
		}
	}
	rawJSONBytes, _ := json.MarshalIndent(initialConfig, "", "  ")
	rawEntry := widget.NewMultiLineEntry()
	rawEntry.SetText(string(rawJSONBytes))
	rawEntry.Wrapping = fyne.TextWrapOff
	rawEntry.SetMinRowsVisible(16)
	rawScroll := container.NewScroll(rawEntry)
	rawScroll.SetMinSize(fyne.NewSize(400, 360))

	// Raw documentation button (opens ParserConfig.md "Секция outbounds")
	rawDocButton := widget.NewButton(locale.T("wizard.outbound.button_docs"), func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/ParserConfig.md#%D1%81%D0%B5%D0%BA%D1%86%D0%B8%D1%8F-outbounds"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_open_docs"), err), parent)
		}
	})
	rawHeader := container.NewHBox(
		widget.NewLabel(locale.T("wizard.outbound.label_raw_json")),
		layout.NewSpacer(),
		rawDocButton,
	)
	rawContainer := container.NewBorder(
		rawHeader,
		nil,
		nil,
		nil,
		rawScroll,
	)

	var currentTab string = "settings"

	var dialogWin fyne.Window
	getScopeFromForm := func() (scopeKind string, idx int) {
		scopeKind = "global"
		idx = -1
		if scopeSelect.Selected != "" && strings.HasPrefix(scopeSelect.Selected, locale.T("wizard.outbound.scope_source")) {
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
	// buildConfigForPreview builds a config.OutboundConfig snapshot based on current UI state.
	// It is used by the Preview tab; errors are returned to be shown inline.
	buildConfigForPreview := func() (*config.OutboundConfig, error) {
		if currentTab == "raw" {
			var cfg config.OutboundConfig
			if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
				return nil, fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_invalid_json"), err)
			}
			if strings.TrimSpace(cfg.Tag) == "" {
				return nil, fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required"))
			}
			return &cfg, nil
		}

		tag := strings.TrimSpace(tagEntry.Text)
		if tag == "" {
			return nil, fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required"))
		}
		obType := "selector"
		if typeSelect.Selected == locale.T("wizard.outbound.type_auto") {
			obType = "urltest"
		}

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
				"url":      "https://cp.cloudflare.com/generate_204",
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

		return cfg, nil
	}

	save := func() {
		if currentTab == "raw" {
			var cfg config.OutboundConfig
			if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbound.error_invalid_json"), err), dialogWin)
				return
			}
			if strings.TrimSpace(cfg.Tag) == "" {
				dialog.ShowError(fmt.Errorf("%s", locale.T("wizard.outbound.error_tag_required")), dialogWin)
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

		cfg, err := buildConfigForPreview()
		if err != nil {
			dialog.ShowError(err, dialogWin)
			return
		}
		scopeKind, idx := getScopeFromForm()

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
		widget.NewLabel(locale.T("wizard.outbound.label_scope")),
		scopeSelect,
		widget.NewLabel(locale.T("wizard.outbound.label_tag_field")),
		tagEntry,
		widget.NewLabel(locale.T("wizard.outbound.label_type")),
		typeSelect,
		widget.NewLabel(locale.T("wizard.outbound.label_comment")),
		commentEntry,
		widget.NewLabel(locale.T("wizard.outbound.label_filters")),
		container.NewGridWithColumns(2, filterKeyLabel, filterValEntry),
		widget.NewLabel(locale.T("wizard.outbound.label_preferred")),
		container.NewGridWithColumns(2, defKeyLabel, defValEntry),
		widget.NewLabel(locale.T("wizard.outbound.label_add_outbounds")),
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

	// Preview tab: uses preview cache from the wizard model (via editPresenter.Model()).
	previewStatusLabel := widget.NewLabel(locale.T("wizard.outbound.preview_switch"))
	type previewRow struct {
		text  string
		color color.Color
	}
	var previewRows []previewRow
	previewList := widget.NewList(
		func() int { return len(previewRows) },
		func() fyne.CanvasObject { return canvas.NewText("", color.White) },
		func(id int, o fyne.CanvasObject) {
			if id < 0 || id >= len(previewRows) {
				return
			}
			if txt, ok := o.(*canvas.Text); ok {
				txt.Text = previewRows[id].text
				txt.Color = previewRows[id].color
			}
		},
	)
	previewListScroll := container.NewScroll(previewList)
	previewListScroll.SetMinSize(fyne.NewSize(400, 320))
	previewContent := container.NewBorder(
		previewStatusLabel,
		nil,
		nil,
		nil,
		previewListScroll,
	)

	buildPreview := func() {
		previewRows = nil
		previewList.Refresh()

		if editPresenter == nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_no_presenter"))
			return
		}
		model := editPresenter.Model()
		if model == nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_model_nil"))
			return
		}

		cfg, err := buildConfigForPreview()
		if err != nil {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_invalid_json"))
			return
		}

		// Ensure preview cache is up to date.
		errorCount, err := wizardbusiness.RebuildPreviewCache(model)
		if err != nil {
			previewStatusLabel.SetText(locale.Tf("wizard.outbound.preview_cache_failed", err))
			return
		}
		allNodes := model.PreviewNodes
		if len(allNodes) == 0 {
			previewStatusLabel.SetText(locale.T("wizard.outbound.preview_no_nodes"))
			return
		}

		var filteredNodes []*config.ParsedNode
		var defaultTag string
		if model.ParserConfig != nil {
			filteredNodes, defaultTag = config.PreviewGlobalSelectorNodes(allNodes, model.ParserConfig.ParserConfig.Proxies, *cfg)
		} else {
			filteredNodes, defaultTag = config.PreviewSelectorNodes(allNodes, *cfg)
		}
		filteredSet := make(map[*config.ParsedNode]bool, len(filteredNodes))
		for _, n := range filteredNodes {
			filteredSet[n] = true
		}

		// Map node pointer to source label using PreviewNodesBySource and ParserConfig.
		sourceLabels := make(map[*config.ParsedNode]string)
		if model.ParserConfig != nil && model.PreviewNodesBySource != nil {
			for si, nodes := range model.PreviewNodesBySource {
				if si < 0 || si >= len(model.ParserConfig.ParserConfig.Proxies) {
					continue
				}
				proxy := model.ParserConfig.ParserConfig.Proxies[si]
				label := proxy.Source
				if label == "" {
					label = locale.T("wizard.outbound.label_source") + fmt.Sprintf("%d", si+1)
				}
				label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
				for _, n := range nodes {
					sourceLabels[n] = label
				}
			}
		}

		// Build rows: default node first, then the rest in original allNodes order.
		defaultRows := make([]previewRow, 0)
		otherRows := make([]previewRow, 0, len(allNodes))

		for _, node := range allNodes {
			inSelector := filteredSet[node]
			isDefault := inSelector && node.Tag == defaultTag

			src := sourceLabels[node]
			if src == "" {
				src = locale.T("wizard.outbound.preview_unknown_source")
			}
			text := node.Tag
			if text == "" {
				if node.Label != "" {
					text = node.Label
				} else if node.Server != "" {
					text = fmt.Sprintf("%s:%d", node.Server, node.Port)
				} else {
					text = node.Scheme
				}
			}
			text = textnorm.NormalizeProxyDisplay(text)
			text = fmt.Sprintf("%s — %s", text, src)
			if isDefault {
				text = "[default] " + text
			}

			var rowColor color.Color
			switch {
			case isDefault:
				rowColor = color.RGBA{R: 0, G: 128, B: 255, A: 255} // blue
			case inSelector:
				rowColor = color.RGBA{R: 0, G: 160, B: 0, A: 255} // green
			default:
				rowColor = color.RGBA{R: 200, G: 0, B: 0, A: 255} // red
			}

			row := previewRow{text: text, color: rowColor}
			if isDefault {
				defaultRows = append(defaultRows, row)
			} else {
				otherRows = append(otherRows, row)
			}
		}

		previewRows = append(defaultRows, otherRows...)
		previewList.Refresh()

		status := locale.Tf("wizard.outbound.preview_status", len(allNodes), len(filteredNodes))
		if defaultTag != "" {
			status += locale.Tf("wizard.outbound.preview_default", defaultTag)
		}
		if len(cfg.AddOutbounds) > 0 {
			status += locale.Tf("wizard.outbound.preview_also_includes", strings.Join(cfg.AddOutbounds, ", "))
		}
		if errorCount > 0 {
			status += locale.Tf("wizard.outbound.preview_source_errors", errorCount)
		}
		previewStatusLabel.SetText(status)
	}

	// syncRawToForm parses the Raw tab JSON and updates Settings form fields (tag, type, comment, filters, etc.).
	// Called when user switches from Raw to Settings so the form reflects the raw JSON.
	syncRawToForm := func() {
		var cfg config.OutboundConfig
		if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
			return // invalid JSON: leave form as is
		}
		if strings.TrimSpace(cfg.Tag) == "" {
			return
		}
		tagEntry.SetText(cfg.Tag)
		if cfg.Type == "urltest" {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_auto"))
		} else {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
		}
		commentEntry.SetText(cfg.Comment)
		filterValEntry.SetText("")
		if cfg.Filters != nil {
			if v, ok := cfg.Filters["tag"]; ok {
				if s, ok := v.(string); ok {
					filterValEntry.SetText(s)
				}
			}
		}
		defValEntry.SetText("")
		if cfg.PreferredDefault != nil {
			if v, ok := cfg.PreferredDefault["tag"]; ok {
				if s, ok := v.(string); ok {
					defValEntry.SetText(s)
				}
			}
		}
		directCheck.SetChecked(false)
		rejectCheck.SetChecked(false)
		for _, c := range otherTagChecks {
			c.SetChecked(false)
		}
		if len(cfg.AddOutbounds) > 0 {
			for _, t := range cfg.AddOutbounds {
				if t == "direct-out" {
					directCheck.SetChecked(true)
				} else if t == "reject" {
					rejectCheck.SetChecked(true)
				} else if c, ok := otherTagsMap[t]; ok {
					c.SetChecked(true)
				}
			}
		}
	}

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("wizard.outbound.tab_settings"), dialogScroll),
		container.NewTabItem(locale.T("wizard.outbound.tab_raw"), rawContainer),
		container.NewTabItem(locale.T("wizard.outbound.tab_preview"), previewContent),
	)
	tabs.OnSelected = func(t *container.TabItem) {
		switch t.Text {
		case locale.T("wizard.outbound.tab_raw"):
			currentTab = "raw"
		case locale.T("wizard.outbound.tab_preview"):
			currentTab = "preview"
			buildPreview()
		default:
			currentTab = "settings"
			syncRawToForm()
		}
	}

	cancelBtn := widget.NewButton(locale.T("wizard.outbound.button_cancel"), func() {
		if dialogWin != nil {
			dialogWin.Close()
		}
	})
	saveBtn := widget.NewButton(locale.T("wizard.outbound.button_save"), func() { save() })

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
