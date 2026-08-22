package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/fynewidget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// Shared row scaffolding for the reorderable lists in the Rules and DNS tabs.
//
// The Rules tab (custom + preset-ref) and the DNS tab (user + preset rules) each
// have their own row builder with distinct wiring (outbound-select lifecycle,
// model targets, SRS flows, delete paths — all genuinely different and kept
// separate on purpose). What was IDENTICAL across all of them is the visual
// scaffolding: the drag-grip + checkbox leading cluster, the edit/delete icon
// cluster, the tap-to-toggle label wrapper, and the Border→HoverRow→tooltip-hover
// tail.
//
// That scaffolding had silently drifted (a tightHBox spacing change landed in the
// custom builder but not the preset one, producing visibly inconsistent rows).
// Centralizing it here makes the spacing provably identical and structurally
// undriftable, while each builder keeps its own behavior.

// buildRowDragLead builds the leading cluster for a reorderable row: the drag
// grip, followed by the enable checkbox in its leading wrap. The grip reserves
// the width the old ↑/↓ pair held, so the label column keeps its position.
// handle == nil (системное правило, SPEC 106) — на месте захвата остаётся
// распорка той же ширины, иначе строка съезжает относительно соседних.
func buildRowDragLead(handle fyne.CanvasObject, check *widget.Check) *fyne.Container {
	if handle == nil {
		handle = fynewidget.NewDragHandleSpacer()
	}
	return container.NewHBox(
		handle,
		fynewidget.CheckLeadingWrap(check),
	)
}

// buildRowEditDelCluster packs the edit + delete action icons tight.
// del == nil — кластер из одной кнопки (системное правило не удаляется).
func buildRowEditDelCluster(edit, del fyne.CanvasObject) *fyne.Container {
	if del == nil {
		return container.New(tightHBox{spacing: rowIconGap}, edit)
	}
	return container.New(tightHBox{spacing: rowIconGap}, edit, del)
}

// newRowLabelToggleTap wraps a row label so a tap toggles the row's checkbox
// (no-op when the checkbox is disabled).
func newRowLabelToggleTap(label fyne.CanvasObject, check *widget.Check) *fynewidget.TapWrap {
	return fynewidget.NewTapWrap(label, func() {
		if check.Disabled() {
			return
		}
		check.SetChecked(!check.Checked)
	})
}

// finalizeRow assembles the shared row tail: the Border (leftLead | center |
// rightCluster), the HoverRow wrapper, the tooltip hover wiring for the row's
// label, and appends the row to its box. Returns the HoverRow so the caller can
// assign it to the variable its rowGetter closure captured — rows are built
// synchronously before display, so the assignment always happens before any
// hover event can fire.
func finalizeRow(box *fyne.Container, leftLead, rightCluster, center fyne.CanvasObject, hoverLabel *ttwidget.Label) *fynewidget.HoverRow {
	rowInner := container.NewBorder(nil, nil, leftLead, rightCluster, center)
	row := fynewidget.NewHoverRow(rowInner, fynewidget.HoverRowConfig{})
	row.WireTooltipLabelHover(hoverLabel)
	box.Add(row)
	return row
}

// finalizeDragRow is finalizeRow plus registration of the finished row with the
// drag group, which needs every row's geometry — not just the dragged one — to
// resolve a drop target.
func finalizeDragRow(
	box *fyne.Container,
	group *fynewidget.DragReorderGroup,
	slotIdx int,
	leftLead, rightCluster, center fyne.CanvasObject,
	hoverLabel *ttwidget.Label,
) *fynewidget.HoverRow {
	row := finalizeRow(box, leftLead, rightCluster, center, hoverLabel)
	group.Register(slotIdx, row)
	return row
}
