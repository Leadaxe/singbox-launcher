package ui

import (
	"sync"
	"sync/atomic"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
)

// SPEC 097 — подключение вкладки Servers к УДАЛЁННОМУ демону `sing-box lxd`.
//
// Зачем рядом с Clash-override (clash_remote.go): у remote-конфига Clash API
// нет by design — им управляет демон по gRPC. То есть роутер, для которого
// лаунчер собрал конфиг, принципиально недостижим Clash-путём, и без этого
// override'а Servers к нему подключиться не может.
//
// Как и Clash-override, это RAM-only выбор: какой из сохранённых демонов
// сейчас смотрим. Сами подключения (адрес, пин, ключи) живут в реестре
// services.RemoteRegistry и переживают перезапуск — эфемерен только выбор.

var (
	lxdOverrideMu     sync.RWMutex
	lxdOverrideID     string
	lxdOverrideName   string
	lxdOverrideActive bool
	// lxdOverrideTransport — живой транспорт; держим его, чтобы
	// переиспользовать gRPC-соединение между запросами вкладки и закрыть при
	// переключении.
	lxdOverrideTransport *services.LxdRemoteTransport
)

// SetLxdRemoteOverride переключает Servers на сохранённого удалённого демона.
// Предыдущий транспорт закрывается — иначе висело бы лишнее gRPC-соединение
// к машине, которую пользователь уже не смотрит.
func SetLxdRemoteOverride(ac *core.AppController, id string) error {
	if ac == nil || ac.FileService == nil {
		return errNoController
	}
	registry := services.NewRemoteRegistry(ac.FileService.ExecDir)
	entry, ok, err := registry.Get(id)
	if err != nil {
		return err
	}
	if !ok {
		return errUnknownRemote
	}
	transport, err := registry.Transport(id)
	if err != nil {
		return err
	}

	lxdOverrideMu.Lock()
	prev := lxdOverrideTransport
	lxdOverrideID = entry.ID
	lxdOverrideName = entry.Name
	lxdOverrideTransport = transport
	lxdOverrideActive = true
	lxdOverrideMu.Unlock()

	if prev != nil {
		_ = prev.Close()
	}
	atomic.AddUint64(&clashConfigGeneration, 1)
	notifyOverrideChanged()
	debuglog.InfoLog("lxd override: Servers switched to %q (%s)", entry.Name, entry.Addr)
	return nil
}

// ClearLxdRemoteOverride возвращает Servers к локальному источнику.
func ClearLxdRemoteOverride() {
	lxdOverrideMu.Lock()
	prev := lxdOverrideTransport
	lxdOverrideID = ""
	lxdOverrideName = ""
	lxdOverrideTransport = nil
	lxdOverrideActive = false
	lxdOverrideMu.Unlock()

	if prev != nil {
		_ = prev.Close()
	}
	atomic.AddUint64(&clashConfigGeneration, 1)
	notifyOverrideChanged()
}

// GetLxdRemoteOverride — снимок текущего выбора (id, имя, активен ли).
func GetLxdRemoteOverride() (string, string, bool) {
	lxdOverrideMu.RLock()
	defer lxdOverrideMu.RUnlock()
	return lxdOverrideID, lxdOverrideName, lxdOverrideActive
}

// lxdOverrideTransportOrNil — активный транспорт или nil.
func lxdOverrideTransportOrNil() services.ProxyTransport {
	lxdOverrideMu.RLock()
	defer lxdOverrideMu.RUnlock()
	if !lxdOverrideActive || lxdOverrideTransport == nil {
		return nil
	}
	return lxdOverrideTransport
}

// RemoteDaemonGroups возвращает selector-группы активного удалённого демона.
//
// Локальный путь берёт список групп из bin/config.json; для чужой машины
// такого файла у нас нет, поэтому спрашиваем само ядро по gRPC.
func RemoteDaemonGroups() ([]string, bool) {
	lxdOverrideMu.RLock()
	transport := lxdOverrideTransport
	active := lxdOverrideActive
	lxdOverrideMu.RUnlock()
	if !active || transport == nil {
		return nil, false
	}
	groups, err := transport.Groups()
	if err != nil {
		debuglog.WarnLog("lxd override: GetGroups failed: %v", err)
		return nil, false
	}
	return groups, true
}

type overrideError string

func (e overrideError) Error() string { return string(e) }

const (
	errNoController  = overrideError("controller or FileService unavailable")
	errUnknownRemote = overrideError("unknown remote daemon")
)
