//go:build darwin
// +build darwin

package platform

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"

	"singbox-launcher/internal/constants"
)

// GetExecutableNames returns platform-specific executable names
func GetExecutableNames() string {
	return "sing-box"
}

// GetWintunPath returns empty string on macOS (wintun is Windows-only)
func GetWintunPath(execDir string) string {
	return ""
}

// OpenFolder opens a folder in the default file manager
func OpenFolder(path string) error {
	return exec.Command("open", path).Start()
}

// OpenURL opens a URL in the default browser
func OpenURL(url string) error {
	return exec.Command("open", url).Start()
}

// KillProcess kills a process by name
func KillProcess(processName string) error {
	return exec.Command("killall", processName).Run()
}

// KillProcessByPID kills a process by PID
func KillProcessByPID(pid int) error {
	return exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
}

// SendCtrlBreak is not applicable on macOS; provided for interface parity.
func SendCtrlBreak(pid int) error {
	// CTRL_BREAK_EVENT is Windows-specific; callers should use SIGINT directly on macOS.
	return fmt.Errorf("SendCtrlBreak not supported on darwin for pid %d", pid)
}

// PrepareCommand prepares a command with platform-specific attributes
func PrepareCommand(cmd *exec.Cmd) {
	// No special attributes needed for macOS
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
		{"Sing-Box", filepath.Join(execDir, constants.BinDirName, constants.SingBoxExecName)},
		{"Config.json", filepath.Join(execDir, constants.BinDirName, constants.ConfigFileName)},
	}
}

// GetProcessNameForCheck returns the process name to check for running instances
func GetProcessNameForCheck() string {
	return "sing-box"
}

// GetBuildFlags returns platform-specific build flags
func GetBuildFlags() string {
	return ""
}

// CheckAndSuggestCapabilities is a no-op on macOS (capabilities not needed)
func CheckAndSuggestCapabilities(singboxPath string) string {
	return "" // Capabilities are Linux-specific, not needed on macOS
}
