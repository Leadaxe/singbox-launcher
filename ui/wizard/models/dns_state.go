package models

import "encoding/json"

// PersistedDNSState is the dns_options object in state.json (not sing-box config.dns).
// See docs/WIZARD_STATE.md, SUB_SPEC_DNS_TAB_VARS (032): в новых снимках — только **servers** и **rules**;
// поля Final/Strategy/… остаются в структуре для чтения старых файлов и одноразовой миграции в **state.vars** (dns_*).
//
// Each element of Servers may include wizard-only keys: "description" (string), "enabled" (bool, default true).
// DNS rules: JSON array **rules** (same as sing-box dns.rules / wizard_template dns_options.rules).
type PersistedDNSState struct {
	Servers          []json.RawMessage `json:"servers"`
	Rules            []json.RawMessage `json:"rules,omitempty"`
	Final            string            `json:"final,omitempty"`
	Strategy         string            `json:"strategy,omitempty"`
	IndependentCache *bool             `json:"independent_cache,omitempty"`
	DefaultDomainResolver string       `json:"default_domain_resolver,omitempty"`
	ResolverUnset         bool         `json:"default_domain_resolver_unset,omitempty"`
}
