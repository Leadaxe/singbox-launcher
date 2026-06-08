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
		Vars:         raw.Vars,
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

	// SPEC 070 ADR-070-2: read-time migration shim. v5 CustomRules/DNSOptions
	// (disk-only locals) мигрируются forward в canonical Rules/DNS in-memory —
	// они НЕ хранятся в State. Следующий Save пишет только v6.
	migrateLegacyIntoCanonical(s, raw.CustomRules, raw.DNSOptions)

	// Заполняем legacy proxies-view из Connections для backward-compat
	// callsite'ов (UI source_tab, dashboard counters, parser).
	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// migrateLegacyIntoCanonical — read-time migration shim (SPEC 070 ADR-070-2).
//
// Конвертит legacy v2..v5 CustomRules/DNSOptions (распарсенные из disk shape,
// передаются как локалы — НЕ State-поля) в canonical s.Rules / s.DNS, если
// canonical поля пусты. Без этого legacy-файл, загруженный headless'но (без
// Configurator UI), терял бы custom rules + DNS на следующем Save (marshalDisk
// сериализует ТОЛЬКО v6). Headless writers: Debug API PATCH, auto-save,
// log-level, subscription-refresh — все Load→mutate→Save без re-emit из модели.
//
// Template maps nil на parse-time (нет шаблона): best-effort kind detection;
// Configurator передеривит точно на open. Идемпотентно: noop когда canonical
// уже заполнен (чистый v6-файл такой ветки не достигает).
func migrateLegacyIntoCanonical(s *State, customRules []CustomRule, dnsOptions *LegacyDNSOptionsV5) {
	if len(s.Rules) == 0 && len(customRules) > 0 {
		for _, cr := range customRules {
			if r, _ := migrateCustomRule(cr, nil); r != nil {
				s.Rules = append(s.Rules, *r)
			}
		}
	}
	if len(s.DNS.Servers) == 0 && len(s.DNS.Rules) == 0 && dnsOptions != nil {
		s.DNS, _ = migrateDNS(dnsOptions, nil)
	}
}
