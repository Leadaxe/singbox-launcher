// File preset_ref_sync.go — мосты UI model <-> core/state v6 (SPEC 053).
//
// Sync функции вызываются на Save (UI → state) и Load (state → UI). Это два
// независимых поля рядом со старыми CustomRules/DNSOptions:
//
//	UI model.PresetRefs            <-> state.RulesV6 (kind=preset entries)
//	UI model.DNSTemplateOverrides  <-> state.DNSV6.TemplateServers
//
// Подход «параллельное хранилище» нужен пока UI Rules tab продолжает
// работать на legacy CustomRules для kind=inline/srs (без переписывания).
// Когда UI Phase 6 полностью переедет на v6 — sync можно упростить.
package models

import (
	"encoding/json"

	v6 "singbox-launcher/core/state/v6"
)

// SyncAllRulesToStateRulesV6 — full sync model rules → state.RulesV6 (БЕЗ порядка).
//
// Кладёт в state.RulesV6:
//   - preset-ref'ы из model.PresetRefs как kind=preset (сначала)
//   - inline/srs правила из model.CustomRules как kind=inline/srs (после)
//
// **Не сохраняет порядок RuleOrder.** Для save с правильным порядком
// используется SyncRulesByOrderToStateRulesV6 (см. ниже). Эта функция —
// fallback для случаев когда RuleOrder не заполнен (нечасто).
func SyncAllRulesToStateRulesV6(presetRefs []*PresetRefState, customRules []*RuleState) []v6.Rule {
	out := make([]v6.Rule, 0, len(presetRefs)+len(customRules))

	// 1. Preset-refs
	out = append(out, SyncPresetRefsToStateRules(presetRefs)...)

	// 2. Legacy custom rules → inline/srs
	for _, cr := range customRules {
		if cr == nil {
			continue
		}
		r := customRuleStateToV6Rule(cr)
		if r != nil {
			out = append(out, *r)
		}
	}

	return out
}

// SyncRulesByOrderToStateRulesV6 — sync с сохранением порядка RuleOrder.
// Обходит slots в порядке RuleOrder, dispatch'ит по kind, эмитит v6.Rule.
//
// Гарантирует что state.RulesV6 имеет тот же порядок что UI Rules tab видит.
// Это критично для build pipeline (emit в правильном порядке) и round-trip
// load→save→load (порядок не теряется).
func SyncRulesByOrderToStateRulesV6(order []RuleSlot, presetRefs []*PresetRefState, customRules []*RuleState) []v6.Rule {
	if len(order) == 0 {
		// Fallback: используем legacy concat если RuleOrder пуст.
		return SyncAllRulesToStateRulesV6(presetRefs, customRules)
	}
	out := make([]v6.Rule, 0, len(order))
	for _, slot := range order {
		switch slot.Kind {
		case SlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(presetRefs) {
				continue
			}
			pr := presetRefs[slot.Index]
			if pr == nil || pr.Ref == "" {
				continue
			}
			vars := pr.Vars
			if vars == nil {
				vars = map[string]string{}
			}
			body, _ := jsonMarshalPreset(vars)
			out = append(out, v6.Rule{
				Kind:    v6.RuleKindPreset,
				Ref:     pr.Ref,
				Enabled: pr.Enabled,
				Body:    body,
			})
		case SlotKindCustom:
			if slot.Index < 0 || slot.Index >= len(customRules) {
				continue
			}
			cr := customRules[slot.Index]
			if cr == nil {
				continue
			}
			r := customRuleStateToV6Rule(cr)
			if r != nil {
				out = append(out, *r)
			}
		}
	}
	return out
}

// jsonMarshalPreset — helper для serialization PresetBody (избавляет от
// дублирования в SyncPresetRefsToStateRules / SyncRulesByOrderToStateRulesV6).
func jsonMarshalPreset(vars map[string]string) ([]byte, error) {
	return json.Marshal(v6.PresetBody{Vars: vars})
}

// RuleOrderFromStateRulesV6 — обратная конверсия: восстанавливает model.RuleOrder
// из state.RulesV6, чтобы UI после load увидел правила в том же порядке как
// они были при save.
//
// Параметры (presetRefs, customRules) должны быть уже заполнены (после
// SyncStateRulesToPresetRefs + restoreCustomRules) — функция строит slot'ы
// сопоставляя ref'ы / id'шники.
//
// Возвращает order. Если совпадения по ref/id нет (e.g. legacy state v5
// без RulesV6), возвращает пустой list → caller должен сделать RebuildRuleOrder.
func RuleOrderFromStateRulesV6(rules []v6.Rule, presetRefs []*PresetRefState, customRules []*RuleState) []RuleSlot {
	if len(rules) == 0 {
		return nil
	}
	// Index lookups by ref / id / label.
	prByRef := make(map[string]int, len(presetRefs))
	for i, pr := range presetRefs {
		if pr != nil {
			prByRef[pr.Ref] = i
		}
	}
	crByID := make(map[string]int, len(customRules))
	crByLabel := make(map[string]int, len(customRules))
	for i, cr := range customRules {
		if cr == nil {
			continue
		}
		id := stableRuleID(cr)
		if id != "" {
			crByID[id] = i
		}
		if cr.Rule.Label != "" {
			crByLabel[cr.Rule.Label] = i
		}
	}

	out := make([]RuleSlot, 0, len(rules))
	for _, r := range rules {
		switch r.Kind {
		case v6.RuleKindPreset:
			if idx, ok := prByRef[r.Ref]; ok {
				out = append(out, RuleSlot{Kind: SlotKindPresetRef, Index: idx})
			}
		case v6.RuleKindInline, v6.RuleKindSrs:
			if r.ID != "" {
				if idx, ok := crByID[r.ID]; ok {
					out = append(out, RuleSlot{Kind: SlotKindCustom, Index: idx})
					continue
				}
			}
			// Fallback по label из decoded body.
			if body, err := r.DecodeBody(); err == nil {
				var name string
				switch b := body.(type) {
				case *v6.InlineBody:
					name = b.Name
				case *v6.SrsBody:
					name = b.Name
				}
				if name != "" {
					if idx, ok := crByLabel[name]; ok {
						out = append(out, RuleSlot{Kind: SlotKindCustom, Index: idx})
					}
				}
			}
		}
	}
	return out
}

// customRuleStateToV6Rule — конверсия RuleState (legacy) → v6.Rule (kind=inline|srs).
func customRuleStateToV6Rule(rs *RuleState) *v6.Rule {
	if rs == nil {
		return nil
	}
	label := rs.Rule.Label
	outbound := rs.SelectedOutbound

	// kind=srs если есть rule_set'ы remote
	if len(rs.Rule.RuleSets) > 0 {
		for _, rsRaw := range rs.Rule.RuleSets {
			var probe struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			}
			if err := json.Unmarshal(rsRaw, &probe); err == nil && probe.Type == "remote" && probe.URL != "" {
				body, _ := json.Marshal(v6.SrsBody{
					Name:     label,
					SrsURL:   probe.URL,
					Outbound: outbound,
				})
				id := stableRuleID(rs)
				return &v6.Rule{
					Kind:    v6.RuleKindSrs,
					ID:      id,
					Enabled: rs.Enabled,
					Body:    body,
				}
			}
		}
	}

	// kind=inline (default)
	match := stripOutboundAction(rs.Rule.Rule)
	if len(match) == 0 {
		return nil
	}
	body, _ := json.Marshal(v6.InlineBody{
		Name:     label,
		Match:    match,
		Outbound: outbound,
	})
	id := stableRuleID(rs)
	return &v6.Rule{
		Kind:    v6.RuleKindInline,
		ID:      id,
		Enabled: rs.Enabled,
		Body:    body,
	}
}

// stableRuleID — generate ID based on label hash if not already set.
// Используется при первой конверсии legacy → v6 (после этого ID становится частью save state).
func stableRuleID(rs *RuleState) string {
	// Используем label + outbound как сид — стабильно между save'ами для того же правила.
	// Для production требуется ULID; здесь упрощённо — hash от label достаточен.
	if rs.Rule.Label == "" {
		return "rule-unnamed"
	}
	return "rule-" + sanitizeIDPart(rs.Rule.Label)
}

func sanitizeIDPart(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, byte(r))
		} else if r == ' ' {
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "rule"
	}
	return string(out)
}

func stripOutboundAction(rule map[string]interface{}) map[string]interface{} {
	if rule == nil {
		return nil
	}
	out := make(map[string]interface{}, len(rule))
	for k, v := range rule {
		switch k {
		case "outbound", "action", "method":
			// drop
		default:
			out[k] = v
		}
	}
	return out
}

// SyncDNSFullToStateV6 — full sync model DNS → state.DNSV6.
//
// Помимо TemplateServers overrides конвертит model.DNSServers (legacy
// []json.RawMessage) в TemplateServers если tag совпадает с template-defined.
//
// SPEC 057: extras удалены. Серверы с tag'ом НЕ из template — игнорируются
// (раньше попадали в ExtraServers). dnsRulesText целиком игнорируется —
// state не хранит user-defined DNS rules; они должны жить в preset.dns_rule.
//
// templateDNSTags — set tag'ов известных template-defined серверов.
// Параметр dnsRulesText сохранён в сигнатуре для backward-compat callsite'ов
// (parser.go / create_config.go), но не используется.
func SyncDNSFullToStateV6(
	dnsServers []json.RawMessage,
	dnsRulesText string,
	templateOverrides map[string]bool,
	templateDNSTags map[string]bool,
) v6.DNSConfig {
	_ = dnsRulesText // SPEC 057: extra rules dropped — no state field
	cfg := v6.DNSConfig{}

	// 1. Apply explicit overrides (юзер кликал чекбокс template-сервера).
	if len(templateOverrides) > 0 {
		cfg.TemplateServers = make(map[string]v6.TemplateServerOvr, len(templateOverrides))
		for tag, enabled := range templateOverrides {
			cfg.TemplateServers[tag] = v6.TemplateServerOvr{Enabled: enabled}
		}
	}

	// 2. Walk model.DNSServers — template-overrides only (extras dropped).
	for _, raw := range dnsServers {
		var srv map[string]interface{}
		if err := json.Unmarshal(raw, &srv); err != nil {
			continue
		}
		tag, _ := srv["tag"].(string)
		if tag == "" {
			continue
		}
		if templateDNSTags != nil && templateDNSTags[tag] {
			if enabled, ok := srv["enabled"].(bool); ok {
				if cfg.TemplateServers == nil {
					cfg.TemplateServers = make(map[string]v6.TemplateServerOvr)
				}
				if _, alreadySet := cfg.TemplateServers[tag]; !alreadySet {
					cfg.TemplateServers[tag] = v6.TemplateServerOvr{Enabled: enabled}
				}
			}
		}
		// SPEC 057: non-template tag — silently dropped (UI must not let user
		// add custom DNS servers anymore; if a legacy save sneaks one in, we
		// drop it on next save).
	}

	return cfg
}

// SyncPresetRefsToStateRules — UI → state. Конвертит model.PresetRefs в []v6.Rule.
func SyncPresetRefsToStateRules(refs []*PresetRefState) []v6.Rule {
	if len(refs) == 0 {
		return nil
	}
	out := make([]v6.Rule, 0, len(refs))
	for _, r := range refs {
		if r == nil || r.Ref == "" {
			continue
		}
		vars := r.Vars
		if vars == nil {
			vars = map[string]string{}
		}
		body, _ := json.Marshal(v6.PresetBody{Vars: vars})
		out = append(out, v6.Rule{
			Kind:    v6.RuleKindPreset,
			Ref:     r.Ref,
			Enabled: r.Enabled,
			Body:    body,
		})
	}
	return out
}

// SyncStateRulesToPresetRefs — state → UI. Возвращает model.PresetRefs из state.RulesV6.
// Только kind=preset попадают; остальные kind'ы (inline/srs) идут через legacy CustomRules
// path (см. core/state/load.go::legacyCustomRulesFromV6).
func SyncStateRulesToPresetRefs(rules []v6.Rule) []*PresetRefState {
	if len(rules) == 0 {
		return nil
	}
	out := make([]*PresetRefState, 0, len(rules))
	for _, r := range rules {
		if r.Kind != v6.RuleKindPreset || r.Ref == "" {
			continue
		}
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		pb, _ := body.(*v6.PresetBody)
		if pb == nil {
			continue
		}
		out = append(out, &PresetRefState{
			Ref:     r.Ref,
			Enabled: r.Enabled,
			Vars:    pb.Vars,
		})
	}
	return out
}

// SyncDNSToStateV6 — UI → state. Конвертит DNSTemplateOverrides + текущие DNSServers/Rules
// в v6.DNSConfig.
//
// Семантика:
//   - overrides идут в TemplateServers as-is
//   - DNSServers RawMessage[] которые НЕ совпадают с известными template-default'ами
//     становятся extra_servers (приближение: считаем что любой server без template tag — extra)
//
// Пока упрощённая версия — пока юзер не редактирует через новый DNS tab UI Phase 7,
// TemplateServers будет пуст и extra_servers заполнится из legacy DNSServers.
func SyncDNSToStateV6(overrides map[string]bool, dnsServers []json.RawMessage, dnsRulesText string) v6.DNSConfig {
	cfg := v6.DNSConfig{}
	if len(overrides) > 0 {
		cfg.TemplateServers = make(map[string]v6.TemplateServerOvr, len(overrides))
		for tag, enabled := range overrides {
			cfg.TemplateServers[tag] = v6.TemplateServerOvr{Enabled: enabled}
		}
	}
	// dnsServers / dnsRulesText пока не конвертируются в extras напрямую (riskier);
	// SaveState основной path остаётся через legacy DNSOptions. ExtraServers пустой
	// — Save v6 запустится только когда есть preset-refs, и тогда tplDNS purgesы выберут
	// нужные из template defaults+overrides.
	return cfg
}

// SyncStateV6ToDNSOverrides — state → UI. Возвращает overrides map из state.DNSV6.
func SyncStateV6ToDNSOverrides(dns v6.DNSConfig) map[string]bool {
	if len(dns.TemplateServers) == 0 {
		return nil
	}
	out := make(map[string]bool, len(dns.TemplateServers))
	for tag, ovr := range dns.TemplateServers {
		out[tag] = ovr.Enabled
	}
	return out
}
