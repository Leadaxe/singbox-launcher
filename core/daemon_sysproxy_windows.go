//go:build windows && !386

package core

// Системный прокси в daemon-режиме на Windows (SPEC 141 §7): ядро под
// LocalSystem ставит WinINet-прокси в профиль SYSTEM, у пользователя его
// нет, поэтому прокси ставит и снимает лаунчер (platform.SetUserSystemProxy
// / ClearUserSystemProxy) с меткой владения daemon_system_proxy в
// settings.json. Точки вызова (фаза 2): успешный Apply с адресом —
// поставить; Apply без set_system_proxy, кадр статуса «ядро не started»,
// StopVPN, выход с daemon_stop_vpn_on_exit, смена движка daemon → classic,
// Unpair, Uninstall — снять своё; старт daemon-движка после краша —
// сверка по первому кадру статуса.

// daemonLauncherSetsSystemProxy — прокси ставит лаунчер: prepareConfigForDaemon
// переводит set_system_proxy inbound'ов в false и отдаёт адрес первого.
const daemonLauncherSetsSystemProxy = true

// setDaemonSystemProxy — TODO(SPEC 141 фаза 2): SetUserSystemProxy(server),
// затем метка daemon_system_proxy = server. Ошибка — WARN, apply не
// откатывается.
func (ac *AppController) setDaemonSystemProxy(server string) { _ = server }

// clearDaemonSystemProxy — TODO(SPEC 141 фаза 2): «снять своё»: метка есть,
// в HKCU ProxyEnable = 1 и ProxyServer = метке → ClearUserSystemProxy;
// метка стирается в любом случае. reason — для лога.
func (ac *AppController) clearDaemonSystemProxy(reason string) { _ = reason }
