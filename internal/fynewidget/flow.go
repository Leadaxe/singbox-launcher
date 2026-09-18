// File flow.go — раскладка «строка за строкой с переносом» и виджет-обёртка,
// которая честно сообщает свою высоту.
//
// # Зачем
//
// Ряд чипов нельзя класть ни в HBox (уедет за край окна), ни в сетку с
// фиксированным числом колонок (каждый чип растянется на долю ширины, и
// «🇳🇱 1» станет шириной с «Reality+Vision»). Нужен поток: каждый элемент своей
// ширины, перенос по краю контейнера.
//
// # Ловушка высоты
//
// Fyne меряет MinSize ДО того, как узнаёт ширину, а у потока высота от ширины
// и зависит: та же сотня чипов — это четыре строки в узком окне и две в
// широком. Layout.MinSize вернуть правильную высоту не может — ему ширину не
// передают.
//
// Решение — обёртка FlowBox (виджет, а не голый контейнер). Её Resize знает
// фактическую ширину, пересчитывает по ней высоту потока и, если та
// изменилась, помечает себя изменённой и просит РОДИТЕЛЯ переразложиться
// (canvas.Refresh по дереву вверх у Fyne нет, поэтому будим родителя явно —
// VBox перемеряет детей и раздвинет строки). Без этого шага строки ниже
// наезжали бы на перенесённые чипы, а после сужения окна оставалась бы
// дырка от прежней высоты.
//
// MinSize самой обёртки по ширине — САМЫЙ ШИРОКИЙ элемент, а не их сумма:
// сумма раздула бы окно до ширины всех чипов в одну строку, ровно то, от чего
// уходим.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Зазоры потока: по горизонтали между соседями, по вертикали между строками.
const (
	flowGapH = 4
	flowGapV = 4
)

var _ fyne.Widget = (*FlowBox)(nil)

// FlowLayout раскладывает элементы слева направо с переносом по ширине.
type FlowLayout struct{}

// Layout implements fyne.Layout.
func (FlowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	x, y := float32(0), float32(0)
	rowHeight := float32(0)
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		// Перенос: не лезет в остаток строки и строка не пуста (одинокий
		// элемент шире контейнера остаётся на своей строке, иначе он ушёл бы
		// в бесконечный перенос).
		if x > 0 && x+ms.Width > size.Width {
			x = 0
			y += rowHeight + flowGapV
			rowHeight = 0
		}
		o.Resize(ms)
		o.Move(fyne.NewPos(x, y))
		x += ms.Width + flowGapH
		if ms.Height > rowHeight {
			rowHeight = ms.Height
		}
	}
}

// MinSize implements fyne.Layout: ширина — самый широкий элемент, высота — одна
// строка. Настоящую высоту под фактическую ширину считает FlowBox.
func (FlowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	w, h := float32(0), float32(0)
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		if ms.Width > w {
			w = ms.Width
		}
		if ms.Height > h {
			h = ms.Height
		}
	}
	return fyne.NewSize(w, h)
}

// flowHeight — высота потока при заданной ширине.
func flowHeight(objects []fyne.CanvasObject, width float32) float32 {
	x, y := float32(0), float32(0)
	rowHeight := float32(0)
	any := false
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		any = true
		ms := o.MinSize()
		if x > 0 && x+ms.Width > width {
			x = 0
			y += rowHeight + flowGapV
			rowHeight = 0
		}
		x += ms.Width + flowGapH
		if ms.Height > rowHeight {
			rowHeight = ms.Height
		}
	}
	if !any {
		return 0
	}
	return y + rowHeight
}

// FlowBox — контейнер-поток, знающий свою высоту при текущей ширине.
type FlowBox struct {
	widget.BaseWidget

	// MaxHeight — потолок высоты; 0 = без потолка. Выше потолка содержимое
	// прокручивается: сотня эмодзи иначе выдавила бы кнопки за край экрана.
	MaxHeight float32

	objects []fyne.CanvasObject
	inner   *fyne.Container
	scroll  *container.Scroll

	// lastWidth/lastHeight — от чего считали прошлый раз; пересчёт только на
	// смену ширины, иначе Resize зацикливал бы сам себя через Refresh.
	lastWidth  float32
	lastHeight float32
	// pendingRelayout — перекладка окна уже заказана и ещё не выполнена.
	pendingRelayout bool
}

// NewFlowBox собирает поток из objects.
func NewFlowBox(objects ...fyne.CanvasObject) *FlowBox {
	f := &FlowBox{objects: objects}
	f.inner = container.New(FlowLayout{}, objects...)
	f.ExtendBaseWidget(f)
	return f
}

// SetObjects заменяет содержимое потока.
func (f *FlowBox) SetObjects(objects []fyne.CanvasObject) {
	f.objects = objects
	f.inner.Objects = objects
	f.lastWidth = 0 // высоту обязательно пересчитать под новый состав
	f.inner.Refresh()
	f.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (f *FlowBox) CreateRenderer() fyne.WidgetRenderer {
	if f.MaxHeight > 0 {
		f.scroll = container.NewVScroll(f.inner)
		return widget.NewSimpleRenderer(f.scroll)
	}
	return widget.NewSimpleRenderer(f.inner)
}

// MinSize implements fyne.Widget.
//
// Высота — посчитанная под последнюю известную ширину; пока её нет (первая
// раскладка) — высота одной строки от FlowLayout.MinSize.
func (f *FlowBox) MinSize() fyne.Size {
	base := FlowLayout{}.MinSize(f.objects)
	h := f.lastHeight
	if f.lastWidth == 0 {
		h = base.Height
	}
	if f.MaxHeight > 0 && h > f.MaxHeight {
		h = f.MaxHeight
	}
	return fyne.NewSize(base.Width, h)
}

// Resize implements fyne.CanvasObject: здесь и только здесь известна настоящая
// ширина, значит здесь и считается высота.
//
// Родитель зовёт Resize с высотой, которую взял из ПРОШЛОГО MinSize — под
// прежнюю ширину. Принять её как есть значило бы отрисовать поток обрезанным
// (после сужения окна) или с дыркой (после расширения) до следующего кадра,
// поэтому свою высоту виджет ставит сам и просит родителя перемериться.
func (f *FlowBox) Resize(size fyne.Size) {
	if size.Width > 0 && size.Width != f.lastWidth {
		f.lastWidth = size.Width
		h := flowHeight(f.objects, size.Width)
		if h != f.lastHeight {
			f.lastHeight = h
			// Высота изменилась — родительская раскладка обязана перемерить
			// нас и раздвинуть соседей. Откладываем на следующий тик: мы
			// внутри чужого Layout, и перезапуск раскладки прямо отсюда
			// Fyne не гарантирует довести.
			f.requestParentRelayout()
		}
	}
	// Ниже посчитанной высоты не сжимаемся (с оглядкой на потолок, за которым
	// содержимое прокручивается, а не растёт).
	want := f.lastHeight
	if f.MaxHeight > 0 && want > f.MaxHeight {
		want = f.MaxHeight
	}
	if size.Height < want {
		size.Height = want
	}
	f.BaseWidget.Resize(size)
}

// requestParentRelayout будит перерасчёт раскладки окна.
//
// Зациклиться это не может: высота — чистая функция ширины, и второй проход
// при той же ширине даёт то же число, на котором проверка `h != f.lastHeight`
// гасит цепочку. Флаг pendingRelayout снимает лишь дубли, когда за один кадр
// приходит несколько Resize (окно тянут мышью).
//
// Refresh всего содержимого окна, а не только себя: раздвинуть обязан РОДИТЕЛЬ
// (VBox формы), а он перемеряет детей только при собственной перекладке.
func (f *FlowBox) requestParentRelayout() {
	if f.pendingRelayout {
		return
	}
	f.pendingRelayout = true
	obj := fyne.CanvasObject(f)
	fyne.Do(func() {
		f.pendingRelayout = false
		if c := fyne.CurrentApp().Driver().CanvasForObject(obj); c != nil {
			if content := c.Content(); content != nil {
				content.Refresh()
			}
		}
	})
}
