package main

import (
	_ "embed" // For embedding resource files (icons)
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	controller, err := core.NewAppController(appIconData, greyIconData, greenIconData)
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
			// Create the menu for the system tray.
			desk.SetSystemTrayMenu(fyne.NewMenu("Simple Sing-Box Launcher",
				fyne.NewMenuItem("Open", func() { controller.MainWindow.Show() }),
				fyne.NewMenuItemSeparator(),
				fyne.NewMenuItem("Start VPN", controller.StartSingBox),
				fyne.NewMenuItem("Stop VPN", controller.StopSingBox),
				fyne.NewMenuItemSeparator(),
				fyne.NewMenuItem("Quit", controller.GracefulExit),
			))
		})
	}

	controller.MainWindow = controller.Application.NewWindow("Simple Sing-Box Launcher") // Create the main application window
	controller.MainWindow.SetIcon(controller.AppIconData)

	// Create tabs for the main window: "Control", "Diagnostics", "Tools", "Clash API"
	tabs := container.NewAppTabs(
		container.NewTabItem("Control", ui.CreateMainContent(controller)),
		container.NewTabItem("Diagnostics", ui.CreateDiagnosticsContent(controller)),
		container.NewTabItem("Tools", ui.CreateToolsContent(controller)),
		container.NewTabItem("Clash API", ui.CreateClashAPIContent(controller)),
	)

	tabs.OnSelected = func(item *container.TabItem) {
		// Проверяем, является ли выбранная вкладка "Clash API"
		if item.Text == "Clash API" {
			// Вызываем метод контроллера для автоматического теста и загрузки
			controller.RefreshAPIFunc()
		}
	}

	controller.MainWindow.SetContent(tabs)               // Set the window's content
	controller.MainWindow.Resize(fyne.NewSize(350, 450)) // initial window size
	controller.MainWindow.CenterOnScreen()               // Center the window on the screen

	core.CheckIfLauncherAlreadyRunningUtil(controller)

	// Intercept the window close event (clicking "X") to hide it instead of exiting completely.
	controller.MainWindow.SetCloseIntercept(func() {
		controller.MainWindow.Hide()
	})

	controller.UpdateUI()

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
