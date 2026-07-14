//go:build darwin

package platform

import "testing"

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
