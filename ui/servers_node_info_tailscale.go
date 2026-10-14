// File servers_node_info_tailscale.go — общие слова и строки tailnet для
// окна узла и списка endpoint'ов (SPEC 130). Вкладка Network окна узла —
// servers_node_network_tab.go (SPEC 148): решение владельца 27.09.2026
// заменило секцию «Tailscale» вкладки Details отдельной вкладкой.
package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/icons"
)

// tailscalePeerLines — устройство tailnet строкой списка серверов (SPEC 095):
// те же canvas.Text без собственных отступов, имя жирным, пояснения мелким
// серым; адреса — справа, первый в строке имени, второй в строке ОС:
//
//	● GL-MT2500                              100.104.79.7
//	  linux · exit node     fd7a:115c:a1e0::9f01:4f07 +1 ▾
//	  ☁ relay ams
//
// Третья строка держит высоту и пустой, чтобы список не «дышал», когда у
// устройства появляется путь. Адресов больше двух — «+N ▾» раскрывает
// остальные под вторым (onToggle перестраивает строку с expanded); nil —
// без раскрытия, показываются первые два.
func tailscalePeerLines(p services.TailscalePeer, conn string, expanded bool, onToggle func()) fyne.CanvasObject {
	fg := theme.Color(theme.ColorNameForeground)
	dim := theme.Color(theme.ColorNamePlaceHolder)
	text := func(s string, size float32, c color.Color) *canvas.Text {
		t := canvas.NewText(s, c)
		t.TextSize = size
		return t
	}

	mark := text("○", serversNameTextSize, dim)
	if p.Online {
		mark.Text = "●"
		mark.Color = theme.Color(theme.ColorNameSuccess)
	}
	name := text(truncateRunes(p.HostName, serversNameMaxRunes), serversNameTextSize, fg)
	name.TextStyle.Bold = true

	ips := p.TailscaleIPs
	first, second := "", ""
	if len(ips) > 0 {
		first = ips[0]
	}
	if len(ips) > 1 {
		second = ips[1]
	}
	line1 := container.NewBorder(nil, nil, nil, text(first, serversNameTextSize, fg), name)

	right2 := container.NewHBox(text(second, serversSubtitleTextSize, dim))
	if len(ips) > 2 && onToggle != nil {
		caret := theme.MenuDropDownIcon()
		toggle := container.NewHBox()
		if expanded {
			caret = theme.MenuDropUpIcon()
		} else {
			toggle.Add(text(fmt.Sprintf("+%d", len(ips)-2), serversSubtitleTextSize, dim))
		}
		toggle.Add(tailnetInlineIcon(theme.NewColoredResource(caret, theme.ColorNamePlaceHolder)))
		right2.Add(fynewidget.NewTapWrap(toggle, onToggle))
	}
	details := text(truncateSubtitle(tailscaleDeviceDetails(p)), serversSubtitleTextSize, dim)
	lines := container.New(tightVBoxLayout{gap: serversTitleSubtitleGap},
		line1, container.NewBorder(nil, nil, nil, right2, details))
	if len(ips) > 2 && (expanded || onToggle == nil) {
		// Без раскрытия (onToggle nil) остальные адреса не прячем вовсе:
		// показывать нечем, а терять их хуже, чем занять строку.
		for _, ip := range ips[2:] {
			lines.Add(container.NewBorder(nil, nil, nil, text(ip, serversSubtitleTextSize, dim), nil))
		}
	}

	var icon fyne.CanvasObject
	if res := tailscalePathIcon(p.Path); res != nil {
		icon = tailnetInlineIcon(res)
	}
	lines.Add(container.NewBorder(nil, nil, icon, nil, text(truncateSubtitle(conn), serversSubtitleTextSize, dim)))

	return container.NewBorder(nil, nil, container.New(tightVBoxLayout{}, mark), nil, lines)
}

// tailnetInlineIconSize — слот значка в мелкой строке: чуть шире высоты,
// облако низкое и широкое, молния узкая — обе вписываются (Contain).
var tailnetInlineIconSize = fyne.NewSize(16, 12)

// tailnetInlineIcon — SVG-значок на высоту мелкой строки (canvas.Image, а не
// widget.Icon: тот берёт размер иконки темы и раздул бы строку).
func tailnetInlineIcon(res fyne.Resource) fyne.CanvasObject {
	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(tailnetInlineIconSize)
	return img
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
// «📶» — пакеты ходят прямо между узлами, «☁» — через посредника (DERP или
// peer relay). Эмодзи: текстовые стрелки ↝/↪ шрифт либо не знает, либо
// рисует цветным глифом EmojiOne, так что честнее сразу взять картинку.
//
// НЕ «⚡» U+26A1: картинка в EmojiOneColor есть, но go-text/render v0.2.1
// (drawSVG) собирает прямоугольник вывода как image.Rect(XBearing, -YBearing,
// pixWidth, pixHeight) — размер подставлен вместо второго угла. Узкий глиф с
// большим левым отступом (молния: отступ 297, ширина 406 из 1000) сжимается до
// нескольких пустых столбцов и не рисуется вовсе; широкие (☁, ❌, 📶) задевает
// только обрезкой края. В upstream main то же самое. Выбирать знаки, чей
// рисунок занимает почти весь em-квадрат.
const (
	tailscalePathGlyphDirect = "📶"
	tailscalePathGlyphRelay  = "☁"
)

// tailscalePathWords — путь пира словами (SPEC 158, таблица CONSUMERS ядра):
// «direct 1.2.3.4:41641», «peer relay», «relay fra»; пусто — узел ни разу не
// слал пиру, путь не выбран. Домашний регион при direct не пишем: в строке
// устройства он только шум, а в Diagnostics код виден в строке relay.
func tailscalePathWords(p services.TailscalePeer) string {
	switch p.Path {
	case services.TailscalePathDirect:
		if p.Endpoint != "" {
			return locale.T("direct") + " " + p.Endpoint
		}
		return locale.T("direct")
	case services.TailscalePathPeerRelay:
		return locale.T("peer relay")
	case services.TailscalePathDERP:
		if p.DERPRegionCode != "" {
			return locale.T("relay") + " " + p.DERPRegionCode
		}
		return locale.T("relay")
	}
	return ""
}

// tailscalePathText — путь со знаком-эмодзи для строк, где есть только текст
// (подзаголовок списка серверов): «📶 direct 1.2.3.4:41641», «☁ relay fra».
// В строках устройств знак — SVG (tailscalePathIcon).
func tailscalePathText(p services.TailscalePeer) string {
	words := tailscalePathWords(p)
	switch p.Path {
	case services.TailscalePathDirect:
		return tailscalePathGlyphDirect + " " + words
	case services.TailscalePathPeerRelay, services.TailscalePathDERP:
		return tailscalePathGlyphRelay + " " + words
	}
	return words
}

// tailscalePathIcon — значок пути в строке устройства: зелёная молния —
// пакеты ходят прямо, оранжевое облако — через посредника (DERP или peer
// relay); nil — пути нет. SVG, а не эмодзи: глифы EmojiOneColor go-text/render
// масштабирует по экстентам рисунка и режет края (см. выше), значок выходит
// мелким и неровным.
func tailscalePathIcon(path services.TailscalePeerPath) fyne.Resource {
	switch path {
	case services.TailscalePathDirect:
		return theme.NewColoredResource(icons.Bolt, theme.ColorNameSuccess)
	case services.TailscalePathPeerRelay, services.TailscalePathDERP:
		return theme.NewColoredResource(icons.Cloud, theme.ColorNameWarning)
	}
	return nil
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
