package traffic

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	tprof "singbox-launcher/internal/traffic"
)

// Карточки строк вкладок Domains и IPs.
//
// Строка там — не одно соединение, а свод: у домена это все его соединения
// сразу (в списке видно «2 conn, 1 IPs»). Поэтому карточка показывает не поля
// события, а то, что в свод не поместилось: полная цепочка CNAME, все адреса
// домена, все outbound'ы и найденные проблемы. Обрезанный многоточием список
// адресов иначе не раскрыть ничем.

// showDomainDetail — карточка строки вкладки Domains.
func showDomainDetail(parent fyne.Window, d tprof.DomainStats) {
	if parent == nil {
		return
	}
	showAggregateDetail(parent, d.Domain, formatDomainDetail(d))
}

// showIPDetail — карточка строки вкладки IPs.
func showIPDetail(parent fyne.Window, s tprof.IPStats) {
	if parent == nil {
		return
	}
	title := s.IP
	if s.Port > 0 {
		title = fmt.Sprintf("%s:%d", s.IP, s.Port)
	}
	showAggregateDetail(parent, title, formatIPDetail(s))
}

// showAggregateDetail — общая обёртка: моноширинный текст в прокрутке, как у
// карточки события. Заголовок несёт сам домен или адрес: «Detail» без него
// не отличить одну карточку от другой, когда открыли несколько подряд.
func showAggregateDetail(parent fyne.Window, title, text string) {
	body := widget.NewLabel(text)
	body.Wrapping = fyne.TextWrapWord
	body.TextStyle = fyne.TextStyle{Monospace: true}
	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(560, 320))
	d := dialog.NewCustom(title, "Close", scroll, parent)
	d.Resize(fyne.NewSize(620, 400))
	d.Show()
}

func formatDomainDetail(d tprof.DomainStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Domain:      %s\n", d.Domain)
	fmt.Fprintf(&b, "Connections: %d\n", d.Connections)
	fmt.Fprintf(&b, "Traffic:     up %s   down %s\n", formatBytes(d.UpBytes), formatBytes(d.DownBytes))

	// Окно наблюдения: у долгоживущего домена оно отвечает на вопрос «это
	// разовый заход или он идёт всю сессию».
	if !d.FirstSeen.IsZero() {
		fmt.Fprintf(&b, "First seen:  %s\n", d.FirstSeen.Format("15:04:05"))
	}
	if !d.LastSeen.IsZero() {
		fmt.Fprintf(&b, "Last seen:   %s\n", d.LastSeen.Format("15:04:05"))
	}

	// Цепочка переадресаций целиком — ради неё карточку и открывают: в строке
	// её нет вовсе, а именно она отвечает, куда домен ушёл на самом деле.
	if len(d.CnameChain) > 0 {
		b.WriteString("\nCNAME chain:\n")
		for i, c := range d.CnameChain {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
		}
	}

	// Адреса перечисляем по одному в строке: в списке они сжаты до счётчика
	// «1 IPs», и раскрыть их иначе нечем.
	if len(d.IPs) > 0 {
		b.WriteString("\nResolved to:\n")
		for _, ip := range d.IPs {
			fmt.Fprintf(&b, "  %s\n", ip)
		}
	}
	if len(d.Outbounds) > 0 {
		fmt.Fprintf(&b, "\nOutbounds:   %s\n", strings.Join(d.Outbounds, ", "))
	}
	b.WriteString(formatIssues(d.Issues))
	return b.String()
}

func formatIPDetail(s tprof.IPStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "IP:          %s\n", s.IP)
	if s.Port > 0 {
		fmt.Fprintf(&b, "Port:        %d\n", s.Port)
	}
	fmt.Fprintf(&b, "Connections: %d\n", s.Connections)
	fmt.Fprintf(&b, "Traffic:     up %s   down %s\n", formatBytes(s.UpBytes), formatBytes(s.DownBytes))
	if s.Domain != "" {
		// Домен, который в этот адрес разрешился. Для соединения по голому IP
		// его нет вовсе — тогда строку не печатаем, а не пишем «(none)»:
		// пустое место честнее выдуманного значения.
		fmt.Fprintf(&b, "Domain:      %s\n", s.Domain)
	}
	if len(s.Outbounds) > 0 {
		fmt.Fprintf(&b, "\nOutbounds:   %s\n", strings.Join(s.Outbounds, ", "))
	}
	return b.String()
}

// formatIssues — блок найденных проблем. Пустой список даёт пустую строку:
// заголовок без содержимого читался бы как «проблемы есть, но какие — не
// скажем».
func formatIssues(issues []tprof.ConnectionIssue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nIssues:\n")
	for _, is := range issues {
		if is.Description != "" {
			fmt.Fprintf(&b, "  ⚠ %s — %s\n", is.Kind, is.Description)
		} else {
			fmt.Fprintf(&b, "  ⚠ %s\n", is.Kind)
		}
	}
	return b.String()
}
