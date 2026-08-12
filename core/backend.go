package core

import (
	"fmt"

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
