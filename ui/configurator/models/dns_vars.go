package models

// Имена переменных шаблона (vars) для скаляров вкладки DNS — state.json → vars, маркеры @ в config.
// См. SPECS/032-F-C-WIZARD_SETTINGS_TAB/SUB_SPEC_DNS_TAB_VARS.md
const (
	VarDNSStrategy              = "dns_strategy"
	VarDNSIndependentCache      = "dns_independent_cache"
	VarDNSDefaultDomainResolver = "dns_default_domain_resolver"
	VarDNSFinal                 = "dns_final"
)
