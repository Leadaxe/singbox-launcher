package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"

	"fyne.io/fyne/v2"

	"singbox-launcher/api"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/process"
)

// Constants for log file names
const (
	logFileName      = "logs/" + constants.MainLogFileName
	childLogFileName = "logs/" + constants.ChildLogFileName
	apiLogFileName   = "logs/" + constants.APILogFileName
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
	SingboxPrivilegedMode    bool   // true when sing-box was started with RunWithPrivileges (macOS TUN)
	SingboxPrivilegedPID     int    // PID of the privileged sing-box process
	SingboxPrivilegedPIDFile string // temp file path holding the PID (for privileged kill)
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

	// --- Update popup state ---
	updatePopupShown bool         // Флаг, что попап обновления уже был показан в этой сессии
	updatePopupMutex sync.RWMutex // Мьютекс для защиты updatePopupShown
}

// RunningState - structure for tracking the VPN's running state.
type RunningState struct {
	running bool
	sync.RWMutex
	controller *AppController
}

var (
	instance     *AppController
	instanceOnce sync.Once
)

// GetController returns the global AppController instance (singleton).
// Returns nil if NewAppController has not been called yet.
// In normal operation, NewAppController should be called in main.go before any calls to GetController().
func GetController() *AppController {
	if instance == nil {
		debuglog.WarnLog("GetController: instance is nil, this should not happen. NewAppController should be called first.")
		// Try to create a minimal instance (this is a fallback, not recommended)
		// In practice, this should never happen in normal operation
		instanceOnce.Do(func() {
			// Create minimal instance without UI dependencies
			// This is a fallback and may not work correctly for all use cases
			fileService, err := services.NewFileService()
			if err != nil {
				debuglog.ErrorLog("GetController: failed to create fallback FileService: %v", err)
				return
			}
			instance = &AppController{
				FileService: fileService,
			}
			instance.RunningState = &RunningState{controller: instance}
			instance.ProcessService = NewProcessService(instance)
			instance.ConfigService = NewConfigService(instance)
			instance.ctx, instance.cancelFunc = context.WithCancel(context.Background())
			instance.StateService = services.NewStateService()
		})
	}
	return instance
}

// SetUpdatePopupShown устанавливает флаг, что попап обновления был показан
func (ac *AppController) SetUpdatePopupShown(shown bool) {
	ac.updatePopupMutex.Lock()
	defer ac.updatePopupMutex.Unlock()
	ac.updatePopupShown = shown
}

// isUpdatePopupShown возвращает, был ли попап обновления показан
func (ac *AppController) isUpdatePopupShown() bool {
	ac.updatePopupMutex.RLock()
	defer ac.updatePopupMutex.RUnlock()
	return ac.updatePopupShown
}

// NewAppController creates and initializes a new AppController instance.
// This function should be called only once at application startup (typically in main.go).
// It sets the global singleton instance that can be accessed via GetController().
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
	ac.ConsecutiveCrashAttempts = 0
	ac.ProcessService = NewProcessService(ac)
	ac.ConfigService = NewConfigService(ac)

	// Устанавливаем callback для проверки обновлений при открытии окна
	ac.UIService.OnWindowShown = func() {
		ac.ShowUpdatePopupIfAvailable()
	}

	// Initialize APIService
	apiService, err := services.NewAPIService(
		ac.FileService.ConfigPath,
		ac.FileService.ApiLogFile,
		func() bool { return ac.RunningState.IsRunning() },
		func() {
			// OnProxiesUpdated callback
			if ac.hasUI() {
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
			if ac.hasUI() {
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
	ac.UIService.RefreshAPIFunc = func() { debuglog.DebugLog("RefreshAPIFunc handler is not set yet.") }
	ac.UIService.ResetAPIStateFunc = func() { debuglog.DebugLog("ResetAPIStateFunc handler is not set yet.") }
	ac.UIService.UpdateCoreStatusFunc = func() { debuglog.DebugLog("UpdateCoreStatusFunc handler is not set yet.") }
	ac.UIService.UpdateConfigStatusFunc = func() { debuglog.DebugLog("UpdateConfigStatusFunc handler is not set yet.") }
	ac.UIService.UpdateTrayMenuFunc = func() { debuglog.DebugLog("UpdateTrayMenuFunc handler is not set yet.") }
	ac.UIService.UpdateParserProgressFunc = func(progress float64, status string) {
		debuglog.DebugLog("UpdateParserProgressFunc handler is not set yet. Progress: %.0f%%, Status: %s", progress, status)
	}

	// Initialize context for goroutine cancellation
	ac.ctx, ac.cancelFunc = context.WithCancel(context.Background())

	// Initialize StateService
	ac.StateService = services.NewStateService()

	// Check if config file exists before starting auto-update
	if _, err := os.Stat(ac.FileService.ConfigPath); os.IsNotExist(err) {
		debuglog.InfoLog("Auto-update: Config file does not exist (%s), auto-update disabled", ac.FileService.ConfigPath)
		ac.StateService.SetAutoUpdateEnabled(false)
	}
	go ac.startAutoUpdateLoop()

	// Set global singleton instance
	instanceOnce.Do(func() {
		instance = ac
	})

	return ac, nil
}

// UpdateUI updates all UI elements based on the current application state.
func (ac *AppController) UpdateUI() {
	if ac.hasUI() {
		ac.UIService.UpdateUI()
	}
}

// GetApplication returns the Fyne application instance.
func (ac *AppController) GetApplication() fyne.App {
	if ac.hasUI() {
		return ac.UIService.Application
	}
	return nil
}

// GetMainWindow returns the main window instance.
func (ac *AppController) GetMainWindow() fyne.Window {
	if ac.hasUI() {
		return ac.UIService.MainWindow
	}
	return nil
}

// hasUI проверяет, доступен ли UI для обновлений (MainWindow)
func (ac *AppController) hasUI() bool {
	return ac.UIService != nil && ac.UIService.MainWindow != nil
}

// hasUIWithApp проверяет, доступен ли UI с Application (для ShowAutoHideInfo)
func (ac *AppController) hasUIWithApp() bool {
	return ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil
}

// GracefulExit performs a graceful shutdown of the application.
func (ac *AppController) GracefulExit() {
	// Cancel context to signal all goroutines to stop
	if ac.cancelFunc != nil {
		ac.cancelFunc()
		debuglog.InfoLog("GracefulExit: Context cancelled, signalling goroutines to stop")
	}

	// Stop any pending menu update timer
	if ac.hasUI() {
		ac.UIService.StopTrayMenuUpdateTimer()
	}

	StopSingBoxProcess()

	debuglog.InfoLog("GracefulExit: Waiting for sing-box to stop...")
	// Use ProcessService constant for timeout
	timeout := time.After(2 * time.Second) // gracefulShutdownTimeout from ProcessService
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !ac.RunningState.IsRunning() {
			debuglog.InfoLog("GracefulExit: Sing-box confirmed stopped.")
			break
		}
		select {
		case <-timeout:
			debuglog.WarnLog("GracefulExit: Timeout waiting for sing-box to stop. Forcing kill.")
			ac.CmdMutex.Lock()
			if ac.SingboxCmd != nil && ac.SingboxCmd.Process != nil {
				_ = ac.SingboxCmd.Process.Kill()
			}
			ac.CmdMutex.Unlock()
			goto end_loop
		case <-ticker.C:
			// Check state on each tick - continue loop to re-check IsRunning()
		}
	}
end_loop:

	if ac.FileService != nil {
		ac.FileService.CloseLogFiles()
	}

	if ac.hasUI() {
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
func CheckLinuxCapabilities() {
	ac := GetController()
	if ac == nil {
		return
	}
	if suggestion := platform.CheckAndSuggestCapabilities(ac.FileService.SingboxPath); suggestion != "" {
		debuglog.InfoLog("CheckLinuxCapabilities: %s", suggestion)
		// Show info dialog (not error) - capabilities can be set later
		if ac.hasUI() {
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
// Uses RLock to allow concurrent reads without blocking each other.
func (r *RunningState) IsRunning() bool {
	r.RLock()
	defer r.RUnlock()
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

// StartSingBoxProcess launches the sing-box process.
// skipRunningCheck: если true, пропускает проверку на уже запущенный процесс (для автоперезапуска)
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func StartSingBoxProcess(skipRunningCheck ...bool) {
	ac := GetController()
	if ac == nil {
		return
	}
	if ac.ProcessService == nil {
		debuglog.WarnLog("StartSingBoxProcess: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.Start(skipRunningCheck...)
}

// StopSingBoxProcess is the unified function to stop the sing-box process.
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func StopSingBoxProcess() {
	ac := GetController()
	if ac == nil {
		return
	}
	if ac.ProcessService == nil {
		debuglog.WarnLog("StopSingBoxProcess: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.Stop()
}

// RunParserProcess starts the internal configuration update process.
// Note: ConfigService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func RunParserProcess() {
	ac := GetController()
	if ac == nil {
		return
	}
	if ac.ConfigService == nil {
		debuglog.WarnLog("RunParserProcess: ConfigService is nil, this should not happen. Initializing...")
		ac.ConfigService = NewConfigService(ac)
	}
	ac.ConfigService.RunParserProcess()
}

// CheckIfSingBoxRunningAtStartUtil checks if sing-box is already running at application start.
// Note: ProcessService must be initialized in NewAppController. This is a wrapper for backward compatibility.
func CheckIfSingBoxRunningAtStartUtil() {
	ac := GetController()
	if ac == nil {
		return
	}
	if ac.ProcessService == nil {
		debuglog.WarnLog("CheckIfSingBoxRunningAtStartUtil: ProcessService is nil, this should not happen. Initializing...")
		ac.ProcessService = NewProcessService(ac)
	}
	ac.ProcessService.CheckIfRunningAtStart()
}

// CheckConfigFileExists checks if config.json exists and shows a warning if it doesn't
func CheckConfigFileExists() {
	ac := GetController()
	if ac == nil {
		return
	}
	if _, err := os.Stat(ac.FileService.ConfigPath); os.IsNotExist(err) {
		debuglog.WarnLog("CheckConfigFileExists: config.json not found at %s", ac.FileService.ConfigPath)

		message := fmt.Sprintf(
			"⚠️ Configuration file not found!\n\n"+
				"The file %s is missing from the bin/ folder.\n\n"+
				"To get started:\n"+
				"1. download Wizard\n"+
				"2. use Wizard to generate a configuration file\n"+
				"3. press Start\n",
			constants.ConfigFileName,
		)

		if ac.hasUI() {
			dialogs.ShowInfo(ac.UIService.MainWindow, "Configuration Not Found", message)
		}
	}
}

func CheckIfLauncherAlreadyRunningUtil() {
	ac := GetController()
	if ac == nil {
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		debuglog.ErrorLog("CheckIfLauncherAlreadyRunning: cannot detect executable path: %v", err)
		return
	}
	execName := strings.ToLower(filepath.Base(execPath))
	currentPID := os.Getpid()

	processes, err := process.GetProcesses()
	if err != nil {
		debuglog.ErrorLog("CheckIfLauncherAlreadyRunning: error listing processes: %v", err)
		return
	}

	for _, p := range processes {
		if p.PID == currentPID {
			continue
		}
		if strings.EqualFold(p.Name, execName) {
			if ac.hasUI() {
				dialogs.ShowInfo(ac.UIService.MainWindow, "Information", "The application is already running. Use the existing instance or close it before starting a new one.")
			}
			return
		}
	}
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
