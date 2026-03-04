// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл source_tab.go содержит функции, создающие UI табов визарда:
//   - Вкладка Sources: ввод URL, проверка, список источников и Preview сгенерированных нод/селекторов
//   - Вкладка Outbounds and ParserConfig: редактор ParserConfig JSON и вход в конфигуратор outbounds
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
//
// Используется в:
//   - wizard.go - при создании окна визарда, вызывается CreateSourceTab(presenter)
//
// Взаимодействует с:
//   - presenter - все действия пользователя (нажатия кнопок, ввод текста) обрабатываются через методы presenter
//   - business - вызывает ParseAndPreview через presenter (ApplyURLToParserConfig и т.д.)
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
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizarddialogs "singbox-launcher/ui/wizard/dialogs"
	"singbox-launcher/ui/wizard/outbounds_configurator"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// CreateSourcesTab creates the Sources tab UI (URLs, URL status and preview).
func CreateSourcesTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	model := presenter.Model()

	// Section 1: Subscription URL or Direct Links
	urlLabel := widget.NewLabel("Subscription URL or Direct Links:")
	urlLabel.Importance = widget.MediumImportance

	guiState.SourceURLEntry = widget.NewMultiLineEntry()
	guiState.SourceURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...\nhysteria2://...\nssh://...")
	guiState.SourceURLEntry.Wrapping = fyne.TextWrapOff
	// No automatic application: URLs are applied only when the user clicks Add.
	guiState.SourceURLEntry.OnChanged = func(value string) {
		presenter.Model().PreviewNeedsParse = true
	}

	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://). For multiple links, use a new line for each.")
	hintLabel.Wrapping = fyne.TextWrapWord

	addURLButton := widget.NewButton("Add", func() {
		presenter.SyncGUIToModel()
		trimmed := strings.TrimSpace(guiState.SourceURLEntry.Text)
		if err := wizardbusiness.AppendURLsToParserConfig(model, presenter, trimmed); err != nil {
			debuglog.ErrorLog("source_tab: Add URL error: %v", err)
		}
		model.PreviewNeedsParse = true
		presenter.UpdateParserConfig(model.ParserConfigJSON)
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
		// Clear the URL field after adding so the user can enter the next URL
		guiState.SourceURLEntry.SetText("")
	})

	getFreeVPNButton := widget.NewButton("Get free VPN!", func() {
		wizarddialogs.ShowGetFreeVPNDialog(presenter)
	})

	// Limit width and height of URL input field (3 lines)
	// Wrap MultiLineEntry in Scroll container to show scrollbars
	urlEntryScroll := container.NewScroll(guiState.SourceURLEntry)
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
		hintLabel,   // Hint
	)

	// Section 2: Sources list (based on ParserConfig.ParserConfig.Proxies)
	sourcesLabel := widget.NewLabel("Sources")
	sourcesLabel.Importance = widget.MediumImportance

	sourcesBox := container.NewVBox()

	refreshSourcesList := func() {
		sourcesBox.Objects = sourcesBox.Objects[:0]

		if model.ParserConfig == nil || len(model.ParserConfig.ParserConfig.Proxies) == 0 {
			sourcesBox.Add(widget.NewLabel("No sources defined in ParserConfig."))
			sourcesBox.Refresh()
			return
		}

		for i := range model.ParserConfig.ParserConfig.Proxies {
			proxyPtr := &model.ParserConfig.ParserConfig.Proxies[i]
			proxy := *proxyPtr
			sourceIndex := i

			label := proxy.Source
			if label == "" {
				label = fmt.Sprintf("Source %d", sourceIndex+1)
			}
			if len(label) > 40 {
				label = label[:37] + "..."
			}
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
			sourceButton := widget.NewButton(shortLabel, func() {
				if copyText == "" {
					return
				}
				if guiState.Window != nil && guiState.Window.Clipboard() != nil {
					guiState.Window.Clipboard().SetContent(copyText)
					dialogs.ShowAutoHideInfo(fyne.CurrentApp(), guiState.Window, "Copied", "Source copied to clipboard.")
				}
			})
			sourceButton.Importance = widget.LowImportance
			if tb, ok := interface{}(sourceButton).(interface{ SetToolTip(string) }); ok {
				tb.SetToolTip(tooltipText)
			}

			prefixEntry := widget.NewEntry()
			prefixEntry.SetText(proxy.TagPrefix)
			prefixEntry.SetPlaceHolder("prefix")
			prefixEntry.OnChanged = func(s string) {
				if model.ParserConfig == nil || sourceIndex >= len(model.ParserConfig.ParserConfig.Proxies) {
					return
				}
				model.ParserConfig.ParserConfig.Proxies[sourceIndex].TagPrefix = strings.TrimSpace(s)
				serialized, err := wizardbusiness.SerializeParserConfig(model.ParserConfig)
				if err != nil {
					debuglog.ErrorLog("source_tab: SerializeParserConfig after prefix change: %v", err)
					return
				}
				model.ParserConfigJSON = serialized
				model.PreviewNeedsParse = true
				presenter.UpdateParserConfig(serialized)
				presenter.RefreshOutboundOptions()
			}

			viewBtn := widget.NewButton("View", func() {
				if model.ParserConfig == nil || sourceIndex >= len(model.ParserConfig.ParserConfig.Proxies) {
					return
				}
				prox := &model.ParserConfig.ParserConfig.Proxies[sourceIndex]
				showSourceServersWindow(presenter, guiState.Window, shortLabel, prox.Source, prox.Skip)
			})

			delBtn := widget.NewButton("Del", func() {
				if model.ParserConfig == nil || sourceIndex >= len(model.ParserConfig.ParserConfig.Proxies) {
					return
				}
				proxies := &model.ParserConfig.ParserConfig.Proxies
				*proxies = append((*proxies)[:sourceIndex], (*proxies)[sourceIndex+1:]...)
				serialized, err := wizardbusiness.SerializeParserConfig(model.ParserConfig)
				if err != nil {
					debuglog.ErrorLog("source_tab: SerializeParserConfig after Del source: %v", err)
					return
				}
				model.ParserConfigJSON = serialized
				model.PreviewNeedsParse = true
				presenter.UpdateParserConfig(serialized)
				presenter.RefreshOutboundOptions()
				if guiState.RefreshSourcesList != nil {
					guiState.RefreshSourcesList()
				}
			})

			row := container.NewHBox(
				sourceButton,
				layout.NewSpacer(),
				prefixEntry,
				viewBtn,
				delBtn,
			)
			sourcesBox.Add(row)
		}

		sourcesBox.Refresh()
	}

	// Ensure sources list is initialized from current model state
	refreshSourcesList()
	guiState.RefreshSourcesList = refreshSourcesList

	sourcesScroll := container.NewVScroll(sourcesBox)
	sourcesScroll.SetMinSize(fyne.NewSize(0, 140))

	// Section 3: Preview — servers from all sources (same as View, but combined at bottom of tab)
	var previewNodes []*config.ParsedNode
	previewStatusLabel := widget.NewLabel("Click Refresh to load servers from all sources.")
	previewList := widget.NewList(
		func() int { return len(previewNodes) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id int, o fyne.CanvasObject) {
			if id < len(previewNodes) {
				o.(*widget.Label).SetText(nodeDisplayLine(previewNodes[id]))
			}
		},
	)
	previewScroll := container.NewScroll(previewList)
	previewScroll.SetMinSize(fyne.NewSize(0, 180))
	// 10px strip to the right of the list (scrollbar area)
	previewScrollStrip := canvas.NewRectangle(color.Transparent)
	previewScrollStrip.SetMinSize(fyne.NewSize(10, 0))

	refreshPreview := func() {
		if model.ParserConfig == nil || len(model.ParserConfig.ParserConfig.Proxies) == 0 {
			previewNodes = nil
			previewList.Refresh()
			previewStatusLabel.SetText("No sources. Add URLs and click Refresh.")
			return
		}
		previewStatusLabel.SetText("Loading...")
		proxies := model.ParserConfig.ParserConfig.Proxies
		sources := make([]struct {
			URL  string
			Skip []map[string]string
		}, len(proxies))
		for i := range proxies {
			sources[i].URL = proxies[i].Source
			sources[i].Skip = proxies[i].Skip
		}
		go func() {
			var all []*config.ParsedNode
			var errorCount int
			for _, s := range sources {
				nodes, err := fetchAndParseSource(s.URL, s.Skip)
				if err != nil {
					debuglog.DebugLog("source_tab: preview fetch %q: %v", s.URL, err)
					errorCount++
					continue
				}
				all = append(all, nodes...)
			}
			fyne.Do(func() {
				previewNodes = all
				previewList.Refresh()
				status := fmt.Sprintf("%d server(s) from %d source(s)", len(previewNodes), len(sources))
				if errorCount > 0 {
					status += fmt.Sprintf("  ⚠️ %d error(s)", errorCount)
				}
				previewStatusLabel.SetText(status)
			})
		}()
	}

	previewRefreshBtn := widget.NewButton("Refresh", refreshPreview)
	previewStatusRow := container.NewHBox(previewStatusLabel, layout.NewSpacer(), previewRefreshBtn)
	// List full width, 20px strip on the right
	previewListRow := container.NewBorder(nil, nil, nil, nil, previewScroll)
	previewBox := container.NewVBox(
		previewStatusRow,
		previewListRow,
	)

	// Combine all sections
	content := container.NewVBox(
		widget.NewSeparator(),
		urlContainer,
		widget.NewSeparator(),
		sourcesLabel,
		sourcesScroll,
		widget.NewSeparator(),
		previewBox,
		widget.NewSeparator(),
	)

	// Add scroll for long content
	scrollContainer := container.NewScroll(content)
	scrollContainer.SetMinSize(fyne.NewSize(0, 620))

	return scrollContainer
}

// nodeDisplayLine returns a short one-line description for a parsed node (for list display).
func nodeDisplayLine(node *config.ParsedNode) string {
	if node == nil {
		return ""
	}
	if node.Tag != "" {
		return node.Tag
	}
	if node.Label != "" {
		return node.Label
	}
	if node.Server != "" {
		return fmt.Sprintf("%s:%d", node.Server, node.Port)
	}
	return node.Scheme
}

// fetchAndParseSource fetches a subscription URL or parses a direct link and returns parsed nodes.
func fetchAndParseSource(sourceURL string, skip []map[string]string) ([]*config.ParsedNode, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return nil, fmt.Errorf("empty source URL")
	}
	var nodes []*config.ParsedNode
	if subscription.IsSubscriptionURL(sourceURL) {
		content, err := subscription.FetchSubscription(sourceURL)
		if err != nil {
			return nil, err
		}
		contentStr := string(content)
		contentStr = strings.ReplaceAll(contentStr, "\r\n", "\n")
		contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
		for _, line := range strings.Split(contentStr, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			node, err := subscription.ParseNode(line, skip)
			if err != nil {
				continue
			}
			if node != nil {
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
			nodes = append(nodes, node)
		}
		return nodes, nil
	}
	return nil, fmt.Errorf("not a subscription URL or direct link")
}

// showSourceServersWindow opens a separate window, fetches/parses the subscription and shows the server list (like Servers tab).
// Only one View window is allowed; if already open, focus is moved to it.
func showSourceServersWindow(presenter *wizardpresentation.WizardPresenter, parent fyne.Window, sourceLabel string, sourceURL string, skip []map[string]string) {
	if presenter != nil {
		if w := presenter.OpenViewWindow(); w != nil {
			w.RequestFocus()
			return
		}
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	title := "Servers — " + sourceLabel
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	win := app.NewWindow(title)
	if presenter != nil {
		presenter.SetViewWindow(win)
		win.SetOnClosed(func() {
			presenter.ClearViewWindow()
			presenter.UpdateChildOverlay()
		})
	}
	statusLabel := widget.NewLabel("Loading...")
	closeBtn := widget.NewButton("Close", func() { win.Close() })
	topContent := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), closeBtn), nil, nil, statusLabel)
	win.SetContent(topContent)
	win.Resize(fyne.NewSize(420, 380))
	win.CenterOnScreen()
	win.Show()
	if presenter != nil {
		presenter.UpdateChildOverlay()
	}

	go func() {
		nodes, err := fetchAndParseSource(sourceURL, skip)
		fyne.Do(func() {
			if err != nil {
				statusLabel.SetText("Error: " + err.Error())
				return
			}
			if len(nodes) == 0 {
				statusLabel.SetText("No servers found.")
				return
			}
			statusLabel.SetText(fmt.Sprintf("%d server(s)", len(nodes)))
			list := widget.NewList(
				func() int { return len(nodes) },
				func() fyne.CanvasObject {
					return widget.NewLabel("")
				},
				func(id int, o fyne.CanvasObject) {
					o.(*widget.Label).SetText(nodeDisplayLine(nodes[id]))
				},
			)
			scroll := container.NewScroll(list)
			scroll.SetMinSize(fyne.NewSize(0, 280))
			win.SetContent(container.NewBorder(statusLabel, container.NewHBox(layout.NewSpacer(), closeBtn), nil, nil, scroll))
		})
	}()
}

// CreateOutboundsAndParserConfigTab creates the Outbounds and ParserConfig tab UI.
// For now it reuses the existing ParserConfig editor and Config Outbounds button;
// later it will be extended to embed the outbounds configurator list directly.
func CreateOutboundsAndParserConfigTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	model := presenter.Model()

	// ParserConfig multi-line editor
	guiState.ParserConfigEntry = widget.NewMultiLineEntry()
	guiState.ParserConfigEntry.SetPlaceHolder("Enter ParserConfig JSON here...")
	guiState.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	guiState.ParserConfigEntry.OnChanged = func(string) {
		if guiState.ParserConfigUpdating {
			return
		}
		model := presenter.Model()
		model.PreviewNeedsParse = true
		// Sync GUI to model to update ParserConfigJSON before refreshing outbound options
		presenter.SyncGUIToModel()
		presenter.RefreshOutboundOptions()
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
	docButton := widget.NewButton("📖 Documentation", func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/docs/ParserConfig.md"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open documentation: %w", err), guiState.Window)
		}
	})

	parserLabel := widget.NewLabel("ParserConfig:")
	parserLabel.Importance = widget.MediumImportance

	// Embedded outbounds configurator: use model.ParserConfig so edits apply in place.
	pc := model.ParserConfig
	if pc == nil {
		pc = &config.ParserConfig{}
		raw := strings.TrimSpace(model.ParserConfigJSON)
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), pc); err != nil {
				debuglog.DebugLog("source_tab: initial parse of ParserConfigJSON failed: %v", err)
			}
		}
		model.ParserConfig = pc
	}

	onConfiguratorApply := func() {
		serialized, err := wizardbusiness.SerializeParserConfig(pc)
		if err != nil {
			debuglog.ErrorLog("source_tab: SerializeParserConfig after configurator change: %v", err)
			dialog.ShowError(fmt.Errorf("Failed to serialize ParserConfig: %w", err), guiState.Window)
			return
		}
		model.ParserConfigJSON = serialized
		model.ParserConfig = pc
		model.PreviewNeedsParse = true
		presenter.UpdateParserConfig(serialized)
		presenter.RefreshOutboundOptions()
		if guiState.RefreshSourcesList != nil {
			guiState.RefreshSourcesList()
		}
	}

	configuratorContent := outbounds_configurator.NewConfiguratorContent(guiState.Window, pc, onConfiguratorApply, presenter)

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
