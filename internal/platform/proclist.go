package platform

// ProcessEntry is one running process surfaced to the Traffic Profiler's
// process-picker dialog. We aim for "good enough" cross-platform:
//
//   - Path is the canonical executable path that sing-box's
//     route.find_process also returns. This is the matching key.
//   - DisplayName is the short user-visible label (e.g. "Slack" from
//     /Applications/Slack.app/Contents/MacOS/Slack).
//   - PID is informational; the profiler doesn't filter by PID since
//     sing-box uses path.
//   - IconPath is the absolute path to an icon file we can hand to Fyne
//     (NewLocalResource → Image). Empty means the picker falls back to a
//     generic file icon. Implementations may leave this empty if loading
//     icons is non-trivial on a given platform (best-effort per SPEC).
type ProcessEntry struct {
	PID         int
	Path        string
	DisplayName string
	IconPath    string
}

// ListProcesses returns a snapshot of currently running processes. The
// implementation is in proclist_{darwin,windows,linux}.go.
//
// Permissions / sandboxing:
//   - macOS: `ps -axo` returns paths only for processes the current user
//     can see. Per-user launcher runs as the user; OS-level kernel TCP
//     attribution may still miss, but those are the same processes the
//     UI couldn't list anyway.
//   - Windows: EnumProcesses + QueryFullProcessImageName. Some system
//     PIDs return ERROR_ACCESS_DENIED — we skip them.
//   - Linux: /proc/<pid>/exe symlink. Container PID namespaces may show
//     the host or guest depending on launch; we surface what /proc shows.
//
// Errors are returned only for hard failures (can't run ps, can't read
// /proc). Partial results (some PIDs unreadable) are returned as best
// effort with no error.
func ListProcesses() ([]ProcessEntry, error) {
	return listProcessesImpl()
}
