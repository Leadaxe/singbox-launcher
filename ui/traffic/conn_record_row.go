package traffic

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
)

// Строка подвкладки Connections в записи сессии — колонками, как «By client»
// у машины.
//
// Ширины и правила обрезки берутся оттуда же (colDest, colPort, …): это одна и
// та же таблица соединений, показанная в двух окнах, и расходиться им незачем.
// Отличие одно — колонка состояния: у записи сессии соединения бывают уже
// закрытыми, а в живом обзоре машины закрытых нет вовсе.

// colState — ширина колонки состояния. По длине «closed» плюс место под «⚠».
const colState = 62

// connRecordRow — переиспользуемая строка widget.List. Fyne создаёт виджет
// один раз на видимую строку и переиспользует его при прокрутке, поэтому
// ячейки собираются в конструкторе, а set() только меняет тексты.
type connRecordRow struct {
	widget.BaseWidget

	state    *widget.Label
	dest     *widget.Label
	port     *widget.Label
	age      *widget.Label
	up       *widget.Label
	down     *widget.Label
	outbound *widget.Label
	rule     *widget.Label

	content fyne.CanvasObject
}

func newConnRecordRow() *connRecordRow {
	cell := func(align fyne.TextAlign, truncate bool) *widget.Label {
		l := widget.NewLabelWithStyle("", align, fyne.TextStyle{})
		if truncate {
			l.Truncation = fyne.TextTruncateEllipsis
		} else {
			l.Truncation = fyne.TextTruncateOff
		}
		return l
	}
	r := &connRecordRow{
		state:    cell(fyne.TextAlignLeading, false),
		dest:     cell(fyne.TextAlignLeading, true),
		port:     cell(fyne.TextAlignTrailing, false),
		age:      cell(fyne.TextAlignTrailing, false),
		up:       cell(fyne.TextAlignTrailing, false),
		down:     cell(fyne.TextAlignTrailing, false),
		outbound: cell(fyne.TextAlignLeading, true),
		rule:     cell(fyne.TextAlignLeading, true),
	}
	r.content = container.NewHBox(
		fixedWidth(r.state, colState),
		fixedWidth(r.dest, colDest-colState),
		fixedWidth(r.port, colPort),
		fixedWidth(r.age, colAge),
		fixedWidth(r.up, colTraffic),
		fixedWidth(r.down, colTraffic),
		fixedWidth(r.outbound, colOutbound),
		fixedWidth(r.rule, colRule),
	)
	r.ExtendBaseWidget(r)
	return r
}

func (r *connRecordRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.content)
}

// set перекладывает запись в ячейки.
func (r *connRecordRow) set(d tprof.ConnRecord) {
	state := locale.T("open")
	if d.ClosedAt != nil {
		state = locale.T("closed")
	}
	if len(d.Issues) > 0 {
		// Значок в колонке состояния, а не приклеенный к хосту: иначе он
		// сдвигал бы текст назначения и ломал бы столбец.
		state = "⚠ " + state
	}
	r.state.SetText(state)

	// Домен, а при его отсутствии — адрес: соединение по голому IP домена не
	// имеет вовсе, и пустая колонка не сказала бы, куда оно шло.
	dest := d.Domain
	if dest == "" {
		dest = d.IP
	}
	r.dest.SetText(dest)
	r.port.SetText(strconvItoa(d.Port))
	r.age.SetText(connRecordAge(d))
	r.up.SetText(humanBytes(d.UpBytes))
	r.down.SetText(humanBytes(d.DownBytes))
	r.outbound.SetText(lastOutbound(d.Outbounds))
	r.rule.SetText(shortRule(d.Rule))
}

// showConnRecordDetail открывает карточку соединения записи — тот же диалог,
// что у строки Live и у соединения машины.
//
// Запись перекладывается в TrafficEvent, потому что карточка умеет показывать
// именно события: заводить ей второй формат ради тех же полей значило бы
// держать две вёрстки одного и того же.
func showConnRecordDetail(win fyne.Window, d tprof.ConnRecord) {
	if win == nil {
		return
	}
	e := tprof.TrafficEvent{
		TS:            d.OpenedAt,
		Kind:          tprof.EventTCPOpen,
		ConnID:        d.ConnID,
		Domain:        d.Domain,
		IP:            d.IP,
		Port:          d.Port,
		Network:       d.Network,
		OutboundChain: d.Outbounds,
		Rule:          d.Rule,
		UpBytes:       d.UpBytes,
		DownBytes:     d.DownBytes,
		Issues:        d.Issues,
	}
	// Закрытое соединение — событие закрытия: у него есть длительность, и
	// показывать его как «открыто» значило бы врать о состоянии.
	if d.ClosedAt != nil {
		e.Kind = tprof.EventTCPClose
		if !d.OpenedAt.IsZero() {
			e.Duration = d.ClosedAt.Sub(d.OpenedAt)
		}
	} else if !d.OpenedAt.IsZero() {
		e.Duration = time.Since(d.OpenedAt)
	}
	showEventDetail(win, e)
}

// connRecordRowHeader — шапка таблицы соединений. Значки те же, что у «By
// client»: «:» — порт, «~» — сколько соединение прожило.
func connRecordRowHeader() fyne.CanvasObject {
	cell := func(text string, w float32, align fyne.TextAlign) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(text, align, fyne.TextStyle{Bold: true})
		l.Truncation = fyne.TextTruncateOff
		return fixedWidth(l, w)
	}
	return container.NewHBox(
		cell("", colState, fyne.TextAlignLeading),
		cell(locale.T("DESTINATION"), colDest-colState, fyne.TextAlignLeading),
		cell(":", colPort, fyne.TextAlignTrailing),
		cell("~", colAge, fyne.TextAlignTrailing),
		cell("↑", colTraffic, fyne.TextAlignTrailing),
		cell("↓", colTraffic, fyne.TextAlignTrailing),
		cell(locale.T("OUTBOUND"), colOutbound, fyne.TextAlignLeading),
		cell(locale.T("RULE"), colRule, fyne.TextAlignLeading),
	)
}

// connRecordAge — сколько соединение прожило. Для закрытого это длительность
// от открытия до закрытия, а не «сколько прошло с тех пор»: запись сессии
// смотрят после остановки, и время с закрытия росло бы у всех строк разом,
// ничего не сообщая.
func connRecordAge(d tprof.ConnRecord) string {
	if d.OpenedAt.IsZero() {
		return ""
	}
	end := time.Now()
	if d.ClosedAt != nil {
		end = *d.ClosedAt
	}
	return shortDuration(end.Sub(d.OpenedAt))
}

func shortDuration(d time.Duration) string {
	switch {
	case d < 0:
		return ""
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// lastOutbound — корень цепочки, то есть выбранный outbound. Так же, как
// ClientConn.Outbound() у машины.
func lastOutbound(chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	return chain[len(chain)-1]
}

func strconvItoa(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}
