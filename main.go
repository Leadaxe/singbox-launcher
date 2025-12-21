package main

import (
	_ "embed" // For embedding resource files (icons)
	"flag"
	"log"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	// Import our new packages
	"singbox-launcher/core"
	"singbox-launcher/core/config/parser"
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

	// Check launcher version on startup
	controller.CheckLauncherVersionOnStartup()

	// Configure the system tray if the application is running on a Desktop platform.
	if desk, ok := controller.UIService.Application.(desktop.App); ok {
		log.Println("System tray: Desktop platform detected, initializing...")
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
								log.Printf("updateTrayMenu: Recovered from panic: %v", r)
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
		log.Println("System tray: Setting icon...")
		desk.SetSystemTrayIcon(controller.UIService.GreyIconData)
		log.Println("System tray: Creating initial menu...")
		initialMenu := controller.CreateTrayMenu()
		desk.SetSystemTrayMenu(initialMenu)
		log.Println("System tray: Icon and menu initialized successfully")

		// Set a handler that fires when the application is fully ready
		controller.UIService.Application.Lifecycle().SetOnStarted(func() {
			// Read config once at application startup
			go func() {
				log.Println("Application startup: Reading config...")
				config, err := parser.ExtractParserConfig(controller.FileService.ConfigPath)
				if err != nil {
					log.Printf("Application startup: Failed to read config: %v", err)
					return
				}
				log.Printf("Application startup: Config read successfully (version %d, %d proxy sources, %d outbounds)",
					config.ParserConfig.Version,
					len(config.ParserConfig.Proxies),
					len(config.ParserConfig.Outbounds))
			}()

			// Auto-start VPN if -start flag is provided
			if *autoStart {
				go func() {
					// Wait a bit for everything to initialize
					time.Sleep(autoStartDelay)
					log.Println("Auto-start: Starting VPN due to -start parameter")
					core.StartSingBoxProcess(controller)
				}()
			}

			// Hide window if -tray flag is provided
			if *startInTray {
				go func() {
					// Wait a bit for window to be fully initialized
					time.Sleep(500 * time.Millisecond)
					fyne.Do(func() {
						if controller.UIService.MainWindow != nil {
							controller.UIService.MainWindow.Hide()
							log.Println("Tray mode: Window hidden")
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
	controller.UIService.MainWindow.SetContent(app.GetTabs())      // Set the window's content
	controller.UIService.MainWindow.Resize(fyne.NewSize(350, 450)) // initial window size
	controller.UIService.MainWindow.CenterOnScreen()               // Center the window on the screen

	core.CheckIfLauncherAlreadyRunningUtil(controller)

	// Intercept the window close event (clicking "X") to hide it instead of exiting completely.
	if controller.UIService.MainWindow != nil {
		controller.UIService.MainWindow.SetCloseIntercept(func() {
			controller.UIService.MainWindow.Hide()
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
				if controller.UIService.MainWindow != nil {
					controller.UIService.MainWindow.Show()
					controller.UIService.MainWindow.RequestFocus()
					log.Println("Dock icon clicked (native handler): Window shown and focused")
				}
			})
		})
		log.Println("Dock icon click handler registered for macOS (native NSApplicationDelegate)")
	}

	controller.UpdateUI()

	// Check if config.json exists and show a warning if it doesn't
	core.CheckConfigFileExists(controller)

	// Check Linux capabilities and suggest setup if needed
	core.CheckLinuxCapabilities(controller)

	// Check if sing-box is running on startup and show a warning if it is.
	core.CheckIfSingBoxRunningAtStartUtil(controller)

	// Use app.Run() instead of ShowAndRun() for windowless support
	// This allows the app to keep running even when window is closed/hidden
	// On macOS, this enables standard Dock behavior (applicationShouldHandleReopen)
	// See: https://github.com/fyne-io/fyne/issues/3845
	if !*startInTray {
		// Show window on startup if not starting in tray
		if controller.UIService.MainWindow != nil {
			controller.UIService.MainWindow.Show()
		}
	}

	// Start the application event loop (windowless mode)
	// This keeps the app running even when window is hidden/closed
	// The menu already has "Open" item that calls MainWindow.Show()
	controller.UIService.Application.Run()

	// The code below executes only after app.Run() finishes (when app.Quit() is called).
	// This is where final cleanup is performed.
	log.Println("Application shutting down.")

	// Cleanup platform-specific handlers
	if runtime.GOOS == "darwin" {
		platform.CleanupDockReopenHandler()
	}

	controller.GracefulExit()

	// Close log files through FileService
	if controller.FileService != nil {
		if controller.FileService.MainLogFile != nil {
			controller.FileService.MainLogFile.Close()
	}
		if controller.FileService.ChildLogFile != nil {
			controller.FileService.ChildLogFile.Close()
	}
		if controller.FileService.ApiLogFile != nil {
			controller.FileService.ApiLogFile.Close()
		}
	}
}
