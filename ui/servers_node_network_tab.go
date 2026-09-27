// File servers_node_network_tab.go — вкладка Network окна узла Tailscale
// (SPEC 148 §3, LxBox §581): Status, This device, Exit node, Devices,
// проверка устройства.
//
// Данные — кеш стрима SubscribeTailscaleStatus (SPEC 130); команды —
// SetTailscaleExitNode, TailscaleLogout, StartTailscalePing того же
// источника. Вкладка перерисовывается не чаще раза в секунду и только когда
// пришёл новый снимок или сменилось записанное значение.
//
// Имена устройств, адреса, имя сети, владельцы и ссылка входа в журнал не
// пишутся: ошибки команд логируются только с тегом узла.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// tailscaleNetworkView — что вкладка показывает без данных (§2).
type tailscaleNetworkView int

const (
	tsViewVPNOff tailscaleNetworkView = iota
	tsViewWaiting
	tsViewNotRunning
	tsViewStatus
)

// tailscaleNetworkViewFor — выбор строки таблицы §2 LxBox §581.
func tailscaleNetworkViewFor(running, live, ok bool) tailscaleNetworkView {
	switch {
	case !running:
		return tsViewVPNOff
	case ok:
		return tsViewStatus
	case live:
		// Стрим жив и отдал снимок, а узла в нём нет: ядро узел не подняло.
		return tsViewNotRunning
	}
	return tsViewWaiting
}

// exitNodeWarningText — текст у знака предупреждения блока Exit node (§5).
func exitNodeWarningText(d services.ExitNodeDiff) string {
	switch d {
	case services.ExitNodeSelectedNotSaved:
		return locale.T("Not saved. Traffic is not routed through this node until you save the choice.")
	case services.ExitNodeClearedNotSaved:
		return locale.T("Not saved. The node stays in the lists, but has no exit until you save the choice.")
	case services.ExitNodeOtherNotSaved:
		return locale.T("Not saved. The choice is lost after restart.")
	}
	return ""
}

// writtenExitNode — `exit_node` тела узла в собранном конфиге.
func writtenExitNode(cfgPath, tag string) string {
	node := wizardbusiness.LoadConfigNodes(cfgPath).Lookup(tag)
	if node == nil {
		return ""
	}
	v, _ := node.Raw["exit_node"].(string)
	return strings.TrimSpace(v)
}

// tailscaleNetworkTab строит вкладку и запускает её обновление; обновление
// останавливается, когда окно закрыто.
func tailscaleNetworkTab(ac *core.AppController, win fyne.Window, tag, cfgPath string, local bool) fyne.CanvasObject {
	box := container.NewVBox()
	canSave := local && ac.CanSaveTailscaleExitNode(tag)

	type snapKey struct {
		view     tailscaleNetworkView
		at       time.Time
		written  string
		haveCtrl bool
	}
	var last *snapKey
	refresh := func() {
		running := ac.RunningState != nil && ac.RunningState.IsRunning()
		st, ok := ac.TailscaleStatus(tag)
		live := ac.TailscaleLive()
		written := writtenExitNode(cfgPath, tag)
		key := snapKey{view: tailscaleNetworkViewFor(running, live, ok), at: st.ReceivedAt, written: written, haveCtrl: ac.TailscaleControlAvailable()}
		if last != nil && *last == key {
			return
		}
		last = &key
		fyne.Do(func() {
			box.RemoveAll()
			buildTailscaleNetwork(ac, win, box, tag, key.view, st, written, canSave, key.haveCtrl)
			box.Refresh()
		})
	}
	go func() {
		refresh()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			if !windowOpen(win) {
				return
			}
			refresh()
		}
	}()
	return withScrollGutter(box)
}

// windowOpen — окно ещё среди окон приложения.
func windowOpen(win fyne.Window) bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	for _, w := range app.Driver().AllWindows() {
		if w == win {
			return true
		}
	}
	return false
}

func buildTailscaleNetwork(ac *core.AppController, win fyne.Window, box *fyne.Container, tag string,
	view tailscaleNetworkView, st services.TailscaleStatus, written string, canSave, haveCtrl bool) {
	note := func(text string) {
		l := widget.NewLabel(text)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = widget.LowImportance
		box.Add(l)
	}
	switch view {
	case tsViewVPNOff:
		note(locale.T("Start VPN to see the network."))
		return
	case tsViewWaiting:
		box.Add(widget.NewProgressBarInfinite())
		return
	case tsViewNotRunning:
		note(locale.T("The node is not in the running config."))
		return
	}

	// --- Status ---
	box.Add(sectionHeader(locale.T("Status")))
	word, _ := networksRowState(true, st, true)
	box.Add(infoRow(locale.T("State"), word))
	if st.NetworkName != "" {
		box.Add(infoRow(locale.T("Network"), st.NetworkName))
	}
	if st.KeyAuth {
		note(locale.T("signed in with a key"))
	}
	buttons := container.NewHBox()
	if st.BackendState == services.TailscaleStateNeedsLogin && strings.TrimSpace(st.AuthURL) != "" {
		url := st.AuthURL
		b := widget.NewButton(locale.T("Sign in"), func() {
			if err := platform.OpenURL(url); err != nil {
				debuglog.WarnLog("tailscale %s: sign-in page not opened", tag)
			}
		})
		b.Importance = widget.HighImportance
		buttons.Add(b)
	}
	if st.BackendState == services.TailscaleStateRunning && haveCtrl {
		buttons.Add(widget.NewButton(locale.T("Log out"), func() {
			dialog.ShowConfirm(locale.T("Log out"),
				locale.T("The node leaves the network. If the node has an auth key, it signs in again on the next start."),
				func(yes bool) {
					if !yes {
						return
					}
					go func() {
						if err := ac.TailscaleLogout(tag); err != nil {
							debuglog.WarnLog("tailscale %s: logout failed", tag)
							fyne.Do(func() { ShowError(win, err) })
						}
					}()
				}, win)
		}))
	}
	if len(buttons.Objects) > 0 {
		box.Add(buttons)
	}

	// --- This device ---
	if st.Self != nil {
		self := *st.Self
		box.Add(widget.NewSeparator())
		box.Add(sectionHeader(locale.T("This device")))
		box.Add(copyRow(locale.T("Name"), self.HostName))
		if dns := strings.TrimSuffix(self.DNSName, "."); dns != "" {
			box.Add(copyRow(locale.T("MagicDNS name"), dns))
		}
		for _, ip := range self.TailscaleIPs {
			box.Add(copyRow(locale.T("Address"), ip))
		}
		expiry := locale.T("never")
		if !self.KeyExpiry.IsZero() {
			expiry = self.KeyExpiry.Local().Format("2006-01-02 15:04")
		}
		box.Add(infoRow(locale.T("Key expiry"), expiry))
	}

	// --- Exit node ---
	box.Add(widget.NewSeparator())
	diff := services.CompareExitNode(written, st.ExitNode)
	header := container.NewHBox(sectionHeader(locale.T("Exit node")))
	if diff != services.ExitNodeSame {
		header.Add(widget.NewIcon(theme.WarningIcon()))
	}
	box.Add(header)
	options := services.ExitNodeOptions(st)
	none := locale.T("None")
	labels := []string{none}
	byLabel := map[string]services.TailscalePeer{}
	selected := none
	for _, p := range options {
		l := tailscalePeerLine(p)
		for byLabel[l].StableID != "" {
			l += " "
		}
		labels = append(labels, l)
		byLabel[l] = p
		if st.ExitNode != nil && (p.StableID == st.ExitNode.StableID && p.StableID != "") {
			selected = l
		}
	}
	if st.ExitNode != nil && selected == none {
		// Действующий выход не в списке вариантов (устройство перестало
		// анонсировать выход) — показываем его, чтобы выбор не лгал.
		l := tailscalePeerLine(*st.ExitNode)
		labels = append(labels, l)
		byLabel[l] = *st.ExitNode
		selected = l
	}
	sel := widget.NewSelect(labels, nil)
	sel.SetSelected(selected)
	sel.OnChanged = func(v string) {
		if v == selected {
			return
		}
		stableID := ""
		if v != none {
			stableID = byLabel[v].StableID
		}
		go func() {
			if err := ac.TailscaleSetExitNode(tag, stableID); err != nil {
				debuglog.WarnLog("tailscale %s: exit node not switched", tag)
				fyne.Do(func() { ShowError(win, err) })
			}
		}()
	}
	if !haveCtrl {
		sel.Disable()
	}
	box.Add(sel)
	if diff != services.ExitNodeSame {
		w := widget.NewLabel(exitNodeWarningText(diff))
		w.Wrapping = fyne.TextWrapWord
		w.Importance = widget.WarningImportance
		box.Add(w)
		if canSave {
			value := ""
			if st.ExitNode != nil {
				value = services.ExitNodeValueFor(*st.ExitNode)
			}
			var save *widget.Button
			save = widget.NewButton(locale.T("Save choice"), func() {
				save.Disable()
				go func() {
					err := ac.SaveTailscaleExitNode(tag, value)
					fyne.Do(func() {
						if err != nil {
							save.Enable()
							ShowError(win, err)
						}
					})
				}()
			})
			save.Importance = widget.HighImportance
			box.Add(container.NewHBox(save))
		}
	}

	// --- Devices ---
	online, total := st.PeersOnline()
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.Tf("Devices (%d online / %d)", online, total)))
	if total == 0 {
		note(locale.T("No devices in this network."))
		return
	}
	groups := 0
	for _, g := range st.UserGroups {
		if len(g.Peers) > 0 {
			groups++
		}
	}
	for _, g := range st.UserGroups {
		if len(g.Peers) == 0 {
			continue
		}
		if groups > 1 {
			owner := g.DisplayName
			if owner == "" {
				owner = g.LoginName
			}
			h := widget.NewLabel(owner)
			h.TextStyle.Bold = true
			box.Add(h)
		}
		for _, p := range services.SortDevices(g.Peers) {
			box.Add(tailscaleDeviceRow(ac, win, tag, p, haveCtrl))
		}
	}
}

// copyRow — строка «ключ: значение», нажатие на значение копирует его.
func copyRow(key, value string) fyne.CanvasObject {
	b := widget.NewButton(value, func() { setClipboard(value) })
	b.Importance = widget.LowImportance
	b.Alignment = widget.ButtonAlignLeading
	return container.NewBorder(nil, nil, widget.NewLabel(key+":"), nil, b)
}

// tailscaleDeviceDetails — вторая строка устройства (§6).
func tailscaleDeviceDetails(p services.TailscalePeer, now time.Time) string {
	parts := make([]string, 0, 7)
	if dns := strings.TrimSuffix(p.DNSName, "."); dns != "" {
		parts = append(parts, dns)
	}
	if len(p.TailscaleIPs) > 0 {
		parts = append(parts, p.TailscaleIPs[0])
	}
	if p.OS != "" {
		parts = append(parts, p.OS)
	}
	if p.Online {
		parts = append(parts, locale.T("online"))
	} else if !p.LastSeen.IsZero() {
		parts = append(parts, locale.Tf("last seen %s ago", humanAge(now.Sub(p.LastSeen))))
	}
	if p.Expired {
		parts = append(parts, locale.T("key expired"))
	}
	if p.ShareeNode {
		parts = append(parts, locale.T("shared"))
	}
	if p.ExitNodeOption {
		parts = append(parts, locale.T("exit node"))
	}
	return strings.Join(parts, " · ")
}

// tailscaleDeviceRow — строка устройства и меню: Copy name, Copy address, Ping.
func tailscaleDeviceRow(ac *core.AppController, win fyne.Window, tag string, p services.TailscalePeer, haveCtrl bool) fyne.CanvasObject {
	mark := "○"
	if p.Online {
		mark = "●"
	}
	name := widget.NewLabel(mark + " " + p.HostName)
	name.TextStyle.Bold = p.Online
	sub := widget.NewLabel(tailscaleDeviceDetails(p, time.Now()))
	sub.Importance = widget.LowImportance
	sub.Wrapping = fyne.TextWrapWord

	addr := ""
	if len(p.TailscaleIPs) > 0 {
		addr = p.TailscaleIPs[0]
	}
	var more *widget.Button
	more = widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItem(locale.T("Copy name"), func() { setClipboard(p.HostName) }),
		}
		if addr != "" {
			items = append(items, fyne.NewMenuItem(locale.T("Copy address"), func() { setClipboard(addr) }))
			if haveCtrl {
				items = append(items, fyne.NewMenuItem(locale.T("Ping"), func() {
					showTailscalePing(ac, win, tag, p.HostName, addr)
				}))
			}
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(more)
		widget.NewPopUpMenu(fyne.NewMenu("", items...), win.Canvas()).
			ShowAtPosition(pos.Add(fyne.NewPos(0, more.Size().Height)))
	})
	more.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, nil, more, container.NewVBox(name, sub))
}

// tailscalePingLine — одна строка результата проверки (§7).
func tailscalePingLine(r services.TailscalePingResult) string {
	if r.Error != "" {
		return r.Error
	}
	parts := []string{fmt.Sprintf("%.0f ms", r.LatencyMs)}
	if r.IsDirect {
		parts = append(parts, locale.T("direct"))
		if r.Endpoint != "" {
			parts = append(parts, r.Endpoint)
		}
	} else {
		parts = append(parts, locale.T("relay"))
		if r.DERPRegionCode != "" {
			parts = append(parts, r.DERPRegionCode)
		}
	}
	return strings.Join(parts, " · ")
}

// showTailscalePing — лист проверки устройства: до закрытия или до пяти
// ответов.
func showTailscalePing(ac *core.AppController, win fyne.Window, tag, name, addr string) {
	lines := container.NewVBox(widget.NewProgressBarInfinite())
	ctx, cancel := context.WithCancel(context.Background())
	d := dialog.NewCustom(locale.Tf("Ping %s", name), locale.T("Close"), lines, win)
	d.SetOnClosed(cancel)
	d.Resize(fyne.NewSize(420, 260))
	d.Show()
	go func() {
		first := true
		err := ac.TailscalePing(ctx, tag, addr, func(r services.TailscalePingResult) {
			line := tailscalePingLine(r)
			fyne.Do(func() {
				if first {
					lines.RemoveAll()
					first = false
				}
				lines.Add(widget.NewLabel(line))
				lines.Refresh()
			})
		})
		fyne.Do(func() {
			if first {
				lines.RemoveAll()
			}
			if err != nil && ctx.Err() == nil {
				lines.Add(widget.NewLabel(err.Error()))
			}
			lines.Add(widget.NewLabel(locale.T("Done.")))
			lines.Refresh()
		})
	}()
}
