//go:build windows && !386

package platform

import "errors"

// Системный прокси пользователя через WinINet (SPEC 141 §7): в daemon-режиме
// на Windows ядро работает под LocalSystem, и его WinINet пишет в профиль
// SYSTEM, поэтому прокси профиля пользователя ставит и снимает лаунчер.
// Порт wininet.SetSystemProxy/ClearSystemProxy ядра (sing/common/wininet)
// без зависимости от sing: LAN-настройки (InternetSetOptionW +
// INTERNET_OPTION_PER_CONNECTION_OPTION), затем SETTINGS_CHANGED,
// PROXY_SETTINGS_CHANGED, REFRESH.

// errSysProxyPending — TODO(SPEC 141 фаза 2): тела ниже.
var errSysProxyPending = errors.New("system proxy on Windows: not implemented yet")

// SetUserSystemProxy ставит прокси текущего пользователя:
// PROXY_TYPE_PROXY | PROXY_TYPE_DIRECT, сервер server (та же строка, что у
// ядра: `http://<addr>:<port>`), без bypass (§13 п. 3).
func SetUserSystemProxy(server string) error {
	_ = server
	return errSysProxyPending
}

// ClearUserSystemProxy снимает прокси текущего пользователя:
// PROXY_TYPE_DIRECT | PROXY_TYPE_AUTO_DETECT (как ClearSystemProxy ядра).
// Проверку «своё ли» делает вызывающий (ReadUserSystemProxy и метка).
func ClearUserSystemProxy() error {
	return errSysProxyPending
}

// ReadUserSystemProxy читает ProxyEnable и ProxyServer из
// HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings.
func ReadUserSystemProxy() (enabled bool, server string, err error) {
	return false, "", errSysProxyPending
}
