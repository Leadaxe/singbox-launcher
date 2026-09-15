package build

import corestate "singbox-launcher/core/state"

// presetRule — запись kind=preset в форме state v8: `ref` и `vars` на уровне
// записи, тела нет. Единственный писатель — конструктор состояния (SPEC 127 §0),
// поэтому в тестах пакета тело пресета руками больше не собирается.
func presetRule(ref string, vars map[string]string, enabled bool) corestate.Rule {
	r := corestate.NewPresetRule(ref, vars)
	r.Enabled = enabled
	return r
}
