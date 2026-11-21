package components

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// WheelThrough отдаёт колесо мыши прокрутке формы там, где его иначе забрало
// бы поле ввода.
//
// Зачем: Fyne отдаёт событие колеса самому верхнему Scrollable под курсором, а
// у каждого widget.Entry внутри есть свой Scroll — даже у однострочного. Над
// полем ввода колесо уходило в это поле и форму не двигало.
//
// Как: строка формы оборачивается Wrap — поверх неё ложится невидимый слой,
// который сам Scrollable и передаёт событие прокрутке формы. Клики, фокус,
// наведение и курсор слой не трогает: Fyne ищет получателя по интерфейсу, а
// слой реализует только fyne.Scrollable. Выпадающие списки — отдельное дерево
// overlay, на них слой не действует.
//
// Строку с многострочным полем НЕ оборачивают: такому полю своя прокрутка
// нужна, колесо над ним листает его текст.
type WheelThrough struct {
	scroll *container.Scroll
}

// NewWheelThrough — пустая связка; прокрутку задаёт SetScroll, когда она
// создана (строки формы строятся раньше неё).
func NewWheelThrough() *WheelThrough { return &WheelThrough{} }

// SetScroll задаёт прокрутку, которой достаётся колесо.
func (t *WheelThrough) SetScroll(s *container.Scroll) { t.scroll = s }

// Wrap кладёт поверх obj слой, отдающий колесо прокрутке формы.
func (t *WheelThrough) Wrap(obj fyne.CanvasObject) fyne.CanvasObject {
	layer := &wheelLayer{owner: t}
	layer.ExtendBaseWidget(layer)
	return container.NewStack(obj, layer)
}

type wheelLayer struct {
	widget.BaseWidget
	owner *WheelThrough
}

func (w *wheelLayer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// Scrolled отдаёт колесо прокрутке формы.
func (w *wheelLayer) Scrolled(ev *fyne.ScrollEvent) {
	if w.owner != nil && w.owner.scroll != nil {
		w.owner.scroll.Scrolled(ev)
	}
}
