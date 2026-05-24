// Package outboundutil — shared утилиты для outbound-sentinels.
//
// Reject/drop литералы (см. SPEC 053): юзер выбирает в picker'е
// `"reject"` / `"drop"` как значение outbound; на emit в config.json они
// конвертируются в `action: "reject"` (+ `method: "drop"`).
//
// Этот пакет — single source of truth для конверсии. Используется обоими
// сторонами:
//   - core/build/preset_expand.go (server-side при build pipeline)
//   - ui/configurator/business/rule_utils.go (UI при формировании rule из vars)
package outboundutil

// ApplyOutboundToRule applies outbound selection to a (cloned) rule map.
//
// Handles three cases:
//   - "reject" → sets action: reject (no method), removes outbound
//   - "drop"   → sets action: reject + method: drop, removes outbound
//   - other    → sets outbound: <as is>, removes action/method
//
// Returns the SAME map reference for caller convenience (modifies in-place).
// Caller is responsible for cloning if immutability needed.
func ApplyOutboundToRule(rule map[string]interface{}, outbound string) map[string]interface{} {
	if rule == nil {
		return rule
	}
	switch outbound {
	case "reject":
		delete(rule, "outbound")
		rule["action"] = "reject"
		delete(rule, "method")
	case "drop":
		delete(rule, "outbound")
		rule["action"] = "reject"
		rule["method"] = "drop"
	default:
		if outbound != "" {
			rule["outbound"] = outbound
			delete(rule, "action")
			delete(rule, "method")
		}
	}
	return rule
}
