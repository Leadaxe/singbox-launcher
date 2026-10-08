// File servers_node_diagnostics_tab.go — вкладка Diagnostics окна узла, по
// образцу LxBox: GET через узел к одному из заранее заданных адресов и сырой
// ответ; у цепочки сверху — замер по позициям (addChainSection).
//
// Запрос идёт через узел работающего ядра (gRPC GetURLViaOutbound: локальный
// демон или удалённая машина); активный selector не переключается. В classic
// у Clash API такого запроса нет — вкладка говорит об этом одной строкой.
// Результаты не сохраняются: живут, пока открыто окно.
package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// diagnosticCheck — проверка из фиксированного списка; свой URL не вводится
// намеренно (как в LxBox).
type diagnosticCheck struct {
	label string
	url   string
}

var diagnosticChecks = []diagnosticCheck{
	{"Cloudflare trace", "https://1.1.1.1/cdn-cgi/trace"},
	{"Cloudflare trace (hostname)", "https://cloudflare.com/cdn-cgi/trace"},
	{"IP & location", "https://api.ip2location.io/"},
	{"IP info", "https://ipinfo.io/json"},
}

// urlViaOutboundSourceFor — источник запроса через узел для ядра окна;
// ok=false в classic, у машины без gRPC и у машины, которая больше не выбрана.
func urlViaOutboundSourceFor(ac *core.AppController, target core.CoreTarget) (services.URLViaOutboundSource, bool) {
	if ac == nil {
		return nil, false
	}
	tr, ok := proxyTransportFor(ac, target)
	if !ok {
		return nil, false
	}
	src, ok := tr.(services.URLViaOutboundSource)
	return src, ok
}

// nodeDiagnosticsTab — содержимое вкладки Diagnostics.
func nodeDiagnosticsTab(ac *core.AppController, target core.CoreTarget, bound bool, win fyne.Window, proxy api.ProxyInfo, node *wizardbusiness.ConfigNode, scope services.ProxyScope) fyne.CanvasObject {
	body := container.NewVBox()

	// Цепочка: позиции и послойный замер — только там, где ядро отвечает по
	// gRPC (см. addChainSection).
	if bound && node.Type == configtypes.ChainOutboundType {
		addChainSection(ac, target, body, win, proxy.Name)
	}
	// Tailnet: путь пиров и предупреждения ядра по запросу (SPEC 158) —
	// показывается и у узла без выхода: именно там и ищут причину.
	if bound && node.Type == configtypes.SchemeTailscale {
		addTailnetSection(ac, target, body, win, proxy.Name)
	}

	note := func(text string, imp widget.Importance) {
		l := widget.NewLabel(text)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = imp
		body.Add(l)
	}

	switch {
	case !bound:
		note(locale.T("Diagnostics needs the daemon mode or a remote machine."), widget.LowImportance)
	case node.IsGroup():
		note(locale.T("This is a group — it has no connection of its own. Check its members."), widget.LowImportance)
	case tailscaleHasNoExit(ac, scope, proxy):
		note(locale.T("This node has no exit. Check devices on the Network tab."), widget.WarningImportance)
	default:
		if _, ok := urlViaOutboundSourceFor(ac, target); !ok {
			note(locale.T("Diagnostics needs the daemon mode or a remote machine."), widget.LowImportance)
		} else {
			buildDiagnosticsCheck(ac, target, body, proxy.Name)
		}
	}
	return withScrollGutter(body)
}

// buildDiagnosticsCheck — секции Check и Response.
func buildDiagnosticsCheck(ac *core.AppController, target core.CoreTarget, body *fyne.Container, tag string) {
	labels := make([]string, len(diagnosticChecks))
	for i, c := range diagnosticChecks {
		labels[i] = c.label
	}
	selected := 0

	urlLabel := widget.NewLabel(diagnosticChecks[0].url)
	urlLabel.Selectable = true
	urlLabel.Importance = widget.LowImportance
	urlLabel.Truncation = fyne.TextTruncateEllipsis

	runErr := newChainErrLabel()
	response := container.NewVBox()

	var run *widget.Button
	sel := widget.NewSelect(labels, nil)
	sel.SetSelectedIndex(0)
	sel.OnChanged = func(string) {
		selected = sel.SelectedIndex()
		if selected < 0 {
			selected = 0
		}
		urlLabel.SetText(diagnosticChecks[selected].url)
		// Ответ относится к прошлому адресу.
		response.Objects = nil
		response.Refresh()
		runErr.Hide()
	}

	run = widget.NewButtonWithIcon(locale.T("Run"), theme.MediaPlayIcon(), nil)
	run.Importance = widget.HighImportance
	run.OnTapped = func() {
		src, ok := urlViaOutboundSourceFor(ac, target)
		if !ok {
			runErr.SetText(errEndpointSourceGone.Error())
			runErr.Show()
			return
		}
		url := diagnosticChecks[selected].url
		run.Disable()
		sel.Disable()
		run.SetText(locale.T("Running…"))
		runErr.Hide()
		response.Objects = nil
		response.Refresh()
		go func() {
			res, err := src.GetURLViaOutbound(tag, url)
			if err != nil {
				debuglog.WarnLog("node diagnostics: %s via %s: %v", url, tag, err)
			}
			fyne.Do(func() {
				run.SetText(locale.T("Run"))
				run.Enable()
				sel.Enable()
				if err != nil {
					runErr.SetText(err.Error())
					runErr.Show()
					return
				}
				fillDiagnosticsResponse(response, res)
			})
		}()
	}

	body.Add(widget.NewSeparator())
	body.Add(diagnosticsHeader(locale.T("Check"), locale.T("Request is sent through this node")))
	body.Add(container.NewBorder(nil, nil, infoKeyCell(locale.T("Endpoint")), nil, sel))
	body.Add(container.NewBorder(nil, nil, infoKeyCell(""), nil, urlLabel))
	body.Add(container.NewBorder(nil, nil, nil, run))
	body.Add(runErr)
	body.Add(response)
}

// fillDiagnosticsResponse — секция Response: статус и время, сырое тело,
// обрезка и адрес, к которому подключилось ядро.
func fillDiagnosticsResponse(box *fyne.Container, res services.URLViaOutboundResult) {
	box.Objects = nil
	box.Add(widget.NewSeparator())
	box.Add(diagnosticsHeader(locale.T("Response"), locale.T("through this node in the running core")))

	if res.Error != "" {
		l := newChainErrLabel()
		l.SetText(res.Error)
		l.Show()
		box.Add(l)
		box.Refresh()
		return
	}

	// Не-2xx — результат, а не сбой: подсвечиваем иначе, но показываем тело.
	statusLine := widget.NewLabel(fmt.Sprintf("%d · %d ms", res.Status, res.Elapsed.Milliseconds()))
	statusLine.Importance = widget.SuccessImportance
	if res.Status < 200 || res.Status > 299 {
		statusLine.Importance = widget.WarningImportance
	}
	text := diagnosticsBodyText(res.Body)
	copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() { setClipboard(text) })
	copyBtn.Importance = widget.LowImportance
	box.Add(container.NewBorder(nil, nil, statusLine, copyBtn))

	shown := text
	if shown == "" {
		shown = locale.T("(empty response)")
	}
	bodyLabel := widget.NewLabel(shown)
	bodyLabel.Selectable = true
	bodyLabel.Wrapping = fyne.TextWrapBreak
	bodyLabel.TextStyle = fyne.TextStyle{Monospace: true}
	box.Add(widget.NewCard("", "", bodyLabel))

	if res.Truncated {
		box.Add(diagnosticsCaption(locale.T("Response was longer and got cut off.")))
	}
	if res.RemoteAddr != "" {
		box.Add(diagnosticsCaption(locale.Tf("Connected to %s from inside the tunnel", res.RemoteAddr)))
	}
	box.Refresh()
}

// diagnosticsBodyText — тело как текст; не-UTF-8 байты заменяются: адрес
// произвольный, валидный UTF-8 не гарантирован.
func diagnosticsBodyText(b []byte) string {
	return strings.ToValidUTF8(string(b), "\uFFFD")
}

func diagnosticsCaption(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	l.Wrapping = fyne.TextWrapWord
	return l
}

// diagnosticsHeader — «Заголовок  подпись серым» одной строкой. Подпись
// обрезается многоточием, а не переносится: в строке с заголовком перенос
// схлопнул бы её в столбик.
func diagnosticsHeader(title, caption string) fyne.CanvasObject {
	c := widget.NewLabel(caption)
	c.Importance = widget.LowImportance
	c.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, sectionHeader(title), nil, c)
}
