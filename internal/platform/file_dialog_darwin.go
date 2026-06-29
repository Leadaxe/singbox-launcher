//go:build darwin

package platform

import (
	"os/exec"
	"strings"
)

// pickOpenFileNative uses AppleScript `choose file` — the native Finder open
// panel. `of type {…}` filters by extension; cancel surfaces as a non-zero exit
// with "User canceled" on stderr (→ treated as cancel, not an error).
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	var b strings.Builder
	b.WriteString("POSIX path of (choose file")
	if len(exts) > 0 {
		quoted := make([]string, len(exts))
		for i, e := range exts {
			quoted[i] = appleScriptStringLiteral(e)
		}
		b.WriteString(" of type {")
		b.WriteString(strings.Join(quoted, ", "))
		b.WriteString("}")
	}
	if strings.TrimSpace(prompt) != "" {
		b.WriteString(" with prompt ")
		b.WriteString(appleScriptStringLiteral(prompt))
	}
	b.WriteString(")")

	out, err := exec.Command("osascript", "-e", b.String()).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			// User cancelled the dialog → not an error.
			if strings.Contains(string(ee.Stderr), "User canceled") {
				return "", false, nil
			}
		}
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}

// appleScriptStringLiteral wraps s as an AppleScript string literal, escaping
// backslashes and quotes so a prompt with quotes can't break the script.
func appleScriptStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
