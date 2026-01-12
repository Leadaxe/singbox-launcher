// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл source_tab.go содержит функцию CreateSourceTab, которая создает UI первого таба визарда:
//   - Ввод URL подписки или прямых ссылок (SourceURLEntry)
//   - Проверка URL (CheckURLButton, URLStatusLabel, CheckURLProgress)
//   - Редактирование ParserConfig (ParserConfigEntry)
//   - Preview сгенерированных outbounds (OutboundsPreview)
//   - Кнопка парсинга (ParseButton)
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
//
// Используется в:
//   - wizard.go - при создании окна визарда, вызывается CreateSourceTab(presenter)
//
// Взаимодействует с:
//   - presenter - все действия пользователя (нажатия кнопок, ввод текста) обрабатываются через методы presenter
//   - business - вызывает CheckURL, ParseAndPreview через presenter
package tabs

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// CreateSourceTab creates the Sources & ParserConfig tab UI.
func CreateSourceTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()

	// Section 1: Subscription URL or Direct Links
	// We perform automatic URL checking on input change (debounced) instead of
	// requiring the user to click a "Check" button.
	// Add a padding placeholder container on the right to keep layout similar.
	paddingRect := canvas.NewRectangle(color.Transparent)
	paddingRect.SetMinSize(fyne.NewSize(10, 0)) // 10px padding on right
	guiState.CheckURLContainer = container.NewHBox(
		paddingRect,
	)

	urlLabel := widget.NewLabel("Subscription URL or Direct Links:")
	urlLabel.Importance = widget.MediumImportance

	guiState.SourceURLEntry = widget.NewMultiLineEntry()
	guiState.SourceURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...\nhysteria2://...\nssh://...")
	guiState.SourceURLEntry.Wrapping = fyne.TextWrapOff
	guiState.SourceURLEntry.OnChanged = func(value string) {
		model := presenter.Model()
		model.PreviewNeedsParse = true
		trimmed := strings.TrimSpace(value)
		if err := wizardbusiness.ApplyURLToParserConfig(model, presenter, trimmed); err != nil {
			log.Printf("source_tab: error applying URL to ParserConfig: %v", err)
		}

		// Debounce CheckURL: cancel previous timer and set a new one (2s after last change)
		if guiState.CheckURLTimer != nil {
			guiState.CheckURLTimer.Stop()
			guiState.CheckURLTimer = nil
		}

		// Define the actual check logic as a reusable closure so we can reschedule
		var doCheck func(string)
		doCheck = func(v string) {
			// This runs in goroutine from timer - coordinate with UI thread for state
			fyne.Do(func() {
				// If a check is currently in progress, reschedule after delay
				if guiState.CheckURLInProgress {
					// reschedule
					guiState.CheckURLTimer = time.AfterFunc(2*time.Second, func() { doCheck(v) })
					return
				}
				// Mark in-progress and sync
				guiState.CheckURLInProgress = true
				presenter.SyncGUIToModel()
				// Run the check in background
				go func() {
					if err := wizardbusiness.CheckURL(presenter.Model(), presenter); err != nil {
						log.Printf("source_tab: CheckURL failed: %v", err)
					}
					// Clear in-progress flag
					fyne.Do(func() { guiState.CheckURLInProgress = false })
				}()
			})
		}

		// Schedule the check after debounce interval
		guiState.CheckURLTimer = time.AfterFunc(2*time.Second, func() { doCheck(trimmed) })
	}

	// Hint under input field with Check button on right
	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://). For multiple links, use a new line for each.")
	hintLabel.Wrapping = fyne.TextWrapWord

	var freeVPNDialog dialog.Dialog
	var freeVPNDialogOpen bool
	getFreeVPNButton := widget.NewButton("Get free VPN!", func() {
		if freeVPNDialogOpen {
			return
		}
		thanks := widget.NewLabel("Thank @igareck for providing VPN lists:")
		thanks.Wrapping = fyne.TextWrapWord
		linkURL, _ := url.Parse("https://github.com/igareck/vpn-configs-for-russia?tab=readme-ov-file#-%D1%87%D0%B5%D1%80%D0%BD%D1%8B%D0%B9-%D1%81%D0%BF%D0%B8%D1%81%D0%BE%D0%BA-")
		link := widget.NewHyperlink("https://github.com/igareck/vpn-configs-for-russia", linkURL)
		addButton := widget.NewButton("Add links", func() {
			urls := []string{
				"https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/BLACK_VLESS_RUS.txt",
				"https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/Vless-Reality-White-Lists-Rus-Cable.txt",
				"https://raw.githubusercontent.com/igareck/vpn-configs-for-russia/main/Vless-Reality-White-Lists-Rus-Mobile.txt",
			}
			current := strings.TrimSpace(guiState.SourceURLEntry.Text)
			linksText := strings.Join(urls, "\n")
			if current != "" {
				guiState.SourceURLEntry.SetText(current + "\n" + linksText)
			} else {
				guiState.SourceURLEntry.SetText(linksText)
			}
			if freeVPNDialog != nil {
				freeVPNDialog.Hide()
			}
		})
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(0, addButton.MinSize().Height))
		content := container.NewVBox(
			thanks,
			link,
			spacer,
			addButton,
		)
		freeVPNDialog = dialog.NewCustom("Get free VPN", "Close", content, guiState.Window)
		freeVPNDialog.SetOnClosed(func() { freeVPNDialogOpen = false })
		freeVPNDialogOpen = true
		freeVPNDialog.Show()
	})

	hintRow := container.NewBorder(
		nil,                        // top
		nil,                        // bottom
		nil,                        // left
		guiState.CheckURLContainer, // right - actions
		hintLabel,                  // center - hint takes all available space
	)

	guiState.URLStatusLabel = widget.NewLabel("")
	guiState.URLStatusLabel.Wrapping = fyne.TextWrapWord

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

	// Header row with action on the right
	urlHeader := container.NewHBox(
		urlLabel,
		layout.NewSpacer(),
		getFreeVPNButton,
	)

	urlContainer := container.NewVBox(
		urlHeader,               // Header with action
		urlEntryWithSize,        // Input field with size limit (3 lines)
		hintRow,                 // Hint with button on right
		guiState.URLStatusLabel, // Status
	)

	// Section 2: ParserConfig
	guiState.ParserConfigEntry = widget.NewMultiLineEntry()
	guiState.ParserConfigEntry.SetPlaceHolder("Enter ParserConfig JSON here...")
	guiState.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	guiState.ParserConfigEntry.OnChanged = func(string) {
		if guiState.ParserConfigUpdating {
			return
		}
		model := presenter.Model()
		model.PreviewNeedsParse = true
		presenter.RefreshOutboundOptions()

		// Preview status will be updated when switching to Preview tab
	}

	// Limit width and height of ParserConfig field
	parserConfigScroll := container.NewScroll(guiState.ParserConfigEntry)
	parserConfigScroll.Direction = container.ScrollBoth
	// Create dummy Rectangle to set height via container.NewMax
	parserHeightRect := canvas.NewRectangle(color.Transparent)
	parserHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 lines
	// Wrap in Max container with Rectangle to fix height
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

	// Parse button (positioned to left of ParserConfig)
	guiState.ParseButton = widget.NewButton("Parse", func() {
		// Sync GUI to model before parsing
		presenter.SyncGUIToModel()
		model := presenter.Model()
		// Quick validation: ensure ParserConfig is not empty to provide immediate feedback.
		if strings.TrimSpace(model.ParserConfigJSON) == "" {
			// Show an error dialog and update preview with a clear message
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("ParserConfig is empty. Please enter ParserConfig JSON or load a template."), guiState.Window)
				if guiState.OutboundsPreview != nil {
					presenter.UpdateOutboundsPreview("Error: ParserConfig is empty")
				}
			})
			return
		}
		log.Printf("source_tab: Parse clicked, parser length=%d", len(strings.TrimSpace(model.ParserConfigJSON)))
		if model.AutoParseInProgress {
			return
		}
		model.AutoParseInProgress = true
		model.PreviewNeedsParse = true
		configService := presenter.ConfigServiceAdapter()
		go func() {
			if err := wizardbusiness.ParseAndPreview(model, presenter, configService); err != nil {
				log.Printf("source_tab: ParseAndPreview failed: %v", err)
				// Show error to user in case of parse failure
				fyne.Do(func() {
					if guiState.OutboundsPreview != nil {
						presenter.UpdateOutboundsPreview("Error: Failed to parse ParserConfig - see logs for details")
					}
				})
			}
		}()
	})
	guiState.ParseButton.Importance = widget.MediumImportance

	headerRow := container.NewHBox(
		parserLabel,
		widget.NewLabel("  "), // small spacing between text and button
		guiState.ParseButton,
		layout.NewSpacer(),
		docButton,
	)

	parserContainer := container.NewVBox(
		headerRow,
		parserConfigWithHeight,
	)

	// Section 3: Preview Generated Outbounds
	previewLabel := widget.NewLabel("Preview")
	previewLabel.Importance = widget.MediumImportance

	// Use Entry without Disable for black text, but make it read-only via OnChanged
	guiState.OutboundsPreview = widget.NewMultiLineEntry()
	guiState.OutboundsPreview.SetPlaceHolder("Generated outbounds will appear here after clicking Parse...")
	guiState.OutboundsPreview.Wrapping = fyne.TextWrapOff
	previewText := "Generated outbounds will appear here after clicking Parse..."
	guiState.OutboundsPreview.SetText(previewText)
	guiState.OutboundsPreviewLastText = previewText
	// Make field effectively read-only: ignore programmatic updates, restore last preview on user edits
	guiState.OutboundsPreview.OnChanged = func(text string) {
		if guiState.OutboundsPreviewUpdating {
			// Ignore programmatic updates
			return
		}
		// Restore last known preview text
		if guiState.OutboundsPreviewLastText != "" {
			guiState.OutboundsPreview.SetText(guiState.OutboundsPreviewLastText)
		} else {
			guiState.OutboundsPreview.SetText(previewText)
		}
	}

	// Limit width and height of Preview field
	previewScroll := container.NewScroll(guiState.OutboundsPreview)
	previewScroll.Direction = container.ScrollBoth
	// Create dummy Rectangle to set height via container.NewMax
	previewHeightRect := canvas.NewRectangle(color.Transparent)
	previewHeightRect.SetMinSize(fyne.NewSize(0, 90)) // ~8-9 lines (reduced by ~30px)
	// Wrap in Max container with Rectangle to fix height
	previewWithHeight := container.NewMax(
		previewHeightRect,
		previewScroll,
	)

	previewContainer := container.NewVBox(
		previewLabel,
		previewWithHeight,
	)

	// Combine all sections
	content := container.NewVBox(
		widget.NewSeparator(),
		urlContainer,
		widget.NewSeparator(),
		parserContainer,
		widget.NewSeparator(),
		previewContainer,
		widget.NewSeparator(),
	)

	// Add scroll for long content
	scrollContainer := container.NewScroll(content)
	scrollContainer.SetMinSize(fyne.NewSize(0, 620))

	return scrollContainer
}
