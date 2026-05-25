// File rule_types.go — Rule, RuleKind, PresetBody, InlineBody, SrsBody (SPEC 053).
package state

import (
	"encoding/json"
	"fmt"
)

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
