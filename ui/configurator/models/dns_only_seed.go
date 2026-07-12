package models

// dns_only_seed.go — SPEC 093. DNS-only пресеты (у которых есть dns_servers/
// dns_rules, но НЕТ route-правил — напр. fakeip) авто-сидятся как постоянные
// DNS-правила: включены по умолчанию (default true), отключаются toggle'ом на
// вкладке DNS, но НЕ удаляются и НЕ показываются в библиотеке/на вкладке Rules.

// EnsureDNSOnlyPresetsSeeded гарантирует, что для каждого DNS-only пресета
// шаблона в модели есть preset-ref. Безопасно и идемпотентно:
//   - добавляет ТОЛЬКО отсутствующий ref (Enabled=true, Vars/DNSRuleEnabled не
//     задаются → default enabled);
//   - НЕ трогает существующий ref — поэтому выключенное пользователем состояние
//     (DNSRuleEnabled=false) переживает Save/Load (см. SPEC 093 round-trip);
//   - слоты RuleOrder/DNSRuleOrder НЕ добавляет — это делают Reconcile*RuleOrder,
//     которые вызываются следом.
//
// route-level Enabled ОБЯЗАН быть true: build активирует DNS-серверы/правила
// пресета только при state.Rules[ref].Enabled==true, а toggle на DNS-вкладке
// дизейблится при !Enabled. Пользовательский off живёт в DNSRuleEnabled.
func EnsureDNSOnlyPresetsSeeded(m *WizardModel) {
	if m == nil || m.TemplateData == nil {
		return
	}
	existing := make(map[string]bool, len(m.PresetRefs))
	for _, pr := range m.PresetRefs {
		if pr != nil {
			existing[pr.Ref] = true
		}
	}
	for i := range m.TemplateData.Presets {
		p := &m.TemplateData.Presets[i]
		if !p.IsDNSOnly() || existing[p.ID] {
			continue
		}
		m.PresetRefs = append(m.PresetRefs, &PresetRefState{
			Ref:     p.ID,
			Enabled: true,
		})
		existing[p.ID] = true
	}
}
