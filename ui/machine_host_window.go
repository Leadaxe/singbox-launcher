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
// а выбор того, что именно показано. Две секунды сглаживают дребезг и не
// заваливают роутер: два GET'а раз в две секунды дешевле одной перерисовки
// таблицы профайлера.
const hostWindowPollInterval = 2 * time.Second

// Ширины колонок таблиц.
//
// Фиксированные, а не по содержимому: колонка по содержимому «дышит» от тика к
// тику (12.1% и 7.4% — разной ширины), и числа перестают стоять друг под
// другом ровно в тот момент, когда их сравнивают глазами.
// Ширины подобраны под самое длинное реальное значение своей колонки, а не с
// запасом: сумма всех колонок задаёт минимальную ширину окна, и лишние 20 px
// в каждой выталкивают правый край за экран.
const (
	hostColBar     = 84 // полоска заполнения
	hostColPercent = 70 // «100.0%» плюс зазор от полоски слева
	hostColBytes   = 78 // «213.4 MB»
	hostColFS      = 84 // «squashfs» — под самое длинное имя ФС целиком
	hostColFlags   = 60 // «[state]»
	hostColRate    = 72 // «229 B/s»
	hostColErrors  = 84 // «120 / 52210»
	hostColMTU     = 48 // «65536»
	hostColCore    = 20 // номер ядра
	// Имя — тоже фиксированное. Растягивающийся центр Border забирал всю
	// свободную ширину, из-за чего строка всегда была шире окна и таблицу
	// приходилось листать вбок.
	hostColName = 150 // «/tmp/run/netns», «● phy0-ap0»

	// Ширины блоков верхнего ряда. Сумма (2×250 + 190 ≈ 690) держится ниже
	// ширины таблицы интерфейсов, чтобы окно задавала таблица, а не шапка.
	hostBlockWide = 250 // CPU, память
	hostBlockSide = 190 // температура + дескрипторы
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
		ShowErrorText(ac.UIService.MainWindow, d.Name, locale.T("remote.host.needs_connect"))
		return
	}

	win := ac.UIService.Application.NewWindow(locale.Tf("remote.host.window_title", d.Name))
	view := newHostView()

	// Поллинг живёт в своей горутине и гаснет вместе с окном: без остановки
	// он продолжал бы ходить к роутеру после закрытия.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopCh) }) }

	go func() {
		// Первый заход сразу, дальше по тику: ждать две секунды перед самым
		// первым показом незачем, у окна и так пустая шапка.
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
	win.Resize(fyne.NewSize(940, 700))
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
	machine *widget.Label
	errBar  *widget.Label

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
	fdBar    *widget.ProgressBar
	fdSystem *widget.Label

	disksTitle *widget.Label
	disks      *fyne.Container
	disksHint  *widget.Label

	ifacesTitle *widget.Label
	ifaces      *fyne.Container
	ifacesHint  *widget.Label
}

func newHostView() *hostView {
	v := &hostView{
		machine:     widget.NewLabel(locale.T("remote.host.loading")),
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
		fdBar:       hostBar(),
		fdSystem:    widget.NewLabel(""),
		disksTitle:  hostBoldLabel(""),
		disks:       container.NewVBox(),
		disksHint:   hostDimLabel(""),
		ifacesTitle: hostBoldLabel(""),
		ifaces:      container.NewVBox(),
		ifacesHint:  hostDimLabel(""),
	}
	v.errBar.Wrapping = fyne.TextWrapWord
	v.machine.Wrapping = fyne.TextWrapWord
	v.disksHint.Wrapping = fyne.TextWrapWord
	v.ifacesHint.Wrapping = fyne.TextWrapWord
	v.errBar.Hide()
	return v
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

// content собирает дерево окна.
func (v *hostView) content() fyne.CanvasObject {
	cpuBlock := container.NewVBox(
		v.cpuTitle,
		v.cpuBar,
		v.cpuCores,
		v.cpuLoad,
		v.cpuInterval,
	)
	memBlock := container.NewVBox(
		v.memTitle,
		v.memBar,
		v.memDetail,
		v.memSwap,
	)
	// Температура и дескрипторы — узкой колонкой справа: это то, на что
	// смотрят вторым взглядом, и полная ширина им не нужна.
	sideBlock := container.NewVBox(
		hostBoldLabel(locale.T("remote.host.thermal")),
		v.thermal,
		v.thermalList,
		widget.NewSeparator(),
		hostBoldLabel(locale.T("remote.host.fd")),
		v.fdDaemon,
		v.fdBar,
		v.fdSystem,
	)

	// Три блока в ряд фиксированной ширины. Именно фиксированной: Label без
	// переноса отдаёт минимальной шириной всю свою строку, и один длинный
	// текст («… · cache 168.1 MB») распирал бы ряд шире окна — та же ловушка,
	// что раздувает диалоги на весь экран.
	top := container.NewHBox(
		hostFixedWidth(cpuBlock, hostBlockWide),
		hostFixedWidth(memBlock, hostBlockWide),
		hostFixedWidth(sideBlock, hostBlockSide),
	)

	body := container.NewVBox(
		v.machine,
		v.errBar,
		widget.NewSeparator(),
		top,
		widget.NewSeparator(),
		v.disksTitle,
		hostDisksHeader(),
		v.disks,
		v.disksHint,
		widget.NewSeparator(),
		v.ifacesTitle,
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
		hostHead(locale.T("remote.host.col_path"), hostColName, false),
		hostHead(locale.T("remote.host.col_fs"), hostColFS, false),
		hostHead(locale.T("remote.host.col_used"), hostColPercent, true),
		hostHead("", hostColBar, false),
		hostHead(locale.T("remote.host.col_free"), hostColBytes, true),
		hostHead(locale.T("remote.host.col_flags"), hostColFlags, false),
	)
}

// hostIfacesHeader — шапка таблицы интерфейсов.
//
// Стрелки живут ЗДЕСЬ, по одной на колонку, а не в каждой ячейке. Приклеены к
// слову вплотную: пробел после ↑/↓ в этом шрифте рвёт отрисовку и вместо
// стрелки показывается тофу.
func hostIfacesHeader() fyne.CanvasObject {
	return container.NewHBox(
		hostHead(locale.T("remote.host.col_iface"), hostColName, false),
		hostHead(locale.T("remote.host.col_rx_rate"), hostColRate, true),
		hostHead(locale.T("remote.host.col_tx_rate"), hostColRate, true),
		hostHead(locale.T("remote.host.col_rx_total"), hostColBytes, true),
		hostHead(locale.T("remote.host.col_tx_total"), hostColBytes, true),
		hostHead(locale.T("remote.host.col_errors"), hostColErrors, true),
		hostHead(locale.T("remote.host.col_mtu"), hostColMTU, true),
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
			v.errBar.SetText(locale.T("remote.host.unsupported"))
		} else {
			debuglog.WarnLog("host window: %v", hErr)
			v.errBar.SetText(locale.Tf("remote.host.error", hErr))
		}
		v.errBar.Show()
		return
	}
	v.errBar.Hide()

	v.machine.SetText(hostMachineLine(h))

	// CPU. Заголовок несёт либо процент, либо «ждём второй замер…»: пустая
	// полоска с нулём читалась бы как «простаивает».
	v.cpuTitle.SetText(locale.Tf("remote.host.cpu", hostCPUSummary(h.CPU)))
	v.cpuBar.SetValue(hostBarValue(h.CPU.UsagePercent))
	v.cpuLoad.SetText(hostLoadText(h.CPU))
	v.cpuInterval.SetText(hostInterval(h.CPU.IntervalSeconds))
	v.updateCores(h.CPU)

	// Память: и полоска, и подпись — от available. Среднее по free кричало бы
	// «занято» при памяти, лежащей в page cache.
	v.memTitle.SetText(locale.Tf("remote.host.memory", hostPercent(h.Memory.UsedPercent)))
	v.memBar.SetValue(hostBarValue(h.Memory.UsedPercent))
	v.memDetail.SetText(hostMemoryDetail(h.Memory))
	v.memSwap.SetText(hostSwapText(h.Memory))

	v.thermal.SetText(hostThermalText(h.Thermal))
	v.thermalList.SetText(hostThermalZones(h.Thermal))

	v.fdDaemon.SetText(locale.Tf("remote.host.fd_daemon", hostFDText(h.FD.Open, h.FD.Limit)))
	v.fdBar.SetValue(hostFDRatio(h.FD.Open, h.FD.Limit))
	v.fdSystem.SetText(locale.Tf("remote.host.fd_system", hostFDText(h.FD.SystemOpen, h.FD.SystemLimit)))

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
		for i := range c.PerCorePercent {
			bar := hostBar()
			num := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
			v.cpuCores.Add(container.NewHBox(
				hostText(fmt.Sprintf("%d", i), hostColCore),
				hostFixedWidth(num, hostColPercent),
				hostFixedWidth(bar, hostColBar),
			))
		}
	}
	for i, pct := range c.PerCorePercent {
		row, ok := v.cpuCores.Objects[i].(*fyne.Container)
		if !ok {
			continue
		}
		p := pct
		// Виджеты ищем ПО ТИПУ, а не по индексу: индекс молча разъезжается
		// при перестановке колонок, и строка перестаёт обновляться без
		// единого признака поломки. Номер ядра пропускаем — он не меняется.
		for j, cell := range row.Objects {
			wrap, ok := cell.(*fyne.Container)
			if !ok || len(wrap.Objects) == 0 {
				continue
			}
			switch w := wrap.Objects[0].(type) {
			case *widget.ProgressBar:
				w.SetValue(hostBarValue(&p))
			case *widget.Label:
				if j > 0 {
					w.SetText(hostPercent(&p))
				}
			}
		}
	}
	v.cpuCores.Refresh()
}

// updateDisks рисует точки монтирования таблицей.
func (v *hostView) updateDisks(d lxdclient.HostDisk) {
	v.disksTitle.SetText(locale.Tf("remote.host.disks", hostPercent(d.MaxUsedPercent)))

	v.disks.RemoveAll()
	anyReadOnly := false
	for _, m := range d.Mounts {
		if m.ReadOnly {
			anyReadOnly = true
		}
		bar := hostBar()
		p := m.UsedPercent
		bar.SetValue(hostBarValue(&p))

		// Флаги текстом, а не цветом: пометка должна читаться и на скриншоте
		// в переписке, и в монохроме.
		flags := hostMountFlags(m)
		if flags != "" {
			flags = "[" + flags + "]"
		}
		v.disks.Add(container.NewHBox(
			hostText(m.Path, hostColName),
			hostText(m.FSType, hostColFS),
			hostNum(fmt.Sprintf("%.1f%%", m.UsedPercent), hostColPercent),
			hostFixedWidth(bar, hostColBar),
			hostNum(hostBytes(m.AvailableBytes), hostColBytes),
			hostText(flags, hostColFlags),
		))
	}

	// Подпись про read-only нужна ровно тогда, когда такая ФС есть: на
	// OpenWrt squashfs-корень вечно 100%, и без объяснения строка читается
	// как незамеченная авария.
	hint := ""
	if anyReadOnly {
		hint = locale.T("remote.host.disks_ro_hint")
	}
	if d.StateDirPath != "" {
		if hint != "" {
			hint += "\n"
		}
		hint += locale.Tf("remote.host.state_dir", d.StateDirPath)
	}
	v.disksHint.SetText(hint)
	v.disks.Refresh()
}

// updateInterfaces рисует интерфейсы таблицей.
//
// Показываются ВСЕ, включая lo и лежачие: «wan лёг» — ровно то, что нужно
// увидеть, и прятать это в фильтр значило бы прятать ответ.
func (v *hostView) updateInterfaces(ifs lxdclient.HostInterfaces, err error) {
	if err != nil {
		v.ifacesTitle.SetText(locale.T("remote.host.ifaces"))
		v.ifacesHint.SetText(locale.Tf("remote.host.ifaces_error", err))
		return
	}
	v.ifacesTitle.SetText(locale.Tf("remote.host.ifaces_n", len(ifs.Interfaces)))

	v.ifaces.RemoveAll()
	for _, i := range ifs.Interfaces {
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
	hint := locale.T("remote.host.ifaces_hint")
	if iv := hostInterval(ifs.IntervalSeconds); iv != "" {
		hint += " · " + iv
	}
	v.ifacesHint.SetText(hint)
	v.ifaces.Refresh()
}

// hostThermalZones — датчики построчно; пусто, когда их нет.
func hostThermalZones(t *lxdclient.HostThermal) string {
	if t == nil || len(t.Zones) == 0 {
		return locale.T("remote.host.thermal_none")
	}
	s := ""
	for i, z := range t.Zones {
		if i > 0 {
			s += "\n"
		}
		s += fmt.Sprintf("%s  %.1f °C", z.Name, z.Celsius)
	}
	return s
}
