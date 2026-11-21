package components

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// wheelForwarder — невидимый слой поверх содержимого прокручиваемой формы,
// который отдаёт колесо мыши её прокрутке.
//
// Зачем: Fyne отдаёт событие колеса самому верхнему Scrollable под курсором, а
// у каждого widget.Entry внутри есть свой Scroll — даже у однострочного. Над
// полем ввода колесо уходило в это поле и форму не двигало. Слой лежит выше
// полей и сам Scrollable, поэтому событие достаётся ему.
//
// Клики, фокус, наведение и курсор слой не трогает: Fyne ищет получателя по
// интерфейсу, а слой реализует только fyne.Scrollable. Выпадающие списки —
// отдельное дерево overlay, на них слой не действует.
//
// Цена: многострочное поле внутри такой формы колесом не прокручивается —
// только клавишами.
type wheelForwarder struct {
	widget.BaseWidget
	target *container.Scroll
}

func (w *wheelForwarder) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// Scrolled отдаёт колесо прокрутке формы.
func (w *wheelForwarder) Scrolled(ev *fyne.ScrollEvent) {
	if w.target != nil {
		w.target.Scrolled(ev)
	}
}

// NewVScrollWheelThrough — вертикальная прокрутка формы, которая двигается
// колесом и тогда, когда курсор стоит над полем ввода.
func NewVScrollWheelThrough(content fyne.CanvasObject) *container.Scroll {
	fw := &wheelForwarder{}
	fw.ExtendBaseWidget(fw)
	scroll := container.NewVScroll(container.NewStack(content, fw))
	fw.target = scroll
	return scroll
}
