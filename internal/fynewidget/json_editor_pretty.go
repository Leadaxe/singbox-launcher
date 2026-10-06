//go:build go1.26

// File json_editor_pretty.go — JSON view/editor on go-fyne-pretty-view (fresh
// builds only, see json_editor.go for the split).
//
// # Why onChanged settles immediately
//
// pretty-view debounces its OnChanged (400 ms by default): a human who types
// a value and clicks "Add" within that window would be saved from the FIELDS,
// not from the JSON they just edited, because the dirty flag had not fired
// yet. A negative debounce makes it settle on every keystroke; node bodies
// are small and the reparse is cheap.
package fynewidget

import (
	"fyne.io/fyne/v2"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"
)

// NewJSONView returns a read-only JSON view with highlighting, line numbers,
// folding and mouse selection (Ctrl/Cmd+C and the context menu copy).
func NewJSONView(text string) fyne.CanvasObject {
	return prettyview.NewWithData([]byte(text), prettyview.FormatJSON, prettyview.WithLineNumbers())
}

// NewJSONEditor returns an editable JSON area with live highlighting.
func NewJSONEditor(text string) JSONEditor {
	pv := prettyview.New(
		prettyview.WithFormat(prettyview.FormatJSON),
		prettyview.WithEditable(),
		prettyview.WithLineNumbers(),
		prettyview.WithInputConfig(prettyview.InputConfig{DebounceFor: -1}),
	)
	pv.SetText(text)
	return &prettyEditor{pv: pv}
}

type prettyEditor struct {
	pv *prettyview.PrettyView
}

func (e *prettyEditor) Object() fyne.CanvasObject { return e.pv }

// Text returns the live buffer. PrettyView.Text() would re-serialize the
// parsed document instead, hiding exactly the syntax errors the caller is
// about to report.
func (e *prettyEditor) Text() string { return string(e.pv.Source()) }

func (e *prettyEditor) SetText(s string) { e.pv.SetText(s) }

func (e *prettyEditor) SetOnChanged(fn func(string)) { e.pv.SetOnChanged(fn) }
