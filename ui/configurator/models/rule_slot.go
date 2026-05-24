// File rule_slot.go — единый упорядоченный список slot'ов для Rules tab UI.
//
// Проблема: до этого rendering правил был разделён на два массива
// (CustomRules legacy inline/srs + PresetRefs preset-ref'ы), что приводило
// к двум разделённым секциям в UI и невозможности drag-reorder между типами.
//
// Решение: один `model.RuleOrder []RuleSlot` определяет порядок отображения.
// Каждый RuleSlot ссылается на запись в CustomRules или PresetRefs по индексу
// — это позволяет:
//   - рендерить единый список в Rules tab с произвольным порядком kind'ов
//   - drag ↑↓ работает на индексах RuleOrder без перетряхивания CustomRules/PresetRefs
//   - build pipeline обходит RuleOrder → эмитит fragments в порядке state
//   - на Save: state.RulesV6 заполняется в порядке RuleOrder
//
// Источники истины:
//   - CustomRules / PresetRefs — что хранится (kind-specific данные)
//   - RuleOrder — в каком порядке отображается/эмитится
package models

// RuleSlotKind — тип ссылки на правило в CustomRules / PresetRefs.
type RuleSlotKind int

const (
	// SlotKindCustom — slot ссылается на model.CustomRules[Index] (legacy inline/srs).
	SlotKindCustom RuleSlotKind = iota
	// SlotKindPresetRef — slot ссылается на model.PresetRefs[Index] (preset-ref).
	SlotKindPresetRef
)

// RuleSlot — один элемент упорядоченного списка правил.
type RuleSlot struct {
	Kind  RuleSlotKind
	Index int
}

// RebuildRuleOrder перестраивает model.RuleOrder с нуля по текущим
// CustomRules + PresetRefs. Используется когда:
//   - произошёл add/delete (порядок сбит, легче пересобрать)
//   - load state.json v5 (только CustomRules; preset-ref'ов нет)
//   - load state.json v6 без сохранённого порядка (defensive)
//
// По умолчанию: сначала все CustomRules в их текущем порядке, затем PresetRefs.
// Для restore из v6 с сохранённым порядком — см. ApplyRuleOrderFromV6Rules.
func RebuildRuleOrder(m *WizardModel) {
	if m == nil {
		return
	}
	out := make([]RuleSlot, 0, len(m.CustomRules)+len(m.PresetRefs))
	for i := range m.CustomRules {
		out = append(out, RuleSlot{Kind: SlotKindCustom, Index: i})
	}
	for i := range m.PresetRefs {
		out = append(out, RuleSlot{Kind: SlotKindPresetRef, Index: i})
	}
	m.RuleOrder = out
}

// ReconcileRuleOrder — добавляет недостающие slots для свежесозданных правил
// и удаляет slots ссылающиеся на отсутствующие индексы. Безопасно после
// add/delete операций.
func ReconcileRuleOrder(m *WizardModel) {
	if m == nil {
		return
	}
	customSeen := make(map[int]bool, len(m.CustomRules))
	presetSeen := make(map[int]bool, len(m.PresetRefs))
	for _, s := range m.RuleOrder {
		switch s.Kind {
		case SlotKindCustom:
			customSeen[s.Index] = true
		case SlotKindPresetRef:
			presetSeen[s.Index] = true
		}
	}

	// Pass 1: keep valid existing slots (drop slots pointing past end).
	kept := make([]RuleSlot, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		switch s.Kind {
		case SlotKindCustom:
			if s.Index >= 0 && s.Index < len(m.CustomRules) {
				kept = append(kept, s)
			}
		case SlotKindPresetRef:
			if s.Index >= 0 && s.Index < len(m.PresetRefs) {
				kept = append(kept, s)
			}
		}
	}

	// Pass 2: append slots for items not yet ordered.
	for i := range m.CustomRules {
		if !customSeen[i] {
			kept = append(kept, RuleSlot{Kind: SlotKindCustom, Index: i})
		}
	}
	for i := range m.PresetRefs {
		if !presetSeen[i] {
			kept = append(kept, RuleSlot{Kind: SlotKindPresetRef, Index: i})
		}
	}

	m.RuleOrder = kept
}

// CompactRuleOrderIndices — пересчитывает индексы slot'ов после удаления
// записей в CustomRules/PresetRefs. Должен вызываться сразу после
// `append(slice[:i], slice[i+1:]...)` если индексы сдвинулись.
//
// Параметры: какой kind был изменён и какой индекс был удалён (после удаления
// все индексы >= removedIndex сдвигаются на -1).
func CompactRuleOrderIndices(m *WizardModel, kind RuleSlotKind, removedIndex int) {
	if m == nil {
		return
	}
	out := make([]RuleSlot, 0, len(m.RuleOrder))
	for _, s := range m.RuleOrder {
		if s.Kind != kind {
			out = append(out, s)
			continue
		}
		if s.Index == removedIndex {
			// этот slot указывал на удалённое правило — выбрасываем
			continue
		}
		if s.Index > removedIndex {
			s.Index--
		}
		out = append(out, s)
	}
	m.RuleOrder = out
}
