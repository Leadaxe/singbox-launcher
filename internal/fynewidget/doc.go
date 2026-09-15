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
// fires from the window's first Show(). GLFW re-reads its monitor list only at startup and when AppKit
// reports a screen change, so right after the display wakes up the list can still be empty; the wrapper
// checks both CoreGraphics and GLFW.
//
// # The NSApplication delegate (macOS)
//
// GLFW keeps its monitor list fresh through its own NSApplication delegate, and an app has only one.
// The launcher's delegate (platform.SetupDockReopenHandler) must therefore wrap GLFW's and forward
// every call it does not handle itself. Replacing it outright silently loses the screen-change refresh,
// and without an applicationShouldTerminate: of its own Cmd+Q, Dock Quit and log out end the process at
// once, skipping GracefulExit. What GLFW's delegate does is listed in that function's comment.
package fynewidget
