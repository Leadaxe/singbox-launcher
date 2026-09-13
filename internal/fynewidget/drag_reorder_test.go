package fynewidget

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// rowsAt builds a group whose rows are laid out in a test window, so the
// geometry lookups in targetForY resolve against a real canvas: the midpoint
// rule is the part worth pinning down, and it only works on positioned objects.
func rowsAt(t *testing.T, n int, rowHeight float32) (*DragReorderGroup, fyne.Window) {
	t.Helper()
	g := NewDragReorderGroup(nil)
	objs := make([]fyne.CanvasObject, 0, n)
	for i := 0; i < n; i++ {
		row := widget.NewCheck("", nil)
		row.Resize(fyne.NewSize(100, rowHeight))
		row.Move(fyne.NewPos(0, float32(i)*rowHeight))
		objs = append(objs, row)
		g.Register(i, row)
	}
	w := test.NewWindow(nil)
	c := fyne.NewContainerWithoutLayout(objs...)
	c.Resize(fyne.NewSize(100, float32(n)*rowHeight))
	w.SetContent(c)
	w.Resize(fyne.NewSize(200, float32(n)*rowHeight+20))
	return g, w
}

// bandY returns an absolute Y inside row idx, at the given fraction of its height.
func bandY(t *testing.T, g *DragReorderGroup, idx int, frac float32) float32 {
	t.Helper()
	top, bottom, ok := g.rowBand(idx)
	if !ok {
		t.Fatalf("row %d has no band", idx)
	}
	return top + (bottom-top)*frac
}

// Dragging down only claims the target slot past its midpoint — below that the
// row stays put. Swapping on first contact makes a slow drag oscillate.
func TestTargetForYMidpointDown(t *testing.T) {
	g, w := rowsAt(t, 4, 30)
	defer w.Close()

	if got := g.targetForY(bandY(t, g, 1, 0.2), 0); got != 0 {
		t.Errorf("верхняя половина соседней строки: target = %d, ожидалось 0", got)
	}
	if got := g.targetForY(bandY(t, g, 1, 0.8), 0); got != 1 {
		t.Errorf("нижняя половина соседней строки: target = %d, ожидалось 1", got)
	}
}

func TestTargetForYMidpointUp(t *testing.T) {
	g, w := rowsAt(t, 4, 30)
	defer w.Close()

	if got := g.targetForY(bandY(t, g, 2, 0.8), 3); got != 3 {
		t.Errorf("нижняя половина строки выше: target = %d, ожидалось 3", got)
	}
	if got := g.targetForY(bandY(t, g, 2, 0.2), 3); got != 2 {
		t.Errorf("верхняя половина строки выше: target = %d, ожидалось 2", got)
	}
}

// Оставаясь в своей строке, перетаскиваемый ряд не должен никуда переезжать.
func TestTargetForYSameRowStays(t *testing.T) {
	g, w := rowsAt(t, 4, 30)
	defer w.Close()

	for _, frac := range []float32{0.1, 0.5, 0.9} {
		if got := g.targetForY(bandY(t, g, 2, frac), 2); got != 2 {
			t.Errorf("frac %v: target = %d, ожидалось 2", frac, got)
		}
	}
}

// Промах мимо списка сверху/снизу должен прилипать к краю, а не отменять драг:
// пользователь, утащивший строку за пределы, ожидает первую/последнюю позицию.
func TestTargetForYClampsOutsideList(t *testing.T) {
	g, w := rowsAt(t, 4, 30)
	defer w.Close()

	top, _, _ := g.rowBand(0)
	_, bottom, _ := g.rowBand(3)

	if got := g.targetForY(top-500, 2); got != 0 {
		t.Errorf("выше списка: target = %d, ожидалось 0", got)
	}
	if got := g.targetForY(bottom+500, 1); got != 3 {
		t.Errorf("ниже списка: target = %d, ожидалось 3", got)
	}
}

// Группа без зарегистрированных строк не должна паниковать и обязана вернуть
// исходную позицию — это путь «драг начался до layout'а».
func TestTargetForYEmptyGroup(t *testing.T) {
	g := NewDragReorderGroup(nil)
	if got := g.targetForY(100, 0); got != 0 {
		t.Errorf("пустая группа: target = %d, ожидалось 0", got)
	}
	if g.count() != 0 {
		t.Errorf("count = %d, ожидалось 0", g.count())
	}
}

// Драг одиночной строки не имеет смысла: Dragged должен выйти сразу, не трогая
// dropTarget и не рисуя индикатор.
func TestDragHandleIgnoresSingleRow(t *testing.T) {
	g := NewDragReorderGroup(nil)
	row := widget.NewCheck("", nil)
	g.Register(0, row)

	h := NewDragHandle(g, 0, nil)
	h.Dragged(&fyne.DragEvent{})
	if h.dragging {
		t.Error("одиночная строка: драг стартовал, ожидалось игнорирование")
	}
}

// DragEnd без предшествующего Dragged (клик по ручке без движения) не должен
// вызывать OnReorder — иначе простой клик помечал бы конфиг изменённым.
func TestDragEndWithoutDragDoesNotReorder(t *testing.T) {
	called := false
	g := NewDragReorderGroup(func(from, to int) { called = true })
	h := NewDragHandle(g, 1, nil)

	h.DragEnd()
	if called {
		t.Error("OnReorder вызван без драга")
	}
}

// Register после Reset обязан пересоздать карту строк. Регрессия v1.5.0:
// Reset() обнулял rows в nil, первый же Register паниковал «assignment to
// entry in nil map» и ронял процесс при открытии конфигуратора (вкладка
// Sources — единственный вызывающий Reset между пересборками списка).
func TestRegisterAfterResetDoesNotPanic(t *testing.T) {
	g := NewDragReorderGroup(nil)
	g.Register(0, widget.NewLabel("a"))
	g.Reset()
	g.Register(0, widget.NewLabel("b")) // паниковало
	if g.count() != 1 {
		t.Errorf("count() = %d, ожидалась 1 строка после Reset+Register", g.count())
	}
}

// Индикатор броска живёт в overlay-ах канваса, а Fyne отдаёт клики ТОЛЬКО
// верхнему overlay-у: забытая обёртка = окно не отвечает при живом процессе.
// Снимать её надо по ссылке, не трогая то, что легло поверх (диалог посреди
// броска), — прежний Remove(Top()) снимал диалог, а обёртку оставлял навсегда.
func TestHideIndicatorRemovesOwnOverlayKeepsDialogAbove(t *testing.T) {
	g, w := rowsAt(t, 3, 30)
	defer w.Close()
	c := w.Canvas()

	g.showIndicator(1, 0, c)
	if g.wrapper == nil || len(c.Overlays().List()) != 1 {
		t.Fatalf("после showIndicator ожидался ровно один overlay (обёртка), есть %d", len(c.Overlays().List()))
	}
	pop := widget.NewPopUp(widget.NewLabel("dialog"), c)
	pop.Show()
	if len(c.Overlays().List()) != 2 {
		t.Fatalf("ожидалось 2 overlay-а (обёртка + попап), есть %d", len(c.Overlays().List()))
	}
	// PopUp кладёт в стек не себя, а свой OverlayContainer — сравниваем с ним.
	dialogOverlay := c.Overlays().Top()

	g.hideIndicator()

	list := c.Overlays().List()
	if len(list) != 1 || list[0] != dialogOverlay {
		t.Fatalf("после hideIndicator должен остаться только попап, стек: %v", list)
	}
	if g.wrapper != nil || g.indicator != nil || g.canvas != nil {
		t.Errorf("группа не сбросила ссылки на индикатор: wrapper=%v indicator=%v canvas=%v", g.wrapper, g.indicator, g.canvas)
	}
}

// DragEnd прилетает старому захвату после пересборки списка посреди броска;
// сам он «не тащил», но индикатор висит на группе — снять его обязан любой
// DragEnd, иначе обёртка остаётся в overlay-ах.
func TestDragEndOfNonDraggingHandleClearsIndicator(t *testing.T) {
	g, w := rowsAt(t, 3, 30)
	defer w.Close()
	c := w.Canvas()
	g.showIndicator(2, 0, c)
	if len(c.Overlays().List()) != 1 {
		t.Fatalf("индикатор не поднялся")
	}

	h := NewDragHandle(g, 0, nil)
	h.DragEnd()

	if n := len(c.Overlays().List()); n != 0 {
		t.Errorf("после DragEnd в overlay-ах осталось %d объектов, ожидалось 0", n)
	}
	// Reset при пересборке списка тоже не оставляет индикатор.
	g.showIndicator(2, 0, c)
	g.Reset()
	if n := len(c.Overlays().List()); n != 0 {
		t.Errorf("после Reset в overlay-ах осталось %d объектов, ожидалось 0", n)
	}
}
