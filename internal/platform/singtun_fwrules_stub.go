//go:build !windows
// +build !windows

package platform

// CleanupOrphanSingTunFirewallRules — no-op вне Windows: правила брандмауэра
// sing-tun создаёт только там. Сигнатура зеркалит windows-версию, чтобы
// вызывать без runtime.GOOS-ветвления.
func CleanupOrphanSingTunFirewallRules() (removed int, err error) {
	return 0, nil
}
