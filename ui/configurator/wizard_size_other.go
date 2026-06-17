//go:build !darwin

package configurator

import "fyne.io/fyne/v2"

// clampWizardSize is the non-darwin fallback: it returns the requested size
// unchanged. Screen-fitting is currently only needed on macOS (Big Sur
// laptops with logical 1280×800), where wizard_size_darwin.go reads the
// display height via CoreGraphics.
func clampWizardSize(_ fyne.App, width, height float32) fyne.Size {
	return fyne.NewSize(width, height)
}
