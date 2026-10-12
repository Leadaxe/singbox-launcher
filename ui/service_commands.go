package ui

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
)

// Строка команд демона в шапке окна Service (SPEC 162 §4.2–§4.3): селектор
// имён из GET /admin/commands, Run, текст команды, последний исход и
// свёртываемый лог прогона. Клиент передаёт демону только имя — ни
// аргументов, ни тела (контракт безопасности SPEC 128), поэтому полей ввода
// здесь нет.
//
// Команда update обрабатывается особо (serviceUpdateRun): демон
// перезапускает сам себя, обрыв связи — штатный путь, окно ждёт его
// возвращения и сверяет версию.
//
// Строка создаётся один раз на окно и переживает перестройку шапки; всё
// состояние — в UI-потоке, сеть — через serviceCommander (горутины, done в
// UI-потоке).

const (
	serviceCommandsHintText    = "add \"commands\" to daemon.json → Troubleshooting → Suggested commands"
	serviceUpdateConfirmText   = "The daemon will download the latest core (~60 MB) and restart itself. Connections through it will drop for a few seconds. Continue?"
	serviceUpdateNoCommandText = "✔ Updated to %s. Commands are not available in this core version"
	serviceUpdateRolledText    = "✖ %s did not come up, rolled back to %s"
	serviceReconnectingText    = "Restarting the daemon — reconnecting… %d s"
	serviceCommandGoneText     = "✖ The command is no longer in daemon.json"
)

const (
	// serviceCommandPoll — период опроса прогона (GET по имени).
	serviceCommandPoll = time.Second
	// serviceUpdateWait — сколько ждать возвращения демона после update.
	serviceUpdateWait = 90 * time.Second
	// serviceCommandLogMinHeight — высота раскрытого лога прогона.
	serviceCommandLogMinHeight = 140
	// serviceCommandUpdate / serviceCommandCheckUpdate — имена из рецепта
	// Suggested commands (§4.5); другие имена идут общим путём.
	serviceCommandUpdate      = "update"       // l10n-exempt: daemon.json command name
	serviceCommandCheckUpdate = "check-update" // l10n-exempt: daemon.json command name
)

// serviceReconnectDelays — паузы перед попытками достучаться до демона
// после update; дальше — последняя.
var serviceReconnectDelays = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// serviceUpdateVersionsRe — строка `lxd update` «installed X, available Y».
var serviceUpdateVersionsRe = regexp.MustCompile(`installed (\S+).*available (\S+)`)

type serviceCommandBar struct {
	v *serviceView

	sel       *widget.Select
	run       *widget.Button
	cmdText   *widget.Label
	outcome   *ttwidget.Label
	logText   *widget.Label
	logScroll *container.Scroll
	toggle    *widget.Button
	box       *fyne.Container

	state    serviceCommandsState
	list     []lxdclient.Command
	selected string
	// last — последний полный ответ по имени (с логом).
	last map[string]lxdclient.Command
	// logOpen — лог раскрыт (▴); раскрывается сам при запуске.
	logOpen bool
	// override — исход, который важнее состояния из списка (итог update,
	// ошибка запуска); сбрасывается выбором другой команды и новым Run.
	override           string
	overrideImportance widget.Importance

	// watching — имя команды под наблюдением; gen — поколение опроса
	// (новое наблюдение и dispose гасят прежние таймеры).
	watching string
	gen      int
	timer    *time.Timer

	listAsked bool
	upd       *serviceUpdateRun
	disposed  bool
	// starting — POST запуска в пути: Run неактивна до ответа.
	starting bool
	// reachable/reachableSeen — связь с демоном в прошлом снапшоте: ожидание
	// update включает только переход «отвечал → перестал», а не устаревший
	// снапшот после успешного Reconnect (heartbeat догоняет до 5 с).
	reachable     bool
	reachableSeen bool
}

func newServiceCommandBar(v *serviceView) *serviceCommandBar {
	b := &serviceCommandBar{v: v, last: make(map[string]lxdclient.Command)}
	b.sel = widget.NewSelect(nil, nil)
	b.run = widget.NewButton(locale.T("Run"), b.onRun)
	b.run.Importance = widget.HighImportance
	b.cmdText = serviceNoteLabel("", widget.LowImportance)
	b.cmdText.TextStyle = fyne.TextStyle{Monospace: true}
	b.cmdText.Wrapping = fyne.TextWrapBreak
	b.outcome = ttwidget.NewLabel("")
	b.outcome.Wrapping = fyne.TextWrapWord
	b.logText = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	b.logText.Wrapping = fyne.TextWrapBreak
	b.logText.Selectable = true
	b.logScroll = container.NewVScroll(b.logText)
	b.logScroll.SetMinSize(fyne.NewSize(0, serviceCommandLogMinHeight))
	b.logScroll.Hide()
	b.toggle = widget.NewButton("▾", func() { // l10n-exempt: glyph
		b.logOpen = !b.logOpen
		b.refreshRow()
	})
	b.toggle.Importance = widget.LowImportance
	b.toggle.Hide()

	left := container.NewHBox(widget.NewLabel(locale.T("Daemon command:")), b.sel, b.run)
	row := container.NewBorder(nil, nil, left, b.toggle, b.outcome)
	b.box = container.NewVBox(row, b.cmdText, b.logScroll)
	return b
}

func (b *serviceCommandBar) object() fyne.CanvasObject { return b.box }

func (b *serviceCommandBar) cm() serviceCommander { return b.v.src.Commander() }

// dispose гасит опрос прогона и ожидание update (закрытие окна).
func (b *serviceCommandBar) dispose() {
	b.disposed = true
	b.gen++
	if b.timer != nil {
		b.timer.Stop()
	}
	if b.upd != nil {
		b.upd.stopTicker()
	}
}

// apply раскладывает строку по снапшоту (UI-поток; зовётся из renderHeader).
func (b *serviceCommandBar) apply(s serviceSnapshot) {
	if b.disposed {
		return
	}
	b.state, b.list = s.CommandsState, s.Commands
	if b.cm() == nil {
		b.state = serviceCommandsUnsupported
	}
	reachable := s.Loaded && s.Connected && s.Reachable
	dropped := b.reachableSeen && b.reachable && !reachable
	if s.Loaded && s.Connected {
		b.reachable, b.reachableSeen = reachable, true
	}
	if u := b.upd; u != nil {
		// Демон пропал во время update — это перезапуск, а не авария. Только
		// на переходе и не по снапшоту старше последнего успешного Reconnect.
		fresh := u.reconnectedAt.IsZero() || time.Since(u.reconnectedAt) > 2*heartbeatInterval
		if u.state == serviceUpdatePolling && dropped && fresh {
			b.enterReconnect()
		}
		b.refreshRow()
		return
	}
	// Список при открытии окна — сразу, не дожидаясь heartbeat.
	if !b.listAsked && b.cm() != nil && s.Loaded && s.Reachable {
		b.listAsked = true
		b.refreshList()
	}
	// Прогон, начатый до открытия окна (или другим клиентом), наблюдается
	// так же, как свой.
	if b.state == serviceCommandsReady && b.watching == "" {
		for _, c := range b.list {
			if c.State == lxdclient.CommandRunning {
				b.watch(c)
				return
			}
		}
	}
	b.refreshRow()
}

// refreshRow перерисовывает селектор, Run, исход и лог по состоянию.
func (b *serviceCommandBar) refreshRow() {
	b.sel.OnChanged = nil
	defer func() { b.sel.OnChanged = b.onSelect }()
	busy := b.watching != "" || b.upd != nil || b.starting

	names := make([]string, 0, len(b.list))
	for _, c := range b.list {
		names = append(names, c.Name)
	}
	if b.state != serviceCommandsReady || len(names) == 0 {
		na := locale.T("not available")
		b.sel.Options = []string{na}
		b.sel.SetSelected(na)
		b.sel.Disable()
		b.run.Disable()
		b.cmdText.Hide()
		text, importance := "", widget.LowImportance
		switch {
		case b.override != "":
			text, importance = b.override, b.overrideImportance
		case b.upd != nil:
			text, importance = locale.T("updating…"), widget.MediumImportance
		case b.state == serviceCommandsDown:
			text = locale.T("the daemon does not answer")
		case b.state == serviceCommandsUnsupported && b.cm() != nil, b.state == serviceCommandsReady:
			text = locale.T(serviceCommandsHintText)
		}
		b.setOutcome(text, importance, "")
		b.refreshLog()
		return
	}

	if !containsString(names, b.selected) {
		b.selected = names[0]
	}
	b.sel.Options = names
	b.sel.SetSelected(b.selected)
	if busy {
		b.sel.Disable()
	} else {
		b.sel.Enable()
	}
	cur, _ := b.current()
	if busy || cur.State == lxdclient.CommandRunning {
		b.run.Disable()
	} else {
		b.run.Enable()
	}
	b.cmdText.SetText(cur.Command)
	b.cmdText.Show()

	if b.override != "" {
		b.setOutcome(b.override, b.overrideImportance, cur.Client)
	} else if b.upd != nil && cur.State != lxdclient.CommandRunning {
		b.setOutcome(locale.T("updating…"), widget.MediumImportance, "")
	} else {
		text, importance := serviceCommandOutcome(cur)
		b.setOutcome(text, importance, cur.Client)
	}
	b.refreshLog()
}

// current — выбранная команда: свежий полный ответ, если он не старее
// записи списка, иначе запись списка.
func (b *serviceCommandBar) current() (lxdclient.Command, bool) {
	var fromList lxdclient.Command
	found := false
	for _, c := range b.list {
		if c.Name == b.selected {
			fromList, found = c, true
			break
		}
	}
	if full, ok := b.last[b.selected]; ok {
		if !found || (full.State == fromList.State && full.FinishedAt == fromList.FinishedAt) || b.watching == b.selected {
			return full, true
		}
	}
	return fromList, found
}

func (b *serviceCommandBar) setOutcome(text string, importance widget.Importance, client string) {
	b.outcome.Text = text
	b.outcome.Importance = importance
	b.outcome.Refresh()
	if client != "" {
		b.outcome.SetToolTip(locale.Tf("started by %s", client))
	} else {
		b.outcome.SetToolTip("")
	}
}

// refreshLog — лог выбранной команды (если он уже приходил) и ▴/▾.
func (b *serviceCommandBar) refreshLog() {
	full, ok := b.last[b.selected]
	if !ok || strings.TrimSpace(full.Log) == "" || (b.state != serviceCommandsReady && b.upd == nil && b.override == "") {
		b.toggle.Hide()
		b.logScroll.Hide()
		return
	}
	if b.logText.Text != full.Log {
		b.logText.SetText(full.Log)
		b.logScroll.ScrollToBottom()
	}
	b.toggle.Show()
	if b.logOpen {
		b.toggle.SetText("▴") // l10n-exempt: glyph
		b.logScroll.Show()
	} else {
		b.toggle.SetText("▾") // l10n-exempt: glyph
		b.logScroll.Hide()
	}
}

func (b *serviceCommandBar) onSelect(name string) {
	if name == b.selected {
		return
	}
	b.selected = name
	b.override = ""
	b.refreshRow()
}

func (b *serviceCommandBar) onRun() {
	name := b.selected
	if name == "" || b.cm() == nil {
		return
	}
	if name == serviceCommandUpdate {
		ShowConfirm(b.v.win, locale.T("Update the core"), locale.T(serviceUpdateConfirmText), func(ok bool) {
			if ok {
				b.startUpdate()
			}
		})
		return
	}
	b.override = ""
	b.starting = true
	b.run.Disable()
	b.setOutcome(locale.T("starting…"), widget.MediumImportance, "")
	b.cm().RunCommand(name, func(c lxdclient.Command, err error) {
		b.starting = false
		if b.disposed {
			return
		}
		b.onStarted(name, c, err)
	})
}

// onStarted — ответ на POST: 202 и 409 ведут к наблюдению, 404 —
// перечитать список, иное — ошибка в исход.
func (b *serviceCommandBar) onStarted(name string, c lxdclient.Command, err error) {
	var running *lxdclient.ErrCommandRunning
	switch {
	case err == nil:
		if c.Name == "" {
			c.Name = name
		}
		b.watch(c)
		return
	case errors.As(err, &running):
		cur := running.Current
		if cur.Name == "" {
			cur.Name = name
		}
		b.watch(cur)
		return
	case errors.Is(err, lxdclient.ErrCommandNotFound):
		b.setOverride(locale.T(serviceCommandGoneText), widget.DangerImportance)
		b.refreshList()
	default:
		debuglog.WarnLog("service window: run command %q: %v", name, err)
		b.setOverride("✖ "+err.Error(), widget.DangerImportance)
	}
	b.refreshRow()
}

func (b *serviceCommandBar) setOverride(text string, importance widget.Importance) {
	b.override, b.overrideImportance = text, importance
}

// refreshList перечитывает список (кэш источника) и перестраивает окно.
func (b *serviceCommandBar) refreshList() {
	cm := b.cm()
	if cm == nil {
		return
	}
	cm.Commands(func([]lxdclient.Command, error) {
		if !b.disposed {
			b.v.update(true)
		}
	})
}

// watch — наблюдение за прогоном c.Name: GET раз в секунду до done|lost.
// Запросы не накладываются: следующий ставится после ответа на текущий.
func (b *serviceCommandBar) watch(c lxdclient.Command) {
	b.gen++
	b.watching = c.Name
	b.selected = c.Name
	b.logOpen = true
	b.store(c)
	b.refreshRow()
	if c.State != lxdclient.CommandRunning && c.State != lxdclient.CommandIdle {
		b.finishWatch(c)
		return
	}
	b.schedule(c.Name, b.gen)
}

func (b *serviceCommandBar) schedule(name string, gen int) {
	b.timer = time.AfterFunc(serviceCommandPoll, func() {
		fyne.Do(func() {
			cm := b.cm()
			if b.disposed || gen != b.gen || cm == nil {
				return
			}
			cm.Command(name, func(c lxdclient.Command, err error) {
				if b.disposed || gen != b.gen {
					return
				}
				if err != nil {
					if u := b.upd; u != nil && u.state == serviceUpdatePolling {
						b.enterReconnect()
						return
					}
					b.watching = ""
					if errors.Is(err, lxdclient.ErrCommandNotFound) {
						b.setOverride(locale.T(serviceCommandGoneText), widget.DangerImportance)
						b.refreshList()
					} else {
						b.setOverride("✖ "+err.Error(), widget.DangerImportance)
					}
					b.refreshRow()
					return
				}
				b.store(c)
				if c.State == lxdclient.CommandRunning || c.State == lxdclient.CommandIdle {
					b.refreshRow()
					b.schedule(name, gen)
					return
				}
				b.finishWatch(c)
			})
		})
	})
}

// finishWatch — прогон завершён (done|lost).
func (b *serviceCommandBar) finishWatch(c lxdclient.Command) {
	b.watching = ""
	if u := b.upd; u != nil && c.Name == serviceCommandUpdate {
		// update закончился без замеченного обрыва: демон мог перезапуститься
		// между двумя GET, и паспорт в кэше ещё старый — сперва перечитать
		// его, потом итог.
		b.gen++
		b.cm().Reconnect(func(bool) {
			if b.disposed || b.upd != u {
				return
			}
			b.finishUpdate(c)
		})
		return
	}
	// Наблюдаемый прогон завершён: его исход важнее прежней ошибки.
	b.override = ""
	b.refreshRow()
	b.refreshList()
}

func (b *serviceCommandBar) store(c lxdclient.Command) {
	if c.Name != "" {
		b.last[c.Name] = c
	}
}

// serviceCommandOutcome — исход команды для строки: idle / running /
// done · exit N · <когда> / lost; у check-update — строка «installed …
// available …» из лога.
func serviceCommandOutcome(c lxdclient.Command) (string, widget.Importance) {
	switch c.State {
	case lxdclient.CommandRunning:
		return locale.T("running…"), widget.MediumImportance
	case lxdclient.CommandLost:
		return locale.T("lost"), widget.DangerImportance
	case lxdclient.CommandDone:
		if c.Name == serviceCommandCheckUpdate {
			if line := serviceUpdateVersionsLine(c.Log); line != "" {
				return line, widget.MediumImportance
			}
		}
		code := -1
		if c.ExitCode != nil {
			code = *c.ExitCode
		}
		importance := widget.SuccessImportance
		if code != 0 {
			importance = widget.WarningImportance
		}
		var at time.Time
		if t, err := time.Parse(time.RFC3339, c.FinishedAt); err == nil {
			at = t
		}
		if ago := serviceSinceLabel(at); ago != "" {
			return locale.Tf("done · exit %d · %s ago", code, ago), importance
		}
		return locale.Tf("done · exit %d", code), importance
	}
	return locale.T("idle"), widget.LowImportance
}

// serviceUpdateVersionsLine — строка лога «installed X, available Y».
func serviceUpdateVersionsLine(log string) string {
	lines := strings.Split(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if serviceUpdateVersionsRe.MatchString(lines[i]) {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

// serviceUpdateAvailable — версия, на которую update пытался встать (из
// лога); "" — не нашли.
func serviceUpdateAvailable(log string) string {
	m := serviceUpdateVersionsRe.FindStringSubmatch(log)
	if m == nil {
		return ""
	}
	return strings.TrimRight(m[2], ",;")
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ update

// serviceUpdateState — шаги ожидания команды update (PLAN §5.4).
type serviceUpdateState int

const (
	serviceUpdatePosted serviceUpdateState = iota
	serviceUpdatePolling
	serviceUpdateReconnecting
	serviceUpdateVerifying
)

// serviceUpdateRun — один прогон update: версия до, начало ожидания,
// секундный тикер строки «reconnecting… N s».
type serviceUpdateRun struct {
	state   serviceUpdateState
	old     string
	since   time.Time
	attempt int
	// reconnectedAt — последний успешный Reconnect: снапшот «не отвечает»
	// после него — устаревший кэш heartbeat, а не новый обрыв.
	reconnectedAt time.Time
	// gen — поколение попыток переподключения (dispose / смена состояния
	// гасят отложенные).
	gen int

	tickOnce sync.Once
	tickStop chan struct{}
}

func (u *serviceUpdateRun) stopTicker() {
	if u.tickStop != nil {
		u.tickOnce.Do(func() { close(u.tickStop) })
	}
}

func (u *serviceUpdateRun) elapsed() time.Duration { return time.Since(u.since) }

// reconnectLine — строка состояния демона на время ожидания update; "" —
// ожидания нет, строку пишет снапшот.
func (b *serviceCommandBar) reconnectLine() string {
	u := b.upd
	if u == nil {
		return ""
	}
	switch u.state {
	case serviceUpdateReconnecting:
		return locale.Tf(serviceReconnectingText, int(u.elapsed().Seconds()))
	case serviceUpdateVerifying:
		return locale.T("The daemon is back — checking the update…")
	}
	return ""
}

func (b *serviceCommandBar) startUpdate() {
	cm := b.cm()
	if cm == nil || b.upd != nil {
		return
	}
	old := cm.Version()
	if old == "" {
		old = b.v.snap.Running
	}
	b.override = ""
	b.upd = &serviceUpdateRun{state: serviceUpdatePosted, old: old}
	b.setOutcome(locale.T("starting…"), widget.MediumImportance, "")
	b.refreshRow()
	debuglog.InfoLog("service window: update requested on %q (core %s)", b.v.snap.Title, old)
	cm.RunCommand(serviceCommandUpdate, func(c lxdclient.Command, err error) {
		if b.disposed || b.upd == nil {
			return
		}
		var running *lxdclient.ErrCommandRunning
		switch {
		case err == nil:
		case errors.As(err, &running):
			c = running.Current
		default:
			b.upd = nil
			b.onStarted(serviceCommandUpdate, c, err)
			return
		}
		if c.Name == "" {
			c.Name = serviceCommandUpdate
		}
		b.upd.state = serviceUpdatePolling
		b.watch(c)
	})
}

// enterReconnect — демон пропал во время update: ждём его, обновляя
// строку состояния каждую секунду, и пробуем достучаться с паузами
// 2 → 5 → 10 → 10… с, всего до serviceUpdateWait.
func (b *serviceCommandBar) enterReconnect() {
	u := b.upd
	if u == nil || u.state == serviceUpdateReconnecting {
		return
	}
	b.gen++ // наблюдение за прогоном — до возвращения демона
	b.watching = ""
	first := u.tickStop == nil
	u.state = serviceUpdateReconnecting
	if first {
		u.since = time.Now()
		u.tickStop = make(chan struct{})
		stop := u.tickStop
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					fyne.Do(b.tickReconnect)
				}
			}
		}()
		debuglog.InfoLog("service window: %q went away during update — waiting for it to come back", b.v.snap.Title)
	}
	b.v.renderHeader()
	b.scheduleReconnect()
}

func (b *serviceCommandBar) tickReconnect() {
	u := b.upd
	if b.disposed || u == nil || u.state != serviceUpdateReconnecting {
		return
	}
	b.v.stateLabel.SetText(b.reconnectLine())
}

func (b *serviceCommandBar) scheduleReconnect() {
	u := b.upd
	delay := serviceReconnectDelays[len(serviceReconnectDelays)-1]
	if u.attempt < len(serviceReconnectDelays) {
		delay = serviceReconnectDelays[u.attempt]
	}
	u.attempt++
	if left := serviceUpdateWait - u.elapsed(); delay > left {
		delay = left
	}
	// Не чаще раза в секунду, даже на последней попытке.
	if delay < serviceCommandPoll {
		delay = serviceCommandPoll
	}
	u.gen++
	gen := u.gen
	time.AfterFunc(delay, func() {
		fyne.Do(func() {
			cm := b.cm()
			if b.disposed || b.upd != u || u.gen != gen || u.state != serviceUpdateReconnecting || cm == nil {
				return
			}
			cm.Reconnect(func(ok bool) {
				if b.disposed || b.upd != u || u.gen != gen {
					return
				}
				switch {
				case ok:
					u.reconnectedAt = time.Now()
					b.verifyUpdate()
				case u.elapsed() >= serviceUpdateWait:
					b.failUpdate()
				default:
					b.scheduleReconnect()
				}
			})
		})
	})
}

// verifyUpdate — демон вернулся (паспорт перечитан в Reconnect): итог —
// по GET /admin/commands/update.
func (b *serviceCommandBar) verifyUpdate() {
	u := b.upd
	u.state = serviceUpdateVerifying
	b.v.renderHeader()
	cm := b.cm()
	cm.Command(serviceCommandUpdate, func(c lxdclient.Command, err error) {
		if b.disposed || b.upd != u {
			return
		}
		switch {
		case err == nil && (c.State == lxdclient.CommandRunning || c.State == lxdclient.CommandIdle):
			// Демон вернулся, а прогон ещё идёт (перезапуск не его): ждём дальше.
			u.state = serviceUpdatePolling
			b.v.renderHeader()
			b.watch(c)
		case err == nil:
			b.store(c)
			b.finishUpdate(c)
		case errors.Is(err, lxdclient.ErrCommandNotFound):
			// Новое ядро без команд (или без update в daemon.json): список
			// скажет, какой из двух случаев.
			cm.Commands(func(_ []lxdclient.Command, lerr error) {
				if b.disposed || b.upd != u {
					return
				}
				switch {
				case errors.Is(lerr, lxdclient.ErrCommandsUnsupported):
					b.endUpdate(locale.Tf(serviceUpdateNoCommandText, cm.Version()), widget.MediumImportance)
				case lerr == nil:
					// Команды есть, update из daemon.json убрали: итог — по версии.
					b.finishUpdate(lxdclient.Command{Name: serviceCommandUpdate, State: lxdclient.CommandDone, ExitCode: new(int)})
				case u.elapsed() >= serviceUpdateWait:
					b.failUpdate()
				default:
					b.enterReconnect()
				}
			})
		default:
			if u.elapsed() >= serviceUpdateWait {
				b.failUpdate()
				return
			}
			b.enterReconnect()
		}
	})
}

// finishUpdate — итог update по коду выхода (SPEC 162 §4.3 п. 4).
func (b *serviceCommandBar) finishUpdate(c lxdclient.Command) {
	u := b.upd
	if u == nil {
		return
	}
	now := b.cm().Version()
	if c.State == lxdclient.CommandLost {
		// Новый демон может пометить прогон, оборванный его же перезапуском,
		// как lost: версия сменилась — значит, обновление встало.
		if now != "" && u.old != "" && now != u.old {
			b.endUpdate(locale.Tf("✔ Updated %s → %s", u.old, now), widget.SuccessImportance)
			return
		}
		b.endUpdate(locale.T("lost"), widget.DangerImportance)
		return
	}
	code := -1
	if c.ExitCode != nil {
		code = *c.ExitCode
	}
	switch code {
	case 0:
		if now != "" && u.old != "" && now != u.old {
			b.endUpdate(locale.Tf("✔ Updated %s → %s", u.old, now), widget.SuccessImportance)
		} else {
			b.endUpdate(locale.Tf("✔ Already up to date (%s)", now), widget.SuccessImportance)
		}
	case 1:
		b.endUpdate(locale.T("Nothing changed"), widget.WarningImportance)
	case 2:
		attempted := serviceUpdateAvailable(c.Log)
		if attempted == "" {
			attempted = locale.T("the new core")
		}
		b.endUpdate(locale.Tf(serviceUpdateRolledText, attempted, u.old), widget.DangerImportance)
	default:
		text, importance := serviceCommandOutcome(c)
		b.endUpdate(text, importance)
	}
}

// failUpdate — демон не вернулся за serviceUpdateWait.
func (b *serviceCommandBar) failUpdate() {
	debuglog.WarnLog("service window: %q did not come back after update", b.v.snap.Title)
	b.endUpdate(locale.T("✖ The daemon did not come back"), widget.DangerImportance)
	b.v.selectTab(serviceTabNotRunning)
}

func (b *serviceCommandBar) endUpdate(text string, importance widget.Importance) {
	if u := b.upd; u != nil {
		u.stopTicker()
		u.gen++
	}
	b.upd = nil
	b.watching = ""
	b.gen++
	b.selected = serviceCommandUpdate
	b.logOpen = true
	b.setOverride(text, importance)
	debuglog.InfoLog("service window: update on %q: %s", b.v.snap.Title, text)
	b.v.update(true)
	b.refreshList()
}
