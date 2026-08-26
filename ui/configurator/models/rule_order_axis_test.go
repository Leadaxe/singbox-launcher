package models

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
)

// SPEC 106-B: ось порядка правил подключена к загрузке / перетаскиванию /
// сохранению. Сценарий Романа — перетащил, сохранил, перезашёл — обязан
// пережить round-trip, иначе правила «автосортируются» обратно.

func intp(v int) *int { return &v }

func boolp(v bool) *bool { return &v }

// axisTemplate — шаблон с системным правилом на 0, сортируемыми якорями и
// широким перехватчиком в конце (раскладка bin/wizard_template.json).
func axisTemplate() *wizardtemplate.TemplateData {
	return &wizardtemplate.TemplateData{
		Presets: []wizardtemplate.Preset{
			{ID: "traffic-processing", Num: intp(0), Sortable: boolp(false), DefaultEnabled: true},
			{ID: "private-ips", Num: intp(950)},
			{ID: "block-ads", Num: intp(960)},
			{ID: "russian", Num: intp(1120)},
		},
	}
}

func inlineRule(name string, num *int) corestate.Rule {
	body, _ := json.Marshal(corestate.InlineBody{
		Name:     name,
		Match:    map[string]interface{}{"domain_suffix": name + ".example"},
		Outbound: "proxy-out",
	})
	return corestate.Rule{Kind: corestate.RuleKindInline, Enabled: true, OrderNum: num, Body: body}
}

func presetRule(ref string, num *int) corestate.Rule {
	body, _ := json.Marshal(corestate.PresetBody{Vars: map[string]string{}})
	return corestate.Rule{Kind: corestate.RuleKindPreset, Ref: ref, Enabled: true, OrderNum: num, Body: body}
}

// loadIntoModel воспроизводит restorePresetRefs: normalize → PresetRefs →
// CustomRules (legacy view) → RuleOrder по оси → доразметка.
func loadIntoModel(t *testing.T, rules []corestate.Rule, td *wizardtemplate.TemplateData) *WizardModel {
	t.Helper()
	specs := wizardtemplate.RuleOrderSpecs(td.Presets)
	norm := corestate.NormalizeRuleOrder(rules, specs)

	m := &WizardModel{TemplateData: td}
	m.PresetRefs = SyncStateRulesToPresetRefs(norm)
	m.CustomRules = customRulesFromStateRules(norm)

	order := RuleOrderFromStateRulesV6(norm, m.PresetRefs, m.CustomRules)
	if len(order) == 0 {
		RebuildRuleOrder(m)
	} else {
		m.RuleOrder = order
		ReconcileRuleOrder(m)
	}
	EnsureRuleOrderNums(m)
	return m
}

// customRulesFromStateRules — legacy inline/srs view, как её отдаёт
// core/state.legacyCustomRulesFromV6 + PersistedCustomRuleToRuleState.
func customRulesFromStateRules(rules []corestate.Rule) []*RuleState {
	out := make([]*RuleState, 0)
	for _, r := range rules {
		if r.Kind != corestate.RuleKindInline && r.Kind != corestate.RuleKindSrs {
			continue
		}
		var name string
		var match map[string]interface{}
		outbound := ""
		body, err := r.DecodeBody()
		if err == nil {
			switch b := body.(type) {
			case *corestate.InlineBody:
				name, match, outbound = b.Name, b.Match, b.Outbound
			case *corestate.SrsBody:
				name, outbound = b.Name, b.Outbound
			}
		} else {
			// Безымянное правило: DecodeBody отказывает, но match переживает.
			var ib corestate.InlineBody
			_ = json.Unmarshal(r.Body, &ib)
			match, outbound = ib.Match, ib.Outbound
		}
		if match == nil {
			match = map[string]interface{}{}
		}
		match["outbound"] = outbound
		out = append(out, &RuleState{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label:       name,
				Rule:        match,
				HasOutbound: true,
			},
			Enabled:          r.Enabled,
			SelectedOutbound: outbound,
		})
	}
	return out
}

// saveModel воспроизводит CreateStateFromModel в части state.Rules.
func saveModel(m *WizardModel) []corestate.Rule {
	ReconcileRuleOrder(m)
	return SyncRulesByOrderToStateRulesV6(m.RuleOrder, m.PresetRefs, m.CustomRules)
}

// slotNames — читаемое представление порядка: ref пресета либо label правила.
func slotNames(m *WizardModel) []string {
	out := make([]string, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		switch s.Kind {
		case SlotKindPresetRef:
			out = append(out, m.PresetRefs[s.Index].Ref)
		case SlotKindCustom:
			label := m.CustomRules[s.Index].Rule.Label
			if label == "" {
				label = "<unnamed>"
			}
			out = append(out, label)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
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

// Тест 1 (обязательный, e2e): загрузка → перетаскивание custom-правила выше
// сортируемого пресета → Save → повторная загрузка. Порядок слотов совпадает,
// state.json несёт order_num.
func TestAxisSurvivesSaveLoadRoundTrip(t *testing.T) {
	td := axisTemplate()
	initial := []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		presetRule("private-ips", intp(950)),
		presetRule("block-ads", intp(960)),
		inlineRule("my-rule", intp(1000)),
		presetRule("russian", intp(1120)),
	}

	m := loadIntoModel(t, initial, td)
	before := slotNames(m)
	want := []string{"traffic-processing", "private-ips", "block-ads", "my-rule", "russian"}
	if !equalStrings(before, want) {
		t.Fatalf("порядок после загрузки = %v, ожидалось %v", before, want)
	}

	// Перетаскиваем custom-правило выше block-ads (позиция 2).
	if !MoveRuleSlot(m, 3, 2) {
		t.Fatal("MoveRuleSlot(3,2) вернул false — перетаскивание отклонено")
	}
	afterDrag := slotNames(m)
	wantDrag := []string{"traffic-processing", "private-ips", "my-rule", "block-ads", "russian"}
	if !equalStrings(afterDrag, wantDrag) {
		t.Fatalf("порядок после drag = %v, ожидалось %v", afterDrag, wantDrag)
	}

	saved := saveModel(m)
	for i, r := range saved {
		if r.OrderNum == nil {
			t.Fatalf("state.Rules[%d] (%s/%s) сохранён без order_num", i, r.Kind, r.Ref)
		}
	}

	// Round-trip через JSON — order_num обязан быть в файле, а не только в памяти.
	blob, err := json.Marshal(saved)
	if err != nil {
		t.Fatalf("marshal state.Rules: %v", err)
	}
	var reread []corestate.Rule
	if err := json.Unmarshal(blob, &reread); err != nil {
		t.Fatalf("unmarshal state.Rules: %v", err)
	}

	m2 := loadIntoModel(t, reread, td)
	if got := slotNames(m2); !equalStrings(got, wantDrag) {
		t.Fatalf("порядок после save→load = %v, ожидалось %v (drag откатился)", got, wantDrag)
	}
}

// Тест 2 (обязательный): безымянное inline-правило сохраняет позицию через
// round-trip. Матчинг по label терял такие правила — они доклеивались в конец.
func TestUnnamedInlineRuleKeepsPosition(t *testing.T) {
	td := axisTemplate()
	unnamed := inlineRule("", intp(1001))
	named := inlineRule("named", intp(1002))
	initial := []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		inlineRule("first", intp(1000)),
		unnamed,
		named,
	}

	m := loadIntoModel(t, initial, td)
	want := []string{"traffic-processing", "first", "<unnamed>", "named"}
	if got := slotNames(m); !equalStrings(got, want) {
		t.Fatalf("порядок после загрузки = %v, ожидалось %v", got, want)
	}

	saved := saveModel(m)
	m2 := loadIntoModel(t, saved, td)
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после round-trip = %v, ожидалось %v", got, want)
	}

	// И безымянное правило двигается наравне с остальными.
	if !MoveRuleSlot(m2, 3, 2) {
		t.Fatal("MoveRuleSlot(3,2) вернул false")
	}
	saved2 := saveModel(m2)
	m3 := loadIntoModel(t, saved2, td)
	wantMoved := []string{"traffic-processing", "first", "named", "<unnamed>"}
	if got := slotNames(m3); !equalStrings(got, wantMoved) {
		t.Fatalf("порядок после drag+round-trip = %v, ожидалось %v", got, wantMoved)
	}
}

// Тест 5 (обязательный): state с order_num=null размечается при первой
// загрузке и сохраняется размеченным.
func TestUnmarkedStateGetsMarkedAndPersisted(t *testing.T) {
	td := axisTemplate()
	initial := []corestate.Rule{
		presetRule("block-ads", nil),
		inlineRule("my-rule", nil),
		presetRule("private-ips", nil),
	}

	m := loadIntoModel(t, initial, td)
	saved := saveModel(m)
	if len(saved) == 0 {
		t.Fatal("после сохранения нет правил")
	}
	for i, r := range saved {
		if r.OrderNum == nil {
			t.Fatalf("state.Rules[%d] (%s/%s) остался без order_num", i, r.Kind, r.Ref)
		}
	}
	// Системный пресет пере-засеян нормализацией и стоит первым.
	if saved[0].Ref != "traffic-processing" {
		t.Fatalf("первым сохранён %q, ожидался traffic-processing", saved[0].Ref)
	}
	// Пресеты сели на якоря шаблона, пользовательское правило — в свою зону.
	nums := map[string]int{}
	for _, r := range saved {
		key := r.Ref
		if key == "" {
			key = "inline"
		}
		nums[key] = *r.OrderNum
	}
	if nums["private-ips"] != 950 || nums["block-ads"] != 960 {
		t.Fatalf("пресеты не сели на якоря шаблона: %v", nums)
	}
	if nums["inline"] < corestate.UserRuleNumStart {
		t.Fatalf("пользовательское правило вне своей зоны: %v", nums)
	}

	// Повторная загрузка идемпотентна.
	m2 := loadIntoModel(t, saved, td)
	if !equalStrings(slotNames(m), slotNames(m2)) {
		t.Fatalf("повторная загрузка переставила правила: %v → %v", slotNames(m), slotNames(m2))
	}
}

// Новое правило получает номер от NextUserRuleNum, а не хардкод: оно встаёт
// последним среди пользовательских и не вытесняет шаблонные якоря.
func TestNewRuleGetsNextUserNum(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		inlineRule("a", intp(1000)),
		inlineRule("b", intp(1001)),
		presetRule("russian", intp(1120)),
	}, td)

	got := NextRuleOrderNum(m)
	if got == nil || *got != 1002 {
		t.Fatalf("NextRuleOrderNum = %v, ожидалось 1002", got)
	}
}

// Тест 4 (обязательный): ленивый сдвиг D-053а на пути перетаскивания.
// Правило, брошенное ВПЛОТНУЮ к шаблонному якорю, не двигает якорь: между
// ними сотня свободных номеров, вытеснять некуда и незачем. Каскад +1 съедал
// бы зазоры, и вписать новый пресет между соседями стало бы нельзя.
func TestDragNextToAnchorDoesNotMoveAnchor(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		presetRule("private-ips", intp(950)), // якорь
		presetRule("block-ads", intp(960)),   // якорь
		inlineRule("mine", intp(1000)),       // пользовательское
		presetRule("russian", intp(1120)),    // широкий перехватчик
	}, td)

	// Бросаем пользовательское правило сразу за private-ips — вплотную.
	if !MoveRuleSlot(m, 3, 2) {
		t.Fatal("MoveRuleSlot(3,2) вернул false")
	}

	nums := map[string]int{}
	for _, s := range m.RuleOrder {
		switch s.Kind {
		case SlotKindPresetRef:
			pr := m.PresetRefs[s.Index]
			if pr.OrderNum == nil {
				t.Fatalf("пресет %q остался без номера", pr.Ref)
			}
			nums[pr.Ref] = *pr.OrderNum
		case SlotKindCustom:
			cr := m.CustomRules[s.Index]
			if cr.OrderNum == nil {
				t.Fatal("правило mine осталось без номера")
			}
			nums[cr.Rule.Label] = *cr.OrderNum
		}
	}

	if nums["private-ips"] != 950 {
		t.Errorf("якорь private-ips уехал на %d (был 950)", nums["private-ips"])
	}
	if nums["block-ads"] != 960 {
		t.Errorf("якорь block-ads уехал на %d (был 960) — каскад съел зазор", nums["block-ads"])
	}
	if nums["russian"] != 1120 {
		t.Errorf("якорь russian уехал на %d (был 1120)", nums["russian"])
	}
	if nums["traffic-processing"] != 0 {
		t.Errorf("системный якорь уехал на %d", nums["traffic-processing"])
	}
	if nums["mine"] != 951 {
		t.Errorf("перетащенное правило получило %d, ожидалось 951 (сразу за якорем 950)", nums["mine"])
	}

	// И порядок переживает round-trip.
	m2 := loadIntoModel(t, saveModel(m), td)
	want := []string{"traffic-processing", "private-ips", "mine", "block-ads", "russian"}
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после round-trip = %v, ожидалось %v", got, want)
	}
}

// Занятый соседний номер вытесняется — но только сплошным блоком, до первой
// дырки. Это вторая половина D-053а: вплотную стоящее правило сдвигается
// законно, а якорь за свободными номерами — нет.
func TestDragShiftsOnlyContiguousBlock(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		inlineRule("a", intp(1000)),
		inlineRule("b", intp(1001)),
		inlineRule("c", intp(1002)),
		presetRule("russian", intp(1120)),
	}, td)

	// c бросаем сразу за a — 1001 занят b, 1002 занят самим c (исключается),
	// значит сдвигается только b.
	if !MoveRuleSlot(m, 3, 2) {
		t.Fatal("MoveRuleSlot(3,2) вернул false")
	}
	nums := map[string]int{}
	for _, cr := range m.CustomRules {
		if cr.OrderNum != nil {
			nums[cr.Rule.Label] = *cr.OrderNum
		}
	}
	if nums["a"] != 1000 {
		t.Errorf("a = %d, ожидалось 1000 (сосед сверху не двигается)", nums["a"])
	}
	if nums["c"] != 1001 {
		t.Errorf("c = %d, ожидалось 1001", nums["c"])
	}
	if nums["b"] != 1002 {
		t.Errorf("b = %d, ожидалось 1002 (вытеснен на +1)", nums["b"])
	}
	if m.PresetRefs[1].OrderNum == nil || *m.PresetRefs[1].OrderNum != 1120 {
		t.Errorf("якорь russian сдвинут: %v", m.PresetRefs[1].OrderNum)
	}
}

// Без системного правила самая верхняя позиция достижима: перетащенное туда
// правило обязано остаться первым и после round-trip. Номер соседа снизу тут
// уместнее начала пользовательской зоны (1000) — иначе правило визуально
// уехало бы вверх, а по маршрутизации вниз, ниже якорей 950..990.
func TestDragToVeryTopSurvivesRoundTrip(t *testing.T) {
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
	m2 := loadIntoModel(t, saveModel(m), td)
	if got := slotNames(m2); !equalStrings(got, want) {
		t.Fatalf("порядок после round-trip = %v, ожидалось %v", got, want)
	}
}

// Системное правило по-прежнему прибито: ни само не двигается, ни поверх него
// не встают — и ось при этом не трогает его номер.
func TestSystemRuleNumberUntouchedByDrag(t *testing.T) {
	td := axisTemplate()
	m := loadIntoModel(t, []corestate.Rule{
		presetRule("traffic-processing", intp(0)),
		presetRule("private-ips", intp(950)),
		inlineRule("my-rule", intp(1000)),
	}, td)

	if MoveRuleSlot(m, 2, 0) {
		t.Fatal("правило встало выше системного")
	}
	if !MoveRuleSlot(m, 2, 1) {
		t.Fatal("обычная перестановка отклонена")
	}
	if n := m.PresetRefs[0].OrderNum; n == nil || *n != 0 {
		t.Fatalf("номер системного правила изменился: %v", n)
	}
}
