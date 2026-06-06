// File settings_window.go — separate OS-level window hosting the Settings
// UI that used to live in a main-tab page.
//
// Rationale: Settings outgrew "fits-on-one-tab" once it accumulated
// auto-update, auto-ping, language, HWID identification (SPEC 061) and
// Debug API blocks. Cramming them into a 350×450 main-window tab forced
// a scrollbar and made the sections feel like an afterthought. Promoting
// to a dedicated window keeps the main app slim and gives Settings
// breathing room (default 520×640, resizable, independent of main).
//
// Entry point: the Settings TabItem in the main AppTabs strip stays
// visible — its OnSelected handler (in ui/app.go) calls OpenSettingsWindow
// and immediately reverts tab selection back to the previously active
// tab. Tab acts as a button; the brief highlight on click is intentional
// affordance ("you clicked a thing, here's the window").
//
// Singleton via UIService.SettingsWindow: re-clicking Settings while the
// window is up just focuses the existing one rather than spawning a
// second.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
)

// OpenSettingsWindow opens (or focuses) the Settings window.
//
// Idempotent: safe to call from any goroutine via fyne.Do — on UI thread
// it either focuses the existing UIService.SettingsWindow or creates a
// new one. Window's SetCloseIntercept clears the singleton ref so the
// next call rebuilds fresh content (picks up any state mutated outside
// while the window was closed, e.g. via Debug API endpoints).
func OpenSettingsWindow(ac *core.AppController) {
	if ac == nil || ac.UIService == nil {
		return
	}

	// Singleton — already open: just focus.
	if ac.UIService.SettingsWindow != nil {
		ac.UIService.SettingsWindow.Show()
		ac.UIService.SettingsWindow.RequestFocus()
		return
	}

	app := fyne.CurrentApp()
	if app == nil {
		return
	}

	win := app.NewWindow(locale.T("settings.window_title"))
	// Padded outer wrapper + scroll: long Settings (HWID rows, port entry,
	// hint paragraphs) can exceed the visible window on narrower screens.
	// Scroll inside the window keeps every control reachable without
	// forcing the user to enlarge the window. Gutter reservation is
	// handled by the shared components.WrapInScrollWithGutter helper.
	body := container.NewPadded(buildSettingsContent(ac))
	win.SetContent(components.WrapInScrollWithGutter(body))
	win.Resize(fyne.NewSize(520, 640))
	win.CenterOnScreen()

	// Clear singleton on close so a subsequent OpenSettingsWindow call
	// builds fresh content. Without this, closing+reopening would refocus
	// a window that's been disposed, or worse, leak the reference.
	win.SetOnClosed(func() {
		if ac.UIService != nil && ac.UIService.SettingsWindow == win {
			ac.UIService.SettingsWindow = nil
		}
	})

	ac.UIService.SettingsWindow = win
	win.Show()
}
