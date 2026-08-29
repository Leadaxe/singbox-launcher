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

// pickOpenFilesNative — те же zenity/kdialog с мультивыбором.
//
// Оба инструмента отдают выбранное ОДНОЙ строкой со своим разделителем:
// zenity — заданным через `--separator`, kdialog — пробелами. Пробел в пути
// возможен, поэтому zenity просим разделять переводом строки; у kdialog такой
// опции нет, и мультивыбор его ветки заведомо ломается на путях с пробелами —
// вместо тихой порчи он остаётся одиночным (лучше один правильный файл, чем
// три битых пути).
func pickOpenFilesNative(prompt string, exts []string) ([]string, bool, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		name, args := zenityArgs(prompt, exts)
		args = append(args, "--multiple", "--separator=\n")
		out, rerr := exec.Command(name, args...).Output()
		text := strings.TrimSpace(string(out))
		if rerr != nil {
			if _, ok := rerr.(*exec.ExitError); ok && text == "" {
				return nil, false, nil // cancel
			}
			return nil, false, rerr
		}
		paths := splitPickedPaths(text)
		if len(paths) == 0 {
			return nil, false, nil
		}
		return paths, true, nil
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		path, ok, kerr := runFilePicker(kdialogArgs(prompt, exts))
		if kerr != nil || !ok {
			return nil, ok, kerr
		}
		return []string{path}, true, nil
	}
	return nil, false, ErrNativeDialogUnavailable
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

// pickSaveFileNative — сохранение через zenity/kdialog.
//
// Подтверждение перезаписи просим у самого диалога (--confirm-overwrite у
// zenity; kdialog спрашивает всегда): дублировать этот вопрос в приложении
// значит заставить пользователя отвечать дважды.
func pickSaveFileNative(prompt, defaultName string) (string, bool, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--save", "--confirm-overwrite"}
		if strings.TrimSpace(prompt) != "" {
			args = append(args, "--title="+prompt)
		}
		if defaultName != "" {
			args = append(args, "--filename="+defaultName)
		}
		return runFilePicker("zenity", args)
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		name := defaultName
		if name == "" {
			name = "."
		}
		args := []string{"--getsavefilename", name}
		if strings.TrimSpace(prompt) != "" {
			args = append(args, "--title", prompt)
		}
		return runFilePicker("kdialog", args)
	}
	return "", false, ErrNativeDialogUnavailable
}
