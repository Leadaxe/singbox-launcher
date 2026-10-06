// File json_editor.go — JSON text areas shared by the node windows.
//
// # Two implementations behind one API
//
// Fresh builds (Go 1.26+, Fyne 2.8) use the pretty-view editor: syntax
// highlighting, line numbers, folding, search, mouse selection and copy —
// see json_editor_pretty.go. The Win7 build is pinned to go1.21 / Fyne 2.7.3
// and cannot compile that library, so it keeps the plain multi-line Entry it
// always had — see json_editor_legacy.go. The split is a Go release build
// constraint (`go1.26`), not a custom tag: the Win7 toolchain excludes the
// pretty-view file on its own, and `go get -modfile=go.win7.mod` never sees
// the import.
//
// Callers never touch the concrete widget: they get a [JSONEditor] (editable
// tabs) or a bare canvas object (read-only tabs) and work with text only.
package fynewidget

import "fyne.io/fyne/v2"

// JSONEditor is an editable JSON text area.
//
// Object returns the widget to place in a container (the editor itself is
// not a CanvasObject on purpose: wrapping a Fyne widget in another struct
// breaks renderer lookup, see doc.go). Text returns the live buffer exactly
// as typed, which is what json.Unmarshal must see. OnChanged fires on human
// edits only, never on SetText.
type JSONEditor interface {
	Object() fyne.CanvasObject
	Text() string
	SetText(string)
	SetOnChanged(func(string))
}
