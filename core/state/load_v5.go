package state

import (
	"encoding/json"
	"fmt"
	"time"
)

// parseV5Legacy — прямой read v5-формата (legacy). После SPEC 060 Phase 5
// canonical write всегда v6, но v5-файлы юзеров читаются здесь и нормализуются
// в State. На следующем Save перезаписываются в v6 shape.
func parseV5Legacy(data []byte) (*State, error) {
	var raw struct {
		Meta         metaSectionV5       `json:"meta"`
		Connections  ConnectionsSection  `json:"connections"`
		ConfigParams []ConfigParam       `json:"config_params"`
		CustomRules  []CustomRule        `json:"custom_rules"`
		Vars         []SettingVar        `json:"vars"`
		DNSOptions   *LegacyDNSOptionsV5 `json:"dns_options"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v5 json: %w", err)
	}

	s := &State{
		Version:      raw.Meta.Version,
		Comment:      raw.Meta.Comment,
		Connections:  raw.Connections,
		ConfigParams: raw.ConfigParams,
		CustomRules:  raw.CustomRules,
		Vars:         raw.Vars,
		DNSOptions:   raw.DNSOptions,
		// Legacy флаги, которые v5 больше не сериализует — выставляем
		// дефолты, удобные UI-коду:
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// BUG1 fix: derive canonical v6 Rules/DNS from legacy v5 CustomRules/
	// DNSOptions так, чтобы headless Save (сериализует только v6) их не терял.
	deriveV6FromLegacy(s)

	// Заполняем legacy proxies-view из Connections для backward-compat
	// callsite'ов (UI source_tab, dashboard counters, parser).
	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// deriveV6FromLegacy populates canonical v6 s.Rules / s.DNS from the legacy
// s.CustomRules / s.DNSOptions when the v6 fields are empty. Without it a
// legacy-format state loaded headlessly (no Configurator UI) keeps Rules/DNS
// empty, and the next Save — which serializes ONLY v6 fields (marshalDisk) —
// silently drops the user's custom rules + DNS config (Debug API PATCH /
// auto-save / log-level / subscription-refresh paths all hit Load→mutate→Save
// without re-emitting from a model). Build branch-selection keys on
// len(s.Rules)>0 (routeConfigForUpdate) and v6Active (dnsConfigForUpdate), so
// once populated CustomRules/DNSOptions become a dormant legacy view — no
// double-emit. Template maps are nil at parse time (no template available):
// best-effort kind detection; the Configurator re-derives accurately on open.
func deriveV6FromLegacy(s *State) {
	if len(s.Rules) == 0 && len(s.CustomRules) > 0 {
		for _, cr := range s.CustomRules {
			if r, _ := migrateCustomRule(cr, nil); r != nil {
				s.Rules = append(s.Rules, *r)
			}
		}
	}
	if len(s.DNS.Servers) == 0 && len(s.DNS.Rules) == 0 && s.DNSOptions != nil {
		s.DNS, _ = migrateDNS(s.DNSOptions, nil)
	}
}
