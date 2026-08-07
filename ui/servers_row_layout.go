package ui

import "fyne.io/fyne/v2"

// SPEC 095 — вертикальная укладка «имя + подзаголовок» без зазора.
//
// container.NewVBox ставит между элементами theme.Padding() (обычно 4pt), и
// подзаголовок отрывается от имени: строка читается как две независимые, а не
// как заголовок с пояснением. Свой layout кладёт элементы вплотную.
type tightVBoxLayout struct {
	// gap — зазор между элементами. 0 = вплотную.
	gap float32
}

// Layout расставляет элементы сверху вниз, каждый на своей минимальной высоте.
func (l tightVBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		h := o.MinSize().Height
		o.Resize(fyne.NewSize(size.Width, h))
		o.Move(fyne.NewPos(0, y))
		y += h + l.gap
	}
}

// MinSize — сумма высот с зазорами и максимальная ширина.
func (l tightVBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var width, height float32
	visible := 0
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		m := o.MinSize()
		if m.Width > width {
			width = m.Width
		}
		height += m.Height
		visible++
	}
	if visible > 1 {
		height += l.gap * float32(visible-1)
	}
	return fyne.NewSize(width, height)
}
