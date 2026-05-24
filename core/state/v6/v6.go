// Package v6 — SHIM after SPEC 060 Phase 2.
//
// All v6 types/funcs now live in `core/state` (parent package). This package
// remains only as a thin re-export shim to keep external callsites compiling
// during Phase 2/3/4 — they'll be updated in Phase 4 to import `core/state`
// directly, at which point this package is deleted (Phase 6).
//
// DO NOT add new symbols here. All new work goes in `core/state`.
package v6

import (
	"encoding/json"

	v5 "singbox-launcher/core/state/v5"
	state "singbox-launcher/core/state"
)

// ── Schema constants ──────────────────────────────────────────────

const SchemaVersion = state.SchemaVersionV6
const SchemaName = state.SchemaName

// ── Types ────────────────────────────────────────────────────────

type (
	MetaSection = state.MetaSection

	Rule       = state.Rule
	RuleKind   = state.RuleKind
	PresetBody = state.PresetBody
	InlineBody = state.InlineBody
	SrsBody    = state.SrsBody

	DNSOptions    = state.DNSOptions
	DNSServer     = state.DNSServer
	DNSServerKind = state.DNSServerKind
	DNSRule       = state.DNSRule
	DNSRuleKind   = state.DNSRuleKind

	PresetLite = state.PresetLite
)

// State — disk shape v6 (preserved for callsites which construct it ad-hoc
// for resolver/build pipeline pass-through). In Phase 4 callsites switch
// to `state.State` directly (which has a superset of fields).
type State struct {
	Meta        MetaSection           `json:"meta"`
	Connections v5.ConnectionsSection `json:"connections"`
	Rules       []Rule                `json:"rules"`
	Vars        []v5.SettingVar       `json:"vars,omitempty"`
	DNSOptions  DNSOptions            `json:"dns_options"`
}

// ── Constants ────────────────────────────────────────────────────

const (
	RuleKindPreset = state.RuleKindPreset
	RuleKindInline = state.RuleKindInline
	RuleKindSrs    = state.RuleKindSrs

	DNSServerKindTemplate = state.DNSServerKindTemplate
	DNSServerKindPreset   = state.DNSServerKindPreset
	DNSServerKindUser     = state.DNSServerKindUser

	DNSRuleKindPreset = state.DNSRuleKindPreset
	DNSRuleKindUser   = state.DNSRuleKindUser
)

// ── Functions ────────────────────────────────────────────────────

// SyncDNSOptionsWithActivePresets — re-export from state.
func SyncDNSOptionsWithActivePresets(
	rules []Rule,
	dns *DNSOptions,
	presetByID map[string]PresetLite,
) {
	state.SyncDNSOptionsWithActivePresets(rules, dns, presetByID)
}

// PresetIDFromServerRef / LocalTagFromServerRef — re-export from state.
func PresetIDFromServerRef(ref string) string { return state.PresetIDFromServerRef(ref) }
func LocalTagFromServerRef(ref string) string { return state.LocalTagFromServerRef(ref) }

// IsV6 — schema detection by meta.version.
func IsV6(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Meta.Version == SchemaVersion
}
