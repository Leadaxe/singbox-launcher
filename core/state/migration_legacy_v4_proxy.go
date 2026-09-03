// File migration_legacy_v4_proxy.go — ВХОД миграции: форма `parser_config.
// proxies[]` схем v2–v4.
//
// SPEC 118 W5: сборочная форма (configtypes.ProxySource) больше не несёт
// легаси-полей — она проекция канона v7 и ничего не хранит. Но состояния
// v2–v4 держали источники именно в ней, и одноразовая миграция обязана их
// прочитать. Поэтому on-disk форма живёт ЗДЕСЬ, приватным типом рядом с
// миграцией: она никогда не доезжает ни до сборки, ни до диска.
//
// Санкционированное исключение grep-инвариантов SPEC §4.A («читатели
// миграции — единственное исключение»).
package state

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// legacyProxyV4 — один элемент `parser_config.proxies[]` схем v2–v4.
// Читается ТОЛЬКО migrateLegacySources (v4 → v5-форма sourceV6).
type legacyProxyV4 struct {
	ID          string              `json:"id,omitempty"`
	Label       string              `json:"label,omitempty"`
	Source      string              `json:"source,omitempty"`
	Connections []string            `json:"connections,omitempty"`
	Skip        []map[string]string `json:"skip,omitempty"`
	Disabled    bool                `json:"disabled,omitempty"`

	Outbounds []configtypes.Direction `json:"outbounds,omitempty"`

	TagPrefix  string `json:"tag_prefix,omitempty"`
	TagPostfix string `json:"tag_postfix,omitempty"`
	TagMask    string `json:"tag_mask,omitempty"`

	ExcludeFromGlobal       bool        `json:"exclude_from_global,omitempty"`
	ExposeGroupTagsToGlobal bool        `json:"expose_group_tags_to_global,omitempty"`
	Fold                    *legacyFold `json:"fold,omitempty"`

	DetourTag          string `json:"detour_tag,omitempty"`
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`
	DetourNodeHash     string `json:"detour_node_hash,omitempty"`
	DetourNodeLabel    string `json:"detour_node_label,omitempty"`

	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`
	ConfigJSON    json.RawMessage  `json:"config_json,omitempty"`
}
