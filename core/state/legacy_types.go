// File legacy_types.go — типы для user-rules и legacy v5 DNSOptions shape.
// Содержит ConfigParam, SettingVar, CustomRule, RuleType* constants, IsKnownRuleType.
// Legacy v5 DNSOptions определён здесь как `legacyDNSOptionsV5` (приватный alias-target)
// и используется State.DNSOptions field для backward-compat с v5 файлами.
package state

import (
	"encoding/json"
)

// DefaultMaxNodes — дефолтный потолок числа нод per-source. Зеркалит
// configtypes.MaxNodesPerSubscription (3000), но живёт здесь чтобы
// migration v4→v5 не зависела от парсера.
const DefaultMaxNodes = 3000

// legacySchemaVersionV5 — старый v5 disk schema version (5). Используется
// legacy parse path для detection. Public SchemaVersion = SchemaVersionV6
// после SPEC 060 (canonical write шкала).
const legacySchemaVersionV5 = 5

// ConfigParam — параметр маршрутизации (например, route.final).
type ConfigParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SettingVar — переопределение переменной шаблона (вкладка Settings).
type SettingVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CustomRule — пользовательское правило с полным определением.
type CustomRule struct {
	Label            string                 `json:"label"`
	Type             string                 `json:"type,omitempty"`
	Enabled          bool                   `json:"enabled"`
	SelectedOutbound string                 `json:"selected_outbound"`
	Description      string                 `json:"description,omitempty"`
	Rule             map[string]interface{} `json:"rule,omitempty"`
	DefaultOutbound  string                 `json:"default_outbound,omitempty"`
	HasOutbound      bool                   `json:"has_outbound"`
	Params           map[string]interface{} `json:"params,omitempty"`
	RuleSet          []json.RawMessage      `json:"rule_set,omitempty"`
}

// LegacyDNSOptionsV5 — legacy v5 DNS-секция (servers + rules + scalars).
// Используется только при чтении v5 файлов через parseV5 и доступна
// через State.DNSOptions field (для backward-compat UI кода).
//
// Canonical DNS shape — `DNSOptions` (v6, см. dns_options.go).
//
// Public (а не legacyDNSOptionsV5) чтобы UI (wizardmodels.PersistedDNSState
// = state.LegacyDNSOptionsV5) и v5 shim package могли его использовать без
// дублирования definition'а.
type LegacyDNSOptionsV5 struct {
	Servers               []json.RawMessage `json:"servers"`
	Rules                 []json.RawMessage `json:"rules,omitempty"`
	Final                 string            `json:"final,omitempty"`
	Strategy              string            `json:"strategy,omitempty"`
	IndependentCache      *bool             `json:"independent_cache,omitempty"`
	DefaultDomainResolver string            `json:"default_domain_resolver,omitempty"`
	ResolverUnset         bool              `json:"default_domain_resolver_unset,omitempty"`
}

// Известные константы типов правил.
const (
	RuleTypeIPS       = "ips"
	RuleTypeURLs      = "urls"
	RuleTypeProcesses = "processes"
	RuleTypeSRS       = "srs"
	RuleTypeRaw       = "raw"
)

// IsKnownRuleType возвращает true, если s — одна из актуальных констант типов.
func IsKnownRuleType(s string) bool {
	switch s {
	case RuleTypeIPS, RuleTypeURLs, RuleTypeProcesses, RuleTypeSRS, RuleTypeRaw:
		return true
	default:
		return false
	}
}
