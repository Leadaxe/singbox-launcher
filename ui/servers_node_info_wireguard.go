// File servers_node_info_wireguard.go — секция «WireGuard» окна Info узла:
// состояние WG/AWG-endpoint'а в работающем ядре и выключатель (SPEC 097/106
// ядра). Образец — addTailscaleSection / addPoolSection: гейт по gRPC, запрос
// в горутине, отрисовка через fyne.Do.
//
// Секция появляется только когда ядро отдаёт endpointState для этого тега —
// ядро заполняет его лишь у WG/AWG, поэтому имени схемы здесь нет.
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
)

// endpointRefreshInterval — период перечитывания состояния, пока окно открыто.
// Один GetOutbounds, без проб узлов.
const endpointRefreshInterval = 10 * time.Second

// endpointStateLabel — слово состояния для UI. Неизвестное — как есть: ядро
// новее лаунчера.
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

// addWireGuardSection добавляет секцию, если источник отдаёт состояние
// endpoint'ов и ядро знает этот тег как WG/AWG.
func addWireGuardSection(ac *core.AppController, body *fyne.Container, win fyne.Window, tag string) {
	if ac == nil || body == nil || !ac.EndpointControlAvailable() {
		return
	}
	box := container.NewVBox()
	body.Add(box)

	go func() {
		st, ok, err := ac.EndpointStatus(tag)
		if err != nil {
			debuglog.WarnLog("node info: endpoint status %s: %v", tag, err)
		}
		if err != nil || !ok {
			return
		}
		fyne.Do(func() {
			buildWireGuardSection(ac, box, win, tag, st)
		})
	}()
}

func buildWireGuardSection(ac *core.AppController, box *fyne.Container, win fyne.Window, tag string, initial services.EndpointStatus) {
	stateRow, stateEntry := infoRowEntry(locale.T("State"), "")

	errLabel := widget.NewLabel("")
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Importance = widget.DangerImportance
	errLabel.Hide()

	note := widget.NewLabel(locale.T("Until the core restarts or the config is applied."))
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance

	var btn *widget.Button
	current := initial
	apply := func(st services.EndpointStatus) {
		current = st
		stateEntry.SetText(endpointStateText(st))
		if st.State == services.EndpointStateDisabled {
			btn.SetText(locale.T("Enable"))
		} else {
			btn.SetText(locale.T("Disable"))
		}
	}

	btn = widget.NewButton("", func() {
		enable := current.State == services.EndpointStateDisabled
		btn.Disable()
		errLabel.Hide()
		go func() {
			state, err := ac.SetEndpointEnabled(tag, enable)
			// Отказ не значит «не переключилось»: при Unavailable узел уже
			// включён, но не проснулся. Состояние берём у ядра заново.
			var fresh services.EndpointStatus
			freshOK := false
			if err != nil {
				fresh, freshOK, _ = ac.EndpointStatus(tag)
			}
			fyne.Do(func() {
				defer btn.Enable()
				if err != nil {
					debuglog.WarnLog("node info: endpoint %s enabled=%v: %v", tag, enable, err)
					errLabel.SetText(err.Error())
					errLabel.Show()
					if freshOK {
						apply(fresh)
					}
					return
				}
				// Простой после переключения не известен — перечитается тиком.
				apply(services.EndpointStatus{State: state})
			})
		}()
	})

	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.T("WireGuard")))
	box.Add(stateRow)
	box.Add(container.NewHBox(btn))
	box.Add(note)
	box.Add(errLabel)
	apply(initial)
	box.Refresh()

	// Единственный владелец OnClosed в окне WG-узла: подписка на выбор
	// ставится только у групп.
	stop := make(chan struct{})
	win.SetOnClosed(func() { close(stop) })
	go func() {
		ticker := time.NewTicker(endpointRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			st, ok, err := ac.EndpointStatus(tag)
			if err != nil || !ok {
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
