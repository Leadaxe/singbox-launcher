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

	// Маршрут шаблонных DNS-серверов (1.6.0): переменные, которые вложенная
	// запись `dns_options.servers` объявляет себе и которые при загрузке
	// шаблона становятся `dns_<tag>_<var>` (template.NormalizeDNSOptions).
	// Значение — корневое имя: канал запроса (`outbound` → `detour` сервера,
	// Направление или системный тег) или тег DNS-сервера (`dom_resolver` →
	// `domain_resolver`). На другой машине оно значит то же самое.
	//
	// Без них импорт в пустое состояние терял выбор канала: финальный DNS
	// google_udp, настроенный через proxy-out, на новой машине уходил на
	// дефолт шаблона direct-out — DNS мимо VPN.
	//
	// Список по именам, как весь реестр: новая вложенная запись шаблона с
	// переменной маршрута добавляется сюда и в registry/vars.json вместе.
	"dns_google_udp_outbound":       {},
	"dns_google_dot_outbound":       {},
	"dns_cloudflare_dot_outbound":   {},
	"dns_safe_dns_dot_outbound":     {},
	"dns_safe_dns_dot_dom_resolver": {},
}

// IsPortableVar сообщает, переносится ли переменная в бэкап.
func IsPortableVar(name string) bool {
	_, ok := portableVars[name]
	return ok
}
