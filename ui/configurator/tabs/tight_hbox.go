package tabs

import (
	"fyne.io/fyne/v2"
)

// rowIconGap — зазор между иконками в кластерах source-row. Отрицательный,
// чтобы «подтянуть» кнопки друг к другу: стандартный HBox добавляет
// theme.Padding (~4px) между элементами, а у icon-кнопок есть собственный
// внутренний padding (~8px с каждой стороны) — суммарно между иконками
// набегало ~20px воздуха. Отрицательный gap компенсирует часть внутреннего
// padding, оставляя визуально плотный, но всё ещё кликабельный ряд.
const rowIconGap float32 = -8

// tightHBox — горизонтальная компоновка с настраиваемым (в т.ч. отрицательным)
// зазором между элементами вместо стандартного theme.Padding. Аналог
// tightVBox, но по горизонтали; используется для кластеров иконок source-row.
//
// MinSize и Layout используют одну формулу (sum(widths) + spacing*(n-1)),
// поэтому ширина, которую контейнер занимает как left/right в Border,
// совпадает с реальной раскладкой — выравнивание subtitle через
// leftLead.MinSize().Width не ломается.
type tightHBox struct {
	spacing float32
}

func (l tightHBox) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	visible := 0
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		w += ms.Width
		if ms.Height > h {
			h = ms.Height
		}
		visible++
	}
	if visible > 1 {
		w += l.spacing * float32(visible-1)
	}
	if w < 0 {
		w = 0
	}
	return fyne.NewSize(w, h)
}

func (l tightHBox) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var x float32
	first := true
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		if !first {
			x += l.spacing
		}
		o.Resize(fyne.NewSize(ms.Width, size.Height))
		o.Move(fyne.NewPos(x, 0))
		x += ms.Width
		first = false
	}
}
