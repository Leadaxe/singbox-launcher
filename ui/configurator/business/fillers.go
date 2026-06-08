package business

import (
	"encoding/json"
	"strings"

	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// -----------------------------------------------------------------------------
// Rules / final / strategy / cache / default resolver — only when model gaps
// -----------------------------------------------------------------------------

const dnsRulesPlaceholderMarker = `"rule_set":"example"`

func dnsRulesTextNeedsFill(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	return strings.Contains(s, dnsRulesPlaceholderMarker) && strings.Contains(s, `"server":"tag"`)
}

func fillDNSAuxiliaryIfEmpty(model *wizardmodels.WizardModel, cfg map[string]json.RawMessage, dnsObj map[string]interface{}, optsMap map[string]json.RawMessage) {
	if model == nil || model.TemplateData == nil {
		return
	}
	vd := model.TemplateData.Vars
	hasOpts := optsMap != nil

	if dnsRulesTextNeedsFill(model.DNSRulesText) {
		model.DNSRulesText = pickDNSRulesText(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSFinal) && strings.TrimSpace(model.DNSFinal) == "" {
		model.DNSFinal = pickDNSFinal(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSStrategy) && strings.TrimSpace(model.DNSStrategy) == "" {
		model.DNSStrategy = pickDNSStrategy(hasOpts, optsMap, dnsObj)
	}
	// SPEC: independent_cache deprecated в sing-box 1.14.0 — extract удалён.
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSDefaultDomainResolver) {
		fillDefaultDomainResolverIfEmpty(model, cfg, optsMap)
	}
}

func pickDNSRulesText(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	if hasOpts {
		if raw, ok := optsMap["rules"]; ok {
			var rules []interface{}
			if json.Unmarshal(raw, &rules) == nil {
				return DNSRulesToText(rules)
			}
			debuglog.DebugLog("pickDNSRulesText: dns_options.rules: invalid JSON")
		}
	}
	if dnsObj != nil {
		if rules, ok := dnsObj["rules"].([]interface{}); ok {
			return DNSRulesToText(rules)
		}
	}
	return ""
}

func pickDNSFinal(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	if hasOpts {
		if f := dnsOptsString(optsMap, "dns.final", "final"); f != "" {
			return f
		}
	}
	if dnsObj != nil {
		if f, ok := dnsObj["final"].(string); ok {
			return strings.TrimSpace(f)
		}
	}
	return ""
}

// pickDNSStrategy: сначала скелет config.dns.strategy, затем перекрытие dns_options.strategy шаблона (у второго приоритет).
// Сохранённый state: поле strategy уже в модели до ApplyWizardDNSTemplate; fill вызывает pick только если в модели пусто.
func pickDNSStrategy(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	base := ""
	if dnsObj != nil {
		base = jsonString(dnsObj["strategy"])
	}
	if hasOpts {
		if raw, ok := optsMap["strategy"]; ok && len(raw) > 0 {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return base
}

// pickDNSIndependentCache + ptrBool УДАЛЕНЫ: independent_cache deprecated
// в sing-box 1.14.0.

func fillDefaultDomainResolverIfEmpty(model *wizardmodels.WizardModel, cfg map[string]json.RawMessage, optsMap map[string]json.RawMessage) {
	if model == nil || model.DefaultDomainResolverUnset {
		return
	}
	if strings.TrimSpace(model.DefaultDomainResolver) != "" {
		return
	}
	if optsMap != nil {
		if dr := dnsOptsString(optsMap, "default_domain_resolver", "route.default_domain_resolver"); dr != "" {
			model.DefaultDomainResolver = dr
			return
		}
	}
	if model.TemplateData != nil {
		if dr := strings.TrimSpace(model.TemplateData.DefaultDomainResolver); dr != "" {
			model.DefaultDomainResolver = dr
			return
		}
	}
	rawRoute, ok := cfg["route"]
	if !ok || len(rawRoute) == 0 {
		return
	}
	var route map[string]interface{}
	if json.Unmarshal(rawRoute, &route) != nil {
		return
	}
	if dr := routeDefaultDomainResolver(route); dr != "" {
		model.DefaultDomainResolver = dr
	}
}
