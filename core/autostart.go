package core

// Автозапуск при входе в Windows (SPEC 139 §8): значение singbox-launcher в
// HKCU\…\Run — "<exe>" -tray, со вторым чекбоксом "<exe>" -tray -start.
// -start отдельно: с TUN без прав он на каждом входе открывал бы окно с
// диалогом §4; в proxy-only это и есть «VPN при входе». Один код для
// Settings, флага -autostart (установщик, SPEC 140 §3.3) и очистки данных.

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// AutostartState — состояние чекбоксов автозапуска в Settings.
type AutostartState struct {
	Supported bool   // только Windows
	Enabled   bool   // значение есть и указывает на этот exe
	Start     bool   // с -start: подключать VPN при входе
	OtherExe  string // значение указывает на другую копию лаунчера
	// Locked — повышенный экземпляр: значение пользовательское (HKCU), а
	// повышение могло пойти под другой учётной записью — менять нельзя.
	Locked bool
}

// AutostartState читает значение автозапуска (при открытии Settings).
// StartupApproved (отключение в Диспетчере задач) не учитывается.
func (ac *AppController) AutostartState() AutostartState {
	st := AutostartState{Supported: runtime.GOOS == "windows"}
	if !st.Supported {
		return st
	}
	st.Locked = platform.IsElevated()
	exe, err := paths.Executable()
	if err != nil {
		debuglog.WarnLog("autostart: executable path: %v", err)
		return st
	}
	e, found, err := platform.ReadAutostart()
	if err != nil {
		debuglog.WarnLog("autostart: %v", err)
		return st
	}
	if !found {
		return st
	}
	if sameExePath(e.Exe, exe) {
		st.Enabled, st.Start = true, e.Start
	} else {
		st.OtherExe = e.Exe
	}
	return st
}

// SetAutostart — чекбоксы Start with Windows / Connect VPN at sign-in:
// enabled — записать значение на этот exe (чужое перезаписывается), иначе
// удалить своё.
func (ac *AppController) SetAutostart(enabled, start bool) error {
	exe, err := paths.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	if enabled {
		return writeAutostart(exe, start)
	}
	_, err = removeOwnAutostart(exe)
	return err
}

// AutostartOwned — значение автозапуска указывает на этот exe: пункт
// очистки данных (Remove all data) показывается только тогда.
func (ac *AppController) AutostartOwned() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	exe, err := paths.Executable()
	return err == nil && autostartOwned(exe)
}

// AutostartCLI — флаг -autostart=on|off (SPEC 139 §8, для установщика
// SPEC 140): on пишет "<exe>" -tray, off удаляет значение, только если оно
// указывает на этот exe. Итог — строка в out; возвращает код выхода 0/1.
func AutostartCLI(mode, exe string, out io.Writer) int {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "on":
		if err := writeAutostart(exe, false); err != nil {
			fmt.Fprintf(out, "Autostart: failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "Autostart: on (%s = %s)\n", platform.AutostartLocation, platform.AutostartEntry{Exe: exe}.Command())
		return 0
	case "off":
		removed, err := removeOwnAutostart(exe)
		if err != nil {
			fmt.Fprintf(out, "Autostart: failed: %v\n", err)
			return 1
		}
		if removed {
			fmt.Fprintf(out, "Autostart: off (%s removed)\n", platform.AutostartLocation)
		} else {
			fmt.Fprintln(out, "Autostart: off (no entry for this program)")
		}
		return 0
	}
	fmt.Fprintf(out, "Autostart: unknown value %q, use -%s=on or -%s=off\n", mode, AutostartFlagName, AutostartFlagName)
	return 1
}

// writeAutostart записывает значение автозапуска на exe.
func writeAutostart(exe string, start bool) error {
	e := platform.AutostartEntry{Exe: exe, Start: start}
	debuglog.InfoLog("autostart: writing %s = %s", platform.AutostartLocation, e.Command())
	if err := platform.WriteAutostart(e); err != nil {
		debuglog.WarnLog("autostart: %v", err)
		return err
	}
	debuglog.InfoLog("autostart: on (start=%v)", start)
	return nil
}

// autostartOwned — значение есть и указывает на exe.
func autostartOwned(exe string) bool {
	e, found, err := platform.ReadAutostart()
	return err == nil && found && sameExePath(e.Exe, exe)
}

// removeOwnAutostart удаляет значение автозапуска, только если оно указывает
// на exe: другую копию лаунчера (перенесённую папку) не трогаем.
func removeOwnAutostart(exe string) (removed bool, err error) {
	e, found, err := platform.ReadAutostart()
	if err != nil {
		debuglog.WarnLog("autostart: %v", err)
		return false, err
	}
	if !found || !sameExePath(e.Exe, exe) {
		return false, nil
	}
	if err := platform.DeleteAutostart(); err != nil {
		debuglog.WarnLog("autostart: %v", err)
		return false, err
	}
	debuglog.InfoLog("autostart: off (%s removed)", platform.AutostartLocation)
	return true, nil
}

// removeAutostartText — строка итога удаления значения автозапуска для
// stdout очистки данных (без перевода, как весь вывод CLI).
func removeAutostartText(exe string) string {
	removed, err := removeOwnAutostart(exe)
	switch {
	case err != nil:
		return "Failed: " + platform.AutostartLocation + ": " + err.Error() + "\n"
	case removed:
		return "Removed: " + platform.AutostartLocation + "\n"
	}
	return ""
}

// sameExePath — один и тот же исполняемый файл: пути Windows без учёта
// регистра.
func sameExePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
