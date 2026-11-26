package core

import (
	"errors"
	"fmt"
	"time"

	"singbox-launcher/core/services"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// BackendMode identifies which engine runs the VPN core.
type BackendMode string

const (
	// BackendClassic — исторический режим: лаунчер сам спавнит `sing-box run`
	// (обычный exec или привилегированный скрипт на macOS TUN) и следит за
	// процессом через Monitor/Wait.
	BackendClassic BackendMode = "classic"
	// BackendDaemon — ядро живёт внутри долгоживущего демона `sing-box lxd`
	// (launchd-служба на macOS); лаунчер управляет им по gRPC + admin REST,
	// смена конфига — in-process подмена инстанса без убийства процесса.
	BackendDaemon BackendMode = "daemon"
)

// CoreBackend abstracts how the VPN core is run and controlled. The launcher
// UI (dashboard, tray, shortcuts, debug-API) never talks to a concrete engine:
// everything routes through the controller's active backend, so the classic
// spawn path and the lxd daemon path stay swappable behind one seam.
//
// Semantics mirror the historical package-level entry points: calls are
// fire-and-forget from the UI thread; state comes back asynchronously through
// RunningState / EventBus, errors through dialogs or logs.
type CoreBackend interface {
	// Mode returns the backend identifier.
	Mode() BackendMode
	// StartVPN brings the VPN up. skipRunningCheck mirrors
	// ProcessService.Start (true = авто-перезапуск, пропустить проверку
	// «уже запущен на уровне ОС»).
	StartVPN(skipRunningCheck ...bool)
	// StopVPN brings the VPN down.
	StopVPN()
	// RestartVPN применяет свежий конфиг: classic — kill + автоперезапуск
	// через Monitor; daemon — пересборка config.json + in-process apply.
	RestartVPN()
	// OnAppExit is called from GracefulExit. Returns true when the backend
	// initiated a core shutdown the caller should wait for (classic always
	// stops; daemon leaves the core running unless настройка говорит иначе).
	OnAppExit() bool
	// Close releases backend resources (supervisor goroutines, connections)
	// without touching the core itself. Used when switching modes at runtime.
	Close()
}

// Backend returns the active core backend. Never nil after NewAppController.
func (ac *AppController) Backend() CoreBackend {
	ac.backendMu.RLock()
	defer ac.backendMu.RUnlock()
	return ac.backend
}

// setBackend swaps the active backend, closing the previous one.
//
// Порядок важен: prev.Close() вызывается ДО того, как публикуется новый
// backend. DaemonBackend в Close() снимает свой transport-override; если бы
// новый backend уже успел установить свой (это делается в его конструкторе,
// т.е. ДО setBackend), Close предыдущего затёр бы его в nil. Поэтому:
// закрываем старый, затем — если у нового есть transport — переустанавливаем
// его, гарантируя что публикуемый backend всегда владеет override'ом.
func (ac *AppController) setBackend(b CoreBackend) {
	ac.backendMu.Lock()
	prev := ac.backend
	ac.backendMu.Unlock()
	if prev != nil && prev != b {
		prev.Close()
	}
	ac.backendMu.Lock()
	ac.backend = b
	hook := ac.backendModeChangeHook
	ac.backendMu.Unlock()
	// Переустанавливаем override нового backend'а на случай, если Close
	// предыдущего его снял (общий APIService.transportOverride).
	if reinstaller, ok := b.(interface{ reinstallTransport() }); ok {
		reinstaller.reinstallTransport()
	}
	// Уведомляем подписчиков смены режима (traffic-источник и т.п.).
	if hook != nil {
		hook()
	}
}

// RestoreOwnTransport возвращает APIService транспорт СВОЕГО движка.
//
// Нужен UI-слою при возврате к локальному ядру: снятие удалённого override'а —
// это `SetTransport(nil)`, то есть «никакого транспорта», а не «транспорт
// локального демона». В daemon-режиме своё ядро говорит по gRPC, и без этого
// вызова операции core-слоя без области (AutoLoadProxies, трей, горячие
// клавиши) падали бы обратно на Clash HTTP, которого в lxd-режиме нет
// вовсе — `dial 127.0.0.1:9190: connection refused`. Панели и окна узлов
// с областью от APIService не зависят (LocalProxyTransport, CoreTarget).
//
// В classic-режиме no-op: там своего transport-override нет, и путь через
// Clash HTTP как раз правильный.
func (ac *AppController) RestoreOwnTransport() {
	if ac == nil {
		return
	}
	b := ac.Backend()
	if reinstaller, ok := b.(interface{ reinstallTransport() }); ok {
		reinstaller.reinstallTransport()
	}
}

// SetBackendModeChangeHook регистрирует колбэк, вызываемый после каждой смены
// активного бэкенда (setBackend). Используется UI-слоем, чтобы переустановить
// зависящие от режима источники (traffic-профайлер). Один слот — последний
// caller побеждает (единственный потребитель — traffic_bootstrap).
func (ac *AppController) SetBackendModeChangeHook(fn func()) {
	ac.backendMu.Lock()
	ac.backendModeChangeHook = fn
	ac.backendMu.Unlock()
}

// BackendMode returns the mode of the active backend.
func (ac *AppController) BackendMode() BackendMode {
	if b := ac.Backend(); b != nil {
		return b.Mode()
	}
	return BackendClassic
}

// SwitchBackendMode переключает движок ядра в рантайме (Settings → режим).
// Требует остановленного VPN: живой процесс classic-режима нельзя молча
// передать демону и наоборот. Персистит настройку caller (load-mutate-save).
func (ac *AppController) SwitchBackendMode(mode BackendMode) error {
	if ac.BackendMode() == mode {
		return nil
	}
	if ac.RunningState.IsRunning() {
		return fmt.Errorf("stop the VPN before switching the core engine")
	}
	switch mode {
	case BackendClassic:
		// SPEC 141 §7: прокси, поставленный лаунчером под демон, снимается.
		if leaver, ok := ac.Backend().(interface{ onEngineLeave() }); ok {
			leaver.onEngineLeave()
		}
		ac.setBackend(NewLegacyBackend(ac))
		return nil
	case BackendDaemon:
		b, err := newDaemonBackend(ac)
		if err != nil {
			return err
		}
		ac.setBackend(b)
		return nil
	default:
		return fmt.Errorf("unknown backend mode %q", mode)
	}
}

// --- Опциональные способности бэкенда (реализует только DaemonBackend) ---

// poolSource — бэкенд, умеющий отдавать пул балансировщика (gRPC GetPool).
// Запрашивается всегда для конкретной группы (node info urltest-группы) —
// отдельного перечисления групп не требуется. Тип состояния общий с
// удалённой машиной (services.PoolState): RPC один и тот же.
type poolSource interface {
	PoolState(group string) (services.PoolState, error)
}

// ChainPositionInfo, ChainInfo — состояние цепочки у работающего ядра; тип
// общий с удалённой машиной (services), RPC один и тот же (GetChainStatus).
type (
	ChainPositionInfo = services.ChainPositionInfo
	ChainInfo         = services.ChainInfo
)

// chainSource — бэкенд, умеющий отдать состояние цепочки (gRPC GetChainStatus,
// SPEC 165 ядра: по тегу; перечисления всех цепочек у ядра больше нет).
type chainSource interface {
	ChainStatus(tag string) (ChainInfo, error)
	// ProbeLayer меряет путь от клиента до позиции pos включительно.
	// Возвращает задержку и текст ошибки ЯДРА (не транспорта): «не
	// поднялось» — это диагноз, а не сбой вызова.
	ProbeLayer(chainTag string, pos int) (int64, string, error)
	// SetPositionEnabled включает/выключает позицию в рантайме (SPEC 075
	// ядра). Звеньев с SPEC 165 нет, прогревать нечего — только ошибка вызова.
	SetPositionEnabled(chainTag string, pos int, enabled bool) error
}

// coreLogSource — бэкенд с собственным источником логов ядра (gRPC
// SubscribeLog вместо файла logs/sing-box.log).
type coreLogSource interface {
	CoreLogLines(max int) []string
}

// clashEndpointSource — бэкенд знает, где слушает Clash API его ядра. В
// daemon-режиме порт перенесён (развод с classic), поэтому UI-слой Clash
// (Test API, traffic-профайлер) должен спрашивать адрес у бэкенда, а не
// читать config.json напрямую.
type clashEndpointSource interface {
	ClashEndpoint() (baseURL, token string, ok bool)
}

// DaemonClashEndpoint возвращает Clash-адрес активного бэкенда, если он его
// переопределяет (daemon-режим). ok=false — использовать config.json как
// обычно (classic).
func (ac *AppController) DaemonClashEndpoint() (baseURL, token string, ok bool) {
	src, isSrc := ac.Backend().(clashEndpointSource)
	if !isSrc {
		return "", "", false
	}
	return src.ClashEndpoint()
}

// connSnapshotSource — бэкенд отдаёт снимок соединений по gRPC (traffic без
// Clash). Реализует только DaemonBackend.
type connSnapshotSource interface {
	// ConnSnapshotFunc возвращает функцию-источник для traffic-профайлера;
	// сигнатура совпадает с traffic.SnapshotFunc, но пакет core не должен
	// зависеть от traffic в этом интерфейсе — поэтому возвращаем any и
	// приводим на UI-стороне. (traffic импортируется в backend_daemon.)
	connSnapshotFuncAny() any
}

// DaemonConnSnapshotFunc возвращает gRPC-источник соединений активного
// бэкенда как any (traffic.SnapshotFunc); nil — источника нет (classic).
// UI-слой (traffic_bootstrap) приводит к traffic.SnapshotFunc.
func (ac *AppController) DaemonConnSnapshotFunc() any {
	src, ok := ac.Backend().(connSnapshotSource)
	if !ok {
		return nil
	}
	return src.connSnapshotFuncAny()
}

// dnsQuerySource — бэкенд отдаёт DNS-плоскость структурным стримом (SPEC 018
// форка) вместо разбора sing-box.log. Реализует только DaemonBackend: в
// classic gRPC нет, и там остаётся разбор лога.
type dnsQuerySource interface {
	// dnsQuerySourceAny возвращает функцию подписки; сигнатура совпадает с
	// func(func(services.DNSQuery)) (func(), error), но возвращаем any по той
	// же причине, что и у connSnapshotFuncAny, — приведение на UI-стороне.
	dnsQuerySourceAny() any
}

// DaemonDNSQuerySource возвращает подписку на DNS-события активного бэкенда
// как any; nil — источника нет (classic → профайлер читает лог).
func (ac *AppController) DaemonDNSQuerySource() any {
	src, ok := ac.Backend().(dnsQuerySource)
	if !ok {
		return nil
	}
	return src.dnsQuerySourceAny()
}

// CoreTarget — чьё ядро описывает панель или окно: своё (Local) или
// удалённой машины. MachineID "" у Remote — машина, выбранная сейчас
// (панель Remote); непустой — только эта машина (окно узла запоминает её при
// открытии, и команда не уйдёт в другую, если выбор сменили).
type CoreTarget struct {
	Scope     services.ProxyScope
	MachineID string
}

// CoreIn — цель области панели; у Remote — выбранная сейчас машина.
func CoreIn(scope services.ProxyScope) CoreTarget {
	return CoreTarget{Scope: scope}
}

// machineTransport — транспорт удалённой машины цели из её ВЫБОРА
// (UIService.LxdMachineTransportFunc), а не то, что стоит в APIService: там
// содержимое меняется по вкладкам главного окна, а соединение с машиной
// принадлежит машине, не вкладке. ok=false — цель не Remote или машина не
// подключена (у непустого MachineID — не подключена именно она).
func (ac *AppController) machineTransport(t CoreTarget) (interface{}, bool) {
	if ac == nil || t.Scope != services.ScopeRemote {
		return nil, false
	}
	if ac.UIService == nil || ac.UIService.LxdMachineTransportFunc == nil {
		return nil, false
	}
	return ac.UIService.LxdMachineTransportFunc(t.MachineID)
}

// ownTransportSource — бэкенд со своим proxy-транспортом (DaemonBackend).
type ownTransportSource interface {
	ownProxyTransport() services.ProxyTransport
}

// LocalProxyTransport — транспорт proxy-операций СВОЕГО ядра, независимо от
// того, что сейчас стоит в APIService. В daemon-режиме — gRPC-транспорт
// своего бэкенда (тот же, что ставит RestoreOwnTransport), в classic — Clash
// HTTP своего ядра. Транспорт удалённой машины сюда не попадает никогда:
// APIService держит его, пока на экране вкладка Remote, и чтение оттуда
// уводило команды панели Local и её окон на роутер.
//
// Всегда возвращает готовый транспорт: при выключенном clash_api это
// ClashTransport с пустым baseURL — запрос завершится ошибкой соединения.
func (ac *AppController) LocalProxyTransport() services.ProxyTransport {
	if ac == nil {
		return services.NewClashTransport("", "")
	}
	if src, ok := ac.Backend().(ownTransportSource); ok {
		if t := src.ownProxyTransport(); t != nil {
			return t
		}
	}
	if base, tok, ok := ac.DaemonClashEndpoint(); ok {
		return services.NewClashTransport(base, tok)
	}
	if ac.APIService == nil {
		return services.NewClashTransport("", "")
	}
	base, tok, _ := ac.APIService.GetClashAPIConfig()
	return services.NewClashTransport(base, tok)
}

// OwnDaemonTransport — gRPC-транспорт своего бэкенда; ok=false в classic.
func (ac *AppController) OwnDaemonTransport() (services.ProxyTransport, bool) {
	if ac == nil {
		return nil, false
	}
	src, ok := ac.Backend().(ownTransportSource)
	if !ok {
		return nil, false
	}
	t := src.ownProxyTransport()
	return t, t != nil
}

// DaemonPoolAvailable — источник цели отдаёт пул балансировщика.
//
// Пул — свойство ЯДРА цели: у Remote это ядро машины из её выбора, у Local —
// бэкенд своего ядра (`ac.Backend()`). Выбор по цели, а не по содержимому
// APIService: окно узла, открытое из Local, не должно показывать пул роутера
// оттого, что главное окно переключили на Remote.
func (ac *AppController) DaemonPoolAvailable(t CoreTarget) bool {
	if t.Scope == services.ScopeRemote {
		tr, ok := ac.machineTransport(t)
		if !ok {
			return false
		}
		_, ok = tr.(remotePoolSource)
		return ok
	}
	_, ok := ac.Backend().(poolSource)
	return ok
}

// outboundConfigSource — источник итогового конфига у работающего ядра
// (SPEC 165 ядра): свой демон и транспорт удалённой машины.
type outboundConfigSource interface {
	OutboundConfig(tag string) (services.OutboundConfig, error)
	EffectiveConfig() (string, error)
}

// outboundConfigSourceFor — источник цели; ok=false в classic-режиме и у
// машины без подключения.
func (ac *AppController) outboundConfigSourceFor(t CoreTarget) (outboundConfigSource, bool) {
	if t.Scope == services.ScopeRemote {
		tr, ok := ac.machineTransport(t)
		if !ok {
			return nil, false
		}
		src, ok := tr.(outboundConfigSource)
		return src, ok
	}
	src, ok := ac.Backend().(outboundConfigSource)
	return src, ok
}

// NodeOutboundConfig — узел по тегу у работающего ядра цели: как написан, как
// работает и что ядро в нём поменяло. Ошибка — источника нет, ядро старше
// SPEC 165 (Unimplemented) или узла нет в работающем конфиге (NotFound);
// окно тогда показывает узел из конфига лаунчера.
func (ac *AppController) NodeOutboundConfig(t CoreTarget, tag string) (services.OutboundConfig, error) {
	src, ok := ac.outboundConfigSourceFor(t)
	if !ok {
		return services.OutboundConfig{}, fmt.Errorf("the running core config is not available for this target")
	}
	return src.OutboundConfig(tag)
}

// EffectiveCoreConfig — весь конфиг работающего ядра цели после
// lx-предобработки (SPEC 165 ядра).
func (ac *AppController) EffectiveCoreConfig(t CoreTarget) (string, error) {
	src, ok := ac.outboundConfigSourceFor(t)
	if !ok {
		return "", fmt.Errorf("the running core config is not available for this target")
	}
	return src.EffectiveConfig()
}

// remotePoolSource — транспорт, умеющий отдать пул (services.LxdRemoteTransport).
// Интерфейс объявлен здесь, чтобы core не зависел от конкретного типа.
type remotePoolSource interface {
	PoolState(group string) (services.PoolState, error)
}

// DaemonPoolState возвращает пул выбранной группы у источника цели.
func (ac *AppController) DaemonPoolState(t CoreTarget, group string) (services.PoolState, error) {
	if t.Scope == services.ScopeRemote {
		tr, _ := ac.machineTransport(t)
		src, ok := tr.(remotePoolSource)
		if !ok {
			return services.PoolState{}, fmt.Errorf("pool source is not available for this machine")
		}
		return src.PoolState(group)
	}
	src, ok := ac.Backend().(poolSource)
	if !ok {
		return services.PoolState{}, fmt.Errorf("pool source is not available in this mode")
	}
	return src.PoolState(group)
}

// remoteChainSource — транспорт удалённой машины, умеющий то же самое.
type remoteChainSource interface {
	ChainStatus(tag string) (services.ChainInfo, error)
	ProbeLayer(chainTag string, pos int) (int64, string, error)
	SetPositionEnabled(chainTag string, pos int, enabled bool) error
}

// remoteChainSourceFor — источник цепочек машины цели; ok=false у Local и у
// машины, которая не подключена (или не умеет цепочки).
func (ac *AppController) remoteChainSourceFor(t CoreTarget) (remoteChainSource, bool) {
	tr, ok := ac.machineTransport(t)
	if !ok {
		return nil, false
	}
	src, ok := tr.(remoteChainSource)
	return src, ok
}

// errNoMachineChains — у Remote-цели нет подключённой машины с цепочками.
var errNoMachineChains = errors.New("chain state is not available for this machine")

// ChainsAvailable — доступно ли состояние цепочек у источника цели (выбор
// источника — как у DaemonPoolAvailable).
//
// Только gRPC-пути: служебные теги позиций (`<chain>#<i>`) намеренно не
// попадают в Clash API, поэтому classic-режим послойную пробу не потянет —
// и обещать её в UI там нельзя.
func (ac *AppController) ChainsAvailable(t CoreTarget) bool {
	if t.Scope == services.ScopeRemote {
		_, ok := ac.remoteChainSourceFor(t)
		return ok
	}
	_, ok := ac.Backend().(chainSource)
	return ok
}

// ChainFor возвращает цепочку по тегу у источника цели. ok=false — такой
// цепочки в работающем ядре нет (её могли переименовать или ещё не пересобрать
// конфиг), источник недоступен или ядро старше SPEC 165 (GetChainStatus нет).
func (ac *AppController) ChainFor(t CoreTarget, tag string) (ChainInfo, bool) {
	var (
		info ChainInfo
		err  error
	)
	if t.Scope == services.ScopeRemote {
		src, ok := ac.remoteChainSourceFor(t)
		if !ok {
			return ChainInfo{}, false
		}
		info, err = src.ChainStatus(tag)
	} else {
		src, ok := ac.Backend().(chainSource)
		if !ok {
			return ChainInfo{}, false
		}
		info, err = src.ChainStatus(tag)
	}
	if err != nil {
		debuglog.DebugLog("chain status %s: %v", tag, err)
		return ChainInfo{}, false
	}
	return info, true
}

// ProbeChainLayer меряет префикс цепочки до позиции pos включительно.
//
// pos < 0 — сама цепочка целиком (тег без `#i`): её замер включает то, чего
// нет ни в одном префиксе, — выбор звена и обвязку самого outbound'а.
func (ac *AppController) ProbeChainLayer(t CoreTarget, chainTag string, pos int) (int64, string, error) {
	if t.Scope == services.ScopeRemote {
		src, ok := ac.remoteChainSourceFor(t)
		if !ok {
			return 0, "", errNoMachineChains
		}
		return src.ProbeLayer(chainTag, pos)
	}
	src, ok := ac.Backend().(chainSource)
	if !ok {
		return 0, "", fmt.Errorf("chain probe is not available in this mode")
	}
	return src.ProbeLayer(chainTag, pos)
}

// ErrChainToggleUnsupported — ядро не знает SetChainPositionEnabled.
//
// Тег `with_lx_chain` для гейта не годится: он есть и у ядер до lx.28, где
// сам тумблер ещё не реализован. Единственный честный признак — ответ
// Unimplemented на вызов, поэтому проверяем по факту, а не пробой заранее.
var ErrChainToggleUnsupported = errors.New("core does not support chain position toggle — update the core")

// SetChainPositionEnabled включает/выключает позицию цепочки в работающем
// ядре (SPEC 075 ядра). pos — порядок пакета, 0 = вход.
func (ac *AppController) SetChainPositionEnabled(t CoreTarget, chainTag string, pos int, enabled bool) error {
	if t.Scope == services.ScopeRemote {
		src, ok := ac.remoteChainSourceFor(t)
		if !ok {
			return errNoMachineChains
		}
		err := src.SetPositionEnabled(chainTag, pos, enabled)
		// services не может импортировать core (граф замкнулся бы), и
		// «старое ядро» приезжает оттуда своим значением ошибки.
		// Сводим к одному, чтобы UI проверял один errors.Is.
		if errors.Is(err, services.ErrChainToggleUnsupported) {
			return ErrChainToggleUnsupported
		}
		return err
	}
	src, ok := ac.Backend().(chainSource)
	if !ok {
		return fmt.Errorf("chain position toggle is not available in this mode")
	}
	return src.SetPositionEnabled(chainTag, pos, enabled)
}

// DaemonCoreLogLines отдаёт хвост логов ядра из бэкенда (daemon-режим:
// кольцевой буфер SubscribeLog). ok=false — источник недоступен, читать файл.
func (ac *AppController) DaemonCoreLogLines(max int) ([]string, bool) {
	src, ok := ac.Backend().(coreLogSource)
	if !ok {
		return nil, false
	}
	return src.CoreLogLines(max), true
}

// initBackendFromSettings поднимает daemon-режим при старте лаунчера, если
// он включён в settings.json. Ошибка конструирования не фатальна — лаунчер
// остаётся на classic (уже установлен в NewAppController) и пишет warning.
func (ac *AppController) initBackendFromSettings() {
	st := locale.LoadSettings(ac.FileService.Layout.Data.Bin())
	if st.CoreBackendMode != string(BackendDaemon) {
		return
	}
	b, err := newDaemonBackend(ac)
	if err != nil {
		debuglog.WarnLog("backend: daemon mode configured but unavailable (%v); falling back to classic", err)
		return
	}
	debuglog.InfoLog("backend: daemon mode active (lxd control channel)")
	ac.setBackend(b)
}

// DaemonLinkState — состояние канала к локальному демону (lxd) для индикатора в UI.
//
// Собирается из уже существующего стрима SubscribeServiceStatus, а не отдельным
// опросом: полученный кадр = демон отвечает, обрыв/неподнявшийся стрим = промах.
// Новых запросов в сеть индикатор не добавляет.
type DaemonLinkState struct {
	// EverConnected — был ли получен хотя бы один кадр статуса с момента создания бэкенда.
	EverConnected bool
	// FailStreak — сколько раз подряд стрим статуса оборвался/не поднялся (0 = сейчас живой).
	FailStreak int
	// LastErr — ошибка последнего промаха.
	LastErr string
	// LastOK — когда последний раз приходил кадр.
	LastOK time.Time
	// CoreFatal / FatalErr — демон ответил, но ядро в FATAL (последний кадр).
	CoreFatal bool
	FatalErr  string
}

// daemonLinkSource — бэкенд, умеющий рассказать о своём канале к демону.
// Через интерфейс, а не через *DaemonBackend: тип есть только на darwin, а
// UI-код общий для всех платформ.
type daemonLinkSource interface {
	LinkState() DaemonLinkState
}

// DaemonLink возвращает состояние канала к демону; ok=false, если активный
// бэкенд не daemon (classic ничего про демона не знает — индикатор прячется).
func (ac *AppController) DaemonLink() (DaemonLinkState, bool) {
	src, ok := ac.Backend().(daemonLinkSource)
	if !ok {
		return DaemonLinkState{}, false
	}
	return src.LinkState(), true
}
