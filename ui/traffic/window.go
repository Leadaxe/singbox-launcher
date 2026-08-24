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
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"singbox-launcher/internal/locale"
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

	// RemoteMachine — окно наблюдает за УДАЛЁННОЙ машиной, а не за своим
	// ядром. Меняет состав вкладок: процессов там нет вовсе (трафик идут от
	// устройств в сети, find_process на роутере смысла не имеет), поэтому
	// вместо «Per-process» показывается разбивка по клиентам — исходным
	// IP:port из потока соединений.
	RemoteMachine bool

	// CloseConns рвёт соединения по их id. Есть только у remote-окна:
	// на роутере это осмысленное действие — после смены правила устройство
	// иначе доживает на прежних сессиях и идёт старым маршрутом.
	CloseConns func(ids []string)

	// ClientsInfo — справочник «IP → устройство» локальной сети машины
	// (аренды DHCP, ARP, мост, Wi-Fi, свои метки). Только у remote-окна: у
	// своего ядра трафик идёт от процессов, а не от устройств сети.
	//
	// Это таблица подстановки, а не поле соединения: имена меняются в
	// масштабе часов, и join делает UI. ok=false, пока справочник не получен.
	ClientsInfo func() (map[string]DeviceInfo, bool)

	// SetClientLabel задаёт своё имя устройству (ключ — IP или MAC).
	SetClientLabel func(key, name string) error
	// DeleteClientLabel снимает своё имя.
	DeleteClientLabel func(key string) error
}

// DeviceInfo — что известно об устройстве локальной сети.
//
// Пустое поле — состояние, а не ошибка: у проводного клиента нет SSID, а вне
// Linux часть провайдеров молчит вовсе.
type DeviceInfo struct {
	Name  string
	MAC   string
	SSID  string
	Iface string
	Port  string
	// Source — какие провайдеры заполнили запись. При разборе «почему
	// устройство потеряло имя» это первый вопрос: одинокий label означает,
	// что аренда DHCP истекла.
	Source string
}

// Manager owns the singleton window pointer. There's one Manager per
// running app — instantiated from the UI layer wiring (ui/app.go or
// equivalent) and reused by the Diagnostics button.
type Manager struct {
	mu          sync.Mutex
	win         fyne.Window
	deps        WindowDeps
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

	var tabs *container.AppTabs
	// Останов второй вкладки: у режимов она разная, и держать обе ссылки
	// значило бы разбирать nil на закрытии.
	var stopSecond func()
	if deps.RemoteMachine {
		// У машины нет процессов — есть клиенты. Показывать пикер процессов
		// значило бы предлагать выбрать из списка процессов ЭТОГО компьютера,
		// к её трафику отношения не имеющих.
		// Своё окно передаём явно: заголовок «Traffic Profiler» одинаков у
		// локального окна и у окон машин, они открыты одновременно, и поиск
		// родителя по заголовку выбросил бы диалог в чужое окно.
		byClient := buildByClientView(deps, win)
		stopSecond = byClient.Stop
		tabs = container.NewAppTabs(
			container.NewTabItem("Live", live.Content),
			container.NewTabItem("By client", byClient.Content),
		)
	} else {
		perProcess := buildPerProcessView(deps, func() {
			// Defer to avoid synchronous re-entry into m.refreshTitle's mutex
			// while we're still inside build()'s UI-thread stack. The first
			// real refresh fires from startTitleTimer / profiler subscriber
			// shortly anyway.
			go func() { fyne.Do(func() { m.refreshTitle() }) }()
		})
		stopSecond = perProcess.Stop
		tabs = container.NewAppTabs(
			container.NewTabItem("Live", live.Content),
			container.NewTabItem("Per-process", perProcess.Content),
		)
	}

	// nil для окна машины — Border просто не рисует верхнюю полосу.
	toolbar := buildWindowToolbar(deps, win)

	// Счётчик соединений и «разорвать все» кладём В полосу вкладок, справа:
	// своя строка стоила бы ещё одного ряда высоты, а место там пустует.
	var tabsRow fyne.CanvasObject
	if stopCounter := m.attachConnCounter(deps, win, tabs); stopCounter != nil {
		tabsRow = stopCounter.content
		prevStop := stopSecond
		stopSecond = func() {
			prevStop()
			stopCounter.stop()
		}
	} else {
		// Счётчика нет (локальное окно) — ⋮ всё равно живёт в полосе вкладок.
		// Иначе он оставался бы единственным жильцом тулбара, и целая строка
		// уходила бы под одну кнопку.
		tabsRow = overlayOnTabs(tabs, buildOverflowButton(deps, win))
	}
	root := container.NewBorder(toolbar, nil, nil, nil, tabsRow)

	// Wrap with tooltip layer so ttwidget tooltips work inside the window
	// (otherwise fyne-tooltip warns "no tool tip layer for current
	// overlay"). Same pattern as ui/configurator/configurator.go and
	// source_edit_window.go.
	win.SetContent(fynetooltip.AddWindowToolTipLayer(root, win.Canvas()))
	// Окно машины шире: таблица By client несёт семь колонок, и на 720 хвост с
	// outbound'ом и правилом — ровно то, ради чего её открывают, — уезжает за
	// край.
	if deps.RemoteMachine {
		win.Resize(fyne.NewSize(900, 560))
	} else {
		win.Resize(fyne.NewSize(720, 520))
	}
	win.CenterOnScreen()

	// Close intercept: just close, don't quit. The profiler keeps
	// running in the background (rolling buffer + active session
	// continue) so re-opening shows accumulated state immediately.
	win.SetOnClosed(func() {
		live.Stop()
		stopSecond()
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
// connCounter — счётчик соединений и «разорвать все», наложенные на правый
// край полосы вкладок.
type connCounter struct {
	content fyne.CanvasObject
	stop    func()
}

// attachConnCounter кладёт справа от вкладок число открытых соединений и
// кнопку обрыва.
//
// Только для окна машины: рвать соединения своего ядра из профайлера нечем —
// CloseConns есть лишь у remote-окна, где это осмысленное действие после
// смены правила.
func (m *Manager) attachConnCounter(deps WindowDeps, win fyne.Window, tabs *container.AppTabs) *connCounter {
	if deps.CloseConns == nil || deps.Profiler == nil {
		return nil
	}

	countL := widget.NewLabel("")
	killBtn := widget.NewButton(locale.T("Close all"), func() {
		ids, ok := deps.Profiler.LiveConnIDs()
		if !ok || len(ids) == 0 {
			return
		}
		// Спрашиваем: действие рвёт ВСЁ разом, включая чужие сессии на
		// роутере, и промахнуться мимо кнопки легко — она рядом с вкладками.
		dialog.ShowConfirm(
			locale.T("Close all"),
			fmt.Sprintf(locale.T("Close all connections (%d)? Devices will reconnect on their own."), len(ids)),
			func(yes bool) {
				if yes {
					deps.CloseConns(ids)
				}
			}, win)
	})
	killBtn.Importance = widget.LowImportance

	refresh := func() {
		ids, ok := deps.Profiler.LiveConnIDs()
		if !ok {
			// До первого кадра стрима «0» означал бы «соединений нет», хотя мы
			// просто ещё не знаем.
			countL.SetText(locale.T("conns: …"))
			killBtn.Disable()
			return
		}
		countL.SetText(fmt.Sprintf(locale.T("conns: %d"), len(ids)))
		if len(ids) == 0 {
			killBtn.Disable()
		} else {
			killBtn.Enable()
		}
	}
	refresh()

	stopCh := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				fyne.Do(refresh)
			}
		}
	}()

	return &connCounter{
		content: overlayOnTabs(tabs, countL, killBtn, buildOverflowButton(deps, win)),
		stop:    func() { close(stopCh) },
	}
}

// overlayOnTabs кладёт элементы в правый край полосы вкладок.
//
// Поверх вкладок, а не рядом: AppTabs занимает всю ширину, и в Border справа
// он бы сжался, оставив ярлыки обрезанными.
//
// Верхний слой обязан быть ростом с сами элементы. Border, у которого занят
// только top-слот, растягивается на всю высоту контейнера, и его пустая
// середина накрывает список под вкладками — клики по строкам туда и уходили.
// В окне машины это не проявлялось: там строки нарисованы кнопками, а они
// перехватывают нажатие сами; в локальном окне списки живут на OnSelected.
func overlayOnTabs(tabs *container.AppTabs, objs ...fyne.CanvasObject) fyne.CanvasObject {
	right := container.NewHBox(objs...)
	// VBox прижимает строку к верху и НЕ растягивает её по высоте — ниже
	// остаётся пустое место, сквозь которое список получает свои клики.
	overlay := container.NewVBox(container.NewHBox(layout.NewSpacer(), right))
	return container.NewStack(tabs, overlay)
}

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
		// Байты ушли в правые колонки строки — здесь остаётся длительность,
		// иначе одно и то же печаталось бы дважды.
		return fmt.Sprintf("%s  TCP·  closed (%s)", ts, e.Duration.Truncate(time.Millisecond))
	case tprof.EventUDPOpen:
		dom := e.Domain
		if dom == "" {
			dom = e.IP
		}
		return fmt.Sprintf("%s  UDP   %s:%d", ts, dom, e.Port)
	case tprof.EventUDPClose:
		return fmt.Sprintf("%s  UDP·  closed", ts)
	}
	return fmt.Sprintf("%s  %s", ts, e.Kind)
}

// Ensure widget package is referenced (silence unused for builds without it).
var _ = widget.NewLabel
