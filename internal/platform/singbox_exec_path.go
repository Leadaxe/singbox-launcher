//go:build !linux

package platform

import "singbox-launcher/internal/paths"

// ResolveSingboxExecPath returns the sing-box binary path used to run the core.
// On non-Linux platforms the bundled path (next to the launcher) is always used.
func ResolveSingboxExecPath(_ paths.DataDir, bundledPath string) string {
	return bundledPath
}
