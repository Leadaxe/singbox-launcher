//go:build !darwin
// +build !darwin

package platform

import "errors"

// errPrivilegedNotSupported is returned by RunWithPrivileges on non-darwin platforms.
var errPrivilegedNotSupported = errors.New("privileged execution not supported on this platform")

// RunWithPrivileges runs a command with elevated privileges (macOS only).
// On non-darwin platforms it returns (0, error).
func RunWithPrivileges(toolPath string, args []string) (pid int, err error) {
	_ = toolPath
	_ = args
	return 0, errPrivilegedNotSupported
}

// WaitForPrivilegedExit is a no-op on non-darwin.
func WaitForPrivilegedExit(pid int) {
	_ = pid
}
