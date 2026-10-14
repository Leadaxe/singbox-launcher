// File servers_node_network_tab.go — вкладка Network окна узла Tailscale
// (SPEC 148 §3, LxBox §581): Status, This device, Exit node, Devices,
// проверка устройства.
//
// Данные — запросный GetTailscaleStatus (SPEC 115 ядра) раз в
// tailnetPollInterval, пока окно открыто: путь пира в кеше стрима
// SubscribeTailscaleStatus (SPEC 130) живёт только до следующего события, а
// пользователь открыл вкладку ради текущей картины. Ядро без запроса или
// ответа нет — кеш стрима, как раньше. Команды — SetTailscaleExitNode,
// TailscaleLogout, StartTailscalePing того же источника: ядра области окна —
// своего (Local) или выбранной машины (Remote). Вкладка перерисовывается не
// чаще раза в секунду и только когда сменилось показываемое.
//
// Имена устройств, адреса, имя сети, владельцы и ссылка входа в журнал не
// пишутся: ошибки команд логируются только с тегом узла.
package ui

import (
	"context"
	"errors"
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
	"singbox-launcher/ui/configurator"
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
//
// Статус и команды — ядра области scope. У Remote окно привязано к машине,
// выбранной при открытии (core.TailscaleTarget с её id): источник — её
// транспорт из выбора машины, не из APIService, поэтому переход главного
// окна на Local вкладку не гасит. Переключись пользователь на другую машину
// — транспорта этой больше нет: вкладка гаснет до вида без данных, команды
// отвечают «not available» и в другую машину не уходят.
func tailscaleNetworkTab(ac *core.AppController, win fyne.Window, tag, cfgPath string, scope services.ProxyScope) fyne.CanvasObject {
	box := container.NewVBox()
	// Ядро окна — то же, что у остальных секций окна узла (nodeWindowTarget).
	target, bound := nodeWindowTarget(scope)
	canSave := bound && ac.CanSaveTailscaleExitNode(target.MachineID, tag)
	cmd := tailscaleCommands{ac: ac, target: target}

	type snapKey struct {
		view     tailscaleNetworkView
		state    string
		written  string
		haveCtrl bool
	}
	var (
		last     *snapKey
		fresh    services.TailscaleStatus // последний ответ GetTailscaleStatus
		freshAt  time.Time
		polledAt time.Time
		noPaths  bool   // ядро без GetTailscaleStatus — только стрим
		lastErr  string // в журнал — только смена ошибки, не каждый опрос
	)
	// Раскрытые адреса устройств (по StableID) переживают перерисовку;
	// трогается только из UI-потока (fyne.Do и тап).
	expanded := map[string]bool{}
	refresh := func() {
		var (
			st      services.TailscaleStatus
			ok      bool
			running bool
			live    bool
		)
		haveCtrl := false
		if bound {
			running = ac.TailscaleCoreRunning(cmd.target)
			st, ok = ac.TailscaleStatus(cmd.target, tag)
			live = ac.TailscaleLive(cmd.target)
			haveCtrl = ac.TailscaleControlAvailable(cmd.target)
		}
		if haveCtrl && running && !noPaths && time.Since(polledAt) >= tailnetPollInterval {
			polledAt = time.Now()
			now, nowOK, err := ac.TailscaleStatusNow(cmd.target, tag)
			switch {
			case errors.Is(err, services.ErrTailscalePathUnsupported):
				noPaths = true
			case err != nil:
				if err.Error() != lastErr {
					lastErr = err.Error()
					debuglog.WarnLog("node network: tailscale status %s: %v", tag, err)
				}
			case nowOK:
				fresh, freshAt, lastErr = now, polledAt, ""
			}
		}
		// Ответ старше трёх опросов (ядро перестало отвечать) не держим:
		// стрим хотя бы знает, что узел жив.
		if !freshAt.IsZero() && time.Since(freshAt) < 3*tailnetPollInterval {
			st, ok = fresh, true
		}
		written := writtenExitNode(cfgPath, tag)
		key := snapKey{view: tailscaleNetworkViewFor(running, live, ok), state: tailscaleNetworkKey(st), written: written, haveCtrl: haveCtrl}
		if last != nil && *last == key {
			return
		}
		last = &key
		fyne.Do(func() {
			var draw func()
			draw = func() {
				box.RemoveAll()
				buildTailscaleNetwork(cmd, win, box, tag, key.view, st, written, canSave, key.haveCtrl,
					deviceExpand{open: expanded, redraw: draw})
				box.Refresh()
			}
			draw()
		})
	}
	go func() {
		refresh()
		t := time.NewTicker(time.Second) // стрим — раз в секунду, запрос — реже (tailnetPollInterval)
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

// tailscaleNetworkKey — показываемое содержимое статуса строкой: ключ
// перерисовки. Свежий ответ приходит каждые tailnetPollInterval с новым
// ReceivedAt и бегущими счётчиками — по ним перерисовка закрывала бы открытый
// список Exit node, поэтому то, чего вкладка не показывает, обнуляется.
func tailscaleNetworkKey(st services.TailscaleStatus) string {
	peer := func(p services.TailscalePeer) services.TailscalePeer {
		p.RxBytes, p.TxBytes, p.Active, p.LastHandshake = 0, 0, false, time.Time{}
		return p
	}
	var self, exit services.TailscalePeer
	if st.Self != nil {
		self = peer(*st.Self)
	}
	if st.ExitNode != nil {
		exit = peer(*st.ExitNode)
	}
	groups := make([]services.TailscaleUserGroup, len(st.UserGroups))
	for i, g := range st.UserGroups {
		groups[i] = g
		groups[i].Peers = make([]services.TailscalePeer, len(g.Peers))
		for j, p := range g.Peers {
			groups[i].Peers[j] = peer(p)
		}
	}
	st.Self, st.ExitNode, st.UserGroups, st.ReceivedAt = nil, nil, nil, time.Time{}
	return fmt.Sprintf("%+v|%+v|%+v|%+v", st, self, exit, groups)
}

// deviceExpand — раскрытые адреса устройств и перерисовка вкладки по тапу «+N».
type deviceExpand struct {
	open   map[string]bool
	redraw func()
}

// tailscaleCommands — команды вкладки, привязанные к ядру окна (область и
// машина, запомненная при открытии).
type tailscaleCommands struct {
	ac     *core.AppController
	target core.TailscaleTarget
}

// saveExitNode — Save choice. Local: запись в профиль Local и пересборка
// bin/config.json (SaveTailscaleExitNode). Remote: запись в профиль ЭТОЙ
// машины и пересборка её config.json тем же шагом, что Save её Мастера
// (configurator.RebuildMachineConfig). Машине конфиг не отправляется — как
// и после Save Мастера; remoteWritten=true — показать тот же признак, что
// показывает Мастер («Remote config exported»), доставка за Deploy config.
func (c tailscaleCommands) saveExitNode(tag, value string) (remoteWritten bool, err error) {
	if c.target.Scope != services.ScopeRemote {
		return false, c.ac.SaveTailscaleExitNode(tag, value)
	}
	if err := core.WriteTailscaleExitNodeChoice(c.ac.FileService.Layout.Data, c.target.MachineID, tag, value); err != nil {
		return false, err
	}
	if _, err := configurator.RebuildMachineConfig(c.target.MachineID); err != nil {
		return false, err
	}
	return true, nil
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

func buildTailscaleNetwork(cmd tailscaleCommands, win fyne.Window, box *fyne.Container, tag string,
	view tailscaleNetworkView, st services.TailscaleStatus, written string, canSave, haveCtrl bool, expand deviceExpand) {
	ac := cmd.ac
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

	// --- Status: заголовок, состояние и кнопки входа одной строкой ---
	word, _ := networksRowState(true, st, true)
	stateMark := widget.NewLabel("○")
	stateMark.Importance = widget.LowImportance
	if st.BackendState == services.TailscaleStateRunning {
		stateMark.SetText("●")
		stateMark.Importance = widget.SuccessImportance
	}
	stateText := widget.NewLabel(word)
	stateText.Truncation = fyne.TextTruncateEllipsis
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
						if err := ac.TailscaleLogout(cmd.target, tag); err != nil {
							debuglog.WarnLog("tailscale %s: logout failed", tag)
							fyne.Do(func() { ShowError(win, err) })
						}
					}()
				}, win)
		}))
	}
	box.Add(container.NewBorder(nil, nil,
		container.NewHBox(sectionHeader(locale.T("Status")), stateMark), buttons, stateText))
	if st.NetworkName != "" {
		network := st.NetworkName
		if st.KeyAuth {
			network += " · " + locale.T("signed in with a key")
		}
		box.Add(infoRow(locale.T("Network"), network))
	}

	// --- This device ---
	if st.Self != nil {
		self := *st.Self
		box.Add(widget.NewSeparator())
		box.Add(sectionHeader(locale.T("This device")))
		box.Add(infoRow(locale.T("Name"), self.HostName))
		if dns := strings.TrimSuffix(self.DNSName, "."); dns != "" {
			box.Add(infoRow(locale.T("MagicDNS name"), dns))
		}
		if len(self.TailscaleIPs) > 0 {
			box.Add(infoRow(locale.T("Address"), strings.Join(self.TailscaleIPs, " · ")))
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
	exitKey := container.NewHBox(sectionHeader(locale.T("Exit node")))
	if diff != services.ExitNodeSame {
		exitKey.Add(widget.NewIcon(theme.WarningIcon()))
	}
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
			if err := ac.TailscaleSetExitNode(cmd.target, tag, stableID); err != nil {
				debuglog.WarnLog("tailscale %s: exit node not switched", tag)
				fyne.Do(func() { ShowError(win, err) })
			}
		}()
	}
	if !haveCtrl {
		sel.Disable()
	}
	box.Add(container.NewBorder(nil, nil, exitKey, nil, sel))
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
					remoteWritten, err := cmd.saveExitNode(tag, value)
					if err != nil {
						debuglog.WarnLog("tailscale %s: exit node choice not saved", tag)
					}
					fyne.Do(func() {
						if err != nil {
							save.Enable()
							ShowError(win, err)
							return
						}
						if remoteWritten {
							dialog.ShowInformation(locale.T("Remote config exported"),
								locale.T("The config for the remote machine was written to a separate file. The local bin/config.json was not touched."), win)
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
	box.Add(diagnosticsHeader(locale.T("Devices"), locale.Tf("%d online / %d", online, total)))
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
			box.Add(infoKeyCell(owner))
		}
		for i, p := range services.SortDevices(g.Peers) {
			if i > 0 {
				box.Add(widget.NewSeparator())
			}
			box.Add(tailscaleDeviceRow(cmd, win, tag, p, haveCtrl, expand))
		}
	}
}

// tailscaleDeviceDetails — кто это (первая строка устройства, §6): ОС и
// роли. Имя и адрес стоят в строке отдельными полями, «в сети» показывает
// точка; как подключено — во второй строке (tailscaleDeviceConnection).
func tailscaleDeviceDetails(p services.TailscalePeer) string {
	parts := make([]string, 0, 3)
	if p.OS != "" {
		parts = append(parts, p.OS)
	}
	if p.ExitNodeOption {
		parts = append(parts, locale.T("exit node"))
	}
	if p.ShareeNode {
		parts = append(parts, locale.T("shared"))
	}
	return strings.Join(parts, " · ")
}

// tailscaleDeviceConnection — как подключено (третья строка): путь словами
// (SPEC 158; знак пути — SVG перед строкой); в сети без пути — «no traffic
// yet» (Tailscale поднимает туннель к пиру по первому пакету, вердикта ядру
// не из чего сделать); не в сети — давность last seen; ключ истёк — туда же.
func tailscaleDeviceConnection(p services.TailscalePeer, now time.Time) string {
	parts := make([]string, 0, 2)
	if path := tailscalePathWords(p); path != "" {
		parts = append(parts, path)
	} else if p.Online {
		parts = append(parts, locale.T("no traffic yet"))
	} else if !p.LastSeen.IsZero() {
		parts = append(parts, locale.Tf("last seen %s ago", humanAge(now.Sub(p.LastSeen))))
	}
	if p.Expired {
		parts = append(parts, locale.T("key expired"))
	}
	return strings.Join(parts, " · ")
}

// tailscaleDeviceRow — устройство строкой списка серверов
// (tailscalePeerLines) и меню справа: Copy name, Copy MagicDNS name, Copy
// address (адресов несколько — подменю со всеми), Ping (по первому адресу).
func tailscaleDeviceRow(cmd tailscaleCommands, win fyne.Window, tag string, p services.TailscalePeer, haveCtrl bool, expand deviceExpand) fyne.CanvasObject {
	addr := ""
	if len(p.TailscaleIPs) > 0 {
		addr = p.TailscaleIPs[0]
	}
	dns := strings.TrimSuffix(p.DNSName, ".")

	var more *widget.Button
	more = widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItem(locale.T("Copy name"), func() { setClipboard(p.HostName) }),
		}
		if dns != "" {
			items = append(items, fyne.NewMenuItem(locale.T("Copy MagicDNS name"), func() { setClipboard(dns) }))
		}
		switch len(p.TailscaleIPs) {
		case 0:
		case 1:
			items = append(items, fyne.NewMenuItem(locale.T("Copy address"), func() { setClipboard(addr) }))
		default:
			sub := make([]*fyne.MenuItem, 0, len(p.TailscaleIPs))
			for _, ip := range p.TailscaleIPs {
				ip := ip
				sub = append(sub, fyne.NewMenuItem(ip, func() { setClipboard(ip) }))
			}
			copyAddr := fyne.NewMenuItem(locale.T("Copy address"), nil)
			copyAddr.ChildMenu = fyne.NewMenu("", sub...)
			items = append(items, copyAddr)
		}
		if addr != "" && haveCtrl {
			items = append(items, fyne.NewMenuItem(locale.T("Ping"), func() {
				showTailscalePing(cmd, win, tag, p.HostName, addr)
			}))
		}
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(more)
		widget.NewPopUpMenu(fyne.NewMenu("", items...), win.Canvas()).
			ShowAtPosition(pos.Add(fyne.NewPos(0, more.Size().Height)))
	})
	more.Importance = widget.LowImportance

	id := p.StableID
	if id == "" {
		id = p.HostName + "|" + addr
	}
	var toggle func()
	if expand.redraw != nil {
		toggle = func() {
			expand.open[id] = !expand.open[id]
			expand.redraw()
		}
	}
	lines := tailscalePeerLines(p, tailscaleDeviceConnection(p, time.Now()), expand.open[id], toggle)
	return container.NewBorder(nil, nil, nil, container.NewCenter(more), lines)
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
func showTailscalePing(cmd tailscaleCommands, win fyne.Window, tag, name, addr string) {
	lines := container.NewVBox(widget.NewProgressBarInfinite())
	ctx, cancel := context.WithCancel(context.Background())
	d := dialog.NewCustom(locale.Tf("Ping %s", name), locale.T("Close"), lines, win)
	d.SetOnClosed(cancel)
	d.Resize(fyne.NewSize(420, 260))
	d.Show()
	go func() {
		first := true
		err := cmd.ac.TailscalePing(ctx, cmd.target, tag, addr, func(r services.TailscalePingResult) {
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
