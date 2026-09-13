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

// User-cancel detection must be language-independent: it keys off AppleScript
// error code -128, not the localized message (English / Russian / …).
func TestIsAppleScriptCancel(t *testing.T) {
	cancels := []string{
		"15:45: execution error: User canceled. (-128)",
		"15:45: execution error: Отменено пользователем. (-128)",
		"execution error: 已取消。 (-128)",
	}
	for _, s := range cancels {
		if !isAppleScriptCancel([]byte(s)) {
			t.Errorf("should detect cancel: %q", s)
		}
	}
	notCancels := []string{
		"execution error: File not found. (-43)",
		"some unrelated error",
		"",
	}
	for _, s := range notCancels {
		if isAppleScriptCancel([]byte(s)) {
			t.Errorf("should NOT be cancel: %q", s)
		}
	}
}

// The panel filter takes type identifiers, not extensions: a bare "json"
// greys out every file on macOS 26 (issue #120). The script must resolve
// each extension through UTType and never hand the bare string to `of type`.
func TestChooseFileScriptResolvesExtensionsToUTI(t *testing.T) {
	s := chooseFileScript("Open", []string{"json", "vpn"}, false)
	if !strings.Contains(s, "typeWithFilenameExtension") {
		t.Fatalf("extensions are not resolved through UTType:\n%s", s)
	}
	if strings.Contains(s, `of type {"json"`) {
		t.Fatalf("bare extension handed to `of type`:\n%s", s)
	}
	if !strings.Contains(s, "of type utis") {
		t.Fatalf("resolved UTI list is not used as the filter:\n%s", s)
	}
	if got := chooseFileScript("Open", nil, true); strings.Contains(got, "of type") || !strings.Contains(got, "multiple selections allowed") {
		t.Fatalf("no-filter multi-select script is wrong:\n%s", got)
	}
}
