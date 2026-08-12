package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
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

// splitColumnRatio — доля ширины под левую колонку.
//
// Список прокси шире панели управления: в нём длинные имена узлов и строки
// подписи (протокол, задержка), тогда как справа короткие поля — версия ядра,
// адрес машины. При 1000px это ~600/400, что укладывается в измеренные
// минимумы обеих колонок (§3.2).
const splitColumnRatio = 0.6

// CreateLocalTab — вкладка Local: слева прокси локального ядра, справа
// управление этим ядром (бывшая вкладка Core целиком).
func CreateLocalTab(ac *core.AppController) fyne.CanvasObject {
	split := container.NewHSplit(
		CreateProxyListPanel(ac),
		CreateCoreDashboardTab(ac),
	)
	split.SetOffset(splitColumnRatio)
	return split
}

// CreateRemoteTab — вкладка Remote: слева прокси ВЫБРАННОЙ машины, справа
// список машин с управлением.
//
// onSelectionChanged перезагружает левую колонку после смены активной машины.
// Без этого список остался бы с узлами предыдущей — то есть показывал бы
// чужие данные под именем новой машины (нарушение инварианта §5.3).
func CreateRemoteTab(ac *core.AppController) fyne.CanvasObject {
	proxyPanel := CreateProxyListPanel(ac)
	machines := CreateMachineListPanel(ac, func() {
		if ac.UIService != nil && ac.UIService.RefreshAPIFunc != nil {
			ac.UIService.RefreshAPIFunc()
		}
	})
	split := container.NewHSplit(proxyPanel, machines)
	split.SetOffset(splitColumnRatio)
	return split
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
var MinWindowSize = fyne.NewSize(1000, 700)

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
