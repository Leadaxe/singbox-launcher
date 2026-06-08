package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

var (
	_ fyne.Widget       = (*HoverRow)(nil)
	_ desktop.Hoverable = (*HoverRow)(nil)
)

// Row hover uses ColorNameBackground + a small share of ColorNamePrimary (blue-ish in default themes),
// not ColorNameHover (often gray). Weights are parts of 100; higher primary share = stronger cool tint.
const (
	hoverBlendBackgroundPercent = 95
	hoverBlendPrimaryPercent    = 5
)

// HoverRowConfig configures NewHoverRow: optional persistent selection tint (e.g. checkbox)
// plus themed hover when the pointer is over the row.
type HoverRowConfig struct {
	// IsSelected returns true when the row should show a selection background (e.g. library row checked).
	// If nil, only hover is applied.
	IsSelected func() bool
	// SelectedFill is used when IsSelected returns true. If nil, theme ColorNameSelection is used.
	SelectedFill *color.NRGBA
}

// HoverRow draws a list-like hover (and optional selection) background behind Content.
type HoverRow struct {
	widget.BaseWidget

	Content fyne.CanvasObject
	cfg     HoverRowConfig

	hovered bool
	bg      *canvas.Rectangle
}

// NewHoverRow wraps content with a subtle hover highlight; optional selection tint from cfg.
func NewHoverRow(content fyne.CanvasObject, cfg HoverRowConfig) *HoverRow {
	w := &HoverRow{Content: content, cfg: cfg}
	w.ExtendBaseWidget(w)
	return w
}

// CreateRenderer implements fyne.Widget.
func (w *HoverRow) CreateRenderer() fyne.WidgetRenderer {
	// BaseWidget is already extended in NewHoverRow (sole constructor for HoverRow).
	w.bg = canvas.NewRectangle(color.Transparent)
	w.bg.Hide()
	w.applyBackground()
	return widget.NewSimpleRenderer(container.NewStack(w.bg, w.Content))
}

// Refresh updates selection/hover visuals (e.g. after checkbox toggles without pointer move).
func (w *HoverRow) Refresh() {
	w.BaseWidget.Refresh()
	w.applyBackground()
}

// MouseIn implements desktop.Hoverable.
func (w *HoverRow) MouseIn(*desktop.MouseEvent) {
	w.hovered = true
	w.applyBackground()
}

// MouseMoved implements desktop.Hoverable.
func (w *HoverRow) MouseMoved(*desktop.MouseEvent) {}

// MouseOut implements desktop.Hoverable.
func (w *HoverRow) MouseOut() {
	w.hovered = false
	w.applyBackground()
}

// WireTooltipLabelHover forwards desktop hover from a fyne-tooltip Label to this row.
// The label handles hover for tooltips and would otherwise prevent the row from seeing MouseIn/MouseOut;
// chain the label’s OnMouseIn/OnMouseMoved/OnMouseOut to call this row’s handlers (previous callbacks preserved).
func (w *HoverRow) WireTooltipLabelHover(lbl *ttwidget.Label) {
	if lbl == nil {
		return
	}
	prevIn := lbl.OnMouseIn
	prevMoved := lbl.OnMouseMoved
	prevOut := lbl.OnMouseOut
	lbl.OnMouseIn = func(e *desktop.MouseEvent) {
		w.MouseIn(e)
		if prevIn != nil {
			prevIn(e)
		}
	}
	lbl.OnMouseMoved = func(e *desktop.MouseEvent) {
		w.MouseMoved(e)
		if prevMoved != nil {
			prevMoved(e)
		}
	}
	lbl.OnMouseOut = func() {
		w.MouseOut()
		if prevOut != nil {
			prevOut()
		}
	}
}

func (w *HoverRow) applyBackground() {
	if w.bg == nil {
		return
	}
	th := w.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	w.bg.CornerRadius = th.Size(theme.SizeNameSelectionRadius)

	sel := w.cfg.IsSelected != nil && w.cfg.IsSelected()
	switch {
	case sel:
		if w.cfg.SelectedFill != nil {
			w.bg.FillColor = *w.cfg.SelectedFill
		} else {
			w.bg.FillColor = th.Color(theme.ColorNameSelection, v)
		}
		w.bg.Show()
	case w.hovered:
		w.bg.FillColor = blendHoverWithBackground(th, v)
		w.bg.Show()
	default:
		w.bg.Hide()
	}
	w.bg.Refresh()
	canvas.Refresh(w)
}

func blendHoverWithBackground(th fyne.Theme, v fyne.ThemeVariant) color.NRGBA {
	b := th.Color(theme.ColorNameBackground, v)
	p := th.Color(theme.ColorNamePrimary, v)
	br, bg, bb, ba := b.RGBA()
	pr, pg, pb, pa := p.RGBA()
	wb := uint32(hoverBlendBackgroundPercent)
	wp := uint32(hoverBlendPrimaryPercent)
	mix := func(x, y uint32) uint8 {
		return uint8(((x*wb+y*wp)/100)>>8) & 0xff
	}
	return color.NRGBA{
		R: mix(br, pr),
		G: mix(bg, pg),
		B: mix(bb, pb),
		A: mix(ba, pa),
	}
}
