// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл source_tab.go содержит функции, создающие UI табов визарда:
//   - Вкладка Sources: ввод URL, проверка, список источников; объединённый превью серверов — в отдельном окне
//   - Вкладка Outbounds and ParserConfig: редактор ParserConfig JSON и вход в конфигуратор outbounds
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
//
// Используется в:
//   - configurator.go - при создании окна конфигуратора вызывается CreateSourcesTab(presenter)
//
// Взаимодействует с:
//   - presenter - все действия пользователя (нажатия кнопок, ввод текста) обрабатываются через методы presenter
//   - business - AppendURLsToSources по кнопке Add; список источников из model.Sources (canonical v5)
package tabs

import (
	"encoding/json"
	"fmt"
	"strings"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizarddialogs "singbox-launcher/ui/configurator/dialogs"
	"singbox-launcher/ui/configurator/outbounds_configurator"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardutils "singbox-launcher/ui/configurator/utils"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// CreateSourcesTab creates the Sources tab UI (URLs, URL status and preview).
func CreateSourcesTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	const directLinksDocURL = "https://github.com/Leadaxe/singbox-launcher/blob/6beb136b9082823699c6509d32e62f212fd7ff90/docs/ParserConfig.md#%D1%84%D0%BE%D1%80%D0%BC%D0%B0%D1%82%D1%8B-uri-%D0%B4%D0%BB%D1%8F-%D0%BF%D1%80%D1%8F%D0%BC%D1%8B%D1%85-%D1%81%D1%81%D1%8B%D0%BB%D0%BE%D0%BA"

	// Section 1: Subscription URL or Direct Links
	urlLabel := widget.NewLabel(locale.T("wizard.source.label_url"))
	urlLabel.Importance = widget.MediumImportance

	guiState.SourceURLEntry = widget.NewMultiLineEntry()
	guiState.SourceURLEntry.SetPlaceHolder(locale.T("wizard.source.placeholder_url"))
	guiState.SourceURLEntry.Wrapping = fyne.TextWrapOff
	// No automatic application: URLs are applied only when the user clicks Add.
	guiState.SourceURLEntry.OnChanged = func(value string) {
		if guiState.SourceURLsProgrammatic {
			return
		}
		presenter.Model().PreviewNeedsParse = true
		presenter.MarkAsChanged()
	}

	hintLabel := widget.NewLabel(locale.T("wizard.source.hint"))
	hintLabel.Wrapping = fyne.TextWrapWord
	wireguardHelpButton := widget.NewButton("?", func() {
		if err := platform.OpenURL(directLinksDocURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open docs: %w", err), guiState.Window)
		}
	})
	wireguardHelpButton.Importance = widget.LowImportance
	// Keep help button compact (single-symbol width) and pinned to the right.
	helpButtonCompact := container.NewGridWrap(fyne.NewSize(24, 24), wireguardHelpButton)
	hintRow := container.NewBorder(nil, nil, nil, helpButtonCompact, hintLabel)
	// applyAddedSources runs the shared Add path: parse `text` (URI links /
	// vpn:// / [Interface]/[Peer] conf) into sources, refresh UI, clear the
	// field. Used by both the Add button and Add-from-file (SPEC 079).
	applyAddedSources := func(text string) {
		presenter.MergeGUIToModel()
		if err := wizardbusiness.AppendURLsToSources(presenter, strings.TrimSpace(text)); err != nil {
			debuglog.ErrorLog("source_tab: Add error: %v", err)
			return
		}
		m := presenter.Model()
		m.PreviewNeedsParse = true
		m.TemplatePreviewNeedsUpdate = true
		presenter.UpdateParserConfig(m.ParserConfigJSON)
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		presenter.MarkAsChanged()
		// Clear the URL field after adding so the user can enter the next URL
		guiState.SourceURLsProgrammatic = true
		guiState.SourceURLEntry.SetText("")
		guiState.SourceURLsProgrammatic = false
	}

	addURLButton := widget.NewButton(locale.T("wizard.source.button_add"), func() {
		applyAddedSources(guiState.SourceURLEntry.Text)
	})

	// SPEC 079: WG/AWG configs are often shared as files (.conf with
	// [Interface]/[Peer], or .vpn with a vpn:// link). Pick a file → its text
	// goes through the same import path as the Add field.
	addFromFileButton := widget.NewButton(locale.T("wizard.source.button_add_from_file"), func() {
		fileDialog := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, guiState.Window)
				return
			}
			if rc == nil {
				return // cancelled
			}
			defer rc.Close()
			text, rerr := wizardbusiness.ReadSourceFileText(rc)
			if rerr != nil {
				dialog.ShowError(rerr, guiState.Window)
				return
			}
			if text == "" {
				return
			}
			applyAddedSources(text)
		}, guiState.Window)
		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".conf", ".vpn", ".txt"}))
		fileDialog.Show()
	})

	// «Free community servers» — picker (LxBox-style): клик подставляет URL
	// из bin/get_free.json в поле SourceURLEntry, ничего не сохраняет в
	// state.json и не мутирует модель. Юзер сам нажимает Add.
	getFreeVPNButton := widget.NewButton(locale.T("wizard.source.button_get_free"), func() {
		wizarddialogs.ShowGetFreeVPNDialog(presenter)
	})

	// Limit width and height of URL input field (3 lines)
	// Wrap MultiLineEntry in Scroll container to show scrollbars; right gutter for scrollbar strip
	urlURIGutter := components.NewScrollGutter()
	urlEntryScrollInner := container.NewBorder(nil, nil, nil, urlURIGutter, guiState.SourceURLEntry)
	urlEntryScroll := container.NewScroll(urlEntryScrollInner)
	urlEntryScroll.Direction = container.ScrollBoth
	// Create dummy Rectangle to set size (height 3 lines, width limited)
	urlEntrySizeRect := canvas.NewRectangle(color.Transparent)
	urlEntrySizeRect.SetMinSize(fyne.NewSize(0, 60)) // Width 900px, height ~3 lines (approx 20px per line)
	// Wrap in Max container with Rectangle to fix size
	// Scroll container will be limited by this size and show scrollbars when content doesn't fit
	urlEntryWithSize := container.NewStack(
		urlEntrySizeRect,
		urlEntryScroll,
	)

	// Header row: label + add-from-file + community-picker buttons on the right.
	urlHeader := container.NewHBox(
		urlLabel,
		layout.NewSpacer(),
		addFromFileButton,
		getFreeVPNButton,
	)

	// URL field with Add button on the right, vertically centered with the field.
	// Use Border so the entry takes all remaining width and Add stays compact on the right.
	urlEntryRow := container.NewBorder(
		nil, nil,
		nil,
		container.NewCenter(addURLButton),
		urlEntryWithSize,
	)

	urlContainer := container.NewVBox(
		urlHeader,   // Header with Get free VPN
		urlEntryRow, // Input field + Add button on the right
		hintRow,     // Hint + docs button
	)

	// Section 2: Sources list (based on ParserConfig.ParserConfig.Proxies)
	sourcesLabel := widget.NewLabel(locale.T("wizard.source.label_sources"))
	sourcesLabel.Importance = widget.MediumImportance

	sourcesBox := container.NewVBox()

	refreshSourcesList := func() {
		sourcesBox.Objects = sourcesBox.Objects[:0]
		m := presenter.Model()
		if len(m.Sources) == 0 {
			emptyGutter := components.NewScrollGutter()
			sourcesBox.Add(container.NewHBox(widget.NewLabel(locale.T("wizard.source.no_sources")), layout.NewSpacer(), emptyGutter))
			sourcesBox.Refresh()
			return
		}

		for i := range m.Sources {
			// IIFE so each row's closures capture the correct index (avoids loop variable capture bug)
			func(sourceIndex int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				srcPtr := &m.Sources[sourceIndex]
				src := *srcPtr

				isSubscription := src.Type == corestate.SourceTypeSubscription
				meta := src.Meta
				sourceID := src.ID

				// Label / tooltip data из v5 Source (canonical).
				// SPEC 052 phase 8: для подписки приоритет — profile_title (читабельно
				// для человека), URL уходит в tooltip + Edit-окно. Для server —
				// label или URI fragment.
				label := ""
				if isSubscription {
					if meta != nil && strings.TrimSpace(meta.ProfileTitle) != "" {
						label = strings.TrimSpace(meta.ProfileTitle)
					} else {
						label = src.URL
					}
				} else {
					label = src.Label
					if label == "" {
						label = src.URI
					}
					if label == "" {
						// Fallback: первый node tag из preview (если есть).
						if m.PreviewNodesBySource != nil &&
							sourceIndex < len(m.PreviewNodesBySource) &&
							len(m.PreviewNodesBySource[sourceIndex]) > 0 {
							first := m.PreviewNodesBySource[sourceIndex][0]
							if first.Tag != "" {
								label = first.Tag
							} else if first.Label != "" {
								label = first.Label
							}
						}
					}
					if label == "" {
						label = locale.Tf("wizard.source.source_n", sourceIndex+1)
					}
				}
				label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
				shortLabel := label

				fullURL := src.URL
				var tagPrefix, tagPostfix, tagMask string
				if src.Tag != nil {
					tagPrefix = src.Tag.Prefix
					tagPostfix = src.Tag.Postfix
					tagMask = src.Tag.Mask
				}

				localTags := make([]string, 0, len(src.Outbounds))
				for _, ob := range src.Outbounds {
					if ob.Tag != "" {
						localTags = append(localTags, ob.Tag)
					}
				}

				tooltipLines := []string{
					fmt.Sprintf("URL: %s", fullURL),
					fmt.Sprintf("tag_prefix: %s", tagPrefix),
					fmt.Sprintf("tag_postfix: %s", tagPostfix),
					fmt.Sprintf("tag_mask: %s", tagMask),
					fmt.Sprintf("local outbounds: %d", len(localTags)),
				}
				if len(localTags) > 0 {
					tooltipLines = append(tooltipLines, "tags: "+strings.Join(localTags, ", "))
				}
				if metaTip := metaTooltip(meta); metaTip != "" {
					tooltipLines = append(tooltipLines, "—— meta ——", metaTip)
				}
				tooltipText := strings.Join(tooltipLines, "\n")

				copyText := fullURL
				if copyText == "" {
					copyText = src.URI
				}
				sourceLabel := ttwidget.NewLabel(shortLabel)
				sourceLabel.Wrapping = fyne.TextWrapOff
				sourceLabel.Truncation = fyne.TextTruncateEllipsis

				// SPEC 052 phase 8: type indicator убран как визуальный шум — тип
				// и так читается из URL (https://) vs URI (vless://, wireguard://);
				// в Edit-окне есть Overview tab с явным "Type: Subscription/Server".
				var leftBlock fyne.CanvasObject
				var prefixLabel *ttwidget.Label
				if pfx := strings.TrimSpace(tagPrefix); pfx != "" {
					pfxShow := wizardutils.TruncateStringEllipsis(pfx, 24, "...")
					prefixLabel = ttwidget.NewLabel(pfxShow)
					prefixLabel.Importance = widget.MediumImportance
					if pfxShow != pfx {
						prefixLabel.SetToolTip(pfx)
					}
					leftBlock = prefixLabel
				}
				_ = tagPostfix
				var rowCenter fyne.CanvasObject = container.NewBorder(nil, nil, leftBlock, nil, sourceLabel)

				// Enable/disable toggle — persists to Source.Enabled.
				// Dim the label importance so disabled rows are visibly inactive.
				enableCheck := widget.NewCheck("", nil)
				enableCheck.SetChecked(srcPtr.Enabled)
				if !srcPtr.Enabled {
					sourceLabel.Importance = widget.LowImportance
					if prefixLabel != nil {
						prefixLabel.Importance = widget.LowImportance
					}
				}
				enableCheck.OnChanged = func(enabled bool) {
					m := presenter.Model()
					if sourceIndex >= len(m.Sources) {
						return
					}
					m.Sources[sourceIndex].Enabled = enabled
					// Shared mutation chain (marks dirty, re-derives, refreshes
					// outbound options + list). The MarkAsChanged rationale and
					// the previously-missing RefreshOutboundOptions live there.
					applySourceMutation(presenter, guiState)
				}

				copyBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.ContentCopyIcon(), func() {
					if copyText == "" {
						return
					}
					if guiState.Window != nil {
						fyne.CurrentApp().Clipboard().SetContent(copyText)
						dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window, locale.T("wizard.source.dialog_copied_title"), locale.T("wizard.source.dialog_copied_message"))
					}
				}, rowGetter)
				copyBtn.Importance = widget.LowImportance
				sourceLabel.SetToolTip(tooltipText)
				fynewidget.SetToolTipSafe(copyBtn, tooltipText)

				editBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DocumentCreateIcon(), func() {
					presenter.MergeGUIToModel()
					m := presenter.Model()
					if m == nil || sourceIndex >= len(m.Sources) {
						return
					}
					showSourceEditWindow(presenter, guiState, guiState.Window, sourceIndex, shortLabel)
				}, rowGetter)
				editBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(editBtn, locale.T("wizard.source.button_edit"))

				delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(), func() {
					// Confirm before removing — deletion drops the source (and its
					// nodes) from the config; matches the Rules-tab delete UX.
					dialog.ShowConfirm(
						locale.T("wizard.dialog_confirmation"),
						locale.Tf("wizard.source.dialog_delete_confirm", shortLabel),
						func(ok bool) {
							if !ok {
								return
							}
							m := presenter.Model()
							if sourceIndex >= len(m.Sources) {
								return
							}
							m.Sources = append(m.Sources[:sourceIndex], m.Sources[sourceIndex+1:]...)
							applySourceMutation(presenter, guiState)
						},
						guiState.Window,
					)
				}, rowGetter)
				delBtn.Importance = widget.LowImportance
				fynewidget.SetToolTipSafe(delBtn, locale.T("wizard.source.button_del"))

				// SPEC 052 phase 8: статус из subtitle (⚠ при err); badge на главной
				// строке убран как избыточный. Refresh-icon только для подписок.
				var refreshBtn *fynewidget.HoverForwardButton
				if isSubscription && sourceID != "" {
					refreshBtn = fynewidget.NewHoverForwardButtonWithIcon("", theme.ViewRefreshIcon(), func() {
						refreshOneSourceFromUI(presenter, guiState, sourceID)
					}, rowGetter)
					refreshBtn.Importance = widget.LowImportance
					fynewidget.SetToolTipSafe(refreshBtn, locale.T("wizard.source.tooltip_refresh_one"))
				}

				// SPEC 061 Phase 3: ⚠ / 📢 icon-button — persistent affordance to
				// open the source-error dialog when meta carries an error or a
				// provider announce. Placed to the LEFT of copy/edit so the
				// row's edit/delete cluster keeps a stable visual position.
				var noticeBtn *fynewidget.HoverForwardButton
				if isSubscription && meta != nil && (meta.LastStatus == "err" || (meta.ProviderAnnounce != nil && !meta.ProviderAnnounce.IsEmpty())) {
					icon := theme.WarningIcon()
					tooltipKey := "wizard.source.tooltip_error_details"
					if meta.LastStatus != "err" {
						// Success-with-notice path: provider sent content + announce.
						// Use info-styled icon. We don't have an info-theme icon
						// in our minimal set, fall back to QuestionIcon (📢-ish).
						icon = theme.QuestionIcon()
						tooltipKey = "wizard.source.tooltip_provider_notice"
					}
					srcLabel := shortLabel
					metaCopy := meta // capture by value for closure (meta is *SubscriptionMeta, stable)
					noticeBtn = fynewidget.NewHoverForwardButtonWithIcon("", icon, func() {
						wizarddialogs.ShowSourceErrorDialog(guiState.Window, srcLabel, metaCopy)
					}, rowGetter)
					noticeBtn.Importance = widget.LowImportance
					fynewidget.SetToolTipSafe(noticeBtn, locale.T(tooltipKey))
				}

				// Reorder buttons (↑/↓) — move this source within the list.
				// Order is plain slice order in model.Sources and persists to
				// state.connections.sources on Save (handles both subscriptions
				// and direct servers, since both live in the same Sources slice).
				moveUpBtn := fynewidget.NewHoverForwardButton("↑", func() {
					moveSourceUp(presenter, guiState, sourceIndex)
				}, rowGetter)
				moveUpBtn.Importance = widget.LowImportance
				if sourceIndex <= 0 {
					moveUpBtn.Disable()
					fynewidget.SetToolTipSafe(moveUpBtn, locale.T("wizard.source.tooltip_move_up_off"))
				} else {
					fynewidget.SetToolTipSafe(moveUpBtn, locale.T("wizard.source.tooltip_move_up"))
				}

				moveDownBtn := fynewidget.NewHoverForwardButton("↓", func() {
					moveSourceDown(presenter, guiState, sourceIndex)
				}, rowGetter)
				moveDownBtn.Importance = widget.LowImportance
				if sourceIndex >= len(m.Sources)-1 {
					moveDownBtn.Disable()
					fynewidget.SetToolTipSafe(moveDownBtn, locale.T("wizard.source.tooltip_move_down_off"))
				} else {
					fynewidget.SetToolTipSafe(moveDownBtn, locale.T("wizard.source.tooltip_move_down"))
				}

				rowGutter := components.NewScrollGutter()
				rightControlsItems := []fyne.CanvasObject{}
				if noticeBtn != nil {
					rightControlsItems = append(rightControlsItems, noticeBtn)
				}
				// SPEC 069 feature: provider support / web-page link — small inline
				// icon in the info panel (TG plane / link), tooltip = URL, click opens.
				// No extra row height; nil for sources without a support URL.
				if supportBtn := supportLinkButton(meta, rowGetter); supportBtn != nil {
					rightControlsItems = append(rightControlsItems, supportBtn)
				}
				rightControlsItems = append(rightControlsItems, copyBtn, editBtn)
				if refreshBtn != nil {
					rightControlsItems = append(rightControlsItems, refreshBtn)
				}
				rightControlsItems = append(rightControlsItems, delBtn)
				// Pack the action icons tightly (tightHBox with a negative gap),
				// then keep the scroll gutter separated at the right edge with the
				// normal HBox padding so it still reserves the scrollbar strip.
				rightControls := container.NewHBox(
					container.New(tightHBox{spacing: rowIconGap}, rightControlsItems...),
					rowGutter,
				)
				// Guideline (Rules tab): reorder ↑/↓ go to the LEFT of the enable
				// checkbox in a leading cluster, action buttons stay on the right.
				// Arrows are packed tight; the checkbox keeps its own leading wrap.
				arrowsCluster := container.New(tightHBox{spacing: rowIconGap}, moveUpBtn, moveDownBtn)
				leftLead := container.NewHBox(arrowsCluster, fynewidget.CheckLeadingWrap(enableCheck))
				titleRow := container.NewBorder(nil, nil, leftLead, rightControls, rowCenter)

				// Subtitle row: meta inline (nodes / interval / fetched / quota / expires).
				// tightVBox — custom layout без theme.Padding между title/subtitle
				// (стандартный VBox / Border даёт ~12px воздуха, slишком много).
				var rowInner fyne.CanvasObject = titleRow
				if isSubscription {
					lines := []fyne.CanvasObject{titleRow}
					if subtitle := formatSourceSubtitle(meta, srcPtr.Update, m.Defaults.Reload); subtitle != "" {
						subtitleText := canvas.NewText(subtitle, theme.PlaceHolderColor())
						subtitleText.TextSize = theme.CaptionTextSize()
						// Indent the subtitle by the exact width of the leading
						// cluster (↑ ↓ + checkbox) so it starts right under the
						// title — the title in titleRow also sits after leftLead.
						// Hardcoding broke once the reorder arrows were added.
						leftPad := canvas.NewRectangle(color.Transparent)
						leftPad.SetMinSize(fyne.NewSize(leftLead.MinSize().Width, 0))
						lines = append(lines, container.NewBorder(nil, nil, leftPad, nil, subtitleText))
					}
					if len(lines) > 1 {
						rowInner = container.New(tightVBox{}, lines...)
					}
				}

				row = fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
				row.WireTooltipLabelHover(sourceLabel)
				if prefixLabel != nil {
					row.WireTooltipLabelHover(prefixLabel)
				}
				sourcesBox.Add(row)
			}(i)
		}

		sourcesBox.Refresh()
	}

	// Ensure sources list is initialized from current model state
	refreshSourcesList()
	guiState.RefreshSourcesList = refreshSourcesList

	sourcesScroll := container.NewVScroll(sourcesBox)
	sourcesScroll.SetMinSize(fyne.NewSize(0, 80))

	previewAllBtn := widget.NewButton(locale.T("wizard.source.button_preview_all"), func() {
		showSourcePreviewAllWindow(presenter)
	})
	sourcesHeader := container.NewHBox(
		sourcesLabel,
		layout.NewSpacer(),
		previewAllBtn,
	)

	topBlock := container.NewVBox(
		widget.NewSeparator(),
		urlContainer,
		widget.NewSeparator(),
		sourcesHeader,
	)

	tabScrollGutter := components.NewScrollGutter()

	// Sources list fills remaining tab height (preview all servers moved to a separate window).
	body := container.NewBorder(
		topBlock,
		nil,
		nil,
		tabScrollGutter,
		sourcesScroll,
	)

	return body
}

// moveSourceUp swaps the source at idx with the one above it, then re-derives
// the parser config and refreshes the list. Order persists on the next Save.
func moveSourceUp(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, idx int) {
	m := presenter.Model()
	if m == nil || idx <= 0 || idx >= len(m.Sources) {
		return
	}
	m.Sources[idx-1], m.Sources[idx] = m.Sources[idx], m.Sources[idx-1]
	applySourceMutation(presenter, guiState)
}

// moveSourceDown swaps the source at idx with the one below it.
func moveSourceDown(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, idx int) {
	m := presenter.Model()
	if m == nil || idx < 0 || idx >= len(m.Sources)-1 {
		return
	}
	m.Sources[idx], m.Sources[idx+1] = m.Sources[idx+1], m.Sources[idx]
	applySourceMutation(presenter, guiState)
}

// applySourceMutation is the single refresh chain every Sources-list mutation
// runs after editing model.Sources (reorder ↑/↓, enable toggle, delete):
// mark dirty → re-derive ParserConfig → invalidate preview cache →
// UpdateParserConfig → refresh outbound options → rebuild the list.
//
// MarkAsChanged is called explicitly (and first) on purpose: UpdateParserConfig
// below suppresses the ParserConfig text widget's OnChanged → MarkAsChanged
// (see UpdateParserConfig), so without this the mutation would be silently
// lost on close and the Save button wouldn't light up.
//
// Keeping all source mutations on this one helper is deliberate — the chain
// drifted before (the enable toggle used to skip RefreshOutboundOptions, so a
// disabled source's outbounds lingered in the rule selectors). Add new source
// mutations here, not as a fresh inline copy.
func applySourceMutation(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) {
	m := presenter.Model()
	if m == nil {
		return
	}
	presenter.MarkAsChanged()
	m.RefreshDerivedParserConfig()
	m.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(m)
	presenter.UpdateParserConfig(m.ParserConfigJSON)
	presenter.RefreshOutboundOptions()
	if guiState != nil && guiState.RefreshSourcesList != nil {
		guiState.RefreshSourcesList()
	}
}

// showSourcePreviewAllWindow opens a window with the combined server list from all sources (uses View window slot).
func showSourcePreviewAllWindow(presenter *wizardpresentation.WizardPresenter) {
	if presenter == nil {
		return
	}
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

	win := app.NewWindow(locale.T("wizard.source.preview_all_title"))
	presenter.SetViewWindow(win)
	win.SetOnClosed(func() {
		presenter.ClearViewWindow()
		presenter.UpdateChildOverlay()
	})

	var previewNodes []*config.ParsedNode
	previewStatusLabel := widget.NewLabel(locale.T("wizard.source.preview_click_refresh"))
	previewStatusLabel.Wrapping = fyne.TextWrapOff
	previewStatusScroll := container.NewHScroll(previewStatusLabel)
	previewList := widget.NewList(
		func() int { return len(previewNodes) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id int, o fyne.CanvasObject) {
			if id < len(previewNodes) {
				o.(*widget.Label).SetText(nodeDisplayLine(previewNodes[id]))
			}
		},
	)

	refreshPreview := func() {
		m := presenter.Model()
		if m.ParserConfig == nil || len(m.ParserConfig.ParserConfig.Proxies) == 0 {
			previewNodes = nil
			previewList.Refresh()
			previewStatusLabel.SetText(locale.T("wizard.source.preview_no_sources"))
			return
		}
		previewStatusLabel.SetText(locale.T("wizard.source.preview_loading"))

		go func() {
			mm := m
			errorCount, err := wizardbusiness.RebuildPreviewCache(mm)
			presenter.UpdateUI(func() {
				if err != nil {
					previewNodes = nil
					previewList.Refresh()
					previewStatusLabel.SetText(locale.Tf("wizard.source.preview_error", err.Error()))
					return
				}
				previewNodes = mm.PreviewNodes
				previewList.Refresh()
				sourcesCount := 0
				if mm.ParserConfig != nil {
					sourcesCount = len(mm.ParserConfig.ParserConfig.Proxies)
				}
				status := locale.Tf("wizard.source.preview_servers", len(previewNodes), sourcesCount)
				if errorCount > 0 {
					status += locale.Tf("wizard.source.preview_errors", errorCount)
				}
				previewStatusLabel.SetText(status)
			})
		}()
	}

	refreshBtn := widget.NewButton(locale.T("wizard.source.button_refresh"), refreshPreview)
	closeBtn := widget.NewButton(locale.T("wizard.source.view_close"), func() { win.Close() })
	topRow := container.NewBorder(nil, nil, nil, refreshBtn, previewStatusScroll)
	listStrip := components.NewScrollGutter()
	previewScroll := container.NewScroll(previewList)
	previewScroll.Direction = container.ScrollVerticalOnly
	listRow := container.NewBorder(nil, nil, nil, listStrip, previewScroll)
	bottomRow := container.NewHBox(layout.NewSpacer(), closeBtn)

	minList := canvas.NewRectangle(color.Transparent)
	minList.SetMinSize(fyne.NewSize(0, 320))
	listFill := container.NewStack(minList, listRow)

	content := container.NewBorder(
		container.NewVBox(topRow, widget.NewSeparator()),
		bottomRow,
		nil, nil,
		listFill,
	)

	win.SetContent(content)
	win.Resize(fyne.NewSize(560, 520))
	win.CenterOnScreen()
	refreshPreview()
	win.Show()
	presenter.UpdateChildOverlay()
}

// nodeDisplayLine returns a short one-line description for a parsed node (for list display).
// textnorm.NormalizeProxyDisplay repairs UTF-8 and maps ❯/»/› to ASCII " > " for Fyne on macOS.
func nodeDisplayLine(node *config.ParsedNode) string {
	if node == nil {
		return ""
	}
	var s string
	switch {
	case node.Tag != "":
		s = node.Tag
	case node.Label != "":
		s = node.Label
	case node.Server != "":
		return fmt.Sprintf("%s:%d", node.Server, node.Port)
	default:
		s = node.Scheme
	}
	return textnorm.NormalizeProxyDisplay(s)
}

// CreateOutboundsAndParserConfigTab creates the Outbounds and ParserConfig tab UI.
// For now it reuses the existing ParserConfig editor and Config Outbounds button;
// later it will be extended to embed the outbounds configurator list directly.
func CreateOutboundsAndParserConfigTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()

	// ParserConfig multi-line editor
	guiState.ParserConfigEntry = widget.NewMultiLineEntry()
	guiState.ParserConfigEntry.SetPlaceHolder(locale.T("wizard.outbounds.placeholder"))
	guiState.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	guiState.ParserConfigEntry.OnChanged = func(string) {
		if guiState.ParserConfigUpdating {
			return
		}
		model := presenter.Model()
		model.PreviewNeedsParse = true
		// Sync GUI to model to update ParserConfigJSON before refreshing outbound options
		presenter.MergeGUIToModel()
		presenter.MarkAsChanged()
		presenter.ScheduleRefreshOutboundOptionsDebounced()
		// Preview status will be updated when switching to Preview tab
	}

	// Limit width and height of ParserConfig field
	parserConfigScroll := container.NewScroll(guiState.ParserConfigEntry)
	parserConfigScroll.Direction = container.ScrollBoth
	parserHeightRect := canvas.NewRectangle(color.Transparent)
	parserHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 lines
	parserConfigWithHeight := container.NewStack(
		parserHeightRect,
		parserConfigScroll,
	)

	// Documentation button
	docButton := widget.NewButton(locale.T("wizard.outbounds.button_docs"), func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/ParserConfig.md"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_open_docs"), err), guiState.Window)
		}
	})

	parserLabel := widget.NewLabel(locale.T("wizard.outbounds.label"))
	parserLabel.Importance = widget.MediumImportance

	// Ensure model.ParserConfig is set so configurator can edit it (configurator reads via editPresenter.Model()).
	m := presenter.Model()
	if m.ParserConfig == nil {
		pc := &config.ParserConfig{}
		raw := strings.TrimSpace(m.ParserConfigJSON)
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), pc); err != nil {
				debuglog.DebugLog("source_tab: initial parse of ParserConfigJSON failed: %v", err)
			}
		}
		m.ParserConfig = pc
	}

	onConfiguratorApply := func() {
		m := presenter.Model()
		// SPEC 052 phase 8: outbounds-configurator мутирует m.ParserConfig
		// (legacy view); переносим назад в canonical Sources/GlobalOutbounds,
		// потом re-derive ParserConfig (round-trip).
		if m.ParserConfig != nil {
			m.GlobalOutbounds = append([]configtypes.OutboundConfig(nil), m.ParserConfig.ParserConfig.Outbounds...)
			// Per-source outbounds: ParserConfig.Proxies[i] построен из
			// m.Sources[i] через AsParserConfig (1:1 порядок), поэтому
			// обратный sync безопасен по тому же индексу. Без этого правки
			// в Outbounds tab при Scope ≠ "For All" терялись на Save —
			// state.json пишет m.Sources[i].Outbounds, а они не обновлялись.
			proxies := m.ParserConfig.ParserConfig.Proxies
			for i := range m.Sources {
				if i >= len(proxies) {
					break
				}
				m.Sources[i].Outbounds = append([]configtypes.OutboundConfig(nil), proxies[i].Outbounds...)
			}
		}
		m.RefreshDerivedParserConfig()
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidatePreviewCache(m)
		presenter.UpdateParserConfig(m.ParserConfigJSON)
		presenter.RefreshOutboundOptions()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		// UpdateParserConfig sets ParserConfigUpdating during SetText so OnChanged does not MarkAsChanged;
		// outbounds list actions (Edit/Add/Delete, ↑/↓) must mark dirty explicitly.
		presenter.MarkAsChanged()
	}

	configuratorContent, refreshOutboundsConfigurator := outbounds_configurator.NewConfiguratorContent(guiState.Window, presenter, onConfiguratorApply)
	guiState.RefreshOutboundsConfiguratorList = refreshOutboundsConfigurator

	// No Parse button on this tab per SPEC: update is automatic via configurator callback and tab switch (Rules/Preview).
	headerRow := container.NewHBox(
		parserLabel,
		layout.NewSpacer(),
		docButton,
	)

	parserContainer := container.NewVBox(
		headerRow,
		parserConfigWithHeight,
		widget.NewSeparator(),
		configuratorContent,
	)

	content := container.NewVBox(
		widget.NewSeparator(),
		parserContainer,
		widget.NewSeparator(),
	)

	scrollContainer := container.NewScroll(content)
	// Adaptive min-height: a fixed 620px forced the whole wizard window taller
	// than small laptop screens (Big Sur 1280×800), pushing nav buttons under
	// the Dock. Scale with window height; the fallback (used before the window
	// is measured at first layout) is kept small enough to fit a 600px window.
	scrollContainer.SetMinSize(adaptiveScrollSize(guiState, 0.62, 440))

	return scrollContainer
}

// refreshOneSourceFromUI — SPEC 052 phase 7: per-source Refresh button click handler.
//
// Использует RefreshSourceInPlace (in-memory path) вместо
// RefreshSingleSubscription (state.json path), чтобы Refresh работал на cold
// start — когда state.json ещё нет, потому что пользователь не нажимал Save.
// Каноничный Source хранится в model; refresh fetch'ит, пишет .raw на диск,
// и обновляет Meta в нашей snapshot-копии. На UI thread snapshot ассайнится
// обратно в model. State.json не трогается — он запишется при следующем Save
// пользователем (теперь уже со свежей Meta).
//
// Race protection: snapshot источника берётся на UI thread (включая deep-copy
// Meta), goroutine мутирует свою копию, на UI thread snapshot переезжает в
// model. Параллельный Add нового source не разваливается — slice может
// reallocate'нуться, мы по ID находим место заново.
func refreshOneSourceFromUI(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceID string,
) {
	// UI thread: snapshot Source из model. Deep-copy Meta — иначе goroutine
	// мутирует общий объект (refreshOneSubscriptionSource на failure-path
	// дёргает src.Meta.X = ... через pointer).
	m := presenter.Model()
	var snapshot corestate.Source
	found := false
	for i := range m.Sources {
		if m.Sources[i].ID == sourceID {
			snapshot = m.Sources[i]
			if snapshot.Meta != nil {
				metaCopy := *snapshot.Meta
				snapshot.Meta = &metaCopy
			}
			found = true
			break
		}
	}
	if !found {
		return
	}

	configService := presenter.ConfigServiceAdapter()
	go func() {
		_, err := configService.RefreshSourceInPlace(&snapshot)
		presenter.UpdateUI(func() {
			if err != nil {
				if guiState != nil && guiState.Window != nil && fyne.CurrentApp() != nil {
					dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
						locale.T("wizard.source.button_refresh_one"),
						locale.Tf("wizard.source.refresh_failed", err.Error()))
				}
				return
			}
			// Snapshot обратно в model. Slice мог reallocate'нуться (Add /
			// Del между snapshot-таймом и сейчас), поэтому ищем по ID заново.
			m := presenter.Model()
			for i := range m.Sources {
				if m.Sources[i].ID == sourceID {
					m.Sources[i] = snapshot
					break
				}
			}
			if guiState != nil && guiState.RefreshSourcesList != nil {
				guiState.RefreshSourcesList()
			}
			if guiState != nil && guiState.Window != nil && fyne.CurrentApp() != nil {
				dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window,
					locale.T("wizard.source.button_refresh_one"),
					locale.T("wizard.source.refresh_succeeded"))
			}
			// Mark dirty: model.Sources[].Meta изменился, при следующем Save
			// эти изменения уедут в state.json. Это пользовательский edit-ish
			// — даём ему dirty marker, чтобы Save-кнопка светилась.
			presenter.MarkAsChanged()
		})
	}()
}
