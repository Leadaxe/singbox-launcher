//go:build darwin
// +build darwin

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// GetSetCapCommand returns empty on macOS (capabilities are Linux-only).
func GetSetCapCommand(singboxPath string) string {
	return ""
}

// GetSystemSOCKSProxy returns system SOCKS proxy settings if enabled
// Returns proxy host, port, and enabled status
// Checks network interfaces in priority order and returns the first interface with enabled proxy
func GetSystemSOCKSProxy() (host string, port int, enabled bool, err error) {
	// Get network service order to determine priority
	cmd := exec.Command("networksetup", "-listnetworkserviceorder")
	output, err := cmd.Output()
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to list network services: %w", err)
	}

	// Parse network service order - extract interface names in priority order
	lines := strings.Split(string(output), "\n")
	var interfaces []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match lines like "(1) Wi-Fi" or "(2) Ethernet"
		if strings.HasPrefix(line, "(") && strings.Contains(line, ")") {
			parts := strings.SplitN(line, ")", 2)
			if len(parts) == 2 {
				interfaceName := strings.TrimSpace(parts[1])
				// Skip disabled interfaces (marked with asterisk)
				if !strings.HasPrefix(interfaceName, "*") {
					interfaces = append(interfaces, interfaceName)
				}
			}
		}
	}

	// Check each interface in priority order for enabled SOCKS proxy
	for _, iface := range interfaces {
		host, port, enabled, err := getSOCKSProxyForInterface(iface)
		if err != nil {
			// Log error but continue checking other interfaces
			continue
		}
		if enabled {
			return host, port, true, nil
		}
	}

	return "", 0, false, nil
}

// getSOCKSProxyForInterface gets SOCKS proxy settings for a specific network interface
func getSOCKSProxyForInterface(iface string) (host string, port int, enabled bool, err error) {
	cmd := exec.Command("networksetup", "-getsocksfirewallproxy", iface)
	output, err := cmd.Output()
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to get proxy settings for %s: %w", iface, err)
	}

	// Parse output: Enabled: Yes/No, Server: <host>, Port: <port>
	lines := strings.Split(string(output), "\n")
	var proxyHost string
	var proxyPort int
	var proxyEnabled bool

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Enabled:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				enabledStr := strings.TrimSpace(parts[1])
				proxyEnabled = enabledStr == "Yes"
			}
		} else if strings.HasPrefix(line, "Server:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				proxyHost = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "Port:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				portStr := strings.TrimSpace(parts[1])
				parsedPort, parseErr := strconv.Atoi(portStr)
				if parseErr == nil {
					proxyPort = parsedPort
				}
			}
		}
	}

	if proxyEnabled && proxyHost != "" && proxyPort > 0 {
		return proxyHost, proxyPort, true, nil
	}

	return "", 0, false, nil
}
