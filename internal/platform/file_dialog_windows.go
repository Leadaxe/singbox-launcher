//go:build windows

package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// pickOpenFileNative uses PowerShell + System.Windows.Forms.OpenFileDialog —
// the native Win32 open dialog (available on Win7+). `-STA` is required for
// WinForms dialogs. Cancel → no output on stdout.
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	filter := winFilter(exts)
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms;`+
			`$d = New-Object System.Windows.Forms.OpenFileDialog;`+
			`$d.Title = %s;`+
			`$d.Filter = %s;`+
			`if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }`,
		psSingleQuote(prompt), psSingleQuote(filter),
	)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", ps).Output()
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
	filter := winFilter(exts)
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms;`+
			`$d = New-Object System.Windows.Forms.OpenFileDialog;`+
			`$d.Title = %s;`+
			`$d.Filter = %s;`+
			`$d.Multiselect = $true;`+
			`if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileNames | ForEach-Object { [Console]::Out.WriteLine($_) } }`,
		psSingleQuote(prompt), psSingleQuote(filter),
	)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", ps).Output()
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

// psSingleQuote wraps s as a PowerShell single-quoted literal (only `'` needs
// escaping, doubled), so a prompt/filter can't inject PowerShell.
func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// pickSaveFileNative uses PowerShell + SaveFileDialog. OverwritePrompt is on
// by default in WinForms, so the OS asks about overwrite itself.
func pickSaveFileNative(prompt, defaultName string) (string, bool, error) {
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName System.Windows.Forms;`+
			`$d = New-Object System.Windows.Forms.SaveFileDialog;`+
			`$d.Title = %s;`+
			`$d.FileName = %s;`+
			`$d.Filter = %s;`+
			`if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }`,
		psSingleQuote(prompt), psSingleQuote(defaultName), psSingleQuote("JSON (*.json)|*.json|All files (*.*)|*.*"),
	)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", ps).Output()
	if err != nil {
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil // cancel
	}
	return path, true, nil
}
