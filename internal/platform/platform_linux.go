//go:build linux
// +build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"singbox-launcher/internal/constants"
)

// GetExecutableNames returns platform-specific executable names
func GetExecutableNames() string {
	return "sing-box"
}

// GetWintunPath returns empty string on Linux (wintun is Windows-only)
func GetWintunPath(execDir string) string {
	return ""
}

// OpenFolder opens a folder in the default file manager
func OpenFolder(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// OpenURL opens a URL in the default browser
func OpenURL(url string) error {
	return exec.Command("xdg-open", url).Start()
}

// KillProcess kills a process by name
func KillProcess(processName string) error {
	return exec.Command("killall", processName).Run()
}

// KillProcessByPID kills a process by PID
func KillProcessByPID(pid int) error {
	return exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
}

// SendCtrlBreak is not applicable on Linux; provided for interface parity.
func SendCtrlBreak(pid int) error {
	// CTRL_BREAK_EVENT is Windows-specific; callers should use SIGINT directly on Linux.
	return fmt.Errorf("SendCtrlBreak not supported on linux for pid %d", pid)
}

// PrepareCommand prepares a command with platform-specific attributes
func PrepareCommand(cmd *exec.Cmd) {
	// No special attributes needed for Linux
	// Capabilities should be set on the sing-box binary itself
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

// CheckSingBoxCapabilities checks if sing-box has the required capabilities
// Returns true if capabilities are set, false otherwise
func CheckSingBoxCapabilities(singboxPath string) bool {
	// Use getcap to check capabilities
	cmd := exec.Command("getcap", singboxPath)
	output, err := cmd.Output()
	if err != nil {
		// getcap returns error if no capabilities are set
		return false
	}

	// Check if output contains required capabilities
	outputStr := string(output)
	hasNetAdmin := strings.Contains(outputStr, "cap_net_admin")
	hasNetBind := strings.Contains(outputStr, "cap_net_bind_service")

	return hasNetAdmin && hasNetBind
}

// GetSetCapCommand returns the command to set capabilities on sing-box
func GetSetCapCommand(singboxPath string) string {
	return fmt.Sprintf("sudo setcap 'cap_net_admin,cap_net_bind_service=+ep' %s", singboxPath)
}

// SuggestCapabilities shows a dialog suggesting to set capabilities
// This should be called from the UI layer
func SuggestCapabilities(singboxPath string) string {
	command := GetSetCapCommand(singboxPath)
	return fmt.Sprintf(
		"⚠️ Sing-box requires elevated privileges to create TUN interface.\n\n"+
			"To avoid entering password every time, set capabilities once:\n\n"+
			"%s\n\n"+
			"This will allow sing-box to run without sudo/pkexec.",
		command,
	)
}

// CheckAndSuggestCapabilities checks capabilities and returns a suggestion if needed
// Returns empty string if capabilities are OK, otherwise returns suggestion message
func CheckAndSuggestCapabilities(singboxPath string) string {
	// Check if file exists
	if _, err := os.Stat(singboxPath); os.IsNotExist(err) {
		return "" // File doesn't exist yet, skip check
	}

	if !CheckSingBoxCapabilities(singboxPath) {
		return SuggestCapabilities(singboxPath)
	}

	return "" // Capabilities are OK
}
