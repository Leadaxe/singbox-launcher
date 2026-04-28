package tabs

import (
	"fyne.io/fyne/v2"
)

// tightVBox — вертикальная компоновка без theme.Padding между элементами.
// Используется для title/subtitle строк source-row, чтобы они визуально
// "прилипали" друг к другу (стандартный VBox даёт ~12px воздуха).
type tightVBox struct{}

func (tightVBox) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		if ms.Width > w {
			w = ms.Width
		}
		h += ms.Height
	}
	return fyne.NewSize(w, h)
}

func (tightVBox) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var y float32
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		o.Resize(fyne.NewSize(size.Width, ms.Height))
		o.Move(fyne.NewPos(0, y))
		y += ms.Height
	}
}
