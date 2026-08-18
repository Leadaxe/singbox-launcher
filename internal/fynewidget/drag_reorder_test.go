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
