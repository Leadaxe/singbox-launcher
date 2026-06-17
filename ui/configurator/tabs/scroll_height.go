package tabs

import (
	"fyne.io/fyne/v2"

	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// adaptiveScrollHeight returns a min-height for a tab scroll area that scales
// with the current window height instead of a fixed constant. Fixed minimums
// (e.g. 620px) force the whole wizard window to grow taller than the screen on
// laptops with limited vertical space (macOS 11 Big Sur, logical 1280×800),
// pushing the navigation buttons under the Dock.
//
// frac is the fraction of the window height to use (e.g. 0.6). fallback is used
// before the window has a measured size (first layout, canvas size 0).
func adaptiveScrollHeight(guiState *wizardpresentation.GUIState, frac, fallback float32) float32 {
	if guiState != nil && guiState.Window != nil {
		if h := guiState.Window.Canvas().Size().Height; h > 0 {
			return h * frac
		}
	}
	return fallback
}

// adaptiveScrollSize is a convenience wrapper returning a width-0 Size with the
// adaptive height, ready for scroll.SetMinSize.
func adaptiveScrollSize(guiState *wizardpresentation.GUIState, frac, fallback float32) fyne.Size {
	return fyne.NewSize(0, adaptiveScrollHeight(guiState, frac, fallback))
}
