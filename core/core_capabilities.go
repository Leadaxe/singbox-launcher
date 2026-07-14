package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// SPEC 044 follow-up (the deferred "sing-box binary feature-probe"): naive
// outbound exists only in cores built with `with_naive_outbound`, and purego
// builds additionally load libcronet.{dll,dylib,so} at runtime from the
// binary's directory / library search path. A single naive node on a core
// without support fails `sing-box check` for the WHOLE config — the probe
// below lets the generator degrade naive nodes instead (same policy as
// broken-URI nodes: drop the node, keep the config).
//
// The verdict is conservative: any uncertainty (no core binary, version call
// failed, no "Tags:" line) → "supported", so we never silently drop nodes on
// guesswork; `sing-box check` remains the backstop for exotic cores.

// naiveSupportVerdict — cached probe result, keyed by the core binary's
// (mtime, size) so a core reinstall in the same session re-probes.
type naiveSupportVerdict struct {
	binMtime  time.Time
	binSize   int64
	supported bool
	reason    string
}

// CoreSupportsNaive reports whether the installed sing-box core can create
// naive outbounds, with a human-readable reason when it can't.
func (ac *AppController) CoreSupportsNaive() (bool, string) {
	if ac == nil || ac.FileService == nil {
		return true, ""
	}
	singboxPath := ac.FileService.SingboxPath
	if resolved, err := exec.LookPath(singboxPath); err == nil {
		singboxPath = resolved
	}
	st, err := os.Stat(singboxPath)
	if err != nil {
		return true, "" // no core installed — nothing to probe, check is skipped anyway
	}

	ac.naiveSupportCacheMu.Lock()
	defer ac.naiveSupportCacheMu.Unlock()
	if c := ac.naiveSupportCache; c != nil && c.binMtime.Equal(st.ModTime()) && c.binSize == st.Size() {
		return c.supported, c.reason
	}

	supported, reason := probeNaiveSupport(singboxPath)
	ac.naiveSupportCache = &naiveSupportVerdict{
		binMtime:  st.ModTime(),
		binSize:   st.Size(),
		supported: supported,
		reason:    reason,
	}
	if !supported {
		debuglog.WarnLog("CoreSupportsNaive: %s", reason)
	}
	return supported, reason
}

// probeNaiveSupport runs `sing-box version` and derives the verdict from the
// build tags plus (for purego builds) libcronet presence.
func probeNaiveSupport(singboxPath string) (bool, string) {
	cmd := exec.Command(singboxPath, "version")
	platform.PrepareCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		debuglog.WarnLog("probeNaiveSupport: sing-box version failed: %v", err)
		return true, ""
	}
	return naiveVerdictFromVersionOutput(string(output), cronetLibAvailable(singboxPath))
}

var versionTagsRegex = regexp.MustCompile(`(?m)^Tags:\s*(\S+)`)

// naiveVerdictFromVersionOutput — pure part of the probe, unit-testable.
// libAvailable is the cronetLibAvailable result for the same binary.
func naiveVerdictFromVersionOutput(versionOutput string, libAvailable bool) (bool, string) {
	m := versionTagsRegex.FindStringSubmatch(versionOutput)
	if m == nil {
		return true, "" // unknown output format — don't degrade on guesswork
	}
	tags := strings.Split(m[1], ",")
	hasTag := func(want string) bool {
		for _, t := range tags {
			if t == want {
				return true
			}
		}
		return false
	}
	if !hasTag("with_naive_outbound") {
		return false, "sing-box core is built without with_naive_outbound"
	}
	// Static (musl/CGO) builds link cronet in; only purego builds need the
	// companion library at runtime.
	if hasTag("with_purego") && !libAvailable {
		return false, fmt.Sprintf("sing-box core needs %s next to the binary — re-download the core to get it", cronetLibName())
	}
	return true, ""
}

// cronetLibName — platform-specific companion library filename the cronet
// purego loader looks for.
func cronetLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "libcronet.dll"
	case "darwin":
		return "libcronet.dylib"
	default:
		return "libcronet.so"
	}
}

// cronetLibAvailable mirrors the search order of cronet-go's purego loader:
// the binary's directory, then PATH (Windows) or LD_LIBRARY_PATH /
// DYLD_LIBRARY_PATH + /usr/local/lib + /usr/lib (unix).
func cronetLibAvailable(singboxPath string) bool {
	libName := cronetLibName()
	dirs := []string{filepath.Dir(singboxPath)}
	switch runtime.GOOS {
	case "windows":
		dirs = append(dirs, filepath.SplitList(os.Getenv("PATH"))...)
	default:
		dirs = append(dirs, filepath.SplitList(os.Getenv("LD_LIBRARY_PATH"))...)
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.SplitList(os.Getenv("DYLD_LIBRARY_PATH"))...)
		}
		dirs = append(dirs, "/usr/local/lib", "/usr/lib")
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, libName)); err == nil {
			return true
		}
	}
	return false
}
