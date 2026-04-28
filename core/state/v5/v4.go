package v5

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// V4File — on-disk форма state.json v4 в её "упрощённом" виде (как пишет
// текущий core/state.Save). Используется только как input для
// MigrateV4ToV5 — ни core/state, ни остальной код её не используют.
//
// Схема собрана по:
//
//   - core/state/state.go (legacy SchemaVersion=4)
//   - core/state/save.go::marshalDisk (упрощённый parser_config)
//   - core/state/load.go::rawFile / decodeParserConfig (что реально
//     умеет читать сейчас Load)
type V4File struct {
	Version              int                   `json:"version"`
	ID                   string                `json:"id,omitempty"`
	Comment              string                `json:"comment,omitempty"`
	CreatedAt            string                `json:"created_at"`
	UpdatedAt            string                `json:"updated_at"`
	ParserConfig         V4ParserConfig        `json:"parser_config"`
	ConfigParams         []ConfigParam         `json:"config_params"`
	Vars                 []SettingVar          `json:"vars,omitempty"`
	SelectableRuleStates []V4SelectableRuleSt  `json:"selectable_rule_states,omitempty"`
	CustomRules          []CustomRule          `json:"custom_rules"`
	RulesLibraryMerged   bool                  `json:"rules_library_merged,omitempty"`
	DNSOptions           *DNSOptions           `json:"dns_options,omitempty"`
}

// V4ParserConfig — упрощённый layout (без обёртки configtypes.ParserConfig).
type V4ParserConfig struct {
	Version   int                          `json:"version,omitempty"`
	Proxies   []configtypes.ProxySource    `json:"proxies"`
	Outbounds []configtypes.OutboundConfig `json:"outbounds"`
	Parser    V4Parser                     `json:"parser,omitempty"`
}

// V4Parser — параметры обновления (только Reload остаётся в v5).
type V4Parser struct {
	Reload      string `json:"reload,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// V4SelectableRuleSt — снимок выбора пользователя для template-rule.
// В v5 SelectableRuleStates выпиливается; поле сохраняется здесь только
// для read-в-MigrateV4ToV5 (миграция игнорирует, но мы должны корректно
// распарсить файл с этим ключом).
type V4SelectableRuleSt struct {
	Label            string `json:"label"`
	Enabled          bool   `json:"enabled"`
	SelectedOutbound string `json:"selected_outbound"`
}

// ParseV4 разбирает v4-state из сырых байт. Не делает миграцию — только
// JSON-парсинг в V4File.
func ParseV4(data []byte) (*V4File, error) {
	var f V4File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
