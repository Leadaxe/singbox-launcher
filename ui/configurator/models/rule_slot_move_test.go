package models

import "testing"

// order helper — компактное представление порядка для сравнения в тестах.
func order(m *WizardModel) []int {
	out := make([]int, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		out = append(out, s.Index)
	}
	return out
}

func modelWithSlots(n int) *WizardModel {
	m := &WizardModel{}
	for i := 0; i < n; i++ {
		m.RuleOrder = append(m.RuleOrder, RuleSlot{Kind: SlotKindCustom, Index: i})
	}
	return m
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Перенос вниз: элемент встаёт на целевую позицию, промежуточные сдвигаются
// вверх на одну — это отличие от swap, ради которого MoveRuleSlot и написан.
func TestMoveRuleSlotDown(t *testing.T) {
	m := modelWithSlots(5)
	if !MoveRuleSlot(m, 0, 3) {
		t.Fatal("MoveRuleSlot(0,3) вернул false")
	}
	want := []int{1, 2, 3, 0, 4}
	if got := order(m); !equalInts(got, want) {
		t.Fatalf("порядок после move(0,3) = %v, ожидалось %v", got, want)
	}
}

func TestMoveRuleSlotUp(t *testing.T) {
	m := modelWithSlots(5)
	if !MoveRuleSlot(m, 4, 1) {
		t.Fatal("MoveRuleSlot(4,1) вернул false")
	}
	want := []int{0, 4, 1, 2, 3}
	if got := order(m); !equalInts(got, want) {
		t.Fatalf("порядок после move(4,1) = %v, ожидалось %v", got, want)
	}
}

// Соседний перенос должен совпадать со старым swap-поведением ↑/↓, иначе
// drag на одну позицию отличался бы от клика по стрелке.
func TestMoveRuleSlotAdjacentMatchesSwap(t *testing.T) {
	m := modelWithSlots(4)
	MoveRuleSlot(m, 2, 1)
	want := []int{0, 2, 1, 3}
	if got := order(m); !equalInts(got, want) {
		t.Fatalf("порядок после move(2,1) = %v, ожидалось %v", got, want)
	}
}

// Границы и no-op: вызывающий UI-код по false пропускает MarkAsChanged, так что
// ложное true пометило бы конфиг грязным без изменений.
func TestMoveRuleSlotRejectsInvalid(t *testing.T) {
	m := modelWithSlots(3)
	cases := []struct{ from, to int }{
		{1, 1},  // no-op
		{-1, 1}, // from вне диапазона
		{0, 3},  // to вне диапазона
		{5, 0},
	}
	for _, c := range cases {
		if MoveRuleSlot(m, c.from, c.to) {
			t.Errorf("MoveRuleSlot(%d,%d) вернул true, ожидался false", c.from, c.to)
		}
	}
	if got := order(m); !equalInts(got, []int{0, 1, 2}) {
		t.Fatalf("порядок изменился при отклонённых move: %v", got)
	}
}

// Перетаскивание не должно терять или дублировать slot'ы: длина и состав
// сохраняются при любой последовательности перемещений.
func TestMoveRuleSlotPreservesSet(t *testing.T) {
	m := modelWithSlots(6)
	moves := [][2]int{{0, 5}, {3, 0}, {5, 2}, {1, 4}}
	for _, mv := range moves {
		MoveRuleSlot(m, mv[0], mv[1])
	}
	if len(m.RuleOrder) != 6 {
		t.Fatalf("длина RuleOrder = %d, ожидалось 6", len(m.RuleOrder))
	}
	seen := make(map[int]bool)
	for _, s := range m.RuleOrder {
		if seen[s.Index] {
			t.Fatalf("дубликат slot index %d в %v", s.Index, order(m))
		}
		seen[s.Index] = true
	}
}

func TestMoveDNSRuleSlot(t *testing.T) {
	m := &WizardModel{}
	for i := 0; i < 4; i++ {
		m.DNSRuleOrder = append(m.DNSRuleOrder, DNSRuleSlot{Kind: DNSSlotKindUser, Index: i})
	}
	if !MoveDNSRuleSlot(m, 3, 0) {
		t.Fatal("MoveDNSRuleSlot(3,0) вернул false")
	}
	got := make([]int, 0, 4)
	for _, s := range m.DNSRuleOrder {
		got = append(got, s.Index)
	}
	if !equalInts(got, []int{3, 0, 1, 2}) {
		t.Fatalf("порядок DNS после move(3,0) = %v, ожидалось [3 0 1 2]", got)
	}
	if MoveDNSRuleSlot(m, 0, 0) {
		t.Error("MoveDNSRuleSlot(0,0) вернул true, ожидался false")
	}
}
