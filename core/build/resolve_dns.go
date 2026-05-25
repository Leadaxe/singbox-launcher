// Package build — File resolve_dns.go (SPEC 056-R-N follow-up).
//
// Единый resolver DNS section: pure func которая берёт state+template+vars
// и возвращает структурированное представление «что есть в DNS». Используется
// и build pipeline'ом (для эмита config.json::dns), и UI render'ом
// (для отображения DNS tab).
//
// Принципы:
//   - Memory == disk == emit. Что показывается в UI — то и эмитится в config
//     (с учётом Active && Enabled).
//   - Consumption-filter мёртв (см. SPEC 056-R-N follow-up): все bundled
//     DNS-серверы preset'а включены в результат; юзер управляет per-server
//     чекбоксом.
//   - evalIf/substitute живут в одном месте — никакого дублирования в UI vs build.
//
// См. SPECS/056-R-N-DNS_SCHEMA_REDESIGN/RESOLVE_REFACTOR_PLAN.md.
package build

import (
	"encoding/json"
	"sort"
	"strings"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// DNSSource — discriminator происхождения DNS entry.
//
// Каждая ResolvedDNSServer entry знает откуда она пришла — это нужно UI для
// kind-aware рендера (lock для template/preset, edit/del для user) и для
// сохранения/чтения state (DNSV6.Servers[kind=...]).
type DNSSource string

const (
	// DNSSourceTemplate — template.dns_options.servers[] (единый список DNS-серверов).
	// `required: true` маркирует mandatory entries (locked в UI, всегда эмитятся).
	// Остальные — toggleable через state.DNS.Servers[kind=template, tag].Enabled.
	DNSSourceTemplate DNSSource = "template"

	// DNSSourcePreset — template.presets[X].dns_servers[Y] (bundled от preset'а).
	// Юзер toggle'ит enabled per-server; тело берётся из template + applied
	// preset vars.
	DNSSourcePreset DNSSource = "preset"

	// DNSSourceUser — state.DNS.Servers[kind=user] (genuinely user-added).
	// Полное тело в state. Юзер edit/delete.
	DNSSourceUser DNSSource = "user"
)

// ResolvedDNSServer — одна entry финального DNS-server list'а.
//
// Содержит всё что нужно UI render'у и build emit'у: тело sing-box-valid,
// metadata о происхождении, статусы Active/Enabled/Locked.
type ResolvedDNSServer struct {
	// Tag — финальный sing-box tag (с preset prefix для kind=preset).
	// Используется как ключ в config.dns.servers[].tag.
	Tag string

	// LocalTag — tag без preset prefix (для UI display "yandex_doh" вместо
	// "russian:yandex_doh"). Для kind=template/user совпадает с Tag.
	LocalTag string

	// Body — готовое sing-box DNS-server тело: {type, server, server_port,
	// tls, detour, ...}. После substitute @var и strip wizard-only полей
	// (description/enabled/title/if/if_or/...). Готов для прямого эмита.
	Body map[string]interface{}

	// Source — откуда entry пришла (для kind-aware UI и debug).
	Source DNSSource

	// PresetID — id preset'а (только для Source=preset). Пуст иначе.
	PresetID string

	// PresetLabel — UI label preset'а (только для Source=preset).
	PresetLabel string

	// Active — true если entry прошла if/if_or фильтрацию. Если false —
	// в config НЕ эмитится; UI показывает greyed с tooltip InactiveReason.
	Active bool

	// Enabled — юзерский toggle (state.DNS.Servers[i].Enabled).
	// Для Source=core всегда true (нельзя toggle). Для Source=template/preset/user —
	// читается из state с дефолтом true (template имеет default_enabled).
	Enabled bool

	// Locked — true для Source=core (template.config.dns.servers) — нельзя
	// toggle/edit/del. UI рисует greyed checkbox без edit/del кнопок.
	Locked bool

	// InactiveReason — для UI tooltip когда Active=false. Формат:
	// "if=use_dns_override" (значит var use_dns_override=false), либо
	// "if_or=ipv4_enabled,ipv6_enabled" (ни одна из vars не true).
	// Пусто если Active=true.
	InactiveReason string
}

// ResolvedDNSRule — одна entry финального DNS-rule list'а.
//
// Аналог ResolvedDNSServer для dns rules. Только preset/user sources
// (template/core не имеют отдельных rules — те живут в template.config.dns.rules
// и эмитятся as-is).
type ResolvedDNSRule struct {
	// Body — готовое sing-box DNS-rule тело: {server, rule_set, domain_*,
	// ip_cidr, ...}. После substitute @var и rewrite rule_set refs (preset
	// prefix).
	Body map[string]interface{}

	// Source — только preset|user для rules.
	Source DNSSource

	// PresetID/Label — только для Source=preset.
	PresetID    string
	PresetLabel string

	// Active — прошёл if/if_or.
	Active bool

	// Enabled — юзерский toggle.
	Enabled bool

	// InactiveReason — UI tooltip когда !Active.
	InactiveReason string
}

// ResolvedDNS — результат ResolveDNS(): полное представление DNS section
// для UI render'а или build emit'а.
type ResolvedDNS struct {
	// Servers — все DNS-серверы в порядке: core → template → preset → user.
	// UI рендерит все; build эмитит где Active && Enabled.
	Servers []ResolvedDNSServer

	// Rules — все DNS-rules в порядке: preset → user.
	Rules []ResolvedDNSRule

	// Scalars (читаются из state.Vars[dns_*] с fallback на template).
	// Пусто если в state нет override и в template нет default.
	Strategy string
	Final    string
	// SPEC: IndependentCache УДАЛЕНО — deprecated в sing-box 1.14.0.
	DefaultDomainResolver string
}

// ── Implementation ─────────────────────────────────────────────────

// ResolveDNS — единая точка резолва DNS section. Pure func: без I/O,
// без shared state, idempotent.
//
// Аргументы:
//   - state — v6 state (Rules + DNS, обязателен; nil → пустой результат)
//   - td    — TemplateData (presets, dns_options library)
//   - templateVars — глобальные template vars (state.Vars карта) для template-level
//     if/if_or (опционально — сейчас template DNS не использует if/if_or)
//
// Возвращает структурированный ResolvedDNS с meta-данными (Source/Active/
// Enabled/Locked/InactiveReason) для каждой entry.
//
// SPEC unify: больше нет отдельной CORE секции — `local_dns_resolver` /
// `direct_dns_resolver` живут в `template.dns_options.servers[]` с
// `required: true` маркером. Locked entries попадают в результат с
// Source=template + Locked=true; Enabled всегда true.
func ResolveDNS(state *corestate.State, td *template.TemplateData, templateVars map[string]string) ResolvedDNS {
	var out ResolvedDNS
	if td == nil {
		return out
	}

	// TEMPLATE LIBRARY: единый источник — template.dns_options.servers[].
	// Required entries → Locked=true (Enabled forced true ParseTemplateDNSDefaults'ом).
	// Иначе → Enabled из state override (с fallback на template default).
	for _, raw := range templateDNSLibraryFromTemplate(td) {
		tag := tagFromBody(raw)
		if tag == "" {
			continue
		}
		defaultEnabled := bodyEnabled(raw, true)
		required := bodyRequired(raw)
		var enabled bool
		if required {
			enabled = true
		} else {
			enabled = stateTemplateEnabled(state, tag, defaultEnabled)
		}
		out.Servers = append(out.Servers, ResolvedDNSServer{
			Tag:      tag,
			LocalTag: tag,
			Body:     stripDNSWizardOnlyFields(raw),
			Source:   DNSSourceTemplate,
			Active:   true,
			Enabled:  enabled,
			Locked:   required,
		})
	}

	// 3. PRESETS: bundled DNS-серверы + dns_rule от активных preset-ref'ов.
	presetByID := make(map[string]*template.Preset, len(td.Presets))
	for i := range td.Presets {
		presetByID[td.Presets[i].ID] = &td.Presets[i]
	}
	if state != nil {
		for _, rule := range state.Rules {
			if rule.Kind != corestate.RuleKindPreset || !rule.Enabled || rule.Ref == "" {
				continue
			}
			p := presetByID[rule.Ref]
			if p == nil {
				continue
			}
			body, err := rule.DecodeBody()
			if err != nil {
				continue
			}
			pb := body.(*corestate.PresetBody)
			presetVars := buildPresetVarsMap(p, pb.Vars)

			// 3a. DNS servers — все bundled, без consumption-фильтра.
			for i := range p.DNSServers {
				ds := &p.DNSServers[i]
				if ds.Tag == "" {
					continue
				}
				active, reason := evalIfWithReason(ds.If, ds.IfOr, presetVars)
				bodyMap := substitutePresetDNSServer(ds, presetVars)
				ref := p.ID + ":" + ds.Tag
				bodyMap["tag"] = ref
				enabled := statePresetServerEnabled(state, ref, true)
				out.Servers = append(out.Servers, ResolvedDNSServer{
					Tag:            ref,
					LocalTag:       ds.Tag,
					Body:           stripDNSWizardOnlyFields(bodyMap),
					Source:         DNSSourcePreset,
					PresetID:       p.ID,
					PresetLabel:    presetDisplayLabel(p),
					Active:         active,
					Enabled:        enabled,
					InactiveReason: reason,
				})
			}

			// 3b. dns_rule (один на preset, если есть).
			if p.DNSRule != nil {
				active, reason := evalIfFromRuleMap(p.DNSRule, presetVars)
				ruleBody, ok := substitutePresetDNSRule(p, presetVars)
				if !ok {
					continue
				}
				enabled := statePresetRuleEnabled(state, p.ID, true)
				out.Rules = append(out.Rules, ResolvedDNSRule{
					Body:           ruleBody,
					Source:         DNSSourcePreset,
					PresetID:       p.ID,
					PresetLabel:    presetDisplayLabel(p),
					Active:         active,
					Enabled:        enabled,
					InactiveReason: reason,
				})
			}
		}
	}

	// 4. USER: state.DNS.Servers[kind=user] + state.DNS.Rules[kind=user].
	if state != nil {
		for i := range state.DNS.Servers {
			srv := &state.DNS.Servers[i]
			if srv.Kind != corestate.DNSServerKindUser {
				continue
			}
			body := make(map[string]interface{}, len(srv.Body)+1)
			for k, v := range srv.Body {
				body[k] = v
			}
			if _, has := body["tag"]; !has && srv.Tag != "" {
				body["tag"] = srv.Tag
			}
			out.Servers = append(out.Servers, ResolvedDNSServer{
				Tag:      srv.Tag,
				LocalTag: srv.Tag,
				Body:     stripDNSWizardOnlyFields(body),
				Source:   DNSSourceUser,
				Active:   true,
				Enabled:  srv.Enabled,
			})
		}
		for i := range state.DNS.Rules {
			r := &state.DNS.Rules[i]
			if r.Kind != corestate.DNSRuleKindUser {
				continue
			}
			body := make(map[string]interface{}, len(r.Body))
			for k, v := range r.Body {
				body[k] = v
			}
			out.Rules = append(out.Rules, ResolvedDNSRule{
				Body:    body,
				Source:  DNSSourceUser,
				Active:  true,
				Enabled: r.Enabled,
			})
		}
	}

	// 5. Scalars. state.Vars[dns_*] перебивает template default.
	out.Strategy = templateVars["dns_strategy"]
	out.Final = templateVars["dns_final"]
	out.DefaultDomainResolver = templateVars["dns_default_domain_resolver"]
	// SPEC: dns_independent_cache УДАЛЕНО (sing-box 1.14 deprecation).
	// State.DNSOptions scalars (для in-memory работы — обычно дублируют vars).
	if state != nil {
		if out.Strategy == "" {
			out.Strategy = state.DNS.Strategy
		}
		if out.Final == "" {
			out.Final = state.DNS.Final
		}
		if out.DefaultDomainResolver == "" {
			out.DefaultDomainResolver = state.DNS.DefaultDomainResolver
		}
	}

	return out
}

// ── Helpers ────────────────────────────────────────────────────────

// coreDNSServersFromTemplate — УДАЛЕНА в SPEC unify. Все DNS-серверы
// (включая required: local_dns_resolver/direct_dns_resolver) живут в
// template.dns_options.servers[]. Locked entries имеют `required: true`.

// templateDNSLibraryFromTemplate — template.dns_options.servers[] (библиотека).
func templateDNSLibraryFromTemplate(td *template.TemplateData) []map[string]interface{} {
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	return dnsOpt.Servers
}

// tagFromBody — извлекает tag из DNS-server body map.
func tagFromBody(body map[string]interface{}) string {
	t, _ := body["tag"].(string)
	return t
}

// bodyRequired — true если в template body выставлен флаг required.
func bodyRequired(body map[string]interface{}) bool {
	if v, has := body["required"]; has {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// bodyEnabled — читает enabled/default_enabled из template body, fallback default.
func bodyEnabled(body map[string]interface{}, fallback bool) bool {
	if v, has := body["default_enabled"]; has {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	if v, has := body["enabled"]; has {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}

// stateTemplateEnabled — читает enabled override для kind=template entry в state.
// Если state нет или entry нет — возвращает default.
func stateTemplateEnabled(state *corestate.State, tag string, defaultEnabled bool) bool {
	if state == nil {
		return defaultEnabled
	}
	for _, s := range state.DNS.Servers {
		if s.Kind == corestate.DNSServerKindTemplate && s.Tag == tag {
			return s.Enabled
		}
	}
	return defaultEnabled
}

// statePresetServerEnabled — читает enabled для kind=preset entry в state.
// Если state нет или entry нет — возвращает default (true).
func statePresetServerEnabled(state *corestate.State, ref string, defaultEnabled bool) bool {
	if state == nil {
		return defaultEnabled
	}
	for _, s := range state.DNS.Servers {
		if s.Kind == corestate.DNSServerKindPreset && s.Ref == ref {
			return s.Enabled
		}
	}
	return defaultEnabled
}

// statePresetRuleEnabled — readонй для kind=preset rule entry.
func statePresetRuleEnabled(state *corestate.State, ref string, defaultEnabled bool) bool {
	if state == nil {
		return defaultEnabled
	}
	for _, r := range state.DNS.Rules {
		if r.Kind == corestate.DNSRuleKindPreset && r.Ref == ref {
			return r.Enabled
		}
	}
	return defaultEnabled
}

// buildPresetVarsMap — строит vars map для preset из preset.Vars defaults +
// user-overrides (state.Rules[].Body.Vars). Применяет if/if_or каскад (как
// ExpandPreset).
func buildPresetVarsMap(p *template.Preset, userVars map[string]string) map[string]string {
	varsMap := make(map[string]string, len(p.Vars))
	for _, v := range p.Vars {
		if userVal, ok := userVars[v.Name]; ok && userVal != "" {
			varsMap[v.Name] = userVal
		} else {
			varsMap[v.Name] = v.Default
		}
	}
	activeVars := filterActiveVars(p.Vars, varsMap)
	for name := range varsMap {
		if !activeVars[name] {
			delete(varsMap, name)
		}
	}
	return varsMap
}

// substitutePresetDNSServer — конвертит PresetDNSServer struct в map с
// applied substitute. Возвращает чистый sing-box-valid body.
func substitutePresetDNSServer(ds *template.PresetDNSServer, varsMap map[string]string) map[string]interface{} {
	body := map[string]interface{}{
		"tag":  ds.Tag,
		"type": ds.Type,
	}
	if ds.Server != "" {
		body["server"] = ds.Server
	}
	if ds.ServerPort != 0 {
		body["server_port"] = ds.ServerPort
	}
	if ds.Path != "" {
		body["path"] = ds.Path
	}
	if ds.Detour != "" {
		body["detour"] = ds.Detour
	}
	if ds.Description != "" {
		body["description"] = ds.Description
	}
	if ds.TLS != nil {
		body["tls"] = ds.TLS
	}
	// substituteAny inplace — резолвит @var строки в значения.
	substituted, _ := substituteAny(body, varsMap)
	out, _ := substituted.(map[string]interface{})
	if out == nil {
		return body
	}
	// detour=direct-out → strip (sing-box резолвит без forwarding).
	if det, ok := out["detour"].(string); ok && det == "direct-out" {
		delete(out, "detour")
	}
	return out
}

// substitutePresetDNSRule — резолвит preset.DNSRule body через ExpandPreset
// (он умеет rewrite rule_set refs с preset prefix).
func substitutePresetDNSRule(p *template.Preset, varsMap map[string]string) (map[string]interface{}, bool) {
	if p == nil || p.DNSRule == nil {
		return nil, false
	}
	// Reuse ExpandPreset.frags.DNSRule — он делает substitute + rewrite refs.
	// Эту часть пока оставим — она независима от consumption filter.
	frags, _, ok := ExpandPreset(p, varsMap)
	if !ok || frags == nil || frags.DNSRule == nil {
		return nil, false
	}
	out := make(map[string]interface{}, len(frags.DNSRule))
	for k, v := range frags.DNSRule {
		out[k] = v
	}
	return out, true
}

// evalIfWithReason — то же что evalIf, но возвращает причину отказа для UI tooltip.
// Возвращает (true, "") если активна. Иначе (false, "if=foo" / "if_or=a,b").
func evalIfWithReason(ifList, ifOrList []string, varsMap map[string]string) (bool, string) {
	for _, name := range ifList {
		if !strings.EqualFold(varsMap[name], "true") {
			return false, "if=" + name
		}
	}
	if len(ifOrList) > 0 {
		anyTrue := false
		for _, name := range ifOrList {
			if strings.EqualFold(varsMap[name], "true") {
				anyTrue = true
				break
			}
		}
		if !anyTrue {
			sort.Strings(ifOrList) // detered output for test stability
			return false, "if_or=" + strings.Join(ifOrList, ",")
		}
	}
	return true, ""
}

// evalIfFromRuleMap — extract'ит if/if_or из map[string]interface{} (preset.Rule/DNSRule)
// и вызывает evalIfWithReason.
func evalIfFromRuleMap(m map[string]interface{}, varsMap map[string]string) (bool, string) {
	ifList, ifOrList := extractIfFromMap(m)
	return evalIfWithReason(ifList, ifOrList, varsMap)
}

// presetDisplayLabel — fallback на ID если Label пусто.
func presetDisplayLabel(p *template.Preset) string {
	if p.Label != "" {
		return p.Label
	}
	return p.ID
}
