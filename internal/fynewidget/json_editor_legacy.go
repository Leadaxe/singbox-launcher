//go:build !go1.26

// File json_editor_legacy.go — JSON view/editor on the plain multi-line Entry
// (Win7 build: go1.21 / Fyne 2.7.3, see json_editor.go for the split).
package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// NewJSONView returns a multi-line Entry with the text, no wrapping. It is
// read-only the way the source window always did it: OnChanged rolls any
// input back to the last set text (Disable() is not an option — on macOS
// disabled text renders in the background color).
func NewJSONView(text string) JSONView {
	v := &entryView{e: widget.NewMultiLineEntry()}
	v.e.Wrapping = fyne.TextWrapOff
	v.e.OnChanged = func(s string) {
		if s != v.last {
			v.e.SetText(v.last)
		}
	}
	v.SetText(text)
	return v
}

type entryView struct {
	e    *widget.Entry
	last string
}

func (v *entryView) Object() fyne.CanvasObject { return v.e }

func (v *entryView) SetText(s string) {
	v.last = s
	v.e.SetText(s)
}

// NewJSONEditor returns an editable multi-line Entry, no wrapping.
func NewJSONEditor(text string) JSONEditor {
	e := widget.NewMultiLineEntry()
	e.Wrapping = fyne.TextWrapOff
	e.SetText(text)
	return &entryEditor{e: e}
}

type entryEditor struct {
	e *widget.Entry
}

func (e *entryEditor) Object() fyne.CanvasObject { return e.e }

func (e *entryEditor) Text() string { return e.e.Text }

func (e *entryEditor) SetText(s string) { e.e.SetText(s) }

func (e *entryEditor) SetOnChanged(fn func(string)) { e.e.OnChanged = fn }
