package main

import (
	_ "embed" // For embedding resource files (icons)
	"flag"
	"log"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config/parser"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui"
)

// Embedded resources (icons for the system tray)
//
//go:embed assets/app.ico
var appIconData []byte // Main application icon

//go:embed assets/off.ico
var greyIconData []byte // Icon for "off" state

//go:embed assets/on.ico
var greenIconData []byte // Icon for "on" state

// Constants
const (
	autoStartDelay = 1 * time.Second // Delay before auto-starting VPN with -start parameter
)

// main is the application's entry point. It simply creates and runs the AppController.
func main() {
	// Parse command line arguments
	autoStart := flag.Bool("start", false, "Automatically start VPN on launch")
	startInTray := flag.Bool("tray", false, "Start minimized to system tray (hide window on launch)")
	flag.Parse()

	// Create the application controller. If an error occurs, print it and exit the program.
	// Use greyIconData for red icon (no separate red icon yet)
	controller, err := core.NewAppController(appIconData, greyIconData, greenIconData, greyIconData)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Load locale settings and external translations
	binDir := platform.GetBinDir(controller.FileService.ExecDir)
	locale.LoadExternalLocales(locale.GetLocaleDir(binDir))
	settings := locale.LoadSettings(binDir)
	locale.SetLang(settings.Lang)
	if settings.PingTestURL != "" {
		api.SetPingTestURL(settings.PingTestURL)
	}
	if settings.PingTestAllConcurrency != 0 {
		api.SetPingTestAllConcurrency(settings.PingTestAllConcurrency)
	}
	// Honor persisted opt-out of subscription auto-update. The loop is started
	// unconditionally later; this just flips the in-memory gate so it skips work.
	if settings.SubscriptionAutoUpdateDisabled {
		controller.StateService.SetAutoUpdateEnabled(false)
		debuglog.InfoLog("Auto-update: disabled by user setting (subscription_auto_update_disabled=true)")
	}
	if settings.AutoPingAfterConnectDisabled {
		controller.StateService.SetAutoPingAfterConnectEnabled(false)
		debuglog.InfoLog("Auto-ping: disabled by user setting (auto_ping_after_connect_disabled=true)")
	}
	// Optional debug-API (localhost:9269 by default). Off unless user toggled
	// it on in the Diagnostics tab; token is generated on first enable.
	if settings.DebugAPIEnabled && settings.DebugAPIToken != "" {
		if err := controller.StartDebugAPI(settings.DebugAPIPort, settings.DebugAPIToken); err != nil {
			debuglog.WarnLog("debug-api: failed to start: %v", err)
		}
	}
	debuglog.InfoLog("Locale: language set to %q, available: %v", locale.GetLang(), locale.Languages())

	// Check launcher version on startup (always checks, popup shown on first window display)
	controller.CheckLauncherVersionOnStartup()

	// Configure the system tray if the application is running on a Desktop platform.
	//nolint:unused // desktop is used for type assertion, even if linter can't detect it
	if desk, ok := controller.UIService.Application.(desktop.App); ok {
		debuglog.InfoLog("System tray: Desktop platform detected, initializing...")
		// Create the menu update function for the system tray with proxy selection submenu
		// Safe wrapper with debounce to prevent "Invalid menu handle" errors
		// when menu updates happen too quickly
		updateTrayMenu := func() {
			controller.UIService.TrayMenuUpdateMutex.Lock()
			defer controller.UIService.TrayMenuUpdateMutex.Unlock()

			// Cancel previous timer if it exists
			if controller.UIService.TrayMenuUpdateTimer != nil {
				controller.UIService.TrayMenuUpdateTimer.Stop()
			}

			// Calculate dynamic delay based on number of proxies
			// Get proxy count to determine appropriate delay
			var proxyCount int
			if controller.APIService != nil {
				proxyCount = len(controller.APIService.GetProxiesList())
			}

			// Dynamic delay formula:
			// - Base delay: 100ms for small menus (0-10 proxies)
			// - For each proxy above 10, add 20ms
			// - Maximum delay: 500ms to ensure systray has enough time for large menus
			delay := 100 * time.Millisecond
			if proxyCount > 10 {
				extraDelay := time.Duration(proxyCount-10) * 20 * time.Millisecond
				delay += extraDelay
				// Cap at 500ms maximum
				if delay > 500*time.Millisecond {
					delay = 500 * time.Millisecond
				}
			}

			// Create new timer with dynamic debounce delay
			// This prevents rapid successive menu updates that cause systray errors
			controller.UIService.TrayMenuUpdateTimer = time.AfterFunc(delay, func() {
				if platform.IsSleeping() {
					return // Skip tray menu update while system is sleeping
				}
				// Check if update is already in progress
				controller.UIService.TrayMenuUpdateMutex.Lock()
				if controller.UIService.TrayMenuUpdateInProgress {
					controller.UIService.TrayMenuUpdateMutex.Unlock()
					return // Skip update if already in progress
				}
				controller.UIService.TrayMenuUpdateInProgress = true
				controller.UIService.TrayMenuUpdateMutex.Unlock()

				fyne.Do(func() {
					defer func() {
						// Reset flag after update completes
						controller.UIService.TrayMenuUpdateMutex.Lock()
						controller.UIService.TrayMenuUpdateInProgress = false
						controller.UIService.TrayMenuUpdateTimer = nil
						controller.UIService.TrayMenuUpdateMutex.Unlock()
					}()

					menu := controller.CreateTrayMenu()
					// Use recover to handle any panics during menu update
					func() {
						defer func() {
							if r := recover(); r != nil {
								debuglog.ErrorLog("updateTrayMenu: Recovered from panic: %v", r)
							}
						}()
						desk.SetSystemTrayMenu(menu)
					}()
				})
			})
		}
		controller.UIService.UpdateTrayMenuFunc = updateTrayMenu

		// Initialize system tray immediately (required on macOS, works on Windows too)
		// On macOS, system tray must be initialized BEFORE app.Run() to work properly
		debuglog.InfoLog("System tray: Setting icon...")
		desk.SetSystemTrayIcon(controller.UIService.GreyIconData)
		debuglog.InfoLog("System tray: Creating initial menu...")
		initialMenu := controller.CreateTrayMenu()
		desk.SetSystemTrayMenu(initialMenu)
		debuglog.InfoLog("System tray: Icon and menu initialized successfully")

		// Set a handler that fires when the application is fully ready
		controller.UIService.Application.Lifecycle().SetOnStarted(func() {
			// Read config once at application startup
			go func() {
				debuglog.InfoLog("Application startup: Reading config...")
				config, err := parser.ExtractParserConfig(controller.FileService.ConfigPath)
				if err != nil {
					debuglog.WarnLog("Application startup: Failed to read config: %v", err)
					return
				}
				debuglog.InfoLog("Application startup: Config read successfully (version %d, %d proxy sources, %d outbounds)",
					config.ParserConfig.Version,
					len(config.ParserConfig.Proxies),
					len(config.ParserConfig.Outbounds))
			}()

			// Auto-start VPN if -start flag is provided
			if *autoStart {
				go func() {
					// Wait a bit for everything to initialize
					<-time.After(autoStartDelay)
					debuglog.InfoLog("Auto-start: Starting VPN due to -start parameter")
					core.StartSingBoxProcess()
				}()
			}

			// Hide window if -tray flag is provided
			if *startInTray {
				go func() {
					// Wait a bit for window to be fully initialized
					<-time.After(500 * time.Millisecond)
					fyne.Do(func() {
						if controller.UIService.MainWindow != nil {
							controller.UIService.MainWindow.Hide()
							debuglog.InfoLog("Tray mode: Window hidden")
						}
					})
				}()
			}
		})
	}

	controller.UIService.MainWindow = controller.UIService.Application.NewWindow("Singbox Launcher") // Create the main application window
	controller.UIService.MainWindow.SetIcon(controller.UIService.AppIconData)

	// Create App structure to manage UI
	app := ui.NewApp(controller.UIService.MainWindow, controller)
	controller.UIService.MainWindow.SetContent(fynetooltip.AddWindowToolTipLayer(app.GetContent(), controller.UIService.MainWindow.Canvas()))
	controller.UIService.MainWindow.Resize(fyne.NewSize(350, 450)) // initial window size
	controller.UIService.MainWindow.CenterOnScreen()               // Center the window on the screen

	core.CheckIfLauncherAlreadyRunningUtil()

	// Intercept the window close event (clicking "X") to hide it instead of exiting completely.
	if controller.UIService.MainWindow != nil {
		controller.UIService.MainWindow.SetCloseIntercept(func() {
			controller.UIService.MainWindow.Hide()
			if controller.UIService.HideAppFromDock {
				platform.HideDockIcon()
			}
		})
	}

	// Handle Dock icon click on macOS - show window when app is activated
	// This makes Dock icon behave like "Open" in tray menu
	// Platform-specific: macOS only (Dock is macOS-specific)
	// Uses native NSApplicationDelegate to handle applicationShouldHandleReopen
	// This is a workaround for Fyne issue #3845 (Dock click not showing hidden window)
	if runtime.GOOS == "darwin" {
		platform.SetupDockReopenHandler(func() {
			fyne.Do(func() {
				// Show() is safe to call even if window is already visible
				if controller.UIService != nil {
					// Ensure Dock is restored before showing the window
					platform.RestoreDockIcon()
					controller.UIService.ShowMainWindowOrFocusWizard()
					debuglog.InfoLog("Dock icon clicked (native handler): Dock restored and focused (main or wizard)")
				}
			})
		})
		debuglog.InfoLog("Dock icon click handler registered for macOS (native NSApplicationDelegate)")
	}

	controller.UpdateUI()

	// Reset Clash API HTTP connections after sleep/hibernation and re-sync UI.
	// Platform no-op where not supported (darwin/linux stubs).
	//
	// Problem: after close-lid / open-lid the launcher would often show stale
	// ping numbers and a silent Clash API silence because Windows closes TCP
	// connections during sleep. ResetClashHTTPTransport handles the sockets;
	// we add a small delay-then-refresh dance so the UI visibly reconciles
	// once network is actually back (wake event typically fires before
	// interfaces are fully up).
	platform.RegisterPowerResumeCallback(func() {
		api.ResetClashHTTPTransport()
		debuglog.InfoLog("Power resume: Clash API HTTP transport reset, scheduling re-sync")
		// Give network interfaces a couple seconds to come back, then:
		//   - Re-test the Clash API connection (RefreshAPIFunc → onTestAPIConnection
		//     in clash_api_tab.go — this also reloads the proxies list).
		//   - Trigger the auto-ping-after-connect hook if sing-box is running,
		//     so latency numbers get refreshed instead of staying stuck at
		//     pre-sleep values.
		time.AfterFunc(3*time.Second, func() {
			if controller.UIService != nil && controller.UIService.RefreshAPIFunc != nil {
				// RefreshAPIFunc (onTestAPIConnection) mutates labels directly
				// before dispatching its goroutine, so call from UI thread.
				fyne.Do(controller.UIService.RefreshAPIFunc)
			}
			if controller.RunningState != nil && controller.RunningState.IsRunning() &&
				controller.StateService != nil && controller.StateService.IsAutoPingAfterConnectEnabled() &&
				controller.UIService != nil && controller.UIService.AutoPingAfterConnectFunc != nil {
				// Another 2s beyond the Refresh so /proxies has repopulated.
				time.AfterFunc(2*time.Second, func() {
					if controller.RunningState.IsRunning() {
						debuglog.DebugLog("Power resume: triggering post-resume auto-ping")
						// AutoPingAfterConnectFunc already wraps pingAllProxies
						// in fyne.Do internally (clash_api_tab.go registers it
						// that way), but double-wrapping is a harmless no-op if
						// we're already on the UI thread and cheap insurance
						// otherwise.
						fyne.Do(controller.UIService.AutoPingAfterConnectFunc)
					}
				})
			}
		})
	})

	// Check if config.json exists and show a warning if it doesn't
	core.CheckConfigFileExists()

	// Check Linux capabilities and suggest setup if needed
	core.CheckLinuxCapabilities()

	// Check if sing-box is running on startup and show a warning if it is.
	core.CheckIfSingBoxRunningAtStartUtil()

	// Use app.Run() instead of ShowAndRun() for windowless support
	// This allows the app to keep running even when window is closed/hidden
	// On macOS, this enables standard Dock behavior (applicationShouldHandleReopen)
	// See: https://github.com/fyne-io/fyne/issues/3845
	if !*startInTray {
		// Show window on startup if not starting in tray
		if controller.UIService != nil {
			controller.UIService.ShowMainWindowOrFocusWizard()
		}
	}

	// Start the application event loop (windowless mode)
	// This keeps the app running even when window is hidden/closed
	// The menu already has "Open" item that calls MainWindow.Show()
	controller.UIService.Application.Run()

	// The code below executes only after app.Run() finishes (when app.Quit() is called).
	// This is where final cleanup is performed.
	debuglog.InfoLog("Application shutting down.")

	// Cleanup platform-specific handlers
	if runtime.GOOS == "darwin" {
		platform.CleanupDockReopenHandler()
	}
	platform.StopPowerResumeListener()

	controller.GracefulExit()

	// Close log files through FileService
	if controller.FileService != nil {
		if controller.FileService.MainLogFile != nil {
			debuglog.RunAndLog("main: close main log file", controller.FileService.MainLogFile.Close)
		}
		if controller.FileService.ChildLogFile != nil {
			debuglog.RunAndLog("main: close child log file", controller.FileService.ChildLogFile.Close)
		}
		if controller.FileService.ApiLogFile != nil {
			api.SetAPILogFile(nil)
			debuglog.RunAndLog("main: close API log file", controller.FileService.ApiLogFile.Close)
		}
	}
}
