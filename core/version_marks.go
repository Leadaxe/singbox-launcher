// File version_marks.go — отметки «какой лаунчер и какое ядро мы видели в
// прошлый раз» и подъём событий смены (SPEC 132).
//
// Отметки живут в settings.json рядом с LastTemplateLauncherVersion — тем же
// приёмом, что и она, но СВОИ: та отвечает за свежесть шаблона и трогать её
// чужой логикой нельзя.
//
// Работ по этим событиям сегодня нет (core/maintenance — пустая закладка);
// смысл вызова — сделать смену версии видимой в логе релиза, который пишет
// только WARN.
package core

import (
	"strings"

	"singbox-launcher/core/maintenance"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// CheckVersionMarks сверяет текущие версии лаунчера и ядра с отметками и
// поднимает события смены.
//
// Зовётся на старте лаунчера ВНЕ UI-потока: читает settings.json и запускает
// `sing-box version`.
//
// Отметки нет → записывается текущая версия БЕЗ события: прошлой версии мы не
// знаем, и объявлять смену не на чем.
func (ac *AppController) CheckVersionMarks() {
	if ac == nil || ac.FileService == nil {
		return
	}
	binDir := ac.FileService.Layout.Data.Bin()
	settings := locale.LoadSettings(binDir)
	changed := false

	if mark, ok := checkLauncherMark(settings.LastLauncherVersion); ok {
		settings.LastLauncherVersion = mark
		changed = true
	}
	if mark, ok := ac.checkCoreMark(settings.LastCoreVersion); ok {
		settings.LastCoreVersion = mark
		changed = true
	}
	if !changed {
		return
	}
	if err := locale.SaveSettings(binDir, settings); err != nil {
		// Отметка не записалась — событие поднимется снова на следующем
		// старте. Работы обязаны это переживать (они идемпотентны).
		debuglog.WarnLog("version marks: settings.json not saved: %v", err)
	}
}

// checkLauncherMark — вернуть новую отметку лаунчера, если её надо записать.
//
// Dev-сборки (`v-local-test`, `unnamed-dev`) отметку обновляют, но события НЕ
// поднимают: со стабильной версией они не сравниваются осмысленно, и событие
// срабатывало бы на каждом переключении между сборками.
func checkLauncherMark(last string) (string, bool) {
	current := strings.TrimSpace(constants.AppVersion)
	last = strings.TrimSpace(last)
	if current == "" || last == current {
		return "", false
	}
	if last == "" {
		return current, true // первая отметка, без события
	}
	if isDevAppVersion(current) || isDevAppVersion(last) {
		return current, true
	}
	if err := maintenance.OnLauncherVersionChanged(last, current); err != nil {
		// Работа упала — отметку НЕ двигаем: событие повторится.
		debuglog.WarnLog("version marks: launcher maintenance failed: %v", err)
		return "", false
	}
	return current, true
}

// checkCoreMark — то же для ядра. Версию берём у самого бинаря.
//
// Версии нет (бинаря нет, вывод не разобрался) — не трогаем отметку ВООБЩЕ:
// иначе установка ядра позже выглядела бы как «версия не менялась».
func (ac *AppController) checkCoreMark(last string) (string, bool) {
	current, err := ac.GetInstalledCoreVersion()
	current = strings.TrimSpace(current)
	if err != nil || current == "" {
		return "", false
	}
	last = strings.TrimSpace(last)
	if last == current {
		return "", false
	}
	if last == "" {
		return current, true // первая отметка, без события
	}
	if merr := maintenance.OnCoreVersionChanged(last, current); merr != nil {
		debuglog.WarnLog("version marks: core maintenance failed: %v", merr)
		return "", false
	}
	return current, true
}
