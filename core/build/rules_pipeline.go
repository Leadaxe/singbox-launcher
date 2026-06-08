package build

import (
	"encoding/json"
	"fmt"
)

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
