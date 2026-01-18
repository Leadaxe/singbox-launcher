package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core/config/parser"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/process"
)

// Constants for log file names
const (
	logFileName       = "logs/" + constants.MainLogFileName
	childLogFileName  = "logs/" + constants.ChildLogFileName
	parserLogFileName = "logs/" + constants.ParserLogFileName
	apiLogFileName    = "logs/" + constants.APILogFileName
	restartDelay      = 2 * time.Second
)

// Constants for auto-update configuration
const (
	autoUpdateMinInterval   = 10 * time.Minute // Minimum check interval (constant)
	autoUpdateRetryInterval = 10 * time.Second // Interval between retry attempts
	autoUpdateMaxRetries    = 10               // Maximum consecutive failed attempts
	autoUpdateDefaultReload = "4h"             // Default reload interval if not specified
)

// AppController - the main structure encapsulating all application state and logic.
// AppController is the central controller coordinating all application components.
// It manages UI state, process lifecycle, configuration, API interactions, and logging.
// The controller delegates specific responsibilities to specialized services:
// - ProcessService: sing-box process management
// - ConfigService: configuration parsing and updates
// The controller maintains application-wide state and provides callbacks for UI updates.
type AppController struct {
	// --- Services ---
	// UIService manages UI-related state, callbacks, and tray menu logic
	UIService *services.UIService
	// APIService manages Clash API interactions and proxy list management
	APIService *services.APIService
	// StateService manages application state including version caches and auto-update state
	StateService *services.StateService
	// FileService manages file paths and log file handles
	FileService *services.FileService
	// ProcessService manages sing-box process lifecycle (start, stop, monitor, auto-restart)
	ProcessService *ProcessService
	// ConfigService handles configuration parsing, subscription fetching, and JSON generation
	ConfigService *ConfigService

	// --- Process State ---
	SingboxCmd               *exec.Cmd
	CmdMutex                 sync.Mutex
	ParserMutex              sync.Mutex // Mutex for ParserRunning
	ParserRunning            bool
	StoppedByUser            bool
	ConsecutiveCrashAttempts int

	// --- VPN Operation State ---
	RunningState *RunningState

	// --- Context for goroutine cancellation ---
	ctx        context.Context    // Context for cancellation
	cancelFunc context.CancelFunc // Cancel function for stopping goroutines
}

// RunningState - structure for tracking the VPN's running state.
type RunningState struct {
	running bool
	sync.Mutex
	controller *AppController
}

// NewAppController creates and initializes a new AppController instance.
func NewAppController(appIconData, greyIconData, greenIconData, redIconData []byte) (*AppController, error) {
	ac := &AppController{}

	// Initialize FileService first (needed by other services)
	fileService, err := services.NewFileService()
	if err != nil {
		return nil, fmt.Errorf("NewAppController: cannot create FileService: %w", err)
	}
	ac.FileService = fileService

	// Open log files with rotation support
	if err := ac.FileService.OpenLogFiles(logFileName, childLogFileName, apiLogFileName); err != nil {
		return nil, fmt.Errorf("NewAppController: cannot open log files: %w", err)
	}

	// Initialize RunningState before UIService (needed for callback)
	ac.RunningState = &RunningState{controller: ac}
	ac.RunningState.Set(false)

	// Initialize UIService
	uiService, err := services.NewUIService(
		appIconData, greyIconData, greenIconData, redIconData,
		func() bool { return ac.RunningState.IsRunning() },
		ac.FileService.SingboxPath,
		func() { ac.UpdateUI() },
	)
	if err != nil {
		return nil, fmt.Errorf("NewAppController: cannot create UIService: %w", err)
	}
	ac.UIService = uiService
	ac.RunningState = &RunningState{controller: ac}
	ac.RunningState.Set(false) // Use Set() method instead of direct assignment
	ac.ConsecutiveCrashAttempts = 0
	ac.ProcessService = NewProcessService(ac)
	ac.ConfigService = NewConfigService(ac)

	// Initialize APIService
	apiService, err := services.NewAPIService(
		ac.FileService.ConfigPath,
		ac.FileService.ApiLogFile,
		func() bool { return ac.RunningState.IsRunning() },
		func() {
			// OnProxiesUpdated callback
			if ac.UIService != nil {
				if ac.UIService.ProxiesListWidget != nil {
					ac.UIService.ProxiesListWidget.Refresh()
				}
				if ac.UIService.ListStatusLabel != nil {
					group := ac.APIService.GetSelectedClashGroup()
					active := ac.APIService.GetActiveProxyName()
					ac.UIService.ListStatusLabel.SetText(fmt.Sprintf("Proxies loaded for '%s'. Active: %s", group, active))
				}
				if ac.UIService.RefreshAPIFunc != nil {
					ac.UIService.RefreshAPIFunc()
				}
				if ac.UIService.UpdateTrayMenuFunc != nil {
					ac.UIService.UpdateTrayMenuFunc()
				}
			}
		},
		func() {
			// OnProxySwitched callback
			if ac.UIService != nil {
				if ac.UIService.UpdateTrayMenuFunc != nil {
					ac.UIService.UpdateTrayMenuFunc()
				}
				if ac.UIService.RefreshAPIFunc != nil {
					ac.UIService.RefreshAPIFunc()
				}
			}
		},
	)
	if err != nil {
		return nil, fmt.Errorf("NewAppController: cannot create APIService: %w", err)
	}
	ac.APIService = apiService

	// Initialize UI callbacks (delegated to UIService)
	ac.UIService.RefreshAPIFunc = func() { log.Println("RefreshAPIFunc handler is not set yet.") }
	ac.UIService.ResetAPIStateFunc = func() { log.Println("ResetAPIStateFunc handler is not set yet.") }
	ac.UIService.UpdateCoreStatusFunc = func() { log.Println("UpdateCoreStatusFunc handler is not set yet.") }
	ac.UIService.UpdateConfigStatusFunc = func() { log.Println("UpdateConfigStatusFunc handler is not set yet.") }
	ac.UIService.UpdateTrayMenuFunc = func() { log.Println("UpdateTrayMenuFunc handler is not set yet.") }
	ac.UIService.UpdateParserProgressFunc = func(progress float64, status string) {
		log.Printf("UpdateParserProgressFunc handler is not set yet. Progress: %.0f%%, Status: %s", progress, status)
	}

	// Initialize context for goroutine cancellation
	ac.ctx, ac.cancelFunc = context.WithCancel(context.Background())

	// Initialize StateService
	ac.StateService = services.NewStateService()

	// Check if config file exists before starting auto-update
	if _, err := os.Stat(ac.FileService.ConfigPath); os.IsNotExist(err) {
		log.Printf("Auto-update: Config file does not exist (%s), auto-update disabled", ac.FileService.ConfigPath)
		ac.StateService.SetAutoUpdateEnabled(false)
	}
	go ac.startAutoUpdateLoop()
	return ac, nil
}

// UpdateUI updates all UI elements based on the current application state.
func (ac *AppController) UpdateUI() {
	if ac.UIService != nil {
		ac.UIService.UpdateUI()
	}
}

// GetApplication returns the Fyne application instance.
func (ac *AppController) GetApplication() fyne.App {
	if ac.UIService != nil {
		return ac.UIService.Application
	}
	return nil
}

// GetMainWindow returns the main window instance.
func (ac *AppController) GetMainWindow() fyne.Window {
	if ac.UIService != nil {
		return ac.UIService.MainWindow
	}
	return nil
}

// GracefulExit performs a graceful shutdown of the application.
func (ac *AppController) GracefulExit() {
	// Cancel context to signal all goroutines to stop
	if ac.cancelFunc != nil {
		ac.cancelFunc()
		log.Println("GracefulExit: Context cancelled, signalling goroutines to stop")
	}

	// Stop any pending menu update timer
	if ac.UIService != nil {
		ac.UIService.StopTrayMenuUpdateTimer()
	}

	StopSingBoxProcess(ac)

	log.Println("GracefulExit: Waiting for sing-box to stop...")
	// Use ProcessService constant for timeout
	timeout := time.After(2 * time.Second) // gracefulShutdownTimeout from ProcessService
	for {
		if !ac.RunningState.IsRunning() {
			log.Println("GracefulExit: Sing-box confirmed stopped.")
			break
		}
		select {
		case <-timeout:
			log.Println("GracefulExit: Timeout waiting for sing-box to stop. Forcing kill.")
			ac.CmdMutex.Lock()
			if ac.SingboxCmd != nil && ac.SingboxCmd.Process != nil {
				_ = ac.SingboxCmd.Process.Kill()
			}
			ac.CmdMutex.Unlock()
			goto end_loop
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
end_loop:

	if ac.FileService != nil {
		ac.FileService.CloseLogFiles()
	}

	if ac.UIService != nil {
		ac.UIService.QuitApplication()
	}
}

// RunHidden launches an external command in a hidden window.
func (ac *AppController) RunHidden(name string, args []string, logPath string, dir string) error {
	cmd := exec.Command(name, args...)
	platform.PrepareCommand(cmd)
	if dir != "" {
		cmd.Dir = dir
	}

	if logPath != "" {
		if logPath == filepath.Join(ac.FileService.ExecDir, childLogFileName) && ac.FileService.ChildLogFile != nil {
			// For sing-box logs, check and rotate if needed before writing
			ac.FileService.CheckAndRotateLogFile(logPath)
			logFile := ac.FileService.ChildLogFile
			// Don't truncate - append to preserve logs, rotation handles size limits
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		} else {
			// For other logs (parser), use truncate mode for clean start
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("RunHidden: cannot open log file '%s': %w", logPath, err)
			}
			defer debuglog.RunAndLog(fmt.Sprintf("RunHidden: close log file %s", logPath), logFile.Close)
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
	}

	return cmd.Run()
}

// CheckLinuxCapabilities checks Linux capabilities and shows a suggestion if needed
func CheckLinuxCapabilities(ac *AppController) {
	if suggestion := platform.CheckAndSuggestCapabilities(ac.FileService.SingboxPath); suggestion != "" {
		log.Printf("CheckLinuxCapabilities: %s", suggestion)
		// Show info dialog (not error) - capabilities can be set later
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowInfo(ac.UIService.MainWindow, "Linux Capabilities", suggestion)
		}
	}
}

// Set sets the new value for the 'running' state and triggers a UI update.
func (r *RunningState) Set(value bool) {
	r.Lock()
	if r.running == value {
		r.Unlock()
		return
	}
	r.running = value
	r.Unlock()

	r.controller.UpdateUI()
	// Call callback to update status in Core Dashboard
	if r.controller.UIService != nil && r.controller.UIService.UpdateCoreStatusFunc != nil {
		r.controller.UIService.UpdateCoreStatusFunc()
	}
}

// IsRunning checks if the VPN is running.
func (r *RunningState) IsRunning() bool {
	r.Lock()
	defer r.Unlock()
	return r.running
}

// SetProxiesList safely sets the proxies list with mutex protection.
func (ac *AppController) SetProxiesList(proxies []api.ProxyInfo) {
	if ac.APIService != nil {
		ac.APIService.SetProxiesList(proxies)
	}
}

// GetProxiesList safely gets a copy of the proxies list with mutex protection.
func (ac *AppController) GetProxiesList() []api.ProxyInfo {
	if ac.APIService != nil {
		return ac.APIService.GetProxiesList()
	}
	return []api.ProxyInfo{}
}

// SetActiveProxyName safely sets the active proxy name with mutex protection.
func (ac *AppController) SetActiveProxyName(name string) {
	if ac.APIService != nil {
		ac.APIService.SetActiveProxyName(name)
	}
}

// GetActiveProxyName safely gets the active proxy name with mutex protection.
func (ac *AppController) GetActiveProxyName() string {
	if ac.APIService != nil {
		return ac.APIService.GetActiveProxyName()
	}
	return ""
}

// SetLastSelectedProxyForGroup sets the last selected proxy name for a specific selector group.
func (ac *AppController) SetLastSelectedProxyForGroup(group, name string) {
	if ac.APIService != nil {
		ac.APIService.SetLastSelectedProxyForGroup(group, name)
	}
}

// GetLastSelectedProxyForGroup gets the last selected proxy name for a specific selector group.
func (ac *AppController) GetLastSelectedProxyForGroup(group string) string {
	if ac.APIService != nil {
		return ac.APIService.GetLastSelectedProxyForGroup(group)
	}
	return ""
}

// SetSelectedIndex safely sets the selected index with mutex protection.
func (ac *AppController) SetSelectedIndex(index int) {
	if ac.APIService != nil {
		ac.APIService.SetSelectedIndex(index)
	}
}

// GetSelectedIndex safely gets the selected index with mutex protection.
func (ac *AppController) GetSelectedIndex() int {
	if ac.APIService != nil {
		return ac.APIService.GetSelectedIndex()
	}
	return -1
}

// getOurPID safely gets the PID of the tracked sing-box process
func getOurPID(ac *AppController) int {
	ac.CmdMutex.Lock()
	defer ac.CmdMutex.Unlock()
	if ac.SingboxCmd != nil && ac.SingboxCmd.Process != nil {
		return ac.SingboxCmd.Process.Pid
	}
	return -1
}

// parseCSVLine parses a CSV line, handling quoted fields
func parseCSVLine(line string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, r := range line {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}
	// Add remaining content after the loop
	if current.Len() > 0 || len(parts) > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// StartSingBoxProcess launches the sing-box process.
// skipRunningCheck: если true, пропускает проверку на уже запущенный процесс (для автоперезапуска)
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func StartSingBoxProcess(ac *AppController, skipRunningCheck ...bool) {
	if ac.ProcessService == nil {
		log.Printf("StartSingBoxProcess: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.Start(skipRunningCheck...)
}

// MonitorSingBoxProcess monitors the sing-box process.
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func MonitorSingBoxProcess(ac *AppController, cmdToMonitor *exec.Cmd) {
	if ac.ProcessService == nil {
		log.Printf("MonitorSingBoxProcess: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.Monitor(cmdToMonitor)
}

// StopSingBoxProcess is the unified function to stop the sing-box process.
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func StopSingBoxProcess(ac *AppController) {
	if ac.ProcessService == nil {
		log.Printf("StopSingBoxProcess: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.Stop()
}

// RunParserProcess starts the internal configuration update process.
// Note: ConfigService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func RunParserProcess(ac *AppController) {
	if ac.ConfigService == nil {
		log.Printf("RunParserProcess: ConfigService is nil, this should not happen. Initializing...")
		ac.ConfigService = NewConfigService(ac)
	}
	ac.ConfigService.RunParserProcess()
}

// CheckIfSingBoxRunningAtStartUtil checks if sing-box is already running at application start.
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func CheckIfSingBoxRunningAtStartUtil(ac *AppController) {
	if ac.ProcessService == nil {
		log.Printf("CheckIfSingBoxRunningAtStartUtil: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.CheckIfRunningAtStart()
}

// CheckConfigFileExists checks if config.json exists and shows a warning if it doesn't
func CheckConfigFileExists(ac *AppController) {
	if _, err := os.Stat(ac.FileService.ConfigPath); os.IsNotExist(err) {
		log.Printf("CheckConfigFileExists: config.json not found at %s", ac.FileService.ConfigPath)

		message := fmt.Sprintf(
			"⚠️ Configuration file not found!\n\n"+
				"The file %s is missing from the bin/ folder.\n\n"+
				"To get started:\n"+
				"1. download Wizard\n"+
				"2. use Wizard to generate a configuration file\n"+
				"3. press Start\n",
			constants.ConfigFileName,
		)

		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowInfo(ac.UIService.MainWindow, "Configuration Not Found", message)
		}
	}
}

func CheckIfLauncherAlreadyRunningUtil(ac *AppController) {
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("CheckIfLauncherAlreadyRunning: cannot detect executable path: %v", err)
		return
	}
	execName := strings.ToLower(filepath.Base(execPath))
	currentPID := os.Getpid()

	processes, err := process.GetProcesses()
	if err != nil {
		log.Printf("CheckIfLauncherAlreadyRunning: error listing processes: %v", err)
		return
	}

	for _, p := range processes {
		if p.PID == currentPID {
			continue
		}
		if strings.EqualFold(p.Name, execName) {
			if ac.UIService != nil && ac.UIService.MainWindow != nil {
				dialogs.ShowInfo(ac.UIService.MainWindow, "Information", "The application is already running. Use the existing instance or close it before starting a new one.")
			}
			return
		}
	}
}

func CheckFilesUtil(ac *AppController) {
	files := platform.GetRequiredFiles(ac.FileService.ExecDir)
	msg := "File check:\n\n"
	allOk := true
	for _, f := range files {
		info, err := os.Stat(f.Path)
		if err == nil {
			size := FormatBytesUtil(info.Size())
			msg += fmt.Sprintf("%s (%s): Found (%s)\n", f.Name, f.Path, size)
		} else {
			msg += fmt.Sprintf("%s (%s): Not Found (Error: %v)\n", f.Name, f.Path, err)
			allOk = false
		}
	}
	if allOk {
		msg += "\nAll files found. ✅"
	} else {
		msg += "\nSome files missing. ❌"
	}
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowInfo(ac.UIService.MainWindow, "File Check", msg)
	}
}

func FormatBytesUtil(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func ShowSingBoxAlreadyRunningWarningUtil(ac *AppController) {
	label := widget.NewLabel("Sing-Box appears to be already running.\nWould you like to kill the existing process?")
	killButton := widget.NewButton("Kill Process", nil)
	closeButton := widget.NewButton("Close This Warning", nil)
	content := container.NewVBox(label, killButton, closeButton)
	var d dialog.Dialog
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		d = dialog.NewCustomWithoutButtons("Warning", content, ac.UIService.MainWindow)
	}
	killButton.OnTapped = func() {
		go func() {
			processName := platform.GetProcessNameForCheck()
			_ = platform.KillProcess(processName)
			ac.RunningState.Set(false)
		}()
		fyne.Do(func() { d.Hide() })
	}
	closeButton.OnTapped = func() { fyne.Do(func() { d.Hide() }) }
	fyne.Do(func() { d.Show() })
}

// AutoLoadProxies attempts to load proxies with retry intervals (1, 3, 7, 13, 17 seconds).
func (ac *AppController) AutoLoadProxies() {
	if ac.APIService != nil {
		ac.APIService.AutoLoadProxies(ac.ctx)
	}
}

// VPNButtonState represents the state of Start/Stop VPN buttons
type VPNButtonState struct {
	BinaryExists bool
	IsRunning    bool
	StartEnabled bool
	StopEnabled  bool
}

// GetVPNButtonState returns the current state for VPN buttons (used by both Core Dashboard and Tray Menu)
func (ac *AppController) GetVPNButtonState() VPNButtonState {
	// Check if sing-box executable exists (same logic as Core Dashboard tab)
	_, err := ac.GetInstalledCoreVersion()
	binaryExists := err == nil

	// Check if config.json exists
	configExists := false
	if _, err := os.Stat(ac.FileService.ConfigPath); err == nil {
		configExists = true
	}

	// Check if wintun.dll exists (only on Windows)
	wintunExists := true // Default to true for non-Windows
	if runtime.GOOS == "windows" {
		exists, err := ac.CheckWintunDLL()
		if err != nil {
			// Error checking - assume not available
			wintunExists = false
		} else {
			wintunExists = exists
		}
	}

	// Get current running state
	isRunning := ac.RunningState.IsRunning()

	state := VPNButtonState{
		BinaryExists: binaryExists,
		IsRunning:    isRunning,
	}

	// Determine button states based on all requirements
	// Start button is enabled only if:
	// - sing-box binary exists
	// - config.json exists
	// - wintun.dll exists (on Windows)
	// - VPN is not already running
	allRequirementsMet := binaryExists && configExists && wintunExists

	if allRequirementsMet {
		if isRunning {
			// VPN is running - Start disabled, Stop enabled
			state.StartEnabled = false
			state.StopEnabled = true
		} else {
			// VPN is not running and all requirements met - Start enabled, Stop disabled
			state.StartEnabled = true
			state.StopEnabled = false
		}
	} else {
		// Requirements not met - both buttons disabled
		state.StartEnabled = false
		state.StopEnabled = false
	}

	return state
}

// addHideDockMenuItem adds "Hide app from Dock" toggle menu item (macOS only)
func (ac *AppController) addHideDockMenuItem(menuItems []*fyne.MenuItem) []*fyne.MenuItem {
	if runtime.GOOS != "darwin" {
		return menuItems
	}

	hideDockEnabled := ac.UIService.HideAppFromDock
	hideDockLabel := "Hide app from Dock"
	if hideDockEnabled {
		hideDockLabel = "✓ " + hideDockLabel
	}

	menuItems = append(menuItems, fyne.NewMenuItem(hideDockLabel, func() {
		// Toggle the preference
		ac.UIService.HideAppFromDock = !ac.UIService.HideAppFromDock

		// Apply the change immediately on macOS
		if runtime.GOOS == "darwin" {
			if ac.UIService.HideAppFromDock {
				platform.HideDockIcon()
				// Also hide the main window when hiding from Dock
				if ac.UIService.MainWindow != nil {
					ac.UIService.MainWindow.Hide()
				}
				log.Println("Tray: Hide app from Dock enabled — Dock hidden and window hidden")
			} else {
				platform.RestoreDockIcon()
				// Restore and show the main window when unchecking (or focus wizard if open)
				if ac.UIService != nil {
					ac.UIService.ShowMainWindowOrFocusWizard()
				}
				log.Println("Tray: Hide app from Dock disabled — Dock restored and window shown")
			}
		}

		if ac.UIService.UpdateTrayMenuFunc != nil {
			ac.UIService.UpdateTrayMenuFunc()
		}
	}))
	menuItems = append(menuItems, fyne.NewMenuItemSeparator())

	return menuItems
}

// CreateTrayMenu creates the system tray menu with proxy selection submenu
func (ac *AppController) CreateTrayMenu() *fyne.Menu {
	/**
	@TODO:if ac.APIService == nil { кажется это приводит к дублированию кода, может лучше бы делать if ac.APIService != nil {
	*/
	if ac.APIService == nil {
		// Return minimal menu if APIService is not initialized
		menuItems := []*fyne.MenuItem{
			fyne.NewMenuItem("Open", func() {
				if ac.UIService != nil {
					platform.RestoreDockIcon()
					ac.UIService.ShowMainWindowOrFocusWizard()
				}
			}),
			fyne.NewMenuItemSeparator(),
		}

		if runtime.GOOS == "darwin" {
			menuItems = ac.addHideDockMenuItem(menuItems)
		}
		menuItems = append(menuItems, fyne.NewMenuItem("Quit", ac.GracefulExit))
		return fyne.NewMenu("Singbox Launcher", menuItems...)
	}

	// Get proxies from current group
	proxies := ac.APIService.GetProxiesList()
	activeProxy := ac.APIService.GetActiveProxyName()
	selectedGroup := ac.APIService.GetSelectedClashGroup()
	_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()

	// Auto-load proxies if list is empty and API is enabled
	// Note: AutoLoadProxies has internal guard to prevent multiple simultaneous loads
	if clashAPIEnabled && selectedGroup != "" && len(proxies) == 0 {
		// Only auto-load if sing-box is running
		if ac.RunningState.IsRunning() {
			// Check if auto-load is already in progress to avoid duplicate calls
			ac.APIService.AutoLoadMutex.Lock()
			alreadyInProgress := ac.APIService.AutoLoadInProgress
			ac.APIService.AutoLoadMutex.Unlock()

			if !alreadyInProgress {
				// Start auto-loading in background (non-blocking)
				go ac.AutoLoadProxies()
			}
		}
	}

	// Create proxy submenu items
	var proxyMenuItems []*fyne.MenuItem
	if clashAPIEnabled && selectedGroup != "" && len(proxies) > 0 {
		for i := range proxies {
			proxy := proxies[i]
			proxyName := proxy.Name
			isActive := proxyName == activeProxy

			// Create local copy for closure
			pName := proxyName
			menuItem := fyne.NewMenuItem(proxyName, func() {
				// Switch to selected proxy
				go func() {
					err := ac.APIService.SwitchProxy(selectedGroup, pName)
					fyne.Do(func() {
						if err != nil {
							log.Printf("CreateTrayMenu: Failed to switch proxy: %v", err)
							if ac.UIService != nil && ac.UIService.MainWindow != nil {
								dialogs.ShowError(ac.UIService.MainWindow, err)
							}
						}
						// OnProxySwitched callback is already called in APIService.SwitchProxy
					})
				}()
			})

			// Mark active proxy with checkmark
			if isActive {
				menuItem.Label = "✓ " + proxyName
			}

			proxyMenuItems = append(proxyMenuItems, menuItem)
		}
	} else {
		// Show disabled item if no proxies available
		disabledItem := fyne.NewMenuItem("No proxies available", nil)
		disabledItem.Disabled = true
		proxyMenuItems = append(proxyMenuItems, disabledItem)
	}

	// Create proxy submenu
	proxySubmenu := fyne.NewMenu("Select Proxy", proxyMenuItems...)

	// Get button state from centralized function
	buttonState := ac.GetVPNButtonState()

	// Create main menu items
	menuItems := []*fyne.MenuItem{
		fyne.NewMenuItem("Open", func() {
			if ac.UIService != nil {
				platform.RestoreDockIcon()
				ac.UIService.ShowMainWindowOrFocusWizard()
			}
		}),
		fyne.NewMenuItemSeparator(),
	}

	// Add Start/Stop VPN buttons based on centralized state
	if buttonState.StartEnabled {
		menuItems = append(menuItems, fyne.NewMenuItem("Start VPN", func() { StartSingBoxProcess(ac) }))
	} else {
		startItem := fyne.NewMenuItem("Start VPN", nil)
		startItem.Disabled = true
		menuItems = append(menuItems, startItem)
	}

	if buttonState.StopEnabled {
		menuItems = append(menuItems, fyne.NewMenuItem("Stop VPN", func() { StopSingBoxProcess(ac) }))
	} else {
		stopItem := fyne.NewMenuItem("Stop VPN", nil)
		stopItem.Disabled = true
		menuItems = append(menuItems, stopItem)
	}

	menuItems = append(menuItems, fyne.NewMenuItemSeparator())

	// Add proxy submenu if Clash API is enabled
	if clashAPIEnabled && selectedGroup != "" {
		selectProxyItem := fyne.NewMenuItem("Select Proxy", nil)
		selectProxyItem.ChildMenu = proxySubmenu
		menuItems = append(menuItems, selectProxyItem)
		menuItems = append(menuItems, fyne.NewMenuItemSeparator())
	}

	// Add "Hide app from Dock" toggle (macOS only) before Quit
	if runtime.GOOS == "darwin" {
		menuItems = ac.addHideDockMenuItem(menuItems)
	}

	// Add Quit item
	menuItems = append(menuItems, fyne.NewMenuItem("Quit", ac.GracefulExit))

	return fyne.NewMenu("Singbox Launcher", menuItems...)
}

// startAutoUpdateLoop runs a background goroutine that periodically checks and updates configuration
// Uses dynamic interval: max(10 minutes, parser.reload from config)
// Handles errors with retries (10 attempts, 10 seconds between retries)
// Resumes after successful manual update
func (ac *AppController) startAutoUpdateLoop() {
	log.Println("Auto-update: Starting auto-update loop")

	for {
		// Check if context is cancelled
		select {
		case <-ac.ctx.Done():
			log.Println("Auto-update: Context cancelled, stopping loop")
			return
		default:
		}

		// Check if auto-update is enabled
		if !ac.StateService.IsAutoUpdateEnabled() {
			// Auto-update is stopped, wait and check again
			select {
			case <-ac.ctx.Done():
				return
			case <-time.After(1 * time.Minute):
				continue
			}
		}

		// Calculate check interval from config
		checkInterval, err := ac.calculateAutoUpdateInterval()
		if err != nil {
			log.Printf("Auto-update: Failed to calculate interval: %v, using default", err)
			checkInterval = autoUpdateMinInterval
		}

		log.Printf("Auto-update: Calculated interval: %v (min: %v)", checkInterval, autoUpdateMinInterval)

		// Check if update is needed immediately (before waiting)
		requiredInterval, err := ac.calculateAutoUpdateInterval()
		if err != nil {
			log.Printf("Auto-update: Failed to calculate required interval: %v, using default", err)
			requiredInterval = autoUpdateMinInterval
		}

		needsUpdate, err := ac.shouldAutoUpdate(requiredInterval)
		if err != nil {
			log.Printf("Auto-update: Failed to check if update needed: %v, skipping this check", err)
			// Don't stop auto-update on check errors, just skip this check and wait
		} else if needsUpdate {
			// Update is needed - check if already in progress
			ac.ParserMutex.Lock()
			updateInProgress := ac.ParserRunning
			ac.ParserMutex.Unlock()

			if !updateInProgress {
				log.Println("Auto-update: Update needed, attempting update...")
				success := ac.attemptAutoUpdateWithRetries(autoUpdateRetryInterval, autoUpdateMaxRetries)
				if success {
					// Success - error counter already reset in attemptAutoUpdateWithRetries
					ac.StateService.ResumeAutoUpdate()
					log.Println("Auto-update: Resumed after successful update")
					log.Println("Auto-update: Completed successfully, error counter reset")
				} else {
					// Failed after all retries - check if we reached max consecutive failures
					failedAttempts := ac.StateService.GetAutoUpdateFailedAttempts()
					if failedAttempts >= autoUpdateMaxRetries {
						ac.StateService.SetAutoUpdateEnabled(false)
						log.Printf("Auto-update: Stopped after %d consecutive failed attempts", failedAttempts)
						fyne.Do(func() {
							if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
								dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Auto-update", "Automatic configuration update stopped after 10 failed attempts. Use manual update.")
							}
						})
					}
				}
			} else {
				log.Println("Auto-update: Update already in progress, skipping")
			}
		} else {
			log.Printf("Auto-update: Update not needed yet, will check again in %v", checkInterval)
		}

		// Wait for check interval before next check
		select {
		case <-ac.ctx.Done():
			return
		case <-time.After(checkInterval):
			// Time for next check
		}
	}
}

// calculateAutoUpdateInterval calculates the check interval: max(10 minutes, parser.reload)
// Returns the interval to use for checking if update is needed
func (ac *AppController) calculateAutoUpdateInterval() (time.Duration, error) {
	// Read ParserConfig from file
	config, err := parser.ExtractParserConfig(ac.FileService.ConfigPath)
	if err != nil {
		// If config doesn't exist or can't be read, use default
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	// Get reload value from config
	reloadStr := config.ParserConfig.Parser.Reload
	if reloadStr == "" {
		// Use default if not specified
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	// Parse reload string to duration
	reloadDuration, err := time.ParseDuration(reloadStr)
	if err != nil {
		log.Printf("Auto-update: Failed to parse reload duration '%s': %v, using default", reloadStr, err)
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	// Return max(10 minutes, reload)
	return maxDuration(autoUpdateMinInterval, reloadDuration), nil
}

// maxDuration returns the maximum of two durations
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// shouldAutoUpdate checks if configuration update is needed
// Returns true if elapsed time since last_updated >= required interval
func (ac *AppController) shouldAutoUpdate(requiredInterval time.Duration) (bool, error) {
	// Read ParserConfig from file
	config, err := parser.ExtractParserConfig(ac.FileService.ConfigPath)
	if err != nil {
		// If config doesn't exist, update is needed
		return true, nil
	}

	// Check last_updated
	lastUpdatedStr := config.ParserConfig.Parser.LastUpdated
	if lastUpdatedStr == "" {
		// No last_updated - update is needed
		return true, nil
	}

	// Parse last_updated timestamp
	lastUpdated, err := time.Parse(time.RFC3339, lastUpdatedStr)
	if err != nil {
		log.Printf("Auto-update: Failed to parse last_updated '%s': %v", lastUpdatedStr, err)
		// If parsing fails, assume update is needed
		return true, nil
	}

	// Calculate elapsed time
	elapsed := time.Since(lastUpdated.UTC())
	log.Printf("Auto-update: Checking if update needed (last_updated: %s, elapsed: %v, required: %v)", lastUpdatedStr, elapsed, requiredInterval)

	// Check if elapsed >= required interval
	return elapsed >= requiredInterval, nil
}

// attemptAutoUpdateWithRetries attempts to update configuration with retries
// Returns true if update succeeded, false if all retries failed
func (ac *AppController) attemptAutoUpdateWithRetries(retryInterval time.Duration, maxRetries int) bool {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Auto-update: Attempting update (attempt %d/%d)", attempt, maxRetries)

		// Call UpdateConfigFromSubscriptions synchronously
		err := ac.ConfigService.UpdateConfigFromSubscriptions()
		if err == nil {
			// Success - reset error counter
			ac.StateService.ResetAutoUpdateFailedAttempts()
			return true
		}

		// Error occurred - increment error counter
		ac.StateService.IncrementAutoUpdateFailedAttempts()
		currentAttempts := ac.StateService.GetAutoUpdateFailedAttempts()

		log.Printf("Auto-update: Failed (attempt %d/%d, total consecutive failures: %d): %v", attempt, maxRetries, currentAttempts, err)

		if attempt < maxRetries {
			// Wait before retry (except for last attempt)
			log.Printf("Auto-update: Retrying in %v...", retryInterval)
			select {
			case <-ac.ctx.Done():
				return false
			case <-time.After(retryInterval):
				// Continue to next attempt
			}
		}
	}

	// All retries failed
	return false
}

// resumeAutoUpdate resumes automatic updates after successful manual update
// Should be called after successful UpdateConfigFromSubscriptions
func (ac *AppController) resumeAutoUpdate() {
	if ac.StateService != nil {
		ac.StateService.ResumeAutoUpdate()
		log.Println("Auto-update: Resumed after successful manual update")
	}
}
