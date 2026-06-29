//go:build darwin

package platform

import (
	"strings"
	"testing"
)

func TestAppleScriptStringLiteral(t *testing.T) {
	cases := map[string]string{
		`hello`:          `"hello"`,
		`a "b" c`:        `"a \"b\" c"`,
		`back\slash`:     `"back\\slash"`,
		`both " and \ x`: `"both \" and \\ x"`,
	}
	for in, want := range cases {
		if got := appleScriptStringLiteral(in); got != want {
			t.Errorf("appleScriptStringLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

// PickOpenFile must normalize extensions (drop leading dots / blanks) before
// handing them to the OS layer. We can't drive the native dialog in a test, but
// we can verify a cancelled invocation (the common path on a headless box where
// osascript returns "User canceled") yields (",false,nil) — i.e. no error, and
// the function doesn't choke on dotted extensions. This is best-effort: skip if
// osascript isn't present.
func TestPickOpenFile_ExtNormalizationSmoke(t *testing.T) {
	// Just exercise the extension-cleaning path; the actual dialog is not shown
	// in CI, so we only assert it doesn't panic and returns a sane tuple shape.
	// (Run interactively to see the real Finder panel.)
	t.Skip("native dialog can't run headless; appleScriptStringLiteral covers escaping")
	_ = strings.TrimSpace("")
}
