package core

import (
	"os"
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

func TestParseSHA256SUMS(t *testing.T) {
	in := "" +
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  sing-box-1.13.13-lx.3-darwin-arm64.tar.gz\n" +
		"DEADBEEF00000000000000000000000000000000000000000000000000000000 *sing-box-1.13.13-lx.3-windows-amd64.zip\n" +
		"\n" +
		"garbage line without two fields and more\n"
	m := parseSHA256SUMS(in)
	if got := m["sing-box-1.13.13-lx.3-darwin-arm64.tar.gz"]; got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("darwin entry = %q", got)
	}
	// leading '*' stripped, hash lowercased
	if got := m["sing-box-1.13.13-lx.3-windows-amd64.zip"]; got != "deadbeef00000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("windows entry = %q", got)
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 valid entries, got %d: %v", len(m), m)
	}
}

func TestVerifyChecksum(t *testing.T) {
	// sha256("hello\n") = 5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03
	dir := t.TempDir()
	p := dir + "/blob"
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const want = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"

	if err := verifyChecksum(p, "blob", map[string]string{"blob": want}); err != nil {
		t.Fatalf("match should pass: %v", err)
	}
	if err := verifyChecksum(p, "blob", map[string]string{"blob": "0000"}); err == nil {
		t.Fatal("mismatch should fail")
	}
	if err := verifyChecksum(p, "blob", map[string]string{"other": want}); err == nil {
		t.Fatal("missing entry should fail")
	}
}
