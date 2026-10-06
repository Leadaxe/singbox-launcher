//go:build !go1.26

// File json_editor_legacy.go — JSON view/editor on the plain multi-line Entry
// (Win7 build: go1.21 / Fyne 2.7.3, see json_editor.go for the split).
package fynewidget

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// NewJSONView returns a multi-line Entry with the text, no wrapping.
func NewJSONView(text string) fyne.CanvasObject {
	e := widget.NewMultiLineEntry()
	e.SetText(text)
	e.Wrapping = fyne.TextWrapOff
	return e
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
