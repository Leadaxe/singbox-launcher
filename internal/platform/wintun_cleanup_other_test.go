//go:build !windows
// +build !windows

package platform

import "testing"

// TestCleanupGhostSingboxTunAdapters_NoopOnNonWindows verifies that the
// non-Windows stub returns cleanly without side-effects. This is a
// regression test for SPEC 065 §Out of scope: "не-Windows платформы
// (полный no-op на macOS/Linux)".
func TestCleanupGhostSingboxTunAdapters_NoopOnNonWindows(t *testing.T) {
	removed, err := CleanupGhostSingboxTunAdapters()
	if err != nil {
		t.Fatalf("expected nil error on non-Windows, got %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected removed=0 on non-Windows, got %d", removed)
	}
}
