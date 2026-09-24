package platform

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// PowerShell-скрипты файловых диалогов Windows (file_dialog_windows.go).
//
// Лаунчер на Windows может работать с правами администратора (перезапуск с
// повышением ради TUN, SPEC 139), а подпись диалога приходит из перевода
// (ru.json в каталоге данных — его может переписать процесс обычной
// целостности). Поэтому значения в код PowerShell строками не вставляются
// вовсе: скрипт целиком собирается здесь, а каждое значение —
// base64 от UTF-8, которое раскодирует сам скрипт. Из одинарных кавычек
// алфавит base64 ([A-Za-z0-9+/=]) не выйдет, какие бы кавычки (в том числе
// ‘ ’ ‚ ‛, которые PowerShell тоже считает кавычками), `$(…)` или
// обратные апострофы ни были в подписи. Скрипт уходит в powershell как
// -EncodedCommand (base64 от UTF-16LE) — без квотинга в командной строке.
//
// Файл без build-тега: тест кодирования идёт на любой платформе.

// psValue — выражение PowerShell, дающее строку s: base64 от UTF-8,
// раскодированный в скрипте.
func psValue(s string) string {
	return "[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" +
		base64.StdEncoding.EncodeToString([]byte(s)) + "'))"
}

// psEncodedCommand — значение -EncodedCommand: base64 от UTF-16LE скрипта.
func psEncodedCommand(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(raw[2*i:], u)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// psDialogArgs — аргументы powershell для скрипта диалога. -STA нужен
// WinForms-диалогам.
func psDialogArgs(script string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-STA", "-EncodedCommand", psEncodedCommand(script)}
}

// psOpenFileScript — OpenFileDialog: выбранный путь (или пути через перевод
// строки при multiple) в stdout; отмена — пусто.
func psOpenFileScript(prompt, filter string, multiple bool) string {
	lines := []string{
		"$caption = " + psValue(prompt),
		"$filter = " + psValue(filter),
		"Add-Type -AssemblyName System.Windows.Forms",
		"$d = New-Object System.Windows.Forms.OpenFileDialog",
		"$d.Title = $caption",
		"$d.Filter = $filter",
	}
	if multiple {
		lines = append(lines,
			"$d.Multiselect = $true",
			"if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileNames | ForEach-Object { [Console]::Out.WriteLine($_) } }")
	} else {
		lines = append(lines,
			"if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }")
	}
	return strings.Join(lines, "\n")
}

// psSaveFileScript — SaveFileDialog (о перезаписи спрашивает сам): путь в
// stdout; отмена — пусто.
func psSaveFileScript(prompt, defaultName, filter string) string {
	return strings.Join([]string{
		"$caption = " + psValue(prompt),
		"$name = " + psValue(defaultName),
		"$filter = " + psValue(filter),
		"Add-Type -AssemblyName System.Windows.Forms",
		"$d = New-Object System.Windows.Forms.SaveFileDialog",
		"$d.Title = $caption",
		"$d.FileName = $name",
		"$d.Filter = $filter",
		"if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }",
	}, "\n")
}
