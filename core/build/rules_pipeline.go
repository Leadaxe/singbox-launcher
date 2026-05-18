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
			tag := "user:" + rule.ID
			rs := map[string]interface{}{
				"tag":   tag,
				"type":  "inline",
				"rules": []interface{}{normalizeMatch(ib.Match)},
			}
			mergeRuleSets(&result, ruleSetSeen, []map[string]interface{}{rs})
			routeRule := map[string]interface{}{"rule_set": tag}
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

	// Pass 2: template DNS defaults (с effective_enabled override из state)
	tplDNS := emitTemplateDNSDefaults(templateDNSDefaults, state.DNS.TemplateServers)
	// Prepend template defaults: они должны идти первыми, bundled и extras следом.
	result.DNSServers = append(tplDNS, result.DNSServers...)

	// Pass 3: extra DNS servers (user-defined через DNS tab UI)
	for _, extra := range state.DNS.ExtraServers {
		copy := make(map[string]interface{}, len(extra))
		for k, v := range extra {
			copy[k] = v
		}
		result.DNSServers = append(result.DNSServers, copy)
	}

	// Pass 4: extra DNS rules
	for _, extra := range state.DNS.ExtraRules {
		copy := make(map[string]interface{}, len(extra))
		for k, v := range extra {
			copy[k] = v
		}
		result.DNSRules = append(result.DNSRules, copy)
	}

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
// `default_enabled` и поле `if`/`if_or` strip'аются при emit.
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
		copy := make(map[string]interface{}, len(d.Raw))
		for k, v := range d.Raw {
			if k == "default_enabled" || k == "if" || k == "if_or" {
				continue
			}
			copy[k] = v
		}
		// Ensure tag присутствует
		if _, has := copy["tag"]; !has && d.Tag != "" {
			copy["tag"] = d.Tag
		}
		out = append(out, copy)
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

// launcherOnlyEmitKeys — набор top-level ключей которые НЕ должны попадать
// в финальный sing-box config. Точка истины для всех emit-путей (DNS server,
// preset rule_set / rule / dns_rule / dns_server / outbound, template
// dns_defaults). sing-box 1.12+ строгий decoder отвергает unknown fields,
// поэтому даже невинная документация в `comment`/`description` валит запуск.
//
// Внутренние convention'ы:
//   - Любой ключ с префиксом "_" считается launcher-private (см. SanitizeMap).
//   - Конкретные ключи перечислены явно — некоторые также появляются в
//     template-документации без подчёркивания.
//
// `filters` и `addOutbounds` НЕ в этом списке — они **inputs** для merge
// функций (applyOutboundUpdate, resolveAddFiltersIntoOutbounds) которые
// резолвят их в `outbounds` list и удаляют после consume. На final pass
// merge'а есть отдельный safety-strip через SanitizeMapFinal.
var launcherOnlyEmitKeys = map[string]struct{}{
	"if":              {},
	"if_or":           {},
	"title":           {},
	"description":     {}, // sing-box 1.12+ rejects
	"comment":         {}, // sing-box 1.12+ rejects
	"enabled":         {}, // top-level UI checkbox; tls.enabled etc во вложенных не затрагиваются
	"wizard":          {}, // launcher metadata block (wizard.required etc)
	"default_enabled": {}, // template default flag for UI
}

// SanitizeMap — единая точка очистки top-level map перед emit в sing-box
// config. Удаляет известные launcher-only поля + любые `_*` ключи (внутренний
// convention). In-place mutation.
//
// Используется во ВСЕХ emit путях: preset.outbounds/dns_servers/rule_set/rule/dns_rule,
// template DNS defaults, MergeDNSSection, MergePresetsIntoOutbounds final pass.
func SanitizeMap(m map[string]interface{}) {
	if m == nil {
		return
	}
	for k := range m {
		if _, isLauncherOnly := launcherOnlyEmitKeys[k]; isLauncherOnly {
			delete(m, k)
			continue
		}
		if strings.HasPrefix(k, "_") {
			delete(m, k)
		}
	}
}

// SanitizeMapFinal — расширенный strip для post-merge pass. Дополнительно
// удаляет `filters` и `addOutbounds` (на случай если merge не сконсумировал
// их в outbounds list, например preview без cache).
func SanitizeMapFinal(m map[string]interface{}) {
	SanitizeMap(m)
	delete(m, "filters")
	delete(m, "addOutbounds")
}

// SanitizeServerForEmit — legacy wrapper. Возвращает копию с очищенными
// ключами. Оставлен для совместимости с rules_pipeline emit; новые callsite'ы
// используют SanitizeMap напрямую (in-place).
func SanitizeServerForEmit(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	SanitizeMap(out)
	return out
}
