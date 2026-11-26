package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

// legendGap — поле подложки шапки по бокам: линия контура «рвётся» с отступом
// от текста и кнопок, а не впритык.
const legendGap = 2

// NewLegendPanel — рамка со скруглённым контуром, шапка которой лежит на
// верхней линии контура и разрывает её, как legend у fieldset:
//
//	╭─ left · note ───────────── right ─╮
//	│ content                           │
//	╰───────────────────────────────────╯
//
// left — кластер слева (название и т.п.), note — необязательная пометка за
// ним: занимает ширину по тексту, а когда её нет, усекается; right — кластер
// справа (крестик). Линия проходит по вертикальной середине строки шапки.
//
// Заливки у рамки нет: под ней фон родителя, и подложки шапки (непрозрачные,
// цвета фона темы) совпадают с ним по обе стороны линии — шва нет. Вложенные
// рамки так же прозрачны, поэтому подложка на любой глубине того же цвета;
// вложенность читается по контурам.
func NewLegendPanel(left fyne.CanvasObject, note *widget.Label, right, content fyne.CanvasObject) *fyne.Container {
	outline := canvas.NewRectangle(color.Transparent)
	outline.CornerRadius = theme.InputRadiusSize()
	leftBg := canvas.NewRectangle(color.Transparent)
	rightBg := canvas.NewRectangle(color.Transparent)
	var noteObj fyne.CanvasObject = layout.NewSpacer()
	if note != nil {
		note.Truncation = fyne.TextTruncateEllipsis
		noteObj = note
	}
	l := &legendPanelLayout{outline: outline, leftBg: leftBg, rightBg: rightBg, left: left, note: note, noteObj: noteObj, right: right}
	l.content = container.NewPadded(content)
	return container.New(l, outline, leftBg, rightBg, left, noteObj, right, l.content)
}

type legendPanelLayout struct {
	outline, leftBg, rightBg *canvas.Rectangle
	left, noteObj, right     fyne.CanvasObject
	note                     *widget.Label
	content                  fyne.CanvasObject
}

func (l *legendPanelLayout) headHeight() float32 {
	h := l.left.MinSize().Height
	if r := l.right.MinSize().Height; r > h {
		h = r
	}
	if l.note != nil {
		if n := l.note.MinSize().Height; n > h {
			h = n
		}
	}
	return h
}

// inset — отступ шапки от боковых краёв: за скруглением угла.
func (l *legendPanelLayout) inset() float32 {
	return theme.InputRadiusSize() + theme.Padding()
}

func (l *legendPanelLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	hh := l.headHeight()
	head := l.left.MinSize().Width + l.right.MinSize().Width + 2*l.inset() + 4*legendGap
	c := l.content.MinSize()
	w := c.Width + 2*listPanelStroke
	if head > w {
		w = head
	}
	return fyne.NewSize(w, hh+c.Height+listPanelStroke)
}

func (l *legendPanelLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	hh := l.headHeight()
	top := hh / 2

	// Цвета берутся при раскладке, а не при создании: смена темы меняет их.
	l.outline.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	l.outline.StrokeWidth = listPanelStroke
	l.outline.Move(fyne.NewPos(0, top))
	l.outline.Resize(fyne.NewSize(size.Width, size.Height-top))
	l.outline.Refresh()

	bg := theme.Color(theme.ColorNameBackground)
	inset := l.inset()

	rw := l.right.MinSize().Width
	rx := size.Width - inset - rw
	l.right.Move(fyne.NewPos(rx, 0))
	l.right.Resize(fyne.NewSize(rw, hh))
	l.rightBg.FillColor = bg
	l.rightBg.Move(fyne.NewPos(rx-legendGap, 0))
	l.rightBg.Resize(fyne.NewSize(rw+2*legendGap, hh))
	l.rightBg.Refresh()

	lx := inset + legendGap
	lw := l.left.MinSize().Width
	l.left.Move(fyne.NewPos(lx, 0))
	l.left.Resize(fyne.NewSize(lw, hh))

	nw := float32(0)
	if l.note != nil && l.note.Visible() {
		want := fyne.MeasureText(l.note.Text, theme.TextSize(), l.note.TextStyle).Width + 2*theme.InnerPadding()
		avail := rx - 2*legendGap - (lx + lw)
		nw = want
		if avail < nw {
			nw = avail
		}
		if nw < 0 {
			nw = 0
		}
	}
	l.noteObj.Move(fyne.NewPos(lx+lw, 0))
	l.noteObj.Resize(fyne.NewSize(nw, hh))
	l.leftBg.FillColor = bg
	l.leftBg.Move(fyne.NewPos(lx-legendGap, 0))
	l.leftBg.Resize(fyne.NewSize(lw+nw+2*legendGap, hh))
	l.leftBg.Refresh()

	l.content.Move(fyne.NewPos(listPanelStroke, hh))
	l.content.Resize(fyne.NewSize(size.Width-2*listPanelStroke, size.Height-hh-listPanelStroke))
}
