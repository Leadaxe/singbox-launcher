package tabs

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	v5 "singbox-launcher/core/state/v5"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Min heights for Source Edit dialog tab bodies (child window; do not use main window canvas before Show).
const (
	sourceEditSettingsScrollMinH float32 = 260
	sourceEditJSONScrollMinH     float32 = 380
)

func showWizardTagConflictError(win fyne.Window) {
	dialog.ShowError(errors.New(locale.T("wizard.source.wizard_tag_conflict")), win)
}

func setFyneWidgetToolTip(w fyne.CanvasObject, tip string) {
	if tb, ok := interface{}(w).(interface{ SetToolTip(string) }); ok {
		tb.SetToolTip(tip)
	}
}

// formatLocalOutboundPreviewLine is a one-line summary for proxies[i].outbounds[] in the Edit → Preview tab.
func formatLocalOutboundPreviewLine(ob *config.OutboundConfig) string {
	if ob == nil {
		return ""
	}
	typ := ob.Type
	if typ == "" {
		typ = "?"
	}
	comment := strings.TrimSpace(ob.Comment)
	rs := []rune(comment)
	const maxR = 96
	if len(rs) > maxR {
		comment = string(rs[:maxR-1]) + "…"
	}
	if ob.Tag == "" {
		return fmt.Sprintf("[%s]  %s", typ, comment)
	}
	return fmt.Sprintf("%s  [%s]  %s", ob.Tag, typ, comment)
}

// parsePreviewNodesFromBody — простой парсер decoded body для Preview tab:
// идём построчно, парсим URI'ы. Не network, не tag-prefix (preview-only).
func parsePreviewNodesFromBody(body []byte, skip []map[string]string) []*config.ParsedNode {
	out := make([]*config.ParsedNode, 0)
	tagCounts := make(map[string]int)
	contentStr := strings.ReplaceAll(string(body), "\r\n", "\n")
	contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
	for _, line := range strings.Split(contentStr, "\n") {
		line = subscription.NormalizeSubscriptionTextLine(line)
		if line == "" {
			continue
		}
		node, perr := subscription.ParseNode(line, skip)
		if perr != nil || node == nil {
			continue
		}
		node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
		out = append(out, node)
		if len(out) >= 200 {
			break
		}
	}
	return out
}

// applyProxyEditToSource — SPEC 052 phase 8: переносит изменения в widget'е
// edit-окна (которое мутирует *config.ProxySource scratch-буфер) обратно
// в canonical `model.Sources[sourceIndex]`.
//
// Маппинг ProxySource → Source:
//   - subscription: URL/Skip/Outbounds/ExcludeFromGlobal/ExposeGroupTagsToGlobal/
//     Enabled (=!Disabled) + Tag из TagPrefix/Postfix/Mask;
//   - server: URI=Connections[0], Label=TagMask (corestate adapter ставит
//     `tag_mask=label` для server-source — тэг node будет точно label).
func applyProxyEditToSource(ps *config.ProxySource, src *wizardmodels.Source) {
	if ps == nil || src == nil {
		return
	}
	if ps.Source != "" {
		// subscription
		src.Type = wizardmodels.SourceTypeSubscription
		src.URL = ps.Source
		src.URI = ""
		src.Skip = ps.Skip
		src.Outbounds = append([]configtypes.OutboundConfig(nil), ps.Outbounds...)
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.ExposeGroupTagsToGlobal = ps.ExposeGroupTagsToGlobal
		src.Enabled = !ps.Disabled
		if ps.TagPrefix != "" || ps.TagPostfix != "" || ps.TagMask != "" {
			src.Tag = &wizardmodels.TagSpec{
				Prefix:  ps.TagPrefix,
				Postfix: ps.TagPostfix,
				Mask:    ps.TagMask,
			}
		} else {
			src.Tag = nil
		}
	} else if len(ps.Connections) > 0 {
		// server
		src.Type = wizardmodels.SourceTypeServer
		src.URI = ps.Connections[0]
		src.URL = ""
		// Если widget'ом задан tag_mask — это и есть label для server.
		if ps.TagMask != "" {
			src.Label = ps.TagMask
		}
		src.Enabled = !ps.Disabled
		src.ExcludeFromGlobal = ps.ExcludeFromGlobal
		src.Outbounds = nil
		src.Tag = nil
	}
}

func serializeParserAfterSourceEdit(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	m *wizardmodels.WizardModel,
	sourceIndex int,
	scratch *config.ProxySource,
	errParent fyne.Window,
) error {
	if scratch != nil && sourceIndex >= 0 && sourceIndex < len(m.Sources) {
		applyProxyEditToSource(scratch, &m.Sources[sourceIndex])
	}
	m.RefreshDerivedParserConfig()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(m)
	presenter.UpdateParserConfig(m.ParserConfigJSON)
	presenter.ScheduleRefreshOutboundOptionsDebounced()
	presenter.MarkAsChanged()
	if guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
	return nil
}

// showSourceEditWindow opens Settings | Preview | JSON for one proxy source (SPEC 026).
func showSourceEditWindow(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	parent fyne.Window,
	sourceIndex int,
	shortLabel string,
) {
	if presenter == nil {
		return
	}
	// One modal child workflow: finish Outbound Edit or another Source Edit (View slot) first.
	if w := presenter.OpenOutboundEditWindow(); w != nil {
		w.RequestFocus()
		return
	}
	if w := presenter.OpenViewWindow(); w != nil {
		w.RequestFocus()
		return
	}
	presenter.MergeGUIToModel()

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	m := presenter.Model()
	if m == nil {
		return
	}
	if sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}

	// Window title — берём полный URL/Label без обрезки; OS title-bar
	// сам ellipsis'ит до доступной ширины (избегаем двойного "...").
	mm := presenter.Model()
	fullTitleSrc := shortLabel
	if mm != nil && sourceIndex < len(mm.Sources) {
		s := mm.Sources[sourceIndex]
		switch s.Type {
		case wizardmodels.SourceTypeSubscription:
			if s.Meta != nil && strings.TrimSpace(s.Meta.ProfileTitle) != "" {
				fullTitleSrc = s.Meta.ProfileTitle
			} else if s.URL != "" {
				fullTitleSrc = s.URL
			}
		case wizardmodels.SourceTypeServer:
			if s.Label != "" {
				fullTitleSrc = s.Label
			} else if s.URI != "" {
				fullTitleSrc = s.URI
			}
		}
	}
	title := locale.Tf("wizard.source.edit_title", fullTitleSrc)
	win := app.NewWindow(title)
	presenter.SetViewWindow(win)
	win.SetOnClosed(func() {
		presenter.ClearViewWindow()
		presenter.UpdateChildOverlay()
	})

	// SPEC 052 phase 8: scratch ProxySource — derived из Sources[i] на open;
	// widget'ы мутируют его in-place; на сохранение → applyProxyEditToSource
	// синхронизирует обратно в canonical Sources[i].
	scratch := m.Sources[sourceIndex].ToProxySourceV4()
	proxyRef := func() *config.ProxySource {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return nil
		}
		return &scratch
	}

	prefixEntry := widget.NewEntry()
	prefixEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_prefix"))

	// SPEC 052 phase 8: URL/URI/Label/Postfix/Mask editors теперь доступны
	// в Settings tab. URL/Postfix/Mask показываются только для subscription;
	// URI/Label — только для server. Все мутации идут через scratch + Source.
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/sub")

	uriEntry := widget.NewEntry()
	uriEntry.SetPlaceHolder("vless://uuid@host:443?...#tokyo")

	labelEntry := widget.NewEntry()
	labelEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_label"))

	postfixEntry := widget.NewEntry()
	postfixEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_postfix"))

	maskEntry := widget.NewEntry()
	maskEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_mask"))

	autoCheck := widget.NewCheck(locale.T("wizard.source.local_auto"), nil)
	selectCheck := widget.NewCheck(locale.T("wizard.source.local_select"), nil)
	excludeCheck := widget.NewCheck(locale.T("wizard.source.exclude_global"), nil)
	exposeCheck := widget.NewCheck(locale.T("wizard.source.expose_tags"), nil)
	hintLabel := widget.NewLabel("")
	hintLabel.Wrapping = fyne.TextWrapWord

	var afterSync func()

	var exposeOnChanged func(bool)
	exposeOnChanged = func(v bool) {
		if exposeCheck.Disabled() {
			return
		}
		pp := proxyRef()
		if pp == nil {
			return
		}
		pp.ExposeGroupTagsToGlobal = v
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		if afterSync != nil {
			afterSync()
		}
	}
	exposeCheck.OnChanged = exposeOnChanged

	refreshExposeAvailability := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		has := wizardbusiness.ProxyHasLocalAuto(p) || wizardbusiness.ProxyHasLocalSelect(p)
		exposeCheck.OnChanged = nil
		if has {
			exposeCheck.Enable()
			exposeCheck.SetChecked(p.ExposeGroupTagsToGlobal)
		} else {
			exposeCheck.Disable()
			exposeCheck.SetChecked(false)
		}
		exposeCheck.OnChanged = exposeOnChanged
		tip := locale.T("wizard.source.expose_tags_tooltip")
		if has {
			tip = ""
		}
		setFyneWidgetToolTip(exposeCheck, tip)
	}

	refreshExcludeHint := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		if p.ExcludeFromGlobal && (!wizardbusiness.ProxyHasLocalAuto(p) || !wizardbusiness.ProxyHasLocalSelect(p)) {
			hintLabel.SetText(locale.T("wizard.source.exclude_hint"))
			hintLabel.Show()
		} else {
			hintLabel.SetText("")
			hintLabel.Hide()
		}
	}

	syncFormFromModel := func() {
		p := proxyRef()
		if p == nil {
			return
		}
		urlEntry.SetText(p.Source)
		prefixEntry.SetText(p.TagPrefix)
		postfixEntry.SetText(p.TagPostfix)
		maskEntry.SetText(p.TagMask)
		// URI / Label — для server-type.
		uriText := ""
		if len(p.Connections) > 0 {
			uriText = p.Connections[0]
		}
		uriEntry.SetText(uriText)
		// Label берём из Source напрямую (scratch не имеет Label-поля).
		mm := presenter.Model()
		if mm != nil && sourceIndex < len(mm.Sources) {
			labelEntry.SetText(mm.Sources[sourceIndex].Label)
		}
		autoCheck.SetChecked(wizardbusiness.ProxyHasLocalAuto(p))
		selectCheck.SetChecked(wizardbusiness.ProxyHasLocalSelect(p))
		excludeCheck.SetChecked(p.ExcludeFromGlobal)
		refreshExposeAvailability()
		refreshExcludeHint()
		if afterSync != nil {
			afterSync()
		}
	}

	urlEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.Source = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	uriEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		s = strings.TrimSpace(s)
		if s == "" {
			p.Connections = nil
		} else {
			p.Connections = []string{s}
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	labelEntry.OnChanged = func(s string) {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			return
		}
		mm.Sources[sourceIndex].Label = strings.TrimSpace(s)
		// Для server-type: Label также используется как TagMask в derived
		// view (см. ToProxySourceV4) — синхронизируем scratch.
		if mm.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer {
			scratch.TagMask = strings.TrimSpace(s)
		}
		mm.RefreshDerivedParserConfig()
		presenter.UpdateParserConfig(mm.ParserConfigJSON)
		presenter.MarkAsChanged()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
	}

	prefixEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagPrefix = strings.TrimSpace(s)
		wizardbusiness.RenameWizardLocalOutboundTags(p, sourceIndex)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	postfixEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagPostfix = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	maskEntry.OnChanged = func(s string) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.TagMask = strings.TrimSpace(s)
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
	}

	autoCheck.OnChanged = func(on bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		if on {
			if err := wizardbusiness.EnsureLocalAuto(p, sourceIndex); err != nil {
				autoCheck.SetChecked(false)
				showWizardTagConflictError(win)
				return
			}
		} else {
			wizardbusiness.RemoveWizardSelectOutbounds(p)
			wizardbusiness.RemoveWizardAutoOutbounds(p)
			wizardbusiness.SyncExposeFlagWhenNoLocalGroups(p)
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	selectCheck.OnChanged = func(on bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		if on {
			if err := wizardbusiness.EnsureLocalSelect(p, sourceIndex); err != nil {
				selectCheck.SetChecked(false)
				showWizardTagConflictError(win)
				return
			}
		} else {
			wizardbusiness.RemoveWizardSelectOutbounds(p)
			wizardbusiness.SyncExposeFlagWhenNoLocalGroups(p)
		}
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		syncFormFromModel()
	}

	excludeCheck.OnChanged = func(v bool) {
		p := proxyRef()
		if p == nil {
			return
		}
		p.ExcludeFromGlobal = v
		_ = serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win)
		refreshExcludeHint()
		if afterSync != nil {
			afterSync()
		}
	}

	// SPEC 052 phase 8: Settings tab type-conditional. Subscription и server
	// показывают разные блоки полей.
	settingsContent := container.NewVBox()
	rebuildSettingsLayout := func() {
		settingsContent.Objects = settingsContent.Objects[:0]
		mm := presenter.Model()
		isServer := mm != nil && sourceIndex < len(mm.Sources) && mm.Sources[sourceIndex].Type == wizardmodels.SourceTypeServer

		if isServer {
			// Server: URI + Label + ExcludeFromGlobal.
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_uri")))
			settingsContent.Add(uriEntry)
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_label_field")))
			settingsContent.Add(labelEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(excludeCheck)
		} else {
			// Subscription: URL + Tag prefix/postfix/mask + auto/select/exclude/expose.
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_url_edit")))
			settingsContent.Add(urlEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_prefix")))
			settingsContent.Add(prefixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_postfix")))
			settingsContent.Add(postfixEntry)
			settingsContent.Add(widget.NewLabel(locale.T("wizard.source.label_mask")))
			settingsContent.Add(maskEntry)
			settingsContent.Add(widget.NewSeparator())
			settingsContent.Add(autoCheck)
			settingsContent.Add(selectCheck)
			settingsContent.Add(excludeCheck)
			settingsContent.Add(exposeCheck)
			settingsContent.Add(hintLabel)
		}
		settingsContent.Refresh()
	}
	rebuildSettingsLayout()
	settingsScroll := container.NewVScroll(settingsContent)
	settingsScroll.SetMinSize(fyne.NewSize(0, sourceEditSettingsScrollMinH))
	settingsGutter := canvas.NewRectangle(color.Transparent)
	settingsGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	settingsWithGutter := container.NewBorder(nil, nil, nil, settingsGutter, settingsScroll)

	previewStatus := widget.NewLabel(locale.T("wizard.source.preview_loading"))
	previewStatus.Wrapping = fyne.TextWrapOff
	previewStatusScroll := container.NewHScroll(previewStatus)
	previewListHost := container.NewMax()
	previewGutter := canvas.NewRectangle(color.Transparent)
	previewGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	previewBox := container.NewBorder(previewStatusScroll, nil, nil, previewGutter, previewListHost)

	previewRefreshSeq := 0
	refreshPreviewTab := func() {
		previewRefreshSeq++
		seq := previewRefreshSeq
		previewStatus.SetText(locale.T("wizard.source.preview_loading"))
		previewListHost.Objects = nil
		previewListHost.Add(layout.NewSpacer())
		previewListHost.Refresh()
		go func() {
			model := presenter.Model()
			var nodes []*config.ParsedNode
			var err error

			// SPEC 052 phase 8 preview pipeline:
			//   - server-source: parse URI напрямую (без сети);
			//   - subscription с .raw на диске: декодим cached body;
			//   - subscription без .raw: fallback на network fetch.
			if model != nil && sourceIndex < len(model.Sources) {
				src := model.Sources[sourceIndex]
				switch src.Type {
				case wizardmodels.SourceTypeServer:
					if src.URI != "" {
						node, perr := subscription.ParseNode(src.URI, nil)
						if perr != nil {
							err = perr
						} else if node != nil {
							node.Tag = src.Label
							nodes = []*config.ParsedNode{node}
						}
					}
				case wizardmodels.SourceTypeSubscription:
					subsDir := platform.GetSubscriptionsDir(model.ExecDir)
					if raw, rerr := v5.ReadRawBody(subsDir, src.ID); rerr == nil && len(raw) > 0 {
						decoded, decErr := subscription.DecodeSubscriptionContent(raw)
						if decErr != nil {
							err = decErr
						} else {
							pp := proxyRef()
							skip := []map[string]string(nil)
							if pp != nil {
								skip = pp.Skip
							}
							nodes = parsePreviewNodesFromBody(decoded, skip)
						}
					} else {
						// Нет кэша → пользователь должен нажать Refresh per-source.
						// Не дёргаем сеть автоматически (это приводит к "Loading..." на 30+ сек).
						err = fmt.Errorf("no cached body — press Refresh to fetch this subscription")
					}
				}
			}
			fyne.Do(func() {
				if seq != previewRefreshSeq {
					return
				}
				previewListHost.Objects = nil
				if err != nil {
					previewStatus.SetText(locale.Tf("wizard.source.preview_status_err", 0, err.Error()))
				} else {
					previewStatus.SetText(locale.Tf("wizard.source.preview_servers", len(nodes), 1))
				}
				body := container.NewVBox()
				if err == nil {
					if len(nodes) == 0 {
						body.Add(widget.NewLabel(locale.T("wizard.source.view_no_servers")))
					} else {
						nn := nodes
						srvList := widget.NewList(
							func() int { return len(nn) },
							func() fyne.CanvasObject { return widget.NewLabel("") },
							func(id int, o fyne.CanvasObject) {
								o.(*widget.Label).SetText(nodeDisplayLine(nn[id]))
							},
						)
						sc := container.NewScroll(srvList)
						sc.SetMinSize(fyne.NewSize(0, 280))
						body.Add(sc)
					}
				}
				previewListHost.Add(container.NewVScroll(body))
				previewListHost.Refresh()
			})
		}()
	}

	// JSON: same pattern as wizard Preview tab — MultiLineEntry inside Max + VScroll (no duplicate tab title).
	jsonEntry := widget.NewMultiLineEntry()
	jsonEntry.Wrapping = fyne.TextWrapOff
	jsonEntry.OnChanged = func(string) { /* display-only; changes are not saved */ }
	jsonScroll := container.NewVScroll(container.NewMax(
		canvas.NewRectangle(color.Transparent),
		jsonEntry,
	))
	jsonScroll.SetMinSize(fyne.NewSize(0, sourceEditJSONScrollMinH))

	// SPEC 052 phase 8: JSON tab показывает v5 Source layout (canonical),
	// а не legacy ProxySource (derived). Это match'ит state.json формат.
	refreshJSONTab := func() {
		mm := presenter.Model()
		if mm == nil || sourceIndex >= len(mm.Sources) {
			jsonEntry.SetText("")
			return
		}
		src := mm.Sources[sourceIndex]
		b, jerr := json.MarshalIndent(src, "", "  ")
		if jerr != nil {
			jsonEntry.SetText(jerr.Error())
			return
		}
		jsonEntry.SetText(string(b))
	}

	jsonHint := widget.NewLabel(locale.T("wizard.source.json_hint"))
	jsonHint.Wrapping = fyne.TextWrapWord
	jsonGutter := canvas.NewRectangle(color.Transparent)
	jsonGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	jsonScrollWithGutter := container.NewBorder(nil, nil, nil, jsonGutter, jsonScroll)
	jsonCol := container.NewVBox(jsonHint, jsonScrollWithGutter)

	// SPEC 052 phase 8: Overview-tab включает raw body section (раньше был
	// отдельный Raw tab — слили чтобы не дублировать read-only inspection).
	overviewContent, refreshOverviewTab := buildOverviewTab(presenter, sourceIndex)

	settingsTab := container.NewTabItem(locale.T("wizard.source.tab_settings"), settingsWithGutter)
	previewTab := container.NewTabItem(locale.T("wizard.source.tab_preview"), previewBox)
	overviewTab := container.NewTabItem(locale.T("wizard.source.tab_overview"), overviewContent)
	jsonTab := container.NewTabItem(locale.T("wizard.source.tab_json"), jsonCol)
	tabs := container.NewAppTabs(settingsTab, previewTab, overviewTab, jsonTab)
	afterSync = func() {
		if tabs.Selected() == overviewTab {
			refreshOverviewTab()
		}
		if tabs.Selected() == previewTab {
			refreshPreviewTab()
		}
		if tabs.Selected() == jsonTab {
			refreshJSONTab()
		}
	}
	tabs.OnSelected = func(ti *container.TabItem) {
		switch ti {
		case overviewTab:
			refreshOverviewTab()
		case previewTab:
			refreshPreviewTab()
		case jsonTab:
			refreshJSONTab()
		}
	}

	cancelBtn := widget.NewButton(locale.T("wizard.outbound.button_cancel"), func() {
		win.Close()
	})
	saveBtn := widget.NewButton(locale.T("wizard.outbound.button_save"), func() {
		if err := serializeParserAfterSourceEdit(presenter, guiState, presenter.Model(), sourceIndex, &scratch, win); err != nil {
			return
		}
		win.Close()
	})
	buttonsRow := container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn)
	root := container.NewBorder(nil, buttonsRow, nil, nil, tabs)

	win.SetContent(root)
	win.Resize(fyne.NewSize(880, 600))
	win.CenterOnScreen()
	syncFormFromModel()
	win.Show()
	presenter.UpdateChildOverlay()
}
