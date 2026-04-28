package models

// PersistedDNSState — снимок вкладки DNS визарда в state.json.
//
// SPEC 052 phase 7: тип переехал в core/state (DNSOptions); здесь
// сохранён только алиас для backward-compat callsite'ов.
//
// См. docs/WIZARD_STATE.md, SUB_SPEC_DNS_TAB_VARS (032): в новых снимках —
// только Servers и Rules; поля Final/Strategy/… остаются в структуре
// для чтения старых файлов и одноразовой миграции в state.vars (dns_*).
//
// Each element of Servers may include wizard-only keys: "description"
// (string), "enabled" (bool, default true). DNS rules: JSON array rules
// (same as sing-box dns.rules / wizard_template dns_options.rules).
//
// Алиас определён в wizard_state_file.go:
//
//	type PersistedDNSState = corestate.DNSOptions
