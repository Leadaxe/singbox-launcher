package core

import (
	"testing"

	"singbox-launcher/internal/constants"
)

// SPEC 072 (Variant A, fork v1.13.13-lx.5): the sing-box-lx fork now builds every
// platform — including Windows 7 (windows/386, the `legacy-windows-7` asset) — so
// the core source is the fork on all platforms (no upstream SagerNet split anymore).
func TestCoreReleaseRepoFor(t *testing.T) {
	cases := []struct{ goos, goarch string }{
		{"windows", "386"}, // Win7 — now the fork too
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	}
	for _, c := range cases {
		if got := coreReleaseRepoFor(c.goos, c.goarch); got != constants.SingboxCoreRepo {
			t.Errorf("coreReleaseRepoFor(%q,%q) = %q, want the fork %q", c.goos, c.goarch, got, constants.SingboxCoreRepo)
		}
	}
	if constants.SingboxCoreRepo != "Leadaxe/sing-box-lx" {
		t.Errorf("SingboxCoreRepo = %q, want the fork", constants.SingboxCoreRepo)
	}
}
