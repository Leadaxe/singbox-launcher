//go:build darwin || linux

package platform

import (
	"strings"
	"testing"
)

// TestListProcesses_Smoke just checks ListProcesses returns *something*
// without panicking. We can't assert specific entries — CI runs in
// constrained sandboxes that may show only 1-2 processes.
func TestListProcesses_Smoke(t *testing.T) {
	entries, err := ListProcesses()
	if err != nil {
		t.Fatalf("ListProcesses err: %v", err)
	}
	if len(entries) == 0 {
		t.Skip("no processes visible — likely sandboxed CI")
	}
	for _, e := range entries[:min(3, len(entries))] {
		if e.Path == "" {
			t.Errorf("empty Path: %+v", e)
		}
		if e.DisplayName == "" {
			t.Errorf("empty DisplayName: %+v", e)
		}
		if !strings.HasPrefix(e.Path, "/") {
			t.Errorf("Path not absolute: %+v", e)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
