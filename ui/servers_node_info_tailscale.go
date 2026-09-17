// File servers_node_info_tailscale.go — секция «Tailscale» окна Info узла
// (SPEC 130). Образец — addChainSection: гейт, чтение кеша, fyne.Do.
//
// Секция, а не отдельная вкладка: у окна одна форма (решение владельца
// 16.09.2026), вкладки не появляются. Всё tailnet-специфичное — здесь;
// список endpoint'ов (core_runtime_window.go) несёт только слово состояния.
//
// Login — КНОПКА, не строка с URL: ссылка входа глазу бесполезна, ей нужно
// открыться в браузере. Сегодня она видна только в логе ядра, на роутере —
// нигде; это самая ценная строка секции.
package ui

import (
	"fmt"
	"sort"
	"strings"
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

// tailscaleStateLabel — слово состояния для UI по BackendState ядра.
//
// Слова наши (locale.T), а не StateText ядра: locale в gRPC-метаданных не
// шлётся, и текст пришёл бы на дефолте ядра. Неизвестное состояние —
// как есть: ядро новее лаунчера, врать словом хуже, чем показать сырое.
func tailscaleStateLabel(state string) string {
	switch state {
	case services.TailscaleStateRunning:
		return locale.T("running")
	case services.TailscaleStateStarting:
		return locale.T("starting")
	case services.TailscaleStateNeedsLogin:
		return locale.T("needs login")
	case services.TailscaleStateStopped:
		return locale.T("stopped")
	case services.TailscaleStateNoState, "":
		return locale.T("no state")
	}
	return state
}

// addTailscaleSection добавляет секцию, если источник умеет статус tailnet.
// Читает кеш — сети нет, но fyne.Do всё равно: единообразно с chain.
func addTailscaleSection(ac *core.AppController, body *fyne.Container, tag string) {
	if ac == nil || body == nil || !ac.TailscaleAvailable() {
		return
	}
	box := container.NewVBox()
	body.Add(box)

	go func(epTag string) {
		st, ok := ac.TailscaleStatus(epTag)
		live := ac.TailscaleLive()
		fyne.Do(func() {
			buildTailscaleSection(box, st, ok, live)
			box.Refresh()
		})
	}(tag)
}

// buildTailscaleSection рисует секцию. ok=false — статуса по тегу нет:
// показываем причину словами, а не пустую секцию (иначе «ничего нет» и
// «ещё не пришло» неотличимы).
func buildTailscaleSection(box *fyne.Container, st services.TailscaleStatus, ok, live bool) {
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.T("Tailscale")))

	if !ok {
		msg := locale.T("No tailnet status for this node yet — the core has not reported it.")
		if !live {
			msg = locale.T("Tailnet status stream is not connected.")
		}
		l := widget.NewLabel(msg)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = widget.LowImportance
		box.Add(l)
		return
	}

	state := tailscaleStateLabel(st.BackendState)
	if !live {
		// Снимок устарел: стрим оборван, показываем возраст, а не свежесть.
		state += " · " + locale.Tf("stale, %s ago", humanAge(time.Since(st.ReceivedAt)))
	}
	box.Add(infoRow(locale.T("State"), state))

	// Login — только когда ядро ждёт входа и есть куда идти. Кнопка, не URL.
	if st.BackendState == services.TailscaleStateNeedsLogin && strings.TrimSpace(st.AuthURL) != "" {
		url := st.AuthURL
		btn := widget.NewButton(locale.T("Open login URL"), func() {
			if err := platform.OpenURL(url); err != nil {
				debuglog.WarnLog("tailscale login url: %v", err)
			}
		})
		btn.Importance = widget.HighImportance
		box.Add(container.NewHBox(btn))
	}

	auth := locale.T("interactive")
	if st.KeyAuth {
		auth = "auth_key" // l10n-exempt: config field name
	}
	box.Add(infoRow(locale.T("Auth"), auth))

	if st.NetworkName != "" || st.MagicDNSSuffix != "" {
		net := st.NetworkName
		if st.MagicDNSSuffix != "" {
			if net != "" {
				net += " · "
			}
			net += "MagicDNS " + st.MagicDNSSuffix // l10n-exempt: product term
		}
		box.Add(infoRow(locale.T("Network"), net))
	}

	// Self пуст до входа (NoState/NeedsLogin) — строки просто нет.
	if st.Self != nil {
		box.Add(infoRow(locale.T("This node"), tailscalePeerLine(*st.Self)))
	}

	exit := locale.T("none")
	if st.ExitNode != nil {
		exit = tailscalePeerLine(*st.ExitNode)
		if st.ExitNode.Online {
			exit += " · " + locale.T("online")
		} else {
			exit += " · " + locale.T("offline")
		}
	}
	box.Add(infoRow(locale.T("Exit node"), exit))

	online, total := st.PeersOnline()
	box.Add(widget.NewSeparator())
	box.Add(sectionHeader(locale.Tf("Peers (%d online / %d)", online, total)))
	if total == 0 {
		l := widget.NewLabel(locale.T("No peers in this tailnet."))
		l.Importance = widget.LowImportance
		box.Add(l)
		return
	}
	// Группы — по пользователю, как в статусе; внутри онлайновые первыми.
	for _, g := range st.UserGroups {
		if len(g.Peers) == 0 {
			continue
		}
		owner := g.DisplayName
		if owner == "" {
			owner = g.LoginName
		}
		if owner != "" && len(st.UserGroups) > 1 {
			h := widget.NewLabel(owner)
			h.Importance = widget.LowImportance
			box.Add(h)
		}
		peers := append([]services.TailscalePeer(nil), g.Peers...)
		sort.SliceStable(peers, func(i, j int) bool {
			if peers[i].Online != peers[j].Online {
				return peers[i].Online
			}
			return peers[i].HostName < peers[j].HostName
		})
		for _, p := range peers {
			box.Add(tailscalePeerRow(p))
		}
	}
}

// tailscalePeerLine — «hostname · ip» одной строкой.
func tailscalePeerLine(p services.TailscalePeer) string {
	parts := make([]string, 0, 2)
	if p.HostName != "" {
		parts = append(parts, p.HostName)
	}
	if len(p.TailscaleIPs) > 0 {
		parts = append(parts, p.TailscaleIPs[0])
	}
	return strings.Join(parts, " · ")
}

// tailscalePeerRow — строка пира: маркер онлайна, имя, IP, ОС, роль, трафик
// или «last seen». Метки — словами; иконок в наборе проекта под это нет.
func tailscalePeerRow(p services.TailscalePeer) fyne.CanvasObject {
	mark := "○"
	if p.Online {
		mark = "●"
	}
	name := widget.NewLabel(mark + " " + p.HostName)
	name.TextStyle.Bold = p.Online

	details := make([]string, 0, 5)
	if len(p.TailscaleIPs) > 0 {
		details = append(details, p.TailscaleIPs[0])
	}
	if p.OS != "" {
		details = append(details, p.OS)
	}
	switch {
	case p.ExitNode:
		details = append(details, locale.T("exit node (in use)"))
	case p.ExitNodeOption:
		details = append(details, locale.T("exit node"))
	}
	if p.Online {
		if p.RxBytes > 0 || p.TxBytes > 0 {
			details = append(details, fmt.Sprintf("↓%s ↑%s", hostBytes(uint64(p.RxBytes)), hostBytes(uint64(p.TxBytes))))
		}
	} else if !p.LastSeen.IsZero() {
		details = append(details, locale.Tf("last seen %s ago", humanAge(time.Since(p.LastSeen))))
	}
	if p.Expired {
		details = append(details, locale.T("expired"))
	}
	sub := widget.NewLabel(strings.Join(details, " · "))
	sub.Importance = widget.LowImportance
	sub.Wrapping = fyne.TextWrapWord

	return container.NewVBox(name, sub)
}

// humanAge — «5m», «2h», «3d»: возраст снимка и last seen.
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return locale.Tf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return locale.Tf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return locale.Tf("%dh", int(d.Hours()))
	}
	return locale.Tf("%dd", int(d.Hours()/24))
}
