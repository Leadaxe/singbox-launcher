package tabs

import "testing"

// Арифметика окна проверяется таблицей и без экрана: виджетов в
// source_folder_drill_window.go нет намеренно (см. шапку файла).

func TestFolderDrillWindowRange(t *testing.T) {
	cases := []struct {
		name                       string
		total, first, last, margin int
		wantStart, wantEnd         int
	}{
		{"пустой состав", 0, 0, 10, 100, 0, 0},
		{"от начала", 500, 0, 20, 100, 0, 121},
		{"середина", 500, 200, 220, 100, 100, 321},
		{"хвост обрезан по total", 500, 480, 499, 100, 380, 500},
		{"окно шире состава", 50, 0, 10, 100, 0, 50},
		{"нулевой запас", 500, 200, 220, 0, 200, 221},
		{"перевёрнутый вход даёт начало", 500, 30, 10, 100, 0, 101},
		{"отрицательное смещение", 500, -5, 3, 10, 0, 14},
	}
	for _, c := range cases {
		start, end := folderDrillWindowRange(c.total, c.first, c.last, c.margin)
		if start != c.wantStart || end != c.wantEnd {
			t.Errorf("%s: folderDrillWindowRange(%d,%d,%d,%d) = %d,%d; want %d,%d",
				c.name, c.total, c.first, c.last, c.margin, start, end, c.wantStart, c.wantEnd)
		}
		if start < 0 || end > c.total || (c.total > 0 && end <= start) {
			t.Errorf("%s: диапазон %d..%d вне [0,%d)", c.name, start, end, c.total)
		}
	}
}

// TestFolderDrillSpacerHeightsMatchFullRender — главная проверка затеи: при
// ЛЮБОМ положении окна суммарная высота (распорка + отступы + строки +
// распорка) совпадает с полной отрисовкой, а первая построенная строка стоит
// там же, где стояла бы без окна.
func TestFolderDrillSpacerHeightsMatchFullRender(t *testing.T) {
	const (
		rowHeight = 37.0
		padding   = 4.0
		total     = 500
	)
	pitch := float32(rowHeight + padding)
	// Полная отрисовка: total элементов, между ними total-1 отступов.
	fullHeight := float32(total)*rowHeight + float32(total-1)*padding

	windows := [][2]int{{0, 121}, {100, 321}, {380, 500}, {0, total}, {499, 500}, {1, 2}}
	for _, w := range windows {
		start, end := w[0], w[1]
		top, bottom := folderDrillSpacerHeights(start, end, total, rowHeight, padding)

		// Собираем высоту так же, как её соберёт VBox: считаем элементы и
		// отступы между соседями.
		elems := end - start
		if top > 0 {
			elems++
		}
		if bottom > 0 {
			elems++
		}
		got := top + bottom + float32(end-start)*rowHeight + float32(elems-1)*padding
		if diff := got - fullHeight; diff > 0.01 || diff < -0.01 {
			t.Errorf("окно [%d,%d): высота содержимого %v, у полной отрисовки %v",
				start, end, got, fullHeight)
		}

		// Позиция первой построенной строки: верхняя распорка плюс её отступ.
		wantY := float32(start) * pitch
		gotY := float32(0)
		if top > 0 {
			gotY = top + padding
		}
		if diff := gotY - wantY; diff > 0.01 || diff < -0.01 {
			t.Errorf("окно [%d,%d): строка %d встала на y=%v, полная отрисовка ставит на %v",
				start, end, start, gotY, wantY)
		}
	}
}

func TestFolderDrillSpacerHeightsDegenerate(t *testing.T) {
	if top, bottom := folderDrillSpacerHeights(0, 0, 0, 37, 4); top != 0 || bottom != 0 {
		t.Errorf("пустой состав: хотели 0,0, получили %v,%v", top, bottom)
	}
	if top, bottom := folderDrillSpacerHeights(0, 10, 10, 37, 4); top != 0 || bottom != 0 {
		t.Errorf("окно = весь состав: распорок быть не должно, получили %v,%v", top, bottom)
	}
}

func TestFolderDrillWindowNeedsShift(t *testing.T) {
	cases := []struct {
		name                                        string
		winStart, winEnd, total, first, last, slack int
		want                                        bool
	}{
		{"центр окна — не двигаем", 100, 321, 500, 200, 220, 30, false},
		{"подошли к верху окна", 100, 321, 500, 125, 145, 30, true},
		{"подошли к низу окна", 100, 321, 500, 275, 295, 30, true},
		{"верх состава — двигать некуда", 0, 121, 500, 0, 20, 30, false},
		{"хвост состава — двигать некуда", 380, 500, 500, 470, 499, 30, false},
		{"пустое окно всегда перестраиваем", 0, 0, 500, 0, 20, 30, true},
		{"нулевой состав", 0, 0, 0, 0, 0, 30, true},
		{"без запаса двигаем только по краю", 100, 321, 500, 101, 200, 0, false},
		{"без запаса: край виден", 100, 321, 500, 99, 200, 0, true},
	}
	for _, c := range cases {
		got := folderDrillWindowNeedsShift(c.winStart, c.winEnd, c.total, c.first, c.last, c.slack)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFolderDrillVisibleRows(t *testing.T) {
	cases := []struct {
		name                string
		offset, view, pitch float32
		wantFirst, wantLast int
	}{
		{"начало списка", 0, 400, 41, 0, 9},
		{"прокручено на 100 строк", 4100, 400, 41, 100, 109},
		{"строка ещё не измерена", 4100, 400, 0, 0, 0},
		{"оттяжка вверх", -50, 400, 41, 0, 9},
		{"нулевая высота области", 4100, 0, 41, 100, 100},
	}
	for _, c := range cases {
		first, last := folderDrillVisibleRows(c.offset, c.view, c.pitch)
		if first != c.wantFirst || last != c.wantLast {
			t.Errorf("%s: got %d,%d; want %d,%d", c.name, first, last, c.wantFirst, c.wantLast)
		}
	}
}
