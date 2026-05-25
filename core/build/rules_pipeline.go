package build

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/template"
	corestate "singbox-launcher/core/state"
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
	state *corestate.State,
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
		// Empty state — template defaults (required всегда эмитятся; иначе по Enabled).
		for _, d := range templateDNSDefaults {
			if !d.Enabled && !d.Required {
				continue
			}
			cleaned := stripDNSWizardOnlyFields(d.Raw)
			if _, has := cleaned["tag"]; !has && d.Tag != "" {
				cleaned["tag"] = d.Tag
			}
			result.DNSServers = append(result.DNSServers, cleaned)
		}
		return result
	}

	// Pass 1: expand active preset-refs + emit user inline/srs.
	// Bundled DNS-серверы от preset'ов добавляются здесь же — это эквивалент
	// "kind=preset" entries в state.DNS.Servers[]. Финальный walk DNS
	// идёт в Pass 2 ниже (template + user).
	for _, rule := range state.Rules {
		if !rule.Enabled {
			continue
		}

		switch rule.Kind {
		case corestate.RuleKindPreset:
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
			pb := body.(*corestate.PresetBody)

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
				}
			}

		case corestate.RuleKindInline:
			body, err := rule.DecodeBody()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("decode inline body: %v", err))
				continue
			}
			ib := body.(*corestate.InlineBody)
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

		case corestate.RuleKindSrs:
			body, err := rule.DecodeBody()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("decode srs body: %v", err))
				continue
			}
			sb := body.(*corestate.SrsBody)
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

	// Pass 2: walk state.DNS.Servers[] — kind switch (SPEC 056-R-N).
	//
	// Template-defined серверы материализуются здесь (template entries в state
	// хранят только {tag, enabled}; тело берётся из templateDNSDefaults).
	// User-defined серверы эмитятся через stripDNSWizardOnlyFields.
	// Preset entries резолвятся через ExpandPreset (тело + Vars).
	templateDNSByTag := make(map[string]TemplateDNSServer, len(templateDNSDefaults))
	for _, d := range templateDNSDefaults {
		templateDNSByTag[d.Tag] = d
	}
	presetVarsByID := make(map[string]map[string]string, len(state.Rules))
	for _, r := range state.Rules {
		if r.Kind != corestate.RuleKindPreset || !r.Enabled {
			continue
		}
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		pb := body.(*corestate.PresetBody)
		presetVarsByID[r.Ref] = pb.Vars
	}

	dnsServerSeenTags := make(map[string]bool)
	// Prepend bundled DNS (from active preset expand pass 1) into seen-set.
	// They came first because Pass 1 already appended them to result.DNSServers.
	for _, m := range result.DNSServers {
		if t, _ := m["tag"].(string); t != "" {
			dnsServerSeenTags[t] = true
		}
	}

	var templateDNSEmit []map[string]interface{}
	for _, srv := range state.DNS.Servers {
		if !srv.Enabled {
			continue
		}
		switch srv.Kind {
		case corestate.DNSServerKindTemplate:
			d, ok := templateDNSByTag[srv.Tag]
			if !ok || dnsServerSeenTags[srv.Tag] {
				continue
			}
			cleaned := stripDNSWizardOnlyFields(d.Raw)
			if _, has := cleaned["tag"]; !has && d.Tag != "" {
				cleaned["tag"] = d.Tag
			}
			templateDNSEmit = append(templateDNSEmit, cleaned)
			dnsServerSeenTags[d.Tag] = true

		case corestate.DNSServerKindPreset:
			// Preset DNS-серверы уже эмитнуты в Pass 1 через ExpandPreset.
			// Если по какой-то причине не эмитнули (Pass 1 skipped) — здесь
			// тоже пропускаем (preset уже не активен, либо tag не consumed).
			continue

		case corestate.DNSServerKindUser:
			body := make(map[string]interface{}, len(srv.Body)+1)
			for k, v := range srv.Body {
				body[k] = v
			}
			if _, has := body["tag"]; !has && srv.Tag != "" {
				body["tag"] = srv.Tag
			}
			tag, _ := body["tag"].(string)
			if tag != "" && dnsServerSeenTags[tag] {
				continue
			}
			cleaned := stripDNSWizardOnlyFields(body)
			result.DNSServers = append(result.DNSServers, cleaned)
			if tag != "" {
				dnsServerSeenTags[tag] = true
			}
		}
	}
	// Prepend template DNS emit: они должны идти первыми (стабильный порядок
	// для golden tests / диагностики).
	result.DNSServers = append(templateDNSEmit, result.DNSServers...)

	// Pass 3: walk state.DNS.Rules[] (SPEC 056-R-N).
	for _, dr := range state.DNS.Rules {
		if !dr.Enabled {
			continue
		}
		switch dr.Kind {
		case corestate.DNSRuleKindPreset:
			// Preset DNS rules уже эмитнуты в Pass 1 через ExpandPreset.
			continue
		case corestate.DNSRuleKindUser:
			copy := make(map[string]interface{}, len(dr.Body))
			for k, v := range dr.Body {
				copy[k] = v
			}
			result.DNSRules = append(result.DNSRules, copy)
		}
	}

	return result
}

// TemplateDNSServer — template.dns_options.servers[] элемент.
//
// `Required: true` маркирует mandatory entries (`local_dns_resolver` /
// `direct_dns_resolver`) — locked в UI, всегда эмитятся в config.json
// независимо от state. Для не-required: `Enabled` — это default state когда
// в state.DNS.Servers нет override.
type TemplateDNSServer struct {
	Tag      string                 `json:"tag"`
	Enabled  bool                   `json:"enabled"`
	Required bool                   `json:"required,omitempty"`
	Raw      map[string]interface{} `json:"-"` // полный raw для emit
}

// emitTemplateDNSDefaults — УДАЛЕНА в SPEC 056-R-N.
//
// Старая функция фильтровала template-defined серверы по effective_enabled
// через `map[tag]TemplateServerOvr`. После рефактора template-эмит идёт через
// flat walk по state.DNS.Servers[kind=template] в Pass 2 BuildRulesAndDNS
// и в MergePresetsIntoDNS — каждый template entry знает свой Enabled flag
// напрямую (no override map).

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

// ParseTemplateDNSDefaults — парсит template.dns_options.servers[] для emit.
//
// SPEC unify: `required: true` маркирует mandatory entry (всегда эмитится,
// locked в UI). `enabled: true|false` — default state; для required форсится
// в true (loader warning, если в template false; см. ValidateTemplateDNSServers).
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
		enabled := true // default true если поле отсутствует
		if v, has := m["enabled"]; has {
			if b, ok := v.(bool); ok {
				enabled = b
			}
		}
		required := false
		if v, has := m["required"]; has {
			if b, ok := v.(bool); ok {
				required = b
			}
		}
		// Required всегда enabled — loader-уровень coherence force.
		// (Warning эмитит ValidateTemplateDNSServers; здесь silent fix.)
		if required && !enabled {
			enabled = true
		}
		out = append(out, TemplateDNSServer{
			Tag:      tag,
			Enabled:  enabled,
			Required: required,
			Raw:      m,
		})
	}
	return out
}

// ValidateTemplateDNSServers — проверяет invariants на template.dns_options.servers[]
// при load:
//   - tag-uniqueness: duplicate tags → warning (loader skip'ает duplicates).
//   - required + enabled coherence: `required: true && enabled: false` → warning,
//     value force'ится в `enabled: true` (см. ParseTemplateDNSDefaults).
//
// Возвращает список warning'ов (non-fatal; template грузится дальше).
func ValidateTemplateDNSServers(servers []TemplateDNSServer) []string {
	var warns []string
	seen := make(map[string]bool, len(servers))
	for _, s := range servers {
		if s.Tag == "" {
			continue
		}
		if seen[s.Tag] {
			warns = append(warns, fmt.Sprintf("template dns_options.servers: duplicate tag %q (later entries ignored)", s.Tag))
			continue
		}
		seen[s.Tag] = true
		// Required + enabled=false coherence (raw check от json — ParseTemplate уже
		// сфорсил Enabled=true, но юзер должен знать что в template ошибка).
		if s.Required {
			if rawEnabled, has := s.Raw["enabled"]; has {
				if b, ok := rawEnabled.(bool); ok && !b {
					warns = append(warns, fmt.Sprintf("template dns_options.servers[%q]: required=true conflicts with enabled=false; forcing enabled=true", s.Tag))
				}
			}
		}
	}
	return warns
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
