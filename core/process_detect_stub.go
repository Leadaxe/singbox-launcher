//go:build !darwin

package core

import (
	"errors"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"singbox-launcher/internal/platform"
)

// findSingboxRunProcessDarwin — заглушка для не-darwin платформ; ошибка
// уводит caller на общий имя-ориентированный скан.
func findSingboxRunProcessDarwin() (bool, int, error) {
	return false, -1, errors.New("darwin only")
}

// findPrivilegedCopyInUserSession — PID защищённой копии ядра
// (sing-box-lxd.exe), запущенной повышенным classic в сессии пользователя
// (SPEC 141 §8); служба живёт в сессии 0 и сюда не попадает. -1 — нет
// (или платформа без копии: Linux, Win7).
func findPrivilegedCopyInUserSession() int {
	if runtime.GOOS != "windows" || platform.PrivilegedCopyName == "" {
		return -1
	}
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq "+platform.PrivilegedCopyName, "/FI", "SESSION ne 0", "/FO", "CSV", "/NH")
	platform.PrepareCommand(cmd)
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(out), "\n") {
		parts := parseCSVLine(strings.TrimSpace(line))
		if len(parts) < 2 || !strings.EqualFold(strings.Trim(parts[0], `"`), platform.PrivilegedCopyName) {
			continue
		}
		if pid, err := strconv.Atoi(strings.Trim(parts[1], `"`)); err == nil && pid > 0 {
			return pid
		}
	}
	return -1
}
