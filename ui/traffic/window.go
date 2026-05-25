// Package traffic implements the Traffic Profiler UI (SPEC 059).
//
// A single Fyne window opens from the Diagnostics tab. The window is a
// singleton — repeat clicks focus the existing window rather than opening
// a second one. The window subscribes to the always-on
// internal/traffic.TrafficProfiler for live events; closing the window
// only unsubscribes the UI — the profiler keeps capturing in the
// background (including any active recording session). Re-opening the
// window shows the still-active session.
package traffic

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	tprof "singbox-launcher/internal/traffic"
)

// WindowDeps is what the Diagnostics tab hands to ShowWindow. We keep the
// surface tiny to avoid pulling AppController into this package and to
// make the window testable with mocks if we ever want UI tests.
type WindowDeps struct {
	App      fyne.App
	Profiler *tprof.TrafficProfiler

	// ConfigReader returns the current sing-box log level. Used by the
	// verbose toggle to render its checkbox state. nil → toggle hidden.
	ConfigReader func() (logLevel string, ok bool)
	// ConfigWriter applies a new log level and triggers a sing-box
	// rebuild + restart. Use ConfigConfirmApply in UI code — this raw
	// writer is for advanced callers that handle their own dialog.
	ConfigWriter func(level string) error
	// ConfigConfirmApply shows the "active connections will reset"
	// confirmation modal and applies on user confirm. Recommended UI
	// path. May be nil.
	ConfigConfirmApply func(level string, parent fyne.Window, done func())

	// FindProcessEnabled returns true if the active config has
	// route.find_process: true. Used to decide whether to show the
	// "process detection disabled" banner. Nil → assume true.
	FindProcessEnabled func() bool

	// ParentRefresh is called when the recording badge state changes so
	// the Diagnostics tab can re-render its button label with/without ⚡.
	ParentRefresh func()

	// SingBoxRunning reports whether sing-box is up. Banner-driver.
	SingBoxRunning func() bool
}

// Manager owns the singleton window pointer. There's one Manager per
// running app — instantiated from the UI layer wiring (ui/app.go or
// equivalent) and reused by the Diagnostics button.
type Manager struct {
	mu     sync.Mutex
	win    fyne.Window
	deps   WindowDeps
	titleStopCh chan struct{}
}

// NewManager constructs an unopened window manager. Deps may be filled
// in later via SetDeps if the caller can't supply them up front.
func NewManager(deps WindowDeps) *Manager {
	return &Manager{deps: deps}
}

// SetDeps replaces the dependency bundle. Safe to call before the
// window is open.
func (m *Manager) SetDeps(deps WindowDeps) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deps = deps
}

// Show either creates the singleton window or focuses the existing one.
// Must be called on the Fyne UI thread.
func (m *Manager) Show() {
	m.mu.Lock()
	if m.win != nil {
		w := m.win
		m.mu.Unlock()
		w.Show()
		w.RequestFocus()
		return
	}
	m.mu.Unlock()
	m.build()
}

// IsRecording is the helper the Diagnostics tab calls when it re-renders
// its button label — controls the ⚡ badge.
func (m *Manager) IsRecording() bool {
	if m.deps.Profiler == nil {
		return false
	}
	return m.deps.Profiler.ActiveSession() != nil
}

func (m *Manager) build() {
	// Snapshot deps + abort early without holding the lock across all the
	// sub-builds. Sub-views (buildPerProcessView) may invoke m.refreshTitle
	// transitively which itself takes m.mu — holding it here = deadlock.
	// Show() is always called from the UI thread (button OnTap), so we
	// don't need a mutex to serialize concurrent build() calls.
	m.mu.Lock()
	if m.win != nil {
		m.mu.Unlock()
		return
	}
	if m.deps.App == nil || m.deps.Profiler == nil {
		m.mu.Unlock()
		return
	}
	deps := m.deps
	m.mu.Unlock()

	win := deps.App.NewWindow("Traffic Profiler")

	live := buildLiveView(deps)
	perProcess := buildPerProcessView(deps, func() {
		// Defer to avoid synchronous re-entry into m.refreshTitle's mutex
		// while we're still inside build()'s UI-thread stack. The first
		// real refresh fires from startTitleTimer / profiler subscriber
		// shortly anyway.
		go func() { fyne.Do(func() { m.refreshTitle() }) }()
	})

	tabs := container.NewAppTabs(
		container.NewTabItem("Live", live.Content),
		container.NewTabItem("Per-process", perProcess.Content),
	)

	toolbar := buildWindowToolbar(deps, win)
	root := container.NewBorder(toolbar, nil, nil, nil, tabs)

	// Wrap with tooltip layer so ttwidget tooltips work inside the window
	// (otherwise fyne-tooltip warns "no tool tip layer for current
	// overlay"). Same pattern as ui/configurator/configurator.go and
	// source_edit_window.go.
	win.SetContent(fynetooltip.AddWindowToolTipLayer(root, win.Canvas()))
	win.Resize(fyne.NewSize(720, 520))
	win.CenterOnScreen()

	// Close intercept: just close, don't quit. The profiler keeps
	// running in the background (rolling buffer + active session
	// continue) so re-opening shows accumulated state immediately.
	win.SetOnClosed(func() {
		live.Stop()
		perProcess.Stop()
		m.mu.Lock()
		m.win = nil
		m.mu.Unlock()
		m.stopTitleTimer()
		if deps.ParentRefresh != nil {
			deps.ParentRefresh()
		}
	})

	m.mu.Lock()
	m.win = win
	m.mu.Unlock()

	// Drive a once-per-second window title refresh while open so the
	// recording timer ticks up in the title bar.
	m.startTitleTimer()
	// Initial title — defer to next UI tick via goroutine + fyne.Do.
	// CRITICAL: m.refreshTitle() invokes m.deps.ParentRefresh() which
	// dispatches fyne.Do(...). Calling fyne.Do FROM the UI thread (we are
	// here — button OnTap → Show() → build()) deadlocks Fyne 2.7 because
	// fyne.Do blocks waiting for the UI thread, which is busy in this
	// stack. Wrap in goroutine to ensure fyne.Do runs after Show() returns.
	go func() {
		fyne.Do(func() { m.refreshTitle() })
	}()

	win.Show()
}

// refreshTitle updates the window title + invokes ParentRefresh for the
// Diagnostics tab to re-render its button. Called on session start/stop
// and once per second while a session is active.
func (m *Manager) refreshTitle() {
	m.mu.Lock()
	w := m.win
	m.mu.Unlock()
	if w == nil {
		return
	}
	active := m.deps.Profiler.ActiveSession()
	title := tprof.FormatRecordingTitle(active)
	w.SetTitle(title)
	if m.deps.ParentRefresh != nil {
		m.deps.ParentRefresh()
	}
}

func (m *Manager) startTitleTimer() {
	m.stopTitleTimer()
	m.mu.Lock()
	stop := make(chan struct{})
	m.titleStopCh = stop
	m.mu.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fyne.Do(func() {
					m.refreshTitle()
				})
			}
		}
	}()
}

func (m *Manager) stopTitleTimer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.titleStopCh != nil {
		close(m.titleStopCh)
		m.titleStopCh = nil
	}
}

// formatBytes is shared by Live + Per-process views.
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}

// formatEventRow returns the short summary line for an event (one line
// in the Live list). Centralized so Live + Per-process Live agree.
func formatEventRow(e tprof.TrafficEvent) string {
	ts := e.TS.Format("15:04:05")
	switch e.Kind {
	case tprof.EventDNSResolve:
		ip := e.IP
		if ip == "" && len(e.CnameChain) > 0 {
			ip = "CNAME " + e.CnameChain[len(e.CnameChain)-1]
		}
		return fmt.Sprintf("%s  DNS  %s -> %s", ts, e.Domain, ip)
	case tprof.EventDNSFail:
		return fmt.Sprintf("%s  DNS×  %s  (failed)", ts, e.Domain)
	case tprof.EventTCPOpen:
		dom := e.Domain
		if dom == "" {
			dom = e.IP
		}
		return fmt.Sprintf("%s  TCP   %s:%d", ts, dom, e.Port)
	case tprof.EventTCPClose:
		return fmt.Sprintf("%s  TCP·  closed (up %s / down %s, %s)", ts, formatBytes(e.UpBytes), formatBytes(e.DownBytes), e.Duration.Truncate(time.Millisecond))
	case tprof.EventUDPOpen:
		dom := e.Domain
		if dom == "" {
			dom = e.IP
		}
		return fmt.Sprintf("%s  UDP   %s:%d", ts, dom, e.Port)
	case tprof.EventUDPClose:
		return fmt.Sprintf("%s  UDP·  closed (up %s / down %s)", ts, formatBytes(e.UpBytes), formatBytes(e.DownBytes))
	}
	return fmt.Sprintf("%s  %s", ts, e.Kind)
}

// Ensure widget package is referenced (silence unused for builds without it).
var _ = widget.NewLabel
