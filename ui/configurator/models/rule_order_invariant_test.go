package models

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
)

// SPEC 113-C: закон оси — массив правил всегда отсортирован по номерам, а
// файловый порядок не несёт самостоятельного смысла. Тесты ниже — регрессы на
// три места, где двойной порядок расходился: очередь безымянных правил (C6),
// дроп под системное правило (M7) и равные номера при дропе в самый верх
// (movedPos == 0).
//
// M7 пересмотрен решением пользователя 28.08.2026: клэмп к UserRuleNumStart
// снят, пользовательское правило вправе встать в любую позицию, кроме как выше
// несортируемой системной головы. Кейс-эталон — 4pda между якорями 950 и 960.

// loadCrooked — загрузка БЕЗ нормализации: воспроизводит файл, собранный чужой
// рукой, где массив идёт против оси. Нормализация такой файл выпрямила бы, и
// расхождение построения/потребления очереди осталось бы незамеченным.
func loadCrooked(rules []corestate.Rule, td *wizardtemplate.TemplateData) *WizardModel {
	m := &WizardModel{TemplateData: td}
	m.PresetRefs = SyncStateRulesToPresetRefs(rules)
	m.CustomRules = customRulesFromStateRules(rules)
	m.RuleOrder = RuleOrderFromAxis(rules, m.PresetRefs, m.CustomRules)
	ReconcileRuleOrder(m)
	EnsureRuleOrderNums(m)
	return m
}

// ruleMatchSuffix — по чему отличать два безымянных правила: имени у них нет,
// а тело есть.
func ruleMatchSuffix(cr *RuleState) string {
	if cr == nil {
		return ""
	}
	v, _ := cr.Rule.Rule["domain_suffix"].(string)
	return v
}

// unnamedRule — inline-правило без имени: identity у таких общая ("unnamed"),
// и различает их только очередь.
func unnamedRule(suffix string, num *int) corestate.Rule {
	body, _ := json.Marshal(corestate.InlineBody{
		Match:    map[string]interface{}{"domain_suffix": suffix},
		Outbound: "proxy-out",
	})
	return corestate.Rule{Kind: corestate.RuleKindInline, Enabled: true, OrderNum: num, Body: body}
}

// C6 (обязательный регресс): два безымянных inline-правила, файловый порядок
// обратен оси. Очередь строилась в файловом порядке, а потреблялась в осевом —
// правила получали чужие номера, и после save→load они менялись местами.
func TestUnnamedQueueFollowsAxisOnCrookedFile(t *testing.T) {
	td := axisTemplate()
	// В файле: сначала «beta» с номером 1001, потом «alpha» с 1000.
	crooked := []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		unnamedRule("beta", intp(1001)),
		unnamedRule("alpha", intp(1000)),
	}

	m := loadCrooked(crooked, td)

	// По оси первым идёт alpha (1000), вторым beta (1001).
	gotOrder := make([]string, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		if s.Kind == SlotKindCustom {
			gotOrder = append(gotOrder, ruleMatchSuffix(m.CustomRules[s.Index]))
		}
	}
	if !equalStrings(gotOrder, []string{"alpha", "beta"}) {
		t.Fatalf("порядок безымянных после загрузки = %v, ожидалось [alpha beta]", gotOrder)
	}

	// И номер каждому достался СВОЙ, а не соседа: на старом коде очередь
	// отдавала осевому первому (alpha) индекс файлового первого (beta).
	for _, cr := range m.CustomRules {
		suffix := ruleMatchSuffix(cr)
		if cr.OrderNum == nil {
			t.Fatalf("правило %q осталось без номера", suffix)
		}
		want := map[string]int{"alpha": 1000, "beta": 1001}[suffix]
		if *cr.OrderNum != want {
			t.Errorf("правило %q получило номер %d, ожидался %d — номера перекрестились",
				suffix, *cr.OrderNum, want)
		}
	}

	// Round-trip не переставляет их местами.
	m2 := loadIntoModel(t, saveModel(m), td)
	gotAfter := make([]string, 0, len(m2.RuleOrder))
	for _, s := range m2.RuleOrder {
		if s.Kind == SlotKindCustom {
			gotAfter = append(gotAfter, ruleMatchSuffix(m2.CustomRules[s.Index]))
		}
	}
	if !equalStrings(gotAfter, []string{"alpha", "beta"}) {
		t.Fatalf("порядок после save→load = %v, ожидалось [alpha beta]", gotAfter)
	}
}

// M7, редакция 28.08.2026 (норма — зеркало LxBox §370): дроп вплотную ПОД
// системное правило ЛЕГАЛЕН и даёт номер 1 — позицию сразу под несортируемой
// головой. Прежняя редакция клэмпила такой драг к UserRuleNumStart, и правило,
// брошенное в самый верх, уезжало под все шаблонные якоря.
//
// Чем это безопасно для сборки: core/build/preset_merge.go делит правила на
// «голову» (num < UserRuleNumStart) и хвост, СОХРАНЯЯ относительный порядок
// внутри каждой группы. Якоря 950..990 сами живут в голове, так что правило на
// 1 встаёт перед ними ровно там, где его показали, а traffic-processing на 0
// остаётся первым.
func TestDropUnderSystemRuleIsLegalAndKeepsAnchors(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)), // системное, несортируемое
		presetRule("private-ips", intp(950)),      // якорь
		presetRule("block-ads", intp(960)),        // якорь
		inlineRule("mine", intp(1000)),            // пользовательское
	}, td)

	// Позиция 1 — сразу под системным правилом, выше уже нельзя.
	if !MoveRuleSlot(m, 3, 1) {
		t.Fatal("MoveRuleSlot(3,1) вернул false — дроп под системное правило отклонён")
	}

	var moved *RuleState
	for _, cr := range m.CustomRules {
		if cr.Rule.Label == "mine" {
			moved = cr
		}
	}
	if moved == nil || moved.OrderNum == nil {
		t.Fatal("перетащенное правило потеряло номер")
	}
	if *moved.OrderNum != 1 {
		t.Errorf("правило получило номер %d, ожидался 1 — позиция сразу под головой", *moved.OrderNum)
	}
	if *moved.OrderNum < corestate.MinSortableRuleNum {
		t.Errorf("правило получило номер %d — провалилось под системную голову", *moved.OrderNum)
	}

	// Якоря не тронуты: между 1 и 950 дырка, вытеснять некого.
	anchors := map[string]int{}
	for _, pr := range m.PresetRefs {
		if pr.OrderNum != nil {
			anchors[pr.Ref] = *pr.OrderNum
		}
	}
	if anchors["traffic-processing"] != 0 {
		t.Errorf("системный якорь уехал на %d", anchors["traffic-processing"])
	}
	if anchors["private-ips"] != 950 || anchors["block-ads"] != 960 {
		t.Errorf("якоря шаблона сдвинуты: %v", anchors)
	}

	// Правило стоит там, куда его бросили, и переживает round-trip.
	want := []string{"traffic-processing", "mine", "private-ips", "block-ads"}
	if got := slotNames(m); !equalStrings(got, want) {
		t.Fatalf("порядок после drag = %v, ожидалось %v", got, want)
	}
	m2 := loadIntoModel(t, saveModel(m), td)
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после save→load = %v, ожидалось %v", got, want)
	}

	// Список показывает то же, что скажет маршрутизация.
	assertSlotsMatchAxis(t, m)
}

// Кейс-эталон 4pda (решение пользователя 28.08.2026): пользовательское правило
// встаёт МЕЖДУ шаблонными якорями «локальная сеть» (950) и «русские домены»
// (960) — после локалки, до рудоменов — получает промежуточный номер 951..959,
// якоря при этом НЕ двигаются, и позиция переживает save→load.
func TestUserRuleLandsBetweenAnchors4pda(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		presetRule("private-ips", intp(950)), // «локальная сеть»
		presetRule("block-ads", intp(960)),   // «русские домены»
		inlineRule("4pda", intp(1000)),
		presetRule("russian", intp(1120)), // хвостовой catch-all
	}, td)

	// Бросаем 4pda на позицию 2 — сразу за «локальной сетью».
	if !MoveRuleSlot(m, 3, 2) {
		t.Fatal("MoveRuleSlot(3,2) вернул false")
	}

	nums := collectAxisNums(t, m)
	if nums["4pda"] != 951 {
		t.Errorf("4pda получило номер %d, ожидался 951 (между 950 и 960)", nums["4pda"])
	}
	if nums["4pda"] <= 950 || nums["4pda"] >= 960 {
		t.Errorf("4pda на %d — вне промежутка между якорями 950 и 960", nums["4pda"])
	}
	if nums["private-ips"] != 950 || nums["block-ads"] != 960 {
		t.Errorf("якоря сдвинуты: private-ips=%d block-ads=%d", nums["private-ips"], nums["block-ads"])
	}
	if nums["traffic-processing"] != 0 {
		t.Errorf("системная голова сдвинута на %d", nums["traffic-processing"])
	}
	if nums["russian"] != 1120 {
		t.Errorf("хвостовой catch-all уехал на %d", nums["russian"])
	}

	want := []string{"traffic-processing", "private-ips", "4pda", "block-ads", "russian"}
	if got := slotNames(m); !equalStrings(got, want) {
		t.Fatalf("порядок после drag = %v, ожидалось %v", got, want)
	}

	// Главное требование кейса: позиция ОСТАЁТСЯ после save→load.
	m2 := loadIntoModel(t, saveModel(m), td)
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после save→load = %v, ожидалось %v", got, want)
	}
	nums2 := collectAxisNums(t, m2)
	if nums2["4pda"] != 951 {
		t.Errorf("после save→load 4pda получило %d, ожидался 951", nums2["4pda"])
	}
	if nums2["private-ips"] != 950 || nums2["block-ads"] != 960 {
		t.Errorf("после save→load якоря сдвинуты: %v", nums2)
	}
}

// Дроп НАД всеми якорями (в первую доступную строку под системной головой):
// номер < 950, якоря не тронуты. Это верхняя граница свободы пользователя.
func TestDropAboveAllAnchorsGetsNumBelowAnchors(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		presetRule("private-ips", intp(950)),
		presetRule("block-ads", intp(960)),
		inlineRule("mine", intp(1000)),
	}, td)

	if !MoveRuleSlot(m, 3, 1) {
		t.Fatal("MoveRuleSlot(3,1) вернул false")
	}

	nums := collectAxisNums(t, m)
	if nums["mine"] >= 950 {
		t.Errorf("mine получило %d — не поднялось выше якорей (< 950)", nums["mine"])
	}
	if nums["mine"] < corestate.MinSortableRuleNum {
		t.Errorf("mine получило %d — провалилось под системную голову", nums["mine"])
	}
	if nums["private-ips"] != 950 || nums["block-ads"] != 960 {
		t.Errorf("якоря сдвинуты: %v", nums)
	}
}

// movedPos == 0 (обязательный регресс): дроп в самый верх выдавал
// перетащенному номер соседа снизу как есть — на оси оставались два правила с
// равным номером, и порядок держался только стабильностью сортировки.
func TestDropToTopDoesNotProduceEqualNums(t *testing.T) {
	td := &wizardtemplate.TemplateData{
		Presets: []wizardtemplate.Preset{
			{ID: "private-ips", Num: intp(950)},
			{ID: "block-ads", Num: intp(960)},
		},
	}
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("private-ips", intp(950)),
		presetRule("block-ads", intp(960)),
		inlineRule("mine", intp(1000)),
	}, td)

	if !MoveRuleSlot(m, 2, 0) {
		t.Fatal("MoveRuleSlot(2,0) вернул false")
	}

	want := []string{"mine", "private-ips", "block-ads"}
	if got := slotNames(m); !equalStrings(got, want) {
		t.Fatalf("порядок после drag = %v, ожидалось %v", got, want)
	}

	nums := collectAxisNums(t, m)
	seen := map[int]string{}
	for name, n := range nums {
		if other, dup := seen[n]; dup {
			t.Errorf("равные номера у %q и %q: %d", other, name, n)
		}
		seen[n] = name
	}
	if nums["mine"] >= nums["private-ips"] {
		t.Errorf("перетащенное (%d) не выше соседа (%d)", nums["mine"], nums["private-ips"])
	}
	// Вытеснение ленивое: block-ads стоит за дыркой и не двигается.
	if nums["block-ads"] != 960 {
		t.Errorf("якорь block-ads уехал на %d — каскад не остановился на первой дырке", nums["block-ads"])
	}

	m2 := loadIntoModel(t, saveModel(m), td)
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после round-trip = %v, ожидалось %v", got, want)
	}
}

// Инвариант §1 на пути сохранения: что бы модель ни подала, на выходе массив
// отсортирован по оси. Слоты подаём заведомо против оси.
func TestSaveEmitsRulesSortedByAxis(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		inlineRule("a", intp(1000)),
		inlineRule("b", intp(1001)),
		presetRule("russian", intp(1120)),
	}, td)

	// Разворачиваем слоты — модель подаёт порядок, противоречащий номерам.
	rev := make([]RuleSlot, 0, len(m.RuleOrder))
	for i := len(m.RuleOrder) - 1; i >= 0; i-- {
		rev = append(rev, m.RuleOrder[i])
	}
	saved := EmitStateRulesInAxisOrder(rev, m.PresetRefs, m.CustomRules)

	assertRulesSortedByAxis(t, saved)
	if saved[0].Ref != "traffic-processing" {
		t.Errorf("первым сохранено %q, ожидался traffic-processing (номер 0)", saved[0].Ref)
	}
}

// Тот же инвариант на fallback-пути (RuleOrder пуст): конкатенация «пресеты,
// потом inline» — файловый порядок, и он обязан быть выпрямлен по оси.
func TestSaveFallbackEmitsRulesSortedByAxis(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		inlineRule("a", intp(1000)),
		presetRule("russian", intp(1120)),
	}, td)

	saved := EmitStateRulesInAxisOrder(nil, m.PresetRefs, m.CustomRules)
	assertRulesSortedByAxis(t, saved)
}

// collectAxisNums — номера оси по читаемому имени правила.
func collectAxisNums(t *testing.T, m *WizardModel) map[string]int {
	t.Helper()
	nums := map[string]int{}
	for _, s := range m.RuleOrder {
		switch s.Kind {
		case SlotKindPresetRef:
			pr := m.PresetRefs[s.Index]
			if pr.OrderNum == nil {
				t.Fatalf("пресет %q без номера", pr.Ref)
			}
			nums[pr.Ref] = *pr.OrderNum
		case SlotKindCustom:
			cr := m.CustomRules[s.Index]
			if cr.OrderNum == nil {
				t.Fatalf("правило %q без номера", cr.Rule.Label)
			}
			nums[cr.Rule.Label] = *cr.OrderNum
		}
	}
	return nums
}

// assertSlotsMatchAxis — слоты идут по неубыванию номеров.
func assertSlotsMatchAxis(t *testing.T, m *WizardModel) {
	t.Helper()
	prev := -1 << 31
	for i, s := range m.RuleOrder {
		n := m.slotOrderNum(s)
		if n == nil {
			t.Fatalf("слот %d без номера", i)
		}
		if *n < prev {
			t.Fatalf("слот %d несёт номер %d после %d — список расходится с осью", i, *n, prev)
		}
		prev = *n
	}
}

// assertRulesSortedByAxis — массив state.Rules идёт по неубыванию номеров.
func assertRulesSortedByAxis(t *testing.T, rules []corestate.Rule) {
	t.Helper()
	prev := -1 << 31
	for i, r := range rules {
		if r.OrderNum == nil {
			t.Fatalf("state.Rules[%d] (%s/%s) без order_num", i, r.Kind, r.Ref)
		}
		if *r.OrderNum < prev {
			t.Fatalf("state.Rules[%d] несёт номер %d после %d — массив не отсортирован по оси",
				i, *r.OrderNum, prev)
		}
		prev = *r.OrderNum
	}
}
