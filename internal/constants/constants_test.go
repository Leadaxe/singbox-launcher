package constants

import (
	"regexp"
	"testing"
)

// Both pinned-dependency refs are static invariants of the build. If they ever
// drift to invalid shapes (e.g. someone clears RequiredCoreVersion or pastes
// a branch name into RequiredTemplateRef) we want a test to catch it before
// download paths break at runtime.
//
// RequiredTemplateRef is a Git object hash: 40-hex SHA-1 (or 64-hex SHA-256
// if the repo is upgraded). Anything else means the ldflags injection or the
// source-default went sideways.

func TestRequiredCoreVersion_SemVerShape(t *testing.T) {
	// SPEC 072: core is the sing-box-lx fork. Tags evolved from a single
	// `-lx.N` suffix (1.13.13-lx.6) to dot-separated prerelease chains
	// (1.14.0-lx.1-rc.16). Accept the general SemVer shape — base X.Y.Z plus
	// zero or more `-<ident>` suffixes (alnum, dot-separated) — so future tags
	// like `-lx.2-rc.1` or a stable `-lx.1` pass without another regex bump,
	// while branch names / empty / a leading `v` / dangling dashes still fail.
	re := regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)*$`)
	if !re.MatchString(RequiredCoreVersion) {
		t.Fatalf("RequiredCoreVersion = %q, want X.Y.Z optionally followed by -<ident> suffixes", RequiredCoreVersion)
	}
}

func TestRequiredTemplateRef_LooksLikeGitSHA(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	if !re.MatchString(RequiredTemplateRef) {
		t.Fatalf("RequiredTemplateRef = %q, want a 40- or 64-char lowercase hex SHA", RequiredTemplateRef)
	}
}
