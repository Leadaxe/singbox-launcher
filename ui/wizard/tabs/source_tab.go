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
	"strings"

	"image/color"

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
	guiState.CheckURLButton = widget.NewButton("Check", func() {
		if guiState.CheckURLInProgress {
			return
		}
		go func() {
			if err := wizardbusiness.CheckURL(presenter.Model(), presenter); err != nil {
				log.Printf("source_tab: CheckURL failed: %v", err)
			}
		}()
	})

	// Create progress bar for Check button
	guiState.CheckURLProgress = widget.NewProgressBar()
	guiState.CheckURLProgress.Hide()
	guiState.CheckURLProgress.SetValue(0)

	// Set fixed size via placeholder
	checkButtonWidth := float32(180)
	checkButtonHeight := guiState.CheckURLButton.MinSize().Height + 4 // Slightly taller

	// Create placeholder to preserve size (always show to preserve size)
	checkURLPlaceholder := canvas.NewRectangle(color.Transparent)
	checkURLPlaceholder.SetMinSize(fyne.NewSize(checkButtonWidth, checkButtonHeight))
	checkURLPlaceholder.Show() // Always show to preserve size

	// Create container with stack (placeholder, button, progress)
	checkURLStack := container.NewStack(
		checkURLPlaceholder,
		guiState.CheckURLButton,
		guiState.CheckURLProgress,
	)

	// Add padding from right edge (10 units in Fyne)
	// Use empty Rectangle to create padding
	paddingRect := canvas.NewRectangle(color.Transparent)
	paddingRect.SetMinSize(fyne.NewSize(10, 0)) // 10px padding on right
	guiState.CheckURLContainer = container.NewHBox(
		checkURLStack, // Button/progress
		paddingRect,   // Right padding
	)

	urlLabel := widget.NewLabel("Subscription URL or Direct Links:")
	urlLabel.Importance = widget.MediumImportance

	guiState.SourceURLEntry = widget.NewMultiLineEntry()
	guiState.SourceURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...\nhysteria2://...\nssh://...")
	guiState.SourceURLEntry.Wrapping = fyne.TextWrapOff
	guiState.SourceURLEntry.OnChanged = func(value string) {
		model := presenter.Model()
		model.PreviewNeedsParse = true
		if err := wizardbusiness.ApplyURLToParserConfig(model, presenter, strings.TrimSpace(value)); err != nil {
			log.Printf("source_tab: error applying URL to ParserConfig: %v", err)
		}
	}

	// Hint under input field with Check button on right
	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://, ssh://).\nFor multiple links, use a new line for each.")
	hintLabel.Wrapping = fyne.TextWrapWord

	hintRow := container.NewBorder(
		nil,                        // top
		nil,                        // bottom
		nil,                        // left
		guiState.CheckURLContainer, // right - button/progress
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

	urlContainer := container.NewVBox(
		urlLabel,                // Header
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
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/README.md#configuring-configjson"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open documentation: %w", err), guiState.Window)
		}
	})

	parserLabel := widget.NewLabel("ParserConfig:")
	parserLabel.Importance = widget.MediumImportance

	// Parse button (positioned to left of ParserConfig)
	guiState.ParseButton = widget.NewButton("Parse", func() {
		model := presenter.Model()
		if model.AutoParseInProgress {
			return
		}
		model.AutoParseInProgress = true
		model.PreviewNeedsParse = true
		configService := presenter.ConfigServiceAdapter()
		go func() {
			if err := wizardbusiness.ParseAndPreview(model, presenter, configService); err != nil {
				log.Printf("source_tab: ParseAndPreview failed: %v", err)
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
	// Make field read-only, but text remains black (not disabled)
	guiState.OutboundsPreview.OnChanged = func(text string) {
		// Restore saved text when trying to edit
		if text != previewText {
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
