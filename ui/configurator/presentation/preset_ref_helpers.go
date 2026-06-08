// Package presentation — UI presenter helpers.
//
// File preset_ref_helpers.go — SPEC 053 helpers для добавления preset-ref'ов
// в model. Изолировано в отдельном файле чтобы избежать дополнительных
// зависимостей в presenter_state.go.
package presentation

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// extractTemplateDNSTags перенесён в business.ExtractTemplateDNSTags
// (SPEC 070 Stage B): копия в presentation существовала только чтобы избежать
// import cycle business→presentation; presentation уже импортирует business,
// поэтому единственный дом теперь в business/dns_helpers.go.

// applyPresetEnabledOverrides — после SyncDNSOptionsWithActivePresets
// проходит по kind=preset entries в state.DNS и применяет toggle overrides
// из PresetRefState.DNSServerEnabled / DNSRuleEnabled.
// (SPEC 056-R-N follow-up: per-server toggle живёт в PresetRefState.)
func applyPresetEnabledOverrides(dns *state.DNSOptions, presetRefs []*wizardmodels.PresetRefState) {
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
		if s.Kind != state.DNSServerKindPreset {
			continue
		}
		presetID := state.PresetIDFromServerRef(s.Ref)
		localTag := state.LocalTagFromServerRef(s.Ref)
		pr, ok := refByPresetID[presetID]
		if !ok || localTag == "" {
			continue
		}
		s.Enabled = pr.IsDNSServerEnabled(localTag)
	}
	for i := range dns.Rules {
		r := &dns.Rules[i]
		if r.Kind != state.DNSRuleKindPreset {
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
func populatePresetEnabledFromState(presetRefs []*wizardmodels.PresetRefState, dns state.DNSOptions) {
	refByPresetID := make(map[string]*wizardmodels.PresetRefState, len(presetRefs))
	for _, pr := range presetRefs {
		if pr != nil && pr.Ref != "" {
			refByPresetID[pr.Ref] = pr
		}
	}
	for _, s := range dns.Servers {
		if s.Kind != state.DNSServerKindPreset || s.Ref == "" {
			continue
		}
		presetID := state.PresetIDFromServerRef(s.Ref)
		localTag := state.LocalTagFromServerRef(s.Ref)
		pr, ok := refByPresetID[presetID]
		if !ok || localTag == "" {
			continue
		}
		pr.SetDNSServerEnabled(localTag, s.Enabled)
	}
	for _, r := range dns.Rules {
		if r.Kind != state.DNSRuleKindPreset || r.Ref == "" {
			continue
		}
		pr, ok := refByPresetID[r.Ref]
		if !ok {
			continue
		}
		pr.SetDNSRuleEnabled(r.Enabled)
	}
}

// populateUserDNSFromState — restore kind=user DNS servers/rules из v6
// state.DNS обратно в model.DNSServers / model.DNSRulesText.
//
// SPEC 056 phase 7 (rename DNSV6→DNS) regression fix: legacy v5 path
// (LoadPersistedWizardDNS, читающий sf.DNSOptions) для v6-файлов больше
// не срабатывает — parseCurrent заполняет только sf.DNS, оставляя
// sf.DNSOptions == nil. Без этого helper'а user-added DNS-сервера и
// user DNS rules терялись после Save+reopen, потому что
// ApplyWizardDNSTemplate перезаполняет model.DNSServers только из
// template-секции (kind=template), а kind=user никем не восстанавливается.
//
// Идемпотентно:
//   - Server skip'аем если такой tag уже есть в model.DNSServers (legacy
//     v5 path мог уже его положить во время v5→v6 round-trip миграции —
//     не делаем double-add).
//   - DNSRulesText трогаем только если он пустой (legacy v5 путь может
//     уже его выставить из sf.DNSOptions.Rules).
//
// kind=template/preset entries здесь НЕ обрабатываются:
//   - template servers заполняются ApplyWizardDNSTemplate из шаблона;
//     их enabled-state восстанавливается через SyncStateV6ToDNSOverrides →
//     DNSTemplateOverrides;
//   - preset servers/rules — через populatePresetEnabledFromState.
//
// applyDNSServerEnabledFromState overlays the saved `enabled` flag from canonical
// state DNS onto model.DNSServers by tag — for ALL servers (template + user).
//
// Why: populateUserDNSFromState only restores kind=user entries, and the
// DNSTemplateOverrides map (SyncStateV6ToDNSOverrides) is never applied back onto
// model.DNSServers — yet the UI checkboxes and DNSEnabledTagOptions read enabled
// from model.DNSServers. So a user's enable/disable of a TEMPLATE server (e.g.
// enabling cloudflare_udp) was lost on reopen, which additionally dropped that tag
// from the enabled-server options and reset the Default-resolver / Final selects.
// Must run AFTER ApplyWizardDNSTemplate built the full server list and BEFORE the
// DNS selects refresh. No-op when dns has no servers (legacy v5 path is unaffected).
func applyDNSServerEnabledFromState(model *wizardmodels.WizardModel, dns state.DNSOptions) {
	if model == nil || len(dns.Servers) == 0 {
		return
	}
	enabledByTag := make(map[string]bool, len(dns.Servers))
	for _, s := range dns.Servers {
		if t := strings.TrimSpace(s.Tag); t != "" {
			enabledByTag[t] = s.Enabled
		}
	}
	for i, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag = strings.TrimSpace(tag); tag == "" {
			continue
		}
		en, ok := enabledByTag[tag]
		if !ok {
			continue
		}
		if cur, _ := m["enabled"].(bool); cur == en {
			continue
		}
		m["enabled"] = en
		if b, err := json.Marshal(m); err == nil {
			model.DNSServers[i] = json.RawMessage(b)
		}
	}
}

func populateUserDNSFromState(model *wizardmodels.WizardModel, dns state.DNSOptions) {
	if model == nil {
		return
	}

	// 1. Servers — append kind=user в model.DNSServers, дедупликация по tag.
	existingTags := make(map[string]struct{}, len(model.DNSServers))
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if t, ok := m["tag"].(string); ok {
			if t = strings.TrimSpace(t); t != "" {
				existingTags[t] = struct{}{}
			}
		}
	}
	for _, s := range dns.Servers {
		if s.Kind != state.DNSServerKindUser {
			continue
		}
		tag := strings.TrimSpace(s.Tag)
		if tag == "" {
			continue
		}
		if _, dup := existingTags[tag]; dup {
			continue
		}
		// Reconstruct wizard JSON shape (inverse of syncDNSServersOnly's
		// kind=user branch in preset_ref_sync.go): tag + enabled top-level,
		// плюс flatten Body. Body уже не содержит kind/ref/enabled
		// (Unmarshal в DNSServer их выкидывает), но защищаемся от tag-
		// коллизии (поле Tag — source of truth).
		entry := make(map[string]interface{}, 2+len(s.Body))
		entry["tag"] = tag
		entry["enabled"] = s.Enabled
		for k, v := range s.Body {
			switch k {
			case "kind", "ref", "enabled", "tag":
				continue
			}
			entry[k] = v
		}
		b, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		model.DNSServers = append(model.DNSServers, json.RawMessage(b))
		existingTags[tag] = struct{}{}
	}

	// 2. Rules — overwrite DNSRulesText только если оно пустое.
	if strings.TrimSpace(model.DNSRulesText) != "" {
		return
	}
	var userRules []interface{}
	for _, r := range dns.Rules {
		if r.Kind != state.DNSRuleKindUser {
			continue
		}
		if len(r.Body) == 0 {
			continue
		}
		body := make(map[string]interface{}, len(r.Body))
		for k, v := range r.Body {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			body[k] = v
		}
		userRules = append(userRules, body)
	}
	if len(userRules) > 0 {
		model.DNSRulesText = build.DNSRulesToText(userRules)
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
