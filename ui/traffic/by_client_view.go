package traffic

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	tprof "singbox-launcher/internal/traffic"
)

// Вкладка «By client» — разбивка трафика удалённой машины по устройствам сети.
//
// Заменяет «Per-process» из локального окна, а не дополняет его: у роутера
// процессов нет, find_process там смысла не имеет, и предлагать выбрать
// процесс значило бы показывать список процессов ЭТОГО компьютера, к её
// трафику отношения не имеющих. Клиенты различаются исходным адресом —
// единственным, что о них известно.

type byClientView struct {
	Content fyne.CanvasObject

	deps     WindowDeps
	win      fyne.Window
	list     *fyne.Container
	summary  *widget.Label
	expanded map[string]bool
	stopCh   chan struct{}

	// Фильтры разделены по природе данных: перечислимое — списками,
	// исчислимое сотнями (домены, IP, порты) — строкой поиска.
	filter      tprof.ClientFilter
	clientSel   *widget.Select
	outboundSel *widget.Select
	ruleSel     *widget.Select
	search      *widget.Entry
	// optsSig — отпечаток последнего набора вариантов. Списки перезаполняются
	// только при его смене: SetOptions на каждом тике схлопывал бы раскрытый
	// выпадающий список под курсором.
	optsSig string
}

// byClientRefresh — как часто пересчитывается агрегат. Совпадает с тиком
// стрима соединений: чаще нечего показывать, реже — цифры отстают от глаза.
const byClientRefresh = time.Second

// anyOption — пункт «снять фильтр». Пустая строка для этого не годится:
// widget.Select показывает её как пустую строку, неотличимую от «не выбрано».
// Тире по краям отличают его от значений — иначе «все» читается как имя
// клиента или outbound'а.
//
// Функция, а не переменная пакета: язык выставляется в main уже после init'ов,
// и константа поймала бы английский ключ независимо от настройки.
func anyOption() string { return locale.T("traffic.byclient.any") }

// Ширины колонок в клетках сетки. Пропорциональный шрифт не выравнивается
// пробелами, поэтому колонки задаются размерами, а не форматом строки.
//
// Числовые колонки узкие по содержимому: порт — четыре цифры, счётчик
// соединений — редко больше двух, трафик после humanBytes не длиннее «999.9M».
// Всё сэкономленное уходит хосту и правилу — единственным колонкам, где текст
// реально не помещается и режется многоточием.
const (
	colDest     = 330
	colPort     = 44
	colAge      = 46
	colTraffic  = 62
	colOutbound = 110
	colRule     = 250
)

func buildByClientView(deps WindowDeps, win fyne.Window) *byClientView {
	v := &byClientView{
		deps:     deps,
		win:      win,
		list:     container.NewVBox(),
		summary:  widget.NewLabel(""),
		expanded: map[string]bool{},
	}

	v.clientSel = widget.NewSelect(nil, func(s string) {
		v.filter.Client = valueOrEmpty(s)
		v.refresh()
	})
	v.outboundSel = widget.NewSelect(nil, func(s string) {
		v.filter.Outbound = valueOrEmpty(s)
		v.refresh()
	})
	v.ruleSel = widget.NewSelect(nil, func(s string) {
		v.filter.Rule = valueOrEmpty(s)
		v.refresh()
	})
	// Имя фильтра — внутри самого списка, а не подписью над ним: отдельная
	// строка подписей съедала целый ряд высоты, а «all» без неё не говорит,
	// чем именно фильтруем.
	v.clientSel.PlaceHolder = locale.T("traffic.byclient.client")
	v.outboundSel.PlaceHolder = locale.T("traffic.byclient.outbound")
	v.ruleSel.PlaceHolder = locale.T("traffic.byclient.rule")

	v.search = widget.NewEntry()
	v.search.SetPlaceHolder(locale.T("traffic.byclient.search_hint"))
	v.search.OnChanged = func(s string) {
		v.filter.Search = strings.TrimSpace(s)
		v.refresh()
	}

	reset := widget.NewButtonWithIcon(locale.T("traffic.byclient.reset"), theme.ContentClearIcon(), v.reset)
	reset.Importance = widget.LowImportance

	filters := container.NewGridWithColumns(3, v.clientSel, v.outboundSel, v.ruleSel)
	searchRow := container.NewBorder(nil, nil,
		widget.NewLabel(locale.T("traffic.byclient.search")),
		container.NewHBox(v.summary, reset),
		v.search,
	)

	v.Content = container.NewBorder(
		container.NewVBox(filters, searchRow, v.headerRow(), widget.NewSeparator()),
		nil, nil, nil,
		container.NewVScroll(v.list),
	)

	v.refresh()
	v.startTimer()
	return v
}

func valueOrEmpty(s string) string {
	if s == anyOption() {
		return ""
	}
	return optionValue(s)
}

func (v *byClientView) reset() {
	v.filter = tprof.ClientFilter{}
	// Выбор снимаем без вызова OnChanged: три обработчика подряд означали бы
	// три полных пересчёта таблицы на одно нажатие.
	v.clientSel.Selected = ""
	v.outboundSel.Selected = ""
	v.ruleSel.Selected = ""
	v.clientSel.Refresh()
	v.outboundSel.Refresh()
	v.ruleSel.Refresh()
	v.search.OnChanged = nil
	v.search.SetText("")
	v.search.OnChanged = func(s string) {
		v.filter.Search = strings.TrimSpace(s)
		v.refresh()
	}
	v.refresh()
}

// startTimer держит агрегат живым: данные приходят стримом, и без
// периодического пересчёта таблица застыла бы на первом кадре.
func (v *byClientView) startTimer() {
	v.stopCh = make(chan struct{})
	go func() {
		t := time.NewTicker(byClientRefresh)
		defer t.Stop()
		for {
			select {
			case <-v.stopCh:
				return
			case <-t.C:
				fyne.Do(v.refresh)
			}
		}
	}()
}

func (v *byClientView) refresh() {
	v.syncOptions()

	clients := v.deps.Profiler.ClientSummaries(v.filter)
	shown, total := v.deps.Profiler.ClientTotals(v.filter)
	v.list.RemoveAll()

	if shown == total {
		v.summary.SetText(fmt.Sprintf("%d conns", total))
	} else {
		v.summary.SetText(fmt.Sprintf("%d / %d conns", shown, total))
	}

	if len(clients) == 0 {
		// Пусто — это либо «ядро молчит», либо «фильтр никого не оставил».
		// Разница существенная: во втором случае искать причину в машине не
		// надо, и сказать об этом словами честнее, чем оставить голое место.
		key := "traffic.byclient.empty"
		if total > 0 {
			key = "traffic.byclient.empty_filtered"
		}
		hint := widget.NewLabel(locale.T(key))
		hint.Wrapping = fyne.TextWrapWord
		v.list.Add(hint)
		v.list.Refresh()
		return
	}

	for _, c := range clients {
		v.list.Add(v.clientRow(c))
	}
	v.list.Refresh()
}

// syncOptions обновляет содержимое списков по живому потоку.
func (v *byClientView) syncOptions() {
	opts := v.deps.Profiler.ClientOptions()
	sig := optionsSignature(opts)
	if sig == v.optsSig {
		return
	}
	v.optsSig = sig
	setOptions(v.clientSel, opts.Clients, v.filter.Client)
	setOptions(v.outboundSel, opts.Outbounds, v.filter.Outbound)
	setOptions(v.ruleSel, opts.Rules, v.filter.Rule)
}

// setOptions заполняет список подписями вида «proxy-out (12)».
//
// Счётчик — часть подписи, а не отдельная колонка: widget.Select показывает
// строки как есть. Обратно в фильтр значение достаётся срезом по последнему
// « (» — см. valueOrEmpty.
//
// Выбранное значение сохраняется, даже если оно ушло из потока: соединения по
// нему могли закрыться, и молча сбросить фильтр значило бы показать чужие
// данные под прежней подписью.
func setOptions(sel *widget.Select, opts []tprof.OptionCount, selected string) {
	values := make([]string, 0, len(opts)+2)
	values = append(values, anyOption())
	present := false
	for _, o := range opts {
		values = append(values, fmt.Sprintf("%s (%d)", o.Value, o.Count))
		if o.Value == selected {
			present = true
		}
	}
	if selected != "" && !present {
		values = append(values, selected)
	}
	sel.Options = values

	// Подпись выбранного меняется вместе со счётчиком, поэтому её надо
	// переставить: иначе Select показывал бы «proxy-out (12)» на давно
	// изменившемся числе.
	if selected != "" {
		for _, val := range values {
			if optionValue(val) == selected {
				sel.Selected = val
				break
			}
		}
	}
	sel.Refresh()
}

// optionValue снимает счётчик с подписи варианта.
//
// Разбор идёт с конца и требует обеих скобок: строки правил приходят из ядра
// как есть и сами содержат скобки (`rule_set=[geosite] (dns)`), поэтому
// отрезать по ПЕРВОМУ « (» значило бы искалечить значение фильтра.
func optionValue(s string) string {
	if !strings.HasSuffix(s, ")") {
		return s
	}
	i := strings.LastIndex(s, " (")
	if i <= 0 {
		return s
	}
	// Между скобками должно быть только число — иначе это часть значения.
	for _, r := range s[i+2 : len(s)-1] {
		if r < '0' || r > '9' {
			return s
		}
	}
	return s[:i]
}

func optionsSignature(o tprof.ClientOptions) string {
	var b strings.Builder
	for _, g := range [][]tprof.OptionCount{o.Clients, o.Outbounds, o.Rules} {
		for _, c := range g {
			b.WriteString(c.Value)
			b.WriteByte('\x00')
		}
		b.WriteByte('\x01')
	}
	return b.String()
}

// headerRow — шапка таблицы. Ширины те же, что у строк соединений, поэтому
// колонки стоят друг под другом при любом шрифте.
//
// Узкие колонки подписаны значками, а не словами: жирные PORT и CONNS шире
// своих 44 и 46 пунктов и налезали друг на друга. Значок в одну букву влезает
// с запасом, а что он значит, видно по самим числам под ним.
func (v *byClientView) headerRow() fyne.CanvasObject {
	cell := func(text string, w float32, align fyne.TextAlign) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(text, align, fyne.TextStyle{Bold: true})
		l.Truncation = fyne.TextTruncateOff
		return fixedWidth(l, w)
	}
	// Значки вместо слов: «:» — порт, «~» — сколько соединение живёт.
	// Не эмодзи: «⏱» шрифт темы рисует цветным глифом, выбивающимся из
	// строки подписей.
	return container.NewHBox(
		cell(locale.T("traffic.byclient.col_dest"), colDest, fyne.TextAlignLeading),
		cell(":", colPort, fyne.TextAlignTrailing),
		cell("~", colAge, fyne.TextAlignTrailing),
		cell("↑", colTraffic, fyne.TextAlignTrailing),
		cell("↓", colTraffic, fyne.TextAlignTrailing),
		cell(locale.T("traffic.byclient.col_outbound"), colOutbound, fyne.TextAlignLeading),
		cell(locale.T("traffic.byclient.col_rule"), colRule, fyne.TextAlignLeading),
	)
}

func (v *byClientView) clientRow(c tprof.ClientSummary) fyne.CanvasObject {
	open := v.expanded[c.Addr]
	arrow := "▸"
	if open {
		arrow = "▾"
	}

	// Имя устройства в скобках после адреса. Именно так, а не вместо адреса:
	// ключ группы, фильтра и правил маршрутизации — всё равно IP, и подменять
	// его на имя значило бы показывать не то, по чему идёт отбор.
	title := fmt.Sprintf("%s  %s", arrow, c.Addr)
	if dev, ok := v.device(c.Addr); ok && dev.Name != "" {
		title += "  (" + dev.Name + ")"
	}
	if c.MixedOutbound {
		// Ради этого признака профайлер на роутере и открывают: устройство
		// ушло мимо VPN частью трафика, а не целиком.
		title += "  " + locale.T("traffic.byclient.mixed")
	}
	head := widget.NewButton(title, func() {
		v.expanded[c.Addr] = !v.expanded[c.Addr]
		v.refresh()
	})
	head.Alignment = widget.ButtonAlignLeading
	head.Importance = widget.LowImportance

	totals := container.NewHBox(
		// Счётчик соединений осмыслен на заголовке группы: это размер самой
		// группы, а не свойство строки-соединения под ней.
		fixedWidth(widget.NewLabelWithStyle(fmt.Sprintf("%d", c.Conns), fyne.TextAlignTrailing, fyne.TextStyle{}), colAge),
		fixedWidth(widget.NewLabelWithStyle("↑"+humanBytes(c.Up), fyne.TextAlignTrailing, fyne.TextStyle{}), colTraffic),
		fixedWidth(widget.NewLabelWithStyle("↓"+humanBytes(c.Down), fyne.TextAlignTrailing, fyne.TextStyle{}), colTraffic),
	)
	// Кнопка растянута на всю строку, числа лежат ПОВЕРХ неё: рядом с кнопкой
	// они были бы мёртвой зоной, и клик по правой половине строки — там, где
	// как раз и смотрят на трафик, — ничего не делал бы.
	headRow := container.NewStack(head, container.NewBorder(nil, nil, nil, totals))
	rows := []fyne.CanvasObject{headRow}
	if !open {
		return container.NewVBox(rows...)
	}

	// Раскрыто: назначения клиента, каким outbound'ом ушло каждое и какое
	// правило этот outbound выбрало.
	for _, item := range c.Items {
		rows = append(rows, v.connRow(item))
	}

	// Обрыв всех соединений клиента: после смены правила устройство иначе
	// доживает на старых сессиях и продолжает идти прежним маршрутом.
	if v.deps.CloseConns != nil {
		ids := append([]string(nil), c.IDs...)
		addr := c.Addr
		closeBtn := widget.NewButtonWithIcon(locale.T("traffic.byclient.close_conns"), theme.CancelIcon(), func() {
			v.deps.CloseConns(ids)
			v.expanded[addr] = false
			v.refresh()
		})
		closeBtn.Importance = widget.LowImportance
		rows = append(rows, container.NewBorder(nil, nil, nil, closeBtn))
	}

	rows = append(rows, widget.NewSeparator())
	return container.NewVBox(rows...)
}

// connRow — одна строка таблицы, то есть одно соединение.
func (v *byClientView) connRow(c tprof.ClientConn) fyne.CanvasObject {
	// Truncate вместо ручной обрезки по числу символов: шрифт
	// пропорциональный, и «rr12---sn-axq7sn76» с «WWWWWWWWWWWWWWWWWW» одной
	// длины занимают разную ширину — считать символы значит резать наугад.
	cell := func(text string, w float32, align fyne.TextAlign) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(text, align, fyne.TextStyle{})
		l.Truncation = fyne.TextTruncateEllipsis
		return fixedWidth(l, w)
	}
	host := cell(c.Host, colDest-destIndent, fyne.TextAlignLeading)
	cells := container.NewHBox(
		// Отступ вложенной строки — настоящий, а не пробелы в тексте: пробелы
		// съедали бы ширину самого хоста и сдвигали бы точку обрезки.
		container.NewHBox(fixedWidth(widget.NewLabel(""), destIndent), host),
		cell(c.Port, colPort, fyne.TextAlignTrailing),
		cell(connAge(c), colAge, fyne.TextAlignTrailing),
		cell(humanBytes(c.Up), colTraffic, fyne.TextAlignTrailing),
		cell(humanBytes(c.Down), colTraffic, fyne.TextAlignTrailing),
		cell(c.Outbound(), colOutbound, fyne.TextAlignLeading),
		cell(shortRule(c.Rule), colRule, fyne.TextAlignLeading),
	)

	// Строка целиком кликабельна и открывает детали — так же, как строка в
	// Live: колонки хоста и правила обрезаны по ширине, и добраться до полного
	// значения иначе нечем.
	btn := widget.NewButton("", func() {
		v.showConnDetail(c)
	})
	btn.Importance = widget.LowImportance
	return container.NewStack(btn, cells)
}

// connAge — сколько соединение живёт. Длительность отвечает на вопрос
// «висит или только что открылось», которого в таблице иначе нет.
func connAge(c tprof.ClientConn) string {
	if c.Start.IsZero() {
		return ""
	}
	d := time.Since(c.Start)
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// device — запись справочника по адресу клиента.
func (v *byClientView) device(addr string) (DeviceInfo, bool) {
	d := deviceFor(v.deps, addr)
	if d == nil {
		return DeviceInfo{}, false
	}
	return *d, true
}

// showConnDetail показывает соединение тем же форматом, что и Live.
//
// Общий formatEventDetail, а не своя разметка: строка здесь — одно соединение,
// ровно как строка в Live, и два разных вида одного и того же только заставили
// бы сверять поля глазами.
func (v *byClientView) showConnDetail(c tprof.ClientConn) {
	if v.win == nil {
		return
	}
	port := 0
	if c.Port != "" {
		port, _ = strconv.Atoi(c.Port)
	}
	e := tprof.TrafficEvent{
		TS:            c.Start,
		Kind:          tprof.EventTCPOpen,
		ConnID:        c.ID,
		SourceAddr:    c.Source,
		Port:          port,
		Network:       c.Network,
		OutboundChain: c.Chains,
		DetourChain:   c.Detour,
		Rule:          c.Rule,
		UpBytes:       c.Up,
		DownBytes:     c.Down,
	}
	// Хост может быть доменом или голым IP — Live различает эти поля, и
	// класть адрес в Domain значило бы показать «Domain: 149.154.167.255».
	if net.ParseIP(c.Host) != nil {
		e.IP = c.Host
	} else {
		e.Domain = c.Host
	}
	if !c.Start.IsZero() {
		e.Duration = time.Since(c.Start)
	}
	// Карточка устройства рядом с соединением: адрес отвечает «кто», имя,
	// SSID и порт коммутатора — «что это за коробка и как она подключена».
	showEventDetailWithDevice(v.win, e, deviceFor(v.deps, c.Source))
}

// shortRule снимает с правила служебную обёртку, оставляя то, что различает
// строки.
//
// Ядро отдаёт правило как `rule_set=[russian:ru-domains rus…]`, и первые
// десять символов у всех строк одинаковы — на узкой колонке из-за них
// многоточие съедало ровно ту часть, ради которой колонку и читают. Имя
// критерия сохраняем: domain_suffix и rule_set — разные способы попасть в
// outbound, и путать их нельзя.
func shortRule(s string) string {
	open := strings.IndexByte(s, '[')
	if open < 0 || !strings.HasSuffix(s, "]") {
		return s
	}
	kind := strings.TrimSuffix(s[:open], "=")
	inner := s[open+1 : len(s)-1]
	if kind == "" || inner == "" {
		return s
	}
	return kind + ": " + inner
}

// destIndent — сдвиг строки назначения под её клиентом.
const destIndent = 24

// fixedWidthLayout ставит объекту заданную ширину, оставляя высоту по
// содержимому. Без него Label растягивается по своему тексту, и колонки
// разъезжаются от строки к строке.
type fixedWidthLayout struct{ w float32 }

func (l fixedWidthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objs {
		if s := o.MinSize().Height; s > h {
			h = s
		}
	}
	return fyne.NewSize(l.w, h)
}

func (l fixedWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(l.w, size.Height))
	}
}

func fixedWidth(o fyne.CanvasObject, w float32) fyne.CanvasObject {
	return container.New(fixedWidthLayout{w: w}, o)
}

// humanBytes — компактная форма объёма («1.2M») для узких колонок таблицы
// By client. Отдельно от formatBytes намеренно: там полная форма («1.2 MB»),
// которая в колонку фиксированной ширины не влезает.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fK", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// Stop гасит таймер пересчёта: окно закрыто — считать агрегат не для кого.
func (v *byClientView) Stop() {
	if v.stopCh != nil {
		close(v.stopCh)
		v.stopCh = nil
	}
}
