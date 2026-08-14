package ui

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	tprof "singbox-launcher/internal/traffic"
	uitraffic "singbox-launcher/ui/traffic"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// trafficManager is the lazily-initialized singleton window manager for
// the Traffic Profiler window. Lives here (ui package) because it needs
// access to the AppController to build its WindowDeps.
var (
	trafficManagerOnce sync.Once
	trafficManager     *uitraffic.Manager
)

// profilerHTTPClient is a dedicated HTTP client for /connections polling.
// We don't reuse api.getHTTPClient because that one's transport gets
// reset on power-resume and we don't want to fight over it. 5s timeout
// is well under our 1s poll interval — a stuck request gets cancelled
// before the next tick.
var profilerHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 2 * time.Second,
		}).DialContext,
		IdleConnTimeout:   30 * time.Second,
		DisableKeepAlives: false,
	},
}

// wireTrafficBadgeToProfiler registers a callback so the Diagnostics
// tab Traffic Profiler button label re-renders (⚡ on/off) when the
// active session state changes. Safe to call multiple times — last
// caller wins (acceptable: there's only one Diagnostics tab).
func wireTrafficBadgeToProfiler(cb func()) {
	tprof.GetInstance().SetOnSessionChange(cb)
}

// EnsureTrafficProfilerStarted spins up the always-on
// internal/traffic.TrafficProfiler if it isn't running yet, and routes
// poller / tailer warnings into debuglog. Safe to call repeatedly — the
// underlying profiler is itself singleton-guarded.
//
// Called from main.go after the AppController is fully wired so that
// FileService.ExecDir is known. The profiler runs whether or not the
// Traffic Profiler window is open — recording survives window close.
func EnsureTrafficProfilerStarted(ac *core.AppController) {
	tprof.SetPollerWarn(debuglog.WarnLog)
	tprof.SetTailerWarn(debuglog.WarnLog)
	p := tprof.GetInstance()

	cfg := func() (string, string, bool) {
		if ac.APIService == nil {
			return "", "", false
		}
		// Daemon-режим: Clash API вырезан из конфига (управление и трафик по
		// gRPC). Backend это сигналит через DaemonClashEndpoint. Пока трафик
		// в daemon-режиме идёт не через Clash-поллер — возвращаем "не
		// поллить", чтобы не биться в отсутствующий Clash-порт.
		// (gRPC SubscribeConnections — отдельный источник, подключается ниже.)
		if base, tok, ok := ac.DaemonClashEndpoint(); ok {
			return base, tok, true
		}
		if ac.BackendMode() == core.BackendDaemon {
			return "", "", false // daemon без Clash — Clash-поллер выключен
		}
		return ac.APIService.GetClashAPIConfig()
	}
	logPath := filepath.Join(platform.GetLogsDir(ac.FileService.ExecDir), constants.ChildLogFileName)
	p.Start(cfg, logPath, profilerHTTPClient)

	// Источник трафика по режиму: daemon → gRPC SubscribeConnections,
	// classic → nil (Clash HTTP через cfg выше). Переустанавливается при
	// смене backend (ac.OnBackendModeChanged).
	applyTrafficSource(ac, p)
	ac.SetBackendModeChangeHook(func() { applyTrafficSource(ac, p) })
}

// applyTrafficSource ставит профайлеру gRPC-источник соединений в
// daemon-режиме, снимает (nil → Clash HTTP) в classic. Заодно переключает
// DNS-плоскость: daemon → структурный стрим, classic → разбор лога.
func applyTrafficSource(ac *core.AppController, p *tprof.TrafficProfiler) {
	if fnAny := ac.DaemonConnSnapshotFunc(); fnAny != nil {
		if fn, ok := fnAny.(tprof.SnapshotFunc); ok {
			p.SetConnSnapshotFunc(fn)
			applyDNSSource(ac, p)
			return
		}
	}
	p.SetConnSnapshotFunc(nil)
	applyDNSSource(ac, p)
}

// localDNSSub — активная подписка локального профайлера на DNS-стрим.
// Одна на процесс: профайлер тоже один. Снимается при уходе из daemon-режима,
// иначе стрим пережил бы свой backend и держал бы ядро в режиме эмита.
var (
	localDNSMu     sync.Mutex
	localDNSCancel func()
)

// applyDNSSource подписывает профайлер на SubscribeDNSQueries в daemon-режиме
// и снимает подписку в classic.
//
// В classic источника нет by design: gRPC там отсутствует, Clash отдаёт только
// точечный /dns/query, и единственный поток DNS — текстовый лог. Это и есть та
// часть функционала, что доступна лишь на lxd gRPC.
func applyDNSSource(ac *core.AppController, p *tprof.TrafficProfiler) {
	localDNSMu.Lock()
	defer localDNSMu.Unlock()

	// Снимаем прежнюю подписку в любом случае: backend мог смениться, и старый
	// стрим больше не описывает то ядро, за которым мы наблюдаем.
	if localDNSCancel != nil {
		localDNSCancel()
		localDNSCancel = nil
	}
	p.SetDNSFromStream(false)

	subAny := ac.DaemonDNSQuerySource()
	if subAny == nil {
		return // classic → остаётся LogTailer
	}
	subscribe, ok := subAny.(func(func(services.DNSQuery), func()) (func(), error))
	if !ok {
		debuglog.WarnLog("traffic: неожиданный тип источника DNS %T", subAny)
		return
	}
	cancel, err := subscribe(
		func(q services.DNSQuery) { p.PushEvent(dnsQueryToEvent(q)) },
		// Ядро собрано без with_lx_command: стрима не будет. Возвращаем лог
		// как источник — иначе DNS-плоскость исчезла бы совсем, ведь лог мы
		// к этому моменту уже подавили.
		func() { p.SetDNSFromStream(false) },
	)
	if err != nil {
		// Демон недоступен или ядро без with_lx_command. Профайлер остаётся
		// рабочим — DNS-плоскость просто приедет из лога, как в classic.
		debuglog.WarnLog("traffic: dns stream: %v", err)
		return
	}
	localDNSCancel = cancel
	p.SetDNSFromStream(true)
}

// trafficWindowManager lazily creates the window-singleton manager and
// (re)populates its dependency bundle with the current AppController.
// Called by the Diagnostics tab Traffic Profiler button.
func trafficWindowManager(ac *core.AppController, parentRefresh func()) *uitraffic.Manager {
	trafficManagerOnce.Do(func() {
		trafficManager = uitraffic.NewManager(uitraffic.WindowDeps{})
	})
	trafficManager.SetDeps(uitraffic.WindowDeps{
		App:      ac.UIService.Application,
		Profiler: tprof.GetInstance(),
		ConfigReader: func() (string, bool) {
			level, set, err := ReadCurrentLogLevel(ac)
			if err != nil {
				return "", false
			}
			if !set {
				// Fall back to wizard_template.json default — for the
				// purposes of "is verbose on?" treat any non-debug as
				// no.
				return "warn", true
			}
			return level, true
		},
		ConfigWriter: func(level string) error {
			return ApplyLogLevelAndReload(ac, level)
		},
		ConfigConfirmApply: func(level string, parent fyne.Window, done func()) {
			ConfirmAndApplyLogLevel(ac, parent, level, done)
		},
		FindProcessEnabled: func() bool {
			return readFindProcessFromConfig(ac.FileService.ConfigPath)
		},
		ParentRefresh: parentRefresh,
		SingBoxRunning: func() bool {
			if ac == nil || ac.RunningState == nil {
				return false
			}
			return ac.RunningState.IsRunning()
		},
	})
	return trafficManager
}

// readFindProcessFromConfig reads config.json and returns the value of
// route.find_process (default false when missing, since that's the
// sing-box default). Used to show a banner when process attribution
// will be empty.
//
// Best-effort: if config.json doesn't exist (cold start before first
// Save) we return false so the banner displays — guides the user to
// run the wizard and Save.
func readFindProcessFromConfig(path string) bool {
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	clean := jsonc.ToJSON(data)
	var probe struct {
		Route struct {
			FindProcess bool `json:"find_process"`
		} `json:"route"`
	}
	if err := json.Unmarshal(clean, &probe); err != nil {
		return false
	}
	return probe.Route.FindProcess
}
