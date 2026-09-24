package platform

import (
	"errors"
	"strings"
)

// Автозапуск при входе в Windows (SPEC 139 §8): значение singbox-launcher в
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run. Реестр —
// autostart_windows.go; здесь формат строки, общий для всех ОС.

// AutostartValueName — имя значения в ключе Run.
const AutostartValueName = "singbox-launcher"

// AutostartLocation — где лежит значение: для плана очистки и логов.
const AutostartLocation = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\` + AutostartValueName

// ErrAutostartNotSupported — автозапуск через Run есть только на Windows.
var ErrAutostartNotSupported = errors.New("autostart is not supported on this platform")

// AutostartEntry — разобранное значение автозапуска.
type AutostartEntry struct {
	Exe   string // путь к исполняемому файлу (в значении — в кавычках)
	Start bool   // с -start: подключать VPN при входе
}

// Command — строка значения: "<exe>" -tray, со Start — "<exe>" -tray -start.
func (e AutostartEntry) Command() string {
	s := `"` + e.Exe + `" -tray`
	if e.Start {
		s += " -start"
	}
	return s
}

// ParseAutostartCommand разбирает строку значения: путь в кавычках (или до
// первого пробела, если кавычек нет — так значение могли записать руками) и
// флаг -start среди аргументов.
func ParseAutostartCommand(cmd string) AutostartEntry {
	cmd = strings.TrimSpace(cmd)
	var exe, rest string
	if strings.HasPrefix(cmd, `"`) {
		if end := strings.Index(cmd[1:], `"`); end >= 0 {
			exe, rest = cmd[1:1+end], cmd[2+end:]
		} else {
			exe = cmd[1:]
		}
	} else if sp := strings.IndexByte(cmd, ' '); sp >= 0 {
		exe, rest = cmd[:sp], cmd[sp+1:]
	} else {
		exe = cmd
	}
	e := AutostartEntry{Exe: exe}
	for _, arg := range strings.Fields(rest) {
		if arg == "-start" || arg == "--start" || arg == "-start=true" {
			e.Start = true
		}
	}
	return e
}
