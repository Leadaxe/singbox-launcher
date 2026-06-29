//go:build linux

package platform

import (
	"os/exec"
	"strings"
)

// pickOpenFileNative uses zenity (GTK) or, failing that, kdialog (KDE) — the
// native file choosers on the two common desktop stacks. If neither is
// installed, returns ErrNativeDialogUnavailable so the caller falls back to the
// in-app Fyne dialog. Cancel → non-zero exit with empty output.
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		return runFilePicker(zenityArgs(prompt, exts))
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		return runFilePicker(kdialogArgs(prompt, exts))
	}
	return "", false, ErrNativeDialogUnavailable
}

func runFilePicker(name string, args []string) (string, bool, error) {
	out, err := exec.Command(name, args...).Output()
	path := strings.TrimSpace(string(out))
	if err != nil {
		// Both tools exit non-zero on cancel; with no path that's a cancel.
		if _, ok := err.(*exec.ExitError); ok && path == "" {
			return "", false, nil
		}
		return "", false, err
	}
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}

func zenityArgs(prompt string, exts []string) (string, []string) {
	args := []string{"--file-selection"}
	if strings.TrimSpace(prompt) != "" {
		args = append(args, "--title="+prompt)
	}
	if len(exts) > 0 {
		pats := make([]string, len(exts))
		for i, e := range exts {
			pats[i] = "*." + e
		}
		args = append(args, "--file-filter=Configs | "+strings.Join(pats, " "))
		args = append(args, "--file-filter=All files | *")
	}
	return "zenity", args
}

func kdialogArgs(prompt string, exts []string) (string, []string) {
	// kdialog --getopenfilename <startdir> "<patterns>|<label>"
	args := []string{"--getopenfilename", "."}
	if len(exts) > 0 {
		pats := make([]string, len(exts))
		for i, e := range exts {
			pats[i] = "*." + e
		}
		args = append(args, strings.Join(pats, " ")+"|Configs")
	}
	if strings.TrimSpace(prompt) != "" {
		args = append(args, "--title", prompt)
	}
	return "kdialog", args
}
