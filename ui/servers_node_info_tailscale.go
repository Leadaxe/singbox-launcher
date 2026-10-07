// File servers_node_info_tailscale.go — общие слова и строки tailnet для
// окна узла и списка endpoint'ов (SPEC 130). Вкладка Network окна узла —
// servers_node_network_tab.go (SPEC 148): решение владельца 27.09.2026
// заменило секцию «Tailscale» вкладки Details отдельной вкладкой.
package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/locale"
)

// tailscaleDeviceLines — устройство в две строки, как узел в списке серверов:
// верх — виджеты строки (точка, имя, адрес, ОС), низ — подстрока «как
// подключено» тем же canvas.Text, что подзаголовок списка (serversSubtitle*),
// без собственных отступов: Label в верхней строке уже несёт внутренний
// отступ снизу, поэтому зазор 0. indent — начало подстроки под именем.
func tailscaleDeviceLines(top fyne.CanvasObject, conn string, indent float32) fyne.CanvasObject {
	sub := canvas.NewText(truncateSubtitle(conn), theme.Color(theme.ColorNamePlaceHolder))
	sub.TextSize = serversSubtitleTextSize
	line := container.NewBorder(nil, nil, rowGap(indent), nil, sub)
	return container.New(tightVBoxLayout{gap: 0}, top, line)
}

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

// tailscalePathGlyphDirect / tailscalePathGlyphRelay — знак перед путём:
// «⚡» — пакеты ходят прямо между узлами, «☁» — через посредника (DERP или
// peer relay). Эмодзи: текстовые стрелки ↝/↪ шрифт либо не знает, либо
// рисует цветным глифом EmojiOne, так что честнее сразу взять картинку.
const (
	tailscalePathGlyphDirect = "⚡"
	tailscalePathGlyphRelay  = "☁"
)

// tailscalePathText — путь пира словами (SPEC 158, таблица CONSUMERS ядра):
// «⚡ direct 1.2.3.4:41641», «☁ peer relay», «☁ relay fra»; пусто — узел ни
// разу не слал пиру, путь не выбран. Домашний регион при direct не пишем: в
// строке устройства он только шум, а в Diagnostics код виден в строке relay.
func tailscalePathText(p services.TailscalePeer) string {
	switch p.Path {
	case services.TailscalePathDirect:
		if p.Endpoint != "" {
			return tailscalePathGlyphDirect + " " + locale.T("direct") + " " + p.Endpoint
		}
		return tailscalePathGlyphDirect + " " + locale.T("direct")
	case services.TailscalePathPeerRelay:
		return tailscalePathGlyphRelay + " " + locale.T("peer relay")
	case services.TailscalePathDERP:
		if p.DERPRegionCode != "" {
			return tailscalePathGlyphRelay + " " + locale.T("relay") + " " + p.DERPRegionCode
		}
		return tailscalePathGlyphRelay + " " + locale.T("relay")
	}
	return ""
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
