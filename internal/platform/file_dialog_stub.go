//go:build !darwin && !windows && !linux

package platform

// pickOpenFileNative — no native dialog on this OS; caller falls back to the
// in-app Fyne dialog.
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	return "", false, ErrNativeDialogUnavailable
}

// pickOpenFilesNative — no native dialog on this OS.
func pickOpenFilesNative(prompt string, exts []string) ([]string, bool, error) {
	return nil, false, ErrNativeDialogUnavailable
}

// pickSaveFileNative — no native dialog on this OS.
func pickSaveFileNative(prompt, defaultName string) (string, bool, error) {
	return "", false, ErrNativeDialogUnavailable
}
