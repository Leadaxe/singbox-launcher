//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strconv"
)

// listProcessesImpl on Linux walks /proc/<pid>/exe, dereferences the
// symlink, and uses basename for the display name. No .desktop parsing
// on MVP — that's a TODO if users ask for "real" app names.
//
// /proc/<pid>/exe is a special symlink owned by the process; users see
// only the processes they own (kernel hides others unless ptrace is
// permitted). That matches macOS behavior and the SPEC's "best effort".
func listProcessesImpl() ([]ProcessEntry, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []ProcessEntry
	seen := make(map[string]struct{})
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(ent.Name())
		if err != nil {
			continue
		}
		exe := filepath.Join("/proc", ent.Name(), "exe")
		path, err := os.Readlink(exe)
		if err != nil {
			continue // typically EACCES for other-user processes
		}
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, ProcessEntry{
			PID:         pid,
			Path:        path,
			DisplayName: filepath.Base(path),
		})
	}
	return out, nil
}
