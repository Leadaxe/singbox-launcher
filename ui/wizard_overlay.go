package ui

import (
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
	"singbox-launcher/ui/components"
)

// wizardOverlayEnabled — feature flag for the main-window click-redirect
// overlay. When true (legacy behavior pre-v0.9.8), an invisible overlay
// sits on top of the main-window tabs while the configurator is open and
// redirects every click to focus the configurator → main window becomes
// effectively read-only.
//
// Set to false so users can drive Update / Restart / Start / Stop / Servers
// tab in parallel with the configurator. Flip back to true if you need
// the legacy "wizard owns the foreground" UX without ripping out the
// implementation.
//
// Independent of the wizard's *internal* ChildWindowsOverlay
// (`presenter.UpdateChildOverlay`), which still uses `components.ClickRedirect`
// over its own tabs to keep child dialogs (Edit Outbound, View Source,
// rule dialog) on top within the wizard window.
const wizardOverlayEnabled = false

// InitWizardOverlay creates the click redirect overlay, attaches it to the app content
// and subscribes to UIService.OnStateChange so that overlay visibility follows
// wizard open/close state. Extracted to a separate file for modularity and testability.
//
// When `wizardOverlayEnabled` is false (current default) this function is a
// near no-op: `app.content` stays as the bare tabs and no OnStateChange
// hook is registered, so clicks on the main window flow normally to their
// targets while the wizard is open.
func InitWizardOverlay(app *App, controller *core.AppController) {
	if app == nil || controller == nil {
		return
	}

	if !wizardOverlayEnabled {
		// Main-window overlay disabled — leave app.content as the bare tabs
		// so input passes through to Update / Restart / tab buttons even
		// while the configurator is open.
		app.content = app.tabs
		return
	}

	// Create overlay widget and attach it on top of the tabs
	overlay := components.NewClickRedirect(controller)
	app.overlay = overlay
	app.content = container.NewStack(app.tabs, overlay)

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
