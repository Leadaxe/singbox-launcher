//go:build darwin

package netiface

import (
	"os/exec"
	"strings"
	"sync"
)

// friendlyNames кэширует разбор `networksetup -listnetworkserviceorder`.
// Вызов внешнего процесса на каждый интерфейс в списке означал бы N запусков
// подпроцесса при каждой отрисовке вкладки настроек.
// (sync.Once, а не sync.OnceValue: легаси-сборка Win7 идёт тулчейном go1.20.)
var (
	friendlyOnce  sync.Once
	friendlyCache map[string]string
)

func friendlyNames() map[string]string {
	friendlyOnce.Do(func() { friendlyCache = loadFriendlyNames() })
	return friendlyCache
}

func loadFriendlyNames() map[string]string {
	out := map[string]string{}
	raw, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return out
	}
	// Строки вида: «(Hardware Port: Wi-Fi, Device: en0)».
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "(Hardware Port:") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "(Hardware Port:"), ")")
		parts := strings.SplitN(body, ", Device:", 2)
		if len(parts) != 2 {
			continue
		}
		port, dev := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if dev != "" && port != "" {
			out[dev] = port
		}
	}
	return out
}

func friendlyName(dev string) string { return friendlyNames()[dev] }
