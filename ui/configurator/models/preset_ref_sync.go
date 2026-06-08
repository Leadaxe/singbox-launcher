// File preset_ref_sync.go — мосты UI model <-> core/state v6 (SPEC 053).
//
// Sync функции вызываются на Save (UI → state) и Load (state → UI). Это два
// независимых поля рядом со старыми CustomRules/DNSOptions:
//
//	UI model.PresetRefs            <-> state.Rules (kind=preset entries)
//	UI model.DNSTemplateOverrides  <-> state.DNS.TemplateServers
//
// Подход «параллельное хранилище» нужен пока UI Rules tab продолжает
// работать на legacy CustomRules для kind=inline/srs (без переписывания).
// Когда UI Phase 6 полностью переедет на v6 — sync можно упростить.
package models

import (
	"encoding/json"
	"singbox-launcher/core/state"
)

// SyncAllRulesToStateRulesV6 — full sync model rules → state.Rules (БЕЗ порядка).
//
// Кладёт в state.Rules:
//   - preset-ref'ы из model.PresetRefs как kind=preset (сначала)
//   - inline/srs правила из model.CustomRules как kind=inline/srs (после)
//
// **Не сохраняет порядок RuleOrder.** Для save с правильным порядком
// используется SyncRulesByOrderToStateRulesV6 (см. ниже). Эта функция —
// fallback для случаев когда RuleOrder не заполнен (нечасто).
func SyncAllRulesToStateRulesV6(presetRefs []*PresetRefState, customRules []*RuleState) []state.Rule {
	out := make([]state.Rule, 0, len(presetRefs)+len(customRules))

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
// Обходит slots в порядке RuleOrder, dispatch'ит по kind, эмитит state.Rule.
//
// Гарантирует что state.Rules имеет тот же порядок что UI Rules tab видит.
// Это критично для build pipeline (emit в правильном порядке) и round-trip
// load→save→load (порядок не теряется).
func SyncRulesByOrderToStateRulesV6(order []RuleSlot, presetRefs []*PresetRefState, customRules []*RuleState) []state.Rule {
	if len(order) == 0 {
		// Fallback: используем legacy concat если RuleOrder пуст.
		return SyncAllRulesToStateRulesV6(presetRefs, customRules)
	}
	out := make([]state.Rule, 0, len(order))
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
			out = append(out, state.Rule{
				Kind:    state.RuleKindPreset,
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
	return json.Marshal(state.PresetBody{Vars: vars})
}

// RuleOrderFromStateRulesV6 — обратная конверсия: восстанавливает model.RuleOrder
// из state.Rules, чтобы UI после load увидел правила в том же порядке как
// они были при save.
//
// Параметры (presetRefs, customRules) должны быть уже заполнены (после
// SyncStateRulesToPresetRefs + restoreCustomRules) — функция строит slot'ы
// сопоставляя ref'ы / id'шники.
//
// Возвращает order. Если совпадения по ref/id нет (e.g. legacy state v5
// без RulesV6), возвращает пустой list → caller должен сделать RebuildRuleOrder.
func RuleOrderFromStateRulesV6(rules []state.Rule, presetRefs []*PresetRefState, customRules []*RuleState) []RuleSlot {
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
	// SPEC 063: identity для kind=inline/srs = state.StableRuleID(r), которая
	// для legacy RuleState равна sanitize(rs.Rule.Label) (body.name берётся
	// из Label при конверсии). Совпадение по identity → совпадение по label.
	crByLabel := make(map[string]int, len(customRules))
	for i, cr := range customRules {
		if cr == nil {
			continue
		}
		if cr.Rule.Label != "" {
			crByLabel[cr.Rule.Label] = i
		}
	}

	out := make([]RuleSlot, 0, len(rules))
	for _, r := range rules {
		switch r.Kind {
		case state.RuleKindPreset:
			if idx, ok := prByRef[r.Ref]; ok {
				out = append(out, RuleSlot{Kind: SlotKindPresetRef, Index: idx})
			}
		case state.RuleKindInline, state.RuleKindSrs:
			// Lookup по body.name (=label) — единственная точка истины с SPEC 063.
			if body, err := r.DecodeBody(); err == nil {
				var name string
				switch b := body.(type) {
				case *state.InlineBody:
					name = b.Name
				case *state.SrsBody:
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

// customRuleStateToV6Rule — конверсия RuleState (legacy) → state.Rule (kind=inline|srs).
//
// SPEC 063: identity не stored, вычисляется из body.name. Body.Name = rs.Rule.Label —
// то же, что прежний `stableRuleID` использовал как seed. Так что identity (=
// state.StableRuleID для нового state.Rule) семантически совпадает со старым
// `"rule-" + sanitize(label)` в плане «стабильна между save'ами для того же
// правила». Точное строковое представление поменялось — старый префикс
// "rule-" удалён — но это безопасно: identity больше нигде не сохраняется
// и не используется как filename (filename теперь URL-derived, см. issue #77).
func customRuleStateToV6Rule(rs *RuleState) *state.Rule {
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
				body, _ := json.Marshal(state.SrsBody{
					Name:     label,
					SrsURL:   probe.URL,
					Outbound: outbound,
				})
				return &state.Rule{
					Kind:    state.RuleKindSrs,
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
	body, _ := json.Marshal(state.InlineBody{
		Name:     label,
		Match:    match,
		Outbound: outbound,
	})
	return &state.Rule{
		Kind:    state.RuleKindInline,
		Enabled: rs.Enabled,
		Body:    body,
	}
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

// SyncDNSByOrderToState — SPEC 062-F-N: full DNS sync с уважением к DNSRuleOrder.
//
// Зеркало SyncRulesByOrderToStateRulesV6 для DNS rules. Servers собираются
// через syncDNSServersOnly (kind=template/preset/user
// классификация по tag); rules собираются обходом DNSRuleOrder:
//
//	for slot in order:
//	  if slot.Kind == DNSSlotKindPresetRef:
//	    emit DNSRule{Kind=preset, Ref=presetRefs[idx].Ref, Enabled=...}
//	  if slot.Kind == DNSSlotKindUser:
//	    emit DNSRule{Kind=user, Body=userRules[idx].Body, Enabled=...}
//
// Если order пустой — fallback: rules из dnsRulesText как раньше (legacy
// state.json который ещё не имеет DNSRuleOrder).
//
// **Не вызывает** SyncDNSOptionsWithActivePresets — это делается caller'ом
// после получения результата. При непустом DNSRuleOrder presenter может
// пропустить SyncDNSOptionsWithActivePresets вообще, так как DNSRuleOrder
// уже определяет какие preset-rules в state.DNS.Rules.
func SyncDNSByOrderToState(
	order []DNSRuleSlot,
	presetRefs []*PresetRefState,
	userRules []DNSUserRule,
	dnsServers []json.RawMessage,
	dnsRulesText string,
	templateOverrides map[string]bool,
	templateDNSTags map[string]bool,
) state.DNSOptions {
	// Servers — server portion only (без rules portion).
	cfg := syncDNSServersOnly(dnsServers, templateOverrides, templateDNSTags)

	// Rules — order-aware (preferred) ИЛИ legacy fallback из dnsRulesText.
	if len(order) > 0 {
		cfg.Rules = buildDNSRulesFromOrder(order, presetRefs, userRules)
	} else {
		cfg.Rules = buildDNSRulesFromText(dnsRulesText)
	}
	return cfg
}

// syncDNSServersOnly — только server portion DNS sync (без rules).
// Используется SyncDNSByOrderToState.
func syncDNSServersOnly(
	dnsServers []json.RawMessage,
	templateOverrides map[string]bool,
	templateDNSTags map[string]bool,
) state.DNSOptions {
	cfg := state.DNSOptions{}

	explicitOverrides := make(map[string]bool, len(templateOverrides))
	for tag, enabled := range templateOverrides {
		explicitOverrides[tag] = enabled
	}

	seenTemplateTags := make(map[string]bool)
	seenUserTags := make(map[string]bool)
	seenPresetRefs := make(map[string]bool)

	for _, raw := range dnsServers {
		var srv map[string]interface{}
		if err := json.Unmarshal(raw, &srv); err != nil {
			continue
		}
		tag, _ := srv["tag"].(string)
		if tag == "" {
			continue
		}

		if templateDNSTags == nil || !templateDNSTags[tag] {
			if idx := indexColon(tag); idx > 0 {
				ref := tag
				if seenPresetRefs[ref] {
					continue
				}
				enabled := true
				if e, ok := srv["enabled"].(bool); ok {
					enabled = e
				}
				cfg.Servers = append(cfg.Servers, state.DNSServer{
					Kind:    state.DNSServerKindPreset,
					Ref:     ref,
					Enabled: enabled,
				})
				seenPresetRefs[ref] = true
				continue
			}
		}

		if templateDNSTags != nil && templateDNSTags[tag] {
			if seenTemplateTags[tag] {
				continue
			}
			enabled := true
			if e, ok := explicitOverrides[tag]; ok {
				enabled = e
			} else if e, ok := srv["enabled"].(bool); ok {
				enabled = e
			}
			cfg.Servers = append(cfg.Servers, state.DNSServer{
				Kind:    state.DNSServerKindTemplate,
				Tag:     tag,
				Enabled: enabled,
			})
			seenTemplateTags[tag] = true
		} else {
			if seenUserTags[tag] {
				continue
			}
			enabled := true
			if e, ok := srv["enabled"].(bool); ok {
				enabled = e
			}
			body := make(map[string]interface{}, len(srv))
			for k, v := range srv {
				if k == "enabled" {
					continue
				}
				body[k] = v
			}
			cfg.Servers = append(cfg.Servers, state.DNSServer{
				Kind:    state.DNSServerKindUser,
				Tag:     tag,
				Enabled: enabled,
				Body:    body,
			})
			seenUserTags[tag] = true
		}
	}

	for tag, enabled := range explicitOverrides {
		if seenTemplateTags[tag] {
			continue
		}
		if templateDNSTags != nil && !templateDNSTags[tag] {
			continue
		}
		cfg.Servers = append(cfg.Servers, state.DNSServer{
			Kind:    state.DNSServerKindTemplate,
			Tag:     tag,
			Enabled: enabled,
		})
		seenTemplateTags[tag] = true
	}

	return cfg
}

// buildDNSRulesFromOrder — обходит DNSRuleOrder, dispatch по slot.Kind,
// emit'ит state.DNSRule. Skip'ает slots с disabled toggle (user rule с
// !Enabled, preset-ref с !pr.Enabled).
//
// Preset entry: Kind=preset, Ref=pr.Ref, Enabled=pr.IsDNSRuleEnabled().
// User entry: Kind=user, Body=clone(ur.Body), Enabled=ur.Enabled.
//
// Rules в порядке slot'ов — это order in which sing-box применит first-match.
func buildDNSRulesFromOrder(
	order []DNSRuleSlot,
	presetRefs []*PresetRefState,
	userRules []DNSUserRule,
) []state.DNSRule {
	if len(order) == 0 {
		return nil
	}
	out := make([]state.DNSRule, 0, len(order))
	for _, slot := range order {
		switch slot.Kind {
		case DNSSlotKindPresetRef:
			if slot.Index < 0 || slot.Index >= len(presetRefs) {
				continue
			}
			pr := presetRefs[slot.Index]
			if pr == nil || pr.Ref == "" {
				continue
			}
			if !pr.Enabled {
				// Preset выключен на уровне route rule — не эмитим dns_rule
				// (тот же inv что SyncDNSOptionsWithActivePresets: !r.Enabled → skip).
				continue
			}
			out = append(out, state.DNSRule{
				Kind:    state.DNSRuleKindPreset,
				Ref:     pr.Ref,
				Enabled: pr.IsDNSRuleEnabled(),
			})
		case DNSSlotKindUser:
			if slot.Index < 0 || slot.Index >= len(userRules) {
				continue
			}
			ur := userRules[slot.Index]
			if len(ur.Body) == 0 {
				continue
			}
			body := make(map[string]interface{}, len(ur.Body))
			for k, v := range ur.Body {
				switch k {
				case "kind", "ref", "enabled":
					continue
				}
				body[k] = v
			}
			out = append(out, state.DNSRule{
				Kind:    state.DNSRuleKindUser,
				Enabled: ur.Enabled,
				Body:    body,
			})
		}
	}
	return out
}

// buildDNSRulesFromText — fallback для callsite'ов с пустым DNSRuleOrder:
// парсит DNSRulesText как user rules.
func buildDNSRulesFromText(dnsRulesText string) []state.DNSRule {
	if dnsRulesText == "" {
		return nil
	}
	var parsed struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(dnsRulesText), &parsed); err != nil {
		return nil
	}
	out := make([]state.DNSRule, 0, len(parsed.Rules))
	for _, body := range parsed.Rules {
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			if k == "enabled" {
				continue
			}
			clean[k] = v
		}
		out = append(out, state.DNSRule{
			Kind:    state.DNSRuleKindUser,
			Enabled: true,
			Body:    clean,
		})
	}
	return out
}

// DNSRuleOrderFromStateRules — обратная конверсия: из state.DNS.Rules
// восстанавливает model.DNSRuleOrder + populates user rules into a slice
// returned alongside (caller must assign to model.DNSUserRules).
//
// Параметры:
//   - rules — state.DNS.Rules
//   - presetRefs — already restored model.PresetRefs (для маппинга kind=preset ref→Index)
//
// Возвращает:
//   - order — slots in same order as rules slice
//   - userRules — kind=user entries в том же порядке, что в rules (slot.Index
//     ссылается в этот slice)
//
// Если rules пустой → возвращает (nil, nil); caller fallback'ается на
// RebuildDNSRuleOrder.
func DNSRuleOrderFromStateRules(
	rules []state.DNSRule,
	presetRefs []*PresetRefState,
) (order []DNSRuleSlot, userRules []DNSUserRule) {
	if len(rules) == 0 {
		return nil, nil
	}
	prByRef := make(map[string]int, len(presetRefs))
	for i, pr := range presetRefs {
		if pr != nil && pr.Ref != "" {
			prByRef[pr.Ref] = i
		}
	}
	order = make([]DNSRuleSlot, 0, len(rules))
	userRules = make([]DNSUserRule, 0)
	for _, r := range rules {
		switch r.Kind {
		case state.DNSRuleKindPreset:
			if idx, ok := prByRef[r.Ref]; ok {
				order = append(order, DNSRuleSlot{Kind: DNSSlotKindPresetRef, Index: idx})
			}
			// Если preset-ref не найден в presetRefs (broken) — slot
			// просто не появится; ReconcileDNSRuleOrder ничего не добавит
			// (нет лишних preset-ref'ов).
		case state.DNSRuleKindUser:
			body := make(map[string]interface{}, len(r.Body))
			for k, v := range r.Body {
				switch k {
				case "kind", "ref", "enabled":
					continue
				}
				body[k] = v
			}
			newIdx := len(userRules)
			userRules = append(userRules, DNSUserRule{
				Enabled: r.Enabled,
				Body:    body,
			})
			order = append(order, DNSRuleSlot{Kind: DNSSlotKindUser, Index: newIdx})
		}
	}
	return order, userRules
}

// indexColon — мелкий helper: позиция первого ':' или -1.
func indexColon(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}

// SyncPresetRefsToStateRules — UI → state. Конвертит model.PresetRefs в []state.Rule.
func SyncPresetRefsToStateRules(refs []*PresetRefState) []state.Rule {
	if len(refs) == 0 {
		return nil
	}
	out := make([]state.Rule, 0, len(refs))
	for _, r := range refs {
		if r == nil || r.Ref == "" {
			continue
		}
		vars := r.Vars
		if vars == nil {
			vars = map[string]string{}
		}
		body, _ := json.Marshal(state.PresetBody{Vars: vars})
		out = append(out, state.Rule{
			Kind:    state.RuleKindPreset,
			Ref:     r.Ref,
			Enabled: r.Enabled,
			Body:    body,
		})
	}
	return out
}

// SyncStateRulesToPresetRefs — state → UI. Возвращает model.PresetRefs из state.Rules.
// Только kind=preset попадают; остальные kind'ы (inline/srs) идут через legacy CustomRules
// projection (см. core/state.LegacyCustomRulesView, SPEC 070 ADR-070-2).
func SyncStateRulesToPresetRefs(rules []state.Rule) []*PresetRefState {
	if len(rules) == 0 {
		return nil
	}
	out := make([]*PresetRefState, 0, len(rules))
	for _, r := range rules {
		if r.Kind != state.RuleKindPreset || r.Ref == "" {
			continue
		}
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		pb, _ := body.(*state.PresetBody)
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

// SyncStateV6ToDNSOverrides — state → UI. Возвращает overrides map из state.DNS
// (только entries с kind=template, формат map[tag]→enabled).
//
// Используется UI DNS tab чтобы восстановить состояние чекбоксов template-серверов
// после load state'а.
func SyncStateV6ToDNSOverrides(dns state.DNSOptions) map[string]bool {
	if len(dns.Servers) == 0 {
		return nil
	}
	out := make(map[string]bool, len(dns.Servers))
	for _, s := range dns.Servers {
		if s.Kind != state.DNSServerKindTemplate {
			continue
		}
		out[s.Tag] = s.Enabled
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
