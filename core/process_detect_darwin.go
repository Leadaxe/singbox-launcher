//go:build darwin

package core

import (
	"os/exec"
	"strconv"
	"strings"

	"singbox-launcher/internal/platform"
)

// findSingboxRunProcessDarwin ищет запущенный sing-box по ПОЛНОЙ командной
// строке (pgrep -f), тем же паттерном, которым privileged-путь его убивает:
// `sing-box run|start-singbox-privileged`. В отличие от скана по имени
// бинаря, не срабатывает на демон `sing-box lxd` и на `sing-box check`.
//
// Возвращает (found, pid, err); err != nil — pgrep недоступен, caller делает
// fallback на имя-ориентированный скан.
func findSingboxRunProcessDarwin() (bool, int, error) {
	out, err := exec.Command("pgrep", "-f", platform.PrivilegedPkillPattern).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, -1, nil // exit 1 = совпадений нет
		}
		return false, -1, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pid, convErr := strconv.Atoi(strings.TrimSpace(line)); convErr == nil && pid > 0 {
			return true, pid, nil
		}
	}
	return false, -1, nil
}
