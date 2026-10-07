// File servers_node_diagnostics_tailnet.go — секция Tailnet вкладки
// Diagnostics узла Tailscale (SPEC 158): путь каждого пира (direct / peer
// relay / DERP), возраст хендшейка и предупреждения ядра.
//
// Данные — запросный GetTailscaleStatus (SPEC 115 ядра), а не кеш потока:
// смена пути событием IPN-шины не является, и в потоке путь живёт только до
// следующего события. Опрос раз в tailnetPollInterval, пока окно открыто
// (windowOpen, как таймер вкладки Network); со старым ядром — одна строка и
// опрос останавливается. Имена устройств и адреса в журнал не пишутся.
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
)

// tailnetPollInterval — ориентир CONSUMERS ядра: 2–5 с, в фоне не опрашивать.
const tailnetPollInterval = 3 * time.Second

// addTailnetSection добавляет секцию и запускает опрос ядра окна (target).
func addTailnetSection(ac *core.AppController, target core.CoreTarget, body *fyne.Container, win fyne.Window, tag string) {
	if ac == nil || body == nil || !ac.TailscaleControlAvailable(target) {
		return
	}
	box := container.NewVBox(widget.NewProgressBarInfinite())
	body.Add(widget.NewSeparator())
	body.Add(diagnosticsHeader(locale.T("Tailnet"), locale.Tf("live status, refreshed every %d s", int(tailnetPollInterval.Seconds()))))
	body.Add(box)

	go func() {
		last := ""
		t := time.NewTicker(tailnetPollInterval)
		defer t.Stop()
		for {
			st, ok, err := ac.TailscaleStatusNow(target, tag)
			unsupported := errors.Is(err, services.ErrTailscalePathUnsupported)
			if err != nil && !unsupported {
				debuglog.WarnLog("node diagnostics: tailnet status %s: %v", tag, err)
			}
			lines := tailnetLines(st, ok, err, time.Now())
			if key := strings.Join(lines, "\n"); key != last {
				last = key
				fyne.Do(func() {
					box.RemoveAll()
					buildTailnetSection(box, st, ok, err, time.Now())
					box.Refresh()
				})
			}
			if unsupported {
				return
			}
			<-t.C
			if !windowOpen(win) {
				return
			}
		}
	}()
}

// tailnetLines — содержимое секции строками: ключ перерисовки, чтобы не
// пересобирать виджеты на каждый тик без изменений.
func tailnetLines(st services.TailscaleStatus, ok bool, err error, now time.Time) []string {
	if err != nil {
		return []string{"err:" + err.Error()}
	}
	if !ok {
		return []string{"not-running"}
	}
	lines := append([]string(nil), st.Health...)
	if st.ExitNode != nil {
		lines = append(lines, "exit:"+tailnetPeerDetails(*st.ExitNode, now)+st.ExitNode.HostName)
	} else {
		lines = append(lines, "exit:none")
	}
	for _, p := range tailnetPeersWithPath(st) {
		lines = append(lines, p.StableID+":"+tailnetPeerDetails(p, now))
	}
	return lines
}

// tailnetPeersWithPath — устройства, к которым путь выбран, в порядке
// вкладки Network; выход в список не входит — у него своя строка выше.
func tailnetPeersWithPath(st services.TailscaleStatus) []services.TailscalePeer {
	var all []services.TailscalePeer
	for _, g := range st.UserGroups {
		all = append(all, g.Peers...)
	}
	var out []services.TailscalePeer
	for _, p := range services.SortDevices(all) {
		if p.Path == services.TailscalePathNone {
			continue
		}
		if st.ExitNode != nil && p.StableID != "" && p.StableID == st.ExitNode.StableID {
			continue
		}
		out = append(out, p)
	}
	return out
}

// tailnetPeerDetails — «direct 1.2.3.4:41641 · handshake 36s ago».
func tailnetPeerDetails(p services.TailscalePeer, now time.Time) string {
	parts := make([]string, 0, 2)
	if path := tailscalePathText(p); path != "" {
		parts = append(parts, path)
	}
	if p.LastHandshake.IsZero() {
		parts = append(parts, locale.T("no handshake"))
	} else {
		parts = append(parts, locale.Tf("handshake %s ago", humanAge(now.Sub(p.LastHandshake))))
	}
	return strings.Join(parts, " · ")
}

// buildTailnetSection — тело секции по ответу ядра.
func buildTailnetSection(box *fyne.Container, st services.TailscaleStatus, ok bool, err error, now time.Time) {
	note := func(text string, imp widget.Importance) {
		l := widget.NewLabel(text)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = imp
		box.Add(l)
	}
	switch {
	case errors.Is(err, services.ErrTailscalePathUnsupported):
		note(locale.T("This core does not report peer paths (needs sing-box-lx 1.14.2-lx.12-rc.2)."), widget.LowImportance)
		return
	case err != nil:
		l := newChainErrLabel()
		l.SetText(err.Error())
		l.Show()
		box.Add(l)
		return
	case !ok:
		note(locale.T("The node is not running in the core."), widget.LowImportance)
		return
	}

	for _, h := range st.Health {
		note(h, widget.WarningImportance)
	}

	// Exit node: через какой выход идёт трафик и как к нему идёт путь.
	if st.ExitNode != nil {
		box.Add(container.NewBorder(nil, nil, infoKeyCell(locale.T("Exit node")), nil, tailnetPeerRow(*st.ExitNode, now)))
	} else {
		box.Add(infoRow(locale.T("Exit node"), locale.T("None")))
	}

	// Устройства с выбранным путём; без трафика — только счётчик: у них
	// пути нет по определению, и список из «пусто» ничего не скажет.
	peers := tailnetPeersWithPath(st)
	_, total := st.PeersOnline()
	if len(peers) > 0 {
		box.Add(container.NewBorder(nil, nil, infoKeyCell(locale.T("Devices")), nil, tailnetPeerRow(peers[0], now)))
		for _, p := range peers[1:] {
			box.Add(container.NewBorder(nil, nil, infoKeyCell(""), nil, tailnetPeerRow(p, now)))
		}
	}
	if st.ExitNode != nil {
		total--
	}
	if rest := total - len(peers); rest > 0 {
		box.Add(container.NewBorder(nil, nil, infoKeyCell(""), nil, diagnosticsCaption(locale.Tf("%d devices without traffic", rest))))
	}
}

// tailnetPeerRow — «●  GL-MT2500   100.104.79.7   direct 1.2.3.4:41641 ·
// handshake 36s ago», как строка устройства на вкладке Network.
func tailnetPeerRow(p services.TailscalePeer, now time.Time) fyne.CanvasObject {
	mark := widget.NewLabel("○")
	mark.Importance = widget.LowImportance
	if p.Online {
		mark.SetText("●")
		mark.Importance = widget.SuccessImportance
	}
	name := widget.NewLabel(p.HostName)
	addr := ""
	if len(p.TailscaleIPs) > 0 {
		addr = p.TailscaleIPs[0]
	}
	ip := widget.NewLabel(addr)
	ip.Selectable = true
	details := widget.NewLabel(tailnetPeerDetails(p, now))
	details.Importance = widget.LowImportance
	details.Truncation = fyne.TextTruncateEllipsis
	left := container.NewHBox(mark, name, rowGap(12), ip, rowGap(12))
	return container.NewBorder(nil, nil, left, nil, details)
}
