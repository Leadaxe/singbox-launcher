package ui

import (
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"

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

	// Транспорт ставим в APIService: через него ходят ВСЕ proxy-операции
	// core-слоя (AutoLoadProxies, ping, переключение узла). Без этого
	// EffectiveProxyTransport отдавал наш gRPC только тем вызовам, что идут
	// из UI, а список прокси грузился локальным Clash-путём — роутер работал
	// на своём конфиге, а вкладка показывала локальные узлы.
	if ac.APIService != nil {
		ac.APIService.SetTransport(transport)
		// Сменилась машина — прежние узлы и группа принадлежали ДРУГОМУ ядру.
		// Без сброса список показывал бы данные предыдущей машины под именем
		// новой, пока не придёт ответ (инвариант «UI не подменяет данные»).
		ac.APIService.ResetScope(services.ScopeRemote)
	}

	if prev != nil {
		_ = prev.Close()
	}
	atomic.AddUint64(&clashConfigGeneration, 1)
	notifyOverrideChanged()
	debuglog.InfoLog("lxd override: Servers switched to %q (%s)", entry.Name, entry.Addr)
	return nil
}

// ClearLxdRemoteOverride возвращает Servers к локальному источнику.
//
// ac нужен, чтобы снять транспорт-override в APIService; nil допустим для
// вызовов, у которых контроллера под рукой нет (тогда снимается только
// UI-состояние).
func ClearLxdRemoteOverride(ac *core.AppController) {
	// Снимаем override ТОЛЬКО если он наш: в daemon-режиме там стоит
	// транспорт локального демона, и его сбрасывать нельзя — иначе своё же
	// ядро останется без gRPC-канала.
	//
	// SetTransport(nil) означает «никакого транспорта», а не «транспорт
	// локального демона», поэтому сразу возвращаем транспорт своего движка.
	// Без этого в lxd-режиме Servers падал обратно на Clash HTTP, которого
	// там нет вовсе: `dial 127.0.0.1:9190: connection refused`. В classic
	// RestoreOwnTransport — no-op, и путь через Clash HTTP остаётся верным.
	if ac != nil && ac.APIService != nil {
		if _, isRemote := ac.APIService.TransportOverride().(*services.LxdRemoteTransport); isRemote {
			ac.APIService.SetTransport(nil)
			ac.RestoreOwnTransport()
		}
	}
	// Машина больше не выбрана — её список узлов ни к чему не относится.
	// Без сброса вкладка Remote показывала бы узлы отключённой машины (а на
	// старте, до первого выбора, — узлы локального ядра, потому что раньше
	// состояние было общим).
	if ac != nil && ac.APIService != nil {
		ac.APIService.ResetScope(services.ScopeRemote)
	}

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

// lxdOverrideTransportForID — транспорт машины, если активна именно она.
//
// Профайлер и список узлов работают по одному каналу: отдельное соединение
// ради профайлера означало бы второй gRPC-стрим к той же машине.
//
// Снимок под одной блокировкой: проверять id и брать транспорт раздельно
// значило бы поймать смену машины между двумя чтениями и отдать транспорт
// уже не той машины, про которую спросили.
func lxdOverrideTransportForID(id string) (*services.LxdRemoteTransport, bool) {
	lxdOverrideMu.RLock()
	defer lxdOverrideMu.RUnlock()
	if !lxdOverrideActive || lxdOverrideID != id || lxdOverrideTransport == nil {
		return nil, false
	}
	return lxdOverrideTransport, true
}

// ReapplyLxdRemoteTransport возвращает в APIService транспорт УЖЕ выбранной
// машины (SPEC 098).
//
// Нужен на входе во вкладку Remote: пока пользователь смотрел Local, там
// стоял транспорт своего движка. Канал к машине при этом не рвался —
// соединение принадлежит машине, а не вкладке, и снимает его только явный
// Disconnect.
//
// No-op, когда машина не выбрана: подставлять чужой транспорт «на всякий
// случай» значило бы говорить с машиной, которую пользователь не выбирал.
func ReapplyLxdRemoteTransport(ac *core.AppController) {
	if ac == nil || ac.APIService == nil {
		return
	}
	transport := lxdOverrideTransportOrNil()
	if transport == nil {
		return
	}
	ac.APIService.SetTransport(transport)
}

// RemoteDaemonGroups возвращает selector-группы активного удалённого демона.
//
// Локальный путь берёт список групп из bin/config.json; для чужой машины
// такого файла у нас нет, поэтому спрашиваем само ядро по gRPC.
func RemoteDaemonGroups() (groups []string, isRemote bool, err error) {
	lxdOverrideMu.RLock()
	transport := lxdOverrideTransport
	active := lxdOverrideActive
	lxdOverrideMu.RUnlock()
	if !active || transport == nil {
		return nil, false, nil
	}
	groups, err = transport.Groups()
	if err != nil {
		debuglog.WarnLog("lxd override: GetGroups failed: %v", err)
		// isRemote=true при ошибке — это НЕ мелочь: вызывающий не должен
		// откатываться на локальный config.json. Он описывает другое ядро, и
		// подстановка его групп выдавала бы чужой список за список выбранной
		// машины (симптом: «переключил на роутер, а список локальный»).
		return nil, true, err
	}
	return groups, true, nil
}

// RegisterOverrideAPIHooks отдаёт Debug API управление remote-override
// (SPEC 100 §3.8): тот же Connect/Disconnect, что кнопки вкладки Remote.
//
// Зовётся из NewApp ПОСЛЕ создания вкладок: подписчики OnOverrideChanged к
// этому моменту стоят, и API-переключение перерисовывает список машин ровно
// тем же путём, что кнопочное. До регистрации (headless, ранний старт)
// соответствующие endpoints отвечают 503.
func RegisterOverrideAPIHooks(ac *core.AppController) {
	if ac == nil || ac.UIService == nil {
		return
	}
	ac.UIService.LxdOverrideConnectFunc = func(id string) error {
		// Паритет с connectMachine: окна прежней машины гасим до
		// переключения — они должны закрыться по СВОЕЙ машине, а не по новой.
		// fyne.Do — вызов приходит из HTTP-горутины Debug API.
		if prevID, _, ok := GetLxdRemoteOverride(); ok && prevID != id {
			fyne.Do(func() {
				CloseMachineProfiler(prevID)
				CloseMachineHostWindow(prevID)
			})
		}
		return SetLxdRemoteOverride(ac, id)
	}
	ac.UIService.LxdOverrideDisconnectFunc = func() {
		if prevID, _, ok := GetLxdRemoteOverride(); ok {
			fyne.Do(func() {
				CloseMachineProfiler(prevID)
				CloseMachineHostWindow(prevID)
			})
		}
		ClearLxdRemoteOverride(ac)
	}
	ac.UIService.LxdOverrideStateFunc = GetLxdRemoteOverride
}

type overrideError string

func (e overrideError) Error() string { return string(e) }

const (
	errNoController  = overrideError("controller or FileService unavailable")
	errUnknownRemote = overrideError("unknown remote daemon")
)
