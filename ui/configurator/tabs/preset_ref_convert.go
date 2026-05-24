// File preset_ref_convert.go (SPEC 053) — one-way конверсия preset-ref → user rules.
//
// Алгоритм:
//   1. Expand preset с текущими varsValues → fragments.
//   2. Если preset имеет remote rule_set'ы → создать по одному kind=srs rule per remote set
//      (юзер должен повторно скачать .srs, потому что URL'ы перевязаны на content-addressed tag'и).
//   3. Если preset имеет inline rule_set → создать kind=inline rule с объединённым match'ом.
//   4. Если preset не имеет rule_set'ов (inline match в preset.rule) → один kind=inline rule.
//
// После Convert юзерское правило **теряет связь с template** — bump'ы template'а
// больше не подтягиваются. Это явный intent юзера (предупреждается в confirm dialog'е).
package tabs

import (
	"encoding/json"
	"fmt"

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
	frags, _, ok := build.ExpandPreset(tplPreset, vars)
	if !ok {
		return 0
	}

	created := 0
	outbound := ""
	if frags.RoutingRule != nil {
		if v, ok := frags.RoutingRule["outbound"].(string); ok {
			outbound = v
		} else if act, ok := frags.RoutingRule["action"].(string); ok && act == "reject" {
			// Map reject action → outbound literal "reject" (SPEC 053 contract).
			method, _ := frags.RoutingRule["method"].(string)
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

	// Без rule_set'ов — inline match прямо в preset.rule (Private IPs / BitTorrent / etc).
	if frags.RoutingRule != nil {
		match := make(map[string]interface{}, len(frags.RoutingRule))
		for k, v := range frags.RoutingRule {
			if k == "outbound" || k == "action" || k == "method" {
				continue
			}
			match[k] = v
		}
		cr := wizardmodels.RuleState{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: tplPreset.Label,
				Rule:  applyOutboundToMatchMap(match, outbound),
			},
			Enabled:          enabled,
			SelectedOutbound: outbound,
		}
		model.CustomRules = append(model.CustomRules, &cr)
		created++
	}
	return created
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
