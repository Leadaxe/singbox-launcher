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
	re := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !re.MatchString(RequiredCoreVersion) {
		t.Fatalf("RequiredCoreVersion = %q, want X.Y.Z", RequiredCoreVersion)
	}
}

func TestRequiredTemplateRef_LooksLikeGitSHA(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	if !re.MatchString(RequiredTemplateRef) {
		t.Fatalf("RequiredTemplateRef = %q, want a 40- or 64-char lowercase hex SHA", RequiredTemplateRef)
	}
}
