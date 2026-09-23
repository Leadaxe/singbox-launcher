//go:build linux

package platform

import (
	"os"
	"os/exec"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/paths"
)

// ResolveSingboxExecPath returns the sing-box binary path used to run the core.
// If sing-box is found in PATH (e.g. distro package), that path is used; otherwise bundledPath.
func ResolveSingboxExecPath(_ paths.DataDir, bundledPath string) string {
	name := GetExecutableNames()
	p, err := exec.LookPath(name)
	if err != nil {
		debuglog.DebugLog("ResolveSingboxExecPath: LookPath(%q): %v; using bundled %s", name, err, bundledPath)
		return bundledPath
	}
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		debuglog.DebugLog("ResolveSingboxExecPath: PATH candidate %q unusable (%v); using bundled %s", p, err, bundledPath)
		return bundledPath
	}
	debuglog.DebugLog("ResolveSingboxExecPath: using PATH %s", p)
	return p
}
