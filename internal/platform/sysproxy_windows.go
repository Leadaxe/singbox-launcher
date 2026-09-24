//go:build windows && !386

package platform

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Системный прокси пользователя через WinINet (SPEC 141 §7): в daemon-режиме
// на Windows ядро работает под LocalSystem, и его WinINet пишет в профиль
// SYSTEM, поэтому прокси профиля пользователя ставит и снимает лаунчер.
// Порт wininet.SetSystemProxy/ClearSystemProxy ядра (sing/common/wininet)
// без зависимости от sing: LAN-настройки (InternetSetOptionW +
// INTERNET_OPTION_PER_CONNECTION_OPTION; WinINet сам обновляет ProxyEnable
// и ProxyServer в HKCU), затем SETTINGS_CHANGED, PROXY_SETTINGS_CHANGED,
// REFRESH. Чтение — из реестра HKCU.

var (
	modWininet             = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOptionW = modWininet.NewProc("InternetSetOptionW")
)

const (
	internetOptionPerConnectionOption  = 75
	internetOptionSettingsChanged      = 39
	internetOptionRefresh              = 37
	internetOptionProxySettingsChanged = 95

	internetPerConnFlags       = 1
	internetPerConnProxyServer = 2

	proxyTypeDirect     = 1
	proxyTypeProxy      = 2
	proxyTypeAutoDetect = 8

	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

type internetPerConnOptionList struct {
	dwSize        uint32
	pszConnection uintptr
	dwOptionCount uint32
	dwOptionError uint32
	pOptions      uintptr
}

type internetPerConnOption struct {
	dwOption uint32
	value    uint64
}

func internetSetOption(option uintptr, buf uintptr, size uintptr) error {
	r0, _, err := syscall.SyscallN(procInternetSetOptionW.Addr(), 0, option, buf, size)
	if r0 != 1 {
		return err
	}
	return nil
}

func setProxyOptions(options ...internetPerConnOption) error {
	var list internetPerConnOptionList
	list.dwSize = uint32(unsafe.Sizeof(list))
	list.dwOptionCount = uint32(len(options))
	list.pOptions = uintptr(unsafe.Pointer(&options[0]))
	if err := internetSetOption(internetOptionPerConnectionOption, uintptr(unsafe.Pointer(&list)), uintptr(list.dwSize)); err != nil {
		return os.NewSyscallError("InternetSetOption(PerConnectionOption)", err)
	}
	if err := internetSetOption(internetOptionSettingsChanged, 0, 0); err != nil {
		return os.NewSyscallError("InternetSetOption(SettingsChanged)", err)
	}
	if err := internetSetOption(internetOptionProxySettingsChanged, 0, 0); err != nil {
		return os.NewSyscallError("InternetSetOption(ProxySettingsChanged)", err)
	}
	if err := internetSetOption(internetOptionRefresh, 0, 0); err != nil {
		return os.NewSyscallError("InternetSetOption(Refresh)", err)
	}
	return nil
}

// SetUserSystemProxy ставит прокси текущего пользователя:
// PROXY_TYPE_PROXY | PROXY_TYPE_DIRECT, сервер server (та же строка, что у
// ядра: `http://<addr>:<port>`), без bypass (§13 п. 3).
func SetUserSystemProxy(server string) error {
	if server == "" {
		return errors.New("empty proxy server")
	}
	serverPtr, err := windows.UTF16PtrFromString(server)
	if err != nil {
		return err
	}
	var flags internetPerConnOption
	flags.dwOption = internetPerConnFlags
	*(*uint32)(unsafe.Pointer(&flags.value)) = proxyTypeProxy | proxyTypeDirect
	var proxy internetPerConnOption
	proxy.dwOption = internetPerConnProxyServer
	*(*uintptr)(unsafe.Pointer(&proxy.value)) = uintptr(unsafe.Pointer(serverPtr))
	return setProxyOptions(flags, proxy)
}

// ClearUserSystemProxy снимает прокси текущего пользователя:
// PROXY_TYPE_DIRECT | PROXY_TYPE_AUTO_DETECT (как ClearSystemProxy ядра).
// Проверку «своё ли» делает вызывающий (ReadUserSystemProxy и метка).
func ClearUserSystemProxy() error {
	var flags internetPerConnOption
	flags.dwOption = internetPerConnFlags
	*(*uint32)(unsafe.Pointer(&flags.value)) = proxyTypeDirect | proxyTypeAutoDetect
	return setProxyOptions(flags)
}

// ReadUserSystemProxy читает ProxyEnable и ProxyServer из
// HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings.
func ReadUserSystemProxy() (enabled bool, server string, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return false, "", fmt.Errorf("open HKCU\\%s: %w", internetSettingsKey, err)
	}
	defer func() { _ = k.Close() }()
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		enabled = v != 0
	} else if !errors.Is(err, registry.ErrNotExist) {
		return false, "", fmt.Errorf("read ProxyEnable: %w", err)
	}
	if s, _, err := k.GetStringValue("ProxyServer"); err == nil {
		server = s
	} else if !errors.Is(err, registry.ErrNotExist) {
		return enabled, "", fmt.Errorf("read ProxyServer: %w", err)
	}
	return enabled, server, nil
}
