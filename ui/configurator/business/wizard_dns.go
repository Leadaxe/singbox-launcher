package business

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// -----------------------------------------------------------------------------
// Public API — wizard DNS tab (state + template → model; model → sing-box)
// -----------------------------------------------------------------------------

// LoadPersistedWizardDNS copies dns_options from state.json into the model (servers + editor fields).
// Does not merge with the template; call ApplyWizardDNSTemplate after this.
func LoadPersistedWizardDNS(model *wizardmodels.WizardModel, p *wizardmodels.PersistedDNSState) {
	if model == nil || p == nil {
		return
	}
	model.DNSServers = append([]json.RawMessage(nil), p.Servers...)
	if len(p.Rules) > 0 {
		var objs []interface{}
		for _, raw := range p.Rules {
			var v interface{}
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			objs = append(objs, v)
		}
		model.DNSRulesText = DNSRulesToText(objs)
	} else {
		model.DNSRulesText = ""
	}
	// Скаляры strategy/final/cache/resolver — только state.vars (dns_*); см. MigrateDNSScalarsFromPersistedToSettingsVars + ApplyDNSVarsFromSettingsToModel.
}

// ApplyWizardDNSTemplate reconciles dns.servers with the effective template (config.dns + dns_options),
// prepends missing type=local from config if needed, and fills empty auxiliary fields
// (rules, final, strategy, default_domain_resolver) from dns_options / config.dns.
//
// Typical use:
//   - After LoadPersistedWizardDNS (persisted row wins until reconcile merges tags with template).
//   - On a fresh model (no persistence): same call; reconcile builds the list from the template only.
func ApplyWizardDNSTemplate(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	// effectiveTemplate(..., true) материализует secrets перед resolve —
	// сохраняет поведение прежнего effectiveWizardConfig. Order игнорируем.
	cfg, _ := effectiveTemplate(model, true)
	dnsObj := dnsSectionFromConfig(cfg)
	optsMap := parseDNSOptionsMap(model.TemplateData.DNSOptionsRaw)

	reconcileDNSServers(model, dnsObj, optsMap)
	prependMissingLocalServers(model, dnsObj)
	fillDNSAuxiliaryIfEmpty(model, cfg, dnsObj, optsMap)
}

// DNSTagLocked — true для tag'ов с `required: true` в template.dns_options.servers[].
// Locked entries нельзя toggle'нуть в UI (чекбокс greyed).
//
// SPEC unify: семантика "toggle block" (только required). Для "edit/delete block"
// (любой template entry) используй DNSTagFromTemplate.
func DNSTagLocked(model *wizardmodels.WizardModel, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, s := range templateDNSServersOf(model) {
		if t, _ := s["tag"].(string); t == tag {
			if r, ok := s["required"].(bool); ok && r {
				return true
			}
			return false
		}
	}
	return false
}

// DNSTagFromTemplate — true для любого tag'а присутствующего в
// template.dns_options.servers[]. Template-owned entries нельзя
// editировать/удалять (юзер может только toggle если не required).
//
// SPEC unify: семантика "edit/delete block" (все template entries).
// Для "toggle block" (только required) используй DNSTagLocked.
func DNSTagFromTemplate(model *wizardmodels.WizardModel, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, s := range templateDNSServersOf(model) {
		if t, _ := s["tag"].(string); t == tag {
			return true
		}
	}
	return false
}

// templateDNSServersOf — общий helper для DNSTagLocked / DNSTagFromTemplate.
// Парсит template.DNSOptionsRaw → servers list. Возвращает nil если template нет.
func templateDNSServersOf(model *wizardmodels.WizardModel) []map[string]interface{} {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	if len(model.TemplateData.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(model.TemplateData.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	return dnsOpt.Servers
}

// -----------------------------------------------------------------------------
// JSON helpers
// -----------------------------------------------------------------------------

func parseDNSOptionsMap(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func dnsSectionFromConfig(cfg map[string]json.RawMessage) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg["dns"]
	if !ok || len(raw) == 0 {
		return nil
	}
	var dnsObj map[string]interface{}
	if err := json.Unmarshal(raw, &dnsObj); err != nil {
		debuglog.WarnLog("dnsSectionFromConfig: %v", err)
		return nil
	}
	return dnsObj
}

func dnsOptsString(opts map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := opts[key]
		if !ok || len(raw) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func jsonString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func tagFromServerJSON(raw json.RawMessage) string {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return strings.TrimSpace(jsonString(m["tag"]))
}

func routeDefaultDomainResolver(route map[string]interface{}) string {
	v, ok := route["default_domain_resolver"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// -----------------------------------------------------------------------------
// Rules serialization (state ↔ editor text)
// -----------------------------------------------------------------------------

// PersistedDNSRulesForState builds dns_options.rules from the multiline editor text.
// On parse or marshal error returns nil (no rules key in state when empty).
func PersistedDNSRulesForState(rulesText string) []json.RawMessage {
	rulesText = strings.TrimSpace(rulesText)
	if rulesText == "" {
		return nil
	}
	parsed, err := build.ParseDNSRulesText(rulesText)
	if err != nil {
		return nil
	}
	var rules []json.RawMessage
	for _, r := range parsed {
		b, err := json.Marshal(r)
		if err != nil {
			return nil
		}
		rules = append(rules, json.RawMessage(b))
	}
	return rules
}

// DNSRulesToText — тонкий re-export `build.DNSRulesToText`. Сохранён
// под этим именем, чтобы не править все callsite'ы пакета (визард / тесты).
// Живая реализация — в `core/build/dns_merge.go` рядом с `ParseDNSRulesText`.
func DNSRulesToText(rules []interface{}) string {
	return build.DNSRulesToText(rules)
}
