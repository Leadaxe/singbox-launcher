package core

import (
	"errors"
	"fmt"
	"time"

	"singbox-launcher/core/services"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
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
// вызова Servers падал обратно на Clash HTTP, которого в lxd-режиме нет
// вовсе — пользователь получал `dial 127.0.0.1:9190: connection refused`.
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

// PoolSlotInfo — слот пула балансировщика (lx-наблюдаемость urltest-группы).
type PoolSlotInfo struct {
	Slot  uint32
	Tag   string
	Delay uint32
}

// poolSource — бэкенд, умеющий отдавать пул балансировщика (gRPC GetPool).
// Запрашивается всегда для конкретной группы (node info urltest-группы) —
// отдельного перечисления групп не требуется.
type poolSource interface {
	PoolSlots(group string) ([]PoolSlotInfo, error)
}

// ChainPositionInfo — одна позиция цепочки в рантайме (SPEC 110).
//
// Отличается от позиции в конфиге тем, что знает РАЗРЕШЁННЫЙ узел: за тегом
// группы стоит её текущий выбор, и увидеть его иначе как у работающего ядра
// нельзя.
type ChainPositionInfo struct {
	Tag string
	// Now — во что позиция резолвится сейчас (для группы — её выбор,
	// раскрытый до листа). Пусто, если ядро не смогло разрешить.
	Now string
	// IsGroup — за позицией группа, и Now может смениться без нашего ведома.
	IsGroup bool
	// Transparent — в позиции выбран direct: хоп схлопнут, в рантайме его
	// нет. Мерить такую позицию нечем, и виновником она быть не может.
	Transparent bool
	// Disabled — позицию выключил пользователь тумблером (SPEC 075 ядра).
	// Отличается от Transparent источником решения: Transparent — так
	// написан конфиг (в позиции direct), Disabled — воля пользователя в
	// рантайме, ядро помнит её в cache-file и восстанавливает при старте.
	// Now у выключенной позиции остаётся заполненным: видно, ЧТО выключено.
	Disabled bool
	// CloneState — состояние звена: starting | active | idle. Пусто у
	// позиции 0 (вход не клонируется) и пока звено не создано.
	CloneState string
	LastError  string
}

// ChainInfo — состояние одной цепочки у работающего ядра.
type ChainInfo struct {
	Tag       string
	Positions []ChainPositionInfo
}

// chainSource — бэкенд, умеющий отдать состояние цепочек (gRPC GetChains).
//
// Перечисление, а не запрос по тегу: RPC ядра аргументов не принимает, и
// выбирать нужную цепочку приходится на нашей стороне.
type chainSource interface {
	Chains() ([]ChainInfo, error)
	// ProbeLayer меряет путь от клиента до позиции pos включительно.
	// Возвращает задержку и текст ошибки ЯДРА (не транспорта): «не
	// поднялось» — это диагноз, а не сбой вызова.
	ProbeLayer(chainTag string, pos int) (int64, string, error)
	// SetPositionEnabled включает/выключает позицию в рантайме.
	// Второй результат — warmupError ЯДРА: флаг применён, но прогрев
	// звена не удался. Это диагноз узла, а не сбой вызова, поэтому
	// приезжает данными рядом с nil-ошибкой (контракт SPEC 075 ядра).
	SetPositionEnabled(chainTag string, pos int, enabled bool) (string, error)
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

// DaemonPoolAvailable — текущий источник отдаёт пул балансировщика.
//
// SPEC 097: сначала смотрим на активный транспорт, потом на бэкенд. Пул —
// свойство ЯДРА, за которым мы сейчас наблюдаем: при выбранной удалённой
// машине это её ядро, а `ac.Backend()` описывает локальное. Без этого окно
// узла удалённой машины не показывало активного участника urltest-группы.
func (ac *AppController) DaemonPoolAvailable() bool {
	if ac.APIService != nil {
		if _, ok := ac.APIService.TransportOverride().(remotePoolSource); ok {
			return true
		}
	}
	_, ok := ac.Backend().(poolSource)
	return ok
}

// remotePoolSource — транспорт, умеющий отдать пул (services.LxdRemoteTransport).
// Интерфейс объявлен здесь, чтобы core не зависел от конкретного типа.
type remotePoolSource interface {
	PoolSlots(group string) ([]services.PoolSlot, error)
}

// DaemonPoolSlots возвращает слоты пула выбранной группы у текущего источника.
func (ac *AppController) DaemonPoolSlots(group string) ([]PoolSlotInfo, error) {
	if ac.APIService != nil {
		if src, ok := ac.APIService.TransportOverride().(remotePoolSource); ok {
			slots, err := src.PoolSlots(group)
			if err != nil {
				return nil, err
			}
			out := make([]PoolSlotInfo, 0, len(slots))
			for _, s := range slots {
				out = append(out, PoolSlotInfo{Slot: s.Slot, Tag: s.Tag, Delay: s.Delay})
			}
			return out, nil
		}
	}
	src, ok := ac.Backend().(poolSource)
	if !ok {
		return nil, fmt.Errorf("pool source is not available in this mode")
	}
	return src.PoolSlots(group)
}

// remoteChainSource — транспорт удалённой машины, умеющий то же самое.
type remoteChainSource interface {
	Chains() ([]services.ChainInfo, error)
	ProbeLayer(chainTag string, pos int) (int64, string, error)
	SetPositionEnabled(chainTag string, pos int, enabled bool) (string, error)
}

// ChainsAvailable — доступно ли состояние цепочек у текущего источника.
//
// Только gRPC-пути: служебные теги позиций (`<chain>#<i>`) намеренно не
// попадают в Clash API, поэтому classic-режим послойную пробу не потянет —
// и обещать её в UI там нельзя.
func (ac *AppController) ChainsAvailable() bool {
	if ac.APIService != nil {
		if _, ok := ac.APIService.TransportOverride().(remoteChainSource); ok {
			return true
		}
	}
	_, ok := ac.Backend().(chainSource)
	return ok
}

// Chains возвращает состояние цепочек текущего источника.
func (ac *AppController) Chains() ([]ChainInfo, error) {
	if ac.APIService != nil {
		if src, ok := ac.APIService.TransportOverride().(remoteChainSource); ok {
			chains, err := src.Chains()
			if err != nil {
				return nil, err
			}
			out := make([]ChainInfo, 0, len(chains))
			for _, c := range chains {
				positions := make([]ChainPositionInfo, 0, len(c.Positions))
				for _, p := range c.Positions {
					positions = append(positions, ChainPositionInfo{
						Tag:         p.Tag,
						Now:         p.Now,
						IsGroup:     p.IsGroup,
						Transparent: p.Transparent,
						Disabled:    p.Disabled,
						CloneState:  p.CloneState,
						LastError:   p.LastError,
					})
				}
				out = append(out, ChainInfo{Tag: c.Tag, Positions: positions})
			}
			return out, nil
		}
	}
	src, ok := ac.Backend().(chainSource)
	if !ok {
		return nil, fmt.Errorf("chain state is not available in this mode")
	}
	return src.Chains()
}

// ChainFor возвращает цепочку по тегу. ok=false — такой цепочки в работающем
// ядре нет (её могли переименовать или ещё не пересобрать конфиг).
func (ac *AppController) ChainFor(tag string) (ChainInfo, bool) {
	chains, err := ac.Chains()
	if err != nil {
		return ChainInfo{}, false
	}
	for _, c := range chains {
		if c.Tag == tag {
			return c, true
		}
	}
	return ChainInfo{}, false
}

// ProbeChainLayer меряет префикс цепочки до позиции pos включительно.
//
// pos < 0 — сама цепочка целиком (тег без `#i`): её замер включает то, чего
// нет ни в одном префиксе, — выбор звена и обвязку самого outbound'а.
func (ac *AppController) ProbeChainLayer(chainTag string, pos int) (int64, string, error) {
	if ac.APIService != nil {
		if src, ok := ac.APIService.TransportOverride().(remoteChainSource); ok {
			return src.ProbeLayer(chainTag, pos)
		}
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
//
// Первый результат — warmupError ядра: тумблер УЖЕ применён, но поднять
// звено на включённой позиции не удалось. Ядро отдаёт это данными, а не
// статус-ошибкой, потому что тумблер выражает волю пользователя, а
// здоровье узла — отдельный факт; UI обязан показать оба.
func (ac *AppController) SetChainPositionEnabled(chainTag string, pos int, enabled bool) (string, error) {
	if ac.APIService != nil {
		if src, ok := ac.APIService.TransportOverride().(remoteChainSource); ok {
			warmup, err := src.SetPositionEnabled(chainTag, pos, enabled)
			// services не может импортировать core (граф замкнулся бы), и
			// «старое ядро» приезжает оттуда своим значением ошибки.
			// Сводим к одному, чтобы UI проверял один errors.Is.
			if errors.Is(err, services.ErrChainToggleUnsupported) {
				return warmup, ErrChainToggleUnsupported
			}
			return warmup, err
		}
	}
	src, ok := ac.Backend().(chainSource)
	if !ok {
		return "", fmt.Errorf("chain position toggle is not available in this mode")
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
	st := locale.LoadSettings(platform.GetBinDir(ac.FileService.ExecDir))
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
