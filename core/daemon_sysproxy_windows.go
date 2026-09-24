//go:build windows && !386

package core

import (
	"strings"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Системный прокси в daemon-режиме на Windows (SPEC 141 §7): ядро под
// LocalSystem ставит WinINet-прокси в профиль SYSTEM, у пользователя его
// нет, поэтому прокси ставит и снимает лаунчер (platform.SetUserSystemProxy
// / ClearUserSystemProxy) с меткой владения daemon_system_proxy в
// settings.json. Точки вызова: успешный Apply с адресом — поставить; Apply
// без set_system_proxy, кадр статуса «ядро не started», StopVPN, выход с
// daemon_stop_vpn_on_exit, смена движка daemon → classic, Unpair,
// Uninstall — снять своё; краш лаунчера — первый кадр статуса сверяет.

// daemonLauncherSetsSystemProxy — прокси ставит лаунчер: prepareConfigForDaemon
// переводит set_system_proxy inbound'ов в false и отдаёт адрес первого.
const daemonLauncherSetsSystemProxy = true

// daemonTailscaleLocalRoot — корень state_directory узлов tailscale в
// DataDir: в daemon-режиме их переносят в <StateDir>\tailscale\<тег>
// (SPEC 141 §7 шаг 4, §13 п. 5) — SYSTEM не пишет в каталог пользователя.
func daemonTailscaleLocalRoot() string { return config.TailscaleStateDirRoot() }

// sameProxyServer — строка сервера в HKCU и метка: без схемы http://, без
// учёта регистра.
func sameProxyServer(a, b string) bool {
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		return strings.TrimPrefix(s, "http://")
	}
	return norm(a) != "" && norm(a) == norm(b)
}

// setDaemonSystemProxy — SetUserSystemProxy(server), затем метка
// daemon_system_proxy = server. Уже стоит своё — ничего. Ошибка — WARN,
// apply не откатывается.
func (ac *AppController) setDaemonSystemProxy(server string) {
	if server == "" {
		return
	}
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	if st.DaemonSystemProxy == server {
		if enabled, current, err := platform.ReadUserSystemProxy(); err == nil && enabled && sameProxyServer(current, server) {
			return
		}
	}
	if err := platform.SetUserSystemProxy(server); err != nil {
		debuglog.WarnLog("daemon: set the user system proxy %s: %v", server, err)
		return
	}
	st.DaemonSystemProxy = server
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("daemon: save daemon_system_proxy: %v", err)
	}
	debuglog.InfoLog("daemon: user system proxy set to %s", server)
}

// clearDaemonSystemProxy — «снять своё»: метка есть, в HKCU ProxyEnable = 1
// и ProxyServer = метке → ClearUserSystemProxy; метка стирается в любом
// случае (чужой прокси лаунчер не трогает). reason — для лога.
func (ac *AppController) clearDaemonSystemProxy(reason string) {
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	label := st.DaemonSystemProxy
	if label == "" {
		return
	}
	enabled, current, err := platform.ReadUserSystemProxy()
	switch {
	case err != nil:
		debuglog.WarnLog("daemon: read the user system proxy: %v", err)
	case enabled && sameProxyServer(current, label):
		if err := platform.ClearUserSystemProxy(); err != nil {
			debuglog.WarnLog("daemon: clear the user system proxy (%s): %v", reason, err)
			return
		}
		debuglog.InfoLog("daemon: user system proxy %s cleared (%s)", label, reason)
	default:
		debuglog.InfoLog("daemon: the user system proxy is no longer %s (%s): leaving it as is", label, reason)
	}
	st.DaemonSystemProxy = ""
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("daemon: save daemon_system_proxy: %v", err)
	}
}
