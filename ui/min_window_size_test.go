package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// SPEC 098 §3.2: окно не должно сжиматься ниже 1000×700 — ниже этого обе
// колонки Local/Remote превращаются в кашу из обрезанных подписей.
//
// Проверяется layout, а не виджеты: пакет ui не поднимает Fyne-приложение в
// тестах, а конструирование виджетов без него падает.
func TestMinSizeBoxEnforcesWindowFloor(t *testing.T) {
	tiny := canvas.NewRectangle(nil)
	tiny.SetMinSize(fyne.NewSize(10, 10))

	got := (&minSizeBox{min: MinWindowSize}).MinSize([]fyne.CanvasObject{tiny})
	if got.Width < MinWindowSize.Width || got.Height < MinWindowSize.Height {
		t.Fatalf("floor not enforced: got %v, want at least %v", got, MinWindowSize)
	}
}

// Занизить настоящий минимум контента нельзя: это разрешило бы обрезание
// виджетов, которые сами просят больше объявленного пола.
func TestMinSizeBoxRespectsLargerContent(t *testing.T) {
	big := canvas.NewRectangle(nil)
	big.SetMinSize(fyne.NewSize(1600, 1200))

	got := (&minSizeBox{min: MinWindowSize}).MinSize([]fyne.CanvasObject{big})
	if got.Width != 1600 || got.Height != 1200 {
		t.Fatalf("content minimum ignored: got %v, want 1600x1200", got)
	}
}

// Layout растягивает контент на всю выданную площадь: окно можно расширять,
// и пустых полей при этом быть не должно.
func TestMinSizeBoxFillsAvailableSpace(t *testing.T) {
	child := canvas.NewRectangle(nil)
	size := fyne.NewSize(1400, 900)

	(&minSizeBox{min: MinWindowSize}).Layout([]fyne.CanvasObject{child}, size)
	if child.Size() != size {
		t.Fatalf("child not stretched: got %v, want %v", child.Size(), size)
	}
}
