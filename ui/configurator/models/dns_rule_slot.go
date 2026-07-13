// File dns_rule_slot.go — единый упорядоченный список slot'ов для DNS rules
// в Wizard DNS tab (SPEC 062-F-N WIZARD_DNS_RULES_UNIFIED_ORDER).
//
// Зеркало rule_slot.go (SPEC 053 для route rules): rendering DNS rules был
// разделён на две секции (preset bundled DNS rules сверху + user rules внизу),
// что фиксировало порядок «presets first, users second» в state.DNS.Rules[]
// независимо от того, как юзер хотел видеть приоритеты. Sing-box применяет
// first-match wins → preset rules ВСЕГДА выигрывали у user rules с
// пересекающимися match-условиями.
//
// Решение: один `model.DNSRuleOrder []DNSRuleSlot` определяет порядок
// отображения и порядок эмита в state.DNS.Rules. Каждый DNSRuleSlot ссылается
// на запись в DNSUserRules или PresetRefs по индексу — это позволяет:
//   - рендерить единый список в DNS tab с произвольным порядком kind'ов
//   - drag ↑↓ работает на индексах DNSRuleOrder без перетряхивания подлежащих
//     слайсов
//   - на Save: state.DNS.Rules заполняется в порядке DNSRuleOrder
//   - на Load: DNSRuleOrder восстанавливается из state.DNS.Rules slice order
//
// Источники истины:
//   - DNSUserRules / PresetRefs — что хранится (kind-specific данные)
//   - DNSRuleOrder — в каком порядке отображается/эмитится
package models

// DNSRuleSlotKind — тип ссылки на правило в DNSUserRules / PresetRefs.
type DNSRuleSlotKind int

const (
	// DNSSlotKindUser — slot ссылается на model.DNSUserRules[Index] (user DNS rule).
	DNSSlotKindUser DNSRuleSlotKind = iota
	// DNSSlotKindPresetRef — slot ссылается на model.PresetRefs[Index].dns_rule
	// (bundled DNS rule from preset).
	DNSSlotKindPresetRef
)

// DNSRuleSlot — один элемент упорядоченного списка DNS-правил.
type DNSRuleSlot struct {
	Kind  DNSRuleSlotKind
	Index int
}

// RebuildDNSRuleOrder перестраивает model.DNSRuleOrder с нуля по текущим
// DNSUserRules + PresetRefs. Используется когда:
//   - произошёл add/delete user-rule (порядок сбит, легче пересобрать)
//   - load state.json без сохранённого DNS-порядка (defensive)
//
// По умолчанию: сначала все DNSUserRules в их текущем порядке, затем PresetRefs.
// Для restore из state.DNS.Rules с сохранённым порядком — см.
// DNSRuleOrderFromStateRules в preset_ref_sync.go.
func RebuildDNSRuleOrder(m *WizardModel) {
	if m == nil {
		return
	}
	out := make([]DNSRuleSlot, 0, len(m.DNSUserRules)+len(m.PresetRefs))
	for i := range m.DNSUserRules {
		out = append(out, DNSRuleSlot{Kind: DNSSlotKindUser, Index: i})
	}
	for i := range m.PresetRefs {
		out = append(out, DNSRuleSlot{Kind: DNSSlotKindPresetRef, Index: i})
	}
	m.DNSRuleOrder = out
}

// ReconcileDNSRuleOrder — добавляет недостающие slots для свежесозданных
// rules и удаляет slots ссылающиеся на отсутствующие индексы. Безопасно после
// add/delete операций.
//
// SPEC 062 specifics: preset-ref slots only count if the preset actually has
// a `dns_rule` in the template. Bare route-rule presets ("Private IPs", "Block
// Ads") would otherwise occupy DNSRuleOrder entries that the UI renders as
// invisible (buildSingleDNSPresetRuleRow returns early on no-dns-rule),
// which makes the «is last visible row?» check overcount and leaves the down
// arrow active on what looks like the last row. Filter them out here so
// DNSRuleOrder.length matches the number of rows actually rendered.
func ReconcileDNSRuleOrder(m *WizardModel) {
	if m == nil {
		return
	}
	userSeen := make(map[int]bool, len(m.DNSUserRules))
	presetSeen := make(map[int]bool, len(m.PresetRefs))
	for _, s := range m.DNSRuleOrder {
		switch s.Kind {
		case DNSSlotKindUser:
			userSeen[s.Index] = true
		case DNSSlotKindPresetRef:
			presetSeen[s.Index] = true
		}
	}

	// Pass 1: keep valid existing slots (drop slots pointing past end OR
	// pointing at a preset with no dns_rule fragment).
	kept := make([]DNSRuleSlot, 0, len(m.DNSRuleOrder))
	for _, s := range m.DNSRuleOrder {
		switch s.Kind {
		case DNSSlotKindUser:
			if s.Index >= 0 && s.Index < len(m.DNSUserRules) {
				kept = append(kept, s)
			}
		case DNSSlotKindPresetRef:
			if s.Index >= 0 && s.Index < len(m.PresetRefs) &&
				presetHasDNSRuleInModel(m, m.PresetRefs[s.Index].Ref) {
				kept = append(kept, s)
			}
		}
	}

	// Pass 2: append slots for items not yet ordered.
	for i := range m.DNSUserRules {
		if !userSeen[i] {
			kept = append(kept, DNSRuleSlot{Kind: DNSSlotKindUser, Index: i})
		}
	}
	for i := range m.PresetRefs {
		if !presetSeen[i] && presetHasDNSRuleInModel(m, m.PresetRefs[i].Ref) {
			kept = append(kept, DNSRuleSlot{Kind: DNSSlotKindPresetRef, Index: i})
		}
	}

	m.DNSRuleOrder = kept
}

// presetHasDNSRuleInModel — true if the template-preset with given ref ID
// defines a `dns_rule` fragment. Walks model.TemplateData.Presets; returns
// false if TemplateData is missing or the preset isn't found (defensive —
// caller treats unknown presets as «no DNS contribution»).
func presetHasDNSRuleInModel(m *WizardModel, ref string) bool {
	if m == nil || m.TemplateData == nil || ref == "" {
		return false
	}
	for i := range m.TemplateData.Presets {
		if m.TemplateData.Presets[i].ID == ref {
			return m.TemplateData.Presets[i].PresetHasDNSRule()
		}
	}
	return false
}

// CompactDNSRuleOrderIndices — пересчитывает индексы slot'ов после удаления
// записей в DNSUserRules/PresetRefs. Должен вызываться сразу после
// `append(slice[:i], slice[i+1:]...)` если индексы сдвинулись.
//
// Параметры: какой kind был изменён и какой индекс был удалён (после удаления
// все индексы >= removedIndex сдвигаются на -1).
func CompactDNSRuleOrderIndices(m *WizardModel, kind DNSRuleSlotKind, removedIndex int) {
	if m == nil {
		return
	}
	out := make([]DNSRuleSlot, 0, len(m.DNSRuleOrder))
	for _, s := range m.DNSRuleOrder {
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
	m.DNSRuleOrder = out
}
