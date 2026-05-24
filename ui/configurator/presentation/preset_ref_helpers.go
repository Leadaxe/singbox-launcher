// Package presentation — UI presenter helpers.
//
// File preset_ref_helpers.go — SPEC 053 helpers для добавления preset-ref'ов
// в model. Изолировано в отдельном файле чтобы избежать дополнительных
// зависимостей в presenter_state.go.
package presentation

import (
	"encoding/json"

	corev6 "singbox-launcher/core/state/v6"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// extractTemplateDNSTags — выдаёт set tag'ов template-defined DNS-серверов из
// template.DNSOptionsRaw (используется для split'а на overrides vs extras при save v6).
func extractTemplateDNSTags(td *wizardtemplate.TemplateData) map[string]bool {
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	out := make(map[string]bool, len(dnsOpt.Servers))
	for _, s := range dnsOpt.Servers {
		if tag, ok := s["tag"].(string); ok && tag != "" {
			out[tag] = true
		}
	}
	return out
}

// applyPresetEnabledOverrides — после SyncDNSOptionsWithActivePresets
// проходит по kind=preset entries в state.DNS и применяет toggle overrides
// из PresetRefState.DNSServerEnabled / DNSRuleEnabled.
// (SPEC 056-R-N follow-up: per-server toggle живёт в PresetRefState.)
func applyPresetEnabledOverrides(dns *corev6.DNSOptions, presetRefs []*wizardmodels.PresetRefState) {
	if dns == nil {
		return
	}
	refByPresetID := make(map[string]*wizardmodels.PresetRefState, len(presetRefs))
	for _, pr := range presetRefs {
		if pr != nil && pr.Ref != "" {
			refByPresetID[pr.Ref] = pr
		}
	}
	for i := range dns.Servers {
		s := &dns.Servers[i]
		if s.Kind != corev6.DNSServerKindPreset {
			continue
		}
		presetID := corev6.PresetIDFromServerRef(s.Ref)
		localTag := corev6.LocalTagFromServerRef(s.Ref)
		pr, ok := refByPresetID[presetID]
		if !ok || localTag == "" {
			continue
		}
		s.Enabled = pr.IsDNSServerEnabled(localTag)
	}
	for i := range dns.Rules {
		r := &dns.Rules[i]
		if r.Kind != corev6.DNSRuleKindPreset {
			continue
		}
		pr, ok := refByPresetID[r.Ref]
		if !ok {
			continue
		}
		r.Enabled = pr.IsDNSRuleEnabled()
	}
}

// populatePresetEnabledFromState — обратная конверсия для load:
// из state.DNS.{Servers,Rules}[kind=preset] заполняет PresetRefState.
// Default true (отсутствие в map) — но если в state Enabled=false,
// явно записываем false чтобы UI чекбокс показал правильное состояние.
func populatePresetEnabledFromState(presetRefs []*wizardmodels.PresetRefState, dns corev6.DNSOptions) {
	refByPresetID := make(map[string]*wizardmodels.PresetRefState, len(presetRefs))
	for _, pr := range presetRefs {
		if pr != nil && pr.Ref != "" {
			refByPresetID[pr.Ref] = pr
		}
	}
	for _, s := range dns.Servers {
		if s.Kind != corev6.DNSServerKindPreset || s.Ref == "" {
			continue
		}
		presetID := corev6.PresetIDFromServerRef(s.Ref)
		localTag := corev6.LocalTagFromServerRef(s.Ref)
		pr, ok := refByPresetID[presetID]
		if !ok || localTag == "" {
			continue
		}
		pr.SetDNSServerEnabled(localTag, s.Enabled)
	}
	for _, r := range dns.Rules {
		if r.Kind != corev6.DNSRuleKindPreset || r.Ref == "" {
			continue
		}
		pr, ok := refByPresetID[r.Ref]
		if !ok {
			continue
		}
		pr.SetDNSRuleEnabled(r.Enabled)
	}
}

// PresetRefForUI — helper struct для создания preset-ref правил из UI.
type PresetRefForUI struct {
	Ref     string
	Enabled bool
	Vars    map[string]string
}

// AppendTo — добавляет PresetRefForUI как PresetRefState в model.PresetRefs.
// Идемпотентно: если ref уже присутствует, обновляет vars/enabled.
func (p PresetRefForUI) AppendTo(model *wizardmodels.WizardModel) {
	if model == nil || p.Ref == "" {
		return
	}
	// Проверка дубля — обновляем существующий.
	for _, existing := range model.PresetRefs {
		if existing != nil && existing.Ref == p.Ref {
			existing.Enabled = p.Enabled
			if p.Vars != nil {
				existing.Vars = p.Vars
			}
			return
		}
	}
	newIdx := len(model.PresetRefs)
	model.PresetRefs = append(model.PresetRefs, &wizardmodels.PresetRefState{
		Ref:     p.Ref,
		Enabled: p.Enabled,
		Vars:    p.Vars,
	})
	// Append slot to RuleOrder so the new preset-ref shows up in the unified
	// list at the end (юзер потом может drag вверх если хочет приоритет).
	model.RuleOrder = append(model.RuleOrder, wizardmodels.RuleSlot{
		Kind:  wizardmodels.SlotKindPresetRef,
		Index: newIdx,
	})
}
