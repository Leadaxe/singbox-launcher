// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл source_tab.go содержит функции, создающие UI табов визарда:
//   - Вкладка Sources: ввод URL, проверка, список источников; объединённый превью серверов — в отдельном окне
//   - Вкладка Outbounds and ParserConfig: редактор ParserConfig JSON и вход в конфигуратор outbounds
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
//
// Используется в:
//   - wizard.go - при создании окна визарда, вызывается CreateSourceTab(presenter)
//
// Взаимодействует с:
//   - presenter - все действия пользователя (нажатия кнопок, ввод текста) обрабатываются через методы presenter
//   - business - AppendURLsToParserConfig по кнопке Add; список источников из model.ParserConfig.Proxies
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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizarddialogs "singbox-launcher/ui/wizard/dialogs"
	"singbox-launcher/ui/wizard/outbounds_configurator"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardutils "singbox-launcher/ui/wizard/utils"

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
	addURLButton := widget.NewButton(locale.T("wizard.source.button_add"), func() {
		presenter.MergeGUIToModel()
		trimmed := strings.TrimSpace(guiState.SourceURLEntry.Text)
		if err := wizardbusiness.AppendURLsToParserConfig(presenter, trimmed); err != nil {
			debuglog.ErrorLog("source_tab: Add URL error: %v", err)
			return
		}
		m := presenter.Model()
		m.PreviewNeedsParse = true
		m.TemplatePreviewNeedsUpdate = true // so Preview tab refreshes when opened
		presenter.UpdateParserConfig(m.ParserConfigJSON)
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		presenter.MarkAsChanged()
		// Clear the URL field after adding so the user can enter the next URL
		guiState.SourceURLsProgrammatic = true
		guiState.SourceURLEntry.SetText("")
		guiState.SourceURLsProgrammatic = false
	})

	getFreeVPNButton := widget.NewButton(locale.T("wizard.source.button_get_free"), func() {
		wizarddialogs.ShowGetFreeVPNDialog(presenter)
	})

	// Limit width and height of URL input field (3 lines)
	// Wrap MultiLineEntry in Scroll container to show scrollbars; right gutter for scrollbar strip
	urlURIGutter := canvas.NewRectangle(color.Transparent)
	urlURIGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	urlEntryScrollInner := container.NewBorder(nil, nil, nil, urlURIGutter, guiState.SourceURLEntry)
	urlEntryScroll := container.NewScroll(urlEntryScrollInner)
	urlEntryScroll.Direction = container.ScrollBoth
	// Create dummy Rectangle to set size (height 3 lines, width limited)
	urlEntrySizeRect := canvas.NewRectangle(color.Transparent)
	urlEntrySizeRect.SetMinSize(fyne.NewSize(0, 60)) // Width 900px, height ~3 lines (approx 20px per line)
	// Wrap in Max container with Rectangle to fix size
	// Scroll container will be limited by this size and show scrollbars when content doesn't fit
	urlEntryWithSize := container.NewMax(
		urlEntrySizeRect,
		urlEntryScroll,
	)

	// Header row: label, spacer, Get free VPN (Add is to the right of the field below)
	urlHeader := container.NewHBox(
		urlLabel,
		layout.NewSpacer(),
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
		if m.ParserConfig == nil || len(m.ParserConfig.ParserConfig.Proxies) == 0 {
			emptyGutter := canvas.NewRectangle(color.Transparent)
			emptyGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
			sourcesBox.Add(container.NewHBox(widget.NewLabel(locale.T("wizard.source.no_sources")), layout.NewSpacer(), emptyGutter))
			sourcesBox.Refresh()
			return
		}

		for i := range m.ParserConfig.ParserConfig.Proxies {
			// IIFE so each row's closures capture the correct index (avoids loop variable capture bug)
			func(sourceIndex int) {
				var row *fynewidget.HoverRow
				rowGetter := func() *fynewidget.HoverRow { return row }

				proxyPtr := &m.ParserConfig.ParserConfig.Proxies[sourceIndex]
				proxy := *proxyPtr

				label := proxy.Source
				if label == "" {
					// Prefer first node's tag/label from preview when block has only Connections (no URL)
					if len(proxy.Connections) > 0 && m.PreviewNodesBySource != nil &&
						sourceIndex < len(m.PreviewNodesBySource) && len(m.PreviewNodesBySource[sourceIndex]) > 0 {
						first := m.PreviewNodesBySource[sourceIndex][0]
						if first.Tag != "" {
							label = first.Tag
						} else if first.Label != "" {
							label = first.Label
						}
					}
					if label == "" {
						// Connection-only block (no subscription URL): show as "Connections" or "Connections N"
						if len(proxy.Connections) > 0 {
							label = locale.Tf("wizard.source.connections_n", sourceIndex+1)
						} else {
							label = locale.Tf("wizard.source.source_n", sourceIndex+1)
						}
					}
				}
				label = wizardutils.TruncateStringEllipsis(label, wizardutils.MaxLabelRunes, "...")
				shortLabel := label

				fullURL := proxy.Source
				tagPrefix := proxy.TagPrefix
				tagPostfix := proxy.TagPostfix
				tagMask := proxy.TagMask

				localTags := make([]string, 0, len(proxy.Outbounds))
				for _, ob := range proxy.Outbounds {
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
				tooltipText := strings.Join(tooltipLines, "\n")

				copyText := fullURL
				if copyText == "" && len(proxy.Connections) > 0 {
					copyText = strings.Join(proxy.Connections, "\n")
				}
				sourceLabel := ttwidget.NewLabel(shortLabel)
				sourceLabel.Wrapping = fyne.TextWrapOff
				sourceLabel.Truncation = fyne.TextTruncateEllipsis

				// Show tag_prefix in the row (often short like "BL:") so list stays scannable without opening Edit.
				var rowCenter fyne.CanvasObject = sourceLabel
				var prefixLabel *ttwidget.Label
				if pfx := strings.TrimSpace(tagPrefix); pfx != "" {
					pfxShow := wizardutils.TruncateStringEllipsis(pfx, 24, "...")
					prefixLabel = ttwidget.NewLabel(pfxShow)
					prefixLabel.Importance = widget.MediumImportance
					if pfxShow != pfx {
						prefixLabel.SetToolTip(pfx)
					}
					rowCenter = container.NewBorder(nil, nil, prefixLabel, nil, sourceLabel)
				}

				// Enable/disable toggle — persists to ProxySource.Disabled,
				// parser skips disabled sources. Dim the label importance so
				// disabled rows are visibly inactive in the list.
				enableCheck := widget.NewCheck("", nil)
				enableCheck.SetChecked(!proxyPtr.Disabled)
				if proxyPtr.Disabled {
					sourceLabel.Importance = widget.LowImportance
					if prefixLabel != nil {
						prefixLabel.Importance = widget.LowImportance
					}
				}
				enableCheck.OnChanged = func(enabled bool) {
					m := presenter.Model()
					if m.ParserConfig == nil || sourceIndex >= len(m.ParserConfig.ParserConfig.Proxies) {
						return
					}
					m.ParserConfig.ParserConfig.Proxies[sourceIndex].Disabled = !enabled
					serialized, err := wizardbusiness.SerializeParserConfig(m.ParserConfig)
					if err != nil {
						debuglog.ErrorLog("source_tab: SerializeParserConfig after toggle: %v", err)
						return
					}
					m.ParserConfigJSON = serialized
					m.PreviewNeedsParse = true
					wizardbusiness.InvalidatePreviewCache(m)
					presenter.UpdateParserConfig(serialized)
					if guiState.RefreshSourcesList != nil {
						guiState.RefreshSourcesList()
					}
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
				if tb, ok := interface{}(copyBtn).(interface{ SetToolTip(string) }); ok {
					tb.SetToolTip(tooltipText)
				}

				editBtn := fynewidget.NewHoverForwardButton(locale.T("wizard.source.button_edit"), func() {
					presenter.MergeGUIToModel()
					m := presenter.Model()
					if m == nil {
						return
					}
					if err := wizardbusiness.EnsureWizardModelParserConfig(m); err != nil {
						debuglog.ErrorLog("source_tab: EnsureWizardModelParserConfig before Edit: %v", err)
						if guiState.Window != nil {
							dialog.ShowError(err, guiState.Window)
						}
						return
					}
					if m.ParserConfig == nil || sourceIndex >= len(m.ParserConfig.ParserConfig.Proxies) {
						return
					}
					showSourceEditWindow(presenter, guiState, guiState.Window, sourceIndex, shortLabel)
				}, rowGetter)

				delBtn := fynewidget.NewHoverForwardButton(locale.T("wizard.source.button_del"), func() {
					m := presenter.Model()
					if m.ParserConfig == nil || sourceIndex >= len(m.ParserConfig.ParserConfig.Proxies) {
						return
					}
					proxies := &m.ParserConfig.ParserConfig.Proxies
					*proxies = append((*proxies)[:sourceIndex], (*proxies)[sourceIndex+1:]...)
					serialized, err := wizardbusiness.SerializeParserConfig(m.ParserConfig)
					if err != nil {
						debuglog.ErrorLog("source_tab: SerializeParserConfig after Del source: %v", err)
						return
					}
					m.ParserConfigJSON = serialized
					m.PreviewNeedsParse = true
					wizardbusiness.InvalidatePreviewCache(m)
					presenter.UpdateParserConfig(serialized)
					presenter.RefreshOutboundOptions()
					if guiState.RefreshSourcesList != nil {
						guiState.RefreshSourcesList()
					}
				}, rowGetter)

				rowGutter := canvas.NewRectangle(color.Transparent)
				rowGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
				rightControls := container.NewHBox(
					copyBtn,
					editBtn,
					delBtn,
					rowGutter,
				)
				rowInner := container.NewBorder(nil, nil, enableCheck, rightControls, rowCenter)
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

	tabScrollGutter := canvas.NewRectangle(color.Transparent)
	tabScrollGutter.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))

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
	listStrip := canvas.NewRectangle(color.Transparent)
	listStrip.SetMinSize(fyne.NewSize(scrollbarGutterWidth, 0))
	previewScroll := container.NewScroll(previewList)
	previewScroll.Direction = container.ScrollVerticalOnly
	listRow := container.NewBorder(nil, nil, nil, listStrip, previewScroll)
	bottomRow := container.NewHBox(layout.NewSpacer(), closeBtn)

	minList := canvas.NewRectangle(color.Transparent)
	minList.SetMinSize(fyne.NewSize(0, 320))
	listFill := container.NewMax(minList, listRow)

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

// fetchAndParseSource fetches a subscription URL or parses a direct link and returns parsed nodes.
func fetchAndParseSource(sourceURL string, skip []map[string]string) ([]*config.ParsedNode, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	sourceURL = strings.ToValidUTF8(sourceURL, "")
	if sourceURL == "" {
		return nil, fmt.Errorf("empty source URL")
	}
	var nodes []*config.ParsedNode
	tagCounts := make(map[string]int)
	if subscription.IsSubscriptionURL(sourceURL) {
		content, err := subscription.FetchSubscription(sourceURL)
		if err != nil {
			return nil, err
		}
		contentStr := string(content)
		contentStr = strings.ReplaceAll(contentStr, "\r\n", "\n")
		contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
		contentStr = strings.TrimSpace(contentStr)
		if subscription.IsXrayJSONArrayBody(contentStr) {
			arrayNodes, err := subscription.ParseNodesFromXrayJSONArray(contentStr, skip)
			if err != nil {
				return nil, err
			}
			for _, node := range arrayNodes {
				if len(nodes) >= configtypes.MaxNodesPerSubscription {
					debuglog.WarnLog("source_tab: fetchAndParseSource truncated at %d nodes (same limit as subscription loader)",
						configtypes.MaxNodesPerSubscription)
					break
				}
				if node.Jump != nil {
					node.Jump.Tag = subscription.MakeTagUnique(node.Jump.Tag, tagCounts, "ConfigWizard")
				}
				node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
				nodes = append(nodes, node)
			}
			return nodes, nil
		}
		for _, line := range strings.Split(contentStr, "\n") {
			line = subscription.NormalizeSubscriptionTextLine(line)
			if line == "" {
				continue
			}
			if len(nodes) >= configtypes.MaxNodesPerSubscription {
				debuglog.WarnLog("source_tab: fetchAndParseSource truncated at %d nodes (same limit as subscription loader)",
					configtypes.MaxNodesPerSubscription)
				break
			}
			node, err := subscription.ParseNode(line, skip)
			if err != nil {
				continue
			}
			if node != nil {
				node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
				nodes = append(nodes, node)
			}
		}
		return nodes, nil
	}
	if subscription.IsDirectLink(sourceURL) {
		node, err := subscription.ParseNode(sourceURL, skip)
		if err != nil {
			return nil, err
		}
		if node != nil {
			node.Tag = subscription.MakeTagUnique(node.Tag, tagCounts, "ConfigWizard")
			nodes = append(nodes, node)
		}
		return nodes, nil
	}
	return nil, fmt.Errorf("not a subscription URL or direct link")
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
	parserConfigWithHeight := container.NewMax(
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
		serialized, err := wizardbusiness.SerializeParserConfig(m.ParserConfig)
		if err != nil {
			debuglog.ErrorLog("source_tab: SerializeParserConfig after configurator change: %v", err)
			dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_serialize"), err), guiState.Window)
			return
		}
		m.ParserConfigJSON = serialized
		m.PreviewNeedsParse = true
		wizardbusiness.InvalidatePreviewCache(m)
		presenter.UpdateParserConfig(serialized)
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
	scrollContainer.SetMinSize(fyne.NewSize(0, 620))

	return scrollContainer
}

// CreateSourceTab is kept for backward compatibility and currently returns the Sources tab content.
func CreateSourceTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	return CreateSourcesTab(presenter)
}
