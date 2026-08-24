package ui

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/ui/components"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	hostUnsupportedText = "This daemon has no host telemetry: the machine is reachable, but its sing-box build predates /admin/host. Update the daemon on the machine."
)

// Окно телеметрии ХОСТА машины (SPEC 068 форка, §10b документации демона).
//
// Зачем отдельно от профайлера: тот показывает трафик ЯДРА, а вопрос «почему
// роутер тормозит» на нём не решается. Ядро может быть в полном порядке, пока
// overlay забит под завязку, одно ядро CPU стоит в полке, а плата уходит в
// термотротлинг. Здесь описана машина, а не процесс.
//
// Окно на машину — по образцу профайлера: две машины должны открываться рядом,
// именно ради сравнения такие вещи и смотрят.

// hostWindowPollInterval — период опроса.
//
// Он же задаёт окно усреднения: демон считает проценты и скорости как дельту
// между двумя СОСЕДНИМИ запросами, поэтому частота опроса здесь — не косметика,
// а выбор того, что именно показано. Полсекунды дают живую картину, в которой
// видно короткий всплеск нагрузки: на двух секундах он размазывался в ровный
// фон и как раз пропадал тот пик, ради которого окно и открывают.
//
// Цена — вчетверо больше запросов к роутеру. Она приемлема: два GET'а
// возвращают готовый снимок из кэша демона, и опрос живёт только пока окно
// открыто (тикер гасится в SetCloseIntercept).
const hostWindowPollInterval = 500 * time.Millisecond

// Ширины колонок таблиц.
//
// Фиксированные, а не по содержимому: колонка по содержимому «дышит» от тика к
// тику (12.1% и 7.4% — разной ширины), и числа перестают стоять друг под
// другом ровно в тот момент, когда их сравнивают глазами.
// Ширины подобраны под самое длинное реальное значение своей колонки, а не с
// запасом: сумма всех колонок задаёт минимальную ширину окна, и лишние 20 px
// в каждой выталкивают правый край за экран.
const (
	hostColBar     = 84  // полоска заполнения
	hostColPercent = 70  // «100.0%» плюс зазор от полоски слева
	hostColBytes   = 78  // «213.4 MB»
	hostColFS      = 84  // «squashfs» — под самое длинное имя ФС целиком
	hostColFlags   = 132 // «[🔒 ro · ★ state]»
	hostColRate    = 72  // «229 B/s»
	hostColErrors  = 84  // «120 / 52210»
	hostColMTU     = 48  // «65536»
	hostColCore    = 20  // номер ядра
	// Имя — тоже фиксированное. Растягивающийся центр Border забирал всю
	// свободную ширину, из-за чего строка всегда была шире окна и таблицу
	// приходилось листать вбок.
	hostColName = 150 // «/tmp/run/netns», «● phy0-ap0»

	// Ширины блоков верхнего ряда. Сумма (2×250 + 190 ≈ 690) держится ниже
	// ширины таблицы интерфейсов, чтобы окно задавала таблица, а не шапка.
	hostBlockWide = 250 // CPU, память
	hostBlockSide = 190 // температура + дескрипторы

	// hostBarHeight — высота полоски: пятая часть штатной (текст 14 + два
	// отступа по 4 ≈ 22). Полоска здесь отмечает уровень рядом с числом, а
	// не заменяет его, и в полную высоту перетягивала внимание на себя.
	hostBarHeight = 4

	// hostWindowWidth — ширина окна по самой широкой строке, а не «с запасом».
	//
	// Самая широкая — таблица интерфейсов: семь колонок ниже плюс отступы
	// HBox между ними и поля прокрутки. Считается из тех же констант, что и
	// сами колонки, поэтому не разъезжается при их правке: подобранное на
	// глаз число пережило бы ровно одну правку ширины колонки.
	hostWindowWidth = hostColName + 2*hostColRate + 2*hostColBytes +
		hostColErrors + hostColMTU + hostWindowChrome

	// hostWindowChrome — отступы между семью ячейками, поля Padded-контейнера
	// и полоса прокрутки справа.
	hostWindowChrome = 64

	// hostIfacesTitleWidth — ширина заголовка «Интерфейсы (12 из 19)».
	// Задана явно, потому что в голом HBox выпадающий список фильтра садился
	// поверх текста заголовка.
	hostIfacesTitleWidth = 190
)

// machineHostWindow — одно открытое окно телеметрии.
type machineHostWindow struct {
	win  fyne.Window
	stop func()
}

var (
	machineHostWindowsMu sync.Mutex
	// machineHostWindows — по одному окну на машину: повторное открытие
	// фокусирует существующее, а не заводит второй опрос.
	machineHostWindows = map[string]*machineHostWindow{}
)

// OpenMachineHostWindow открывает окно телеметрии хоста машины.
//
// Требует живого соединения — как и профайлер: данные приезжают по её же
// каналу, и без транспорта смотреть нечего.
func OpenMachineHostWindow(ac *core.AppController, d services.RemoteDaemon) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}

	machineHostWindowsMu.Lock()
	if w, ok := machineHostWindows[d.ID]; ok {
		machineHostWindowsMu.Unlock()
		w.win.Show()
		w.win.RequestFocus()
		return
	}
	machineHostWindowsMu.Unlock()

	transport, ok := lxdOverrideTransportForID(d.ID)
	if !ok {
		ShowErrorText(ac.UIService.MainWindow, d.Name, locale.T("Connect to the machine first: host telemetry is read over its control channel."))
		return
	}

	win := ac.UIService.Application.NewWindow(locale.Tf("%s — host", d.Name))
	view := newHostView()
	view.wantOS, view.wantArch = d.GOOS, d.GOARCH

	// Поллинг живёт в своей горутине и гаснет вместе с окном: без остановки
	// он продолжал бы ходить к роутеру после закрытия.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopCh) }) }

	go func() {
		// Первый заход сразу, дальше по тику: ждать целый период перед самым
		// первым показом незачем, у окна и так пустая шапка.
		//
		// Опрос синхронный, и это намеренно: тик приходит только после того,
		// как предыдущий ответ разобран, а лишние тики Ticker отбрасывает.
		// На медленном канале опрос сам станет реже вместо того, чтобы
		// копить очередь запросов к роутеру.
		poll := func() {
			info, infoErr := transport.HostInfo()
			ifaces, ifErr := transport.HostInterfaces()
			fyne.Do(func() { view.update(info, infoErr, ifaces, ifErr) })
		}
		poll()
		t := time.NewTicker(hostWindowPollInterval)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				poll()
			}
		}
	}()

	win.SetContent(container.NewPadded(view.content()))
	win.Resize(fyne.NewSize(hostWindowWidth, 700))
	win.CenterOnScreen()
	win.SetCloseIntercept(func() {
		stop()
		machineHostWindowsMu.Lock()
		delete(machineHostWindows, d.ID)
		machineHostWindowsMu.Unlock()
		win.Close()
	})

	machineHostWindowsMu.Lock()
	machineHostWindows[d.ID] = &machineHostWindow{win: win, stop: stop}
	machineHostWindowsMu.Unlock()

	win.Show()
}

// CloseMachineHostWindow гасит окно телеметрии машины. Зовётся при Disconnect
// и удалении машины: без этого опрос жил бы к машине, с которой разговор уже
// не идёт.
func CloseMachineHostWindow(id string) {
	machineHostWindowsMu.Lock()
	w, ok := machineHostWindows[id]
	delete(machineHostWindows, id)
	machineHostWindowsMu.Unlock()
	if !ok {
		return
	}
	if w.stop != nil {
		w.stop()
	}
	fyne.Do(func() { w.win.Close() })
}

// hostView — виджеты окна и их обновление.
//
// Собран один раз, дальше только меняются тексты: пересборка дерева на каждом
// тике сбрасывала бы позицию скролла — раз в две секунды это делает список
// интерфейсов нечитаемым.
type hostView struct {
	// Шапка машины — две строки: чем машина является (модель, арх, аптайм по
	// правому краю) и что на ней стоит (прошивка, ядро).
	machine   *widget.Label
	machineUp *widget.Label
	machineSw *widget.Label
	// archWarn — расхождение записанной платформы машины с настоящей.
	archWarn *widget.Label
	// wantOS/wantArch — платформа, выбранная оператором при добавлении машины.
	// Под неё собирается config.json (SPEC 098 §2.4), поэтому ошибка здесь
	// не косметическая: конфиг уедет на машину, для которой он не собран.
	wantOS   string
	wantArch string
	errBar   *widget.Label

	cpuTitle    *widget.Label
	cpuBar      *widget.ProgressBar
	cpuCores    *fyne.Container
	cpuLoad     *widget.Label
	cpuInterval *widget.Label

	memTitle  *widget.Label
	memBar    *widget.ProgressBar
	memDetail *widget.Label
	memSwap   *widget.Label

	thermal     *widget.Label
	thermalList *widget.Label

	fdDaemon *widget.Label
	fdSystem *widget.Label

	disksTitle *widget.Label
	disks      *fyne.Container
	disksHint  *widget.Label

	ifacesTitle *widget.Label
	ifaces      *fyne.Container
	ifacesHint  *widget.Label

	// Фильтр интерфейсов. На роутере их два десятка, и почти все — мосты,
	// туннели и пустые lanN; без отсечки живые тонут среди нулей.
	//
	// Отдельной галочки на lo нет: петля — такой же интерфейс со своими
	// счётчиками, и прятать её нужно не чаще, чем любой другой. Режимы ниже
	// убирают её на общих основаниях, когда она под них не подходит.
	ifacesFilter *widget.Select
	// last — последний ответ, чтобы перерисовать по смене фильтра, не дожидаясь
	// следующего опроса: фильтр обязан отвечать мгновенно.
	lastIfaces lxdclient.HostInterfaces
	hasIfaces  bool
}

// Режимы фильтра интерфейсов.
const (
	hostIfaceAll     = "all"
	hostIfaceUp      = "up"
	hostIfaceTraffic = "traffic"
)

func newHostView() *hostView {
	v := &hostView{
		machine:     hostBoldLabel(locale.T("Reading host telemetry…")),
		machineUp:   widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{}),
		machineSw:   hostDimLabel(""),
		archWarn:    widget.NewLabel(""),
		errBar:      widget.NewLabel(""),
		cpuTitle:    hostBoldLabel(""),
		cpuBar:      hostBar(),
		cpuCores:    container.NewVBox(),
		cpuLoad:     widget.NewLabel(""),
		cpuInterval: hostDimLabel(""),
		memTitle:    hostBoldLabel(""),
		memBar:      hostBar(),
		memDetail:   widget.NewLabel(""),
		memSwap:     widget.NewLabel(""),
		thermal:     hostBoldLabel(""),
		thermalList: widget.NewLabel(""),
		fdDaemon:    widget.NewLabel(""),
		fdSystem:    widget.NewLabel(""),
		disksTitle:  hostBoldLabel(""),
		disks:       container.NewVBox(),
		disksHint:   hostDimLabel(""),
		ifacesTitle: hostBoldLabel(""),
		ifaces:      container.NewVBox(),
		ifacesHint:  hostDimLabel(""),
	}
	v.errBar.Wrapping = fyne.TextWrapWord
	// Шапка машины НЕ переносится: это одна опознавательная строка
	// «модель · прошивка · ядро · арх · аптайм», и разорванная посередине она
	// перестаёт читаться как одна. Truncation вместо переноса — если окно
	// сузили руками, лучше отрезать хвост, чем разломать строку надвое.
	v.machine.Wrapping = fyne.TextWrapOff
	v.machine.Truncation = fyne.TextTruncateEllipsis
	v.machineSw.Wrapping = fyne.TextWrapOff
	v.machineSw.Truncation = fyne.TextTruncateEllipsis
	v.archWarn.Wrapping = fyne.TextWrapWord
	v.archWarn.Hide()
	v.disksHint.Wrapping = fyne.TextWrapWord
	v.ifacesHint.Wrapping = fyne.TextWrapWord
	v.errBar.Hide()

	// Фильтр перерисовывает из последнего ответа, а не ждёт следующего опроса:
	// пауза в 2 секунды после клика читается как «не сработало».
	v.ifacesFilter = widget.NewSelect([]string{
		locale.T("All"),
		locale.T("Up only"),
		locale.T("With traffic"),
	}, func(string) { v.redrawInterfaces() })
	v.ifacesFilter.SetSelectedIndex(0)
	return v
}

// ifaceMode — выбранный режим фильтра.
//
// По индексу, а не по подписи: подпись переводится, и сравнение с ней сломало
// бы фильтр в любом языке, кроме того, на котором его писали.
func (v *hostView) ifaceMode() string {
	switch v.ifacesFilter.SelectedIndex() {
	case 1:
		return hostIfaceUp
	case 2:
		return hostIfaceTraffic
	default:
		return hostIfaceAll
	}
}

// hostIfaceVisible — проходит ли интерфейс фильтр.
func hostIfaceVisible(i lxdclient.HostInterface, mode string) bool {
	switch mode {
	case hostIfaceUp:
		return i.Up
	case hostIfaceTraffic:
		// «С трафиком» — именно счётчик, а не скорость: скорость нулевая в
		// паузе между запросами, и интерфейс, по которому только что шли
		// гигабайты, исчезал бы из списка на ровном месте.
		return i.RxBytes > 0 || i.TxBytes > 0
	default:
		return true
	}
}

// hostBar — полоска без собственной подписи: цифра стоит в своей колонке
// рядом, где у неё есть единица измерения.
func hostBar() *widget.ProgressBar {
	b := widget.NewProgressBar()
	b.TextFormatter = func() string { return "" }
	return b
}

func hostBoldLabel(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// hostDimLabel — пояснения под таблицами: курсивом, чтобы не спорить за
// внимание с числами над ними.
func hostDimLabel(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
}

// hostNum — ячейка с числом, прижатым вправо: только так одинаковые величины
// в соседних строках стоят друг под другом.
func hostNum(s string, w float32) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(s, fyne.TextAlignTrailing, fyne.TextStyle{})
	return hostFixedWidth(l, w)
}

// hostDimNum — то же, что hostNum, но курсивом: значение не тревожное, а
// ожидаемое по построению.
func hostDimNum(s string, w float32) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(s, fyne.TextAlignTrailing, fyne.TextStyle{Italic: true})
	return hostFixedWidth(l, w)
}

// hostText — ячейка с текстом по левому краю.
func hostText(s string, w float32) fyne.CanvasObject {
	l := widget.NewLabel(s)
	l.Truncation = fyne.TextTruncateEllipsis
	return hostFixedWidth(l, w)
}

// hostFixedWidthLayout ставит объекту заданную ширину, оставляя высоту по
// содержимому.
//
// Без него Label растягивается по своему тексту, и колонки разъезжаются от
// строки к строке — ровно то, из-за чего таблица перестаёт читаться как
// таблица. Тот же приём, что в таблицах профайлера.
type hostFixedWidthLayout struct{ w float32 }

func (l hostFixedWidthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objs {
		if s := o.MinSize().Height; s > h {
			h = s
		}
	}
	return fyne.NewSize(l.w, h)
}

func (l hostFixedWidthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(l.w, size.Height))
	}
}

func hostFixedWidth(o fyne.CanvasObject, w float32) fyne.CanvasObject {
	return container.New(hostFixedWidthLayout{w: w}, o)
}

// hostThinLayout зажимает высоту виджета, оставляя ширину родителю.
//
// Нужен, потому что высота ProgressBar приходит из темы через MinSize() и
// задать её у самого виджета нельзя: Resize() перетрёт следующий же Layout
// родителя. Полоска здесь — не элемент управления, а отметка уровня рядом с
// числом, и в полную высоту строки она забирает внимание у самого числа.
type hostThinLayout struct{ h float32 }

func (l hostThinLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w := float32(0)
	for _, o := range objs {
		if s := o.MinSize().Width; s > w {
			w = s
		}
	}
	return fyne.NewSize(w, l.h)
}

func (l hostThinLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		// По центру строки: полоска тоньше текста рядом, и прижатая к верху
		// она читалась бы как подчёркивание заголовка сверху.
		o.Move(fyne.NewPos(0, (size.Height-l.h)/2))
		o.Resize(fyne.NewSize(size.Width, l.h))
	}
}

// hostThin — полоска в пятую часть обычной высоты.
func hostThin(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(hostThinLayout{h: hostBarHeight}, o)
}

// hostCellDepth — предел спуска в ячейку.
//
// Ячейка собирается нами и мелкая: контейнер ширины → контейнер высоты →
// виджет, то есть три уровня. Предел стоит не для красоты: виджет Fyne — это
// дерево со СВОИМИ внутренними контейнерами, и обход «до упора» уходил в них
// и дальше, пока рантайм не падал на переносе стека главного потока
// (fatal error: traceback did not unwind completely). Ищем только в своей
// обёртке, чужое внутреннее устройство не наш уровень.
const hostCellDepth = 3

// hostFindBar достаёт полоску из ячейки, сколько бы наших обёрток на ней ни
// было — но не глубже собственной обёртки.
func hostFindBar(o fyne.CanvasObject, depth int) *widget.ProgressBar {
	if depth <= 0 {
		return nil
	}
	switch t := o.(type) {
	case *widget.ProgressBar:
		return t
	case *fyne.Container:
		for _, child := range t.Objects {
			if b := hostFindBar(child, depth-1); b != nil {
				return b
			}
		}
	}
	return nil
}

// hostFindLabel — то же для подписи.
func hostFindLabel(o fyne.CanvasObject, depth int) *widget.Label {
	if depth <= 0 {
		return nil
	}
	switch t := o.(type) {
	case *widget.Label:
		return t
	case *fyne.Container:
		for _, child := range t.Objects {
			if l := hostFindLabel(child, depth-1); l != nil {
				return l
			}
		}
	}
	return nil
}

// content собирает дерево окна.
func (v *hostView) content() fyne.CanvasObject {
	cpuBlock := container.NewVBox(
		v.cpuTitle,
		hostThin(v.cpuBar),
		v.cpuCores,
		v.cpuLoad,
		v.cpuInterval,
	)
	memBlock := container.NewVBox(
		v.memTitle,
		hostThin(v.memBar),
		v.memDetail,
		v.memSwap,
	)
	// Два блока в ряд фиксированной ширины. Именно фиксированной: Label без
	// переноса отдаёт минимальной шириной всю свою строку, и один длинный
	// текст («… · cache 168.1 MB») распирал бы ряд шире окна — та же ловушка,
	// что раздувает диалоги на весь экран.
	top := container.NewHBox(
		hostFixedWidth(cpuBlock, hostBlockWide),
		hostFixedWidth(memBlock, hostBlockWide),
	)

	// Температура — строкой под нагрузкой, а не колонкой сбоку: это третий
	// показатель того же вопроса «тяжело ли машине сейчас», и в одну строку
	// зоны читаются подряд, без вертикального списка на пол-экрана.
	thermalRow := container.NewHBox(
		hostBoldLabel(locale.T("Temperature")),
		v.thermal,
		v.thermalList,
	)

	// Дескрипторы — под дисками: это тоже про исчерпание лимита, только не
	// места, а таблицы открытых файлов. Рядом их и читают.
	fdRow := container.NewHBox(
		hostBoldLabel(locale.T("File descriptors")),
		v.fdDaemon,
		v.fdSystem,
	)

	// Фильтр — в строке заголовка интерфейсов, справа от счётчика: он
	// относится к списку под ним, а не к окну целиком. Заголовку даём его
	// собственную ширину явно: в голом HBox Select налезал на текст.
	ifacesHead := container.NewHBox(
		hostFixedWidth(v.ifacesTitle, hostIfacesTitleWidth),
		v.ifacesFilter,
	)

	// Аптайм прижат к правому краю через Border: он отвечает на свой вопрос
	// («давно ли машина живёт»), и у правого края его находят взглядом сразу,
	// не вычитывая всю строку модели до конца.
	machineHead := container.NewVBox(
		container.NewBorder(nil, nil, nil, v.machineUp, v.machine),
		v.machineSw,
		v.archWarn,
	)

	body := container.NewVBox(
		machineHead,
		v.errBar,
		widget.NewSeparator(),
		top,
		thermalRow,
		widget.NewSeparator(),
		v.disksTitle,
		hostDisksHeader(),
		v.disks,
		v.disksHint,
		fdRow,
		widget.NewSeparator(),
		ifacesHead,
		hostIfacesHeader(),
		v.ifaces,
		v.ifacesHint,
	)
	return components.WrapInScrollWithGutter(body)
}

// hostHead — ячейка шапки: та же фиксированная ширина, что у данных под ней.
func hostHead(s string, w float32, trailing bool) fyne.CanvasObject {
	align := fyne.TextAlignLeading
	if trailing {
		align = fyne.TextAlignTrailing
	}
	return hostFixedWidth(widget.NewLabelWithStyle(s, align, fyne.TextStyle{Bold: true}), w)
}

// hostDisksHeader — шапка таблицы дисков.
//
// Собрана теми же hostFixedWidth, что и строки, поэтому колонки совпадают по
// построению, а не потому, что их не забыли поправить в двух местах.
func hostDisksHeader() fyne.CanvasObject {
	// Процент стоит ПЕРЕД полоской: так заголовок «Used» оказывается прямо
	// над своим числом, а не над безымянной полоской, и глаз не ищет, к
	// какой колонке относится подпись.
	return container.NewHBox(
		hostHead(locale.T("Mount"), hostColName, false),
		hostHead(locale.T("FS"), hostColFS, false),
		hostHead(locale.T("Used"), hostColPercent, true),
		hostHead("", hostColBar, false),
		hostHead(locale.T("Free"), hostColBytes, true),
		hostHead(locale.T("Flags"), hostColFlags, false),
	)
}

// hostIfacesHeader — шапка таблицы интерфейсов.
//
// Стрелки живут ЗДЕСЬ, по одной на колонку, а не в каждой ячейке. Приклеены к
// слову вплотную: пробел после ↑/↓ в этом шрифте рвёт отрисовку и вместо
// стрелки показывается тофу.
func hostIfacesHeader() fyne.CanvasObject {
	return container.NewHBox(
		hostHead(locale.T("Interface"), hostColName, false),
		hostHead(locale.T("↓rate"), hostColRate, true),
		hostHead(locale.T("↑rate"), hostColRate, true),
		hostHead(locale.T("↓total"), hostColBytes, true),
		hostHead(locale.T("↑total"), hostColBytes, true),
		hostHead(locale.T("err / drop"), hostColErrors, true),
		hostHead(locale.T("MTU"), hostColMTU, true),
	)
}

// update перерисовывает окно по свежему ответу.
//
// Ошибку показывает полосой, но НЕ затирает прошлые числа: связь до роутера
// может идти через сам VPN и моргать, а последние известные значения при
// разборе проблемы полезнее пустого экрана.
func (v *hostView) update(h lxdclient.HostInfo, hErr error,
	ifs lxdclient.HostInterfaces, ifErr error) {
	if hErr != nil {
		if errors.Is(hErr, lxdclient.ErrHostUnsupported) {
			// 404 — это «машину видно, демон старый», а не обрыв связи.
			v.errBar.SetText(locale.T(hostUnsupportedText))
		} else {
			debuglog.WarnLog("host window: %v", hErr)
			v.errBar.SetText(locale.Tf("Could not read host telemetry: %v", hErr))
		}
		v.errBar.Show()
		return
	}
	v.errBar.Hide()

	v.machine.SetText(hostMachineTitle(h))
	v.machineUp.SetText(hostMachineUptime(h))
	v.machineSw.SetText(hostMachineSoftware(h))

	// Сверка записанной платформы с настоящей. До этой ручки проверить её
	// было нечем: GOARCH машины оператор выбирает руками при добавлении, и
	// ошибка в выпадающем списке молча уезжала в сборку конфига.
	if warn := hostPlatformMismatch(v.wantOS, v.wantArch, h); warn != "" {
		v.archWarn.SetText(warn)
		v.archWarn.Show()
	} else {
		v.archWarn.Hide()
	}

	// CPU. Заголовок несёт либо процент, либо «ждём второй замер…»: пустая
	// полоска с нулём читалась бы как «простаивает».
	v.cpuTitle.SetText(locale.Tf("CPU  %s", hostCPUSummary(h.CPU)))
	v.cpuBar.SetValue(hostBarValue(h.CPU.UsagePercent))
	v.cpuLoad.SetText(hostLoadText(h.CPU))
	v.cpuInterval.SetText(hostInterval(h.CPU.IntervalSeconds))
	v.updateCores(h.CPU)

	// Память: и полоска, и подпись — от available. Среднее по free кричало бы
	// «занято» при памяти, лежащей в page cache.
	v.memTitle.SetText(locale.Tf("Memory  %s", hostPercent(h.Memory.UsedPercent)))
	v.memBar.SetValue(hostBarValue(h.Memory.UsedPercent))
	v.memDetail.SetText(hostMemoryDetail(h.Memory))
	v.memSwap.SetText(hostSwapText(h.Memory))

	v.thermal.SetText(hostThermalText(h.Thermal))
	v.thermalList.SetText(hostThermalZones(h.Thermal))

	v.fdDaemon.SetText(locale.Tf("daemon  %s", hostFDText(h.FD.Open, h.FD.Limit)))
	v.fdSystem.SetText(locale.Tf("system  %s", hostFDText(h.FD.SystemOpen, h.FD.SystemLimit)))

	v.updateDisks(h.Disk)
	v.updateInterfaces(ifs, ifErr)
}

// updateCores рисует разрез по ядрам.
//
// Он здесь не для полноты: одно ядро в полке при трёх свободных — это диагноз,
// который среднее прячет.
func (v *hostView) updateCores(c lxdclient.HostCPU) {
	if len(c.PerCorePercent) == 0 {
		if len(v.cpuCores.Objects) != 0 {
			v.cpuCores.RemoveAll()
			v.cpuCores.Refresh()
		}
		return
	}
	// Число ядер стабильно, поэтому строки создаются один раз, а дальше
	// только обновляются: пересборка на каждом тике мигала бы полосками.
	if len(v.cpuCores.Objects) != len(c.PerCorePercent) {
		v.cpuCores.RemoveAll()
		// Только число на ядро, без полоски: полосок в блоке уже одна —
		// общая, и повторять её разрез шестью маленькими значит спорить с
		// ней за внимание. Разрез читают, чтобы найти ядро в полке, а для
		// этого достаточно цифры.
		for i := range c.PerCorePercent {
			num := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
			v.cpuCores.Add(container.NewHBox(
				hostText(fmt.Sprintf("%d", i), hostColCore),
				hostFixedWidth(num, hostColPercent),
			))
		}
	}
	for i, pct := range c.PerCorePercent {
		row, ok := v.cpuCores.Objects[i].(*fyne.Container)
		if !ok {
			continue
		}
		p := pct
		// Виджеты ищем ПО ТИПУ и вглубь, а не по индексу: индекс молча
		// разъезжается при перестановке колонок или лишней обёртке, и строка
		// перестаёт обновляться без единого признака поломки. Номер ядра
		// пропускаем — он не меняется.
		for j, cell := range row.Objects {
			if bar := hostFindBar(cell, hostCellDepth); bar != nil {
				bar.SetValue(hostBarValue(&p))
				continue
			}
			if num := hostFindLabel(cell, hostCellDepth); num != nil && j > 0 {
				num.SetText(hostPercent(&p))
			}
		}
	}
	v.cpuCores.Refresh()
}

// updateDisks рисует точки монтирования таблицей.
func (v *hostView) updateDisks(d lxdclient.HostDisk) {
	v.disksTitle.SetText(locale.Tf("Disks  (max %s)", hostPercent(d.MaxUsedPercent)))

	v.disks.RemoveAll()
	anyReadOnly := false
	for _, m := range d.Mounts {
		if m.ReadOnly {
			anyReadOnly = true
		}
		// Флаги текстом со значком: значок ловит взгляд, текст остаётся
		// читаемым и на скриншоте в переписке, и в монохроме.
		flags := hostMountFlags(m)
		if flags != "" {
			flags = "[" + flags + "]"
		}

		// Read-only забит ПО ПОСТРОЕНИЮ: squashfs-образ упакован ровно под
		// размер данных, свободному месту там взяться неоткуда. Полоска в
		// полке — язык аварии, и рисовать им норму значит каждый раз звать
		// перепроверять то, что перепроверять не нужно. Поэтому у такой
		// строки полоски нет вовсе, а вместо процента — причина.
		roByDesign := m.ReadOnly && m.UsedPercent >= 99.5

		pct := hostNum(fmt.Sprintf("%.1f%%", m.UsedPercent), hostColPercent)
		var level fyne.CanvasObject = hostFixedWidth(widget.NewLabel(""), hostColBar)
		if roByDesign {
			pct = hostDimNum(locale.T("by design"), hostColPercent)
		} else {
			bar := hostBar()
			p := m.UsedPercent
			bar.SetValue(hostBarValue(&p))
			level = hostFixedWidth(hostThin(bar), hostColBar)
		}

		v.disks.Add(container.NewHBox(
			hostText(m.Path, hostColName),
			hostText(m.FSType, hostColFS),
			pct,
			level,
			hostNum(hostBytes(m.AvailableBytes), hostColBytes),
			hostText(flags, hostColFlags),
		))
	}

	// Подпись про read-only нужна ровно тогда, когда такая ФС есть: на
	// OpenWrt squashfs-корень вечно 100%, и без объяснения строка читается
	// как незамеченная авария.
	hint := ""
	if anyReadOnly {
		hint = locale.T("Read-only filesystems are excluded from the maximum: a squashfs root sits at a permanent 100%.")
	}
	if d.StateDirPath != "" {
		if hint != "" {
			hint += "\n"
		}
		hint += locale.Tf("State dir: %s", d.StateDirPath)
	}
	v.disksHint.SetText(hint)
	v.disks.Refresh()
}

// updateInterfaces принимает новый ответ и перерисовывает таблицу.
func (v *hostView) updateInterfaces(ifs lxdclient.HostInterfaces, err error) {
	if err != nil {
		v.ifacesTitle.SetText(locale.T("Interfaces"))
		v.ifacesHint.SetText(locale.Tf("Could not read interfaces: %v", err))
		return
	}
	v.lastIfaces = ifs
	v.hasIfaces = true
	v.redrawInterfaces()
}

// redrawInterfaces рисует интерфейсы таблицей из последнего ответа.
//
// По умолчанию показываются ВСЕ, включая lo и лежачие: «wan лёг» — ровно то,
// что нужно увидеть, и прятать это по умолчанию значило бы прятать ответ.
// Фильтр — выбор пользователя, а не поведение по умолчанию.
func (v *hostView) redrawInterfaces() {
	if !v.hasIfaces {
		return
	}
	ifs := v.lastIfaces
	mode := v.ifaceMode()

	shown := 0
	for _, i := range ifs.Interfaces {
		if hostIfaceVisible(i, mode) {
			shown++
		}
	}
	// В заголовке — сколько показано из скольких, иначе отфильтрованный
	// список выглядит как пропавшие интерфейсы.
	if shown == len(ifs.Interfaces) {
		v.ifacesTitle.SetText(locale.Tf("Interfaces (%d)", len(ifs.Interfaces)))
	} else {
		v.ifacesTitle.SetText(locale.Tf("Interfaces (%d of %d)", shown, len(ifs.Interfaces)))
	}

	v.ifaces.RemoveAll()
	for _, i := range ifs.Interfaces {
		if !hostIfaceVisible(i, mode) {
			continue
		}
		// Лежачие гаснут кружком и не жирные: строка остаётся на месте, но
		// перестаёт спорить за внимание с работающими.
		marker := "○"
		if i.Up {
			marker = "●"
		}
		name := widget.NewLabelWithStyle(marker+" "+i.Name,
			fyne.TextAlignLeading, fyne.TextStyle{Bold: i.Up})
		name.Truncation = fyne.TextTruncateEllipsis

		v.ifaces.Add(container.NewHBox(
			hostFixedWidth(name, hostColName),
			hostNum(hostIfaceRxRate(i), hostColRate),
			hostNum(hostIfaceTxRate(i), hostColRate),
			hostNum(hostIfaceRxTotal(i), hostColBytes),
			hostNum(hostIfaceTxTotal(i), hostColBytes),
			hostNum(hostIfaceErrors(i), hostColErrors),
			hostNum(hostIfaceMTU(i), hostColMTU),
		))
	}
	// Окно дельты общее на весь ответ: интерфейсы снимаются одним проходом.
	hint := locale.T("Graph the counter, read the rate: a counter survives restarts and gaps, a rate lies across them.")
	if iv := hostInterval(ifs.IntervalSeconds); iv != "" {
		hint += " · " + iv
	}
	v.ifacesHint.SetText(hint)
	v.ifaces.Refresh()
}

// hostThermalZones — датчики в одну строку через разделитель.
//
// Именно в строку: блок стоит горизонтальной полосой под нагрузкой, и
// перечисление в столбик растянуло бы её на пол-экрана ради трёх чисел.
func hostThermalZones(t *lxdclient.HostThermal) string {
	if t == nil || len(t.Zones) == 0 {
		return locale.T("no sensors on this machine")
	}
	s := ""
	for i, z := range t.Zones {
		if i > 0 {
			s += " · "
		}
		s += fmt.Sprintf("%s %.1f °C", z.Name, z.Celsius)
	}
	return s
}
