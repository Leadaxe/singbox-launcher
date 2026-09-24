//go:build darwin

package core

// Системный прокси в daemon-режиме (SPEC 141 §7): на macOS его ставит ядро
// под root системно — лаунчеру делать нечего. Windows —
// daemon_sysproxy_windows.go.

// daemonLauncherSetsSystemProxy — прокси ставит лаунчер, а не ядро:
// prepareConfigForDaemon переводит set_system_proxy в false и возвращает
// адрес для setDaemonSystemProxy. На macOS — нет.
const daemonLauncherSetsSystemProxy = false

// daemonTailscaleLocalRoot — на macOS state_directory tailscale остаётся в
// DataDir (долг SPEC 141 §11): переносить нечего.
func daemonTailscaleLocalRoot() string { return "" }

// setDaemonSystemProxy — no-op: прокси ставит ядро.
func (ac *AppController) setDaemonSystemProxy(server string) { _ = server }

// clearDaemonSystemProxy — no-op: прокси снимает ядро.
func (ac *AppController) clearDaemonSystemProxy(reason string) { _ = reason }
