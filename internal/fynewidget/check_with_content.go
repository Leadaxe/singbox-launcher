// Package fynewidget holds small reusable Fyne UI building blocks (not full screens).
package fynewidget

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// checkLeadingOverlap — насколько уменьшаем заявленную ширину пустой галки (см. trailingShrinkLayout).
const checkLeadingOverlap float32 = 15

// CheckWithContent groups an empty-label Check and a content area that toggles
// the check on tap and mirrors hover onto the check (so the row reads as one control).
//
// Typical layout:
//
//	container.NewBorder(nil, nil, c.CheckLeading, trailing, c.Content)
//
// CheckLeading — та же галка в контейнере с trailingShrinkLayout (заявленная ширина меньше, правый край
// может заходить на Content). Disable/SetChecked — только на поле Check.
type CheckWithContent struct {
	Check        *widget.Check
	Content      fyne.CanvasObject
	CheckLeading fyne.CanvasObject
}

// CheckWithContentConfig configures NewCheckWithContent.
type CheckWithContentConfig struct {
	// ContentToolTip is applied when content implements SetToolTip(string) (e.g. *ttwidget.Label).
	ContentToolTip string
}

// NewCheckWithContent creates a check with an empty caption and wraps content so that:
//   - primary/secondary tap on the content area toggles the check (unless disabled);
//   - hover highlights the check: for *ttwidget.Label, OnMouseIn/Moved/Out are wired (it captures
//     desktop.Hoverable); otherwise the wrapper implements desktop.Hoverable.
//
// If content is nil, an empty widget.Label is used.
func NewCheckWithContent(onChanged func(bool), content fyne.CanvasObject, cfg CheckWithContentConfig) *CheckWithContent {
	if content == nil {
		content = widget.NewLabel("")
	}
	ch := widget.NewCheck("", onChanged)
	wrapped := newCheckContentWrap(ch, content, strings.TrimSpace(cfg.ContentToolTip))
	checkLeading := CheckLeadingWrap(ch)

	return &CheckWithContent{Check: ch, Content: wrapped, CheckLeading: checkLeading}
}

// CheckLeadingWrap оборачивает пустую галку так же, как поле CheckLeading у CheckWithContent.
func CheckLeadingWrap(ch *widget.Check) fyne.CanvasObject {
	if ch == nil {
		return nil
	}
	return container.New(trailingShrinkLayout{overlap: checkLeadingOverlap}, ch)
}

// trailingShrinkLayout: MinSize по ширине = child.MinSize().Width − overlap; в Layout child получает полную ширину
// (может вылезать вправо под соседа).
type trailingShrinkLayout struct {
	overlap float32
}

func (l trailingShrinkLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	ms := objects[0].MinSize()
	w := ms.Width - l.overlap
	if w < 1 {
		w = 1
	}
	return fyne.NewSize(w, ms.Height)
}

func (l trailingShrinkLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	ch := objects[0]
	ms := ch.MinSize()
	h := fyne.Max(ms.Height, size.Height)
	ch.Resize(fyne.NewSize(ms.Width, h))
	ch.Move(fyne.NewPos(0, (size.Height-h)/2))
}

// --- internal wrap ---

type checkContentWrap struct {
	widget.BaseWidget

	check   *widget.Check
	child   fyne.CanvasObject
	ttLabel *ttwidget.Label // when set, hover is driven by label callbacks only
}

func newCheckContentWrap(check *widget.Check, content fyne.CanvasObject, contentToolTip string) fyne.CanvasObject {
	w := &checkContentWrap{check: check, child: content}
	if lbl, ok := content.(*ttwidget.Label); ok {
		w.ttLabel = lbl
	}
	w.ExtendBaseWidget(w)

	if contentToolTip != "" {
		if tb, ok := interface{}(content).(interface{ SetToolTip(string) }); ok {
			tb.SetToolTip(contentToolTip)
		}
	}

	if w.ttLabel != nil {
		w.ttLabel.OnMouseIn = func(*desktop.MouseEvent) { forwardCheckHover(check, true) }
		w.ttLabel.OnMouseMoved = func(*desktop.MouseEvent) { forwardCheckHover(check, true) }
		w.ttLabel.OnMouseOut = func() { forwardCheckHover(check, false) }
	}

	return w
}

func (w *checkContentWrap) MinSize() fyne.Size {
	if w.child == nil {
		return fyne.NewSize(0, 0)
	}
	return w.child.MinSize()
}

func (w *checkContentWrap) Tapped(*fyne.PointEvent) {
	if w.check == nil || w.check.Disabled() {
		return
	}
	w.check.SetChecked(!w.check.Checked)
}

func (w *checkContentWrap) TappedSecondary(e *fyne.PointEvent) { w.Tapped(e) }

func (w *checkContentWrap) Cursor() desktop.Cursor {
	if w.check != nil && !w.check.Disabled() {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (w *checkContentWrap) MouseIn(*desktop.MouseEvent) {
	if w.ttLabel != nil {
		return
	}
	forwardCheckHover(w.check, true)
}

func (w *checkContentWrap) MouseMoved(*desktop.MouseEvent) {
	if w.ttLabel != nil {
		return
	}
	forwardCheckHover(w.check, true)
}

func (w *checkContentWrap) MouseOut() {
	if w.ttLabel != nil {
		return
	}
	forwardCheckHover(w.check, false)
}

func (w *checkContentWrap) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.child)
}

func forwardCheckHover(check *widget.Check, hover bool) {
	if check == nil || check.Disabled() {
		return
	}
	h, ok := interface{}(check).(desktop.Hoverable)
	if !ok {
		return
	}
	if hover {
		h.MouseIn(&desktop.MouseEvent{
			PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, 0)},
		})
		return
	}
	h.MouseOut()
}
