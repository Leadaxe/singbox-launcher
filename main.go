package main

import (
	_ "embed" // For embedding resource files (icons)
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	// Import our new packages
	"singbox-launcher/core"
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

// main is the application's entry point. It simply creates and runs the AppController.
func main() {
	// Create the application controller. If an error occurs, print it and exit the program.
	// Use greyIconData for red icon (no separate red icon yet)
	controller, err := core.NewAppController(appIconData, greyIconData, greenIconData, greyIconData)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Configure the system tray if the application is running on a Desktop platform.
	if desk, ok := controller.Application.(desktop.App); ok {
		// Set a handler that fires when the application is fully ready
		controller.Application.Lifecycle().SetOnStarted(func() {
			go func() {
				// Add a delay before setting the icon to give the tray time to initialize
				time.Sleep(500 * time.Millisecond)
				fyne.Do(func() {
					// Set the initial icon on the main thread after the delay
					desk.SetSystemTrayIcon(controller.GreyIconData)
				})
			}()
			// Create the menu for the system tray with proxy selection submenu
			// Safe wrapper with debounce to prevent "Invalid menu handle" errors
			// when menu updates happen too quickly
			updateTrayMenu := func() {
				controller.TrayMenuUpdateMutex.Lock()
				defer controller.TrayMenuUpdateMutex.Unlock()

				// Cancel previous timer if it exists
				if controller.TrayMenuUpdateTimer != nil {
					controller.TrayMenuUpdateTimer.Stop()
				}

				// Calculate dynamic delay based on number of proxies
				// Get proxy count to determine appropriate delay
				controller.APIStateMutex.RLock()
				proxyCount := len(controller.ProxiesList)
				controller.APIStateMutex.RUnlock()

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
				controller.TrayMenuUpdateTimer = time.AfterFunc(delay, func() {
					// Check if update is already in progress
					controller.TrayMenuUpdateMutex.Lock()
					if controller.TrayMenuUpdateInProgress {
						controller.TrayMenuUpdateMutex.Unlock()
						return // Skip update if already in progress
					}
					controller.TrayMenuUpdateInProgress = true
					controller.TrayMenuUpdateMutex.Unlock()

					fyne.Do(func() {
						defer func() {
							// Reset flag after update completes
							controller.TrayMenuUpdateMutex.Lock()
							controller.TrayMenuUpdateInProgress = false
							controller.TrayMenuUpdateTimer = nil
							controller.TrayMenuUpdateMutex.Unlock()
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
			controller.UpdateTrayMenuFunc = updateTrayMenu

			// Set initial menu
			updateTrayMenu()

			// Read config once at application startup
			go func() {
				log.Println("Application startup: Reading config...")
				config, err := core.ExtractParserConfig(controller.ConfigPath)
				if err != nil {
					log.Printf("Application startup: Failed to read config: %v", err)
					return
				}
				log.Printf("Application startup: Config read successfully (version %d, %d proxy sources, %d outbounds)",
					config.ParserConfig.Version,
					len(config.ParserConfig.Proxies),
					len(config.ParserConfig.Outbounds))
			}()
		})
	}

	controller.MainWindow = controller.Application.NewWindow("Singbox Launcher") // Create the main application window
	controller.MainWindow.SetIcon(controller.AppIconData)

	// Create App structure to manage UI
	app := ui.NewApp(controller.MainWindow, controller)
	controller.MainWindow.SetContent(app.GetTabs())      // Set the window's content
	controller.MainWindow.Resize(fyne.NewSize(350, 450)) // initial window size
	controller.MainWindow.CenterOnScreen()               // Center the window on the screen

	core.CheckIfLauncherAlreadyRunningUtil(controller)

	// Intercept the window close event (clicking "X") to hide it instead of exiting completely.
	controller.MainWindow.SetCloseIntercept(func() {
		controller.MainWindow.Hide()
	})

	controller.UpdateUI()

	// Ensure tray menu is created and displayed after window is ready
	// This ensures menu is properly initialized even if SetOnStarted hasn't fired yet
	go func() {
		time.Sleep(200 * time.Millisecond) // Small delay to ensure callback is set
		if controller.UpdateTrayMenuFunc != nil {
			controller.UpdateTrayMenuFunc()
		}
	}()

	// Check if config.json exists and show a warning if it doesn't
	core.CheckConfigFileExists(controller)

	// Check Linux capabilities and suggest setup if needed
	core.CheckLinuxCapabilities(controller)

	// Check if sing-box is running on startup and show a warning if it is.
	core.CheckIfSingBoxRunningAtStartUtil(controller)

	controller.MainWindow.ShowAndRun() // Show the main window and start the main Fyne event loop.
	// The code below executes only after ShowAndRun() finishes.
	// This is where final cleanup is performed.
	log.Println("Application shutting down.")
	controller.GracefulExit()

	if controller.MainLogFile != nil {
		controller.MainLogFile.Close()
	}
	if controller.ChildLogFile != nil {
		controller.ChildLogFile.Close()
	}
	if controller.ApiLogFile != nil {
		controller.ApiLogFile.Close()
	}
}
