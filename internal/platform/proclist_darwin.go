//go:build darwin

package platform

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// listProcessesImpl on macOS uses `ps -axo pid=,comm=` which is reliable
// on every macOS release back to 10.10. The `comm` field gives us the
// full executable path for native programs (not the truncated short name
// `ps -A` shows by default).
//
// We deliberately do NOT invoke `osascript` / NSWorkspace via cgo on
// MVP: the SPEC explicitly accepts ps + generic icons as the macOS
// fallback. Cgo + NSWorkspace can come later as a §"Final decisions"
// item if users complain about missing icons.
func listProcessesImpl() ([]ProcessEntry, error) {
	cmd := exec.Command("ps", "-axo", "pid=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var entries []ProcessEntry
	seen := make(map[string]struct{})
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // long command lines on some daemons
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " ")
		if line == "" {
			continue
		}
		// Format: `12345 /path/to/executable`. PID is digits-then-space.
		space := strings.IndexByte(line, ' ')
		if space <= 0 {
			continue
		}
		pidStr := line[:space]
		path := strings.TrimSpace(line[space+1:])
		if path == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		// De-dup: if the user has 20 Chrome Helpers, the picker becomes
		// useless. Collapse by executable path.
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		entries = append(entries, ProcessEntry{
			PID:         pid,
			Path:        path,
			DisplayName: macDisplayName(path),
		})
	}
	return entries, nil
}

// macDisplayName extracts the friendly name from a bundle path. We don't
// parse Info.plist — basename of the .app dir gives the user-visible
// label for 99% of macOS apps.
//
//   /Applications/Slack.app/Contents/MacOS/Slack  →  Slack
//   /usr/local/bin/sing-box                       →  sing-box
//   /Applications/Firefox.app/Contents/MacOS/firefox → Firefox
func macDisplayName(path string) string {
	// Walk up until we find a `.app` segment; if any, the segment minus
	// the suffix is the bundle name.
	parts := strings.Split(path, string(filepath.Separator))
	for _, seg := range parts {
		if strings.HasSuffix(seg, ".app") {
			return strings.TrimSuffix(seg, ".app")
		}
	}
	return filepath.Base(path)
}
