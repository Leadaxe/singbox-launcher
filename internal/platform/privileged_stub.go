//go:build !darwin && (!windows || 386)

package platform

import "errors"

// errPrivilegedNotSupported is returned by RunWithPrivileges on non-darwin platforms.
var errPrivilegedNotSupported = errors.New("privileged execution not supported on this platform")

// Privileged start names, pid file and pkill pattern are empty on non-darwin.
const (
	PrivilegedStartName        = ""
	PrivilegedLegacyScriptName = ""
	PrivilegedPidFileName      = ""
	PrivilegedCopyName         = ""
	PrivilegedPkillPattern     = ""
)

// IsPrivilegedCoreProcessName — копии ядра под root вне macOS нет.
func IsPrivilegedCoreProcessName(name string) bool {
	_ = name
	return false
}

// RunWithPrivileges runs a command with elevated privileges (macOS only).
// On non-darwin platforms it returns (0, 0, error).
func RunWithPrivileges(toolPath string, args []string) (scriptPID, singboxPID int, err error) {
	_ = toolPath
	_ = args
	return 0, 0, errPrivilegedNotSupported
}

// StartPrivilegedCore is macOS-only (TUN start as root, SPEC 137).
func StartPrivilegedCore(corePath, binDir, configName string) (shellPID, corePID int, err error) {
	_ = corePath
	_ = binDir
	_ = configName
	return 0, 0, errPrivilegedNotSupported
}

// PrivilegedCoreLogPath — лога ядра под root вне macOS нет (SPEC 137.1).
func PrivilegedCoreLogPath() string { return "" }

// KillPrivilegedProcess is a no-op on non-darwin (privileged mode is macOS-only).
func KillPrivilegedProcess(scriptPID, singboxPID int, pidFile string) error {
	_ = scriptPID
	_ = singboxPID
	_ = pidFile
	return nil
}

// KillPrivilegedByPattern is macOS-only.
func KillPrivilegedByPattern() error {
	return errPrivilegedNotSupported
}

// WaitForPrivilegedExit is a no-op on non-darwin.
func WaitForPrivilegedExit(pid int) {
	_ = pid
}

// FreePrivilegedAuthorization is a no-op on non-darwin.
func FreePrivilegedAuthorization() {}
