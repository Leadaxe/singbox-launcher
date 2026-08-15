package traffic

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	tprof "singbox-launcher/internal/traffic"
)

// parentWindowOf finds the Traffic Profiler window by title. Both Live
// tabs need a parent fyne.Window to attach the event-detail dialog to,
// but neither view stores the *fyne.Window directly (window.go owns it
// and passes only the WindowDeps bundle). Lookup by title avoids
// threading the window pointer through every sub-component.
//
// Falls back to the first available window — and to nil if there are
// none at all (caller is nil-safe).
// По ПРЕФИКСУ, а не по полному совпадению: во время записи заголовок несёт
// таймер («Traffic Profiler ⚡ Recording · 00:29», см. FormatRecordingTitle),
// и точное сравнение переставало находить своё окно ровно тогда, когда по
// строкам и кликают.
func parentWindowOf(deps WindowDeps) fyne.Window {
	if deps.App == nil {
		return nil
	}
	all := deps.App.Driver().AllWindows()
	for _, w := range all {
		if strings.HasPrefix(w.Title(), "Traffic Profiler") {
			return w
		}
	}
	if len(all) > 0 {
		return all[0]
	}
	return nil
}

// showEventDetail pops a modal with every field of `e` rendered as a
// formatted text block. Both Live tabs (system-wide and per-process)
// wire row OnSelected → this helper to give users a way to drill into
// rows that otherwise truncate to a one-liner.
//
// `parent` is the Traffic Profiler window — the dialog is attached
// there so closing the parent dismisses the popup too. nil-safe; if
// parent is missing we simply skip (avoids panicking from a stray
// background tick).
func showEventDetail(parent fyne.Window, e tprof.TrafficEvent) {
	showEventDetailWithDevice(parent, e, nil)
}

// showEventDetailWithDevice — то же окно плюс блок «что за устройство».
//
// dev необязателен: у своего ядра трафик идёт от процессов, справочника
// устройств там нет, и блок просто не рисуется.
func showEventDetailWithDevice(parent fyne.Window, e tprof.TrafficEvent, dev *DeviceInfo) {
	if parent == nil {
		return
	}
	text := formatEventDetail(e)
	if dev != nil {
		text += formatDeviceDetail(*dev)
	}
	body := widget.NewLabel(text)
	body.Wrapping = fyne.TextWrapWord
	body.TextStyle = fyne.TextStyle{Monospace: true}
	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(560, 360))
	d := dialog.NewCustom("Event detail", "Close", scroll, parent)
	d.Resize(fyne.NewSize(620, 440))
	d.Show()
}

// deviceFor — запись справочника по адресу клиента, nil если её нет.
//
// Принимает адрес и с портом, и без: в событии он приходит как «ip:port», а
// ключ справочника — голый IP. Резать порт в каждой точке вызова значило бы
// однажды забыть.
func deviceFor(deps WindowDeps, addr string) *DeviceInfo {
	if deps.ClientsInfo == nil || addr == "" {
		return nil
	}
	m, ok := deps.ClientsInfo()
	if !ok {
		return nil
	}
	d, ok := m[hostOfAddr(addr)]
	if !ok {
		return nil
	}
	return &d
}

// formatDeviceDetail — блок «Device» для окна деталей.
//
// Пустые поля пропускаются: у проводного клиента нет SSID, у беспроводного —
// порта моста, и печатать пустые строки значило бы выдавать норму за пробел в
// данных. Source печатаем всегда, когда он есть: при разборе «почему
// устройство потеряло имя» это первое, что спрашивают.
func formatDeviceDetail(d DeviceInfo) string {
	if d.Name == "" && d.MAC == "" && d.SSID == "" && d.Iface == "" && d.Port == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	if d.Name != "" {
		fmt.Fprintf(&b, "Device:      %s\n", d.Name)
	}
	if d.MAC != "" {
		fmt.Fprintf(&b, "MAC:         %s\n", d.MAC)
	}
	if d.SSID != "" {
		fmt.Fprintf(&b, "SSID:        %s\n", d.SSID)
	}
	if d.Iface != "" {
		fmt.Fprintf(&b, "Interface:   %s\n", d.Iface)
	}
	if d.Port != "" {
		fmt.Fprintf(&b, "Switch port: %s\n", d.Port)
	}
	if d.Source != "" {
		fmt.Fprintf(&b, "Source:      %s\n", d.Source)
	}
	return b.String()
}

// formatEventDetail returns a human-readable multi-line summary of every
// TrafficEvent field that's set. Empty / zero fields are omitted so the
// output stays compact for sparse events (e.g. a TCP-open without
// duration or close bytes).
func formatEventDetail(e tprof.TrafficEvent) string {
	var b strings.Builder

	// Header block: time / kind / id.
	fmt.Fprintf(&b, "Time:        %s\n", e.TS.Format("15:04:05.000  (Mon Jan 2 2006)"))
	fmt.Fprintf(&b, "Kind:        %s\n", e.Kind)
	if e.ConnID != "" {
		fmt.Fprintf(&b, "Conn ID:     %s\n", e.ConnID)
	}

	// Process attribution block.
	if e.ProcessName != "" || e.ProcessPath != "" {
		b.WriteString("\n")
		if e.ProcessName != "" && e.ProcessPath != "" {
			fmt.Fprintf(&b, "Process:     %s\n             %s\n", e.ProcessName, e.ProcessPath)
		} else if e.ProcessName != "" {
			fmt.Fprintf(&b, "Process:     %s\n", e.ProcessName)
		} else {
			fmt.Fprintf(&b, "Process:     %s\n", e.ProcessPath)
		}
		if e.Confidence != "" {
			via := ""
			if e.MatchedVia != "" {
				via = "  (via " + e.MatchedVia + ")"
			}
			fmt.Fprintf(&b, "Confidence:  %s%s\n", e.Confidence, via)
		}
	}

	// Destination block.
	if e.Domain != "" || e.IP != "" || e.Port != 0 || e.Network != "" {
		b.WriteString("\n")
		if e.Domain != "" {
			fmt.Fprintf(&b, "Domain:      %s\n", e.Domain)
		}
		if len(e.CnameChain) > 0 {
			// ASCII-стрелка, а не «→» (U+2192): глифа нет в шрифте темы
			// Fyne, и вместо стрелки рисуется «плитка» U+FFFD.
			fmt.Fprintf(&b, "CNAME chain: %s\n", strings.Join(e.CnameChain, "  ->  "))
		}
		if e.IP != "" {
			if e.Port != 0 {
				fmt.Fprintf(&b, "IP:          %s:%d\n", e.IP, e.Port)
			} else {
				fmt.Fprintf(&b, "IP:          %s\n", e.IP)
			}
		} else if e.Port != 0 {
			fmt.Fprintf(&b, "Port:        %d\n", e.Port)
		}
		if e.Network != "" {
			fmt.Fprintf(&b, "Network:     %s\n", e.Network)
		}
	}

	// Routing block.
	// Клиент: на роутере процесса нет, и адрес источника — единственное, что
	// отвечает на вопрос «чьё это соединение».
	if e.SourceAddr != "" {
		fmt.Fprintf(&b, "Client:      %s\n", e.SourceAddr)
	}
	if len(e.OutboundChain) > 0 || e.Rule != "" {
		b.WriteString("\n")
		if len(e.OutboundChain) > 0 {
			// Chain order is leaf→root per types.go. Reverse for display
			// so the user reads root→…→leaf.
			rev := make([]string, len(e.OutboundChain))
			for i, s := range e.OutboundChain {
				rev[len(e.OutboundChain)-1-i] = s
			}
			fmt.Fprintf(&b, "Outbound:    %s\n", strings.Join(rev, "  ->  "))
		}
		// Via — транспортный хвост (detour-цепочка) в порядке следования
		// пакета, отдельной подписанной строкой. Не скобками внутри Outbound:
		// подписанная строка самоописуема и не ломается на глубокой
		// вложенности (нода → группа → нода с detour → …). Звенья, уже
		// показанные в Outbound, пропускаем: ядро кладёт часть обёрток в обе
		// коллекции, и без фильтра строка читалась бы «vless → vless → tls».
		if len(e.DetourChain) > 0 {
			shown := make(map[string]struct{}, len(e.OutboundChain))
			for _, s := range e.OutboundChain {
				shown[s] = struct{}{}
			}
			via := make([]string, 0, len(e.DetourChain))
			for _, s := range e.DetourChain {
				if _, dup := shown[s]; dup || s == "" {
					continue
				}
				via = append(via, s)
			}
			if len(via) > 0 {
				fmt.Fprintf(&b, "Via:         %s\n", strings.Join(via, "  ->  "))
			}
		}
		if e.Rule != "" {
			fmt.Fprintf(&b, "Rule:        %s\n", e.Rule)
		}
	}

	// Traffic counters (close events).
	if e.UpBytes > 0 || e.DownBytes > 0 || e.Duration > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Up bytes:    %s\n", formatBytes(e.UpBytes))
		fmt.Fprintf(&b, "Down bytes:  %s\n", formatBytes(e.DownBytes))
		if e.Duration > 0 {
			fmt.Fprintf(&b, "Duration:    %s\n", e.Duration.Truncate(time.Millisecond))
		}
	}

	// Issues block — usually empty.
	if len(e.Issues) > 0 {
		b.WriteString("\n⚠ Issues:\n")
		for _, iss := range e.Issues {
			if iss.Description != "" {
				fmt.Fprintf(&b, "  • %s — %s\n", iss.Kind, iss.Description)
			} else {
				fmt.Fprintf(&b, "  • %s\n", iss.Kind)
			}
		}
	}

	// Provenance markers.
	if e.Backfilled {
		b.WriteString("\n〽 Backfilled from pre-session rolling buffer.\n")
	}

	// Raw log line — useful for debugging mis-parses, but bulky so we
	// stick it at the bottom and only if non-empty.
	if e.RawLogLine != "" {
		fmt.Fprintf(&b, "\nRaw log line:\n%s\n", e.RawLogLine)
	}

	return b.String()
}
