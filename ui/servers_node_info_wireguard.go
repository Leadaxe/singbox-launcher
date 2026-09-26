// File servers_node_info_wireguard.go — состояние WG/AWG-узлов в работающем
// ядре и выключатель (SPEC 097/106 ядра): секция «WireGuard» окна Info,
// статус в подзаголовке строки списка и пункт контекстного меню.
//
// Источник — тот же транспорт, через который панель грузит список
// (EffectiveProxyTransportIn по области), поэтому Local и Remote не путаются.
// Ядро отдаёт endpointState только у WG/AWG — имени схемы здесь нет.
package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
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
	for k, v := range a {
		if b[k] != v {
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

// addWireGuardSection добавляет секцию, если источник области отдаёт
// состояние для этого тега.
func addWireGuardSection(ac *core.AppController, body *fyne.Container, win fyne.Window, tag string, scope services.ProxyScope) {
	src, ok := endpointSourceIn(ac, scope)
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
		fyne.Do(func() {
			buildWireGuardSection(src, box, win, tag, st)
		})
	}()
}

func buildWireGuardSection(src services.EndpointSource, box *fyne.Container, win fyne.Window, tag string, initial services.EndpointStatus) {
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

	apply := func(st services.EndpointStatus) {
		current = st
		stateEntry.SetText(endpointStateText(st))
		if st.State == services.EndpointStateDisabled {
			btn.SetText(locale.T("Enable"))
		} else {
			btn.SetText(locale.T("Disable"))
		}
	}

	reread := func() (services.EndpointStatus, bool) {
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
			_, err := src.SetEndpointEnabled(tag, enable)
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
