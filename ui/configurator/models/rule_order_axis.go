// File rule_order_axis.go — мост между slot'ами Rules tab и разреженной осью
// порядка правил (SPEC 106, core/state/rule_order.go).
//
// Слоты (RuleOrder) — механизм ОТОБРАЖЕНИЯ: они говорят, в каком порядке
// нарисовать строки. Приоритет правила задаёт ось: номер живёт в модели
// (PresetRefState.OrderNum / RuleState.OrderNum), уезжает в state.Rules при
// Save и оттуда попадает в config.json. Позиция в слайсе — только тай-брейк.
//
// Поэтому перетаскивание обязано двигать НОМЕР, а не только слот: иначе Save
// эмитил бы правила без разметки, а следующая загрузка пере-нумеровала бы их
// подряд — ровно тот «откат автосортировкой», с которого начался SPEC 106-B.
//
// Закон оси («массив = ось», SPEC 113-C) сформулирован в package comment
// core/state/rule_order.go; здесь он держится тем, что после каждой правки
// номеров слоты пересортировываются по оси (см. applyAxisAfterMove).
package models

import (
	"sort"

	corestate "singbox-launcher/core/state"
)

// slotOrderNum — номер оси у правила, на которое ссылается slot.
func (m *WizardModel) slotOrderNum(s RuleSlot) *int {
	switch s.Kind {
	case SlotKindPresetRef:
		if s.Index >= 0 && s.Index < len(m.PresetRefs) && m.PresetRefs[s.Index] != nil {
			return m.PresetRefs[s.Index].OrderNum
		}
	case SlotKindCustom:
		if s.Index >= 0 && s.Index < len(m.CustomRules) && m.CustomRules[s.Index] != nil {
			return m.CustomRules[s.Index].OrderNum
		}
	}
	return nil
}

// setSlotOrderNum — записать номер оси правилу, на которое ссылается slot.
func (m *WizardModel) setSlotOrderNum(s RuleSlot, num *int) {
	switch s.Kind {
	case SlotKindPresetRef:
		if s.Index >= 0 && s.Index < len(m.PresetRefs) && m.PresetRefs[s.Index] != nil {
			m.PresetRefs[s.Index].OrderNum = num
		}
	case SlotKindCustom:
		if s.Index >= 0 && s.Index < len(m.CustomRules) && m.CustomRules[s.Index] != nil {
			m.CustomRules[s.Index].OrderNum = num
		}
	}
}

// axisProxyRules — []state.Rule ровно по одному на slot в текущем порядке
// RuleOrder. Это ПРОКСИ: тела правил не собираются (дорого и не нужно), только
// то, что читает ось — Kind/Ref (чтобы отличить несортируемый пресет) и
// OrderNum. Значения номеров копируются, чтобы PlaceRuleAfter не правил модель
// вживую до того, как мы решили принять результат.
func (m *WizardModel) axisProxyRules() []corestate.Rule {
	out := make([]corestate.Rule, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		r := corestate.Rule{Kind: corestate.RuleKindInline}
		if s.Kind == SlotKindPresetRef {
			r.Kind = corestate.RuleKindPreset
			if s.Index >= 0 && s.Index < len(m.PresetRefs) && m.PresetRefs[s.Index] != nil {
				r.Ref = m.PresetRefs[s.Index].Ref
			}
		}
		r.OrderNum = copyOrderNum(m.slotOrderNum(s))
		out = append(out, r)
	}
	return out
}

// applyAxisAfterMove — пересчитывает номера оси после перестановки слотов.
//
// movedPos — позиция перетащенного слота в УЖЕ переставленном RuleOrder;
// сосед сверху (movedPos-1) становится target'ом PlaceRuleAfter. Случай
// «встал самым первым» соседа сверху не имеет и идёт через PlaceRuleBefore.
//
// Ленивый сдвиг D-053а живёт внутри Place*, и здесь не дублируется:
// пере-нумеровывать зону подряд нельзя — это съело бы зазоры между шаблонными
// якорями (см. комментарий в core/state/rule_order.go).
//
// Заканчивается пересортировкой слотов: закон оси (SPEC 113-C) не допускает
// расхождения между тем, что показано, и тем, что скажет маршрутизация.
func (m *WizardModel) applyAxisAfterMove(movedPos int) {
	if movedPos < 0 || movedPos >= len(m.RuleOrder) {
		return
	}
	proxy := m.axisProxyRules()

	// Неразмеченные соседи (state до SPEC 106, ещё не прошедший normalize)
	// сломали бы ленивый сдвиг: nil читается как DefaultRuleNum, но записать
	// его обратно некуда. Размечаем прокси подряд по текущему порядку —
	// это ровно то, что сделал бы MarkRuleOrder на загрузке.
	markProxyGaps(proxy)

	sortable := func(r corestate.Rule) bool { return m.isSortableAxisRule(r) }
	if movedPos == 0 {
		// Самый верх списка И системной головы в нём нет (иначе MoveRuleSlot не
		// пустил бы сюда): соседа сверху не существует. Начало пользовательской
		// зоны (1000) тут не годится — над правилом стоят сортируемые якоря
		// 950..990, и оно уехало бы визуально вверх, а по маршрутизации вниз.
		// Встаём ПЕРЕД соседом снизу, вытесняя его ленивым сдвигом.
		corestate.PlaceRuleBefore(proxy, 0, 1, sortable)
	} else {
		// Обычный драг — зеркало LxBox §370: номер соседа СВЕРХУ + 1, ленивый
		// сдвиг при занятом номере. Дроп в первую доступную строку под системной
		// головой попадает сюда же и даёт num = 1.
		corestate.PlaceRuleAfter(proxy, movedPos, movedPos-1, sortable)
	}

	for i, s := range m.RuleOrder {
		m.setSlotOrderNum(s, copyOrderNum(proxy[i].OrderNum))
	}

	// Закон оси: массив всегда отсортирован по номерам. Ленивый сдвиг может
	// увести соседей на +1, а исчерпанный диапазон — выдать правилу не тот
	// номер, куда его бросили; без этой пересортировки список показывал бы
	// одно, а маршрутизация делала другое до следующей загрузки.
	SortRuleOrderByAxis(m)
}

// markProxyGaps — доразметка прокси: правило без номера получает номер,
// следующий за верхним соседом (или начало пользовательской зоны, если оно
// первое). Номера соседей не трогаются.
func markProxyGaps(proxy []corestate.Rule) {
	prev := corestate.UserRuleNumStart - 1
	for i := range proxy {
		if proxy[i].OrderNum != nil {
			prev = *proxy[i].OrderNum
			continue
		}
		n := prev + 1
		if n < corestate.UserRuleNumStart {
			n = corestate.UserRuleNumStart
		}
		proxy[i].OrderNum = &n
		prev = n
	}
}

// EnsureRuleOrderNums — доразметка модели: правило без номера получает его по
// текущей позиции в RuleOrder, соседи не трогаются.
//
// Нужна на путях, где слот появился мимо оси: legacy state без rules[] (order
// собран RebuildRuleOrder), ReconcileRuleOrder дописал слот для правила,
// которого не было в state. Без этого правило сохранилось бы без order_num и
// при следующей загрузке село бы в начало пользовательской зоны — то есть
// поехало бы вверх относительно соседей.
func EnsureRuleOrderNums(m *WizardModel) {
	if m == nil || len(m.RuleOrder) == 0 {
		return
	}
	proxy := m.axisProxyRules()
	markProxyGaps(proxy)
	for i, s := range m.RuleOrder {
		if m.slotOrderNum(s) == nil {
			m.setSlotOrderNum(s, copyOrderNum(proxy[i].OrderNum))
		}
	}
}

// SortRuleOrderByAxis — переставляет слоты в порядке оси (стабильно).
//
// Нужен там, где слот дописан в конец списка, а его номер говорит другое —
// добавление пресета из Library: пресет с якорем 960 обязан встать среди
// шаблонных, а не последней строкой, которая после перезахода «прыгнет».
// На уже согласованном списке — no-op.
func SortRuleOrderByAxis(m *WizardModel) {
	if m == nil || len(m.RuleOrder) < 2 {
		return
	}
	proxy := m.axisProxyRules()
	idx := make([]int, len(m.RuleOrder))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return axisNum(proxy[idx[a]]) < axisNum(proxy[idx[b]])
	})
	out := make([]RuleSlot, 0, len(m.RuleOrder))
	for _, i := range idx {
		out = append(out, m.RuleOrder[i])
	}
	m.RuleOrder = out
}

func axisNum(r corestate.Rule) int {
	if r.OrderNum == nil {
		return corestate.DefaultRuleNum
	}
	return *r.OrderNum
}

// isSortableAxisRule — можно ли двигать правило и вытеснять его соседями.
// Несортируемым объявлен только пресет с `sortable: false` в шаблоне
// (SPEC 106, D-050): его номер часть инварианта.
func (m *WizardModel) isSortableAxisRule(r corestate.Rule) bool {
	if r.Kind != corestate.RuleKindPreset || m.TemplateData == nil {
		return true
	}
	for i := range m.TemplateData.Presets {
		if m.TemplateData.Presets[i].ID == r.Ref {
			return m.TemplateData.Presets[i].IsSortable()
		}
	}
	return true
}

// NextRuleOrderNum — номер для НОВОГО пользовательского правила: конец занятой
// части пользовательской зоны (state.NextUserRuleNum по текущему набору), а не
// хардкод. Возвращает указатель, готовый лечь в RuleState/PresetRefState.
func NextRuleOrderNum(m *WizardModel) *int {
	if m == nil {
		return nil
	}
	n := corestate.NextUserRuleNum(m.axisProxyRules())
	return &n
}

// PresetRuleOrderNum — номер оси для только что добавленного пресета: якорь из
// шаблона, если он там объявлен, иначе конец пользовательской зоны. Пресет
// обязан вставать на СВОЙ якорь сразу, а не после следующей загрузки: иначе
// включённый через Library пресет ехал бы в конец списка, а после перезахода
// прыгал на место.
func PresetRuleOrderNum(m *WizardModel, ref string) *int {
	if m == nil {
		return nil
	}
	if m.TemplateData != nil {
		for i := range m.TemplateData.Presets {
			if m.TemplateData.Presets[i].ID == ref {
				n := m.TemplateData.Presets[i].OrderNum()
				return &n
			}
		}
	}
	return NextRuleOrderNum(m)
}
