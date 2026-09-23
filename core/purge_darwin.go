//go:build darwin

package core

import (
	"fmt"
	"os"
)

// daemonUninstallHint — DaemonUninstallCommand(true), если служба
// установлена; иначе "".
func (ac *AppController) daemonUninstallHint() string {
	if _, err := os.Stat(daemonSystemPlistPath()); err != nil {
		return ""
	}
	return ac.DaemonUninstallCommand(true)
}

// daemonUninstallHintFor — то же без контроллера (флаг -purge-data): путь
// ядра считается цепочкой §3.3 заново. Формат команды — как у
// DaemonUninstallCommand(true).
func daemonUninstallHintFor(corePath string) string {
	if _, err := os.Stat(daemonSystemPlistPath()); err != nil {
		return ""
	}
	if corePath == "" {
		corePath = "sing-box"
	}
	return fmt.Sprintf("sudo %s lxd --service=uninstall --purge", shellQuote(corePath))
}
