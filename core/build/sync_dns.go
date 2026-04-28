package build

import (
	"strings"

	"singbox-launcher/core/template"
)

// Имена template-vars, в которые DNS-scalars материализуются из state.
// Зеркалят `ui/wizard/models/dns_vars.go::VarDNS*`; дублируются здесь, чтобы
// core/build не импортировал ui/.
const (
	varDNSStrategy              = "dns_strategy"
	varDNSIndependentCache      = "dns_independent_cache"
	varDNSDefaultDomainResolver = "dns_default_domain_resolver"
	varDNSFinal                 = "dns_final"
)

// DNSScalars — DNS-поля state, которые материализуются в template vars.
//
// Соответствует полям WizardModel.DNS* (`DNSStrategy`, `DNSIndependentCache`,
// `DNSFinal`, `DefaultDomainResolver`, `DefaultDomainResolverUnset`); вызывающий
// слой извлекает их в эту структуру перед передачей в core/build.
type DNSScalars struct {
	Strategy              string
	IndependentCache      *bool
	Final                 string
	DefaultDomainResolver string

	// DefaultDomainResolverUnset — true, если пользователь намеренно очистил
	// поле (вместо «не задано» с unset=false). Влияет на наличие ключа
	// в final route.default_domain_resolver.
	DefaultDomainResolverUnset bool
}

// ApplyDNSScalarsToVars копирует DNS-scalars из cfg в vars-map, но только
// для тех имён, что объявлены как template-vars (через `template.TemplateData.Vars`).
//
// Семантика 1:1 с `ui/wizard/business/dns_settings_vars.go::SyncDNSModelToSettingsVars`:
//   - если var объявлена в шаблоне И значение пустое (или Unset для resolver) → удаляем ключ;
//   - если var объявлена И значение непустое → ставим (после TrimSpace);
//   - если var НЕ объявлена в шаблоне → не трогаем vars (даже если значение есть).
//
// Pure: единственный side effect — мутация переданного vars map.
//
// nil-tolerant: nil td / nil vars / zero DNSScalars → no-op.
func ApplyDNSScalarsToVars(td *template.TemplateData, cfg DNSScalars, vars map[string]string) {
	if td == nil || vars == nil {
		return
	}

	if templateDeclaresVar(td.Vars, varDNSStrategy) {
		if v := strings.TrimSpace(cfg.Strategy); v != "" {
			vars[varDNSStrategy] = v
		} else {
			delete(vars, varDNSStrategy)
		}
	}

	if templateDeclaresVar(td.Vars, varDNSIndependentCache) {
		if cfg.IndependentCache == nil {
			delete(vars, varDNSIndependentCache)
		} else if *cfg.IndependentCache {
			vars[varDNSIndependentCache] = "true"
		} else {
			vars[varDNSIndependentCache] = "false"
		}
	}

	if templateDeclaresVar(td.Vars, varDNSFinal) {
		if v := strings.TrimSpace(cfg.Final); v != "" {
			vars[varDNSFinal] = v
		} else {
			delete(vars, varDNSFinal)
		}
	}

	if templateDeclaresVar(td.Vars, varDNSDefaultDomainResolver) {
		switch {
		case cfg.DefaultDomainResolverUnset:
			delete(vars, varDNSDefaultDomainResolver)
		case strings.TrimSpace(cfg.DefaultDomainResolver) == "":
			delete(vars, varDNSDefaultDomainResolver)
		default:
			vars[varDNSDefaultDomainResolver] = strings.TrimSpace(cfg.DefaultDomainResolver)
		}
	}
}

// templateDeclaresVar — true, если в td.Vars есть запись с указанным name
// (игнорирует separator-записи).
func templateDeclaresVar(vars []template.TemplateVar, name string) bool {
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
