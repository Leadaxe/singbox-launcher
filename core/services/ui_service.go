package services

import (
	"os"
	"sync"
	"time"

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
	FocusOpenChildWindows    func() // Focus one of wizard child windows (View, Outbound Edit, rule dialog) when user clicks wizard
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
	ui.ShowUpdatePopupFunc = func(currentVersion, latestVersion string) {
		debuglog.DebugLog("ShowUpdatePopupFunc handler is not set yet. Current: %s, Latest: %s", currentVersion, latestVersion)
	}

	return ui, nil
}

// ShowMainWindowOrFocusWizard ensures the main window is shown (unhidden),
// then if the Wizard is open it brings the Wizard to front and focuses it.
// This avoids the case where both windows are hidden and clicking "Open" does nothing.
func (ui *UIService) ShowMainWindowOrFocusWizard() {
	if ui == nil {
		return
	}
	fyne.Do(func() {
		// Always show the main window first so the application becomes visible.
		if ui.MainWindow != nil {
			ui.MainWindow.Show()
			ui.MainWindow.RequestFocus()
		}

		// If Wizard is open, ensure it is visible and focused on top of the main window.
		if ui.WizardWindow != nil {
			ui.WizardWindow.Show()
			ui.WizardWindow.RequestFocus()
		}

		// Вызываем callback после открытия окна (для проверки обновлений)
		if ui.OnWindowShown != nil {
			ui.OnWindowShown()
		}
	})
}

// UpdateUI updates all UI elements based on the current application state.
func (ui *UIService) UpdateUI() {
	fyne.Do(func() {
		// Update tray icon
		if desk, ok := ui.Application.(desktop.App); ok {
			// Check that icons are initialized
			if ui.GreenIconData == nil || ui.GreyIconData == nil || ui.RedIconData == nil {
				debuglog.WarnLog("UpdateUI: Icons not initialized, skipping icon update")
				return
			}

			var iconToSet fyne.Resource

			if ui.RunningStateIsRunning() {
				// Green icon - if running
				iconToSet = ui.GreenIconData
			} else {
				// Check for binary to determine error state
				if _, err := os.Stat(ui.SingboxPath); os.IsNotExist(err) {
					// Red icon - on error (binary not found)
					iconToSet = ui.RedIconData
				} else {
					// Grey icon - on normal stop
					iconToSet = ui.GreyIconData
				}
			}

			desk.SetSystemTrayIcon(iconToSet)
		}

		// Reset API state if VPN is down
		if !ui.RunningStateIsRunning() && ui.ResetAPIStateFunc != nil {
			debuglog.DebugLog("UpdateUI: Triggering API state reset because state is 'Down'.")
			ui.ResetAPIStateFunc()
		}

		// Update tray menu when state changes
		if ui.UpdateTrayMenuFunc != nil {
			ui.UpdateTrayMenuFunc()
		}

		// Update Core Dashboard status when state changes
		if ui.UpdateCoreStatusFunc != nil {
			ui.UpdateCoreStatusFunc()
		}

		// Don't call OnStateChange here - it would create infinite loop
		// OnStateChange is called from RunningState.Set() and other state change points
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
func (ui *UIService) QuitApplication() {
	if ui.Application != nil {
		ui.Application.Quit()
	}
}
