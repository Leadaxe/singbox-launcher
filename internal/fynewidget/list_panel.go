package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// listPanelStripeAlpha — прозрачность полосы зебры: цвет текста темы поверх
// подложки. Тон, а не контраст — строка остаётся строкой, а не плашкой.
const listPanelStripeAlpha = 10

// listPanelStroke — толщина контура панели. Отступа слева у панели нет:
// скруглённый контур и свой фон и так отделяют её от формы.
const listPanelStroke = 1

// NewListPanel кладёт список строк (VBox и подобное) на подложку с контуром и
// скруглёнными углами и красит каждую вторую видимую строку тоном фона.
//
// Сам список не меняется: его дети остаются его детьми, так что перетаскивание
// строк и геттеры строк работают как раньше. Полосы — отдельный слой под
// списком, их положение пересчитывается при каждой раскладке панели.
func NewListPanel(list *fyne.Container) *fyne.Container {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = theme.InputRadiusSize()
	stripes := container.NewWithoutLayout()
	return container.New(&listPanelLayout{bg: bg, stripes: stripes, list: list}, bg, stripes, list)
}

type listPanelLayout struct {
	bg      *canvas.Rectangle
	stripes *fyne.Container
	list    *fyne.Container
}

func (l *listPanelLayout) pad() float32 { return theme.Padding() }

func (l *listPanelLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	m := l.list.MinSize()
	return fyne.NewSize(m.Width+2*listPanelStroke, m.Height+2*l.pad())
}

func (l *listPanelLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	pad := l.pad()
	l.bg.FillColor = theme.Color(theme.ColorNameInputBackground)
	l.bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	l.bg.StrokeWidth = listPanelStroke
	l.bg.Move(fyne.NewPos(0, 0))
	l.bg.Resize(size)
	l.bg.Refresh()

	inner := fyne.NewSize(size.Width-2*listPanelStroke, size.Height-2*pad)
	origin := fyne.NewPos(listPanelStroke, pad)
	l.list.Move(origin)
	l.list.Resize(inner)
	l.stripes.Move(origin)
	l.stripes.Resize(inner)

	stripe := theme.Color(theme.ColorNameForeground)
	r, g, b, _ := stripe.RGBA()
	fill := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: listPanelStripeAlpha}

	used := 0
	visible := 0
	for _, row := range l.list.Objects {
		if row == nil || !row.Visible() {
			continue
		}
		visible++
		if visible%2 != 0 {
			continue
		}
		var rect *canvas.Rectangle
		if used < len(l.stripes.Objects) {
			rect, _ = l.stripes.Objects[used].(*canvas.Rectangle)
		}
		if rect == nil {
			rect = canvas.NewRectangle(fill)
			if used < len(l.stripes.Objects) {
				l.stripes.Objects[used] = rect
			} else {
				l.stripes.Objects = append(l.stripes.Objects, rect)
			}
		}
		rect.FillColor = fill
		rect.Move(row.Position())
		rect.Resize(row.Size())
		rect.Refresh()
		used++
	}
	l.stripes.Objects = l.stripes.Objects[:used]
}

// RefreshListPanel пересчитывает полосы панели после пересборки её списка:
// Container.Refresh раскладку не запускает, а число строк могло не измениться
// (тогда и минимум панели прежний, и холст её сам не переложит).
func RefreshListPanel(panel *fyne.Container) {
	if panel == nil || panel.Layout == nil {
		return
	}
	panel.Layout.Layout(panel.Objects, panel.Size())
	panel.Refresh()
}

// NewFramedPanel — тот же блок, что NewListPanel, но для произвольного
// содержимого (не списка строк): подложка с контуром и скруглёнными углами,
// содержимое с внутренним отступом, без зебры.
func NewFramedPanel(content fyne.CanvasObject) *fyne.Container {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = theme.InputRadiusSize()
	return container.New(&framedPanelLayout{bg: bg}, bg, container.NewPadded(content))
}

type framedPanelLayout struct{ bg *canvas.Rectangle }

func (l *framedPanelLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	m := objects[1].MinSize()
	return fyne.NewSize(m.Width+2*listPanelStroke, m.Height+2*listPanelStroke)
}

func (l *framedPanelLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Цвета берутся при раскладке, а не при создании: смена темы меняет их.
	l.bg.FillColor = theme.Color(theme.ColorNameInputBackground)
	l.bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	l.bg.StrokeWidth = listPanelStroke
	l.bg.Move(fyne.NewPos(0, 0))
	l.bg.Resize(size)
	l.bg.Refresh()
	objects[1].Move(fyne.NewPos(listPanelStroke, listPanelStroke))
	objects[1].Resize(fyne.NewSize(size.Width-2*listPanelStroke, size.Height-2*listPanelStroke))
}
