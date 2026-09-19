// File badge_dot.go — точка-индикатор в правом верхнем углу виджета.
//
// Приём для «у этой кнопки есть невидимое отсюда состояние»: фильтр списка
// отбирает, у вкладки есть непрочитанное. Подсветка всей кнопки для этого не
// годится — она читается как «нажата» и спорит с обычной важностью соседей в
// ряду, а на кнопке с иконкой темы вдобавок перекрашивает сам глиф.
//
// Точка НЕ перехватывает клики (canvas.Circle не Tappable, и слой с ней не
// содержит ничего интерактивного) и НЕ меняет минимальный размер обёртки:
// Stack меряет себя по самому большому вложенному объекту, а слой точки
// собран из растяжимых прокладок, чей MinSize нулевой.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
)

// badgeDotSize — диаметр точки в точках экрана.
//
// Меньше 6 сливается с глифом кнопки, больше 9 выглядит наклейкой поверх неё.
const badgeDotSize = 8

// badgeDotInset — насколько точка утоплена от краёв виджета, чтобы не висеть
// на самой кромке кнопки.
const badgeDotInset = 1

// BadgeDot — обёртка «виджет + точка в правом верхнем углу».
type BadgeDot struct {
	// Container — то, что кладут в раскладку вместо исходного виджета.
	Container *fyne.Container

	dot *canvas.Circle
}

// NewBadgeDot оборачивает content точкой цвета темы colorName (обычно
// theme.ColorNameWarning — жёлто-оранжевый). Точка спрятана до SetVisible.
func NewBadgeDot(content fyne.CanvasObject, colorName fyne.ThemeColorName) *BadgeDot {
	b := &BadgeDot{}
	b.dot = canvas.NewCircle(theme.Color(colorName))
	b.dot.Resize(fyne.NewSize(badgeDotSize, badgeDotSize))
	b.dot.Hide()

	// Точка прижата вправо-вверх распорками: HBox со спейсером слева даёт
	// правый край, VBox со спейсером снизу — верхний. GridWrap держит её
	// размер, иначе растяжимый слой раздул бы круг во всю кнопку.
	sized := container.NewGridWrap(fyne.NewSize(badgeDotSize, badgeDotSize), b.dot)
	corner := container.NewVBox(
		container.NewHBox(layout.NewSpacer(), sized,
			newFixedSpacer(badgeDotInset, 0)),
		layout.NewSpacer(),
	)
	b.Container = container.NewStack(content, corner)
	return b
}

// SetVisible показывает или прячет точку.
func (b *BadgeDot) SetVisible(on bool) {
	if b == nil || b.dot == nil {
		return
	}
	if on {
		b.dot.Show()
	} else {
		b.dot.Hide()
	}
	b.dot.Refresh()
}

// newFixedSpacer — прозрачная прокладка фиксированного размера.
func newFixedSpacer(w, h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}
