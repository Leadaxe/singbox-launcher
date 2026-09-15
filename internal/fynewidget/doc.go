// Package fynewidget provides small Fyne widgets and interaction helpers for the wizard UI.
//
// # Row hover (list-style rows)
//
// Wrap row content in [HoverRow] for a themed hover background (and optional selection tint via [HoverRowConfig]).
//
// Tooltip labels from github.com/dweymouth/fyne-tooltip handle hover internally and do not bubble to the
// parent; call [HoverRow.WireTooltipLabelHover] for each such label so the row still receives hover.
//
// Buttons, selects, and tooltip buttons need explicit forwarding: use [HoverForwardButton],
// [HoverForwardSelect], or [HoverForwardTTButton] with a [RowHoverGetter] that returns the row.
// Typical setup: declare var row *HoverRow, set rowGetter := func() *HoverRow { return row }, build
// children with that getter, then assign row = NewHoverRow(...).
//
// # Embedding Fyne widgets (critical)
//
// Do not wrap fyne’s widget.NewButton in another type that only stores *widget.Button: the inner button
// keeps its own BaseWidget and breaks hover/refresh on the real leaf.
//
// Embed widget.Button or github.com/dweymouth/fyne-tooltip/widget.Button by value on the outer struct and
// call ExtendBaseWidget once on the outer type so renderer and layout stay tied to the widget in the tree
// (avoids hover gaps and icon/text shifting on refresh).
//
// For a fyne-tooltip Button plus row hover, use [HoverForwardTTButton]. When code needs a *ttwidget.Button
// (e.g. async Disable/SetText), use [HoverForwardTTButton.TTWidget] — the outer type’s first field is the
// embedded value; the conversion follows unsafe.Pointer rules for the leading field.
//
// # Centering windows (critical)
//
// Never call fyne.Window.CenterOnScreen directly: use [CenterOnScreen]. Fyne 2.8.1 dereferences a nil
// monitor when GLFW sees no screen, which on macOS happens while the display is asleep, and the panic
// fires from the window's first Show(). In the launcher GLFW's monitor list is taken once at startup
// (the Dock reopen handler replaces GLFW's NSApplication delegate), so a launcher started with the
// display asleep keeps an empty list after the display wakes up.
package fynewidget
