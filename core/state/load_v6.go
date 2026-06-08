package state

import (
	"encoding/json"
	"fmt"
	"time"
)

// parseCurrent — прямой read canonical (v6) формата (SPEC 053 + SPEC 056-R-N).
//
// v6.State содержит:
//   - meta {version: 6, schema: "presets_v1", ...}
//   - connections (без изменений vs v5)
//   - rules[] с kind discriminator (preset/inline/srs)
//   - vars[] (глобальные template vars, включая dns_* scalars)
//   - dns_options { servers[{kind, ref|tag, enabled, ...body}], rules[...] }
//
// **Backward compat для старого дев-shape (SPEC 053):** если в JSON встречаем
// старую секцию `dns` (с template_servers/extra_servers/extra_rules), конвертим
// её через `legacyDevDNSToOptions` в новый flat shape in-memory. На ближайшем
// Save файл перезаписывается в новом layout'е. v6 не релизился, конверсия
// lossless — backup не нужен.
//
// **TODO (SPEC 056-R-N): удалить `legacyDevDNSToOptions` после release-cycle**
// когда все dev-state'ы перешли на новый shape.
//
// SPEC 070 ADR-070-2: canonical Rules/DNS — единственная stored truth. Legacy
// views (CustomRules / DNSOptions) больше НЕ backfill'ятся в поля State на load;
// UI/business слой берёт их on-demand через LegacyCustomRulesView /
// LegacyDNSOptionsView.
func parseCurrent(data []byte) (*State, error) {
	var raw struct {
		Meta        MetaSection        `json:"meta"`
		Connections ConnectionsSection `json:"connections"`
		Rules       []Rule             `json:"rules"`
		Vars        []SettingVar       `json:"vars"`
		DNSOptions  DNSOptions         `json:"dns_options"`
		// Legacy dev-shape (SPEC 053). Читаем для одноразовой in-place миграции.
		LegacyDNS json.RawMessage `json:"dns"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v6 json: %w", err)
	}

	dnsOpts := raw.DNSOptions
	if dnsOpts.IsEmpty() && len(raw.LegacyDNS) > 0 {
		// Старый dev-shape → конвертим в новый flat layout.
		dnsOpts = legacyDevDNSToOptions(raw.LegacyDNS)
	}

	s := &State{
		Version:            raw.Meta.Version,
		Comment:            raw.Meta.Comment,
		Connections:        raw.Connections,
		Vars:               raw.Vars,
		Rules:              raw.Rules,
		DNS:                dnsOpts,
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// legacyDevDNSToOptions — конверсия старого дев-shape `dns` (SPEC 053:
// template_servers / extra_servers / extra_rules) в новый flat `dns_options`
// (SPEC 056-R-N: servers[]/rules[] через kind discriminator).
//
// Используется только при чтении файлов которые шиппились в HEAD-of-develop
// между SPEC 053 и SPEC 056 — не для шиппнутых юзеров (v6 не релизился).
//
// Преобразование:
//   - dns.template_servers map → servers[] kind=template (tag, enabled)
//   - dns.extra_servers array  → servers[] kind=user (tag, enabled, ...body)
//   - dns.extra_rules array    → rules[] kind=user (enabled=true, ...body)
//   - kind=preset entries создаются позже через SyncDNSOptionsWithActivePresets
//     (см. caller в Load).
func legacyDevDNSToOptions(legacy json.RawMessage) DNSOptions {
	var raw struct {
		Strategy string `json:"strategy"`
		Final    string `json:"final"`
		// SPEC: independent_cache в JSON всё ещё парсим (legacy state read),
		// но в v6 DNSOptions не переносим — sing-box 1.14 deprecation.
		IndependentCache      bool   `json:"independent_cache"`
		DefaultDomainResolver string `json:"default_domain_resolver"`
		TemplateServers       map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"template_servers"`
		ExtraServers []map[string]interface{} `json:"extra_servers"`
		ExtraRules   []map[string]interface{} `json:"extra_rules"`
	}
	if err := json.Unmarshal(legacy, &raw); err != nil {
		return DNSOptions{}
	}
	_ = raw.IndependentCache // intentionally dropped on migration
	out := DNSOptions{
		Strategy:              raw.Strategy,
		Final:                 raw.Final,
		DefaultDomainResolver: raw.DefaultDomainResolver,
	}
	for tag, ovr := range raw.TemplateServers {
		out.Servers = append(out.Servers, DNSServer{
			Kind:    DNSServerKindTemplate,
			Tag:     tag,
			Enabled: ovr.Enabled,
		})
	}
	for _, body := range raw.ExtraServers {
		tag, _ := body["tag"].(string)
		enabled := true
		if v, ok := body["enabled"].(bool); ok {
			enabled = v
		}
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			if k == "enabled" {
				continue
			}
			clean[k] = v
		}
		out.Servers = append(out.Servers, DNSServer{
			Kind:    DNSServerKindUser,
			Tag:     tag,
			Enabled: enabled,
			Body:    clean,
		})
	}
	for _, body := range raw.ExtraRules {
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			if k == "enabled" {
				continue
			}
			clean[k] = v
		}
		out.Rules = append(out.Rules, DNSRule{
			Kind:    DNSRuleKindUser,
			Enabled: true,
			Body:    clean,
		})
	}
	return out
}

// LegacyCustomRulesView — on-demand projection: конвертирует s.Rules[] в legacy
// CustomRule view для UI/business callsite'ов которые ещё работают на v5-модели.
//
// SPEC 070 ADR-070-2: это ПРОЕКЦИЯ, а не stored field. Раньше результат
// записывался в State.CustomRules на load (legacyCustomRulesFromV6 +
// deriveV6FromLegacy backfill); теперь caller вызывает helper когда нужно.
//
// Только kind=inline/srs конвертируются. kind=preset пропускается (не имеет
// сериализованных match-полей — они в template; preset-ref'ы UI показывает
// через PresetRefs напрямую).
func LegacyCustomRulesView(s *State) []CustomRule {
	if s == nil {
		return nil
	}
	rules := s.Rules
	out := make([]CustomRule, 0, len(rules))
	for _, r := range rules {
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		switch r.Kind {
		case RuleKindInline:
			ib := body.(*InlineBody)
			cr := CustomRule{
				Label:            ib.Name,
				Enabled:          r.Enabled,
				SelectedOutbound: ib.Outbound,
				HasOutbound:      true,
				Rule:             cloneMap(ib.Match),
			}
			// Restore outbound в Rule для legacy build-pipeline (он ожидает rule.outbound).
			if cr.Rule == nil {
				cr.Rule = map[string]interface{}{}
			}
			cr.Rule["outbound"] = ib.Outbound
			out = append(out, cr)
		case RuleKindSrs:
			sb := body.(*SrsBody)
			rsRaw, _ := json.Marshal(map[string]interface{}{
				"type":   "remote",
				"format": "binary",
				"url":    sb.SrsURL,
			})
			// SPEC 063 follow-up: edit dialog re-derives rule type через
			// DetermineRuleType(cr.Rule) и игнорирует stored cr.Type. Чтобы
			// SRS rule после re-open не превратилось в "Custom JSON",
			// в cr.Rule кладём rule_set placeholder с identity-based тегом
			// (тот же, что эмитит build/rules_pipeline.go: "user:" + StableRuleID).
			// Это чисто UI-hint — build path не использует cr.Rule для SRS,
			// он берёт state.Rules напрямую.
			tag := "user:" + StableRuleID(r)
			cr := CustomRule{
				Label:            sb.Name,
				Type:             RuleTypeSRS,
				Enabled:          r.Enabled,
				SelectedOutbound: sb.Outbound,
				HasOutbound:      true,
				RuleSet:          []json.RawMessage{rsRaw},
				Rule: map[string]interface{}{
					"rule_set": tag,
					"outbound": sb.Outbound,
				},
			}
			out = append(out, cr)
		case RuleKindPreset:
			// preset-ref пропускается в legacy view (UI Phase 6 покажет через новый dialog).
			// При сохранении preset-ref'ы сохраняются обратно в RulesV6 без потери.
		}
	}
	return out
}

// LegacyDNSOptionsView — on-demand projection: реконструирует v5
// *LegacyDNSOptionsV5 из canonical s.DNS для UI/business/test callsite'ов
// которые ещё работают на v5 DNS-модели.
//
// SPEC 070 ADR-070-2: проекция, не stored field. Раньше State.DNSOptions
// заполнялся raw'ом на load v5 (parseV5Legacy) либо оставался nil на v6;
// теперь поле удалено, а данные живут в canonical s.DNS (после migrateDNS на
// read-time migration shim).
//
// Реконструкция:
//   - scalars (Strategy/Final/DefaultDomainResolver) копируются напрямую;
//   - Servers/Rules восстанавливаются в raw v5 shape (плоский JSON объект
//     {tag, enabled, ...body} без поля kind) — это инвертирует migrateDNS
//     для callsite'ов, ожидающих старый формат (build golden harness).
//
// Возвращает nil если s nil или s.DNS пуст — caller'ы nil-tolerant.
func LegacyDNSOptionsView(s *State) *LegacyDNSOptionsV5 {
	if s == nil {
		return nil
	}
	d := s.DNS
	if d.IsEmpty() {
		return nil
	}
	out := &LegacyDNSOptionsV5{
		Strategy:              d.Strategy,
		Final:                 d.Final,
		DefaultDomainResolver: d.DefaultDomainResolver,
	}
	for i := range d.Servers {
		if raw := legacyRawDNSServer(d.Servers[i]); raw != nil {
			out.Servers = append(out.Servers, raw)
		}
	}
	for i := range d.Rules {
		if raw := legacyRawDNSRule(d.Rules[i]); raw != nil {
			out.Rules = append(out.Rules, raw)
		}
	}
	return out
}

// legacyRawDNSServer — реконструирует raw v5 server JSON {tag, enabled, ...body}
// из canonical DNSServer (инверсия migrateDNS kind=user branch). template/preset
// kinds пропускаются (у них нет body — они материализуются из шаблона).
func legacyRawDNSServer(srv DNSServer) json.RawMessage {
	if srv.Kind != DNSServerKindUser {
		return nil
	}
	entry := make(map[string]interface{}, 2+len(srv.Body))
	for k, v := range srv.Body {
		switch k {
		case "kind", "ref", "enabled":
			continue
		}
		entry[k] = v
	}
	if srv.Tag != "" {
		entry["tag"] = srv.Tag
	}
	entry["enabled"] = srv.Enabled
	b, err := json.Marshal(entry)
	if err != nil {
		return nil
	}
	return b
}

// legacyRawDNSRule — реконструирует raw v5 rule JSON из canonical DNSRule
// (инверсия migrateDNS rules branch). preset kind пропускается.
func legacyRawDNSRule(rl DNSRule) json.RawMessage {
	if rl.Kind != DNSRuleKindUser {
		return nil
	}
	entry := make(map[string]interface{}, len(rl.Body))
	for k, v := range rl.Body {
		switch k {
		case "kind", "ref", "enabled":
			continue
		}
		entry[k] = v
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return nil
	}
	return b
}

// cloneMap — shallow copy of map[string]interface{} for safe legacy view generation.
func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
