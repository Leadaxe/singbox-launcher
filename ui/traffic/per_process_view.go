package traffic

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
	"singbox-launcher/ui/components"
)

// perProcessView is the recording tab. Owns:
//   - target process picker
//   - START / STOP toolbar
//   - 4 inner sub-tabs (Live / Domains / IPs / Connections) bound to the
//     active session, or the saved sessions list when idle
type perProcessView struct {
	Content   fyne.CanvasObject
	onRefresh func() // called when recording state changes (window title + button badge)
	stopFn    func()

	mu              sync.Mutex
	deps            WindowDeps
	targetLabel     *widget.Label
	startStopBtn    *widget.Button
	statusLine      *widget.Label
	subTabs         *container.AppTabs
	liveItems       []tprof.TrafficEvent
	domainsData     []tprof.DomainStats
	ipsData         []tprof.IPStats
	connsData       []tprof.ConnRecord
	savedData       []*tprof.Session
	liveList        *widget.List
	domainsList     *widget.List
	ipsList         *widget.List
	connsList       *widget.List
	savedList       *widget.List
	body            *fyne.Container
	activeBody      fyne.CanvasObject
	idleBody        fyne.CanvasObject
	target          string
	targetDisplay   string
	unsub           func()
	refreshTickStop chan struct{}
	refresh         func()
}

func buildPerProcessView(deps WindowDeps, onRefresh func()) *perProcessView {
	v := &perProcessView{onRefresh: onRefresh, deps: deps}

	v.targetLabel = widget.NewLabel(locale.T("Target: (none)"))
	v.statusLine = widget.NewLabel("")
	v.startStopBtn = widget.NewButtonWithIcon(locale.T("Pick process…"), theme.MediaPlayIcon(), nil)
	v.startStopBtn.OnTapped = v.onStartStop

	// 4 sub-tab lists. Updaters read v.liveItems / v.domainsData etc.
	// Строки те же, что на главной вкладке Live: newLiveRow даёт колонки
	// outbound/ушло/пришло справа от текста события. Раньше здесь был голый
	// Label — одна и та же лента событий выглядела в двух местах по-разному, и
	// трафик соединения тут не показывался вовсе.
	v.liveList = widget.NewList(
		func() int { return len(v.liveItems) },
		func() fyne.CanvasObject { return newLiveRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			label, obL, upL, downL := liveRowParts(o.(*fyne.Container))
			if i < 0 || i >= len(v.liveItems) {
				label.SetText("")
				obL.SetText("")
				upL.SetText("")
				downL.SetText("")
				return
			}
			e := v.liveItems[len(v.liveItems)-1-i]
			line := formatEventRow(e)
			if e.Backfilled {
				line = "〽 " + line
			}
			if len(e.Issues) > 0 {
				line = "⚠ " + line
			}
			label.SetText(line)
			obL.SetText(eventOutbound(e))
			// Байты есть только у событий закрытия: у открытия их ещё нет, и
			// «0 B» там читалось бы как «ничего не передано».
			if e.UpBytes > 0 || e.DownBytes > 0 {
				upL.SetText(formatBytes(e.UpBytes))
				downL.SetText(formatBytes(e.DownBytes))
			} else {
				upL.SetText("")
				downL.SetText("")
			}
		},
	)
	// Click row → detail dialog. Unselect right away so the next click
	// (even on the same row) re-fires OnSelected.
	v.liveList.OnSelected = func(id widget.ListItemID) {
		v.mu.Lock()
		ok := id >= 0 && int(id) < len(v.liveItems)
		var e tprof.TrafficEvent
		if ok {
			e = v.liveItems[len(v.liveItems)-1-int(id)]
		}
		v.mu.Unlock()
		v.liveList.UnselectAll()
		if ok {
			showEventDetail(v.parentWindow(), e)
		}
	}

	v.domainsList = widget.NewList(
		func() int { return len(v.domainsData) },
		func() fyne.CanvasObject { return widget.NewLabel("...") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.domainsData) {
				return
			}
			d := v.domainsData[i]
			lbl := o.(*widget.Label)
			// Стрелка вплотную к числу и humanBytes без пробела перед единицей —
			// как в «By client», где те же стрелки рисуются нормально. Пробел
			// между стрелкой и числом тема Fyne показывает как «замену» (◆), и
			// неразрывный не спасает: его в шрифте тоже нет.
			line := fmt.Sprintf("%s  ↑%s ↓%s  (%d conn, %d IPs)", d.Domain, humanBytes(d.UpBytes), humanBytes(d.DownBytes), d.Connections, len(d.IPs))
			if len(d.Issues) > 0 {
				line = "⚠ " + line
			}
			lbl.SetText(line)
		},
	)
	// Клик по строке → карточка домена. Строка тут — свод по всем его
	// соединениям, и то, ради чего его открывают (цепочка CNAME, все адреса),
	// в неё не помещается: адреса сжаты до счётчика «1 IPs».
	v.domainsList.OnSelected = func(id widget.ListItemID) {
		v.mu.Lock()
		ok := id >= 0 && int(id) < len(v.domainsData)
		var d tprof.DomainStats
		if ok {
			d = v.domainsData[int(id)]
		}
		v.mu.Unlock()
		v.domainsList.UnselectAll()
		if ok {
			showDomainDetail(v.parentWindow(), d)
		}
	}

	v.ipsList = widget.NewList(
		func() int { return len(v.ipsData) },
		func() fyne.CanvasObject { return widget.NewLabel("...") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.ipsData) {
				return
			}
			d := v.ipsData[i]
			lbl := o.(*widget.Label)
			dom := d.Domain
			if dom == "" {
				dom = "(hostless)"
			}
			lbl.SetText(fmt.Sprintf("%s:%d  ↑%s ↓%s  %s", d.IP, d.Port, humanBytes(d.UpBytes), humanBytes(d.DownBytes), dom))
		},
	)
	v.ipsList.OnSelected = func(id widget.ListItemID) {
		v.mu.Lock()
		ok := id >= 0 && int(id) < len(v.ipsData)
		var s tprof.IPStats
		if ok {
			s = v.ipsData[int(id)]
		}
		v.mu.Unlock()
		v.ipsList.UnselectAll()
		if ok {
			showIPDetail(v.parentWindow(), s)
		}
	}

	// Колонками, как «By client» у машины: одной строкой через двойные пробелы
	// значения не выравнивались — шрифт пропорциональный, и хост с правилом
	// разной длины разъезжали, так что глазом столбец не читался. Ширины и
	// правила обрезки общие с той таблицей.
	v.connsList = widget.NewList(
		func() int { return len(v.connsData) },
		func() fyne.CanvasObject { return newConnRecordRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.connsData) {
				return
			}
			o.(*connRecordRow).set(v.connsData[i])
		},
	)
	// Клик по строке → те же детали, что в Live: колонки хоста и правила
	// обрезаны по ширине, и добраться до полного значения иначе нечем.
	v.connsList.OnSelected = func(id widget.ListItemID) {
		v.mu.Lock()
		ok := id >= 0 && int(id) < len(v.connsData)
		var rec tprof.ConnRecord
		if ok {
			rec = v.connsData[int(id)]
		}
		v.mu.Unlock()
		v.connsList.UnselectAll()
		if ok {
			showConnRecordDetail(v.parentWindow(), rec)
		}
	}

	// Saved sessions list — shown when no active session.
	v.savedList = widget.NewList(
		func() int { return len(v.savedData) },
		func() fyne.CanvasObject {
			open := widget.NewButton(locale.T("Open"), nil)
			del := widget.NewButton(locale.T("Delete"), nil)
			lbl := widget.NewLabel("...")
			// Gutter в конце строки: без него полоса прокрутки списка
			// наезжает на кнопку Delete (тот же дефект, что чинили в списке
			// серверов). Общий механизм — components.NewScrollGutter.
			return container.NewBorder(nil, nil, nil,
				container.NewHBox(open, del, components.NewScrollGutter()), lbl)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(v.savedData) {
				return
			}
			s := v.savedData[i]
			row := o.(*fyne.Container)
			lbl := row.Objects[0].(*widget.Label)
			btns := row.Objects[1].(*fyne.Container)
			open := btns.Objects[0].(*widget.Button)
			del := btns.Objects[1].(*widget.Button)
			d := s.Duration().Truncate(time.Second)
			lbl.SetText(fmt.Sprintf("%s · %s · %d events", shortPath(s.TargetProcess), d, len(s.Events())))
			open.OnTapped = func() {
				v.target = s.TargetProcess
				v.targetDisplay = shortPath(s.TargetProcess)
				// Show saved session in sub-tabs as read-only.
				v.liveItems = s.Events()
				v.domainsData = s.AggregateDomains()
				sort.Slice(v.domainsData, func(i, j int) bool {
					return (v.domainsData[i].UpBytes + v.domainsData[i].DownBytes) > (v.domainsData[j].UpBytes + v.domainsData[j].DownBytes)
				})
				v.ipsData = s.AggregateIPs()
				sort.Slice(v.ipsData, func(i, j int) bool {
					return (v.ipsData[i].UpBytes + v.ipsData[i].DownBytes) > (v.ipsData[j].UpBytes + v.ipsData[j].DownBytes)
				})
				v.connsData = s.AggregateConns()
				v.swap(true) // show sub-tabs even though no active session
				v.liveList.Refresh()
				v.domainsList.Refresh()
				v.ipsList.Refresh()
				v.connsList.Refresh()
			}
			del.OnTapped = func() {
				deps.Profiler.DeleteSession(s.ID)
				v.refresh()
			}
		},
	)

	// Sub-tabs + saved block; swap between them.
	v.subTabs = container.NewAppTabs(
		// Шапка колонок — та же, что на главной вкладке Live, и так же
		// закреплена над списком: внутри него она уезжала бы при прокрутке.
		container.NewTabItem(locale.T("Live"), container.NewBorder(
			container.NewVBox(liveHeaderRow(), widget.NewSeparator()),
			nil, nil, nil, v.liveList)),
		container.NewTabItem(locale.T("Domains"), v.domainsList),
		container.NewTabItem(locale.T("IPs"), v.ipsList),
		// Шапка колонок закреплена над списком, а не строкой внутри него:
		// иначе она уезжала бы при прокрутке, и столбцы теряли бы подписи.
		container.NewTabItem(locale.T("Connections"), container.NewBorder(
			container.NewVBox(connRecordRowHeader(), widget.NewSeparator()),
			nil, nil, nil, v.connsList)),
	)
	savedHeader := widget.NewLabel(locale.T("Saved sessions (last 5)"))
	v.activeBody = v.subTabs
	v.idleBody = container.NewBorder(savedHeader, nil, nil, nil, v.savedList)

	v.body = container.NewStack()

	toolbar := container.NewBorder(nil, nil, v.targetLabel, v.startStopBtn, nil)
	header := container.NewVBox(toolbar, v.statusLine, widget.NewSeparator())
	v.Content = container.NewBorder(header, nil, nil, nil, v.body)

	v.refresh = func() {
		active := deps.Profiler.ActiveSession()
		var displayTitle string
		if v.target != "" {
			disp := v.targetDisplay
			if disp == "" {
				disp = shortPath(v.target)
			}
			displayTitle = "Target: " + disp
		} else {
			displayTitle = "Target: (pick a process)"
		}
		v.targetLabel.SetText(displayTitle)
		if active != nil {
			v.startStopBtn.SetText(locale.T("STOP"))
			v.startStopBtn.SetIcon(theme.MediaStopIcon())
			v.liveItems = active.Events()
			v.domainsData = active.AggregateDomains()
			sort.Slice(v.domainsData, func(i, j int) bool {
				return (v.domainsData[i].UpBytes + v.domainsData[i].DownBytes) > (v.domainsData[j].UpBytes + v.domainsData[j].DownBytes)
			})
			v.ipsData = active.AggregateIPs()
			sort.Slice(v.ipsData, func(i, j int) bool {
				return (v.ipsData[i].UpBytes + v.ipsData[i].DownBytes) > (v.ipsData[j].UpBytes + v.ipsData[j].DownBytes)
			})
			v.connsData = active.AggregateConns()
			drops := active.EventsDropped()
			footer := ""
			if drops > 0 {
				footer = fmt.Sprintf("  (events dropped: %d)", drops)
			}
			v.statusLine.SetText(fmt.Sprintf("⏺ Recording · %s · %d domains · %d IPs · %d ev%s",
				active.Duration().Truncate(time.Second), len(v.domainsData), len(v.ipsData), len(v.liveItems), footer))
			v.swap(true)
		} else {
			v.startStopBtn.SetText(locale.T("Pick process & START"))
			v.startStopBtn.SetIcon(theme.MediaPlayIcon())
			// If a saved session is currently being shown (target set
			// while idle), keep sub-tabs visible; else show idle list.
			if v.target == "" {
				v.liveItems = nil
				v.domainsData = nil
				v.ipsData = nil
				v.connsData = nil
				v.savedData = reverseSessions(deps.Profiler.CompletedSessions())
				v.statusLine.SetText(locale.T("Idle — pick a process and START to begin recording."))
				v.swap(false)
			} else {
				v.statusLine.SetText(locale.T("Read-only view of saved session — click START to record a new one."))
			}
		}
		v.liveList.Refresh()
		v.domainsList.Refresh()
		v.ipsList.Refresh()
		v.connsList.Refresh()
		v.savedList.Refresh()
		if v.onRefresh != nil {
			v.onRefresh()
		}
	}

	v.refreshTickStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-v.refreshTickStop:
				return
			case <-ticker.C:
				fyne.Do(v.refresh)
			}
		}
	}()

	ch, unsub := deps.Profiler.Subscribe()
	v.unsub = unsub
	go func() {
		for range ch {
			fyne.Do(v.refresh)
		}
	}()

	v.refresh()
	v.stopFn = func() {
		if v.unsub != nil {
			v.unsub()
			v.unsub = nil
		}
		if v.refreshTickStop != nil {
			close(v.refreshTickStop)
			v.refreshTickStop = nil
		}
	}
	return v
}

// swap shows either the sub-tabs body (active=true) or the saved list
// (active=false). Wrapped here so callers don't duplicate the
// body.Refresh dance.
func (v *perProcessView) swap(active bool) {
	v.body.Objects = nil
	if active {
		v.body.Add(v.activeBody)
	} else {
		v.body.Add(v.idleBody)
	}
	v.body.Refresh()
}

// onStartStop is the button callback. Either picks a process and starts
// a session, or stops the active one.
func (v *perProcessView) onStartStop() {
	if active := v.deps.Profiler.ActiveSession(); active != nil {
		if _, err := v.deps.Profiler.StopSession(); err != nil {
			parent := v.parentWindow()
			if parent != nil {
				dialog.ShowError(err, parent)
			}
			return
		}
		v.refresh()
		return
	}
	parent := v.parentWindow()
	if parent == nil {
		return
	}
	ShowProcessPicker(parent, v.deps.Profiler, func(path, display string) {
		v.target = path
		v.targetDisplay = display
		if _, err := v.deps.Profiler.StartSession(path, false); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		v.refresh()
	})
}

// Stop unsubscribes goroutines. Window close triggers this.
func (v *perProcessView) Stop() {
	if v.stopFn != nil {
		v.stopFn()
	}
}

// parentWindow — окно профайлера, к которому цепляются карточки строк.
//
// По ПРЕФИКСУ, а не по полному совпадению: во время записи заголовок несёт
// таймер («Traffic Profiler ⚡ Recording · 00:29», см. FormatRecordingTitle),
// и точное сравнение переставало находить своё окно ровно тогда, когда по
// строкам и кликают — запись идёт. Диалог уезжал в первое попавшееся окно
// приложения, то есть визуально никуда.
func (v *perProcessView) parentWindow() fyne.Window {
	if v.deps.App == nil {
		return nil
	}
	all := v.deps.App.Driver().AllWindows()
	for _, w := range all {
		if strings.HasPrefix(w.Title(), "Traffic Profiler") || w.Title() == "" {
			return w
		}
	}
	if len(all) > 0 {
		return all[0]
	}
	return nil
}

// reverseSessions returns a fresh slice with newest-last reversed to
// newest-first. The profiler's CompletedSessions ring keeps newest at
// the end (FIFO push); the UI wants newest at the top.
func reverseSessions(s []*tprof.Session) []*tprof.Session {
	out := make([]*tprof.Session, len(s))
	for i := range s {
		out[len(s)-1-i] = s[i]
	}
	return out
}
