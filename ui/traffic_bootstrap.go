package ui

import (
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	tprof "singbox-launcher/internal/traffic"
	uitraffic "singbox-launcher/ui/traffic"
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
		return ac.APIService.GetClashAPIConfig()
	}
	logPath := filepath.Join(platform.GetLogsDir(ac.FileService.ExecDir), constants.ChildLogFileName)
	p.Start(cfg, logPath, profilerHTTPClient)
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
		FindProcessEnabled: func() bool { return true }, // phase 7 reads from template
		ParentRefresh:      parentRefresh,
		SingBoxRunning: func() bool {
			if ac == nil || ac.RunningState == nil {
				return false
			}
			return ac.RunningState.IsRunning()
		},
	})
	return trafficManager
}
