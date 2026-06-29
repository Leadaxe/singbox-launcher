// Package platform — SPEC 082: native open-file dialog via per-OS system
// commands (osascript / PowerShell / zenity|kdialog), no extra Go dependency.
package platform

import (
	"errors"
	"strings"
)

// ErrNativeDialogUnavailable means the OS has no usable native file dialog
// (e.g. Linux without zenity or kdialog). The caller should fall back to the
// in-app Fyne dialog.
var ErrNativeDialogUnavailable = errors.New("native file dialog unavailable")

// PickOpenFile shows a native open-file dialog and returns the chosen path.
//
//   - (path, true, nil)  — user picked a file
//   - ("", false, nil)   — user cancelled
//   - ("", false, ErrNativeDialogUnavailable) — no native dialog on this OS;
//     caller should fall back to the in-app dialog
//
// exts are extensions WITHOUT the dot (e.g. ["conf","vpn","txt"]); empty = any.
// prompt is the window title/prompt. Implementation is per-OS (file_dialog_*.go).
func PickOpenFile(prompt string, exts []string) (string, bool, error) {
	clean := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(strings.TrimPrefix(e, "."))
		if e != "" {
			clean = append(clean, e)
		}
	}
	return pickOpenFileNative(prompt, clean)
}
