// File chip.go — КОМПАКТНЫЙ чип-тоггл: короткая подпись в скруглённой плашке.
//
// Зачем свой виджет, а не widget.Button. У кнопки темы минимальный размер
// считается от InnerPadding + высоты строки обычного кегля, и ряд из полусотни
// чипов «🇩🇪 24» занимает экран. Чип меряет себя ровно по тексту капитального
// кегля плюс свои отступы (≈6pt по горизонтали, ≈2pt по вертикали) и выходит
// вдвое ниже и втрое уже.
//
// Состояний ровно два — выбран/нет, — и рисуются они цветом плашки, а не
// рамкой: ряд чипов читается как «что отобрано», и обводка на невыбранных
// давала бы сетку, в которой выбранное не видно.
//
// Тултип и наведение приходят от ToolTipWidget (fyne-tooltip): он же базовый
// widget.BaseWidget, и MouseIn/MouseOut у него уже разведены — чипу остаётся
// подмешать в них свою перекраску. Тултип показывается только в окне, куда
// добавлен слой fynetooltip.AddWindowToolTipLayer.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

var (
	_ fyne.Widget       = (*Chip)(nil)
	_ fyne.Tappable     = (*Chip)(nil)
	_ desktop.Hoverable = (*Chip)(nil)
)

// Отступы плашки. Горизонтальный заметно больше вертикального: чип должен
// читаться как «таблетка», а не как квадратная кнопка.
const (
	chipPadH = 6
	chipPadV = 2
	// chipMinHeight — пол высоты, чтобы соседние чипы стояли ровно даже когда
	// в одном только цифры, а в другом эмодзи с другой метрикой шрифта.
	chipMinHeight = 20
)

// Chip — однострочная плашка-тоггл с необязательным тултипом.
type Chip struct {
	ttwidget.ToolTipWidget

	// OnChanged зовётся ПОСЛЕ смены состояния кликом. Программная установка
	// (SetSelected) его не зовёт — иначе восстановление снимка состояния
	// перезаписывало бы то, что восстанавливает.
	OnChanged func(selected bool)

	text     string
	selected bool
	hovered  bool

	bg    *canvas.Rectangle
	label *canvas.Text
}

// NewChip создаёт чип с подписью text.
func NewChip(text string, selected bool, onChanged func(bool)) *Chip {
	c := &Chip{text: text, selected: selected, OnChanged: onChanged}
	c.ExtendBaseWidget(c)
	return c
}

// CreateRenderer implements fyne.Widget.
func (c *Chip) CreateRenderer() fyne.WidgetRenderer {
	c.bg = canvas.NewRectangle(color.Transparent)
	c.label = canvas.NewText(c.text, color.Black)
	c.label.TextSize = c.captionSize()
	c.applyStyle()
	return &chipRenderer{chip: c}
}

// Selected — выбран ли чип сейчас.
func (c *Chip) Selected() bool { return c.selected }

// SetSelected ставит состояние БЕЗ вызова OnChanged.
func (c *Chip) SetSelected(on bool) {
	if c.selected == on {
		return
	}
	c.selected = on
	c.applyStyle()
}

// SetText меняет подпись (пересчитывает и минимальный размер).
func (c *Chip) SetText(text string) {
	if c.text == text {
		return
	}
	c.text = text
	if c.label != nil {
		c.label.Text = text
	}
	c.Refresh()
}

// Tapped implements fyne.Tappable.
func (c *Chip) Tapped(*fyne.PointEvent) {
	c.selected = !c.selected
	c.applyStyle()
	if c.OnChanged != nil {
		c.OnChanged(c.selected)
	}
}

// MouseIn implements desktop.Hoverable (тултип + подсветка).
func (c *Chip) MouseIn(e *desktop.MouseEvent) {
	c.ToolTipWidget.MouseIn(e)
	c.hovered = true
	c.applyStyle()
}

// MouseOut implements desktop.Hoverable.
func (c *Chip) MouseOut() {
	c.ToolTipWidget.MouseOut()
	c.hovered = false
	c.applyStyle()
}

// MouseMoved implements desktop.Hoverable.
func (c *Chip) MouseMoved(e *desktop.MouseEvent) { c.ToolTipWidget.MouseMoved(e) }

// Refresh перечитывает тему (смена варианта светлая/тёмная).
func (c *Chip) Refresh() {
	if c.label != nil {
		c.label.TextSize = c.captionSize()
	}
	c.applyStyle()
	c.BaseWidget.Refresh()
}

func (c *Chip) captionSize() float32 {
	return c.Theme().Size(theme.SizeNameCaptionText)
}

// applyStyle перекрашивает плашку и текст под текущее состояние.
func (c *Chip) applyStyle() {
	if c.bg == nil || c.label == nil {
		return
	}
	th := c.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	c.bg.CornerRadius = th.Size(theme.SizeNameSelectionRadius)
	switch {
	case c.selected:
		c.bg.FillColor = th.Color(theme.ColorNamePrimary, v)
		// Текст выбранного — цветом надписи на кнопке-акценте: у тёмной темы
		// обычный ForegroundColor на синей плашке читается плохо.
		c.label.Color = th.Color(theme.ColorNameForegroundOnPrimary, v)
	case c.hovered:
		c.bg.FillColor = th.Color(theme.ColorNameHover, v)
		c.label.Color = th.Color(theme.ColorNameForeground, v)
	default:
		c.bg.FillColor = th.Color(theme.ColorNameInputBackground, v)
		c.label.Color = th.Color(theme.ColorNameForeground, v)
	}
	c.bg.Refresh()
	c.label.Refresh()
	canvas.Refresh(c)
}

type chipRenderer struct {
	chip *Chip
}

func (r *chipRenderer) Layout(size fyne.Size) {
	r.chip.bg.Resize(size)
	r.chip.bg.Move(fyne.NewPos(0, 0))
	ts := r.chip.label.MinSize()
	// Текст по центру плашки: при клампе высоты до chipMinHeight остаток
	// делится поровну, иначе подпись липла бы к верхнему краю.
	r.chip.label.Resize(ts)
	r.chip.label.Move(fyne.NewPos(
		(size.Width-ts.Width)/2,
		(size.Height-ts.Height)/2,
	))
}

func (r *chipRenderer) MinSize() fyne.Size {
	ts := r.chip.label.MinSize()
	h := ts.Height + 2*chipPadV
	if h < chipMinHeight {
		h = chipMinHeight
	}
	return fyne.NewSize(ts.Width+2*chipPadH, h)
}

func (r *chipRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.chip.bg, r.chip.label}
}

func (r *chipRenderer) Refresh() {
	r.chip.label.Text = r.chip.text
	r.chip.applyStyle()
	canvas.Refresh(r.chip)
}

func (r *chipRenderer) Destroy() {}
