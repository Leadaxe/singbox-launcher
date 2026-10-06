// File servers_node_info_wireguard.go — состояние WG/AWG-узлов в работающем
// ядре и выключатель (SPEC 097/106 ядра): секция «WireGuard» окна Info,
// статус в подзаголовке строки списка и пункт контекстного меню.
//
// Источник списка — тот же транспорт, через который панель грузит список
// (EffectiveProxyTransportIn по области), поэтому Local и Remote не путаются.
// Окно Info — ядро, зафиксированное при открытии (nodeWindowTarget): у Remote
// это машина, выбранная тогда, и после смены выбора выключатель в другую
// машину не шлёт.
// Ядро отдаёт endpointState только у WG/AWG — имени схемы здесь нет.
package ui

import (
	"errors"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// endpointRefreshSteps — период перечитывания состояний в секундах: и в окне
// Info, и в списке. Один GetOutbounds, без проб узлов.
const endpointRefreshSteps = 10

// endpointSourceIn — источник состояний для области; ok=false в classic и
// у машины без gRPC.
func endpointSourceIn(ac *core.AppController, scope services.ProxyScope) (services.EndpointSource, bool) {
	if ac == nil {
		return nil, false
	}
	src, ok := EffectiveProxyTransportIn(ac, scope).(services.EndpointSource)
	return src, ok
}

// endpointStateLabel — слово состояния для окна Info. Неизвестное — как есть:
// ядро новее лаунчера.
func endpointStateLabel(state string) string {
	switch state {
	case services.EndpointStateUp:
		return locale.T("up")
	case services.EndpointStateAsleep:
		return locale.T("asleep")
	case services.EndpointStateTornDown:
		return locale.T("released")
	case services.EndpointStateNeverBuilt:
		return locale.T("not started yet")
	case services.EndpointStateBuilding:
		return locale.T("starting")
	case services.EndpointStateDown:
		return locale.T("stopped")
	case services.EndpointStateDisabled:
		return locale.T("disabled")
	}
	return state
}

func endpointStateText(st services.EndpointStatus) string {
	text := endpointStateLabel(st.State)
	// Простой у выключенного и ещё не собранного узла ничего не говорит.
	if st.IdleSince > 0 && st.State != services.EndpointStateDisabled && st.State != services.EndpointStateNeverBuilt {
		text += " · " + locale.Tf("idle %s", humanAge(st.IdleSince))
	}
	return text
}

// endpointStateShort — слово статуса для подзаголовка строки списка. Пусто —
// не показывать: несобранный и остановленный узел строку не шумят.
func endpointStateShort(st services.EndpointStatus) string {
	switch st.State {
	case services.EndpointStateUp:
		return locale.T("up")
	case services.EndpointStateAsleep:
		return locale.T("sleep") + " " + humanAge(st.IdleSince)
	case services.EndpointStateTornDown:
		return locale.T("freed") + " " + humanAge(st.IdleSince)
	case services.EndpointStateBuilding:
		return locale.T("starting")
	case services.EndpointStateDisabled:
		return locale.T("off")
	}
	return ""
}

// endpointToggleLabel — пункт контекстного меню строки.
func endpointToggleLabel(st services.EndpointStatus) string {
	if st.State == services.EndpointStateDisabled {
		return locale.T("Enable WireGuard")
	}
	return locale.T("Disable WireGuard")
}

// --- список: кеш состояний панели ------------------------------------------

// startEndpointPoll привязывает к панели опрос состояний WG/AWG-узлов.
// Гейты те же, что у автообновления Remote: вкладка активна, окно видно.
func (p *ProxyListPanel) startEndpointPoll(ac *core.AppController) {
	p.endpointPoll = &proxyAutoRefresh{steps: endpointRefreshSteps}
	p.endpointPoll.tick = func() { p.pollEndpointStates(ac) }
}

// EndpointPoll — тикер опроса состояний (для гейтов из app.go).
func (p *ProxyListPanel) EndpointPoll() *proxyAutoRefresh {
	if p == nil {
		return nil
	}
	return p.endpointPoll
}

// pollEndpointStates перечитывает состояния и перерисовывает список при
// изменении. Сеть — в вызывающей горутине, не в UI-потоке.
func (p *ProxyListPanel) pollEndpointStates(ac *core.AppController) {
	if p == nil || platform.IsSleeping() {
		return
	}
	src, ok := endpointSourceIn(ac, p.scope)
	var states map[string]services.EndpointStatus
	if ok {
		var err error
		states, err = src.EndpointStatuses()
		if err != nil {
			debuglog.DebugLog("servers: endpoint states (%v): %v", p.scope, err)
			return
		}
	}
	fyne.Do(func() {
		if endpointStatesEqual(p.endpointStates, states) {
			return
		}
		p.endpointStates = states
		if p.proxiesList != nil {
			p.proxiesList.Refresh()
		}
	})
}

// RefreshEndpointStates — внеочередной опрос (вход на вкладку).
func (p *ProxyListPanel) RefreshEndpointStates(ac *core.AppController) {
	if p == nil {
		return
	}
	go p.pollEndpointStates(ac)
}

func endpointStatesEqual(a, b map[string]services.EndpointStatus) bool {
	if len(a) != len(b) {
		return false
	}
	// Пиры списку не нужны: подзаголовок строки показывает только состояние.
	for k, v := range a {
		w, ok := b[k]
		if !ok || w.State != v.State || w.IdleSince != v.IdleSince {
			return false
		}
	}
	return true
}

// endpointStateFor — состояние узла из кеша панели (UI-поток).
func (p *ProxyListPanel) endpointStateFor(tag string) (services.EndpointStatus, bool) {
	if p == nil || p.endpointStates == nil {
		return services.EndpointStatus{}, false
	}
	st, ok := p.endpointStates[tag]
	return st, ok
}

// toggleEndpointFromMenu — пункт «Disable/Enable WireGuard» контекстного меню.
func (p *ProxyListPanel) toggleEndpointFromMenu(ac *core.AppController, status *widget.Label, tag string, enable bool) {
	src, ok := endpointSourceIn(ac, p.scope)
	if !ok {
		return
	}
	go func() {
		_, err := src.SetEndpointEnabled(tag, enable)
		if err != nil {
			debuglog.WarnLog("servers: endpoint %s enabled=%v: %v", tag, enable, err)
			fyne.Do(func() {
				if status != nil {
					status.SetText(locale.Tf("WireGuard switch error: %s", err.Error()))
				}
			})
		}
		// Состояние берём у ядра и при отказе: при Unavailable узел уже
		// включён, но не проснулся.
		p.pollEndpointStates(ac)
	}()
}

// --- окно Info -------------------------------------------------------------

// endpointSourceFor — источник состояний ядра цели окна узла; ok=false в
// classic, у машины без gRPC и у машины, которая больше не выбрана.
func endpointSourceFor(ac *core.AppController, target core.CoreTarget) (services.EndpointSource, bool) {
	if ac == nil {
		return nil, false
	}
	tr, ok := proxyTransportFor(ac, target)
	if !ok {
		return nil, false
	}
	src, ok := tr.(services.EndpointSource)
	return src, ok
}

// errEndpointSourceGone — ядро окна недоступно: машину сменили или отключили.
var errEndpointSourceGone = errors.New("the core of this window is not available")

// addWireGuardSection добавляет секцию, если источник ядра окна отдаёт
// состояние для этого тега.
func addWireGuardSection(ac *core.AppController, target core.CoreTarget, body *fyne.Container, win fyne.Window, tag string) {
	src, ok := endpointSourceFor(ac, target)
	if !ok || body == nil {
		return
	}
	box := container.NewVBox()
	body.Add(box)

	go func() {
		states, err := src.EndpointStatuses()
		if err != nil {
			debuglog.WarnLog("node info: endpoint states: %v", err)
			return
		}
		st, ok := states[tag]
		if !ok {
			return
		}
		source := func() (services.EndpointSource, bool) { return endpointSourceFor(ac, target) }
		fyne.Do(func() {
			buildWireGuardSection(source, box, win, tag, st)
		})
	}()
}

// source отдаёт источник на каждый запрос, а не один раз при открытии:
// сменилась машина — её транспорта больше нет, и команда отвечает ошибкой, а
// не уходит в другое ядро.
func buildWireGuardSection(source func() (services.EndpointSource, bool), box *fyne.Container, win fyne.Window, tag string, initial services.EndpointStatus) {
	var btn *widget.Button
	current := initial

	errLabel := widget.NewLabel("")
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Importance = widget.DangerImportance
	errLabel.Hide()

	note := widget.NewLabel(locale.T("Until the core restarts or the config is applied."))
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance

	btn = widget.NewButton("", nil)
	stateRow, stateEntry := infoRowEntry(locale.T("State"), "", btn)

	peers := newWireGuardPeerRows()

	apply := func(st services.EndpointStatus) {
		current = st
		peers.update(st.Peers, time.Now())
		stateEntry.SetText(endpointStateText(st))
		if st.State == services.EndpointStateDisabled {
			btn.SetText(locale.T("Enable"))
		} else {
			btn.SetText(locale.T("Disable"))
		}
	}

	reread := func() (services.EndpointStatus, bool) {
		src, ok := source()
		if !ok {
			return services.EndpointStatus{}, false
		}
		states, err := src.EndpointStatuses()
		if err != nil {
			return services.EndpointStatus{}, false
		}
		st, ok := states[tag]
		return st, ok
	}

	btn.OnTapped = func() {
		enable := current.State == services.EndpointStateDisabled
		btn.Disable()
		errLabel.Hide()
		go func() {
			var err error
			if src, ok := source(); ok {
				_, err = src.SetEndpointEnabled(tag, enable)
			} else {
				err = errEndpointSourceGone
			}
			// Отказ не значит «не переключилось»: при Unavailable узел уже
			// включён, но не проснулся. Состояние берём у ядра заново.
			fresh, freshOK := reread()
			fyne.Do(func() {
				defer btn.Enable()
				if err != nil {
					debuglog.WarnLog("node info: endpoint %s enabled=%v: %v", tag, enable, err)
					errLabel.SetText(err.Error())
					errLabel.Show()
				}
				if freshOK {
					apply(fresh)
				}
			})
		}()
	}

	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.T("WireGuard")))
	box.Add(stateRow)
	box.Add(note)
	box.Add(errLabel)
	box.Add(peers.box)
	apply(initial)
	box.Refresh()

	// Единственный владелец OnClosed в окне WG-узла: подписка на выбор
	// ставится только у групп.
	stop := make(chan struct{})
	win.SetOnClosed(func() { close(stop) })
	go func() {
		ticker := time.NewTicker(endpointRefreshSteps * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			st, ok := reread()
			if !ok {
				continue
			}
			fyne.Do(func() {
				select {
				case <-stop:
					return
				default:
				}
				apply(st)
			})
		}
	}()
}

// --- окно Info: пиры --------------------------------------------------------

// wgPeerSessionWindow — сколько живёт ключ сессии WG (reject_after_time):
// хендшейк моложе — пир на связи.
const wgPeerSessionWindow = 180 * time.Second

// wireGuardPeerRows — строки пиров секции «WireGuard». Строка = короткий ключ
// и Entry со значением (адрес можно выделить и скопировать). Ряды
// пересобираются только при смене набора ключей, иначе текст меняется на
// месте и выделение не слетает.
type wireGuardPeerRows struct {
	box     *fyne.Container
	keys    []string
	entries map[string]*widget.Entry
	// prevRx — rx прошлого опроса: рост rx при старом хендшейке = на связи.
	prevRx map[string]int64
}

func newWireGuardPeerRows() *wireGuardPeerRows {
	return &wireGuardPeerRows{box: container.NewVBox(), entries: map[string]*widget.Entry{}, prevRx: map[string]int64{}}
}

func (r *wireGuardPeerRows) update(peers []services.EndpointPeer, now time.Time) {
	keys := make([]string, 0, len(peers))
	for _, p := range peers {
		keys = append(keys, p.PublicKey)
	}
	if strings.Join(keys, "\n") != strings.Join(r.keys, "\n") {
		r.keys = keys
		r.entries = map[string]*widget.Entry{}
		r.box.Objects = nil
		if len(peers) > 0 {
			r.box.Add(sectionHeader(locale.T("Peers")))
		}
		for _, p := range peers {
			row, entry := infoRowEntry(shortPeerKey(p.PublicKey), "", nil)
			r.entries[p.PublicKey] = entry
			r.box.Add(row)
		}
		r.box.Refresh()
	}
	rx := make(map[string]int64, len(peers))
	for _, p := range peers {
		prev, seen := r.prevRx[p.PublicKey]
		// Счётчики обнуляются при пересборке устройства: меньшее значение —
		// новая база, а не рост.
		grew := seen && p.RxBytes > prev
		rx[p.PublicKey] = p.RxBytes
		if e := r.entries[p.PublicKey]; e != nil {
			e.SetText(wireGuardPeerText(p, now, grew))
		}
	}
	r.prevRx = rx
}

// wireGuardPeerText — «127.0.0.1:56793 · online 40s · ↓244 B ↑238 B». Адрес
// без хендшейка не показываем: ядро хранит его и после ухода пира.
func wireGuardPeerText(p services.EndpointPeer, now time.Time, rxGrew bool) string {
	parts := make([]string, 0, 3)
	if p.LastHandshake.IsZero() {
		parts = append(parts, locale.T("no handshake yet"))
	} else {
		if p.Endpoint != "" {
			parts = append(parts, p.Endpoint)
		}
		age := now.Sub(p.LastHandshake)
		if age < 0 {
			age = 0
		}
		if age <= wgPeerSessionWindow || rxGrew {
			parts = append(parts, locale.Tf("online, handshake %s ago", humanAge(age)))
		} else {
			parts = append(parts, locale.Tf("no session, last handshake %s ago", humanAge(age)))
		}
	}
	parts = append(parts, "↓"+paths.FormatBytes(p.RxBytes)+" ↑"+paths.FormatBytes(p.TxBytes))
	return strings.Join(parts, " · ")
}

// shortPeerKey — «RIpg…A1Eo»: ключ с вырезанной серединой.
func shortPeerKey(key string) string {
	if len(key) <= 10 {
		return key
	}
	return key[:4] + "…" + key[len(key)-4:]
}
