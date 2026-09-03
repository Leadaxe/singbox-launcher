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

// PickOpenFiles shows a native open-file dialog with MULTI-SELECT and returns
// the chosen paths (SPEC 116 этап 3, W6: «импорт мультивыбором»).
//
// Same contract as PickOpenFile, only plural:
//
//   - (paths, true, nil) — user picked one or more files
//   - (nil, false, nil)  — user cancelled
//   - (nil, false, ErrNativeDialogUnavailable) — no native dialog on this OS
//
// A separate entry point rather than a flag on PickOpenFile: every existing
// caller wants exactly one file and would have to unpack a slice for nothing,
// and the single-file panels differ per OS in more than one switch (AppleScript
// returns a list, PowerShell a different property, zenity needs a separator).
func PickOpenFiles(prompt string, exts []string) ([]string, bool, error) {
	clean := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(strings.TrimPrefix(e, "."))
		if e != "" {
			clean = append(clean, e)
		}
	}
	return pickOpenFilesNative(prompt, clean)
}

// splitPickedPaths splits a multi-select dialog's output into paths.
//
// One newline-separated list is the common shape across all three backends:
// every per-OS implementation is told to emit newlines precisely because a
// newline is the one byte that cannot appear inside a picked path on any of
// the platforms we ship. Empty lines are dropped — trailing separators are
// normal for all three tools.
func splitPickedPaths(out string) []string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// PickSaveFile shows a native save-file dialog and returns the chosen path.
//
// Same contract as PickOpenFile: (path, true, nil) on choice, ("", false, nil)
// on cancel, ErrNativeDialogUnavailable where the OS has no usable dialog.
//
// defaultName is the pre-filled file name (e.g. "lx-backup-2026-08-22.json").
// Overwrite confirmation is the OS panel's job — every native save panel asks,
// and duplicating that question in-app would make the user answer twice.
func PickSaveFile(prompt, defaultName string) (string, bool, error) {
	return pickSaveFileNative(prompt, strings.TrimSpace(defaultName))
}
