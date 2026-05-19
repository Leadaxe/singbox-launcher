package v6

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	v5 "singbox-launcher/core/state/v5"
)

// MigrateWarning — non-fatal warning при миграции v5 → v6.
type MigrateWarning struct {
	RuleLabel string
	Message   string
}

func (w MigrateWarning) String() string {
	prefix := ""
	if w.RuleLabel != "" {
		prefix = fmt.Sprintf("rule %q: ", w.RuleLabel)
	}
	return prefix + w.Message
}

// MigrateV5ToV6 — pure func. Конвертит v5.State в v6.State.
//
// Преобразования:
//  1. meta.version: 5 → 6, meta.schema = "presets_v1"
//  2. custom_rules[] → rules[]:
//     - Если в template есть preset с подходящим id (по label-match) → kind=preset (TODO: phase 8)
//     - Если rule имеет remote rule_set'ы (URL) → kind=srs
//     - Иначе → kind=inline (snapshot match-полей)
//  3. dns_options → dns:
//     - servers[] split: tag совпадает с template-defined → template_servers override; иначе → extra_servers
//     - rules[] → extra_rules
//
// templateDNSDefaults — карта template.dns_defaults.servers[tag] → default_enabled.
// Используется чтобы решить какие из v5 серверов это template-defined (override) а какие user-added (extra).
// Пустая карта = все серверы считаются user-added (extras). Для production migration'а Phase 8 заполняем
// реальными tag'ами из bundled template.
//
// templatePresetIDsByLabel — карта template.presets[].label → preset.id для миграции legacy
// selectable_rule_states по label (если у юзера v5 правило с label "Block Ads", а в template
// есть preset_id "block-ads" с label "Block Ads" — мигрируем как preset-ref).
// Пустая карта = всё мигрируем как user-defined inline/srs.
func MigrateV5ToV6(
	old v5.State,
	templateDNSDefaults map[string]bool,
	templatePresetIDsByLabel map[string]string,
) (State, []MigrateWarning) {
	var warnings []MigrateWarning

	newState := State{
		Meta: MetaSection{
			Version:   SchemaVersion,
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
	newState.DNS, _ = migrateDNS(old.DNSOptions, templateDNSDefaults)

	return newState, warnings
}

// migrateCustomRule — конвертит один v5.CustomRule в v6.Rule.
//
// Эвристика kind:
//  1. label совпадает с template-preset → kind=preset с пустым varsValues
//  2. rule_set[0].type == "remote" → kind=srs (URL берётся из rule_set[0].url)
//  3. иначе → kind=inline (rule как snapshot, без outbound поля)
//
// Возвращает (rule, warnings). rule может быть nil если миграция невозможна
// (warning будет).
func migrateCustomRule(
	cr v5.CustomRule,
	presetIDsByLabel map[string]string,
) (*Rule, []MigrateWarning) {
	var warns []MigrateWarning

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
				ID:      generateULID(),
				Enabled: cr.Enabled,
				Body:    body,
			}, nil
		}
	}

	// 3. inline
	match := stripOutboundFromRule(cr.Rule)
	if len(match) == 0 {
		warns = append(warns, MigrateWarning{
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
		ID:      generateULID(),
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

// migrateDNS — конвертит v5.DNSOptions в v6.DNSConfig.
//
// SPEC 057: v6 теперь хранит ТОЛЬКО refs/overrides, никаких extras. Серверы:
//   - Если tag ∈ templateDefaults → override записываем в TemplateServers
//     с {Enabled: enabled}.
//   - Если tag НЕ в templateDefaults (user-added через DNS tab в v5) →
//     **DROP с warning'ом**: state не хранит копии тел; чтобы добавить
//     custom DNS-сервер, нужно положить его в template или preset.
//
// rules → **DROP полностью** с warning'ом. v5 user-DNS-rules были inline
// копиями preset bodies (см. SPEC 057 rationale), при upgrade нет рефа
// чтобы их сохранить — лучше потерять одно правило, чем тащить dangling
// в новую схему.
func migrateDNS(old *v5.DNSOptions, templateDefaults map[string]bool) (DNSConfig, []MigrateWarning) {
	d := DNSConfig{}
	if old == nil {
		return d, nil
	}

	d.Strategy = old.Strategy
	d.Final = old.Final
	d.DefaultDomainResolver = old.DefaultDomainResolver
	if old.IndependentCache != nil {
		d.IndependentCache = *old.IndependentCache
	}

	var warns []MigrateWarning

	// servers: template overrides only; user extras → drop + warn
	overrides := make(map[string]TemplateServerOvr)
	for _, rawServer := range old.Servers {
		var srv map[string]interface{}
		if err := json.Unmarshal(rawServer, &srv); err != nil {
			continue
		}
		tag, _ := srv["tag"].(string)
		enabled, _ := srv["enabled"].(bool)

		if tag != "" && templateDefaults[tag] {
			overrides[tag] = TemplateServerOvr{Enabled: enabled}
		} else {
			warns = append(warns, MigrateWarning{
				Message: fmt.Sprintf(
					"v5→v6 migration: dropping user-added DNS server %q "+
						"(SPEC 057: state holds only template refs, not bodies)", tag),
			})
		}
	}
	if len(overrides) > 0 {
		d.TemplateServers = overrides
	}

	// rules: drop all + warn (no way to convert inline body to a ref)
	if len(old.Rules) > 0 {
		warns = append(warns, MigrateWarning{
			Message: fmt.Sprintf(
				"v5→v6 migration: dropping %d DNS rule(s) "+
					"(SPEC 057: user DNS rules now come from preset.dns_rule only)",
				len(old.Rules)),
		})
	}

	return d, warns
}

// generateULID — placeholder для миграции (использует v5.ulid пакет уже существующий).
// Для production миграции должен генерить настоящий ULID.
// Здесь упрощённо: timestamp-based unique ID.
func generateULID() string {
	// Берём микросекундный timestamp + counter — достаточно для миграции
	// (она одноразовая, в той же сессии ULID'ы не должны коллидить).
	now := time.Now().UnixNano()
	migrationCounter++
	return fmt.Sprintf("01J%X%X", now, migrationCounter)
}

var migrationCounter uint64

// IsV6 — детект schema version по сырому state JSON.
func IsV6(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Meta.Version == SchemaVersion
}

// IsV5 — детект v5 schema.
func IsV5(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Meta.Version == v5.SchemaVersion
}

// isLikelyLegacyLabel — heuristic для определения legacy-label'а.
// (placeholder для будущих расширений)
func isLikelyLegacyLabel(label string) bool {
	return strings.HasPrefix(label, "Russian ") ||
		strings.HasPrefix(label, "Block ") ||
		strings.HasPrefix(label, "Private ")
}
