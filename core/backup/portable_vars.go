package backup

// Переносимые имена переменных шаблона (SPEC 103, фаза 4).
//
// Список — зеркало contract/registry/vars.json (portable=true) и сверяется
// с ним тестом TestPortableVarsMatchRegistry: разъехавшийся список означает,
// что бэкап либо теряет настройку, либо тащит на чужую машину значение,
// которое там значит другое (пути, интерфейсы, платформенные флаги).

// portableVars — множество имён, переносимых между приложениями.
var portableVars = map[string]struct{}{
	"auto_detect_interface":       {},
	"dns_default_domain_resolver": {},
	"dns_final":                   {},
	"dns_strategy":                {},
	"ipv6_enabled":                {},
	"log_level":                   {},
	"resolve_strategy":            {},
	"tls_fragment":                {},
	"tls_fragment_fallback_delay": {},
	"tls_mixed_case_sni":          {},
	"tls_record_fragment":         {},
	// SPEC 108-follow-up: `tun_address` стал переносимым вместе с закрытием
	// разрыва N7 — прежде это был text_list, куда IPv4 и IPv6 клались
	// строками, и перенести его на мобильный (там text, только IPv4) было
	// нельзя. Теперь по одному адресу на семейство на обеих сторонах.
	"tun_address":       {},
	"tun_address6":      {},
	"tun_mtu":           {},
	"tun_stack":         {},
	"urltest_interval":  {},
	"urltest_tolerance": {},
	"urltest_url":       {},
}

// IsPortableVar сообщает, переносится ли переменная в бэкап.
func IsPortableVar(name string) bool {
	_, ok := portableVars[name]
	return ok
}
