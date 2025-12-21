package services

import (
	"log"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/constants"
)

// UIService manages UI-related state, callbacks, and tray menu logic.
// It encapsulates all Fyne components and UI state to reduce AppController complexity.
type UIService struct {
	// Fyne Components
	Application    fyne.App
	MainWindow     fyne.Window
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

	// Callbacks for UI logic
	RefreshAPIFunc           func()
	ResetAPIStateFunc        func()
	UpdateCoreStatusFunc     func()
	UpdateConfigStatusFunc   func()
	UpdateTrayMenuFunc       func()
	UpdateParserProgressFunc func(progress float64, status string)

	// Dependencies (passed from AppController)
	RunningStateIsRunning func() bool
	SingboxPath           string
	OnStateChange         func() // Called when UI state changes
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
	log.Println("UIService: Initializing Fyne application...")
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
	ui.RefreshAPIFunc = func() { log.Println("RefreshAPIFunc handler is not set yet.") }
	ui.ResetAPIStateFunc = func() { log.Println("ResetAPIStateFunc handler is not set yet.") }
	ui.UpdateCoreStatusFunc = func() { log.Println("UpdateCoreStatusFunc handler is not set yet.") }
	ui.UpdateConfigStatusFunc = func() { log.Println("UpdateConfigStatusFunc handler is not set yet.") }
	ui.UpdateTrayMenuFunc = func() { log.Println("UpdateTrayMenuFunc handler is not set yet.") }
	ui.UpdateParserProgressFunc = func(progress float64, status string) {
		log.Printf("UpdateParserProgressFunc handler is not set yet. Progress: %.0f%%, Status: %s", progress, status)
	}

	return ui, nil
}

// UpdateUI updates all UI elements based on the current application state.
func (ui *UIService) UpdateUI() {
	fyne.Do(func() {
		// Update tray icon
		if desk, ok := ui.Application.(desktop.App); ok {
			// Check that icons are initialized
			if ui.GreenIconData == nil || ui.GreyIconData == nil || ui.RedIconData == nil {
				log.Printf("UpdateUI: Icons not initialized, skipping icon update")
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
			log.Println("UpdateUI: Triggering API state reset because state is 'Down'.")
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
