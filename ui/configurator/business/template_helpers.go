package business

// template_helpers.go — единый дом для разрешения "effective" template config
// (params применены + vars подставлены) поверх GetEffectiveConfig.
//
// SPEC 070 Stage B: раньше существовали два почти-идентичных wrapper'а —
// effectiveWizardConfig (wizard_dns.go) и effectiveTemplateConfig
// (create_config.go). Оба оборачивали wizardtemplate.GetEffectiveConfig.
// Различались в двух наблюдаемых аспектах:
//   1. materializeSecrets — effectiveWizardConfig вызывал
//      MaterializeSecretsIfNeeded(model) перед resolve; effectiveTemplateConfig
//      нет (его единственный caller EffectiveConfigSection материализует сам).
//   2. order — effectiveTemplateConfig возвращал key-order, effectiveWizardConfig
//      его отбрасывал.
// Объединено в effectiveTemplate: всегда возвращает (config, order); поведение
// секретов управляется параметром materializeSecrets, чтобы не менять
// наблюдаемое поведение ни одного callsite.

import (
	"encoding/json"
	"runtime"

	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// effectiveTemplate returns the merged top-level config map (after applying
// params + substituting vars) and the key order.
//
// На неудаче GetEffectiveConfig — fallback на td.Config / td.ConfigOrder
// (template defaults без подставленных vars).
//
// materializeSecrets=true материализует type:"secret" vars в model.SettingsVars
// перед resolve (нужно DNS-табу: ApplyWizardDNSTemplate). false — caller сам
// отвечает за материализацию (EffectiveConfigSection делает это до вызова).
func effectiveTemplate(model *wizardmodels.WizardModel, materializeSecrets bool) (map[string]json.RawMessage, []string) {
	if model == nil || model.TemplateData == nil {
		return nil, nil
	}
	if materializeSecrets {
		MaterializeSecretsIfNeeded(model)
	}
	td := model.TemplateData
	config, order := td.Config, td.ConfigOrder
	if len(td.RawConfig) > 0 && (len(td.Params) > 0 || len(td.Vars) > 0) {
		effective, ord, err := wizardtemplate.GetEffectiveConfig(
			td.RawConfig,
			td.Params,
			runtime.GOOS,
			td.Vars,
			model.SettingsVars,
			td.RawTemplate,
		)
		if err == nil {
			return effective, ord
		}
		debuglog.WarnLog("effectiveTemplate: GetEffectiveConfig: %v", err)
	}
	return config, order
}
