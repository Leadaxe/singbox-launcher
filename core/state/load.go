package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"singbox-launcher/core/config/configtypes"
	v5 "singbox-launcher/core/state/v5"
)

// ErrNotFound — state-файл не существует. Вызывающий обычно интерпретирует
// это как «свежая установка», а не как ошибку.
var ErrNotFound = errors.New("state: file not found")

// Load читает state.json по пути path.
//
// Поведение:
//   - файл отсутствует → ErrNotFound;
//   - v5 (top-level "meta" с "version":5) → парсим напрямую;
//   - v2 / v3 / v4 (top-level "version") → legacy decode + auto-миграция в v5;
//   - неизвестная версия → ошибка «regenerate via wizard»;
//   - битый JSON → ошибка с понятным контекстом.
//
// Save после Load всегда пишет SchemaVersion (v5).
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse — Load из уже прочитанных байтов.
func Parse(data []byte) (*State, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("state: empty payload")
	}

	// Шаг 1: попытаться распознать формат — есть ли top-level "meta"?
	var probe struct {
		TopLevelVersion int `json:"version"`
		Meta            struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("state: parse json: %w", err)
	}

	switch {
	case probe.Meta.Version >= 5:
		return parseV5(data)
	case probe.TopLevelVersion >= 2 && probe.TopLevelVersion <= 4:
		return parseLegacyAndMigrate(data)
	case probe.TopLevelVersion == 0 && probe.Meta.Version == 0:
		return nil, fmt.Errorf("state: unknown schema (neither legacy version nor meta.version present)")
	default:
		return nil, fmt.Errorf("state: unsupported version (top=%d, meta.version=%d) — regenerate via Configurator",
			probe.TopLevelVersion, probe.Meta.Version)
	}
}

// parseV5 — прямой read v5-формата.
func parseV5(data []byte) (*State, error) {
	var raw struct {
		Meta         v5.MetaSection         `json:"meta"`
		Connections  v5.ConnectionsSection  `json:"connections"`
		ConfigParams []ConfigParam          `json:"config_params"`
		CustomRules  []CustomRule           `json:"custom_rules"`
		Vars         []SettingVar           `json:"vars"`
		DNSOptions   *DNSOptions            `json:"dns_options"`
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

	// Заполняем legacy proxies-view из Connections для backward-compat
	// callsite'ов (UI source_tab, dashboard counters, parser).
	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// parseLegacyAndMigrate — v2/v3/v4 → in-memory v5 (с legacy-view заполненной
// для обратной совместимости).
func parseLegacyAndMigrate(data []byte) (*State, error) {
	var raw rawLegacyFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse legacy json: %w", err)
	}

	// 1. Декодируем parser_config (поддерживает оба формата —
	//    "wrapped" v2 и "simplified" v3+).
	var pc configtypes.ParserConfig
	if err := decodeParserConfig(raw.ParserConfig, &pc); err != nil {
		return nil, err
	}

	// 2. Мигрируем selectable / custom rules как раньше.
	var selectable []SelectableRuleState
	if len(raw.SelectableRuleStates) > 0 {
		selectable = migrateSelectableRuleStates(raw.SelectableRuleStates)
	}
	var custom []CustomRule
	if len(raw.CustomRules) > 0 {
		custom = migrateCustomRules(raw.CustomRules)
	}

	// 3. Собираем v4-snapshot для v5-миграции.
	v4 := &v5.V4File{
		Version:      raw.Version,
		ID:           raw.ID,
		Comment:      raw.Comment,
		CreatedAt:    raw.CreatedAt,
		UpdatedAt:    raw.UpdatedAt,
		ConfigParams: raw.ConfigParams,
		Vars:         raw.Vars,
		CustomRules:  custom,
		DNSOptions:   raw.DNSOptions,
		ParserConfig: v5.V4ParserConfig{
			Version:   pc.ParserConfig.Version,
			Proxies:   pc.ParserConfig.Proxies,
			Outbounds: pc.ParserConfig.Outbounds,
			Parser: v5.V4Parser{
				Reload:      pc.ParserConfig.Parser.Reload,
				LastUpdated: pc.ParserConfig.Parser.LastUpdated,
			},
		},
	}
	migrated := v5.MigrateV4ToV5(v4, nil) // production: ULID

	s := &State{
		Version:              SchemaVersion,
		ID:                   raw.ID,
		Comment:              raw.Comment,
		ParserConfig:         pc,
		Connections:          migrated.Connections,
		ConfigParams:         migrated.ConfigParams,
		Vars:                 migrated.Vars,
		SelectableRuleStates: selectable,
		CustomRules:          migrated.CustomRules,
		RulesLibraryMerged:   raw.RulesLibraryMerged,
		DNSOptions:           migrated.DNSOptions,
	}
	if t, err := time.Parse(time.RFC3339, raw.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}
	normalizeNilSlices(s)
	return s, nil
}

// rawLegacyFile — JSON-форма v2/v3/v4 для устойчивого декодирования.
type rawLegacyFile struct {
	Version              int             `json:"version"`
	ID                   string          `json:"id,omitempty"`
	Comment              string          `json:"comment,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	ParserConfig         json.RawMessage `json:"parser_config"`
	ConfigParams         []ConfigParam   `json:"config_params"`
	SelectableRuleStates json.RawMessage `json:"selectable_rule_states"`
	CustomRules          json.RawMessage `json:"custom_rules"`
	RulesLibraryMerged   bool            `json:"rules_library_merged"`
	DNSOptions           *DNSOptions     `json:"dns_options"`
	Vars                 []SettingVar    `json:"vars"`
}

// decodeParserConfig — поддерживает два on-disk формата:
//
//  1. Упрощённый (v3+): {version, proxies, outbounds, parser}.
//  2. Старый (v2 и ранее): обёрнутый {"ParserConfig":{…}}.
func decodeParserConfig(raw json.RawMessage, dst *configtypes.ParserConfig) error {
	if len(raw) == 0 {
		return nil
	}

	var simplified struct {
		Version   int                          `json:"version"`
		Proxies   []configtypes.ProxySource    `json:"proxies"`
		Outbounds []configtypes.OutboundConfig `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`
			LastUpdated string `json:"last_updated,omitempty"`
		} `json:"parser,omitempty"`
	}
	if err := json.Unmarshal(raw, &simplified); err == nil && simplified.Proxies != nil {
		dst.ParserConfig.Version = simplified.Version
		dst.ParserConfig.Proxies = simplified.Proxies
		dst.ParserConfig.Outbounds = simplified.Outbounds
		dst.ParserConfig.Parser = simplified.Parser
		return nil
	}

	var legacy configtypes.ParserConfig
	if err := json.Unmarshal(raw, &legacy); err == nil {
		*dst = legacy
		return nil
	}

	return fmt.Errorf("state: parser_config: unsupported shape")
}

// migrateSelectableRuleStates — перенос как новых snapshots, так и старых
// (v2-эпоха) с вложенным rule.label.
func migrateSelectableRuleStates(raw json.RawMessage) []SelectableRuleState {
	var modern []SelectableRuleState
	if err := json.Unmarshal(raw, &modern); err == nil && (len(modern) == 0 || modern[0].Label != "") {
		return modern
	}

	var legacy []struct {
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label string `json:"label"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil {
		out := make([]SelectableRuleState, 0, len(legacy))
		for _, x := range legacy {
			if x.Rule.Label == "" {
				continue
			}
			out = append(out, SelectableRuleState{
				Label:            x.Rule.Label,
				Enabled:          x.Enabled,
				SelectedOutbound: x.SelectedOutbound,
			})
		}
		return out
	}

	return nil
}

// migrateCustomRules — аналогично, для custom_rules.
func migrateCustomRules(raw json.RawMessage) []CustomRule {
	var modern []CustomRule
	if err := json.Unmarshal(raw, &modern); err == nil && (len(modern) == 0 || modern[0].Label != "") {
		return modern
	}

	var legacy []struct {
		Type             string `json:"type"`
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label           string                 `json:"label"`
			Description     string                 `json:"description"`
			Raw             map[string]interface{} `json:"raw"`
			DefaultOutbound string                 `json:"default_outbound"`
			HasOutbound     bool                   `json:"has_outbound"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil {
		out := make([]CustomRule, 0, len(legacy))
		for _, x := range legacy {
			out = append(out, CustomRule{
				Label:            x.Rule.Label,
				Type:             x.Type,
				Enabled:          x.Enabled,
				SelectedOutbound: x.SelectedOutbound,
				Description:      x.Rule.Description,
				Rule:             x.Rule.Raw,
				DefaultOutbound:  x.Rule.DefaultOutbound,
				HasOutbound:      x.Rule.HasOutbound,
			})
		}
		return out
	}

	return nil
}

// normalizeNilSlices — приводим nil-slices к пустым для удобства callsite'ов.
func normalizeNilSlices(s *State) {
	if s.ConfigParams == nil {
		s.ConfigParams = []ConfigParam{}
	}
	if s.CustomRules == nil {
		s.CustomRules = []CustomRule{}
	}
	if s.Connections.Sources == nil {
		s.Connections.Sources = []Source{}
	}
}
