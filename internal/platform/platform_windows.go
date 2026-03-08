//go:build windows
// +build windows

package platform

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
)

// GetExecutableNames returns platform-specific executable names
func GetExecutableNames() string {
	return "sing-box.exe"
}

// GetWintunPath returns the path to wintun.dll (Windows only)
func GetWintunPath(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.WinTunDLLName)
}

// OpenFolder opens a folder in the default file manager
func OpenFolder(path string) error {
	return exec.Command("explorer", path).Start()
}

// OpenURL opens a URL in the default browser
func OpenURL(url string) error {
	return exec.Command(
		"rundll32",
		"url.dll,FileProtocolHandler",
		url,
	).Start()
}

// KillProcess kills a process by name. Tries without /F first; on error retries with /F.
func KillProcess(processName string) error {
	cmd := exec.Command("taskkill", "/IM", processName)
	PrepareCommand(cmd)
	if err := cmd.Run(); err != nil {
		debuglog.DebugLog("KillProcess: taskkill without /F failed for %s: %v, retrying with /F", processName, err)
		cmd = exec.Command("taskkill", "/IM", processName, "/F")
		PrepareCommand(cmd)
		return cmd.Run()
	}
	return nil
}

// KillProcessByPID kills a process by PID. Tries without /F first; on error retries with /F.
func KillProcessByPID(pid int) error {
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid))
	PrepareCommand(cmd)
	if err := cmd.Run(); err != nil {
		debuglog.DebugLog("KillProcessByPID: taskkill without /F failed for PID %d: %v, retrying with /F", pid, err)
		cmd = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F")
		PrepareCommand(cmd)
		return cmd.Run()
	}
	return nil
}

// SendCtrlBreak sends CTRL_BREAK_EVENT to a process by PID.
func SendCtrlBreak(pid int) error {
	dll := syscall.NewLazyDLL("kernel32.dll")
	proc := dll.NewProc("GenerateConsoleCtrlEvent")
	if r, _, e := proc.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(pid)); r == 0 {
		return e
	}
	return nil
}

// PrepareCommand prepares a command with platform-specific attributes
func PrepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		//	CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// GetProcessNameForCheck returns the process name to check for running instances
func GetProcessNameForCheck() string {
	return "sing-box.exe"
}

// GetBuildFlags returns platform-specific build flags
func GetBuildFlags() string {
	return "-H windowsgui"
}

// CheckAndSuggestCapabilities is a no-op on Windows (capabilities not needed)
func CheckAndSuggestCapabilities(singboxPath string) string {
	return "" // Capabilities are Windows-specific, not needed here
}

// GetSetCapCommand returns empty on Windows (capabilities are Linux-only).
func GetSetCapCommand(singboxPath string) string {
	return ""
}

// GetSystemSOCKSProxy returns system SOCKS proxy settings if enabled (SOCKS is macOS-specific)
// On Windows, this is not currently implemented
func GetSystemSOCKSProxy() (host string, port int, enabled bool, err error) {
	debuglog.DebugLog("platform: GetSystemSOCKSProxy is not implemented on Windows")
	return "", 0, false, fmt.Errorf("GetSystemSOCKSProxy is not implemented on Windows")
}

// SetupDockReopenHandler is a no-op on Windows (Dock is macOS-specific)
func SetupDockReopenHandler(showWindowCallback func()) {
	debuglog.DebugLog("platform: SetupDockReopenHandler is not implemented on Windows (Dock is macOS-specific)")
}

// CleanupDockReopenHandler is a no-op on Windows (Dock is macOS-specific)
func CleanupDockReopenHandler() {
	debuglog.DebugLog("platform: CleanupDockReopenHandler is not implemented on Windows (Dock is macOS-specific)")
}
