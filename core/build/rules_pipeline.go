package build

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/template"
	v6 "singbox-launcher/core/state/v6"
	"singbox-launcher/internal/outboundutil"
)

// RulesPipelineResult — собранные секции config.json для route/dns.
type RulesPipelineResult struct {
	// RouteRuleSet — определения rule_set для route.rule_set[].
	RouteRuleSet []map[string]interface{}

	// RouteRules — route.rules[] без hijack-dns leader (он добавляется в эмиттере отдельно).
	RouteRules []map[string]interface{}

	// DNSServers — финальный dns.servers[]: template defaults (с effective_enabled)
	//              + bundled от активных preset'ов + user extra_servers.
	DNSServers []map[string]interface{}

	// DNSRules — финальный dns.rules[]: bundled dns_rules + user extra_rules.
	DNSRules []map[string]interface{}

	// Warnings — non-fatal предупреждения из expand'а / merge'а / dangling refs.
	Warnings []string
}

// BuildRulesAndDNS — pure func. Собирает route+dns секции из state v6 + template presets.
//
// Аргументы:
//   - presets — `template.presets[]` после LoadPresets (только валидные)
//   - templateDNSDefaults — `template.dns_defaults.servers[]` (для effective_enabled resolve + emit)
//   - state — v6 State (rules + dns config)
//   - srsCachedPaths — map[user-rule-id] → path к скачанному .srs (для kind=srs)
//
// Возвращает структурированные секции и список warning'ов.
func BuildRulesAndDNS(
	presets []template.Preset,
	templateDNSDefaults []TemplateDNSServer,
	state *v6.State,
	srsCachedPaths map[string]string,
) RulesPipelineResult {
	result := RulesPipelineResult{}

	presetByID := make(map[string]*template.Preset, len(presets))
	for i := range presets {
		presetByID[presets[i].ID] = &presets[i]
	}

	// emitted tag-sets для merge collision detection
	ruleSetSeen := make(map[string]map[string]interface{})
	dnsServerSeen := make(map[string]map[string]interface{})

	if state == nil {
		// Empty state — только template defaults для DNS.
		result.DNSServers = emitTemplateDNSDefaults(templateDNSDefaults, nil)
		return result
	}

	// Pass 1: expand active preset-refs + emit user inline/srs
	bundledDNSTags := make(map[string]bool) // tag'и bundled DNS-серверов которые в emit

	for _, rule := range state.Rules {
		if !rule.Enabled {
			continue
		}

		switch rule.Kind {
		case v6.RuleKindPreset:
			p, ok := presetByID[rule.Ref]
			if !ok {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("rule kind=preset ref %q not found in template (broken preset, skipped)", rule.Ref))
				continue
			}
			body, err := rule.DecodeBody()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("decode preset body for %q: %v", rule.Ref, err))
				continue
			}
			pb := body.(*v6.PresetBody)

			frags, warns, ok := ExpandPreset(p, pb.Vars)
			for _, w := range warns {
				result.Warnings = append(result.Warnings, w.String())
			}
			if !ok {
				continue
			}
			mergeRuleSets(&result, ruleSetSeen, frags.RuleSets)
			if frags.RoutingRule != nil {
				result.RouteRules = append(result.RouteRules, frags.RoutingRule)
			}
			if frags.DNSRule != nil {
				result.DNSRules = append(result.DNSRules, frags.DNSRule)
			}
			for _, ds := range frags.DNSServers {
				if tag, _ := ds["tag"].(string); tag != "" {
					if existing, dup := dnsServerSeen[tag]; dup {
						if !mapsEqual(existing, ds) {
							result.Warnings = append(result.Warnings,
								fmt.Sprintf("dns_server tag %q conflict (first-wins)", tag))
						}
						continue
					}
					dnsServerSeen[tag] = ds
					result.DNSServers = append(result.DNSServers, ds)
					bundledDNSTags[tag] = true
				}
			}

		case v6.RuleKindInline:
			body, err := rule.DecodeBody()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("decode inline body: %v", err))
				continue
			}
			ib := body.(*v6.InlineBody)
			// SPEC 056 follow-up: emit user inline match напрямую в route.rules[],
			// без rule_set обёртки — sing-box headless rule_set отвергает
			// connection-level match-поля (protocol/inbound/...). См.
			// preset_merge.go::MergePresetsIntoRoute для полного объяснения.
			match := normalizeMatch(ib.Match)
			routeRule := make(map[string]interface{}, len(match)+1)
			for k, v := range match {
				routeRule[k] = v
			}
			routeRule = outboundutil.ApplyOutboundToRule(routeRule, ib.Outbound)
			result.RouteRules = append(result.RouteRules, routeRule)

		case v6.RuleKindSrs:
			body, err := rule.DecodeBody()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("decode srs body: %v", err))
				continue
			}
			sb := body.(*v6.SrsBody)
			path, hasCache := srsCachedPaths[rule.ID]
			if !hasCache {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("srs rule %q skipped: no cached file at <execDir>/bin/rule-sets/", sb.Name))
				continue
			}
			tag := "user:" + rule.ID
			rs := map[string]interface{}{
				"tag":    tag,
				"type":   "local",
				"format": "binary",
				"path":   path,
			}
			mergeRuleSets(&result, ruleSetSeen, []map[string]interface{}{rs})
			routeRule := map[string]interface{}{"rule_set": tag}
			routeRule = outboundutil.ApplyOutboundToRule(routeRule, sb.Outbound)
			result.RouteRules = append(result.RouteRules, routeRule)

		default:
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("unknown rule kind %q skipped", rule.Kind))
		}
	}

	// Pass 2: template DNS defaults (с effective_enabled override из state).
	// SPEC 057: extras удалены — это последний source for DNSServers.
	// DNSRules заполняется только preset.dns_rule из Pass 1.
	tplDNS := emitTemplateDNSDefaults(templateDNSDefaults, state.DNS.TemplateServers)
	result.DNSServers = append(tplDNS, result.DNSServers...)

	return result
}

// TemplateDNSServer — template.dns_defaults.servers[] элемент.
// Аналог json struct'ы — выделен в типизированный для удобства API.
type TemplateDNSServer struct {
	Tag            string                 `json:"tag"`
	DefaultEnabled bool                   `json:"default_enabled"`
	Raw            map[string]interface{} `json:"-"` // полный raw для emit
}

// emitTemplateDNSDefaults — фильтрует template-defined серверы по effective_enabled.
// Cleanup-fields идёт через единую stripDNSWizardOnlyFields (single source of truth):
// description/enabled/title/if/if_or/default_enabled/_*.
func emitTemplateDNSDefaults(
	defaults []TemplateDNSServer,
	overrides map[string]v6.TemplateServerOvr,
) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(defaults))
	for _, d := range defaults {
		effective := d.DefaultEnabled
		if ovr, has := overrides[d.Tag]; has {
			effective = ovr.Enabled
		}
		if !effective {
			continue
		}
		cleaned := stripDNSWizardOnlyFields(d.Raw)
		// Ensure tag присутствует (stripDNSWizardOnlyFields tag не трогает).
		if _, has := cleaned["tag"]; !has && d.Tag != "" {
			cleaned["tag"] = d.Tag
		}
		out = append(out, cleaned)
	}
	return out
}

// mergeRuleSets — append с identical-skip / first-wins по tag'у.
func mergeRuleSets(
	result *RulesPipelineResult,
	seen map[string]map[string]interface{},
	add []map[string]interface{},
) {
	for _, rs := range add {
		tag, _ := rs["tag"].(string)
		if tag == "" {
			continue
		}
		if existing, dup := seen[tag]; dup {
			if !mapsEqual(existing, rs) {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("rule_set tag %q conflict (first-wins)", tag))
			}
			continue
		}
		seen[tag] = rs
		result.RouteRuleSet = append(result.RouteRuleSet, rs)
	}
}

// mapsEqual — deep JSON equality.
func mapsEqual(a, b map[string]interface{}) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// normalizeMatch — гарантирует что match map есть как минимум {} (не nil).
func normalizeMatch(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

// ParseTemplateDNSDefaults — парсит template.dns_defaults.servers[] для emit.
// Каждый элемент превращает в TemplateDNSServer с raw-snapshot'ом.
//
// servers — это json.RawMessage от template loader'а ([]json.RawMessage).
func ParseTemplateDNSDefaults(servers []json.RawMessage) []TemplateDNSServer {
	out := make([]TemplateDNSServer, 0, len(servers))
	for _, raw := range servers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		tag, _ := m["tag"].(string)
		defaultEnabled := true // по умолчанию включено если поле отсутствует
		if v, has := m["default_enabled"]; has {
			if b, ok := v.(bool); ok {
				defaultEnabled = b
			}
		} else if v, has := m["enabled"]; has {
			// Backward-compat: v5 поле "enabled" читается как default_enabled.
			if b, ok := v.(bool); ok {
				defaultEnabled = b
			}
		}
		out = append(out, TemplateDNSServer{
			Tag:            tag,
			DefaultEnabled: defaultEnabled,
			Raw:            m,
		})
	}
	return out
}

// SanitizeServerForEmit — strip launcher-only keys из server map перед emit.
// (Для future расширений — сейчас все strip'ы делаются в emitTemplateDNSDefaults.)
func SanitizeServerForEmit(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == "default_enabled" || k == "if" || k == "if_or" || strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	return out
}
