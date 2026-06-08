package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var (
	_ fyne.Tappable      = (*TapWrap)(nil)
	_ desktop.Cursorable = (*TapWrap)(nil)
)

// TapWrap делает область Content отвечающей на основной тап (например подпись рядом с чекбоксом).
// Дочерние виджеты без Tapped пробрасывают событие наверх к этой обёртке.
type TapWrap struct {
	widget.BaseWidget

	Content fyne.CanvasObject

	OnTapped func()
}

// NewTapWrap wraps inner content; on primary tap invokes OnTapped if set.
func NewTapWrap(content fyne.CanvasObject, onTapped func()) *TapWrap {
	w := &TapWrap{Content: content, OnTapped: onTapped}
	w.ExtendBaseWidget(w)
	return w
}

// CreateRenderer implements fyne.Widget.
func (w *TapWrap) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.Content)
}

// Tapped implements fyne.Tappable.
func (w *TapWrap) Tapped(_ *fyne.PointEvent) {
	if w.OnTapped != nil {
		w.OnTapped()
	}
}

// Cursor implements desktop.Cursorable (как у ссылки — зона кликабельна).
func (w *TapWrap) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}
