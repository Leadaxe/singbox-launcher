// File migration_v5_to_v6.go — конверсия legacy v5 in-memory shape → canonical (v6).
//
// Pure helper, не экспортируется. Используется только Parse в случае
// legacy v5 файла (meta.version == 5).
package state

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// migrateWarning — non-fatal warning при миграции v5 → v6.
type migrateWarning struct {
	RuleLabel string
	Message   string
}

func (w migrateWarning) String() string {
	prefix := ""
	if w.RuleLabel != "" {
		prefix = fmt.Sprintf("rule %q: ", w.RuleLabel)
	}
	return prefix + w.Message
}

// migrateV5ToV6 — pure func. Конвертит diskStateV5 в canonical (v6) форму
// через приватный diskStateV6 — но возвращает разложенные поля удобные для
// сборки State.
//
// templateDNSDefaults — карта template.dns_defaults.servers[tag] → default_enabled.
// templatePresetIDsByLabel — карта template.presets[].label → preset.id.
func migrateV5ToV6(
	old diskStateV5,
	templateDNSDefaults map[string]bool,
	templatePresetIDsByLabel map[string]string,
) (diskStateV6, []migrateWarning) {
	var warnings []migrateWarning

	newState := diskStateV6{
		Meta: MetaSection{
			Version:   SchemaVersionV6,
			Schema:    SchemaName,
			Comment:   old.Meta.Comment,
			CreatedAt: old.Meta.CreatedAt,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		},
		Connections: old.Connections,
		Vars:        old.Vars,
	}

	// Rules migration
	for _, cr := range old.CustomRules {
		r, w := migrateCustomRule(cr, templatePresetIDsByLabel)
		if r != nil {
			newState.Rules = append(newState.Rules, *r)
		}
		warnings = append(warnings, w...)
	}

	// DNS migration
	newState.DNSOptions, _ = migrateDNS(old.DNSOptions, templateDNSDefaults)

	return newState, warnings
}

// migrateCustomRule — конвертит один CustomRule в Rule.
//
// Эвристика kind:
//  1. label совпадает с template-preset → kind=preset с пустым varsValues
//  2. rule_set[0].type == "remote" → kind=srs (URL берётся из rule_set[0].url)
//  3. иначе → kind=inline (rule как snapshot, без outbound поля)
func migrateCustomRule(
	cr CustomRule,
	presetIDsByLabel map[string]string,
) (*Rule, []migrateWarning) {
	var warns []migrateWarning

	// 1. preset-ref candidate
	if presetID, ok := presetIDsByLabel[cr.Label]; ok {
		body, _ := json.Marshal(PresetBody{Vars: map[string]string{}})
		return &Rule{
			Kind:    RuleKindPreset,
			Ref:     presetID,
			Enabled: cr.Enabled,
			Body:    body,
		}, nil
	}

	// 2. srs candidate
	if len(cr.RuleSet) > 0 {
		var rs struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		}
		if err := json.Unmarshal(cr.RuleSet[0], &rs); err == nil && rs.Type == "remote" && rs.URL != "" {
			body, _ := json.Marshal(SrsBody{
				Name:     cr.Label,
				SrsURL:   rs.URL,
				Outbound: cr.SelectedOutbound,
			})
			return &Rule{
				Kind:    RuleKindSrs,
				ID:      generateMigrationULID(),
				Enabled: cr.Enabled,
				Body:    body,
			}, nil
		}
	}

	// 3. inline
	match := stripOutboundFromRule(cr.Rule)
	if len(match) == 0 {
		warns = append(warns, migrateWarning{
			RuleLabel: cr.Label,
			Message:   "no match fields after stripping outbound — skipped",
		})
		return nil, warns
	}
	body, _ := json.Marshal(InlineBody{
		Name:     cr.Label,
		Match:    match,
		Outbound: cr.SelectedOutbound,
	})
	return &Rule{
		Kind:    RuleKindInline,
		ID:      generateMigrationULID(),
		Enabled: cr.Enabled,
		Body:    body,
	}, nil
}

// stripOutboundFromRule — удаляет outbound/action/method из rule, оставляет только match-поля.
func stripOutboundFromRule(rule map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(rule))
	for k, v := range rule {
		switch k {
		case "outbound", "action", "method":
			// strip
		default:
			out[k] = v
		}
	}
	return out
}

// migrateDNS — конвертит legacy v5 DNSOptions в canonical DNSOptions (SPEC 056-R-N).
//
// servers[] split:
//   - tag ∈ templateDefaults → DNSServer{Kind:template, Tag, Enabled}
//   - tag ∉ templateDefaults → DNSServer{Kind:user, Tag, Enabled, Body:full body}
//
// rules[] → DNSRule{Kind:user, Body} каждое.
//
// kind=preset entries в этой функции **не создаются** — они материализуются
// после миграции через SyncDNSOptionsWithActivePresets (вызывается caller'ом
// после того как заполнятся active preset-ref'ы).
//
// Invariant: template tag НИКОГДА не попадает в kind=user — для template-defined
// tag'ов используется kind=template override. Эта функция держит invariant
// через `templateDefaults` check.
func migrateDNS(old *LegacyDNSOptionsV5, templateDefaults map[string]bool) (DNSOptions, []migrateWarning) {
	d := DNSOptions{}
	if old == nil {
		return d, nil
	}

	d.Strategy = old.Strategy
	d.Final = old.Final
	d.DefaultDomainResolver = old.DefaultDomainResolver
	// SPEC: IndependentCache УДАЛЕНО — sing-box 1.14 deprecation.
	// Legacy v5 поле игнорируется на миграции.

	for _, rawServer := range old.Servers {
		var srv map[string]interface{}
		if err := json.Unmarshal(rawServer, &srv); err != nil {
			continue
		}
		tag, _ := srv["tag"].(string)
		enabled := true
		if v, ok := srv["enabled"].(bool); ok {
			enabled = v
		}

		if tag != "" && templateDefaults[tag] {
			d.Servers = append(d.Servers, DNSServer{
				Kind:    DNSServerKindTemplate,
				Tag:     tag,
				Enabled: enabled,
			})
		} else {
			// User-added → kind=user с полным телом (без поля enabled — оно на top-level).
			body := make(map[string]interface{}, len(srv))
			for k, v := range srv {
				if k == "enabled" {
					continue
				}
				body[k] = v
			}
			d.Servers = append(d.Servers, DNSServer{
				Kind:    DNSServerKindUser,
				Tag:     tag,
				Enabled: enabled,
				Body:    body,
			})
		}
	}

	for _, rawRule := range old.Rules {
		var r map[string]interface{}
		if err := json.Unmarshal(rawRule, &r); err != nil {
			continue
		}
		// Strip enabled из body (это launcher-only flag, не sing-box).
		body := make(map[string]interface{}, len(r))
		for k, v := range r {
			if k == "enabled" {
				continue
			}
			body[k] = v
		}
		d.Rules = append(d.Rules, DNSRule{
			Kind:    DNSRuleKindUser,
			Enabled: true,
			Body:    body,
		})
	}

	return d, nil
}

// generateMigrationULID — placeholder для миграции v5→v6. Используется только
// внутри migration helpers; для UI/runtime используется MakeULID (полноценный
// ULID). Простой timestamp + counter — достаточно для миграции (она одноразовая).
func generateMigrationULID() string {
	now := time.Now().UnixNano()
	migrationCounter++
	return fmt.Sprintf("01J%X%X", now, migrationCounter)
}

var migrationCounter uint64

// isV6 — детект schema version по сырому state JSON.
func isV6(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Meta.Version == SchemaVersionV6
}

// isV5 — детект v5 schema.
func isV5(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Meta.Version == legacySchemaVersionV5
}

// isLikelyLegacyLabel — heuristic для определения legacy-label'а.
// (placeholder для будущих расширений)
func isLikelyLegacyLabel(label string) bool {
	return strings.HasPrefix(label, "Russian ") ||
		strings.HasPrefix(label, "Block ") ||
		strings.HasPrefix(label, "Private ")
}
