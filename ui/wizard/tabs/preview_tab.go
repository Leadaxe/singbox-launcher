package tabs

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardstate "singbox-launcher/ui/wizard/state"
)

// CreatePreviewTab creates the Preview tab UI.
func CreatePreviewTab(state *wizardstate.WizardState) fyne.CanvasObject {
	state.TemplatePreviewEntry = widget.NewMultiLineEntry()
	state.TemplatePreviewEntry.SetPlaceHolder("Preview will appear here")
	state.TemplatePreviewEntry.Wrapping = fyne.TextWrapOff
	state.TemplatePreviewEntry.OnChanged = func(text string) {
		// Read-only field, do nothing on change
	}
	previewWithHeight := container.NewMax(
		canvas.NewRectangle(color.Transparent),
		state.TemplatePreviewEntry,
	)
	state.SetTemplatePreviewText("Preview will appear here")

	previewScroll := container.NewVScroll(previewWithHeight)
	maxHeight := state.Window.Canvas().Size().Height * 0.7
	if maxHeight <= 0 {
		maxHeight = 480
	}
	previewScroll.SetMinSize(fyne.NewSize(0, maxHeight))

	// Create status label and button for generating preview
	state.TemplatePreviewStatusLabel = widget.NewLabel("Click 'Show Preview' to generate preview (this may take a long time for large configurations)")
	state.TemplatePreviewStatusLabel.Wrapping = fyne.TextWrapWord

	state.ShowPreviewButton = widget.NewButton("Show Preview", func() {
		if state.ShowPreviewButton != nil {
			state.ShowPreviewButton.Disable()
		}
		wizardbusiness.UpdateTemplatePreviewAsync(state)
	})

	// Container with status (takes all available space) and button on right
	statusRow := container.NewBorder(
		nil, nil,
		nil,                              // left
		state.ShowPreviewButton,          // right - fixed width by content
		state.TemplatePreviewStatusLabel, // center - takes all available space
	)

	return container.NewVBox(
		widget.NewLabel("Preview"),
		previewScroll,
		statusRow,
	)
}
