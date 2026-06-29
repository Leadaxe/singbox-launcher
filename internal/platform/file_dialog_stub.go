//go:build !darwin && !windows && !linux

package platform

// pickOpenFileNative — no native dialog on this OS; caller falls back to the
// in-app Fyne dialog.
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	return "", false, ErrNativeDialogUnavailable
}
