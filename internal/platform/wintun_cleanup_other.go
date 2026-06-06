//go:build !windows
// +build !windows

// Package platform — wintun_cleanup_other.go is the non-Windows stub for
// SPEC 065. The phantom-adapter problem is Windows-7-specific; on macOS and
// Linux there is nothing to clean.
//
// Lives here (not behind a runtime.GOOS check) so the Windows-only SetupAPI
// bindings in wintun_cleanup_windows.go never link into non-Windows builds.
package platform

// CleanupGhostSingboxTunAdapters — no-op on non-Windows platforms.
//
// Mirrors the Windows signature so callers can invoke it unconditionally
// without runtime.GOOS branching.
func CleanupGhostSingboxTunAdapters() (removed int, err error) {
	return 0, nil
}
