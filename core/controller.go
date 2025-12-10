package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"

	ps "github.com/mitchellh/go-ps"
	"github.com/muhammadmuzzammil1998/jsonc"
)

// Constants for log file names
const (
	logFileName       = "logs/" + constants.MainLogFileName
	childLogFileName  = "logs/" + constants.ChildLogFileName
	parserLogFileName = "logs/" + constants.ParserLogFileName
	apiLogFileName    = "logs/" + constants.APILogFileName
	restartDelay      = 2 * time.Second
)

// AppController - the main structure encapsulating all application state and logic.
// AppController is the central controller coordinating all application components.
// It manages UI state, process lifecycle, configuration, API interactions, and logging.
// The controller delegates specific responsibilities to specialized services:
// - ProcessService: sing-box process management
// - ConfigService: configuration parsing and updates
// The controller maintains application-wide state and provides callbacks for UI updates.
type AppController struct {
	// --- Fyne Components ---
	Application    fyne.App
	MainWindow     fyne.Window
	TrayIcon       fyne.Resource
	ApiStatusLabel *widget.Label

	// --- UI State Fields ---
	ProxiesListWidget *widget.List
	ActiveProxyName   string
	SelectedIndex     int
	ProxiesList       []api.ProxyInfo
	ListStatusLabel   *widget.Label

	// --- Icon Resources ---
	AppIconData   fyne.Resource
	GreenIconData fyne.Resource
	GreyIconData  fyne.Resource
	RedIconData   fyne.Resource // Icon for error state

	// --- Process State ---
	SingboxCmd               *exec.Cmd
	CmdMutex                 sync.Mutex
	ParserMutex              sync.Mutex // Mutex for ParserRunning
	ParserRunning            bool
	StoppedByUser            bool
	ConsecutiveCrashAttempts int
	APIStateMutex            sync.RWMutex // Mutex for API-related fields (ProxiesList, ActiveProxyName, SelectedIndex)

	// --- File Paths ---
	ExecDir     string
	ConfigPath  string
	SingboxPath string
	WintunPath  string

	// --- VPN Operation State ---
	RunningState *RunningState

	// --- Services ---
	// ProcessService manages sing-box process lifecycle (start, stop, monitor, auto-restart)
	ProcessService *ProcessService
	// ConfigService handles configuration parsing, subscription fetching, and JSON generation
	ConfigService *ConfigService

	// --- Logging ---
	MainLogFile  *os.File
	ChildLogFile *os.File
	ApiLogFile   *os.File

	// --- Clash API configuration ---
	ClashAPIBaseURL    string
	ClashAPIToken      string
	ClashAPIEnabled    bool
	SelectedClashGroup string
	AutoLoadInProgress bool       // Flag to prevent multiple auto-load attempts
	AutoLoadMutex      sync.Mutex // Mutex for AutoLoadInProgress

	// --- Tray menu update protection ---
	TrayMenuUpdateInProgress bool        // Flag to prevent multiple simultaneous menu updates
	TrayMenuUpdateMutex      sync.Mutex  // Mutex for TrayMenuUpdateInProgress
	TrayMenuUpdateTimer      *time.Timer // Timer for debouncing menu updates

	// --- Version check caching ---
	VersionCheckCache      string       // Cached latest version
	VersionCheckCacheTime  time.Time    // Time when version was successfully checked
	VersionCheckMutex      sync.RWMutex // Mutex for version check cache
	VersionCheckInProgress bool         // Flag to prevent multiple version checks

	// --- Context for goroutine cancellation ---
	ctx        context.Context    // Context for cancellation
	cancelFunc context.CancelFunc // Cancel function for stopping goroutines

	// --- Callbacks for UI logic ---
	RefreshAPIFunc         func()
	ResetAPIStateFunc      func()
	UpdateCoreStatusFunc   func() // Callback to update status in Core Dashboard
	UpdateConfigStatusFunc func() // Callback to update config status in Core Dashboard
	UpdateTrayMenuFunc     func() // Callback to update tray menu

	// --- Parser progress UI ---
	ParserProgressBar        *widget.ProgressBar
	ParserStatusLabel        *widget.Label
	UpdateParserProgressFunc func(progress float64, status string) // Callback to update parser progress
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

	ex, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("NewAppController: cannot determine executable path: %w", err)
	}
	ac.ExecDir = filepath.Dir(ex)

	// Use platform-specific functions
	if err := platform.EnsureDirectories(ac.ExecDir); err != nil {
		return nil, fmt.Errorf("NewAppController: cannot create directories: %w", err)
	}

	ac.ConfigPath = platform.GetConfigPath(ac.ExecDir)
	singboxName := platform.GetExecutableNames()
	ac.SingboxPath = filepath.Join(ac.ExecDir, "bin", singboxName)
	ac.WintunPath = platform.GetWintunPath(ac.ExecDir)

	// Open log files with rotation support
	logFile, err := openLogFileWithRotation(filepath.Join(ac.ExecDir, logFileName))
	if err != nil {
		return nil, fmt.Errorf("NewAppController: cannot open main log file: %w", err)
	}
	log.SetOutput(logFile)
	ac.MainLogFile = logFile

	childLogFile, err := openLogFileWithRotation(filepath.Join(ac.ExecDir, childLogFileName))
	if err != nil {
		log.Printf("NewAppController: failed to open sing-box child log file: %v", err)
		ac.ChildLogFile = nil
	} else {
		ac.ChildLogFile = childLogFile
	}

	apiLogFile, err := openLogFileWithRotation(filepath.Join(ac.ExecDir, apiLogFileName))
	if err != nil {
		log.Printf("NewAppController: failed to open API log file: %v", err)
		ac.ApiLogFile = nil
	} else {
		ac.ApiLogFile = apiLogFile
	}

	ac.AppIconData = fyne.NewStaticResource("appIcon", appIconData)
	ac.GreyIconData = fyne.NewStaticResource("trayIcon", greyIconData)
	ac.GreenIconData = fyne.NewStaticResource("runningIcon", greenIconData)
	ac.RedIconData = fyne.NewStaticResource("errorIcon", redIconData)

	log.Println("Application initializing...")
	ac.Application = app.NewWithID("com.singbox.launcher")
	ac.Application.SetIcon(ac.AppIconData)
	ac.RunningState = &RunningState{controller: ac}
	ac.RunningState.Set(false) // Use Set() method instead of direct assignment
	ac.ConsecutiveCrashAttempts = 0
	ac.ProcessService = NewProcessService(ac)
	ac.ConfigService = NewConfigService(ac)

	if base, tok, err := api.LoadClashAPIConfig(ac.ConfigPath); err != nil {
		log.Printf("NewAppController: Clash API config error: %v", err)
		ac.ClashAPIBaseURL = ""
		ac.ClashAPIToken = ""
		ac.ClashAPIEnabled = false
	} else {
		ac.ClashAPIBaseURL = base
		ac.ClashAPIToken = tok
		ac.ClashAPIEnabled = true
	}

	// Initialize SelectedClashGroup from config (needed for auto-loading proxies)
	if ac.ClashAPIEnabled {
		_, defaultSelector, err := GetSelectorGroupsFromConfig(ac.ConfigPath)
		if err != nil {
			log.Printf("NewAppController: Failed to get selector groups: %v", err)
			ac.SelectedClashGroup = "proxy-out" // Default fallback
		} else {
			ac.SelectedClashGroup = defaultSelector
			log.Printf("NewAppController: Initialized SelectedClashGroup: %s", defaultSelector)
		}
	}

	// Initialize API state fields (safe during initialization, but using methods for consistency)
	ac.SetProxiesList([]api.ProxyInfo{})
	ac.SetSelectedIndex(-1)
	ac.SetActiveProxyName("")

	ac.RefreshAPIFunc = func() { log.Println("RefreshAPIFunc handler is not set yet.") }
	ac.ResetAPIStateFunc = func() { log.Println("ResetAPIStateFunc handler is not set yet.") }
	ac.UpdateCoreStatusFunc = func() { log.Println("UpdateCoreStatusFunc handler is not set yet.") }
	ac.UpdateConfigStatusFunc = func() { log.Println("UpdateConfigStatusFunc handler is not set yet.") }
	ac.UpdateTrayMenuFunc = func() { log.Println("UpdateTrayMenuFunc handler is not set yet.") }
	ac.UpdateParserProgressFunc = func(progress float64, status string) {
		log.Printf("UpdateParserProgressFunc handler is not set yet. Progress: %.0f%%, Status: %s", progress, status)
	}

	// Initialize context for goroutine cancellation
	ac.ctx, ac.cancelFunc = context.WithCancel(context.Background())

	return ac, nil
}

// UpdateUI updates all UI elements based on the current application state.
func (ac *AppController) UpdateUI() {
	fyne.Do(func() {
		// Update tray icon (this is a system function, not a UI widget)
		if desk, ok := ac.Application.(desktop.App); ok {
			// Check that icons are initialized
			if ac.GreenIconData == nil || ac.GreyIconData == nil || ac.RedIconData == nil {
				log.Printf("UpdateUI: Icons not initialized, skipping icon update")
				return
			}

			var iconToSet fyne.Resource

			if ac.RunningState.IsRunning() {
				// Green icon - if running
				iconToSet = ac.GreenIconData
			} else {
				// Check for binary to determine error state (simple file check)
				if _, err := os.Stat(ac.SingboxPath); os.IsNotExist(err) {
					// Red icon - on error (binary not found)
					iconToSet = ac.RedIconData
				} else {
					// Grey icon - on normal stop
					iconToSet = ac.GreyIconData
				}
			}

			desk.SetSystemTrayIcon(iconToSet)
		}

		// Если состояние Down, сбрасываем API состояние
		if !ac.RunningState.IsRunning() && ac.ResetAPIStateFunc != nil {
			log.Println("UpdateUI: Triggering API state reset because state is 'Down'.")
			ac.ResetAPIStateFunc()
		}

		// Update tray menu when state changes (same as Core Dashboard)
		if ac.UpdateTrayMenuFunc != nil {
			ac.UpdateTrayMenuFunc()
		}

		// Update Core Dashboard status when state changes (synchronize with tray)
		if ac.UpdateCoreStatusFunc != nil {
			ac.UpdateCoreStatusFunc()
		}
	})
}

// GracefulExit performs a graceful shutdown of the application.
func (ac *AppController) GracefulExit() {
	// Stop any pending menu update timer
	ac.TrayMenuUpdateMutex.Lock()
	if ac.TrayMenuUpdateTimer != nil {
		ac.TrayMenuUpdateTimer.Stop()
		ac.TrayMenuUpdateTimer = nil
	}
	ac.TrayMenuUpdateMutex.Unlock()

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

	if ac.MainLogFile != nil {
		ac.MainLogFile.Close()
	}
	if ac.ChildLogFile != nil {
		ac.ChildLogFile.Close()
	}
	if ac.ApiLogFile != nil {
		ac.ApiLogFile.Close()
	}

	ac.Application.Quit()
}

// RunHidden launches an external command in a hidden window.
func (ac *AppController) RunHidden(name string, args []string, logPath string, dir string) error {
	cmd := exec.Command(name, args...)
	platform.PrepareCommand(cmd)
	if dir != "" {
		cmd.Dir = dir
	}

	if logPath != "" {
		if logPath == filepath.Join(ac.ExecDir, childLogFileName) && ac.ChildLogFile != nil {
			// For sing-box logs, check and rotate if needed before writing
			checkAndRotateLogFile(logPath)
			logFile := ac.ChildLogFile
			// Don't truncate - append to preserve logs, rotation handles size limits
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		} else {
			// For other logs (parser), use truncate mode for clean start
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("RunHidden: cannot open log file '%s': %w", logPath, err)
			}
			defer logFile.Close()
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
	}

	return cmd.Run()
}

// CheckLinuxCapabilities checks Linux capabilities and shows a suggestion if needed
func CheckLinuxCapabilities(ac *AppController) {
	if suggestion := platform.CheckAndSuggestCapabilities(ac.SingboxPath); suggestion != "" {
		log.Printf("CheckLinuxCapabilities: %s", suggestion)
		// Show info dialog (not error) - capabilities can be set later
		dialogs.ShowInfo(ac.MainWindow, "Linux Capabilities", suggestion)
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
	if r.controller.UpdateCoreStatusFunc != nil {
		r.controller.UpdateCoreStatusFunc()
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
	ac.APIStateMutex.Lock()
	defer ac.APIStateMutex.Unlock()
	ac.ProxiesList = proxies
}

// GetProxiesList safely gets a copy of the proxies list with mutex protection.
func (ac *AppController) GetProxiesList() []api.ProxyInfo {
	ac.APIStateMutex.RLock()
	defer ac.APIStateMutex.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]api.ProxyInfo, len(ac.ProxiesList))
	copy(result, ac.ProxiesList)
	return result
}

// SetActiveProxyName safely sets the active proxy name with mutex protection.
func (ac *AppController) SetActiveProxyName(name string) {
	ac.APIStateMutex.Lock()
	defer ac.APIStateMutex.Unlock()
	ac.ActiveProxyName = name
}

// GetActiveProxyName safely gets the active proxy name with mutex protection.
func (ac *AppController) GetActiveProxyName() string {
	ac.APIStateMutex.RLock()
	defer ac.APIStateMutex.RUnlock()
	return ac.ActiveProxyName
}

// SetSelectedIndex safely sets the selected index with mutex protection.
func (ac *AppController) SetSelectedIndex(index int) {
	ac.APIStateMutex.Lock()
	defer ac.APIStateMutex.Unlock()
	ac.SelectedIndex = index
}

// GetSelectedIndex safely gets the selected index with mutex protection.
func (ac *AppController) GetSelectedIndex() int {
	ac.APIStateMutex.RLock()
	defer ac.APIStateMutex.RUnlock()
	return ac.SelectedIndex
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

// isSingBoxProcessRunning checks if a sing-box process is currently running on the system.
// Uses tasklist command on Windows for more reliable process detection.
// Returns true if process found, and the PID of found process (or -1 if not found).
func isSingBoxProcessRunning(ac *AppController) (bool, int) {
	processName := platform.GetProcessNameForCheck()
	log.Printf("isSingBoxProcessRunning: Looking for process name '%s'", processName)

	ourPID := getOurPID(ac)
	log.Printf("isSingBoxProcessRunning: Our tracked PID=%d", ourPID)

	// On Windows use tasklist for more reliable process detection
	if runtime.GOOS == "windows" {
		// Use tasklist /FI "IMAGENAME eq sing-box.exe" /FO CSV /NH
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", processName), "/FO", "CSV", "/NH")
		platform.PrepareCommand(cmd) // Hide console window
		output, err := cmd.Output()
		if err != nil {
			log.Printf("isSingBoxProcessRunning: tasklist command failed: %v, falling back to ps library", err)
			return isSingBoxProcessRunningWithPS(ac, ourPID)
		}

		// Parse CSV output from tasklist
		// Format: "name.exe","PID","Session Name","Session#","Mem Usage"
		outputStr := strings.TrimSpace(string(output))
		if outputStr == "" {
			log.Printf("isSingBoxProcessRunning: No sing-box process found via tasklist")
			return false, -1
		}

		// Parse CSV lines
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Parse CSV: "name.exe","PID","..."
			parts := parseCSVLine(line)
			if len(parts) >= 2 {
				name := strings.Trim(parts[0], "\"")
				pidStr := strings.Trim(parts[1], "\"")
				if strings.EqualFold(name, processName) {
					if pid, err := strconv.Atoi(pidStr); err == nil {
						isOurProcess := (ourPID != -1 && pid == ourPID)
						log.Printf("isSingBoxProcessRunning: Found process via tasklist: PID=%d, name='%s' (our tracked PID=%d, isOurProcess=%v)", pid, name, ourPID, isOurProcess)
						return true, pid
					} else {
						log.Printf("isSingBoxProcessRunning: Failed to parse PID '%s': %v", pidStr, err)
					}
				}
			}
		}
		log.Printf("isSingBoxProcessRunning: tasklist found processes but none matched '%s'", processName)
		return false, -1
	}

	// For other OS use ps library
	return isSingBoxProcessRunningWithPS(ac, ourPID)
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

// isSingBoxProcessRunningWithPS uses ps library to check for running process
func isSingBoxProcessRunningWithPS(ac *AppController, ourPID int) (bool, int) {
	processes, err := ps.Processes()
	if err != nil {
		log.Printf("isSingBoxProcessRunningWithPS: error listing processes: %v", err)
		return false, -1
	}
	processName := platform.GetProcessNameForCheck()

	for _, p := range processes {
		execName := p.Executable()
		if strings.EqualFold(execName, processName) {
			foundPID := p.Pid()
			isOurProcess := (ourPID != -1 && foundPID == ourPID)
			log.Printf("isSingBoxProcessRunningWithPS: Found process: PID=%d, executable='%s' (our tracked PID=%d, isOurProcess=%v)", foundPID, execName, ourPID, isOurProcess)
			return true, foundPID
		}
	}
	log.Printf("isSingBoxProcessRunningWithPS: No sing-box process found (checked %d processes)", len(processes))
	return false, -1
}

// checkAndShowSingBoxRunningWarning checks if sing-box is running and shows warning dialog if found.
// Returns true if process was found and warning was shown, false otherwise.
func checkAndShowSingBoxRunningWarning(ac *AppController, context string) bool {
	found, foundPID := isSingBoxProcessRunning(ac)
	if found {
		log.Printf("%s: Found sing-box process already running (PID=%d). Showing warning dialog.", context, foundPID)
		ShowSingBoxAlreadyRunningWarningUtil(ac)
		return true
	}
	log.Printf("%s: No sing-box process found", context)
	return false
}

// getTunInterfaceName extracts TUN interface name from config.json
func getTunInterfaceName(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	// Parse JSONC (with comments) to clean JSON
	cleanData := jsonc.ToJSON(data)

	var config map[string]interface{}
	if err := json.Unmarshal(cleanData, &config); err != nil {
		return "", fmt.Errorf("failed to parse config: %w", err)
	}

	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return "", nil // No inbounds section, no TUN interface
	}

	for _, inbound := range inbounds {
		inboundMap, ok := inbound.(map[string]interface{})
		if !ok {
			continue
		}

		if inboundMap["type"] == "tun" {
			if interfaceName, ok := inboundMap["interface_name"].(string); ok && interfaceName != "" {
				return interfaceName, nil
			}
		}
	}

	return "", nil // No TUN interface found in config
}

// checkTunInterfaceExists checks if TUN interface exists on Windows
func checkTunInterfaceExists(interfaceName string) (bool, error) {
	if runtime.GOOS != "windows" {
		// On Linux/macOS, TUN interfaces are managed by the OS
		// and are automatically removed when the process exits
		return false, nil
	}

	cmd := exec.Command("netsh", "interface", "show", "interface", fmt.Sprintf("name=%s", interfaceName))
	platform.PrepareCommand(cmd) // Hide console window on Windows
	output, err := cmd.Output()

	if err != nil {
		// Interface not found or command failed
		return false, nil
	}

	// Check if interface name appears in output
	outputStr := strings.ToLower(string(output))
	return strings.Contains(outputStr, strings.ToLower(interfaceName)), nil
}

// removeTunInterface removes TUN interface on Windows before starting sing-box
func removeTunInterface(interfaceName string) error {
	if runtime.GOOS != "windows" {
		// On Linux/macOS, interface is removed automatically
		return nil
	}

	// Check if interface exists
	exists, err := checkTunInterfaceExists(interfaceName)
	if err != nil {
		log.Printf("removeTunInterface: Failed to check interface existence: %v", err)
		// Continue anyway - try to remove it
	}

	if !exists {
		log.Printf("removeTunInterface: Interface '%s' does not exist, nothing to remove", interfaceName)
		return nil
	}

	log.Printf("removeTunInterface: Removing existing TUN interface '%s'...", interfaceName)

	// Remove the interface using netsh
	cmd := exec.Command("netsh", "interface", "delete", "interface", fmt.Sprintf("name=%s", interfaceName))
	platform.PrepareCommand(cmd) // Hide console window on Windows

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Interface might be in use or already deleted
		log.Printf("removeTunInterface: Failed to remove interface '%s': %v, output: %s",
			interfaceName, err, string(output))
		// This is not a critical error - sing-box might handle it
		return nil
	}

	log.Printf("removeTunInterface: Successfully removed interface '%s'", interfaceName)

	// Give system time to release resources
	time.Sleep(500 * time.Millisecond)

	return nil
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
	if _, err := os.Stat(ac.ConfigPath); os.IsNotExist(err) {
		log.Printf("CheckConfigFileExists: config.json not found at %s", ac.ConfigPath)

		message := fmt.Sprintf(
			"⚠️ Configuration file not found!\n\n"+
				"The file %s is missing from the bin/ folder.\n\n"+
				"To get started:\n"+
				"1. download Wizard\n"+
				"2. use Wizard to generate a configuration file\n"+
				"3. press Start\n",
			constants.ConfigFileName,
		)

		dialogs.ShowInfo(ac.MainWindow, "Configuration Not Found", message)
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

	processes, err := ps.Processes()
	if err != nil {
		log.Printf("CheckIfLauncherAlreadyRunning: error listing processes: %v", err)
		return
	}

	for _, p := range processes {
		if p.Pid() == currentPID {
			continue
		}
		if strings.EqualFold(p.Executable(), execName) {
			dialogs.ShowInfo(ac.MainWindow, "Information", "The application is already running. Use the existing instance or close it before starting a new one.")
			return
		}
	}
}

func CheckFilesUtil(ac *AppController) {
	files := platform.GetRequiredFiles(ac.ExecDir)
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
	dialogs.ShowInfo(ac.MainWindow, "File Check", msg)
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
	d = dialog.NewCustomWithoutButtons("Warning", content, ac.MainWindow)
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

// AutoLoadProxies attempts to load proxies with retry intervals (1, 3, 7, 13, 17 seconds)
func (ac *AppController) AutoLoadProxies() {
	// Check if already in progress
	ac.AutoLoadMutex.Lock()
	if ac.AutoLoadInProgress {
		ac.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: Already in progress, skipping")
		return
	}
	ac.AutoLoadInProgress = true
	ac.AutoLoadMutex.Unlock()

	if !ac.ClashAPIEnabled {
		ac.AutoLoadMutex.Lock()
		ac.AutoLoadInProgress = false
		ac.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: Clash API is disabled, skipping")
		return
	}

	ac.APIStateMutex.RLock()
	selectedGroup := ac.SelectedClashGroup
	ac.APIStateMutex.RUnlock()

	if selectedGroup == "" {
		ac.AutoLoadMutex.Lock()
		ac.AutoLoadInProgress = false
		ac.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: No group selected, skipping")
		return
	}

	intervals := []time.Duration{1, 3, 3, 5, 5, 5, 5, 5, 10, 10, 10, 10, 15, 15}

	go func() {
		for attempt, interval := range intervals {
			// Check if context is cancelled
			select {
			case <-ac.ctx.Done():
				log.Println("AutoLoadProxies: Stopped (context cancelled)")
				ac.AutoLoadMutex.Lock()
				ac.AutoLoadInProgress = false
				ac.AutoLoadMutex.Unlock()
				return
			default:
			}

			// Wait for the interval (except first attempt)
			if attempt > 0 {
				select {
				case <-ac.ctx.Done():
					log.Println("AutoLoadProxies: Stopped during wait (context cancelled)")
					ac.AutoLoadMutex.Lock()
					ac.AutoLoadInProgress = false
					ac.AutoLoadMutex.Unlock()
					return
				case <-time.After(interval * time.Second):
					// Continue
				}
			}

			// Check if sing-box is running before attempting to connect
			if !ac.RunningState.IsRunning() {
				log.Printf("AutoLoadProxies: Attempt %d/%d skipped - sing-box is not running", attempt+1, len(intervals))
				// Continue to next attempt
				continue
			}

			log.Printf("AutoLoadProxies: Attempt %d/%d to load proxies for group '%s'", attempt+1, len(intervals), selectedGroup)

			// Get current group (it might have changed)
			ac.APIStateMutex.RLock()
			currentGroup := ac.SelectedClashGroup
			baseURL := ac.ClashAPIBaseURL
			token := ac.ClashAPIToken
			ac.APIStateMutex.RUnlock()

			if currentGroup == "" {
				log.Printf("AutoLoadProxies: Group cleared, stopping attempts")
				return
			}

			// Try to load proxies
			proxies, now, err := api.GetProxiesInGroup(baseURL, token, currentGroup, ac.ApiLogFile)
			if err != nil {
				log.Printf("AutoLoadProxies: Attempt %d failed: %v", attempt+1, err)
				// Continue to next attempt
				continue
			}

			// Success - update proxies list
			fyne.Do(func() {
				ac.SetProxiesList(proxies)
				ac.SetActiveProxyName(now)

				// Update UI if callbacks are set
				if ac.ProxiesListWidget != nil {
					ac.ProxiesListWidget.Refresh()
				}
				if ac.ListStatusLabel != nil {
					ac.ListStatusLabel.SetText(fmt.Sprintf("Proxies loaded for '%s'. Active: %s", currentGroup, now))
				}
				if ac.RefreshAPIFunc != nil {
					ac.RefreshAPIFunc()
				}

				// Update tray menu AFTER UI updates (important: this must be last)
				if ac.UpdateTrayMenuFunc != nil {
					ac.UpdateTrayMenuFunc()
				}
			})

			log.Printf("AutoLoadProxies: Successfully loaded %d proxies for group '%s' on attempt %d", len(proxies), currentGroup, attempt+1)

			ac.AutoLoadMutex.Lock()
			ac.AutoLoadInProgress = false
			ac.AutoLoadMutex.Unlock()
			return // Success, stop retrying
		}

		log.Printf("AutoLoadProxies: All %d attempts failed", len(intervals))
		ac.AutoLoadMutex.Lock()
		ac.AutoLoadInProgress = false
		ac.AutoLoadMutex.Unlock()
	}()
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
	if _, err := os.Stat(ac.ConfigPath); err == nil {
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

// CreateTrayMenu creates the system tray menu with proxy selection submenu
func (ac *AppController) CreateTrayMenu() *fyne.Menu {
	// Get proxies from current group
	ac.APIStateMutex.RLock()
	proxies := ac.ProxiesList
	activeProxy := ac.ActiveProxyName
	selectedGroup := ac.SelectedClashGroup
	clashAPIEnabled := ac.ClashAPIEnabled
	ac.APIStateMutex.RUnlock()

	// Auto-load proxies if list is empty and API is enabled
	// Note: AutoLoadProxies has internal guard to prevent multiple simultaneous loads
	if clashAPIEnabled && selectedGroup != "" && len(proxies) == 0 {
		// Only auto-load if sing-box is running
		if ac.RunningState.IsRunning() {
			// Check if auto-load is already in progress to avoid duplicate calls
			ac.AutoLoadMutex.Lock()
			alreadyInProgress := ac.AutoLoadInProgress
			ac.AutoLoadMutex.Unlock()

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
					err := api.SwitchProxy(ac.ClashAPIBaseURL, ac.ClashAPIToken, selectedGroup, pName, ac.ApiLogFile)
					fyne.Do(func() {
						if err != nil {
							log.Printf("CreateTrayMenu: Failed to switch proxy: %v", err)
							dialogs.ShowError(ac.MainWindow, fmt.Errorf("failed to switch proxy: %w", err))
						} else {
							ac.SetActiveProxyName(pName)
							// Update tray menu after switch
							if ac.UpdateTrayMenuFunc != nil {
								ac.UpdateTrayMenuFunc()
							}
							// Refresh UI if callback is set
							if ac.RefreshAPIFunc != nil {
								ac.RefreshAPIFunc()
							}
						}
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
		fyne.NewMenuItem("Open", func() { ac.MainWindow.Show() }),
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

	// Add Quit item
	menuItems = append(menuItems, fyne.NewMenuItem("Quit", ac.GracefulExit))

	return fyne.NewMenu("Singbox Launcher", menuItems...)
}
