package fynewidget

// Drag-and-drop reordering for the wizard's list rows (Rules, DNS, Sources).
//
// # Why a handle and not the whole row
//
// Rows are dense: checkbox, label (tap-to-toggle), SRS button, edit/delete,
// outbound select. Starting a drag from anywhere in the row would fight every
// one of those. The drag lives on its own grip glyph in the leading cluster,
// where the ↑/↓ buttons used to be.
//
// # Why the handle wins over the enclosing scroll container
//
// These lists live inside container.VScroll, which is itself Draggable (that is
// how it pans). The glfw driver resolves a drag with findObjectAtPositionMatching,
// which returns the *deepest* Draggable under the pointer — so a grab on the
// grip drives the reorder, while a grab anywhere else in the list still scrolls.
// No event-swallowing tricks are needed.
//
// # How a drop target is computed
//
// fyne.DragEvent carries AbsolutePosition (canvas coordinates of the pointer),
// so the handle never has to integrate deltas itself. Each handle knows its own
// slot index and asks the shared DragReorderGroup which row's vertical band
// contains that Y. Bands come from AbsolutePositionForObject, which is only
// valid after layout — hence rows register themselves as they are built, and
// every lookup tolerates a missing/zero-sized row.
//
// The insertion point is drawn as a line in the canvas overlay rather than by
// reshuffling widgets mid-drag: the lists rebuild wholesale on commit, so
// moving real widgets during the drag would fight the rebuild and flicker.

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	_ fyne.Widget        = (*DragHandle)(nil)
	_ fyne.Draggable     = (*DragHandle)(nil)
	_ desktop.Hoverable  = (*DragHandle)(nil)
	_ desktop.Cursorable = (*DragHandle)(nil)
)

// DragGripGlyph is the grip shown in place of the old ↑/↓ pair.
// U+283F is a Braille pattern: those live in the default Fyne font on every
// platform, unlike the box-drawing and arrow glyphs that fall back to the
// missing-glyph box (see the "→" regression fixed in the DNS rows).
const DragGripGlyph = "⠿"

// The grip is dimmed until its row is hovered, so a long list does not read as
// a column of noise.
const (
	dragGripIdleAlpha  = 0x66
	dragGripHoverAlpha = 0xff
)

const (
	// dropLineThickness is the insertion indicator's height in points.
	dropLineThickness = 2

	// autoScrollEdge is how close to the viewport edge the pointer must get
	// before the list starts scrolling itself, and autoScrollStep is how far
	// each drag event nudges it. Without this, reordering across a list taller
	// than the viewport is impossible: the pointer hits the edge and stops.
	autoScrollEdge = 24
	autoScrollStep = 12
)

// DragReorderGroup coordinates the handles of one list: it owns the row
// geometry, the drop indicator, and the commit callback.
//
// A group is rebuilt together with the rows it belongs to — these lists
// rerender wholesale after any reorder — so Register is idempotent per index
// and a rebuilt row simply overwrites the stale entry.
type DragReorderGroup struct {
	// OnReorder moves the item at index from to index to in the caller's model
	// and triggers that list's refresh. Called once, on drop, and only when the
	// position actually changes.
	OnReorder func(from, to int)

	// Scroll is the viewport the rows live in. Optional: when set, dragging near
	// its top or bottom edge scrolls the list.
	Scroll *container.Scroll

	// Total — сколько элементов в СПИСКЕ ДАННЫХ, когда он длиннее видимого
	// (widget.List: зарегистрированы только видимые строки, см.
	// RegisterRecycled). 0 = «список равен зарегистрированным строкам», как у
	// VBox-списков — их поведение не меняется.
	//
	// Нужен клампу: без него перетаскивание за нижний край длинного списка
	// упиралось бы в индекс последней ВИДИМОЙ строки и молча роняло узел в
	// середину.
	Total int

	rows map[int]fyne.CanvasObject

	// indicator is the insertion line, parented to the canvas overlay so it can
	// paint across row boundaries without taking part in layout.
	indicator *canvas.Rectangle
	canvas    fyne.Canvas

	dropTarget int
}

// NewDragReorderGroup builds the coordinator for one reorderable list.
func NewDragReorderGroup(onReorder func(from, to int)) *DragReorderGroup {
	return &DragReorderGroup{
		OnReorder: onReorder,
		rows:      make(map[int]fyne.CanvasObject),
	}
}

// Register records the row drawn for slot index idx. Call it for every row in
// the list (typically right after the row object exists): resolving a drop
// target needs all the bands, not just the dragged row's.
// Reset забывает все зарегистрированные строки. Обязателен перед пересборкой
// списка, который мог СТАТЬ КОРОЧЕ: Register идемпотентен по индексу, но
// запись со старшим индексом удалённой строки иначе жила бы вечно — count()
// завышен, а точка вставки может разрешиться в несуществующий индекс или
// чужую полосу.
func (g *DragReorderGroup) Reset() {
	g.rows = nil
}

func (g *DragReorderGroup) Register(idx int, row fyne.CanvasObject) {
	if g == nil || row == nil {
		return
	}
	// Ленивая инициализация: после Reset() карта nil, и запись в неё —
	// паника «assignment to entry in nil map», роняющая весь процесс при
	// первом же построении списка (так падало открытие конфигуратора).
	if g.rows == nil {
		g.rows = make(map[int]fyne.CanvasObject)
	}
	g.rows[idx] = row
}

// RegisterRecycled — Register для списка, который ПЕРЕИСПОЛЬЗУЕТ объекты строк
// (widget.List: видимые строки перепривязываются к другим индексам на каждой
// прокрутке).
//
// Обычного Register там мало: он идемпотентен по индексу, но не по объекту, и
// уехавшая за экран строка остаётся висеть под своим прежним индексом, будучи
// уже перепривязанной к другому. Тогда два индекса заявляют одну и ту же
// полосу экрана, и targetForY отдаёт то один, то другой — точка вставки
// прыгает, а drop уходит в чужой слот.
//
// Здесь карта держится БИЕКЦИЕЙ: объект сначала снимается со всех прежних
// индексов, потом встаёт под текущий. Регистрация покрывает ровно видимые
// строки — этого хватает, потому что бросить строку можно только туда, куда
// пользователь видит (за край экрана списки доводит autoScroll, а он
// перепривязывает строки по дороге).
//
// SPEC 116 W5: понадобилось списку узлов контейнера — первому drag-списку на
// widget.List (у Sources/Rules/DNS строки живут в VBox и строятся целиком).
func (g *DragReorderGroup) RegisterRecycled(idx int, row fyne.CanvasObject) {
	if g == nil || row == nil {
		return
	}
	if g.rows == nil {
		g.rows = make(map[int]fyne.CanvasObject)
	}
	for i, existing := range g.rows {
		if existing == row && i != idx {
			delete(g.rows, i)
		}
	}
	g.rows[idx] = row
}

// count reports how many rows are registered.
func (g *DragReorderGroup) count() int {
	if g == nil {
		return 0
	}
	return len(g.rows)
}

// slots — сколько СЛОТОВ у списка данных: Total, если он задан (виртуальный
// список длиннее экрана), иначе число зарегистрированных строк.
//
// Разведено с count() намеренно: count отвечает на «сколько строк я вижу и
// могу измерить», slots — на «в какой диапазон индексов допустим бросок». Их
// смешение и есть та ошибка, из-за которой длинный список ронял узел в
// середину.
func (g *DragReorderGroup) slots() int {
	if g == nil {
		return 0
	}
	if g.Total > len(g.rows) {
		return g.Total
	}
	return len(g.rows)
}

// rowBand returns the absolute vertical extent of the row at idx.
func (g *DragReorderGroup) rowBand(idx int) (top, bottom float32, ok bool) {
	row, exists := g.rows[idx]
	if !exists {
		return 0, 0, false
	}
	h := row.Size().Height
	if h <= 0 {
		return 0, 0, false
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(row)
	return pos.Y, pos.Y + h, true
}

// targetForY maps an absolute pointer Y to the slot index the dragged row would
// take.
//
// The rule is midpoint-based: the dragged row claims a neighbour's slot only
// once the pointer passes that row's middle. Swapping on first contact instead
// makes a slow drag oscillate between two slots.
func (g *DragReorderGroup) targetForY(y float32, from int) int {
	n := g.slots()
	for idx := range g.rows {
		top, bottom, ok := g.rowBand(idx)
		if !ok || y < top || y > bottom {
			continue
		}
		mid := (top + bottom) / 2
		switch {
		case idx < from && y > mid:
			// Moving up, but only into this row's lower half: stop just below it.
			return clampIndex(idx+1, n)
		case idx > from && y < mid:
			return clampIndex(idx-1, n)
		default:
			return clampIndex(idx, n)
		}
	}
	// Past either end of the list: pin to that end, so a drag that overshoots
	// still lands instead of silently cancelling.
	//
	// «Край» берётся у КРАЙНЕЙ ЗАРЕГИСТРИРОВАННОЙ строки, а не у слота 0 /
	// n-1: у виртуального списка их на экране может не быть вовсе, и жёсткая
	// адресация давала бы «полосы нет» → бросок молча отменялся.
	if idx, top, ok := g.topmostBand(); ok && y < top {
		return idx
	}
	if idx, bottom, ok := g.bottommostBand(); ok && y > bottom {
		return clampIndex(idx, n)
	}
	return from
}

// topmostBand / bottommostBand — крайние ИЗ ЗАРЕГИСТРИРОВАННЫХ строк по
// вертикали. У VBox-списка это ровно слоты 0 и n-1 (прежнее поведение), у
// виртуального — верхняя и нижняя видимые строки.
func (g *DragReorderGroup) topmostBand() (int, float32, bool) {
	bestIdx, bestY, ok := 0, float32(0), false
	for idx := range g.rows {
		top, _, valid := g.rowBand(idx)
		if !valid {
			continue
		}
		if !ok || top < bestY {
			bestIdx, bestY, ok = idx, top, true
		}
	}
	return bestIdx, bestY, ok
}

func (g *DragReorderGroup) bottommostBand() (int, float32, bool) {
	bestIdx, bestY, ok := 0, float32(0), false
	for idx := range g.rows {
		_, bottom, valid := g.rowBand(idx)
		if !valid {
			continue
		}
		if !ok || bottom > bestY {
			bestIdx, bestY, ok = idx, bottom, true
		}
	}
	return bestIdx, bestY, ok
}

// showIndicator draws the insertion line at the edge of the target row: above
// it when moving up, below it when moving down — which is where the dragged row
// will actually come to rest.
func (g *DragReorderGroup) showIndicator(target, from int, c fyne.Canvas) {
	if c == nil {
		return
	}
	top, bottom, ok := g.rowBand(target)
	if !ok {
		return
	}
	row := g.rows[target]
	y := top
	if target > from {
		y = bottom - dropLineThickness
	}

	if g.indicator == nil {
		g.indicator = canvas.NewRectangle(theme.Color(theme.ColorNamePrimary))
		g.canvas = c
		c.Overlays().Add(container.NewWithoutLayout(g.indicator))
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(row)
	g.indicator.Resize(fyne.NewSize(row.Size().Width, dropLineThickness))
	g.indicator.Move(fyne.NewPos(pos.X, y))
	g.indicator.Show()
	g.indicator.Refresh()
}

// hideIndicator removes the insertion line and its overlay wrapper.
func (g *DragReorderGroup) hideIndicator() {
	if g.indicator == nil {
		return
	}
	g.indicator.Hide()
	if g.canvas != nil {
		// Overlays are a stack; the wrapper we pushed is the top one as long as
		// no dialog opened mid-drag. Remove() is index-free and safe either way.
		if top := g.canvas.Overlays().Top(); top != nil {
			g.canvas.Overlays().Remove(top)
		}
	}
	g.indicator = nil
	g.canvas = nil
}

// autoScroll nudges the viewport when the pointer nears its edge during a drag.
func (g *DragReorderGroup) autoScroll(y float32) {
	if g.Scroll == nil {
		return
	}
	top := fyne.CurrentApp().Driver().AbsolutePositionForObject(g.Scroll).Y
	bottom := top + g.Scroll.Size().Height

	switch {
	case y < top+autoScrollEdge:
		g.Scroll.Offset.Y -= autoScrollStep
	case y > bottom-autoScrollEdge:
		g.Scroll.Offset.Y += autoScrollStep
	default:
		return
	}
	if g.Scroll.Offset.Y < 0 {
		g.Scroll.Offset.Y = 0
	}
	g.Scroll.Refresh()
}

func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if n > 0 && i > n-1 {
		return n - 1
	}
	return i
}

// DragHandle is the grip a row is dragged by. It holds the row's slot index and
// delegates every cross-row decision to its group.
type DragHandle struct {
	widget.BaseWidget

	group *DragReorderGroup
	index int

	// rowGetter keeps the row's hover tint lit while the pointer is on the grip,
	// matching the other controls in the leading cluster (see HoverForward*).
	rowGetter RowHoverGetter

	glyph   *canvas.Text
	hovered bool

	dragging bool
}

// NewDragHandle builds the grip for the row at slot idx within group.
func NewDragHandle(group *DragReorderGroup, idx int, rowGetter RowHoverGetter) *DragHandle {
	h := &DragHandle{group: group, index: idx, rowGetter: rowGetter}
	h.ExtendBaseWidget(h)
	return h
}

// SetIndex перепривязывает захват к другому слоту.
//
// Нужен строкам ПЕРЕИСПОЛЬЗУЕМОГО списка (widget.List): там объект строки
// создаётся один раз, а индекс ему сообщают на каждой привязке. У списков в
// VBox индекс задаётся конструктором и не меняется — их этот метод не
// касается.
func (h *DragHandle) SetIndex(idx int) {
	if h != nil {
		h.index = idx
	}
}

// CreateRenderer implements fyne.Widget.
func (h *DragHandle) CreateRenderer() fyne.WidgetRenderer {
	h.glyph = canvas.NewText(DragGripGlyph, h.gripColor())
	h.glyph.TextSize = theme.TextSize()
	h.glyph.Alignment = fyne.TextAlignCenter
	// The grip reserves the width the ↑/↓ pair used to hold, so the checkbox
	// column lines up with rows that have no handle.
	pad := canvas.NewRectangle(color.Transparent)
	pad.SetMinSize(fyne.NewSize(theme.IconInlineSize()+theme.Padding()*2, theme.IconInlineSize()))
	return widget.NewSimpleRenderer(container.NewStack(pad, container.NewCenter(h.glyph)))
}

func (h *DragHandle) gripColor() color.Color {
	r, g, b, _ := theme.Color(theme.ColorNameForeground).RGBA()
	a := uint8(dragGripIdleAlpha)
	if h.hovered || h.dragging {
		a = dragGripHoverAlpha
	}
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}

func (h *DragHandle) refreshGrip() {
	if h.glyph == nil {
		return
	}
	h.glyph.Color = h.gripColor()
	h.glyph.Refresh()
}

// Cursor implements desktop.Cursorable: the pointer change is the main hint
// that the glyph is grabbable.
func (h *DragHandle) Cursor() desktop.Cursor { return desktop.PointerCursor }

// MouseIn implements desktop.Hoverable.
func (h *DragHandle) MouseIn(e *desktop.MouseEvent) {
	h.hovered = true
	h.refreshGrip()
	forwardRowHover(h.rowGetter, func(r *HoverRow) { r.MouseIn(e) })
}

// MouseMoved implements desktop.Hoverable.
func (h *DragHandle) MouseMoved(e *desktop.MouseEvent) {
	forwardRowHover(h.rowGetter, func(r *HoverRow) { r.MouseMoved(e) })
}

// MouseOut implements desktop.Hoverable.
func (h *DragHandle) MouseOut() {
	h.hovered = false
	h.refreshGrip()
	forwardRowHover(h.rowGetter, func(r *HoverRow) { r.MouseOut() })
}

// Dragged implements fyne.Draggable.
func (h *DragHandle) Dragged(e *fyne.DragEvent) {
	if h.group == nil || h.group.count() < 2 {
		return
	}
	if !h.dragging {
		h.dragging = true
		h.group.dropTarget = h.index
		h.refreshGrip()
	}

	y := e.AbsolutePosition.Y
	h.group.autoScroll(y)
	h.group.dropTarget = h.group.targetForY(y, h.index)

	if h.group.dropTarget != h.index {
		h.group.showIndicator(h.group.dropTarget, h.index, fyne.CurrentApp().Driver().CanvasForObject(h))
	} else {
		h.group.hideIndicator()
	}
}

// DragEnd implements fyne.Draggable.
func (h *DragHandle) DragEnd() {
	if h.group == nil || !h.dragging {
		return
	}
	h.dragging = false
	h.refreshGrip()
	h.group.hideIndicator()

	to := h.group.dropTarget
	if to == h.index || h.group.OnReorder == nil {
		return
	}
	h.group.OnReorder(h.index, to)
}

// NewDragHandleSpacer возвращает пустышку ровно той ширины, что занимает
// DragHandle (SPEC 106).
//
// Нужна строкам системных правил, у которых захвата нет: без распорки их
// чекбокс и подпись съезжают влево относительно соседних строк, и список
// выглядит рваным. Ширина повторяет пэд из CreateRenderer — единственное
// место, где размер захвата определён.
func NewDragHandleSpacer() fyne.CanvasObject {
	pad := canvas.NewRectangle(color.Transparent)
	pad.SetMinSize(fyne.NewSize(theme.IconInlineSize()+theme.Padding()*2, theme.IconInlineSize()))
	return pad
}
