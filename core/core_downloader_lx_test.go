package core

import (
	"testing"

	"singbox-launcher/internal/constants"
)

// SPEC 072: core source switched to the sing-box-lx fork, except Windows 7
// (windows/386) which has no fork asset and stays on upstream SagerNet.
func TestCoreReleaseRepoFor(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"windows", "386", constants.SingboxLegacyRepo}, // Win7 → upstream
		{"windows", "amd64", constants.SingboxCoreRepo},
		{"windows", "arm64", constants.SingboxCoreRepo},
		{"darwin", "arm64", constants.SingboxCoreRepo},
		{"darwin", "amd64", constants.SingboxCoreRepo},
		{"linux", "amd64", constants.SingboxCoreRepo},
		{"linux", "arm64", constants.SingboxCoreRepo},
	}
	for _, c := range cases {
		if got := coreReleaseRepoFor(c.goos, c.goarch); got != c.want {
			t.Errorf("coreReleaseRepoFor(%q,%q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
	if constants.SingboxCoreRepo != "Leadaxe/sing-box-lx" {
		t.Errorf("SingboxCoreRepo = %q, want the fork", constants.SingboxCoreRepo)
	}
}
