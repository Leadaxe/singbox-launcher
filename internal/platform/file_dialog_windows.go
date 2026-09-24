//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// pickOpenFileNative uses PowerShell + System.Windows.Forms.OpenFileDialog —
// the native Win32 open dialog (available on Win7+). The script is passed as
// -EncodedCommand with every value base64-encoded inside it (file_dialog_ps.go):
// the launcher runs elevated, and the caption comes from a translation file.
// Cancel → no output on stdout.
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	script := psOpenFileScript(prompt, winFilter(exts), false)
	out, err := exec.Command("powershell", psDialogArgs(script)...).Output()
	if err != nil {
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}

// pickOpenFilesNative — тот же OpenFileDialog с `Multiselect = $true`.
// Выбранное лежит в `$d.FileNames` (множественное число, отдельное свойство):
// печатаем через перевод строки, как договорено в splitPickedPaths.
func pickOpenFilesNative(prompt string, exts []string) ([]string, bool, error) {
	script := psOpenFileScript(prompt, winFilter(exts), true)
	out, err := exec.Command("powershell", psDialogArgs(script)...).Output()
	if err != nil {
		return nil, false, err
	}
	paths := splitPickedPaths(string(out))
	if len(paths) == 0 {
		return nil, false, nil
	}
	return paths, true, nil
}

// winFilter builds an OpenFileDialog.Filter string, e.g.
// "Configs (*.conf;*.vpn)|*.conf;*.vpn|All files (*.*)|*.*".
func winFilter(exts []string) string {
	if len(exts) == 0 {
		return "All files (*.*)|*.*"
	}
	pats := make([]string, len(exts))
	for i, e := range exts {
		pats[i] = "*." + e
	}
	joined := strings.Join(pats, ";")
	return fmt.Sprintf("Configs (%s)|%s|All files (*.*)|*.*", joined, joined)
}

// pickSaveFileNative uses PowerShell + SaveFileDialog. OverwritePrompt is on
// by default in WinForms, so the OS asks about overwrite itself.
func pickSaveFileNative(prompt, defaultName string) (string, bool, error) {
	script := psSaveFileScript(prompt, defaultName, "JSON (*.json)|*.json|All files (*.*)|*.*")
	out, err := exec.Command("powershell", psDialogArgs(script)...).Output()
	if err != nil {
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil // cancel
	}
	return path, true, nil
}
