package traffic

import (
	"net"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
)

// liveView is the system-wide stream tab. Maintains an in-memory ring of
// the most recent events, plus client-side filters (kind chips + search
// box + per-process filter). All filtering is done on display — the
// profiler streams everything to us.
type liveView struct {
	Content fyne.CanvasObject

	mu      sync.Mutex
	events  []tprof.TrafficEvent
	filter  liveFilter
	list    *widget.List
	statusL *widget.Label
	unsub   func()

	// Списки маршрутных фильтров и отпечаток их последнего набора:
	// перезаполнять на каждом тике значило бы схлопывать раскрытый список
	// под курсором.
	clientSel   *widget.Select
	outboundSel *widget.Select
	ruleSel     *widget.Select
	optsSig     string

	// paused — when true, new events from the profiler subscription are
	// dropped instead of appended to v.events. The subscription itself
	// stays open (profiler keeps recording in background); pause just
	// freezes the live-view buffer so the user can read what's on screen
	// without it scrolling away. Set via the Pause/Resume toggle.
	paused bool
}

// liveFilter is the user's current filter state. Defaults: everything on,
// no search.
type liveFilter struct {
	ShowDNS      bool
	ShowDNSFail  bool
	ShowTCP      bool
	ShowTCPClose bool
	ShowUDP      bool
	Search       string
	Process      string // empty = all processes
	// Три списка из вкладки By client: перечислимое отбирается точным
	// совпадением, а не поиском по подстроке. Пустое значение — «все».
	Client   string
	Outbound string
	Rule     string
}

func defaultLiveFilter() liveFilter {
	return liveFilter{
		ShowDNS:      true,
		ShowDNSFail:  true,
		ShowTCP:      true,
		ShowTCPClose: true,
		ShowUDP:      true,
	}
}

// liveViewRingSize caps the live view's in-memory list. Without a cap a
// chatty target could grow it forever; 5000 fits comfortably in a Fyne
// List and ~10 MB of RAM.
const liveViewRingSize = 5000

func buildLiveView(deps WindowDeps) *liveView {
	v := &liveView{filter: defaultLiveFilter()}

	v.statusL = widget.NewLabel("")
	updateStatus := func() {
		v.mu.Lock()
		n := len(v.events)
		v.mu.Unlock()
		v.statusL.SetText("Events in buffer: " + itoa(n) + "  (newest first)")
		v.syncOptions()
	}

	// Backfill from rolling buffer so user sees something immediately.
	snap := deps.Profiler.Snapshot(60 * time.Second)
	v.events = append(v.events, snap...)

	v.list = widget.NewList(
		func() int {
			v.mu.Lock()
			defer v.mu.Unlock()
			return len(v.filteredIndices())
		},
		// Строка = текст события слева + колонки outbound/↑/↓ справа.
		// Колонки фиксированной ширины, иначе при пропорциональном шрифте они
		// разъезжались бы от строки к строке.
		func() fyne.CanvasObject {
			return newLiveRow()
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			v.mu.Lock()
			idxs := v.filteredIndices()
			row := o.(*fyne.Container)
			label, obL, upL, downL := liveRowParts(row)
			if i < 0 || i >= len(idxs) {
				v.mu.Unlock()
				// Clear stale text — Fyne's List.Refresh() doesn't reliably
				// re-query length when filter narrows the visible set, so
				// previously-rendered rows can linger with old content.
				// Defensive blank keeps the UI honest.
				label.SetText("")
				obL.SetText("")
				upL.SetText("")
				downL.SetText("")
				return
			}
			// Newest-first display: reverse the FILTERED indices, not the
			// whole events ring. Was previously `events[len(events)-1-idxs[i]]`
			// which only happened to work when every event passed the filter
			// — partial filters landed on the wrong indices (and on a 0-match
			// filter, the early-return-without-clear above made it look like
			// the filter did nothing at all).
			e := v.events[idxs[len(idxs)-1-i]]
			v.mu.Unlock()
			line := formatEventRow(e)
			if e.ProcessName != "" {
				line += "   [" + e.ProcessName + "]"
			} else if e.ProcessPath != "" {
				line += "   [" + shortPath(e.ProcessPath) + "]"
			} else if e.SourceAddr != "" {
				// Удалённая машина: процесса нет, зато известен адрес
				// клиента — по нему и различаются устройства в сети.
				line += "   [" + hostOfAddr(e.SourceAddr) + "]"
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
	// Click row → detail dialog with every field of the event. Same UX
	// as per-process Live sub-tab. Unselect immediately so re-clicking
	// the same row re-fires.
	v.list.OnSelected = func(id widget.ListItemID) {
		v.mu.Lock()
		idxs := v.filteredIndices()
		var e tprof.TrafficEvent
		ok := id >= 0 && int(id) < len(idxs)
		if ok {
			e = v.events[idxs[len(idxs)-1-int(id)]]
		}
		v.mu.Unlock()
		v.list.UnselectAll()
		if ok {
			// Карточка устройства и здесь: строка Live несёт адрес клиента,
			// а «кто это» отвечает справочник машины.
			showEventDetailWithDevice(parentWindowOf(deps), e, deviceFor(deps, e.SourceAddr))
		}
	}

	// Filter controls.
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search domain / IP / process…")
	searchEntry.OnChanged = func(s string) {
		v.mu.Lock()
		v.filter.Search = strings.ToLower(strings.TrimSpace(s))
		v.mu.Unlock()
		v.list.Refresh()
	}

	mkCheck := func(label string, get func(*liveFilter) *bool) *widget.Check {
		c := widget.NewCheck(label, func(b bool) {
			v.mu.Lock()
			*get(&v.filter) = b
			v.mu.Unlock()
			v.list.Refresh()
		})
		c.SetChecked(true)
		return c
	}
	chipDNS := mkCheck("DNS", func(f *liveFilter) *bool { return &f.ShowDNS })
	chipDNSx := mkCheck("DNS×", func(f *liveFilter) *bool { return &f.ShowDNSFail })
	chipTCP := mkCheck("TCP", func(f *liveFilter) *bool { return &f.ShowTCP })
	chipTCPc := mkCheck("TCP·", func(f *liveFilter) *bool { return &f.ShowTCPClose })
	chipUDP := mkCheck("UDP", func(f *liveFilter) *bool { return &f.ShowUDP })

	// Pause/Resume — freezes the live-view buffer so the user can read the
	// current snapshot without it scrolling away. The background profiler
	// keeps recording (the rolling buffer + any active session continue);
	// only the in-tab append is gated. Toggle flips on each click.
	pauseBtn := widget.NewButton("⏸ Pause", nil)
	pauseBtn.OnTapped = func() {
		v.mu.Lock()
		v.paused = !v.paused
		paused := v.paused
		v.mu.Unlock()
		if paused {
			pauseBtn.SetText("▶ Resume")
		} else {
			pauseBtn.SetText("⏸ Pause")
		}
	}
	// Clear — drops local view buffer (does NOT touch profiler's rolling
	// buffer or any recording session). Cheap reset for noisy screens.
	clearBtn := widget.NewButton("🗑 Clear", func() {
		v.mu.Lock()
		v.events = v.events[:0]
		v.mu.Unlock()
		v.list.Refresh()
		updateStatus()
	})

	filterRow := container.NewHBox(
		chipDNS, chipDNSx, chipTCP, chipTCPc, chipUDP,
		layout.NewSpacer(),
		pauseBtn, clearBtn,
	)

	// Три списка из вкладки By client: клиент, outbound, правило. Чипы выше
	// отбирают ВИД события, эти — маршрут, и заменить одно другим нельзя.
	// Имя фильтра — внутри самого списка: подписи над ними стоили целого ряда
	// высоты, а панель фильтров в Live и без того высокая.
	mkSelect := func(placeholder string, set func(string)) *widget.Select {
		s := widget.NewSelect(nil, func(val string) {
			v.mu.Lock()
			set(valueOrEmpty(val))
			v.mu.Unlock()
			v.list.Refresh()
		})
		s.PlaceHolder = placeholder
		return s
	}
	v.clientSel = mkSelect(locale.T("traffic.byclient.client"), func(s string) { v.filter.Client = s })
	v.outboundSel = mkSelect(locale.T("traffic.byclient.outbound"), func(s string) { v.filter.Outbound = s })
	v.ruleSel = mkSelect(locale.T("traffic.byclient.rule"), func(s string) { v.filter.Rule = s })

	routeRow := container.NewGridWithColumns(3, v.clientSel, v.outboundSel, v.ruleSel)

	top := container.NewVBox(
		searchEntry,
		routeRow,
		filterRow,
		v.statusL,
		liveHeaderRow(),
		widget.NewSeparator(),
	)

	bannerVBox := container.NewVBox()
	rebuildBanners := func() {
		bannerVBox.Objects = nil
		if deps.SingBoxRunning != nil && !deps.SingBoxRunning() {
			bannerVBox.Add(buildBanner("Sing-box is not running — live events will appear after Start."))
		}
		if deps.FindProcessEnabled != nil && !deps.FindProcessEnabled() {
			bannerVBox.Add(buildBanner("Process detection disabled in template — events will lack process attribution. Enable route.find_process and Save in the wizard."))
		}
		bannerVBox.Refresh()
	}
	rebuildBanners()

	body := container.NewBorder(
		container.NewVBox(bannerVBox, top),
		nil, nil, nil,
		v.list,
	)
	v.Content = body
	updateStatus()

	// Subscribe and pump events. fyne.Do to marshal onto UI thread.
	// Pause gating: we always drain the channel (otherwise the profiler's
	// subscriber backpressure would build up) but skip the append+refresh
	// when v.paused is true. The profiler's own rolling buffer + any
	// active recording session continue regardless of UI pause state.
	ch, unsub := deps.Profiler.Subscribe()
	v.unsub = unsub
	go func() {
		for e := range ch {
			ee := e
			v.mu.Lock()
			if v.paused {
				v.mu.Unlock()
				continue
			}
			v.events = append(v.events, ee)
			if len(v.events) > liveViewRingSize {
				v.events = v.events[len(v.events)-liveViewRingSize:]
			}
			v.mu.Unlock()
			fyne.Do(func() {
				v.list.Refresh()
				updateStatus()
				rebuildBanners()
			})
		}
	}()

	return v
}

// Stop unsubscribes the view from the profiler. Called when the window
// closes.
func (v *liveView) Stop() {
	if v.unsub != nil {
		v.unsub()
		v.unsub = nil
	}
}

// filteredIndices returns indices into v.events (oldest→newest order)
// that pass the current filter. The list widget reverses for display.
// Caller must hold v.mu.
func (v *liveView) filteredIndices() []int {
	out := make([]int, 0, len(v.events))
	for i, e := range v.events {
		if !v.passes(e) {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (v *liveView) passes(e tprof.TrafficEvent) bool {
	switch e.Kind {
	case tprof.EventDNSResolve:
		if !v.filter.ShowDNS {
			return false
		}
	case tprof.EventDNSFail:
		if !v.filter.ShowDNSFail {
			return false
		}
	case tprof.EventTCPOpen:
		if !v.filter.ShowTCP {
			return false
		}
	case tprof.EventTCPClose:
		if !v.filter.ShowTCPClose {
			return false
		}
	case tprof.EventUDPOpen, tprof.EventUDPClose:
		if !v.filter.ShowUDP {
			return false
		}
	}
	if v.filter.Process != "" && e.ProcessPath != v.filter.Process {
		return false
	}
	// Клиент сравнивается без порта: в списке он тоже без порта, а каждое
	// соединение приходит со своим — иначе фильтр не совпал бы никогда.
	if v.filter.Client != "" && hostOfAddr(e.SourceAddr) != v.filter.Client {
		return false
	}
	if v.filter.Outbound != "" && eventOutbound(e) != v.filter.Outbound {
		return false
	}
	if v.filter.Rule != "" && e.Rule != v.filter.Rule {
		return false
	}
	if s := v.filter.Search; s != "" {
		hay := strings.ToLower(e.Domain + " " + e.IP + " " + e.ProcessPath + " " + e.ProcessName)
		if !strings.Contains(hay, s) {
			return false
		}
	}
	return true
}

// Ширины правых колонок Live. Трафик уже, чем в By client: тут краткая
// форма formatBytes, а не humanBytes.
const (
	liveColOutbound = 110
	liveColBytes    = 74
)

// newLiveRow строит каркас строки: растяжимый текст события слева, три
// колонки постоянной ширины справа.
func newLiveRow() fyne.CanvasObject {
	text := widget.NewLabel("")
	text.Truncation = fyne.TextTruncateEllipsis
	ob := widget.NewLabel("")
	ob.Truncation = fyne.TextTruncateEllipsis
	up := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
	down := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
	right := container.NewHBox(
		fixedWidth(ob, liveColOutbound),
		fixedWidth(up, liveColBytes),
		fixedWidth(down, liveColBytes),
	)
	return container.NewBorder(nil, nil, nil, right, text)
}

// liveHeaderRow — подписи правых колонок.
//
// Собирается тем же newLiveRow, что и строки списка: ширины тогда совпадают
// по построению, а не потому, что их не забыли поправить в двух местах.
// Список Fyne рисует свои строки с отступом, которого нет у шапки, — на него
// и сдвигаем правый блок.
func liveHeaderRow() fyne.CanvasObject {
	row := newLiveRow().(*fyne.Container)
	text, ob, up, down := liveRowParts(row)
	bold := fyne.TextStyle{Bold: true}
	text.TextStyle, ob.TextStyle, up.TextStyle, down.TextStyle = bold, bold, bold, bold
	text.SetText(locale.T("traffic.live.col_event"))
	ob.SetText(locale.T("traffic.byclient.col_outbound"))
	// Словами, а не стрелками: «↑» одинаково читается и как исходящий, и как
	// «вверх по списку». Ушло = от клиента наружу.
	up.SetText(locale.T("traffic.col_sent"))
	down.SetText(locale.T("traffic.col_recv"))
	return container.NewBorder(nil, nil, nil,
		container.NewHBox(row.Objects[1], fixedWidth(widget.NewLabel(""), liveHeaderPad)),
		row.Objects[0])
}

// liveHeaderPad — поправка на внутренний отступ строки списка: без неё
// подписи стоят правее своих колонок на ширину полосы прокрутки.
const liveHeaderPad = 8

// liveRowParts достаёт виджеты из каркаса newLiveRow.
//
// Border кладёт центр первым, правый блок — вторым; порядок задан здесь же,
// и держать его в одном месте с конструктором надёжнее, чем искать по индексам
// в обработчике.
func liveRowParts(row *fyne.Container) (text, outbound, up, down *widget.Label) {
	text = row.Objects[0].(*widget.Label)
	right := row.Objects[1].(*fyne.Container)
	unwrap := func(i int) *widget.Label {
		return right.Objects[i].(*fyne.Container).Objects[0].(*widget.Label)
	}
	return text, unwrap(0), unwrap(1), unwrap(2)
}

// eventOutbound — корень цепочки, то есть выбранный outbound. Порядок
// leaf→root, как в types.go.
func eventOutbound(e tprof.TrafficEvent) string {
	if len(e.OutboundChain) == 0 {
		return ""
	}
	return e.OutboundChain[len(e.OutboundChain)-1]
}

// liveOptions собирает варианты трёх списков из буфера событий.
//
// Из буфера, а не из снимка соединений: Live показывает и уже закрытые
// события, и фильтр обязан уметь отобрать то, что видно на экране. Caller
// must hold v.mu.
func (v *liveView) liveOptions() (clients, outbounds, rules []tprof.OptionCount) {
	cm, om, rm := map[string]int{}, map[string]int{}, map[string]int{}
	for _, e := range v.events {
		if h := hostOfAddr(e.SourceAddr); h != "" {
			cm[h]++
		}
		if ob := eventOutbound(e); ob != "" {
			om[ob]++
		}
		if e.Rule != "" {
			rm[e.Rule]++
		}
	}
	return tprof.SortedOptions(cm), tprof.SortedOptions(om), tprof.SortedOptions(rm)
}

// syncOptions перезаполняет списки маршрутных фильтров по буферу событий.
func (v *liveView) syncOptions() {
	if v.clientSel == nil {
		return // ещё строимся
	}
	v.mu.Lock()
	clients, outbounds, rules := v.liveOptions()
	sel := v.filter
	v.mu.Unlock()

	var b strings.Builder
	for _, g := range [][]tprof.OptionCount{clients, outbounds, rules} {
		for _, c := range g {
			b.WriteString(c.Value)
			b.WriteByte('\x00')
		}
		b.WriteByte('\x01')
	}
	if sig := b.String(); sig != v.optsSig {
		v.optsSig = sig
		setOptions(v.clientSel, clients, sel.Client)
		setOptions(v.outboundSel, outbounds, sel.Outbound)
		setOptions(v.ruleSel, rules, sel.Rule)
	}
}

func buildBanner(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return container.NewVBox(l, widget.NewSeparator())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	// avoid strconv import bloat — small custom impl
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func shortPath(p string) string {
	// trim to basename for display
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	if i := strings.LastIndex(p, "\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// hostOfAddr отрезает порт от "ip:port": в списке важен клиент, а порт у
// каждого соединения свой и только зашумляет строку.
func hostOfAddr(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}
