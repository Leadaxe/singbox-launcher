// File dns_user_rule.go — типизированный user DNS rule entry для wizard model.
//
// До SPEC 062 user DNS rules жили как JSON-строка `model.DNSRulesText` и
// парсились на-лету в UI/save flow. Это блокировало drag-reorder в едином
// списке с preset DNS rules (нужно индексировать каждый user rule отдельно).
//
// Новая модель:
//   - DNSUserRule { Enabled bool, Body map[string]interface{} } — одно правило.
//   - model.DNSUserRules []DNSUserRule — типизированный список.
//   - model.DNSRulesText остаётся как deprecated derived view для raw-JSON
//     editor toggle (read из DNSUserRulesToText, write через DNSUserRulesFromText).
package models

import (
	"singbox-launcher/core/build"
)

// DNSUserRule — одно user-defined DNS правило.
//
// Body — flat sing-box dns rule поля (domain, domain_suffix, rule_set, server,
// ...). НЕ содержит kind/enabled (они top-level в state).
type DNSUserRule struct {
	Enabled bool
	Body    map[string]interface{}
}

// DNSUserRulesFromText парсит canonical `{"rules":[...]}` / массив / single
// object форму в типизированный список. Использует build.ParseDNSRulesText
// чтобы поведение match'ало legacy raw-editor mode.
//
// Каждое распарсенное правило получает Enabled=true (raw JSON форма не имеет
// per-rule enabled поля). Тех кто хочет per-rule toggle — используют list view.
func DNSUserRulesFromText(text string) []DNSUserRule {
	parsed, err := build.ParseDNSRulesText(text)
	if err != nil || len(parsed) == 0 {
		return nil
	}
	out := make([]DNSUserRule, 0, len(parsed))
	for _, raw := range parsed {
		body, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		// Strip kind/ref/enabled top-level fields (defensive: на случай если
		// юзер скопировал state-format rule в raw editor).
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			clean[k] = v
		}
		out = append(out, DNSUserRule{Enabled: true, Body: clean})
	}
	return out
}

// DNSUserRulesToText — обратная конверсия: типизированный список →
// canonical `{"rules":[...]}` JSON. Используется raw-editor toggle для
// показа serialized form. Disabled rules ВКЛЮЧАЮТСЯ в output (raw editor —
// holistic view; toggle через UI list).
func DNSUserRulesToText(rules []DNSUserRule) string {
	if len(rules) == 0 {
		return ""
	}
	bodies := make([]interface{}, 0, len(rules))
	for _, r := range rules {
		if len(r.Body) == 0 {
			continue
		}
		bodies = append(bodies, r.Body)
	}
	return build.DNSRulesToText(bodies)
}
