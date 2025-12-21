package core

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"

	ps "github.com/mitchellh/go-ps"
)

const (
	// restartAttempts is the maximum number of consecutive crash restart attempts
	restartAttempts = 3

	// stabilityThreshold is the duration a process must run without crashing
	// before the crash counter is reset
	stabilityThreshold = 180 * time.Second

	// gracefulShutdownTimeout is the maximum time to wait for graceful shutdown
	// before forcing kill
	gracefulShutdownTimeout = 2 * time.Second
)

// ProcessService encapsulates sing-box process lifecycle management.
// It handles starting, stopping, monitoring, and auto-restarting the sing-box process.
// The service ensures proper cleanup of TUN interfaces, log rotation, and process state management.
type ProcessService struct {
	ac *AppController
}

// NewProcessService constructs a ProcessService bound to the controller.
func NewProcessService(ac *AppController) *ProcessService {
	return &ProcessService{ac: ac}
}

// Start launches the sing-box process. Behavior is identical to the previous StartSingBoxProcess.
// skipRunningCheck: если true, пропускает проверку на уже запущенный процесс (для автоперезапуска).
func (svc *ProcessService) Start(skipRunningCheck ...bool) {
	ac := svc.ac
	if ac.RunningState.IsRunning() {
		dialogs.ShowAutoHideInfo(ac.Application, ac.MainWindow, "Info", "Sing-Box already running (according to internal state).")
		return
	}

	// Проверяем, не запущен ли уже процесс на уровне ОС (пропускаем при автоперезапуске)
	skipCheck := len(skipRunningCheck) > 0 && skipRunningCheck[0]
	if !skipCheck {
		if svc.checkAndShowSingBoxRunningWarning("startSingBox") {
			return
		}
	}

	ac.CmdMutex.Lock()
	defer ac.CmdMutex.Unlock()

	// Check capabilities on Linux before starting
	if suggestion := platform.CheckAndSuggestCapabilities(ac.SingboxPath); suggestion != "" {
		log.Printf("startSingBox: Capabilities check failed: %s", suggestion)
		dialogs.ShowError(ac.MainWindow, fmt.Errorf("Linux capabilities required\n\n%s", suggestion))
		return
	}

	// Reload API config from config.json before starting (in case it was corrupted)
	log.Println("startSingBox: Reloading API config from config.json...")
	if base, tok, err := api.LoadClashAPIConfig(ac.ConfigPath); err != nil {
		log.Printf("startSingBox: Clash API config error: %v", err)
		ac.ClashAPIBaseURL = ""
		ac.ClashAPIToken = ""
		ac.ClashAPIEnabled = false
	} else {
		ac.ClashAPIBaseURL = base
		ac.ClashAPIToken = tok
		ac.ClashAPIEnabled = true
		log.Printf("startSingBox: API config reloaded successfully")
	}

	// Reload SelectedClashGroup from config
	if ac.ClashAPIEnabled {
		_, defaultSelector, err := config.GetSelectorGroupsFromConfig(ac.ConfigPath)
		if err != nil {
			log.Printf("startSingBox: Failed to get selector groups: %v", err)
			ac.SelectedClashGroup = "proxy-out" // Default fallback
		} else {
			ac.SelectedClashGroup = defaultSelector
			log.Printf("startSingBox: SelectedClashGroup reloaded: %s", defaultSelector)
		}
	}

	// Check and remove existing TUN interface before starting (prevents "file already exists" error)
	if runtime.GOOS == "windows" {
		interfaceName, err := config.GetTunInterfaceName(ac.ConfigPath)
		if err != nil {
			log.Printf("startSingBox: Failed to get TUN interface name from config: %v", err)
			// Continue anyway - maybe config doesn't have TUN
		} else if interfaceName != "" {
			log.Printf("startSingBox: Checking for existing TUN interface '%s'...", interfaceName)
			if err := svc.removeTunInterface(interfaceName); err != nil {
				log.Printf("startSingBox: Warning: Failed to remove TUN interface: %v", err)
				// Non-critical error - sing-box might handle existing interface
			}
		}
	}

	// Reset API cache before starting
	if ac.ResetAPIStateFunc != nil {
		log.Println("startSingBox: Resetting API state cache...")
		ac.ResetAPIStateFunc()
	}

	log.Println("startSingBox: Starting Sing-Box...")
	ac.SingboxCmd = exec.Command(ac.SingboxPath, "run", "-c", filepath.Base(ac.ConfigPath))
	platform.PrepareCommand(ac.SingboxCmd)
	ac.SingboxCmd.Dir = platform.GetBinDir(ac.ExecDir)
	if ac.ChildLogFile != nil {
		// Check and rotate log file before starting new process to prevent unbounded growth
		checkAndRotateLogFile(filepath.Join(ac.ExecDir, childLogFileName))

		// Write directly to file - no buffering in memory
		// This prevents memory leaks from accumulating log output
		// Logs are written immediately to disk, not stored in memory
		ac.SingboxCmd.Stdout = ac.ChildLogFile
		ac.SingboxCmd.Stderr = ac.ChildLogFile
	} else {
		log.Println("startSingBox: Warning: sing-box log file not available, output will not be logged.")
	}
	if err := ac.SingboxCmd.Start(); err != nil {
		ac.ShowStartupError(fmt.Errorf("failed to start Sing-Box process: %w", err))
		log.Printf("startSingBox: Failed to start Sing-Box: %v", err)
		return
	}
	ac.RunningState.Set(true)
	ac.StoppedByUser = false
	// Add log with PID
	log.Printf("startSingBox: Sing-Box started. PID=%d", ac.SingboxCmd.Process.Pid)

	// Start auto-loading proxies after sing-box is running
	go func() {
		// Small delay to ensure API is ready
		time.Sleep(2 * time.Second)
		ac.AutoLoadProxies()
	}()

	go svc.Monitor(ac.SingboxCmd)
}

// Monitor tracks the sing-box process and auto-restarts on crash (same logic as before).
func (svc *ProcessService) Monitor(cmdToMonitor *exec.Cmd) {
	ac := svc.ac
	// Store the PID we're monitoring to avoid conflicts with restarted processes
	monitoredPID := cmdToMonitor.Process.Pid

	// Wait for process completion - no timeout for long-running processes
	// The process should run until it exits or is stopped by user
	err := cmdToMonitor.Wait()

	ac.CmdMutex.Lock()
	defer ac.CmdMutex.Unlock()

	// GOLDEN STANDARD: Check order to prevent all race conditions
	// 1. First PID (is this my process?)
	if ac.SingboxCmd == nil || ac.SingboxCmd.Process == nil || ac.SingboxCmd.Process.Pid != monitoredPID {
		log.Printf("monitorSingBox: Process was restarted (PID changed from %d). This monitor is obsolete. Exiting.", monitoredPID)
		return
	}

	// 2. Then StoppedByUser (did user stop it?)
	if ac.StoppedByUser {
		log.Println("monitorSingBox: Sing-Box exited as requested by user.")
		ac.ConsecutiveCrashAttempts = 0
		ac.RunningState.Set(false)
		ac.StoppedByUser = false // Reset flag for next start
		return
	}

	// 3. Then err == nil (exited normally?)
	if err == nil {
		log.Println("monitorSingBox: Sing-Box exited gracefully (exit code 0).")
		ac.ConsecutiveCrashAttempts = 0
		ac.RunningState.Set(false)
		return
	}

	// 4. Only then — crash → restart
	// Процесс завершился с ошибкой - проверяем лимит попыток
	ac.RunningState.Set(false)
	ac.ConsecutiveCrashAttempts++

	if ac.ConsecutiveCrashAttempts > restartAttempts {
		log.Printf("monitorSingBox: Maximum restart attempts (%d) reached. Stopping auto-restart.", restartAttempts)
		dialogs.ShowError(ac.MainWindow, fmt.Errorf("Sing-Box failed to restart after %d attempts. Check sing-box.log for details.", restartAttempts))
		ac.ConsecutiveCrashAttempts = 0
		return
	}

	// Try to restart
	log.Printf("monitorSingBox: Sing-Box crashed: %v, attempting auto-restart (attempt %d/%d)", err, ac.ConsecutiveCrashAttempts, restartAttempts)
	dialogs.ShowAutoHideInfo(ac.Application, ac.MainWindow, "Crash", fmt.Sprintf("Sing-Box crashed, restarting... (attempt %d/%d)", ac.ConsecutiveCrashAttempts, restartAttempts))

	// Wait 2 seconds before restart
	ac.CmdMutex.Unlock()
	time.Sleep(2 * time.Second)
	svc.Start(true) // skipRunningCheck = true для автоперезапуска
	ac.CmdMutex.Lock()

	if ac.RunningState.IsRunning() {
		log.Println("monitorSingBox: Sing-Box restarted successfully.")
		currentAttemptCount := ac.ConsecutiveCrashAttempts
		go func() {
			select {
			case <-ac.ctx.Done():
				log.Println("monitorSingBox: Stability check cancelled (context cancelled)")
				return
			case <-time.After(stabilityThreshold):
				ac.CmdMutex.Lock()
				defer ac.CmdMutex.Unlock()

				if ac.RunningState.IsRunning() && ac.ConsecutiveCrashAttempts == currentAttemptCount {
					log.Printf("monitorSingBox: Process has been stable for %v. Resetting crash counter from %d to 0.", stabilityThreshold, ac.ConsecutiveCrashAttempts)
					ac.ConsecutiveCrashAttempts = 0
					// Обновляем UI, чтобы счетчик исчез из статуса на вкладке Core
					if ac.UpdateCoreStatusFunc != nil {
						ac.UpdateCoreStatusFunc()
					}
				} else {
					log.Printf("monitorSingBox: Stability timer expired, but conditions for reset not met (running: %v, current attempts: %d, attempts at timer start: %d).", ac.RunningState.IsRunning(), ac.ConsecutiveCrashAttempts, currentAttemptCount)
				}
			}
		}()
	} else {
		log.Printf("monitorSingBox: Restart attempt %d failed.", ac.ConsecutiveCrashAttempts)
	}
}

// Stop attempts graceful shutdown, mirroring previous StopSingBoxProcess.
func (svc *ProcessService) Stop() {
	ac := svc.ac
	ac.CmdMutex.Lock()

	// CRITICAL: Set flag BEFORE sending signal
	// This ensures the monitor sees the flag even if the process exits very quickly
	ac.StoppedByUser = true
	ac.ConsecutiveCrashAttempts = 0

	if !ac.RunningState.IsRunning() {
		ac.StoppedByUser = false
		ac.CmdMutex.Unlock()
		return
	}

	if ac.SingboxCmd == nil || ac.SingboxCmd.Process == nil {
		log.Println("StopSingBoxProcess: Inconsistent state detected. Correcting state.")
		ac.RunningState.Set(false)
		ac.StoppedByUser = false
		ac.CmdMutex.Unlock()
		return
	}

	log.Println("stopSingBox: Attempting graceful shutdown...")
	processToStop := ac.SingboxCmd.Process

	// Разблокируем мьютекс перед отправкой сигнала, чтобы не блокировать
	ac.CmdMutex.Unlock()

	var err error
	if runtime.GOOS == "windows" {
		err = platform.SendCtrlBreak(processToStop.Pid)
	} else {
		err = processToStop.Signal(os.Interrupt)
	}

	if err != nil {
		log.Printf("stopSingBox: Graceful signal failed: %v. Forcing kill.", err)
		if killErr := processToStop.Kill(); killErr != nil {
			log.Printf("stopSingBox: Failed to kill Sing-Box process: %v", killErr)
		}
	} else {
		// Start watchdog timer that will kill the process if it doesn't close itself
		log.Println("stopSingBox: Signal sent, starting watchdog timer...")
		go func(pid int) {
			time.Sleep(gracefulShutdownTimeout)
			p, _ := ps.FindProcess(pid)
			if p != nil {
				log.Printf("stopSingBox watchdog: Process %d still running after timeout. Forcing kill.", pid)
				// Reliably kill the process and its child processes
				_ = platform.KillProcessByPID(pid)
			}
		}(processToStop.Pid)
	}
}

// CheckIfRunningAtStart checks if sing-box is already running at application start.
// Shows a warning dialog if a running instance is detected.
func (svc *ProcessService) CheckIfRunningAtStart() {
	svc.checkAndShowSingBoxRunningWarning("CheckIfSingBoxRunningAtStart")
}

// checkAndShowSingBoxRunningWarning checks if sing-box is running and shows warning dialog if found.
// Returns true if process was found and warning was shown, false otherwise.
func (svc *ProcessService) checkAndShowSingBoxRunningWarning(context string) bool {
	ac := svc.ac
	found, foundPID := svc.isSingBoxProcessRunning()
	if found {
		log.Printf("%s: Found sing-box process already running (PID=%d). Showing warning dialog.", context, foundPID)
		ShowSingBoxAlreadyRunningWarningUtil(ac)
		return true
	}
	log.Printf("%s: No sing-box process found", context)
	return false
}

// isSingBoxProcessRunning checks if sing-box process is running on the system.
// Returns (isRunning, pid) tuple.
// Uses getOurPID for thread-safe PID retrieval (with mutex protection).
func (svc *ProcessService) isSingBoxProcessRunning() (bool, int) {
	ac := svc.ac
	// Use getOurPID for thread-safe access (same as original implementation)
	ourPID := getOurPID(ac)

	if runtime.GOOS == "windows" {
		// Use tasklist on Windows for better reliability
		processName := platform.GetProcessNameForCheck()
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", processName), "/FO", "CSV", "/NH")
		platform.PrepareCommand(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Printf("isSingBoxProcessRunning: tasklist failed: %v", err)
			// Fallback to ps library
			return svc.isSingBoxProcessRunningWithPS(ourPID)
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := parseCSVLine(line)
			if len(parts) >= 2 {
				name := strings.Trim(parts[0], "\"")
				pidStr := strings.Trim(parts[1], "\"")
				// CRITICAL: Check that the process name matches sing-box.exe
				if strings.EqualFold(name, processName) {
					if pid, err := strconv.Atoi(pidStr); err == nil {
						isOurProcess := (ourPID != -1 && pid == ourPID)
						log.Printf("isSingBoxProcessRunning: Found process: PID=%d, name='%s' (our tracked PID=%d, isOurProcess=%v)",
							pid, name, ourPID, isOurProcess)
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
	return svc.isSingBoxProcessRunningWithPS(ourPID)
}

// isSingBoxProcessRunningWithPS uses ps library to check for running process
func (svc *ProcessService) isSingBoxProcessRunningWithPS(ourPID int) (bool, int) {
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

// checkTunInterfaceExists checks if TUN interface exists on Windows
func (svc *ProcessService) checkTunInterfaceExists(interfaceName string) (bool, error) {
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
func (svc *ProcessService) removeTunInterface(interfaceName string) error {
	if runtime.GOOS != "windows" {
		// On Linux/macOS, interface is removed automatically
		return nil
	}

	// Check if interface exists
	exists, err := svc.checkTunInterfaceExists(interfaceName)
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
		return fmt.Errorf("failed to remove interface: %w", err)
	}

	log.Printf("removeTunInterface: Successfully removed TUN interface '%s'", interfaceName)
	return nil
}
