package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

var (
	_ fyne.Tappable          = (*SecondaryTapWrap)(nil)
	_ fyne.SecondaryTappable = (*SecondaryTapWrap)(nil)
	_ desktop.Mouseable      = (*SecondaryTapWrap)(nil)
	_ desktop.Hoverable      = (*SecondaryTapWrap)(nil)
)

// SecondaryTapWrap is a thin widget around content that receives secondary (right) taps.
// Primary taps on non-button areas hit this widget before the List row (Fyne hit-test
// prefers SecondaryTappable here); OnPrimary runs so the row can still be selected.
//
// Tooltip: the wrap embeds ttwidget.ToolTipWidget, so SetToolTip on it shows the
// text anywhere over the wrapped row — the row's own children are canvas objects
// with no hover of their own, and giving each of them a tooltip would mean one
// tooltip per column instead of one per row. Empty text = no tooltip at all.
type SecondaryTapWrap struct {
	ttwidget.ToolTipWidget

	Content fyne.CanvasObject

	// lastPrimaryPressMods is set on MouseDown (primary); Tapped passes it to OnPrimary
	// so Ctrl/Shift survive until click release (CurrentKeyModifiers on tap-up can be wrong on Windows).
	lastPrimaryPressMods fyne.KeyModifier

	// OnPrimary is invoked on left-click when this widget receives the primary tap
	// (e.g. label / padding in a List row). Modifier is from the press event; use 0 if unknown.
	OnPrimary func(mods fyne.KeyModifier)

	// OnSecondary is invoked on right-click / long-press secondary tap.
	OnSecondary func(*fyne.PointEvent)
}

// NewSecondaryTapWrap wraps inner content for TappedSecondary handling.
func NewSecondaryTapWrap(inner fyne.CanvasObject) *SecondaryTapWrap {
	w := &SecondaryTapWrap{Content: inner}
	w.ExtendBaseWidget(w)
	return w
}

// CreateRenderer implements fyne.Widget.
func (w *SecondaryTapWrap) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.Content)
}

// MouseDown implements desktop.Mouseable (capture modifiers at press for reliable Ctrl/Shift+click).
func (w *SecondaryTapWrap) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonPrimary {
		w.lastPrimaryPressMods = e.Modifier
	}
}

// MouseUp implements desktop.Mouseable.
func (w *SecondaryTapWrap) MouseUp(_ *desktop.MouseEvent) {}

// Tapped implements fyne.Tappable (primary tap on the wrap itself, not on child buttons).
func (w *SecondaryTapWrap) Tapped(_ *fyne.PointEvent) {
	mods := w.lastPrimaryPressMods
	w.lastPrimaryPressMods = 0
	if w.OnPrimary != nil {
		w.OnPrimary(mods)
	}
}

// TappedSecondary implements fyne.SecondaryTappable.
func (w *SecondaryTapWrap) TappedSecondary(pe *fyne.PointEvent) {
	if w.OnSecondary != nil {
		w.OnSecondary(pe)
	}
}
