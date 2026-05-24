// File legacy_v4.go — v4 on-disk shape (input для migrateV4ToV5).
// Все типы приватные — используются только в legacy_migration.go.
package state

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// v4File — on-disk форма state.json v4. Только input для migrateV4ToV5.
type v4File struct {
	Version              int                  `json:"version"`
	ID                   string               `json:"id,omitempty"`
	Comment              string               `json:"comment,omitempty"`
	CreatedAt            string               `json:"created_at"`
	UpdatedAt            string               `json:"updated_at"`
	ParserConfig         v4ParserConfig       `json:"parser_config"`
	ConfigParams         []ConfigParam        `json:"config_params"`
	Vars                 []SettingVar         `json:"vars,omitempty"`
	SelectableRuleStates []v4SelectableRuleSt `json:"selectable_rule_states,omitempty"`
	CustomRules          []CustomRule         `json:"custom_rules"`
	RulesLibraryMerged   bool                 `json:"rules_library_merged,omitempty"`
	DNSOptions           *LegacyDNSOptionsV5  `json:"dns_options,omitempty"`
}

// v4ParserConfig — упрощённый layout (без обёртки configtypes.ParserConfig).
type v4ParserConfig struct {
	Version   int                          `json:"version,omitempty"`
	Proxies   []configtypes.ProxySource    `json:"proxies"`
	Outbounds []configtypes.OutboundConfig `json:"outbounds"`
	Parser    v4Parser                     `json:"parser,omitempty"`
}

// v4Parser — параметры обновления (только Reload остаётся в v5).
type v4Parser struct {
	Reload      string `json:"reload,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// v4SelectableRuleSt — снимок выбора пользователя для template-rule.
// В v5 SelectableRuleStates выпиливается; поле сохраняется здесь только
// для read-в-migrateV4ToV5 (миграция игнорирует, но мы должны корректно
// распарсить файл с этим ключом).
type v4SelectableRuleSt struct {
	Label            string `json:"label"`
	Enabled          bool   `json:"enabled"`
	SelectedOutbound string `json:"selected_outbound"`
}

// parseV4File разбирает v4-state из сырых байт. Не делает миграцию — только
// JSON-парсинг в v4File.
func parseV4File(data []byte) (*v4File, error) {
	var f v4File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
