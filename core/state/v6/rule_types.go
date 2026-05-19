// Package v6 — on-disk-схема state.json (SPEC 053).
//
// Расширяет v5: добавляет preset bundles (SPEC 053). Из v5 переиспользуются
// MetaSection, ConnectionsSection, Source и SubscriptionMeta — они без
// изменений. Изменения:
//
//   - state.custom_rules[] → state.rules[] с kind discriminator
//     (preset / inline / srs)
//   - state.config_params[] удалено — vars живут в preset.body.vars
//   - state.dns_options → state.dns с явным разделением:
//     template_servers (overrides) + extra_servers (user-defined)
//   - state.vars[] остаётся (глобальные template vars: cert_store, tun, ...)
//
// Top-level layout:
//
//	{
//	  "meta":        { version: 6, schema: "presets_v1", ... },
//	  "connections": { ... },  // как в v5
//	  "rules":       [...],     // header/body kind discriminator
//	  "vars":        [...],     // глобальные wizard vars (не preset.vars)
//	  "dns":         {...}      // template_servers / extra_servers / extra_rules
//	}
//
// См. SPECS/053-F-N-PRESET_BUNDLES/SPEC.md.
package v6

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion — формат файла state.json, который пишет v6.
const SchemaVersion = 6

// SchemaName — внутренний идентификатор схемы (хранится в meta.schema).
// Используется для диагностики и future-proof'инга.
const SchemaName = "presets_v1"

// RuleKind — дискриминатор типа правила в state.rules[].
type RuleKind string

const (
	// RuleKindPreset — тонкая ссылка на template.presets[].
	// Body: {vars}. Match-поля живут в template.
	RuleKindPreset RuleKind = "preset"

	// RuleKindInline — user-defined inline rule.
	// Body: {name, match, outbound}. Match-поля в state.
	RuleKindInline RuleKind = "inline"

	// RuleKindSrs — user-defined srs rule.
	// Body: {name, srs_url, outbound}. Cached .srs файл на диске.
	RuleKindSrs RuleKind = "srs"
)

// Rule — единица в state.rules[] с header/body разделением.
//
// Header содержит только то что общее для всех kind'ов: discriminator,
// identifier (Ref для preset / ID для user) и enabled toggle. Kind-specific
// payload — в Body, парсится по dispatcher'у через DecodeBody.
//
// Сериализация:
//
//	{
//	  "kind":     "preset" | "inline" | "srs",
//	  "ref":      "<preset_id>",  // только для kind=preset
//	  "id":       "<ulid>",       // только для kind=inline | srs
//	  "enabled":  true | false,
//	  "body":     { ... }         // kind-specific
//	}
type Rule struct {
	// Kind — discriminator. Required.
	Kind RuleKind `json:"kind"`

	// Ref — ссылка на template.presets[].id. Required для kind=preset, иначе пуст.
	Ref string `json:"ref,omitempty"`

	// ID — ULID. Required для kind=inline|srs, иначе пуст.
	ID string `json:"id,omitempty"`

	// Enabled — общий toggle.
	Enabled bool `json:"enabled"`

	// Body — raw payload, декодируется через DecodeBody по Kind.
	Body json.RawMessage `json:"body"`
}

// PresetBody — kind=preset payload.
//
// Vars хранит ТОЛЬКО diff от template-default'ов. Пустой map = всё дефолтное.
// Bump'нули template → юзер автоматически получает новые дефолты для var'ов
// которые он не трогал.
type PresetBody struct {
	Vars map[string]string `json:"vars"`
}

// InlineBody — kind=inline payload (user-defined inline rule).
type InlineBody struct {
	// Name — отображаемое имя в UI.
	Name string `json:"name"`

	// Match — sing-box match-объект (domain/domain_suffix/ip_cidr/port/...).
	Match map[string]interface{} `json:"match"`

	// Outbound — outbound tag или зарезервированный литерал "reject" / "drop".
	Outbound string `json:"outbound"`
}

// SrsBody — kind=srs payload (user-defined srs rule).
type SrsBody struct {
	Name     string `json:"name"`
	SrsURL   string `json:"srs_url"`
	Outbound string `json:"outbound"` // tag | "reject" | "drop"
}

// DecodeBody парсит Rule.Body в kind-specific тип.
// Возвращает один из {*PresetBody, *InlineBody, *SrsBody}.
//
// Ошибки:
//   - kind=preset без ref / inline|srs без id — semantic error
//   - kind unknown — error
//   - JSON unmarshal failed — error
func (r *Rule) DecodeBody() (interface{}, error) {
	switch r.Kind {
	case RuleKindPreset:
		if r.Ref == "" {
			return nil, fmt.Errorf("rule kind=preset requires ref")
		}
		if r.ID != "" {
			return nil, fmt.Errorf("rule kind=preset must not have id (use ref)")
		}
		var body PresetBody
		if len(r.Body) > 0 {
			if err := json.Unmarshal(r.Body, &body); err != nil {
				return nil, fmt.Errorf("decode preset body: %w", err)
			}
		}
		if body.Vars == nil {
			body.Vars = make(map[string]string)
		}
		return &body, nil

	case RuleKindInline:
		if r.ID == "" {
			return nil, fmt.Errorf("rule kind=inline requires id")
		}
		if r.Ref != "" {
			return nil, fmt.Errorf("rule kind=inline must not have ref (use id)")
		}
		var body InlineBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return nil, fmt.Errorf("decode inline body: %w", err)
		}
		return &body, nil

	case RuleKindSrs:
		if r.ID == "" {
			return nil, fmt.Errorf("rule kind=srs requires id")
		}
		if r.Ref != "" {
			return nil, fmt.Errorf("rule kind=srs must not have ref (use id)")
		}
		var body SrsBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			return nil, fmt.Errorf("decode srs body: %w", err)
		}
		return &body, nil

	default:
		return nil, fmt.Errorf("unknown rule kind: %q", r.Kind)
	}
}

// DNSConfig — раздел DNS в state.json (SPEC 053 + SPEC 057 cleanup).
//
// Invariant (SPEC 057): **state хранит только REF'ы на template entities, не
// копии тел**. DNS-серверы и DNS-правила, которые юзер видит в UI, целиком
// определены template (config.dns.servers + dns_options.servers + presets.
// dns_servers + presets.dns_rule). State хранит только:
//   - scalars: strategy / independent_cache / final / default_domain_resolver
//   - TemplateServers — override `{enabled}` для template-defined серверов
//     (thin diff: не trustworthy копии тел, а только тогглы)
//
// extra_servers / extra_rules были изначальной ошибкой SPEC 053 — они хранили
// **копии template body** (server config'и, dns rule'ы) inline. Это давало:
//   - dangling refs когда template менялся (ru-domains tag исчез — rule остался)
//   - double-emit (build видел extras дважды: через legacy view + через v6)
//   - синхронизация двух источников истины
// → удалены полностью (SPEC 057). UI больше не позволяет добавлять кастомный
// DNS-сервер или DNS-rule напрямую; всё через preset.dns_servers / preset.dns_rule.
//
// При build:
//
//	dns.servers[] = template.dns_options.servers[].filter(effective_enabled)
//	              + bundled из active preset-refs
//	              (никаких extras)
//
//	dns.rules[] = preset.dns_rule от каждого active preset-ref
//	            (никаких extras)
//
//	effective_enabled(tag) = state.dns.template_servers[tag].enabled
//	                     ?? template.dns_options.servers[tag].default_enabled
//	                     ?? true
type DNSConfig struct {
	Strategy              string                       `json:"strategy,omitempty"`
	IndependentCache      bool                         `json:"independent_cache,omitempty"`
	Final                 string                       `json:"final,omitempty"`
	DefaultDomainResolver string                       `json:"default_domain_resolver,omitempty"`
	TemplateServers       map[string]TemplateServerOvr `json:"template_servers,omitempty"`
}

// TemplateServerOvr — override для template-defined DNS-сервера.
// Хранится только если значение Enabled отличается от template default_enabled.
type TemplateServerOvr struct {
	Enabled bool `json:"enabled"`
}
