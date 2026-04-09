package business

import (
	"strings"

	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

func templateDeclaresDNSWizardVar(vars []wizardtemplate.TemplateVar, name string) bool {
	for _, v := range vars {
		if v.Separator {
			continue
		}
		if v.Name == name {
			return true
		}
	}
	return false
}

// MigrateDNSScalarsFromPersistedToSettingsVars переносит устаревшие поля dns_options (strategy, final, …)
// в model.SettingsVars при отсутствии ключа с тем же именем (идемпотентно). Вызывать после restoreConfigParams.
func MigrateDNSScalarsFromPersistedToSettingsVars(p *wizardmodels.PersistedDNSState, settings map[string]string, vars []wizardtemplate.TemplateVar) {
	if p == nil || settings == nil {
		return
	}
	setIf := func(varName, val string) {
		if !templateDeclaresDNSWizardVar(vars, varName) {
			return
		}
		if _, exists := settings[varName]; exists {
			return
		}
		if strings.TrimSpace(val) == "" {
			return
		}
		settings[varName] = val
	}
	if p.Strategy != "" {
		setIf(wizardmodels.VarDNSStrategy, p.Strategy)
	}
	if p.Final != "" {
		setIf(wizardmodels.VarDNSFinal, p.Final)
	}
	if p.IndependentCache != nil {
		s := "false"
		if *p.IndependentCache {
			s = "true"
		}
		setIf(wizardmodels.VarDNSIndependentCache, s)
	}
	if !p.ResolverUnset && strings.TrimSpace(p.DefaultDomainResolver) != "" {
		setIf(wizardmodels.VarDNSDefaultDomainResolver, strings.TrimSpace(p.DefaultDomainResolver))
	}
}

// ApplyDNSVarsFromSettingsToModel выставляет поля DNS-модели из state.vars / дефолтов шаблона, если в шаблоне
// объявлены соответствующие dns_* переменные. Вызывать после ApplyWizardDNSTemplate.
func ApplyDNSVarsFromSettingsToModel(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	td := model.TemplateData
	if model.SettingsVars == nil {
		model.SettingsVars = make(map[string]string)
	}
	resolved := wizardtemplate.ResolveTemplateVars(td.Vars, model.SettingsVars, td.RawTemplate)

	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSStrategy) {
		if v, ok := model.SettingsVars[wizardmodels.VarDNSStrategy]; ok {
			model.DNSStrategy = strings.TrimSpace(v)
		} else {
			model.DNSStrategy = ""
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSIndependentCache) {
		if v, ok := model.SettingsVars[wizardmodels.VarDNSIndependentCache]; ok {
			b := strings.EqualFold(strings.TrimSpace(v), "true")
			model.DNSIndependentCache = ptrBool(b)
		} else {
			model.DNSIndependentCache = nil
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSFinal) {
		if v, ok := model.SettingsVars[wizardmodels.VarDNSFinal]; ok {
			model.DNSFinal = strings.TrimSpace(v)
		} else if rv, ok := resolved[wizardmodels.VarDNSFinal]; ok {
			model.DNSFinal = strings.TrimSpace(rv.Scalar)
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSDefaultDomainResolver) {
		if model.DefaultDomainResolverUnset {
			delete(model.SettingsVars, wizardmodels.VarDNSDefaultDomainResolver)
			model.DefaultDomainResolver = ""
		} else if v, ok := model.SettingsVars[wizardmodels.VarDNSDefaultDomainResolver]; ok {
			model.DefaultDomainResolver = strings.TrimSpace(v)
		} else if rv, ok := resolved[wizardmodels.VarDNSDefaultDomainResolver]; ok {
			model.DefaultDomainResolver = strings.TrimSpace(rv.Scalar)
		}
	}
}

// SyncDNSModelToSettingsVars копирует скаляры вкладки DNS в model.SettingsVars для объявленных dns_*.
func SyncDNSModelToSettingsVars(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	td := model.TemplateData
	if model.SettingsVars == nil {
		model.SettingsVars = make(map[string]string)
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSStrategy) {
		if strings.TrimSpace(model.DNSStrategy) == "" {
			delete(model.SettingsVars, wizardmodels.VarDNSStrategy)
		} else {
			model.SettingsVars[wizardmodels.VarDNSStrategy] = strings.TrimSpace(model.DNSStrategy)
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSIndependentCache) {
		if model.DNSIndependentCache == nil {
			delete(model.SettingsVars, wizardmodels.VarDNSIndependentCache)
		} else if *model.DNSIndependentCache {
			model.SettingsVars[wizardmodels.VarDNSIndependentCache] = "true"
		} else {
			model.SettingsVars[wizardmodels.VarDNSIndependentCache] = "false"
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSFinal) {
		if strings.TrimSpace(model.DNSFinal) == "" {
			delete(model.SettingsVars, wizardmodels.VarDNSFinal)
		} else {
			model.SettingsVars[wizardmodels.VarDNSFinal] = strings.TrimSpace(model.DNSFinal)
		}
	}
	if templateDeclaresDNSWizardVar(td.Vars, wizardmodels.VarDNSDefaultDomainResolver) {
		if model.DefaultDomainResolverUnset {
			delete(model.SettingsVars, wizardmodels.VarDNSDefaultDomainResolver)
		} else if strings.TrimSpace(model.DefaultDomainResolver) == "" {
			delete(model.SettingsVars, wizardmodels.VarDNSDefaultDomainResolver)
		} else {
			model.SettingsVars[wizardmodels.VarDNSDefaultDomainResolver] = strings.TrimSpace(model.DefaultDomainResolver)
		}
	}
}
