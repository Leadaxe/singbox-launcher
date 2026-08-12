package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
)

// Вкладки Local и Remote (SPEC 098 §2.1).
//
// Обе устроены одинаково: СЛЕВА список прокси, СПРАВА управление. Разница
// только в том, чем управляем — своим ядром или списком чужих машин.
// Одинаковая геометрия не эстетика: список прокси на обеих вкладках это
// буквально один виджет с одним поведением, и переход между вкладками не
// требует переучиваться.
//
// До SPEC 098 это были вкладки Core и Servers, а управление удалёнными
// машинами жило в трёх местах сразу (шапка Servers, окно подключения, визард).
// Чтобы настроить одну машину, надо было обойти три экрана, и ни на одном не
// было видно её целиком.

// Ширины колонок подобраны по факту, а не по доле.
//
// leftColumnWidth — ширина списка прокси: имя узла вроде
// «AL:🇳🇱-Нидерланды-2» с подписью протокола, плюс колонка задержки и кнопка
// подключения. Уже этого имена начинают резаться многоточием.
//
// rightColumnWidth — панель управления: там короткие строки (версия ядра,
// имя и адрес машины), поэтому она заметно уже левой.
//
// Доля вместо абсолютных величин тут не работает: HSplit считает offset от
// MinSize дочерних элементов, а список прокси просит много ширины и
// продавливает разделитель вправо — правая колонка схлопывалась в ноль.
// Поэтому правой задаётся собственный минимум (minSizeBox), а offset
// вычисляется из фактических ширин.
const (
	leftColumnWidth  = 395
	rightColumnWidth = 165
)

// splitColumnRatio — доля окна под левую колонку при стартовом размере.
// Явный float: целочисленное деление констант дало бы 2, а не 0.7.
const splitColumnRatio = float64(leftColumnWidth) / float64(leftColumnWidth+rightColumnWidth)

// CreateLocalTab — вкладка Local: слева прокси локального ядра, справа
// управление этим ядром (бывшая вкладка Core целиком).
//
// Возвращает и панель списка: у Local и Remote независимые состояния, и
// переключение вкладки обязано отдать разделяемые слоты UIService той панели,
// которую пользователь видит (ProxyListPanel.Activate).
func CreateLocalTab(ac *core.AppController) (fyne.CanvasObject, *ProxyListPanel) {
	panel := CreateProxyListPanel(ac, services.ScopeLocal)
	split := container.NewHSplit(
		panel.Content,
		withColumnWidth(CreateCoreDashboardTab(ac), rightColumnWidth),
	)
	split.SetOffset(splitColumnRatio)
	return split, panel
}

// withColumnWidth фиксирует минимальную ширину колонки, не трогая высоту:
// без этого HSplit отдаёт всё место тому, кто просит больше, и правая
// колонка исчезает.
func withColumnWidth(content fyne.CanvasObject, width float32) fyne.CanvasObject {
	return container.New(&minSizeBox{min: fyne.NewSize(width, 0)}, content)
}

// CreateRemoteTab — вкладка Remote: слева прокси ВЫБРАННОЙ машины, справа
// список машин с управлением.
//
// onSelectionChanged перезагружает левую колонку после смены активной машины.
// Без этого список остался бы с узлами предыдущей — то есть показывал бы
// чужие данные под именем новой машины (нарушение инварианта §5.3).
func CreateRemoteTab(ac *core.AppController) (fyne.CanvasObject, *ProxyListPanel) {
	proxyPanel := CreateProxyListPanel(ac, services.ScopeRemote)
	// Обновляем СВОЮ панель напрямую, а не через UIService.RefreshAPIFunc:
	// выбор машины касается списка Remote, и промахнуться мимо него в момент,
	// когда слоты принадлежат другой вкладке, нельзя.
	machines := CreateMachineListPanel(ac, proxyPanel)
	split := container.NewHSplit(proxyPanel.Content, withColumnWidth(machines, rightColumnWidth))
	split.SetOffset(splitColumnRatio)
	return split, proxyPanel
}

// MinWindowSize — минимальный и стартовый размер главного окна (§3.2).
//
// Растянуть можно, сжать ниже — нет: обе колонки перестают помещаться, и
// вместо деградации получается каша из обрезанных подписей. 1000 берётся из
// измеримого: левая колонка не ужимается ниже ~420px (иначе имена узлов
// схлопываются в многоточие), правой нужно ~300px на строку версии ядра и
// адрес машины, плюс поля и разделитель. 700 по высоте — блок Core занимает
// ~400px, ниже должно оставаться место хотя бы на несколько строк списка.
//
// Адаптивная раскладка (одна колонка на узких экранах) вне scope SPEC 098.
var MinWindowSize = fyne.NewSize(leftColumnWidth+rightColumnWidth, 700)

// minSizeBox — контейнер, навязывающий содержимому нижнюю границу размера.
//
// Fyne не даёт окну стать меньше MinSize его контента, поэтому нижняя граница
// окна задаётся именно так, а не через SetFixedSize (тот запретил бы и
// растягивание, а §3.2 требует «растянуть можно, сжать нельзя»).
type minSizeBox struct {
	min fyne.Size
}

func (b *minSizeBox) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

func (b *minSizeBox) MinSize(objects []fyne.CanvasObject) fyne.Size {
	// Максимум из объявленного минимума и настоящего минимума контента:
	// занизить второй значило бы разрешить обрезание виджетов, которые сами
	// просят больше.
	out := b.min
	for _, o := range objects {
		m := o.MinSize()
		if m.Width > out.Width {
			out.Width = m.Width
		}
		if m.Height > out.Height {
			out.Height = m.Height
		}
	}
	return out
}

// WithMinWindowSize оборачивает контент главного окна, задавая ему нижнюю
// границу MinWindowSize.
func WithMinWindowSize(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&minSizeBox{min: MinWindowSize}, content)
}
