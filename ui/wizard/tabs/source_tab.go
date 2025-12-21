package tabs

import (
	"fmt"
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
	wizardstate "singbox-launcher/ui/wizard/state"
)

// CreateVLESSSourceTab creates the VLESS Source tab UI.
func CreateVLESSSourceTab(state *wizardstate.WizardState) fyne.CanvasObject {
	// Section 1: VLESS Subscription URL or Direct Links
	state.CheckURLButton = widget.NewButton("Check", func() {
		if state.CheckURLInProgress {
			return
		}
		go wizardbusiness.CheckURL(state)
	})

	// Create progress bar for Check button
	state.CheckURLProgress = widget.NewProgressBar()
	state.CheckURLProgress.Hide()
	state.CheckURLProgress.SetValue(0)

	// Set fixed size via placeholder
	checkButtonWidth := float32(180)
	checkButtonHeight := state.CheckURLButton.MinSize().Height + 4 // Slightly taller

	// Create placeholder to preserve size (always show to preserve size)
	state.CheckURLPlaceholder = canvas.NewRectangle(color.Transparent)
	state.CheckURLPlaceholder.SetMinSize(fyne.NewSize(checkButtonWidth, checkButtonHeight))
	state.CheckURLPlaceholder.Show() // Always show to preserve size

	// Create container with stack (placeholder, button, progress)
	checkURLStack := container.NewStack(
		state.CheckURLPlaceholder,
		state.CheckURLButton,
		state.CheckURLProgress,
	)

	// Add padding from right edge (10 units in Fyne)
	// Use empty Rectangle to create padding
	paddingRect := canvas.NewRectangle(color.Transparent)
	paddingRect.SetMinSize(fyne.NewSize(10, 0)) // 10px padding on right
	state.CheckURLContainer = container.NewHBox(
		checkURLStack, // Button/progress
		paddingRect,   // Right padding
	)

	urlLabel := widget.NewLabel("VLESS Subscription URL or Direct Links:")
	urlLabel.Importance = widget.MediumImportance

	state.VLESSURLEntry = widget.NewMultiLineEntry()
	state.VLESSURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...\nhysteria2://...")
	state.VLESSURLEntry.Wrapping = fyne.TextWrapOff
	state.VLESSURLEntry.OnChanged = func(value string) {
		state.PreviewNeedsParse = true
		wizardbusiness.ApplyURLToParserConfig(state, strings.TrimSpace(value))
	}

	// Hint under input field with Check button on right
	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://).\nFor multiple links, use a new line for each.")
	hintLabel.Wrapping = fyne.TextWrapWord

	hintRow := container.NewBorder(
		nil,                     // top
		nil,                     // bottom
		nil,                     // left
		state.CheckURLContainer, // right - button/progress
		hintLabel,               // center - hint takes all available space
	)

	state.URLStatusLabel = widget.NewLabel("")
	state.URLStatusLabel.Wrapping = fyne.TextWrapWord

	// Limit width and height of URL input field (3 lines)
	// Wrap MultiLineEntry in Scroll container to show scrollbars
	urlEntryScroll := container.NewScroll(state.VLESSURLEntry)
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
		urlLabel,             // Header
		urlEntryWithSize,     // Input field with size limit (3 lines)
		hintRow,              // Hint with button on right
		state.URLStatusLabel, // Status
	)

	// Section 2: ParserConfig
	state.ParserConfigEntry = widget.NewMultiLineEntry()
	state.ParserConfigEntry.SetPlaceHolder("Enter ParserConfig JSON here...")
	state.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	state.ParserConfigEntry.OnChanged = func(string) {
		if state.ParserConfigUpdating {
			return
		}
		state.PreviewNeedsParse = true
		state.RefreshOutboundOptions()

		// Preview status will be updated when switching to Preview tab
	}

	// Limit width and height of ParserConfig field
	parserConfigScroll := container.NewScroll(state.ParserConfigEntry)
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
			dialog.ShowError(fmt.Errorf("failed to open documentation: %w", err), state.Window)
		}
	})

	parserLabel := widget.NewLabel("ParserConfig:")
	parserLabel.Importance = widget.MediumImportance

	// Parse button (positioned to left of ParserConfig)
	state.ParseButton = widget.NewButton("Parse", func() {
		if state.AutoParseInProgress {
			return
		}
		state.AutoParseInProgress = true
		state.PreviewNeedsParse = true
		go wizardbusiness.ParseAndPreview(state)
	})
	state.ParseButton.Importance = widget.MediumImportance

	headerRow := container.NewHBox(
		parserLabel,
		widget.NewLabel("  "), // small spacing between text and button
		state.ParseButton,
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
	state.OutboundsPreview = widget.NewMultiLineEntry()
	state.OutboundsPreview.SetPlaceHolder("Generated outbounds will appear here after clicking Parse...")
	state.OutboundsPreview.Wrapping = fyne.TextWrapOff
	state.OutboundsPreviewText = "Generated outbounds will appear here after clicking Parse..."
	state.OutboundsPreview.SetText(state.OutboundsPreviewText)
	// Make field read-only, but text remains black (not disabled)
	state.OutboundsPreview.OnChanged = func(text string) {
		// Restore saved text when trying to edit
		if text != state.OutboundsPreviewText {
			state.OutboundsPreview.SetText(state.OutboundsPreviewText)
		}
	}

	// Limit width and height of Preview field
	previewScroll := container.NewScroll(state.OutboundsPreview)
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


