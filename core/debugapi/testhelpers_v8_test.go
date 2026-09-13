package debugapi

import "singbox-launcher/core/state"

// presetRule — запись kind=preset в форме state v8: `ref` и `vars` на уровне
// записи, тела нет (SPEC 127 §0).
func presetRule(ref string, vars map[string]string, enabled bool) state.Rule {
	r := state.NewPresetRule(ref, vars)
	r.Enabled = enabled
	return r
}

// inlineRule — включённая запись kind=inline в форме state v8: `name` полем
// записи, `body` = правило sing-box целиком (матчеры плюс цель).
func inlineRule(name string, match map[string]interface{}, outbound string) state.Rule {
	r := state.NewInlineRule(name, match, outbound)
	r.Enabled = true
	return r
}
