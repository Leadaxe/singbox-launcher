package state

import "testing"

// SPEC 113-C: закон оси. Массив, отданный наружу любой функцией разметки или
// перестановки, отсортирован по номерам; равных номеров двум правилам не
// выдаётся, пока в пользовательской зоне есть свободные значения.

// assertSorted — массив идёт по неубыванию номеров.
func assertSorted(t *testing.T, rules []Rule) {
	t.Helper()
	prev := -1 << 31
	for i, r := range rules {
		n := ruleNum(r)
		if n < prev {
			t.Fatalf("rules[%d] несёт номер %d после %d — массив не отсортирован по оси", i, n, prev)
		}
		prev = n
	}
}

// §1: разметка заканчивается пересортировкой. На старом коде MarkRuleOrder
// раздавал номера и оставлял массив в прежнем порядке — состояние «номера
// говорят одно, порядок другое» доживало до ближайшего потребителя.
func TestMarkRuleOrderSortsResult(t *testing.T) {
	rules := []Rule{
		presetRule("ru-direct", nil),          // получит 1120
		inlineRule(nil),                       // получит 1000
		presetRule("traffic-processing", nil), // получит 0
	}
	MarkRuleOrder(rules, testSpecs())
	assertSorted(t, rules)
	if rules[0].Ref != "traffic-processing" {
		t.Fatalf("первым идёт %q, ожидался traffic-processing (номер 0)", rules[0].Ref)
	}
}

// M7: want ниже пользовательской зоны клэмпится. Дроп вплотную под системную
// голову (0) давал номер 1 — правило уходило из зоны 1000..1100 навсегда, а
// сборка считала всё ниже UserRuleNumStart системной головой.
func TestPlaceRuleAfterClampsBelowUserZone(t *testing.T) {
	head := presetRule("traffic-processing", numPtr(0))
	mine := inlineRule(numPtr(1000))
	rules := []Rule{head, mine}
	sortable := func(r Rule) bool { return r.Ref != "traffic-processing" }

	PlaceRuleAfter(rules, 1, 0, sortable)

	if got := *rules[1].OrderNum; got < UserRuleNumStart {
		t.Errorf("moved получил %d — ниже пользовательской зоны %d", got, UserRuleNumStart)
	}
	if *rules[0].OrderNum != 0 {
		t.Errorf("системная голова сдвинута на %d", *rules[0].OrderNum)
	}
}

// Тот же клэмп у якорей шаблона: зоны 1..949 не существует, и дроп под якорь
// 950 не имеет права раздать номер 951.
func TestPlaceRuleAfterClampsUnderTemplateAnchor(t *testing.T) {
	rules := []Rule{
		presetRule("private-ips", numPtr(950)),
		inlineRule(numPtr(1000)),
	}
	PlaceRuleAfter(rules, 1, 0, allSortable)

	if got := *rules[1].OrderNum; got < UserRuleNumStart {
		t.Errorf("moved получил %d — ниже пользовательской зоны", got)
	}
	if *rules[0].OrderNum != 950 {
		t.Errorf("якорь уехал на %d", *rules[0].OrderNum)
	}
}

// §4: PlaceRuleBefore ставит правило перед соседом, вытесняя его ленивым
// сдвигом. Прежняя ветка movedPos == 0 просто копировала номер соседа —
// на оси оставались два правила с равным номером.
func TestPlaceRuleBeforeKeepsNumsDistinct(t *testing.T) {
	rules := []Rule{
		inlineRule(numPtr(1000)), // moved — брошен в самый верх
		presetRule("private-ips", numPtr(950)),
		presetRule("ru-direct", numPtr(1120)),
	}
	PlaceRuleBefore(rules, 0, 1, allSortable)

	if *rules[0].OrderNum != 950 {
		t.Errorf("moved получил %d, ожидался 950 (номер соседа)", *rules[0].OrderNum)
	}
	if *rules[1].OrderNum != 951 {
		t.Errorf("сосед вытеснен на %d, ожидался 951", *rules[1].OrderNum)
	}
	if *rules[0].OrderNum == *rules[1].OrderNum {
		t.Error("равные номера у перетащенного и соседа")
	}
	// Ленивость: якорь за дыркой не двигается.
	if *rules[2].OrderNum != 1120 {
		t.Errorf("якорь за дыркой уехал на %d", *rules[2].OrderNum)
	}
}

// PlaceRuleBefore без соседа сверху вырождается в «начало сортируемой части».
func TestPlaceRuleBeforeNilTarget(t *testing.T) {
	rules := []Rule{inlineRule(numPtr(1050))}
	PlaceRuleBefore(rules, 0, -1, allSortable)
	if *rules[0].OrderNum != UserRuleNumStart {
		t.Errorf("при target=nil правило получило %d, ожидалось %d", *rules[0].OrderNum, UserRuleNumStart)
	}
}

// Пин стабильности: при РАВНЫХ номерах сортировка обязана сохранить исходный
// взаимный порядок. Равенство законно — исчерпанная пользовательская зона
// (NextUserRuleNum отдаёт UserRuleNumEnd всем следующим), и тогда тай-брейком
// работает позиция в слайсе. Замена sort.SliceStable на sort.Slice в
// SortRulesByNum / sortRulesByNumInPlace этот тай-брейк уничтожит, а тесты выше
// (они смотрят на номера, а не на порядок равных) её не заметят.
//
// Форма выборки не случайна, и «взять побольше правил с одним номером» тут НЕ
// работает: если равны ВСЕ элементы, pdqsort за sort.Slice видит отсортированный
// прогон и выходит, ничего не переставив — при любом размере. Проверено: на
// полностью равном входе sort.Slice даёт 0 перестановок и на 200 элементах, то
// есть такой тест прошёл бы и на сломанном коде. Поэтому группа равных номеров
// идёт ВПЕРЕМЕЖКУ с различными: тогда партиционирование реально выполняется и
// тасует равных между собой. На этой форме sort.Slice ломается уже с ~20
// элементов; берём 64 с запасом.
func TestSortRulesByNumIsStableOnEqualNums(t *testing.T) {
	const n = 64
	const equalNum = UserRuleNumEnd // исчерпанная зона: NextUserRuleNum отдаёт границу всем

	// Чётные позиции — равный номер (их взаимный порядок и есть предмет пина),
	// нечётные — различные, чтобы сортировке было что переставлять.
	rules := make([]Rule, 0, n)
	var wantEqualOrder []string
	for i := 0; i < n; i++ {
		num := equalNum
		if i%2 == 1 {
			num = UserRuleNumStart + i
		}
		r := inlineRule(numPtr(num))
		// Метка исходной позиции: Ref у inline-правил ось не читает, зато он
		// переживает копирование в SortRulesByNum.
		r.Ref = "pos-" + itoaTest(i)
		rules = append(rules, r)
		if num == equalNum {
			wantEqualOrder = append(wantEqualOrder, r.Ref)
		}
	}

	// equalOrder — метки правил с равным номером в том порядке, в каком они
	// вышли из сортировки.
	equalOrder := func(sorted []Rule) []string {
		var got []string
		for _, r := range sorted {
			if ruleNum(r) == equalNum {
				got = append(got, r.Ref)
			}
		}
		return got
	}

	assertSameOrder := func(what string, got []string) {
		t.Helper()
		if len(got) != len(wantEqualOrder) {
			t.Fatalf("%s: правил с номером %d на выходе %d, подано %d",
				what, equalNum, len(got), len(wantEqualOrder))
		}
		for i := range got {
			if got[i] != wantEqualOrder[i] {
				t.Fatalf("%s: среди равных номеров на позиции %d оказалось %q, "+
					"ожидалось %q — при равных номерах исходный порядок не сохранён "+
					"(сортировка перестала быть стабильной)", what, i, got[i], wantEqualOrder[i])
			}
		}
	}

	sorted := SortRulesByNum(rules)
	assertSorted(t, sorted)
	assertSameOrder("SortRulesByNum", equalOrder(sorted))

	// Тот же пин для in-place пути (им заканчивается MarkRuleOrder).
	inPlace := make([]Rule, len(rules))
	copy(inPlace, rules)
	sortRulesByNumInPlace(inPlace)
	assertSorted(t, inPlace)
	assertSameOrder("sortRulesByNumInPlace", equalOrder(inPlace))
}

// itoaTest — strconv.Itoa для двузначных индексов; вынесен, чтобы не тащить
// импорт ради одной метки.
func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// Сдвиг взаимно однозначен: сплошной блок расходится по разным номерам, а не
// схлопывается в равные. Регресс на «полагаемся на стабильность сортировки».
func TestPlaceRuleAfterKeepsNumsDistinct(t *testing.T) {
	rules := []Rule{
		inlineRule(numPtr(1000)), // target
		inlineRule(numPtr(1001)),
		inlineRule(numPtr(1002)),
		inlineRule(numPtr(1050)), // moved
	}
	PlaceRuleAfter(rules, 3, 0, allSortable)

	seen := map[int]int{}
	for i, r := range rules {
		n := ruleNum(r)
		if prev, dup := seen[n]; dup {
			t.Errorf("равные номера у rules[%d] и rules[%d]: %d", prev, i, n)
		}
		seen[n] = i
	}
}
