// File node_reorder_test.go — перестановка узла внутри контейнера
// (SPEC 116 этап 3, W5; требование П5).
//
// Проверяется ровно то, что здесь можно перепутать: арифметика вставки после
// выреза. Вёрстку и захват мышью не тестируем (no-ui-format-tests).
package tabs

import (
	"testing"

	corestate "singbox-launcher/core/state"
)

func reorderNodes(tags ...string) []corestate.Node {
	out := make([]corestate.Node, 0, len(tags))
	for _, t := range tags {
		out = append(out, corestate.Node{Kind: corestate.SourceKindServer, Tag: t})
	}
	return out
}

func reorderTags(nodes []corestate.Node) []string {
	out := make([]string, 0, len(nodes))
	for i := range nodes {
		out = append(out, nodes[i].Tag)
	}
	return out
}

func TestMoveNodeWithinSlice(t *testing.T) {
	cases := []struct {
		name     string
		from, to int
		want     []string
	}{
		// `to` отсчитывается по слайсу УЖЕ БЕЗ вырезанного элемента — та же
		// семантика, что у chainForm.moveHop (source_chain_tab.go:669).
		// Расхождение двух перестановок под одним захватом было бы хуже
		// любой из двух семантик по отдельности.
		{"вниз через одного", 0, 2, []string{"b", "c", "a", "d"}},
		{"вниз в самый хвост", 0, 3, []string{"b", "c", "d", "a"}},
		{"вниз на соседа", 1, 2, []string{"a", "c", "b", "d"}},
		{"вверх через одного", 3, 1, []string{"a", "d", "b", "c"}},
		{"вверх в голову", 2, 0, []string{"c", "a", "b", "d"}},
		// Границы: молча ничего не портим.
		{"сам в себя", 2, 2, []string{"a", "b", "c", "d"}},
		{"из ниоткуда", -1, 1, []string{"a", "b", "c", "d"}},
		{"за границу", 1, 9, []string{"a", "b", "c", "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := reorderNodes("a", "b", "c", "d")
			got := reorderTags(moveNodeWithinSlice(src, tc.from, tc.to))
			if len(got) != len(tc.want) {
				t.Fatalf("длина изменилась: %v", got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("порядок = %v, ожидался %v", got, tc.want)
				}
			}
		})
	}
}

// Перестановка не должна портить исходный слайс на месте: список превью и
// модель могут делить backing-массив, и правка «через голову» разъехалась бы
// с тем, что показано.
func TestMoveNodeWithinSliceDoesNotMutateInput(t *testing.T) {
	src := reorderNodes("a", "b", "c", "d")
	_ = moveNodeWithinSlice(src, 0, 3)
	if got := reorderTags(src); got[0] != "a" || got[3] != "d" {
		t.Fatalf("исходный слайс переписан на месте: %v", got)
	}
}
