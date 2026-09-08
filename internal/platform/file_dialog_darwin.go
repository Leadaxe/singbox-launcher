//go:build darwin

package platform

import (
	"os/exec"
	"strings"
)

// pickOpenFileNative uses AppleScript `choose file` — the native Finder open
// panel. Cancel surfaces as a non-zero exit with "User canceled" on stderr
// (→ treated as cancel, not an error).
func pickOpenFileNative(prompt string, exts []string) (string, bool, error) {
	out, err := exec.Command("osascript", "-e", chooseFileScript(prompt, exts, false)).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && isAppleScriptCancel(ee.Stderr) {
			return "", false, nil
		}
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}

// pickOpenFilesNative — та же панель Finder с `with multiple selections
// allowed`. AppleScript возвращает СПИСОК алиасов, и `POSIX path of` к списку
// не применяется: путь берётся поэлементно циклом, а строки склеиваются через
// перевод строки — единственный разделитель, которого не может быть внутри
// POSIX-пути (в отличие от запятой и пробела).
func pickOpenFilesNative(prompt string, exts []string) ([]string, bool, error) {
	out, err := exec.Command("osascript", "-e", chooseFileScript(prompt, exts, true)).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && isAppleScriptCancel(ee.Stderr) {
			return nil, false, nil
		}
		return nil, false, err
	}
	paths := splitPickedPaths(string(out))
	if len(paths) == 0 {
		return nil, false, nil
	}
	return paths, true, nil
}

// chooseFileScript — текст AppleScript для панели открытия.
//
// Фильтр `of type {…}` принимает идентификаторы типов (UTI), а не расширения.
// Голое расширение (`{"json"}`) старые системы молча маппили в тип сами; на
// macOS 26 маппинг исчез, и панель серила ВСЕ файлы — импорт бэкапа и «Add
// from file» становились невозможны (issue #120). Поэтому расширение
// резолвится в UTI прямо в скрипте через UTType (macOS 11+, наш минимум):
// `json` → `public.json`, `txt` → `public.plain-text`, а у расширений без
// своего типа (`vpn`, `conf`) получается динамический `dyn.…`, который панель
// принимает наравне с публичными. Если ни одно расширение не резолвится,
// панель открывается без фильтра: показать всё лучше, чем не показать ничего.
func chooseFileScript(prompt string, exts []string, multiple bool) string {
	var b strings.Builder
	b.WriteString("use framework \"Foundation\"\n")
	b.WriteString("use framework \"UniformTypeIdentifiers\"\n")
	b.WriteString("use scripting additions\n")

	choose := "choose file"
	if strings.TrimSpace(prompt) != "" {
		choose += " with prompt " + appleScriptStringLiteral(prompt)
	}
	suffix := ""
	if multiple {
		suffix = " with multiple selections allowed"
	}

	if len(exts) > 0 {
		quoted := make([]string, len(exts))
		for i, e := range exts {
			quoted[i] = appleScriptStringLiteral(e)
		}
		b.WriteString("set utis to {}\n")
		b.WriteString("repeat with e in {" + strings.Join(quoted, ", ") + "}\n")
		b.WriteString("set t to current application's UTType's typeWithFilenameExtension:(e as text)\n")
		b.WriteString("if t is not missing value then set end of utis to (t's identifier() as text)\n")
		b.WriteString("end repeat\n")
		b.WriteString("if (count of utis) > 0 then\n")
		b.WriteString("set picked to " + choose + " of type utis" + suffix + "\n")
		b.WriteString("else\n")
		b.WriteString("set picked to " + choose + suffix + "\n")
		b.WriteString("end if\n")
	} else {
		b.WriteString("set picked to " + choose + suffix + "\n")
	}

	if !multiple {
		b.WriteString("return POSIX path of picked")
		return b.String()
	}
	b.WriteString("set out to \"\"\n")
	b.WriteString("repeat with f in picked\n")
	b.WriteString("set out to out & (POSIX path of f) & linefeed\n")
	b.WriteString("end repeat\n")
	b.WriteString("return out")
	return b.String()
}

// isAppleScriptCancel reports whether osascript stderr is a user-cancel. The
// cancel is AppleScript error -128 (errAECanceled); the message is localized
// ("User canceled" / "Отменено пользователем." / …) so we match the numeric
// code, which is stable across system languages.
func isAppleScriptCancel(stderr []byte) bool {
	return strings.Contains(string(stderr), "-128")
}

// appleScriptStringLiteral wraps s as an AppleScript string literal, escaping
// backslashes and quotes so a prompt with quotes can't break the script.
func appleScriptStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// pickSaveFileNative uses AppleScript `choose file name` — the native Finder
// save panel (it asks about overwrite itself).
func pickSaveFileNative(prompt, defaultName string) (string, bool, error) {
	var b strings.Builder
	b.WriteString("POSIX path of (choose file name")
	if strings.TrimSpace(prompt) != "" {
		b.WriteString(" with prompt ")
		b.WriteString(appleScriptStringLiteral(prompt))
	}
	if defaultName != "" {
		b.WriteString(" default name ")
		b.WriteString(appleScriptStringLiteral(defaultName))
	}
	b.WriteString(")")

	out, err := exec.Command("osascript", "-e", b.String()).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && isAppleScriptCancel(ee.Stderr) {
			return "", false, nil
		}
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}
