package uiservice

import (
	"os"
	"sync"
	"time"

	"fyne.io/systray"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// UIService manages UI-related state, callbacks, and tray menu logic.
// It encapsulates all Fyne components and UI state to reduce AppController complexity.
// Placed in a separate package from other core services so that non-UI packages
// (e.g. ui/wizard/business) can import core/services without pulling in Fyne.
type UIService struct {
	// Fyne Components
	Application fyne.App
	MainWindow  fyne.Window
	// WizardWindow holds the currently open configuration wizard window (if any).
	// We store it here to implement singleton-like behavior for the wizard: only
	// one wizard window exists at a time. Other UI code checks this field to
	// decide whether to create a new wizard or focus the existing one.
	WizardWindow   fyne.Window
	TrayIcon       fyne.Resource
	ApiStatusLabel *widget.Label

	// UI State Fields
	ProxiesListWidget *widget.List
	ListStatusLabel   *widget.Label

	// Icon Resources
	AppIconData   fyne.Resource
	GreenIconData fyne.Resource
	GreyIconData  fyne.Resource
	RedIconData   fyne.Resource

	// Parser progress UI
	ParserProgressBar *widget.ProgressBar
	ParserStatusLabel *widget.Label

	// Tray menu update protection
	TrayMenuUpdateInProgress bool
	TrayMenuUpdateMutex      sync.Mutex
	TrayMenuUpdateTimer      *time.Timer

	// Dock icon visibility state (macOS only)
	HideAppFromDock bool

	// Callbacks for UI logic
	RefreshAPIFunc           func()
	ResetAPIStateFunc        func()
	UpdateCoreStatusFunc     func()
	UpdateConfigStatusFunc   func()
	UpdateTrayMenuFunc       func()
	UpdateParserProgressFunc func(progress float64, status string)

	// ShowSubsResultFunc — финальный статус subscription operation
	// (success/error). Вызывается из core/config_service.go вместо
	// legacy `dialogs.ShowAutoHideInfo` popup'а — результат рендерится
	// in-place под Exit-кнопкой (см. ui/core_dashboard_subs_status.go).
	ShowSubsResultFunc func(success bool, message string)
	// AutoPingAfterConnectFunc is scheduled by the controller ~5s after sing-box
	// transitions into the running state, so node latency in the Servers tab is
	// fresh by the time the user looks at it. Registered by the Servers tab;
	// no-op until then. Controlled by StateService.IsAutoPingAfterConnectEnabled().
	AutoPingAfterConnectFunc func()

	// LxdOverride*Func — управление remote-override вкладки Servers из
	// Debug API (SPEC 100 §3.8): то же, что кнопки Connect/Disconnect на
	// вкладке Remote. Регистрируются UI-слоем (ui/lxd_remote_override.go);
	// nil = UI ещё не создан или лаунчер headless — API отвечает 503.
	LxdOverrideConnectFunc    func(id string) error
	LxdOverrideDisconnectFunc func()
	LxdOverrideStateFunc      func() (id, name string, active bool)
	FocusOpenChildWindows    func()                                     // Focus one of wizard child windows (View, Outbound Edit, rule dialog) when user clicks wizard
	ShowUpdatePopupFunc      func(currentVersion, latestVersion string) // Called to show update popup

	// Dependencies (passed from AppController)
	RunningStateIsRunning func() bool
	SingboxPath           string
	// OnStateChange — опциональный callback, который вызывается при изменениях
	// UI-связанного состояния (например, открытие/закрытие визарда).
	// Используется для того, чтобы UI-компоненты (например, overlay) могли
	// подстраиваться под текущее состояние без жёсткой связи между слоями.
	OnStateChange func() // Called when UI state changes
	// OnWindowShown — опциональный callback, который вызывается после открытия главного окна
	// Используется для проверки обновлений при первом открытии окна после запуска с -tray
	OnWindowShown func() // Called after main window is shown
	// OnWindowHidden — парный к OnWindowShown: окно ушло в трей.
	//
	// Нужен потребителям с периодическим опросом (авто-обновление списка
	// узлов на Remote): пока окно скрыто, их запросы уходят в никуда — данные
	// никто не видит, а сеть и удалённая машина нагружаются.
	OnWindowHidden func() // Called after main window is hidden to tray
}

// HideMainWindow прячет главное окно в трей, уведомляя подписчиков.
//
// Существует ради OnWindowHidden: прямой MainWindow.Hide() в местах вызова
// оставлял бы фоновые тикеры работать на невидимое окно.
func (ui *UIService) HideMainWindow() {
	if ui == nil || ui.MainWindow == nil {
		return
	}
	ui.MainWindow.Hide()
	if ui.OnWindowHidden != nil {
		ui.OnWindowHidden()
	}
}

// NewUIService creates and initializes a new UIService instance.
func NewUIService(appIconData, greyIconData, greenIconData, redIconData []byte,
	runningStateIsRunning func() bool, singboxPath string, onStateChange func()) (*UIService, error) {
	ui := &UIService{
		RunningStateIsRunning: runningStateIsRunning,
		SingboxPath:           singboxPath,
		OnStateChange:         onStateChange,
	}

	// Initialize icon resources
	ui.AppIconData = fyne.NewStaticResource("appIcon", appIconData)
	ui.GreyIconData = fyne.NewStaticResource("trayIcon", greyIconData)
	ui.GreenIconData = fyne.NewStaticResource("runningIcon", greenIconData)
	ui.RedIconData = fyne.NewStaticResource("errorIcon", redIconData)

	// Initialize Fyne application
	debuglog.InfoLog("UIService: Initializing Fyne application...")
	ui.Application = app.NewWithID("com.singbox.launcher")
	ui.Application.SetIcon(ui.AppIconData)

	// Set theme based on constants
	switch constants.AppTheme {
	case "dark":
		ui.Application.Settings().SetTheme(theme.DarkTheme())
	case "light":
		ui.Application.Settings().SetTheme(theme.LightTheme())
	default:
		ui.Application.Settings().SetTheme(theme.DefaultTheme())
	}

	// Initialize callbacks with default no-op handlers
	ui.RefreshAPIFunc = func() { debuglog.DebugLog("RefreshAPIFunc handler is not set yet.") }
	ui.ResetAPIStateFunc = func() { debuglog.DebugLog("ResetAPIStateFunc handler is not set yet.") }
	ui.UpdateCoreStatusFunc = func() {} // placeholder until dashboard sets real handler
	ui.UpdateConfigStatusFunc = func() { debuglog.DebugLog("UpdateConfigStatusFunc handler is not set yet.") }
	ui.UpdateTrayMenuFunc = func() { debuglog.DebugLog("UpdateTrayMenuFunc handler is not set yet.") }
	ui.UpdateParserProgressFunc = func(progress float64, status string) {
		debuglog.DebugLog("UpdateParserProgressFunc handler is not set yet. Progress: %.0f%%, Status: %s", progress, status)
	}
	ui.AutoPingAfterConnectFunc = func() {
		debuglog.DebugLog("AutoPingAfterConnectFunc handler is not set yet.")
	}
	ui.ShowUpdatePopupFunc = func(currentVersion, latestVersion string) {
		debuglog.DebugLog("ShowUpdatePopupFunc handler is not set yet. Current: %s, Latest: %s", currentVersion, latestVersion)
	}

	return ui, nil
}

// forceRepaint marks a window's content dirty right after Show().
//
// Un-hiding takes Fyne's doShowAgain path, which repaints immediately while
// the canvas may still be growing to the content's MinSize (content mutated
// in the background — subscription refresh, status changes — is only applied
// on the first repaint, since hidden windows don't redraw). That repaint sets
// the GL viewport from canvas.Size() rather than the real framebuffer, so the
// frame renders taller than the buffer and, OpenGL's origin being bottom-left,
// the top of the UI (the tab strip) is cut off. The window then catches up in
// size, but the resize is a no-op that leaves no dirty flag, so the broken
// frame stays on screen until something happens to repaint.
//
// Refresh() sets that dirty flag, so the next frame renders with geometry the
// window and canvas agree on. Windows-only in practice (see issue #92): on
// macOS orderFront forces an honest repaint anyway. Cheap enough not to gate
// on GOOS.
func forceRepaint(w fyne.Window) {
	if c := w.Canvas(); c != nil {
		if content := c.Content(); content != nil {
			content.Refresh()
		}
	}
}

// ShowMainWindowOrFocusWizard ensures the main window is shown (unhidden),
// then if the Wizard is open it brings the Wizard to front and focuses it.
// This avoids the case where both windows are hidden and clicking "Open" does nothing.
func (ui *UIService) ShowMainWindowOrFocusWizard() {
	if ui == nil {
		return
	}
	fyne.Do(func() {
		if ui.MainWindow != nil {
			ui.MainWindow.Show()
			ui.MainWindow.RequestFocus()
			forceRepaint(ui.MainWindow)
		}

		if ui.WizardWindow != nil {
			ui.WizardWindow.Show()
			ui.WizardWindow.RequestFocus()
			forceRepaint(ui.WizardWindow)
		}

		if ui.OnWindowShown != nil {
			ui.OnWindowShown()
		}
	})
}

// UpdateUI updates all UI elements based on the current application state.
func (ui *UIService) UpdateUI() {
	fyne.Do(func() {
		if desk, ok := ui.Application.(desktop.App); ok {
			if ui.GreenIconData == nil || ui.GreyIconData == nil || ui.RedIconData == nil {
				debuglog.WarnLog("UpdateUI: Icons not initialized, skipping icon update")
				return
			}

			var iconToSet fyne.Resource

			if ui.RunningStateIsRunning() {
				iconToSet = ui.GreenIconData
			} else {
				if _, err := os.Stat(ui.SingboxPath); os.IsNotExist(err) {
					iconToSet = ui.RedIconData
				} else {
					iconToSet = ui.GreyIconData
				}
			}

			desk.SetSystemTrayIcon(iconToSet)
		}

		if !ui.RunningStateIsRunning() && ui.ResetAPIStateFunc != nil {
			debuglog.DebugLog("UpdateUI: Triggering API state reset because state is 'Down'.")
			ui.ResetAPIStateFunc()
		}

		if ui.UpdateTrayMenuFunc != nil {
			ui.UpdateTrayMenuFunc()
		}

		if ui.UpdateCoreStatusFunc != nil {
			ui.UpdateCoreStatusFunc()
		}
	})
}

// StopTrayMenuUpdateTimer safely stops the tray menu update timer.
func (ui *UIService) StopTrayMenuUpdateTimer() {
	ui.TrayMenuUpdateMutex.Lock()
	defer ui.TrayMenuUpdateMutex.Unlock()
	if ui.TrayMenuUpdateTimer != nil {
		ui.TrayMenuUpdateTimer.Stop()
		ui.TrayMenuUpdateTimer = nil
	}
}

// QuitApplication quits the Fyne application.
//
// Fyne's glfw driver runs its tray teardown (d.trayStop) inside Quit() only
// when `curWindow != nil`, i.e. when one of our windows currently holds focus
// (internal/driver/glfw/driver.go). Quitting from the tray menu with the main
// window hidden or unfocused therefore skips trayStop: on Windows the
// notification-area icon is never NIM_DELETE'd and systray's message pump
// keeps running, which is how quit-from-tray leaves a ghost icon and a
// lingering process behind (tray issue reported 2026-08).
//
// systray.Quit() below is the exact function fyne's own trayStop ends with
// (RunWithExternalLoop's `end` = nativeEnd + Quit) and it is guarded by a
// sync.Once, so calling it explicitly is safe whether or not the driver later
// gets to its own teardown. On Windows it posts WM_CLOSE to the systray window
// and deletes the notify icon immediately.
//
// Application.Quit() then closes all windows and unconditionally closes the
// driver's done channel, unwinding Run(). If the event loop still refuses to
// exit, the watchdog armed in GracefulExit force-exits the process.
//
// fyne.Do is correct from every caller context: tray actions and widget
// callbacks already run on the UI thread (the driver marshals tray actions
// via runOnMain), where Do simply queues for the next loop iteration; after
// the loop has died it executes inline (drained fast-path in
// runOnMainWithWait). Running on the UI thread also makes the darwin
// systray.Quit (AppKit NSStatusItem removal) thread-correct.
func (ui *UIService) QuitApplication() {
	if ui.Application == nil {
		return
	}

	fyne.Do(func() {
		systray.Quit()
		ui.Application.Quit()
	})
}
