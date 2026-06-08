// File preset_ref_convert.go (SPEC 053) — one-way конверсия preset-ref → user rules.
//
// Алгоритм:
//  1. Expand preset с текущими varsValues → fragments.
//  2. Если preset имеет remote rule_set'ы → создать по одному kind=srs rule per remote set
//     (юзер должен повторно скачать .srs, потому что URL'ы перевязаны на content-addressed tag'и).
//  3. Если preset имеет inline rule_set → создать kind=inline rule с объединённым match'ом.
//  4. Если preset не имеет rule_set'ов (inline match в preset.rule) → один kind=inline rule.
//
// После Convert юзерское правило **теряет связь с template** — bump'ы template'а
// больше не подтягиваются. Это явный intent юзера (предупреждается в confirm dialog'е).
package tabs

import (
	"encoding/json"
	"fmt"
	"runtime"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// convertPresetRefToUserRules возвращает количество созданных user rule'ов.
func convertPresetRefToUserRules(
	model *wizardmodels.WizardModel,
	tplPreset *wizardtemplate.Preset,
	vars map[string]string,
	enabled bool,
) int {
	if model == nil || tplPreset == nil {
		return 0
	}
	frags, _, ok := build.ExpandPreset(tplPreset, vars, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return 0
	}

	created := 0
	outbound := ""
	// SPEC 067 Phase 9: preset.rules — slice. Для одно-rule presets берём первый.
	// Multi-rule presets (split-all-traffic) — также используем первую entry для
	// определения outbound; legacy convert path не претендует на полное one-to-one
	// mapping для multi-rule presets (юзер увидит один user-rule, не два).
	firstRule := firstNonNilRule(frags.RoutingRules)
	if firstRule != nil {
		if v, ok := firstRule["outbound"].(string); ok {
			outbound = v
		} else if act, ok := firstRule["action"].(string); ok && act == "reject" {
			// Map reject action → outbound literal "reject" (SPEC 053 contract).
			method, _ := firstRule["method"].(string)
			if method == "drop" {
				outbound = "drop"
			} else {
				outbound = "reject"
			}
		}
	}
	if outbound == "" {
		outbound = "direct-out"
	}

	// Если у preset'а есть rule_set'ы — создаём по одному правилу per set.
	if len(frags.RuleSets) > 0 {
		for _, rs := range frags.RuleSets {
			name := fmt.Sprintf("%s — %v", tplPreset.Label, rs["tag"])
			rsType, _ := rs["type"].(string)
			switch rsType {
			case "inline":
				// Inline rule_set → kind=inline с match-полями из rule_set[].rules[0].
				match := extractFirstRule(rs)
				cr := wizardmodels.RuleState{
					Rule: wizardtemplate.TemplateSelectableRule{
						Label: name,
						Rule:  applyOutboundToMatchMap(match, outbound),
					},
					Enabled:          enabled,
					SelectedOutbound: outbound,
				}
				model.CustomRules = append(model.CustomRules, &cr)
				created++
			case "remote":
				// Remote → kind=srs (user'у надо повторно скачать).
				if url, ok := rs["url"].(string); ok && url != "" {
					rsRaw, _ := json.Marshal(map[string]interface{}{
						"type":   "remote",
						"format": "binary",
						"url":    url,
					})
					cr := wizardmodels.RuleState{
						Rule: wizardtemplate.TemplateSelectableRule{
							Label:    name,
							Rule:     map[string]interface{}{"outbound": outbound},
							RuleSets: []json.RawMessage{rsRaw},
						},
						Enabled:          enabled,
						SelectedOutbound: outbound,
					}
					model.CustomRules = append(model.CustomRules, &cr)
					created++
				}
			}
		}
		return created
	}

	// Без rule_set'ов — inline match прямо в preset.rules[] (Private IPs / BitTorrent / etc).
	// SPEC 067 Phase 9: эмитим по одному user-rule per entry из frags.RoutingRules.
	for idx, rr := range frags.RoutingRules {
		if rr == nil {
			continue
		}
		// Per-rule outbound — если у конкретной rule есть свой outbound/action,
		// используем его (multi-rule preset как split-all-traffic — каждая
		// rule имеет свой outbound).
		ruleOutbound := outbound
		if v, ok := rr["outbound"].(string); ok && v != "" {
			ruleOutbound = v
		} else if act, ok := rr["action"].(string); ok && act == "reject" {
			method, _ := rr["method"].(string)
			if method == "drop" {
				ruleOutbound = "drop"
			} else {
				ruleOutbound = "reject"
			}
		}
		match := make(map[string]interface{}, len(rr))
		for k, v := range rr {
			if k == "outbound" || k == "action" || k == "method" {
				continue
			}
			match[k] = v
		}
		label := tplPreset.Label
		if len(frags.RoutingRules) > 1 {
			label = fmt.Sprintf("%s [%d]", tplPreset.Label, idx+1)
		}
		cr := wizardmodels.RuleState{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: label,
				Rule:  applyOutboundToMatchMap(match, ruleOutbound),
			},
			Enabled:          enabled,
			SelectedOutbound: ruleOutbound,
		}
		model.CustomRules = append(model.CustomRules, &cr)
		created++
	}
	return created
}

// firstNonNilRule — helper: возвращает первый не-nil rule из slice.
func firstNonNilRule(rules []map[string]interface{}) map[string]interface{} {
	for _, r := range rules {
		if r != nil {
			return r
		}
	}
	return nil
}

// extractFirstRule — берёт первое entry из rule_set.rules[].
func extractFirstRule(rs map[string]interface{}) map[string]interface{} {
	rulesArr, ok := rs["rules"].([]interface{})
	if !ok || len(rulesArr) == 0 {
		return map[string]interface{}{}
	}
	first, ok := rulesArr[0].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(first))
	for k, v := range first {
		out[k] = v
	}
	return out
}

// applyOutboundToMatchMap — встраивает outbound в match map (для legacy CustomRule.Rule).
// Для reject/drop литералов добавляем outbound, build pipeline переведёт в action/method.
func applyOutboundToMatchMap(m map[string]interface{}, outbound string) map[string]interface{} {
	if m == nil {
		m = map[string]interface{}{}
	}
	m["outbound"] = outbound
	return m
}
