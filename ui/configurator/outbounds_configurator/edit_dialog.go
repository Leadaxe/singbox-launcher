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
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardutils "singbox-launcher/ui/configurator/utils"
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

	// SPEC 058-R-N: для referenced entries (ref != "") body live из template/preset.
	// displayBody — это merged view (template body + active preset patches + USER patch
	// если есть). Используем для populate формы. Для direct entries — это просто
	// existing as-is.
	displayBody := existing
	if existing != nil && existing.Ref != "" && editPresenter != nil {
		// SPEC 058-R-N: для referenced entries (ref != "") body live из template/preset.
		// Используем тот же pipeline что Preview tab — wizardbusiness.ResolveMergedOutbound
		// сначала прогоняет sync + MergeOutboundUpdatesInPlace на копии parserConfig
		// (как parseAndPreview делает для emit), затем возвращает merged entry по tag.
		// Это устраняет дублирование merge-логики и гарантирует что dialog показывает
		// то же что увидит build pipeline.
		if merged := wizardbusiness.ResolveMergedOutbound(editPresenter.Model(), existing.Tag); merged != nil {
			displayBody = merged
		}
	}

	tagEntry := widget.NewEntry()
	if displayBody != nil {
		tagEntry.SetText(displayBody.Tag)
	}
	tagEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_tag"))

	typeSelect := widget.NewSelect([]string{locale.T("wizard.outbound.type_manual"), locale.T("wizard.outbound.type_auto")}, nil)
	if displayBody != nil {
		if displayBody.Type == "urltest" {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_auto"))
		} else {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
		}
	} else {
		typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
	}

	commentEntry := widget.NewEntry()
	if displayBody != nil {
		commentEntry.SetText(displayBody.Comment)
	}
	commentEntry.SetPlaceHolder(locale.T("wizard.outbound.placeholder_comment"))

	// SPEC: editable fields для urltest outbound options (interval/tolerance/url).
	// interval/tolerance — widget.Select (только dropdown, без свободного ввода);
	// url — widget.SelectEntry (dropdown + ручной ввод, т.к. URL разнообразны).
	// В каждый dropdown добавлен placeholder вида `@varname` чтобы юзер мог
	// явно выбрать «inherit from Settings» (значение переменной из state.vars).
	//
	// Visible только когда Type=urltest (toggled via typeSelect.OnChanged ниже).
	curInterval, curTolerance, curURL := "", "", ""
	if displayBody != nil && displayBody.Options != nil {
		if v, ok := displayBody.Options["interval"]; ok {
			curInterval = fmt.Sprintf("%v", v)
		}
		if v, ok := displayBody.Options["tolerance"]; ok {
			curTolerance = fmt.Sprintf("%v", v)
		}
		if v, ok := displayBody.Options["url"]; ok {
			curURL = fmt.Sprintf("%v", v)
		}
	}

	intervalLabels, intervalLabelToValue := templateVarChoices(editPresenter, "urltest_interval", curInterval)
	urltestIntervalSelect := widget.NewSelect(intervalLabels, nil)
	if lbl := labelForValue(intervalLabelToValue, curInterval); lbl != "" {
		urltestIntervalSelect.SetSelected(lbl)
	}

	toleranceLabels, toleranceLabelToValue := templateVarChoices(editPresenter, "urltest_tolerance", curTolerance)
	urltestToleranceSelect := widget.NewSelect(toleranceLabels, nil)
	if lbl := labelForValue(toleranceLabelToValue, curTolerance); lbl != "" {
		urltestToleranceSelect.SetSelected(lbl)
	}

	urlLabels, _ := templateVarChoices(editPresenter, "urltest_url", curURL)
	urltestURLEntry := widget.NewSelectEntry(urlLabels)
	urltestURLEntry.SetPlaceHolder("https://cp.cloudflare.com/generate_204")
	if curURL != "" {
		urltestURLEntry.SetText(curURL)
	}

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
	if displayBody != nil && displayBody.Filters != nil {
		if v, ok := displayBody.Filters["tag"]; ok {
			if s, ok := v.(string); ok {
				filterValEntry.SetText(s)
			}
		} else {
			for _, v := range displayBody.Filters {
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
	if displayBody != nil && displayBody.PreferredDefault != nil {
		if v, ok := displayBody.PreferredDefault["tag"]; ok {
			if s, ok := v.(string); ok {
				defValEntry.SetText(s)
			}
		} else {
			for _, v := range displayBody.PreferredDefault {
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
	if displayBody != nil && len(displayBody.AddOutbounds) > 0 {
		for _, t := range displayBody.AddOutbounds {
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
		if displayBody != nil && displayBody.Options != nil {
			cfg.Options = make(map[string]interface{})
			for k, v := range displayBody.Options {
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

		// SPEC: для urltest перезаписываем interval/tolerance/url из form fields
		// (юзер мог изменить их через urltestBlock виджеты). Перезапись только
		// для urltest type. Для selector — поля скрыты, не применяем.
		//
		// interval/tolerance — widget.Select: .Selected = label, lookup в
		// labelToValue даёт raw value. URL — SelectEntry: читаем .Text напрямую
		// (юзер мог ввести custom URL).
		if obType == "urltest" {
			if cfg.Options == nil {
				cfg.Options = map[string]interface{}{}
			}
			if lbl := urltestIntervalSelect.Selected; lbl != "" {
				if v, ok := intervalLabelToValue[lbl]; ok && v != "" {
					cfg.Options["interval"] = v
				}
			}
			if lbl := urltestToleranceSelect.Selected; lbl != "" {
				if v, ok := toleranceLabelToValue[lbl]; ok && v != "" {
					// tolerance — число в template; placeholder @urltest_tolerance
					// оставляем строкой (substituter резолвит на build time).
					if strings.HasPrefix(v, "@") {
						cfg.Options["tolerance"] = v
					} else if n, err := strconv.Atoi(v); err == nil {
						cfg.Options["tolerance"] = n
					} else {
						cfg.Options["tolerance"] = v
					}
				}
			}
			if v := strings.TrimSpace(urltestURLEntry.Text); v != "" {
				cfg.Options["url"] = v
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

	// SPEC 058-R-N: applyEditedConfig.
	// Для direct entries (existing.Ref=="") — body inline, copy existing's Updates
	// (если есть юзерские правки накопленные — preserve).
	// Для referenced entries (existing.Ref!="") — вычисляем diff cfg → merged_base
	// и обновляем USER patch в updates[]. Body fields в cfg не идут в save (referenced
	// entries thin — body live из template/preset).
	applyEditedConfig := func(cfg *config.OutboundConfig) {
		if existing == nil {
			return
		}
		cfg.Ref = existing.Ref
		if cfg.Ref == "" {
			// Direct entry: preserve existing Updates (на случай legacy с USER patch).
			if len(existing.Updates) > 0 {
				cfg.Updates = append([]config.OutboundUpdate(nil), existing.Updates...)
			}
			return
		}
		// Referenced entry: diff cfg против merged_base без USER patch.
		var td *template.TemplateData
		if editPresenter != nil {
			if m := editPresenter.Model(); m != nil {
				td = m.TemplateData
			}
		}
		// merged_base = resolved template/preset body + active preset patches
		// (без USER patch — он и есть результат этого edit).
		baseEntry := *existing
		baseEntry.Updates = filterOutUserPatch(existing.Updates)
		mergedBase := build.MergeOutboundUpdates(baseEntry, td)
		diff := build.OutboundFieldDiff(*cfg, mergedBase)
		// updates[] = existing preset patches + новый USER patch (или без него если diff пуст).
		cfg.Updates = build.UpsertUserPatch(
			append([]config.OutboundUpdate(nil), baseEntry.Updates...),
			diff,
		)
		// Strip body fields — referenced entries thin.
		stripDirectBodyForReferenced(cfg)
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
			// SPEC 057-R-N: Raw tab показывает ref/updates юзеру (они в JSON),
			// но юзерский edit мог их случайно изменить/удалить. Преимущество
			// state-managed полей: оверрайдим тем что в state, игнорируем raw edit.
			applyEditedConfig(&cfg)
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

		// SPEC 057-R-N: preserve preset binding (Form tab их не показывает,
		// но они должны "пережить" Form-edit).
		applyEditedConfig(cfg)
		onSave(cfg, scopeKind, idx)
		if dialogWin != nil {
			dialogWin.Close()
		}
	}

	// Urltest-specific options block. Видим только когда Type=urltest.
	// Tooltip объясняет что @varname placeholder означает inherit из Settings tab.
	const urltestTooltip = "Pick a preset value or select @varname to inherit the value from Settings tab (substituted at build time)."
	urltestLabel := widget.NewLabel("URLTest options")
	urltestIntervalLabel := ttwidget.NewLabel("Interval")
	urltestIntervalLabel.SetToolTip(urltestTooltip)
	urltestToleranceLabel := ttwidget.NewLabel("Tolerance (ms)")
	urltestToleranceLabel.SetToolTip(urltestTooltip)
	urltestURLLabel := ttwidget.NewLabel("URL")
	urltestURLLabel.SetToolTip(urltestTooltip)
	urltestBlock := container.NewVBox(
		urltestLabel,
		container.NewGridWithColumns(2, urltestIntervalLabel, urltestIntervalSelect),
		container.NewGridWithColumns(2, urltestToleranceLabel, urltestToleranceSelect),
		container.NewGridWithColumns(2, urltestURLLabel, urltestURLEntry),
	)
	urltestVisible := func() {
		isAuto := typeSelect.Selected == locale.T("wizard.outbound.type_auto")
		if isAuto {
			urltestBlock.Show()
		} else {
			urltestBlock.Hide()
		}
	}
	urltestVisible() // initial state
	prevOnTypeChanged := typeSelect.OnChanged
	typeSelect.OnChanged = func(s string) {
		urltestVisible()
		if prevOnTypeChanged != nil {
			prevOnTypeChanged(s)
		}
	}

	form := container.NewVBox(
		widget.NewLabel(locale.T("wizard.outbound.label_scope")),
		scopeSelect,
		widget.NewLabel(locale.T("wizard.outbound.label_tag_field")),
		tagEntry,
		widget.NewLabel(locale.T("wizard.outbound.label_type")),
		typeSelect,
		urltestBlock,
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

		// SPEC 057-R-N: preview должен показывать final emit. Form/Raw отдают
		// base body (без Updates[] стека), но emit применяет patches от preset'ов.
		// Подмешиваем Updates от existing → merge → preview через final body.
		// Без этого preview proxy-out не отфильтрует RU ноды (filters лежат в
		// Updates[].patch, а cfg.Filters пуст), хотя в config.json фильтр сработает.
		if existing != nil && len(existing.Updates) > 0 {
			cfg.Updates = append([]config.OutboundUpdate(nil), existing.Updates...)
			var td *template.TemplateData
			if editPresenter != nil {
				if m := editPresenter.Model(); m != nil {
					td = m.TemplateData
				}
			}
			merged := build.MergeOutboundUpdates(*cfg, td)
			cfg = &merged
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
	//
	// SPEC 058-R-N: для referenced entries (cfg.Ref != "") Raw содержит thin
	// shape (tag+ref+updates без body) — populate из этого даст пустую форму.
	// Re-merge с template: build.MergeOutboundUpdates резолвит base body и
	// applies updates → получаем full merged view для populate.
	syncRawToForm := func() {
		var cfg config.OutboundConfig
		if err := json.Unmarshal([]byte(rawEntry.Text), &cfg); err != nil {
			return // invalid JSON: leave form as is
		}
		if strings.TrimSpace(cfg.Tag) == "" {
			return
		}
		// Re-merge для referenced entries — иначе форма обнуляется.
		display := cfg
		if cfg.Ref != "" && editPresenter != nil {
			if m := editPresenter.Model(); m != nil {
				display = build.MergeOutboundUpdates(cfg, m.TemplateData)
			}
		}
		tagEntry.SetText(display.Tag)
		if display.Type == "urltest" {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_auto"))
		} else {
			typeSelect.SetSelected(locale.T("wizard.outbound.type_manual"))
		}
		commentEntry.SetText(display.Comment)
		filterValEntry.SetText("")
		if display.Filters != nil {
			if v, ok := display.Filters["tag"]; ok {
				if s, ok := v.(string); ok {
					filterValEntry.SetText(s)
				}
			}
		}
		defValEntry.SetText("")
		if display.PreferredDefault != nil {
			if v, ok := display.PreferredDefault["tag"]; ok {
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
		if len(display.AddOutbounds) > 0 {
			for _, t := range display.AddOutbounds {
				if t == "direct-out" {
					directCheck.SetChecked(true)
				} else if t == "reject" {
					rejectCheck.SetChecked(true)
				} else if c, ok := otherTagsMap[t]; ok {
					c.SetChecked(true)
				}
			}
		}
		// urltest fields — для referenced merged.Options содержит финальные
		// значения (template + preset patches + USER patch).
		if display.Options != nil {
			if v, ok := display.Options["interval"]; ok {
				if lbl := labelForValue(intervalLabelToValue, fmt.Sprintf("%v", v)); lbl != "" {
					urltestIntervalSelect.SetSelected(lbl)
				}
			}
			if v, ok := display.Options["tolerance"]; ok {
				if lbl := labelForValue(toleranceLabelToValue, fmt.Sprintf("%v", v)); lbl != "" {
					urltestToleranceSelect.SetSelected(lbl)
				}
			}
			if v, ok := display.Options["url"]; ok {
				urltestURLEntry.SetText(fmt.Sprintf("%v", v))
			}
		}
	}

	// syncFormToRaw — собирает OutboundConfig из текущего состояния формы
	// и кладёт его JSON в rawEntry. Вызывается при переключении Settings → Raw.
	//
	// SPEC 058-R-N: Raw view показывает SAVE-shape (что реально попадёт в state),
	// не resolved/merged body. Для referenced entries (ref != "") это означает:
	// thin tag+ref + Updates с USER patch (diff формы vs merged_base). Юзер видит
	// то же что и save(), без иллюзии full body.
	syncFormToRaw := func() {
		if currentTab != "settings" {
			return
		}
		cfg, err := buildConfigForPreview()
		if err != nil || cfg == nil {
			return
		}
		// applyEditedConfig делает: для referenced — diff vs merged_base + USER
		// patch + strip body; для direct — preserve Updates + full body.
		applyEditedConfig(cfg)
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return
		}
		rawEntry.SetText(string(b))
	}

	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("wizard.outbound.tab_settings"), dialogScroll),
		container.NewTabItem(locale.T("wizard.outbound.tab_raw"), rawContainer),
		container.NewTabItem(locale.T("wizard.outbound.tab_preview"), previewContent),
	)
	tabs.OnSelected = func(t *container.TabItem) {
		switch t.Text {
		case locale.T("wizard.outbound.tab_raw"):
			// Settings → Raw: материализуем правки формы в JSON.
			syncFormToRaw()
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
			fynetooltip.DestroyWindowToolTipLayer(dialogWin.Canvas())
			editPresenter.ClearOutboundEditWindow()
			editPresenter.UpdateChildOverlay()
		})
	}
	dialogWin.Resize(fyne.NewSize(440, 560))
	dialogWin.CenterOnScreen()
	// fynetooltip layer обязателен для tooltips на ttwidget виджетах в
	// отдельном окне — без него fyne-tooltip пишет "no tool tip layer
	// created for current overlay" и tooltips не показываются.
	dialogWin.SetContent(fynetooltip.AddWindowToolTipLayer(mainContent, dialogWin.Canvas()))
	dialogWin.Show()
	if editPresenter != nil {
		editPresenter.UpdateChildOverlay()
	}
}

// filterOutUserPatch returns Updates with USER patch entry removed (preset
// patches kept). Используется в diff computation: merged_base = template body +
// active preset patches (БЕЗ USER patch — он и есть результат текущего edit).
// templateVarChoices строит (labels, labelToValue) для dropdown'а на основе
// template var. Семантика:
//   - labels[0] = "@varname" placeholder — позволяет выбрать «inherit from
//     Settings» (substituter резолвит в текущее значение state.vars[var]).
//   - labels[1..] = OptionTitles если есть, иначе raw values (mirror того что
//     Settings tab показывает).
//   - currentValue добавляется в конец списка если не matchится ни с placeholder
//     ни с template options — preserve юзерское custom value (например, юзер
//     раньше ввёл нестандартное "7m" — не теряем).
//   - labelToValue: label → raw value (для save mapping).
func templateVarChoices(editPresenter OutboundEditPresenter, varName, currentValue string) ([]string, map[string]string) {
	placeholder := "@" + varName
	labels := []string{placeholder}
	labelToValue := map[string]string{placeholder: placeholder}

	if editPresenter != nil {
		if m := editPresenter.Model(); m != nil && m.TemplateData != nil {
			for _, v := range m.TemplateData.Vars {
				if v.Name != varName {
					continue
				}
				for i, opt := range v.Options {
					label := opt
					if i < len(v.OptionTitles) && v.OptionTitles[i] != "" {
						label = v.OptionTitles[i]
					}
					labels = append(labels, label)
					labelToValue[label] = opt
				}
				break
			}
		}
	}

	// Preserve custom currentValue если не среди известных options/placeholder.
	if currentValue != "" {
		found := false
		for _, val := range labelToValue {
			if val == currentValue {
				found = true
				break
			}
		}
		if !found {
			labels = append(labels, currentValue)
			labelToValue[currentValue] = currentValue
		}
	}
	return labels, labelToValue
}

// labelForValue ищет label соответствующий значению value в map. Возвращает
// первый matching label или пустую строку.
func labelForValue(labelToValue map[string]string, value string) string {
	for label, val := range labelToValue {
		if val == value {
			return label
		}
	}
	return ""
}

func filterOutUserPatch(updates []config.OutboundUpdate) []config.OutboundUpdate {
	if len(updates) == 0 {
		return nil
	}
	out := make([]config.OutboundUpdate, 0, len(updates))
	for _, u := range updates {
		if u.Ref == config.RefUser {
			continue
		}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stripDirectBodyForReferenced — referenced entries (ref != "") хранят thin
// shape: только tag + ref + updates. Body fields обнуляются (live из template/preset).
func stripDirectBodyForReferenced(cfg *config.OutboundConfig) {
	if cfg == nil || cfg.Ref == "" {
		return
	}
	cfg.Type = ""
	cfg.Options = nil
	cfg.Filters = nil
	cfg.AddOutbounds = nil
	cfg.PreferredDefault = nil
	cfg.Comment = ""
}
