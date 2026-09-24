//go:build darwin

package core

// daemonUninstallHint — команда удаления службы, если она установлена;
// иначе "". survivesPurge — команда идёт через root-owned копию службы
// (SPEC 136), которая лежит вне DataDir и переживает удаление данных.
func (ac *AppController) daemonUninstallHint() (command string, survivesPurge bool) {
	return daemonUninstallHintFor(ac.FileService.SingboxPath)
}

// daemonUninstallHintFor — то же без контроллера (флаг -purge-data): путь
// ядра считается цепочкой SPEC 135 §3.3 заново. Бинарь — по тому же
// правилу, что DaemonUninstallCommand: копия службы, если она безопасна,
// иначе ядро лаунчера (его удаление данных уносит — отсюда «сначала»).
// Без `--keep-copy`: копия уходит вместе со службой и данными.
func daemonUninstallHintFor(corePath string) (command string, survivesPurge bool) {
	l := systemDaemonServiceLayout()
	check := inspectDaemonServiceDefinition(l)
	switch {
	case check.State == DaemonServiceNotInstalled:
		return "", false
	case check.CopyUsable():
		return daemonUninstallCommandFor(l.CorePath, true, false), true
	}
	if corePath == "" {
		corePath = "sing-box"
	}
	return daemonUninstallCommandFor(corePath, true, false), false
}
