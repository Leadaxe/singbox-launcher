// File scroll_gutter.go — shared scrollbar-gutter helpers used by every
// scrollable container in the launcher UI.
//
// **Why a single source of truth?** Three packages (`ui`,
// `ui/configurator/tabs`, `ui/configurator/dialogs`) used to each declare
// their own `scrollbarGutterWidth = 10` constant with a "duplicated
// because different package, not worth a shared helper" comment. Once a
// fourth package (`ui/settings_window.go`) added the same constant, the
// "not worth it" calculation flipped — this file is now the canonical
// home, and the per-package copies are gone.
//
// **What the gutter does.** Fyne renders the vertical scrollbar as an
// overlay strip on the right edge of a `container.Scroll`. Without a
// reserved right margin, the scrollbar paints on top of the rightmost
// pixels of content (text, input borders, button right-edges). Reserving
// a fixed 10px strip *inside* the scroll viewport — between content and
// the scroll's right edge — lets the scrollbar sit in empty space.
//
// **Three call patterns** observed in the codebase:
//
//  1. Canonical wrap: `WrapInScrollWithGutter(content)` returns a Scroll
//     whose content already has the gutter on its right. Most new code
//     should use this.
//  2. Manual composition: `NewScrollGutter()` returns a bare
//     `*canvas.Rectangle` for callers that need to assemble the layout
//     themselves (e.g. a Border with custom top/bottom slots, or a
//     horizontal scroll combined with vertical content).
//  3. Row-level alignment: rows in a `widget.List` need the same 10px
//     reserved on the right so the row's controls line up with the
//     scrolled content above them. `NewScrollGutter()` covers this
//     case too — just drop it into the row's HBox/Border.
//
// **Gutter inside vs outside is a deliberate UI signal — not a bug.**
//
// The two layouts look similar but mean different things to the user:
//
//	Inside  (canonical, WrapInScrollWithGutter):
//	  scroll widget is flush to the parent's right edge;
//	  scrollbar overlays a 10px strip inside the viewport.
//	  Reads as: "the WHOLE page/window scrolls."
//	  Use for: a single scroll filling a window or tab body
//	  (Settings window, Edit dialogs that are one tall form).
//
//	Outside  (intentional in nested-scroll layouts):
//	  NewBorder(..., right: gutter, scroll) — entire scroll moves
//	  10px in from the parent's edge; an empty corridor separates
//	  scrollbar from the window edge.
//	  Reads as: "this SECTION has its own scroll, separate from the
//	  page around it." The corridor acts as a visual frame so the
//	  user doesn't mistake an inner list scroll for a page scroll.
//	  Use for: a scrollable list embedded in a larger form, like
//	  the Sources list in Config Wizard (header + URL input above,
//	  Close/Next bar below, scrollable list in the middle).
//
// Both patterns use NewScrollGutter() to build the spacer widget — only
// the slot they go into differs. Do NOT migrate one to the other "for
// consistency" — they communicate different scope semantics.
package components

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// ScrollbarGutterWidth — width in points of the reserved right strip
// under the vertical scrollbar. Must be >= the Fyne theme scrollBar size
// (SizeNameScrollBar = 12 in v2.7.x) or the bar paints over the rightmost
// content pixels. 10 was too narrow — the 12pt bar overhung fields by 2pt
// (visible on long inputs like the FakeIP IPv4/IPv6 range). 14 covers the
// bar plus a hair of breathing room.
const ScrollbarGutterWidth = 14

// NewScrollGutter returns a bare transparent rectangle with a fixed
// `ScrollbarGutterWidth`-wide minimum size. Drop it into any layout slot
// where you need to reserve the right margin under a scrollbar —
// `container.NewBorder(.., right: gutter, content)`, an HBox row
// trailer, or as a column in a grid.
//
// The rectangle is transparent (no visible pixel) — it exists purely
// for layout reservation.
func NewScrollGutter() *canvas.Rectangle {
	g := canvas.NewRectangle(color.Transparent)
	g.SetMinSize(fyne.NewSize(ScrollbarGutterWidth, 0))
	return g
}

// WrapInScrollWithGutter wraps `content` in a vertical Scroll whose
// viewport reserves `ScrollbarGutterWidth` on the right. This is the
// canonical "fit a tall column into a scrollable window without text
// being painted under the scrollbar" pattern.
//
// Composition: NewBorder(.., right: gutter, content) → NewScroll(...).
// The gutter lives *inside* the scroll, so the scroll itself sits flush
// to the parent's right edge while content stays clear of the scrollbar
// overlay.
//
// Returns a `*container.Scroll` so callers retain access to scroll-
// specific knobs (Direction, OnScrolled, Offset). If you need the
// raw `fyne.CanvasObject`, the implicit conversion at the call site
// is free.
func WrapInScrollWithGutter(content fyne.CanvasObject) *container.Scroll {
	inner := container.NewBorder(nil, nil, nil, NewScrollGutter(), content)
	return container.NewScroll(inner)
}

// NewListRowPadded — padding строки `widget.List` БЕЗ правой стороны.
//
// Строка списка отличается от прочих мест тем, что gutter лежит ВНУТРИ неё
// (см. call pattern 3 выше), а не отдельной колонкой снаружи. Обычный
// `container.NewPadded` добавляет отступ со всех четырёх сторон, и справа
// тот СКЛАДЫВАЕТСЯ с gutter'ом: получалось ~18pt вместо 14, и полоса
// прокрутки в списке выглядела заметно шире, чем на других экранах.
//
// Здесь правый отступ задаёт исключительно gutter, поэтому padding ставится
// только слева, сверху и снизу — зазор справа выходит ровно
// ScrollbarGutterWidth, тот же, что везде.
func NewListRowPadded(content fyne.CanvasObject) *fyne.Container {
	return container.New(listRowPadding{}, content)
}

// listRowPadding — layout: отступ темы со всех сторон, кроме правой.
type listRowPadding struct{}

// Layout сдвигает содержимое на padding слева и сверху, оставляя правый край
// нетронутым — там уже стоит gutter.
func (listRowPadding) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	p := theme.Padding()
	for _, o := range objects {
		if o == nil {
			continue
		}
		o.Move(fyne.NewPos(p, p))
		o.Resize(fyne.NewSize(size.Width-p, size.Height-2*p))
	}
}

// MinSize — минимальный размер содержимого плюс отступы (справа их нет).
func (listRowPadding) MinSize(objects []fyne.CanvasObject) fyne.Size {
	p := theme.Padding()
	var min fyne.Size
	for _, o := range objects {
		if o == nil {
			continue
		}
		min = min.Max(o.MinSize())
	}
	return fyne.NewSize(min.Width+p, min.Height+2*p)
}
