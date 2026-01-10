//go:build windows
// +build windows

package platform

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"singbox-launcher/internal/constants"
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
	return exec.Command("explorer", url).Start()
}

// KillProcess kills a process by name
func KillProcess(processName string) error {
	return exec.Command("taskkill", "/IM", processName, "/F").Run()
}

// KillProcessByPID kills a process and its children by PID
func KillProcessByPID(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
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
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// GetRequiredFiles returns platform-specific required files
func GetRequiredFiles(execDir string) []struct {
	Name string
	Path string
} {
	return []struct {
		Name string
		Path string
	}{
		{"Sing-Box", filepath.Join(execDir, "bin", "sing-box.exe")},
		{"Config.json", filepath.Join(execDir, "bin", "config.json")},
		{"WinTun.dll", filepath.Join(execDir, "bin", "wintun.dll")},
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

// GetSystemSOCKSProxy returns system SOCKS proxy settings if enabled (SOCKS is macOS-specific)
// On Windows, this is not currently implemented
func GetSystemSOCKSProxy() (host string, port int, enabled bool, err error) {
	log.Printf("platform: GetSystemSOCKSProxy is not implemented on Windows")
	return "", 0, false, fmt.Errorf("GetSystemSOCKSProxy is not implemented on Windows")
}

// SetupDockReopenHandler is a no-op on Windows (Dock is macOS-specific)
func SetupDockReopenHandler(showWindowCallback func()) {
	log.Printf("platform: SetupDockReopenHandler is not implemented on Windows (Dock is macOS-specific)")
}

// CleanupDockReopenHandler is a no-op on Windows (Dock is macOS-specific)
func CleanupDockReopenHandler() {
	log.Printf("platform: CleanupDockReopenHandler is not implemented on Windows (Dock is macOS-specific)")
}
