package ui

import (
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
	"singbox-launcher/ui/components"
)

// InitWizardOverlay creates the click redirect overlay, attaches it to the app content
// and subscribes to UIService.OnStateChange so that overlay visibility follows
// wizard open/close state. Extracted to a separate file for modularity and testability.
func InitWizardOverlay(app *App, controller *core.AppController) {
	if app == nil || controller == nil {
		return
	}

	// Create overlay widget and attach it on top of the tabs
	overlay := components.NewClickRedirect(controller)
	app.overlay = overlay
	app.content = container.NewMax(app.tabs, overlay)

	// Subscribe to UIService.OnStateChange to keep overlay visibility in sync
	if controller.UIService != nil {
		origOnState := controller.UIService.OnStateChange
		controller.UIService.OnStateChange = func() {
			if origOnState != nil {
				origOnState()
			}
			// OnStateChange вызывается из wizard.go на UI-потоке — fyne.Do не нужен
			app.updateWizardOverlay()
		}
		// Set initial overlay visibility
		app.updateWizardOverlay()
	}
}

// updateWizardOverlay shows or hides the click redirect overlay depending on
// whether the Wizard is open. Kept here with InitWizardOverlay so all overlay
// logic lives in the same file.
func (a *App) updateWizardOverlay() {
	if a.overlay == nil || a.core == nil || a.core.UIService == nil {
		return
	}
	if a.core.UIService.WizardWindow != nil {
		a.overlay.Show()
		a.overlay.Refresh()
	} else {
		a.overlay.Hide()
		a.overlay.Refresh()
	}
}
