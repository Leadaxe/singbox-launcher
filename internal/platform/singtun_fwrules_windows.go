//go:build windows
// +build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows/registry"

	"singbox-launcher/internal/debuglog"
)

// firewallRulesRegPath — persistent-хранилище всех правил брандмауэра.
// Чтение HKLM не требует прав администратора; удаление идёт через netsh
// (нужна элевация — лаунчер и так запущен elevated ради TUN).
const firewallRulesRegPath = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\FirewallRules`

// CleanupOrphanSingTunFirewallRules удаляет правила `sing-tun (<путь>)`,
// чей бинарь больше не существует. Правило для живого пути не трогаем;
// сомнительные случаи (Stat вернул не NotExist) — тоже оставляем.
//
// Возвращает число удалённых правил. Ошибки удаления отдельных правил не
// прерывают проход — логируются и пропускаются (например, нет элевации).
func CleanupOrphanSingTunFirewallRules() (removed int, err error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, firewallRulesRegPath, registry.READ)
	if err != nil {
		return 0, fmt.Errorf("CleanupOrphanSingTunFirewallRules: open FirewallRules key: %w", err)
	}
	defer debuglog.RunAndLog("CleanupOrphanSingTunFirewallRules: close registry key", k.Close)

	valueNames, err := k.ReadValueNames(-1)
	if err != nil {
		return 0, fmt.Errorf("CleanupOrphanSingTunFirewallRules: read value names: %w", err)
	}

	// netsh удаляет ВСЕ правила с совпавшим именем за один вызов, а дубли
	// имён в реестре возможны — дедупим, чтобы не звать netsh по мёртвому
	// имени второй раз.
	orphanNames := make(map[string]struct{})
	for _, vn := range valueNames {
		data, _, getErr := k.GetStringValue(vn)
		if getErr != nil {
			continue
		}
		ruleName, appPath, ok := parseSingTunFirewallRule(data)
		if !ok {
			continue
		}
		if _, statErr := os.Stat(appPath); statErr == nil || !errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		orphanNames[ruleName] = struct{}{}
	}

	for ruleName := range orphanNames {
		cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ruleName)
		PrepareCommand(cmd)
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			debuglog.WarnLog("CleanupOrphanSingTunFirewallRules: delete %q failed: %v (%s)", ruleName, runErr, out)
			continue
		}
		debuglog.InfoLog("CleanupOrphanSingTunFirewallRules: removed orphan rule %q", ruleName)
		removed++
	}
	return removed, nil
}
