package state

import "testing"

// Тесты оси порядка (SPEC 106). Проверяют прежде всего ИНВАРИАНТЫ D-053 —
// каждый из них исправленный баг LxBox, а не стилевое предпочтение: без них
// перенос воспроизвёл бы уже починенные ошибки.

func numPtr(n int) *int { return &n }

func presetRule(ref string, num *int) Rule {
	return Rule{Kind: RuleKindPreset, Ref: ref, Enabled: true, OrderNum: num, Body: []byte(`{}`)}
}

func inlineRule(num *int) Rule {
	return Rule{Kind: RuleKindInline, Enabled: true, OrderNum: num, Body: []byte(`{}`)}
}

func allSortable(Rule) bool { return true }

// specs — типовая раскладка: несортируемая голова на 0 и два якоря шаблона.
func testSpecs() map[string]RuleOrderSpec {
	return map[string]RuleOrderSpec{
		"traffic-processing": {Num: 0, Sortable: false, DefaultEnabled: true},
		"private-ips":        {Num: 950, Sortable: true, DefaultEnabled: true},
		"ru-direct":          {Num: 1120, Sortable: true, DefaultEnabled: true},
	}
}

func TestSeedAddsMissingNonSortablePreset(t *testing.T) {
	// D-050: неотчуждаемый пресет возвращается, даже если его стёрли из state.
	rules := []Rule{presetRule("private-ips", numPtr(950))}
	out := SeedRequiredRules(rules, testSpecs())

	found := false
	for _, r := range out {
		if r.Ref == "traffic-processing" {
			found = true
			if r.OrderNum == nil || *r.OrderNum != 0 {
				t.Errorf("seeded с номером %v, ожидался 0", r.OrderNum)
			}
		}
	}
	if !found {
		t.Fatal("несортируемый пресет не был засеян")
	}
}

func TestSeedDoesNotDuplicateExisting(t *testing.T) {
	rules := []Rule{presetRule("traffic-processing", numPtr(0))}
	out := SeedRequiredRules(rules, testSpecs())
	if len(out) != 1 {
		t.Fatalf("seed задвоил существующий пресет: %d правил", len(out))
	}
}

func TestSeedSkipsSortablePresets(t *testing.T) {
	// Состав сортируемых пресетов — выбор пользователя, их сеять нельзя.
	out := SeedRequiredRules(nil, testSpecs())
	for _, r := range out {
		if r.Ref != "traffic-processing" {
			t.Errorf("засеян сортируемый пресет %q", r.Ref)
		}
	}
}

func TestDedupeKeepsLastOccurrence(t *testing.T) {
	// Из группы одинаковых Ref выживает последний: при импорте вторая копия
	// приехала позже и отражает более свежее намерение.
	a := presetRule("ru-direct", numPtr(1120))
	b := presetRule("ru-direct", numPtr(1121))
	b.Enabled = false
	out := DedupePresetRules([]Rule{a, b})
	if len(out) != 1 {
		t.Fatalf("дедуп оставил %d правил, ожидалось 1", len(out))
	}
	if out[0].Enabled {
		t.Error("выжила первая копия, а должна последняя")
	}
}

func TestDedupeRunsBeforeSeed(t *testing.T) {
	// D-053в: на задвоенном списке seed видит пресет как присутствующий и
	// молчит — тогда удаление одной копии не даёт видимого эффекта.
	// NormalizeRuleOrder обязан дедуплицировать ДО seed'а.
	dup := []Rule{
		presetRule("traffic-processing", numPtr(0)),
		presetRule("traffic-processing", numPtr(0)),
	}
	out := NormalizeRuleOrder(dup, testSpecs())

	count := 0
	for _, r := range out {
		if r.Ref == "traffic-processing" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("после нормализации %d копий неотчуждаемого пресета, ожидалась 1", count)
	}
}

func TestMarkOrderUsesTemplateNumForPresets(t *testing.T) {
	rules := []Rule{presetRule("ru-direct", nil), inlineRule(nil)}
	if !MarkRuleOrder(rules, testSpecs()) {
		t.Fatal("разметка не сообщила об изменении")
	}
	// Позиции после разметки не фиксируем: MarkRuleOrder приводит массив к оси
	// (SPEC 113-C §1), и пользовательское правило 1000 встаёт перед пресетом
	// 1120. Проверяем номера, а не индексы.
	nums := map[string]int{}
	for _, r := range rules {
		key := r.Ref
		if key == "" {
			key = "inline"
		}
		if r.OrderNum == nil {
			t.Fatalf("правило %q осталось без номера", key)
		}
		nums[key] = *r.OrderNum
	}
	if nums["ru-direct"] != 1120 {
		t.Errorf("пресет получил номер %d, ожидался 1120 из шаблона", nums["ru-direct"])
	}
	if nums["inline"] != UserRuleNumStart {
		t.Errorf("пользовательское правило получило %d, ожидалось %d", nums["inline"], UserRuleNumStart)
	}
}

func TestMarkOrderIsIdempotent(t *testing.T) {
	rules := []Rule{presetRule("ru-direct", nil)}
	MarkRuleOrder(rules, testSpecs())
	if MarkRuleOrder(rules, testSpecs()) {
		t.Error("повторная разметка изменила уже размеченный список")
	}
}

func TestSortIsStableOnEqualNums(t *testing.T) {
	a := inlineRule(numPtr(1000))
	a.Ref = "первый"
	b := inlineRule(numPtr(1000))
	b.Ref = "второй"
	out := SortRulesByNum([]Rule{a, b})
	if out[0].Ref != "первый" || out[1].Ref != "второй" {
		t.Error("сортировка не стабильна при равных номерах")
	}
}

func TestNormalizePutsHeadFirst(t *testing.T) {
	// Смысл всей оси: неотчуждаемая голова обязана быть первой независимо от
	// того, в каком порядке правила лежали в state.
	rules := []Rule{
		presetRule("ru-direct", numPtr(1120)),
		presetRule("private-ips", numPtr(950)),
	}
	out := NormalizeRuleOrder(rules, testSpecs())
	if out[0].Ref != "traffic-processing" {
		t.Fatalf("первым идёт %q, ожидался traffic-processing", out[0].Ref)
	}
}

func TestPlaceRuleAfterFreeSlotDoesNotTouchNeighbours(t *testing.T) {
	// want свободен → соседи не двигаются вовсе.
	rules := []Rule{
		inlineRule(numPtr(1000)),
		inlineRule(numPtr(1050)),
	}
	PlaceRuleAfter(rules, 1, 0, allSortable) // 1050 → сразу за 1000
	if *rules[1].OrderNum != 1001 {
		t.Errorf("moved получил %d, ожидался 1001", *rules[1].OrderNum)
	}
	if *rules[0].OrderNum != 1000 {
		t.Errorf("target сдвинулся на %d, а не должен был", *rules[0].OrderNum)
	}
}

func TestPlaceRuleAfterCascadeStopsAtFirstGap(t *testing.T) {
	// D-053а — ключевой инвариант. Сплошной блок 1001,1002 сдвигается; якорь
	// на 1120 за большой дыркой вытеснять НЕКУДА, и он обязан остаться на месте.
	// Наивное «сдвинуть всех с num >= want» уводило бы его на 1121.
	rules := []Rule{
		inlineRule(numPtr(1000)), // target
		inlineRule(numPtr(1001)), // сплошной блок
		inlineRule(numPtr(1002)), // сплошной блок
		inlineRule(numPtr(1120)), // якорь за дыркой
		inlineRule(numPtr(1050)), // moved
	}
	PlaceRuleAfter(rules, 4, 0, allSortable)

	if *rules[4].OrderNum != 1001 {
		t.Errorf("moved получил %d, ожидался 1001", *rules[4].OrderNum)
	}
	if *rules[1].OrderNum != 1002 || *rules[2].OrderNum != 1003 {
		t.Errorf("блок сдвинулся неверно: %d, %d", *rules[1].OrderNum, *rules[2].OrderNum)
	}
	if *rules[3].OrderNum != 1120 {
		t.Errorf("ЯКОРЬ за дыркой уехал на %d — каскад не остановился на первой дырке", *rules[3].OrderNum)
	}
}

func TestPlaceRuleAfterIgnoresNonSortable(t *testing.T) {
	// D-053г: несортируемое не двигается и не вытесняется.
	head := presetRule("traffic-processing", numPtr(0))
	rules := []Rule{head, inlineRule(numPtr(1000))}
	sortable := func(r Rule) bool { return r.Ref != "traffic-processing" }

	PlaceRuleAfter(rules, 0, 1, sortable) // попытка сдвинуть голову
	if *rules[0].OrderNum != 0 {
		t.Errorf("несортируемое правило сдвинулось на %d", *rules[0].OrderNum)
	}
}

func TestPlaceRuleAfterNilTarget(t *testing.T) {
	rules := []Rule{inlineRule(numPtr(1050))}
	PlaceRuleAfter(rules, 0, -1, allSortable)
	if *rules[0].OrderNum != UserRuleNumStart {
		t.Errorf("при target=nil правило получило %d, ожидалось %d", *rules[0].OrderNum, UserRuleNumStart)
	}
}

func TestNextUserRuleNumIgnoresOutOfZone(t *testing.T) {
	// Якоря вне пользовательской зоны не должны влиять на номер нового правила.
	rules := []Rule{
		presetRule("traffic-processing", numPtr(0)),
		inlineRule(numPtr(1005)),
		presetRule("ru-direct", numPtr(1120)),
	}
	if got := NextUserRuleNum(rules); got != 1006 {
		t.Errorf("следующий номер %d, ожидался 1006", got)
	}
}

func TestNextUserRuleNumClampsAtZoneEnd(t *testing.T) {
	rules := []Rule{inlineRule(numPtr(UserRuleNumEnd))}
	if got := NextUserRuleNum(rules); got != UserRuleNumEnd {
		t.Errorf("на границе зоны получено %d, ожидалось %d", got, UserRuleNumEnd)
	}
}
