// Package v5 — SHIM after SPEC 060 Phase 3.
//
// All v5 types/funcs now live in `core/state` (parent package). This package
// remains only as a thin re-export shim to keep external callsites compiling
// during Phase 3/4 — they'll be updated in Phase 4 to import `core/state`
// directly, at which point this package is deleted (Phase 6).
//
// DO NOT add new symbols here. All new work goes in `core/state`.
package v5

import (
	"singbox-launcher/core/state"
)

// ── Constants ────────────────────────────────────────────────────

const (
	SchemaVersion          = state.SchemaVersion
	DefaultMaxNodes        = state.DefaultMaxNodes
	SourceTypeSubscription = state.SourceTypeSubscription
	SourceTypeServer       = state.SourceTypeServer

	RuleTypeIPS       = state.RuleTypeIPS
	RuleTypeURLs      = state.RuleTypeURLs
	RuleTypeProcesses = state.RuleTypeProcesses
	RuleTypeSRS       = state.RuleTypeSRS
	RuleTypeRaw       = state.RuleTypeRaw
)

// ── Types ────────────────────────────────────────────────────────

type (
	ConnectionsSection = state.ConnectionsSection
	Source             = state.Source
	SourceType         = state.SourceType
	Defaults           = state.Defaults
	TagSpec            = state.TagSpec
	UpdateSpec         = state.UpdateSpec
	SubscriptionMeta   = state.SubscriptionMeta
	UserInfo           = state.UserInfo

	ConfigParam = state.ConfigParam
	SettingVar  = state.SettingVar
	CustomRule  = state.CustomRule
)

// DNSOptions — re-export of state.LegacyDNSOptionsV5 (legacy v5 DNS shape).
//
// Используется UI wizardmodels.PersistedDNSState и legacy subscription/meta
// (HTTP header parsing создаёт v5.SubscriptionMeta для записи в Source.Meta).
type DNSOptions = state.LegacyDNSOptionsV5

// MetaSection — re-export of legacy v5 meta section shape.
//
// Точная копия приватного `state.metaSectionV5`. Используется ParseHeaders
// callers через Source.Meta (хотя метаданные подписки живут в SubscriptionMeta,
// а MetaSection — top-level meta файла).
//
// После Phase 4 callsites переходят на state.MetaSection (v6 shape с
// дополнительным Schema field; backward-compat в JSON через omitempty).
type MetaSection struct {
	Version   int    `json:"version"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// State — re-export legacy disk shape v5. Точная копия приватного `state.diskStateV5`.
// Используется только Phase 2/3 для backward-compat — после Phase 4 callsites
// его не используют.
type State struct {
	Meta         MetaSection              `json:"meta"`
	Connections  state.ConnectionsSection `json:"connections"`
	ConfigParams []state.ConfigParam      `json:"config_params"`
	CustomRules  []state.CustomRule       `json:"custom_rules"`
	Vars         []state.SettingVar       `json:"vars,omitempty"`
	DNSOptions   *DNSOptions              `json:"dns_options"`
}

// ── Functions ────────────────────────────────────────────────────

// MakeULID — re-export.
func MakeULID() string { return state.MakeULID() }

// WriteRawBody / ReadRawBody / DeleteOrphans — re-export.
func WriteRawBody(subsDir, id string, body []byte) error {
	return state.WriteRawBody(subsDir, id, body)
}
func ReadRawBody(subsDir, id string) ([]byte, error) {
	return state.ReadRawBody(subsDir, id)
}
func DeleteOrphans(subsDir string, knownIDs []string) ([]string, error) {
	return state.DeleteOrphans(subsDir, knownIDs)
}
