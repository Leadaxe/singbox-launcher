package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/process"
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
		if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Info", "Sing-Box already running (according to internal state).")
		}
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
	if suggestion := platform.CheckAndSuggestCapabilities(ac.FileService.SingboxPath); suggestion != "" {
		debuglog.WarnLog("startSingBox: Capabilities check failed: %s", suggestion)
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Linux capabilities required\n\n%s", suggestion))
		}
		return
	}

	// Reload Clash API configuration from config.json before starting
	// This ensures we pick up any changes made via wizard or manual config edits
	if ac.APIService != nil {
		debuglog.InfoLog("startSingBox: Reloading Clash API configuration...")
		if err := ac.APIService.ReloadClashAPIConfig(); err != nil {
			debuglog.WarnLog("startSingBox: Warning: Failed to reload Clash API config: %v", err)
			// Continue anyway - API might not be configured or config might be invalid
		}
	}

	// Check and remove existing TUN interface before starting (prevents "file already exists" error)
	if runtime.GOOS == "windows" {
		interfaceName, err := config.GetTunInterfaceName(ac.FileService.ConfigPath)
		if err != nil {
			debuglog.WarnLog("startSingBox: Failed to get TUN interface name from config: %v", err)
			// Continue anyway - maybe config doesn't have TUN
		} else if interfaceName != "" {
			debuglog.DebugLog("startSingBox: Checking for existing TUN interface '%s'...", interfaceName)
			if err := svc.removeTunInterface(interfaceName); err != nil {
				debuglog.WarnLog("startSingBox: Warning: Failed to remove TUN interface: %v", err)
				// Non-critical error - sing-box might handle existing interface
			}
		}
	}

	// Reset API cache before starting
	if ac.UIService != nil && ac.UIService.ResetAPIStateFunc != nil {
		debuglog.InfoLog("startSingBox: Resetting API state cache...")
		ac.UIService.ResetAPIStateFunc()
	}

	// On macOS, use privileged start only when config has TUN (so password is asked only when needed)
	if runtime.GOOS == "darwin" {
		hasTun, err := config.ConfigHasTun(ac.FileService.ConfigPath)
		if err != nil {
			debuglog.WarnLog("startSingBox: Could not check TUN in config: %v; using privileged start.", err)
			hasTun = true
		}
		if hasTun {
			if err := svc.startSingBoxPrivileged(); err != nil {
				ac.ShowStartupError(err)
				return
			}
			return
		}
	}

	debuglog.InfoLog("startSingBox: Starting Sing-Box...")
	ac.SingboxCmd = exec.Command(ac.FileService.SingboxPath, "run", "-c", filepath.Base(ac.FileService.ConfigPath))
	platform.PrepareCommand(ac.SingboxCmd)
	ac.SingboxCmd.Dir = platform.GetBinDir(ac.FileService.ExecDir)
	if ac.FileService.ChildLogFile != nil {
		// Check and rotate log file before starting new process to prevent unbounded growth
		ac.FileService.CheckAndRotateLogFile(filepath.Join(ac.FileService.ExecDir, childLogFileName))

		// Write directly to file - no buffering in memory
		// This prevents memory leaks from accumulating log output
		// Logs are written immediately to disk, not stored in memory
		ac.SingboxCmd.Stdout = ac.FileService.ChildLogFile
		ac.SingboxCmd.Stderr = ac.FileService.ChildLogFile
	} else {
		debuglog.WarnLog("startSingBox: Warning: sing-box log file not available, output will not be logged.")
	}
	if err := ac.SingboxCmd.Start(); err != nil {
		ac.ShowStartupError(fmt.Errorf("failed to start Sing-Box process: %w", err))
		debuglog.ErrorLog("startSingBox: Failed to start Sing-Box: %v", err)
		return
	}
	ac.RunningState.Set(true)
	ac.StoppedByUser = false
	// Add log with PID
	debuglog.DebugLog("startSingBox: Sing-Box started. PID=%d", ac.SingboxCmd.Process.Pid)

	// Start auto-loading proxies after sing-box is running
	go func() {
		// Small delay to ensure API is ready
		<-time.After(2 * time.Second)
		ac.AutoLoadProxies()
	}()

	go svc.Monitor(ac.SingboxCmd)
}

// startSingBoxPrivileged starts sing-box with elevated privileges on macOS (for TUN).
// The script echoes its PID and sing-box's PID to stdout, runs sing-box in a subshell in background, then wait.
// On Stop we kill both PIDs explicitly (no process group).
func (svc *ProcessService) startSingBoxPrivileged() error {
	ac := svc.ac
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	configName := filepath.Base(ac.FileService.ConfigPath)
	logPath := filepath.Join(ac.FileService.ExecDir, childLogFileName)
	if ac.FileService.ChildLogFile != nil {
		ac.FileService.CheckAndRotateLogFile(logPath)
	}

	scriptPath := filepath.Join(binDir, "start-singbox-privileged.sh")
	pidFilePath := filepath.Join(binDir, "singbox.pid")

	// Script: echo script PID, run sing-box in subshell (redirect to log so nothing goes to pipe), echo sing-box PID, then exec and wait
	scriptBody := fmt.Sprintf(`#!/bin/sh
echo $$
cd %s
%s run -c %s >> %s 2>&1 &
echo $!
exec 1>>%s 2>&1
wait
`, strconv.Quote(binDir), strconv.Quote(ac.FileService.SingboxPath), strconv.Quote(configName), strconv.Quote(logPath), strconv.Quote(logPath))
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0700); err != nil {
		return fmt.Errorf("failed to write script %s: %w", scriptPath, err)
	}

	debuglog.InfoLog("startSingBox: Starting Sing-Box with elevated privileges (TUN)...")
	type privilegedPids struct{ Script, Singbox int }
	pidCh := make(chan privilegedPids, 1)
	go func() {
		scriptPID, singboxPID, runErr := platform.RunWithPrivileges("/bin/sh", []string{scriptPath})
		if runErr != nil {
			pidCh <- privilegedPids{0, 0}
			ac.CmdMutex.Lock()
			if ac.SingboxPrivilegedMode {
				ac.CmdMutex.Unlock()
				return
			}
			ac.CmdMutex.Unlock()
			debuglog.WarnLog("startSingBox: privileged run failed: %v", runErr)
			return
		}
		pidCh <- privilegedPids{scriptPID, singboxPID}
		if scriptPID <= 0 {
			return
		}
		ac.CmdMutex.Lock()
		ac.SingboxCmd = nil
		ac.SingboxPrivilegedMode = true
		ac.SingboxPrivilegedPID = scriptPID
		ac.SingboxPrivilegedSingboxPID = singboxPID
		ac.SingboxPrivilegedPIDFile = pidFilePath
		ac.RunningState.Set(true)
		ac.StoppedByUser = false
		ac.CmdMutex.Unlock()
		_ = os.WriteFile(pidFilePath, []byte(fmt.Sprintf("%d\n%d", scriptPID, singboxPID)), 0644)
		debuglog.DebugLog("startSingBox: Sing-Box started with privileges (script PID=%d, sing-box PID=%d).", scriptPID, singboxPID)
		platform.WaitForPrivilegedExit(scriptPID)
		svc.onPrivilegedScriptExited()
	}()

	pids := <-pidCh
	if pids.Script <= 0 {
		return fmt.Errorf("privileged start failed or cancelled (no PID)")
	}

	go func() {
		<-time.After(2 * time.Second)
		ac.AutoLoadProxies()
	}()
	return nil
}

// onPrivilegedScriptExited is called when the privileged script process exits (Wait4 returned).
// The script waits on sing-box, so when the script exits, sing-box has exited too.
func (svc *ProcessService) onPrivilegedScriptExited() {
	ac := svc.ac
	ac.CmdMutex.Lock()
	defer ac.CmdMutex.Unlock()
	if !ac.SingboxPrivilegedMode {
		return
	}
	ac.SingboxPrivilegedMode = false
	ac.SingboxPrivilegedPID = 0
	ac.SingboxPrivilegedSingboxPID = 0
	ac.SingboxPrivilegedPIDFile = ""
	ac.RunningState.Set(false)
	if ac.StoppedByUser {
		ac.StoppedByUser = false
		ac.ConsecutiveCrashAttempts = 0
		debuglog.InfoLog("onPrivilegedScriptExited: Stopped by user.")
		return
	}
	ac.ConsecutiveCrashAttempts++
	if ac.ConsecutiveCrashAttempts > restartAttempts {
		debuglog.DebugLog("onPrivilegedScriptExited: Max restart attempts reached.")
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Sing-Box failed to restart after %d attempts. Check sing-box.log for details.", restartAttempts))
		}
		ac.ConsecutiveCrashAttempts = 0
		return
	}
	debuglog.WarnLog("onPrivilegedScriptExited: Sing-Box exited, auto-restart (attempt %d/%d)", ac.ConsecutiveCrashAttempts, restartAttempts)
	if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Crash", fmt.Sprintf("Sing-Box crashed, restarting... (attempt %d/%d)", ac.ConsecutiveCrashAttempts, restartAttempts))
	}
	ac.CmdMutex.Unlock()
	<-time.After(2 * time.Second)
	svc.Start(true)
	ac.CmdMutex.Lock()
	if ac.RunningState.IsRunning() {
		currentAttemptCount := ac.ConsecutiveCrashAttempts
		go func() {
			select {
			case <-ac.ctx.Done():
				return
			case <-time.After(stabilityThreshold):
				ac.CmdMutex.Lock()
				defer ac.CmdMutex.Unlock()
				if ac.RunningState.IsRunning() && ac.ConsecutiveCrashAttempts == currentAttemptCount {
					ac.ConsecutiveCrashAttempts = 0
					if ac.UIService != nil && ac.UIService.UpdateCoreStatusFunc != nil {
						ac.UIService.UpdateCoreStatusFunc()
					}
				}
			}
		}()
	}
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
		debuglog.DebugLog("monitorSingBox: Process was restarted (PID changed from %d). This monitor is obsolete. Exiting.", monitoredPID)
		return
	}

	// 2. Then StoppedByUser (did user stop it?)
	if ac.StoppedByUser {
		debuglog.InfoLog("monitorSingBox: Sing-Box exited as requested by user.")
		ac.ConsecutiveCrashAttempts = 0
		ac.RunningState.Set(false)
		ac.StoppedByUser = false // Reset flag for next start
		return
	}

	// 3. Then err == nil (exited normally?)
	if err == nil {
		debuglog.InfoLog("monitorSingBox: Sing-Box exited gracefully (exit code 0).")
		ac.ConsecutiveCrashAttempts = 0
		ac.RunningState.Set(false)
		return
	}

	// 4. Only then — crash → restart
	// Процесс завершился с ошибкой - проверяем лимит попыток
	ac.RunningState.Set(false)
	ac.ConsecutiveCrashAttempts++

	if ac.ConsecutiveCrashAttempts > restartAttempts {
		debuglog.DebugLog("monitorSingBox: Maximum restart attempts (%d) reached. Stopping auto-restart.", restartAttempts)
		if ac.UIService != nil && ac.UIService.MainWindow != nil {
			dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Sing-Box failed to restart after %d attempts. Check sing-box.log for details.", restartAttempts))
		}
		ac.ConsecutiveCrashAttempts = 0
		return
	}

	// Try to restart
	debuglog.WarnLog("monitorSingBox: Sing-Box crashed: %v, attempting auto-restart (attempt %d/%d)", err, ac.ConsecutiveCrashAttempts, restartAttempts)
	if ac.UIService != nil && ac.UIService.Application != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Crash", fmt.Sprintf("Sing-Box crashed, restarting... (attempt %d/%d)", ac.ConsecutiveCrashAttempts, restartAttempts))
	}

	// Wait 2 seconds before restart
	ac.CmdMutex.Unlock()
	<-time.After(2 * time.Second)
	svc.Start(true) // skipRunningCheck = true для автоперезапуска
	ac.CmdMutex.Lock()

	if ac.RunningState.IsRunning() {
		debuglog.InfoLog("monitorSingBox: Sing-Box restarted successfully.")
		currentAttemptCount := ac.ConsecutiveCrashAttempts
		go func() {
			select {
			case <-ac.ctx.Done():
				debuglog.InfoLog("monitorSingBox: Stability check cancelled (context cancelled)")
				return
			case <-time.After(stabilityThreshold):
				ac.CmdMutex.Lock()
				defer ac.CmdMutex.Unlock()

				if ac.RunningState.IsRunning() && ac.ConsecutiveCrashAttempts == currentAttemptCount {
					debuglog.DebugLog("monitorSingBox: Process has been stable for %v. Resetting crash counter from %d to 0.", stabilityThreshold, ac.ConsecutiveCrashAttempts)
					ac.ConsecutiveCrashAttempts = 0
					// Обновляем UI, чтобы счетчик исчез из статуса на вкладке Core
					if ac.UIService != nil && ac.UIService.UpdateCoreStatusFunc != nil {
						ac.UIService.UpdateCoreStatusFunc()
					}
				} else {
					debuglog.DebugLog("monitorSingBox: Stability timer expired, but conditions for reset not met (running: %v, current attempts: %d, attempts at timer start: %d).", ac.RunningState.IsRunning(), ac.ConsecutiveCrashAttempts, currentAttemptCount)
				}
			}
		}()
	} else {
		debuglog.DebugLog("monitorSingBox: Restart attempt %d failed.", ac.ConsecutiveCrashAttempts)
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

	// Privileged mode (macOS TUN): kill both script and sing-box by PID via RunWithPrivileges
	if ac.SingboxPrivilegedMode && ac.SingboxPrivilegedPID != 0 && ac.SingboxPrivilegedPIDFile != "" {
		pidFile := ac.SingboxPrivilegedPIDFile
		scriptPID := ac.SingboxPrivilegedPID
		singboxPID := ac.SingboxPrivilegedSingboxPID
		ac.CmdMutex.Unlock()
		debuglog.InfoLog("stopSingBox: Stopping privileged Sing-Box (script PID %d, sing-box PID %d)...", scriptPID, singboxPID)
		killScript := fmt.Sprintf("kill -TERM %d 2>/dev/null", scriptPID)
		if singboxPID > 0 {
			killScript += fmt.Sprintf("; kill -TERM %d 2>/dev/null", singboxPID)
		}
		killScript += fmt.Sprintf("; rm -f %s", strconv.Quote(pidFile))
		if _, _, err := platform.RunWithPrivileges("/bin/sh", []string{"-c", killScript}); err != nil {
			debuglog.WarnLog("stopSingBox: Privileged kill failed (process may have already exited): %v", err)
		}
		ac.CmdMutex.Lock()
		ac.SingboxPrivilegedMode = false
		ac.SingboxPrivilegedPID = 0
		ac.SingboxPrivilegedSingboxPID = 0
		ac.SingboxPrivilegedPIDFile = ""
		ac.RunningState.Set(false)
		ac.StoppedByUser = false
		ac.CmdMutex.Unlock()
		return
	}

	if ac.SingboxCmd == nil || ac.SingboxCmd.Process == nil {
		debuglog.InfoLog("StopSingBoxProcess: Inconsistent state detected. Correcting state.")
		ac.RunningState.Set(false)
		ac.StoppedByUser = false
		ac.CmdMutex.Unlock()
		return
	}

	debuglog.InfoLog("stopSingBox: Attempting graceful shutdown...")
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
		debuglog.WarnLog("stopSingBox: Graceful signal failed: %v. Forcing kill.", err)
		if killErr := processToStop.Kill(); killErr != nil {
			debuglog.ErrorLog("stopSingBox: Failed to kill Sing-Box process: %v", killErr)
		}
	} else {
		// Start watchdog timer that will kill the process if it doesn't close itself
		debuglog.InfoLog("stopSingBox: Signal sent, starting watchdog timer...")
		go func(pid int) {
			<-time.After(gracefulShutdownTimeout)
			pInfo, found, err := process.FindProcess(pid)
			if err == nil && found {
				_ = pInfo // pInfo is the process info; we only need to know it exists
				debuglog.DebugLog("stopSingBox watchdog: Process %d still running after timeout. Forcing kill.", pid)
				// Reliably kill the process and its child processes
				_ = platform.KillProcessByPID(pid)
			} else if err != nil {
				debuglog.DebugLog("stopSingBox watchdog: error checking process %d: %v", pid, err)
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
func (svc *ProcessService) checkAndShowSingBoxRunningWarning(ctx string) bool {
	found, foundPID := svc.isSingBoxProcessRunning()
	if found {
		debuglog.DebugLog("%s: Found sing-box process already running (PID=%d). Showing warning dialog.", ctx, foundPID)
		if svc.ac.hasUI() {
			dialogs.ShowProcessKillConfirmation(svc.ac.UIService.MainWindow, func() {
				processName := platform.GetProcessNameForCheck()
				_ = platform.KillProcess(processName)
				svc.ac.RunningState.Set(false)
			})
		}
		return true
	}
	debuglog.DebugLog("%s: No sing-box process found", ctx)
	return false
}

// getTrackedPID safely gets the PID of the tracked sing-box process.
func (svc *ProcessService) getTrackedPID() int {
	svc.ac.CmdMutex.Lock()
	defer svc.ac.CmdMutex.Unlock()
	if svc.ac.SingboxPrivilegedMode && svc.ac.SingboxPrivilegedPID != 0 {
		return svc.ac.SingboxPrivilegedPID
	}
	if svc.ac.SingboxCmd != nil && svc.ac.SingboxCmd.Process != nil {
		return svc.ac.SingboxCmd.Process.Pid
	}
	return -1
}

// parseCSVLine parses a CSV line, handling quoted fields.
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
	if current.Len() > 0 || len(parts) > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// isSingBoxProcessRunning checks if sing-box process is running on the system.
// Returns (isRunning, pid) tuple.
func (svc *ProcessService) isSingBoxProcessRunning() (bool, int) {
	ourPID := svc.getTrackedPID()

	if runtime.GOOS == "windows" {
		// Use tasklist on Windows for better reliability
		processName := platform.GetProcessNameForCheck()
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", processName), "/FO", "CSV", "/NH")
		platform.PrepareCommand(cmd)
		output, err := cmd.Output()
		if err != nil {
			debuglog.WarnLog("isSingBoxProcessRunning: tasklist failed: %v", err)
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
						debuglog.DebugLog("isSingBoxProcessRunning: Found process: PID=%d, name='%s' (our tracked PID=%d, isOurProcess=%v)",
							pid, name, ourPID, isOurProcess)
						return true, pid
					} else {
						debuglog.DebugLog("isSingBoxProcessRunning: Failed to parse PID '%s': %v", pidStr, err)
					}
				}
			}
		}
		debuglog.DebugLog("isSingBoxProcessRunning: tasklist found processes but none matched '%s'", processName)
		return false, -1
	}

	// For other OS use ps library
	return svc.isSingBoxProcessRunningWithPS(ourPID)
}

// isSingBoxProcessRunningWithPS uses ps library to check for running process
func (svc *ProcessService) isSingBoxProcessRunningWithPS(ourPID int) (bool, int) {
	processes, err := process.GetProcesses()
	if err != nil {
		debuglog.WarnLog("isSingBoxProcessRunningWithPS: error listing processes: %v", err)
		return false, -1
	}
	processName := platform.GetProcessNameForCheck()

	for _, p := range processes {
		execName := p.Name
		if strings.EqualFold(execName, processName) {
			foundPID := p.PID
			isOurProcess := (ourPID != -1 && foundPID == ourPID)
			debuglog.DebugLog("isSingBoxProcessRunningWithPS: Found process: PID=%d, name='%s' (our tracked PID=%d, isOurProcess=%v)", foundPID, execName, ourPID, isOurProcess)
			return true, foundPID
		}
	}
	debuglog.DebugLog("isSingBoxProcessRunningWithPS: No sing-box process found (checked %d processes)", len(processes))
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
		debuglog.WarnLog("removeTunInterface: Failed to check interface existence: %v", err)
		// Continue anyway - try to remove it
	}

	if !exists {
		debuglog.DebugLog("removeTunInterface: Interface '%s' does not exist, nothing to remove", interfaceName)
		return nil
	}

	debuglog.DebugLog("removeTunInterface: Removing existing TUN interface '%s'...", interfaceName)

	// Remove the interface using netsh
	cmd := exec.Command("netsh", "interface", "delete", "interface", fmt.Sprintf("name=%s", interfaceName))
	platform.PrepareCommand(cmd) // Hide console window on Windows

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Interface might be in use or already deleted
		debuglog.WarnLog("removeTunInterface: Failed to remove interface '%s': %v, output: %s",
			interfaceName, err, string(output))
		// This is not a critical error - sing-box might handle it
		return fmt.Errorf("failed to remove interface: %w", err)
	}

	debuglog.DebugLog("removeTunInterface: Successfully removed TUN interface '%s'", interfaceName)
	return nil
}
