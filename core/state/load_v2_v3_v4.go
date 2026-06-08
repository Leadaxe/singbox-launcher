package state

import (
	"encoding/json"
	"fmt"
	"time"

	"singbox-launcher/core/config/configtypes"
)

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
	v4 := &v4File{
		Version:      raw.Version,
		ID:           raw.ID,
		Comment:      raw.Comment,
		CreatedAt:    raw.CreatedAt,
		UpdatedAt:    raw.UpdatedAt,
		ConfigParams: raw.ConfigParams,
		Vars:         raw.Vars,
		CustomRules:  custom,
		DNSOptions:   raw.DNSOptions,
		ParserConfig: v4ParserConfig{
			Version:   pc.ParserConfig.Version,
			Proxies:   pc.ParserConfig.Proxies,
			Outbounds: pc.ParserConfig.Outbounds,
			Parser: v4Parser{
				Reload:      pc.ParserConfig.Parser.Reload,
				LastUpdated: pc.ParserConfig.Parser.LastUpdated,
			},
		},
	}
	migrated := migrateV4ToV5(v4, nil) // production: ULID

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
	deriveV6FromLegacy(s) // BUG1: derive v6 Rules/DNS from migrated legacy fields
	normalizeNilSlices(s)
	return s, nil
}

// rawLegacyFile — JSON-форма v2/v3/v4 для устойчивого декодирования.
type rawLegacyFile struct {
	Version              int                 `json:"version"`
	ID                   string              `json:"id,omitempty"`
	Comment              string              `json:"comment,omitempty"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
	ParserConfig         json.RawMessage     `json:"parser_config"`
	ConfigParams         []ConfigParam       `json:"config_params"`
	SelectableRuleStates json.RawMessage     `json:"selectable_rule_states"`
	CustomRules          json.RawMessage     `json:"custom_rules"`
	RulesLibraryMerged   bool                `json:"rules_library_merged"`
	DNSOptions           *LegacyDNSOptionsV5 `json:"dns_options"`
	Vars                 []SettingVar        `json:"vars"`
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
