// File dns_options.go — SPEC 056-R-N: DNS_SCHEMA_REDESIGN.
//
// Новая flat-схема DNS-секции state.json через kind discriminator
// (template/preset/user) для servers, (preset/user) для rules.
//
// JSON layout:
//
//	"dns_options": {
//	  "strategy": "...",
//	  "final": "...",
//	  "default_domain_resolver": "...",
//	  "servers": [
//	    {"kind":"template", "tag":"cloudflare_udp", "enabled":true},
//	    {"kind":"preset",   "ref":"russian:yandex_udp", "enabled":true},
//	    {"kind":"user",     "tag":"my-pihole", "type":"udp", "server":"192.168.1.5", "enabled":true}
//	  ],
//	  "rules": [
//	    {"kind":"preset", "ref":"russian", "enabled":true},
//	    {"kind":"user",   "rule_set":"ru-domains", "server":"yandex_doh", "enabled":true}
//	  ]
//	}
//
// Инварианты:
//  1. Memory == disk — никакого runtime materialization для preset entries.
//     SyncDNSOptionsWithActivePresets синхронизирует list с активным набором
//     preset-ref'ов в state.Rules[] (вызывается на load + на toggle).
//  2. kind=template/preset тело резолвится из template на build/render — на
//     диске только {kind, tag|ref, enabled}.
//  3. kind=user — полное тело сериализуется flat'ом рядом с kind/tag/enabled.
//
// См. SPECS/056-R-N-DNS_SCHEMA_REDESIGN/SPEC.md.
package state

import (
	"encoding/json"
	"fmt"
)

// DNSServerKind — дискриминатор entry в dns_options.servers[].
type DNSServerKind string

const (
	// DNSServerKindTemplate — тонкая ссылка на template.dns_options.servers[tag].
	// Юзер может toggle Enabled; тело берётся из template на build.
	DNSServerKindTemplate DNSServerKind = "template"

	// DNSServerKindPreset — тонкая ссылка на template.presets[X].dns_servers[Y].
	// Ref в формате "<preset_id>:<local_tag>". Авто-add/remove через
	// SyncDNSOptionsWithActivePresets при toggle preset'а в Rules tab.
	DNSServerKindPreset DNSServerKind = "preset"

	// DNSServerKindUser — genuinely user-defined DNS-сервер, нет template-аналога.
	// Полное тело сериализуется flat'ом (type, server, server_port, tls, ...).
	DNSServerKindUser DNSServerKind = "user"
)

// DNSRuleKind — дискриминатор entry в dns_options.rules[].
type DNSRuleKind string

const (
	// DNSRuleKindPreset — тонкая ссылка на template.presets[X].dns_rule.
	// Ref в формате "<preset_id>" (один dns_rule на preset максимум).
	DNSRuleKindPreset DNSRuleKind = "preset"

	// DNSRuleKindUser — user-defined DNS rule. Полное тело (rule_set, server,
	// domain_*, ip_cidr, ...) сериализуется flat'ом.
	DNSRuleKindUser DNSRuleKind = "user"
)

// DNSServer — entry в state.dns_options.servers[].
//
// Сериализация плоская: kind/ref/tag/enabled на верхнем уровне, плюс body-поля
// (только для kind=user). Marshal/Unmarshal реализованы вручную чтобы достичь
// этой формы.
type DNSServer struct {
	Kind DNSServerKind

	// Tag — для kind=template (template.dns_options.servers[tag]) и kind=user
	// (display tag в финальном config.dns.servers[].tag). Пуст для kind=preset.
	Tag string

	// Ref — только для kind=preset, формат "<preset_id>:<local_tag>".
	// Пуст для остальных kind'ов.
	Ref string

	// Enabled — toggle. Build pipeline пропускает entry если false.
	Enabled bool

	// Body — для kind=user полные DNS-server поля (type, server, server_port,
	// tls, detour, ...). nil/пуст для kind=template/preset.
	//
	// **Не содержит** kind/ref/enabled (они на top-level). Может содержать
	// tag (для kind=user — собственный display tag), но это дублирует поле
	// `Tag` и сериализатор предпочитает поле Tag.
	Body map[string]interface{}
}

// DNSRule — entry в state.dns_options.rules[].
type DNSRule struct {
	Kind DNSRuleKind

	// Ref — только для kind=preset, формат "<preset_id>".
	Ref string

	// Enabled — toggle.
	Enabled bool

	// Body — для kind=user полное тело sing-box dns rule (rule_set, server,
	// domain_*, ip_cidr, port, network, ...). nil/пуст для kind=preset.
	Body map[string]interface{}
}

// DNSOptions — раздел dns_options в state.json (SPEC 056-R-N).
//
// dns_* scalars (Strategy / Final / DefaultDomainResolver) дублируются
// здесь и в state.vars[]; для совместимости с template-substitute vars
// остаётся source of truth, а DNSOptions-scalars читаются как fallback
// (см. core/config_service.go::dnsConfigForUpdate).
//
// **Важно:** SPEC 056 решил оставить dns_* scalars в state.vars[] как единый
// KV-store. Поля Strategy/Final/DefaultDomainResolver здесь присутствуют для
// in-memory работы build pipeline, но **не сериализуются** если они zero-value
// (omitempty). На диск ходит вариант "хранится в vars[]".
//
// SPEC: IndependentCache УДАЛЕНО — deprecated в sing-box 1.14.0 (кэш всегда
// per-transport). Legacy state.json с этим ключом парсится без ошибок
// (unknown field ignored), новые state'ы поле не пишут.
type DNSOptions struct {
	Strategy              string `json:"strategy,omitempty"`
	Final                 string `json:"final,omitempty"`
	DefaultDomainResolver string `json:"default_domain_resolver,omitempty"`

	Servers []DNSServer `json:"servers,omitempty"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// ── Marshal/Unmarshal: flat layout ─────────────────────────────────

// MarshalJSON — flat сериализация: kind/ref/tag/enabled на верхнем уровне +
// body fields (для kind=user) рядом с ними.
func (s DNSServer) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{}, 4+len(s.Body))
	out["kind"] = string(s.Kind)
	out["enabled"] = s.Enabled
	switch s.Kind {
	case DNSServerKindTemplate, DNSServerKindUser:
		if s.Tag != "" {
			out["tag"] = s.Tag
		}
	case DNSServerKindPreset:
		if s.Ref != "" {
			out["ref"] = s.Ref
		}
	}
	if s.Kind == DNSServerKindUser {
		for k, v := range s.Body {
			// kind/ref/enabled никогда не должны попадать в body, но если
			// кто-то их туда положил — top-level выигрывает.
			switch k {
			case "kind", "ref", "enabled":
				continue
			case "tag":
				// Tag уже выставлен из поля Tag.
				if s.Tag == "" {
					out["tag"] = v
				}
				continue
			}
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON — flat десериализация: достаёт kind/ref/tag/enabled, остаток
// складывает в Body (для kind=user; для template/preset Body остаётся nil).
func (s *DNSServer) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("dns server: %w", err)
	}
	kind, _ := raw["kind"].(string)
	s.Kind = DNSServerKind(kind)
	if t, ok := raw["tag"].(string); ok {
		s.Tag = t
	}
	if r, ok := raw["ref"].(string); ok {
		s.Ref = r
	}
	if e, ok := raw["enabled"].(bool); ok {
		s.Enabled = e
	}

	if s.Kind == DNSServerKindUser {
		s.Body = make(map[string]interface{}, len(raw))
		for k, v := range raw {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			s.Body[k] = v
		}
	}
	return nil
}

// MarshalJSON — flat сериализация для DNSRule (зеркало DNSServer).
func (r DNSRule) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{}, 3+len(r.Body))
	out["kind"] = string(r.Kind)
	out["enabled"] = r.Enabled
	if r.Kind == DNSRuleKindPreset && r.Ref != "" {
		out["ref"] = r.Ref
	}
	if r.Kind == DNSRuleKindUser {
		for k, v := range r.Body {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON — flat десериализация для DNSRule.
func (r *DNSRule) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("dns rule: %w", err)
	}
	kind, _ := raw["kind"].(string)
	r.Kind = DNSRuleKind(kind)
	if ref, ok := raw["ref"].(string); ok {
		r.Ref = ref
	}
	if e, ok := raw["enabled"].(bool); ok {
		r.Enabled = e
	}
	if r.Kind == DNSRuleKindUser {
		r.Body = make(map[string]interface{}, len(raw))
		for k, v := range raw {
			switch k {
			case "kind", "ref", "enabled":
				continue
			}
			r.Body[k] = v
		}
	}
	return nil
}

// ── Helpers ────────────────────────────────────────────────────────

// FindServerByTag возвращает индекс template/user-server с tag, или -1.
// (kind=preset идентифицируются через Ref, не Tag — для них используй FindServerByRef.)
func (d *DNSOptions) FindServerByTag(tag string) int {
	if d == nil {
		return -1
	}
	for i, s := range d.Servers {
		if (s.Kind == DNSServerKindTemplate || s.Kind == DNSServerKindUser) && s.Tag == tag {
			return i
		}
	}
	return -1
}

// FindServerByRef возвращает индекс preset-server с ref, или -1.
func (d *DNSOptions) FindServerByRef(ref string) int {
	if d == nil {
		return -1
	}
	for i, s := range d.Servers {
		if s.Kind == DNSServerKindPreset && s.Ref == ref {
			return i
		}
	}
	return -1
}

// FindRuleByRef возвращает индекс preset-rule с ref, или -1.
func (d *DNSOptions) FindRuleByRef(ref string) int {
	if d == nil {
		return -1
	}
	for i, r := range d.Rules {
		if r.Kind == DNSRuleKindPreset && r.Ref == ref {
			return i
		}
	}
	return -1
}

// IsEmpty — true если в DNSOptions нет ни одного поля (scalars + servers + rules).
// Используется sequence'ами omitempty / нормализации.
func (d *DNSOptions) IsEmpty() bool {
	if d == nil {
		return true
	}
	return d.Strategy == "" &&
		d.Final == "" &&
		d.DefaultDomainResolver == "" &&
		len(d.Servers) == 0 &&
		len(d.Rules) == 0
}
