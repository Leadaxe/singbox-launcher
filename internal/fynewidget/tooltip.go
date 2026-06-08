package fynewidget

import "fyne.io/fyne/v2"

// SetToolTipSafe sets a tooltip on o if its concrete type implements
// SetToolTip(string) (e.g. github.com/dweymouth/fyne-tooltip widgets). For
// objects that do not support tooltips — including a nil o — it is a no-op.
//
// This consolidates the repeated inline pattern:
//
//	if tb, ok := interface{}(o).(interface{ SetToolTip(string) }); ok {
//		tb.SetToolTip(text)
//	}
//
// Note: callers that need to skip empty tooltip text must guard the text
// themselves; SetToolTipSafe forwards text verbatim (including "").
func SetToolTipSafe(o fyne.CanvasObject, text string) {
	if tb, ok := interface{}(o).(interface{ SetToolTip(string) }); ok {
		tb.SetToolTip(text)
	}
}
