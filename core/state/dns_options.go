// File dns_options.go — DNS-секция состояния (SPEC 056-R-N, форма v8 по SPEC 127).
//
// Вид записи задаёт kind: template/preset/user у серверов, preset/user у правил.
//
// JSON layout (state v8 — ключ корня `dns`):
//
//	"dns": {
//	  "strategy": "...",
//	  "final": "...",
//	  "default_domain_resolver": "...",
//	  "servers": [
//	    {"kind":"template", "tag":"cloudflare_udp", "enabled":true},
//	    {"kind":"preset",   "ref":"russian:yandex_udp", "enabled":true},
//	    {"kind":"user",     "tag":"my-pihole", "enabled":true,
//	     "body":{"type":"udp","server":"192.168.1.5"}}
//	  ],
//	  "rules": [
//	    {"kind":"preset", "ref":"russian", "enabled":true},
//	    {"kind":"user",   "enabled":true,
//	     "body":{"rule_set":"ru-domains","server":"yandex_doh"}}
//	  ]
//	}
//
// До v8 тело разливалось ПЛОСКО рядом с kind/tag/enabled, и ради этого были
// написаны четыре кастомных Marshal/Unmarshal. В v8 их нет: сериализация —
// обычные struct-теги, плоскую форму читает только миграция v7→v8
// (migration_v7_to_v8.go). Тега внутри `body` быть не должно — он метаданные;
// при эмиссии конфига tag дописывается в тело из поля (resolve_dns.go).
//
// Инварианты:
//  1. Memory == disk — никакого runtime materialization для preset entries.
//     SyncDNSOptionsWithActivePresets синхронизирует list с активным набором
//     preset-ref'ов в state.Rules[] (вызывается на load + на toggle).
//  2. kind=template/preset тело резолвится из template на build/render — на
//     диске только {kind, tag|ref, enabled}.
//  3. kind=user — полное тело sing-box в `body`.
//
// См. SPECS/056-R-N-DNS_SCHEMA_REDESIGN/SPEC.md, SPECS/127-F-N-ONE_NAMESPACE_V8/.
package state

// DNSServerKind — дискриминатор entry в dns_options.servers[].
type DNSServerKind string

const (
	// DNSServerKindTemplate — тонкая ссылка на template.dns_options.servers[tag].
	// Юзер может toggle Enabled; тело берётся из template на build.
	DNSServerKindTemplate DNSServerKind = "template"

	// DNSServerKindPreset — тонкая ссылка на template.presets[X].dns_servers[Y].
	// Ref в формате "<preset_id>:<local_tag>". Авто-add/remove через
	// SyncDNSOptionsWithActivePresets при toggle preset'а в Rules tab.
	DNSServerKindPreset DNSServerKind = "preset"

	// DNSServerKindUser — genuinely user-defined DNS-сервер, нет template-аналога.
	// Полное тело сериализуется flat'ом (type, server, server_port, tls, ...).
	DNSServerKindUser DNSServerKind = "user"
)

// DNSRuleKind — дискриминатор entry в dns_options.rules[].
type DNSRuleKind string

const (
	// DNSRuleKindPreset — тонкая ссылка на template.presets[X].dns_rule.
	// Ref в формате "<preset_id>" (один dns_rule на preset максимум).
	DNSRuleKindPreset DNSRuleKind = "preset"

	// DNSRuleKindUser — user-defined DNS rule. Полное тело (rule_set, server,
	// domain_*, ip_cidr, ...) сериализуется flat'ом.
	DNSRuleKindUser DNSRuleKind = "user"
)

// DNSServer — запись в state.dns.servers[] и в sections.dns.servers[].
//
// Сериализация — обычные struct-теги (порядок полей = порядок ключей файла):
// метаданные снаружи, тело sing-box в `body` (только kind=user).
type DNSServer struct {
	Kind DNSServerKind `json:"kind"`

	// Tag — для kind=template (template.dns_options.servers[tag]) и kind=user
	// (display tag в финальном config.dns.servers[].tag). Пуст для kind=preset.
	Tag string `json:"tag,omitempty"`

	// Ref — только для kind=preset, формат "<preset_id>:<local_tag>".
	// Пуст для остальных kind'ов.
	Ref string `json:"ref,omitempty"`

	// Enabled — toggle. Build pipeline пропускает entry если false.
	Enabled bool `json:"enabled"`

	// Body — для kind=user полное тело sing-box DNS-сервера (type, server,
	// server_port, tls, detour, ...). nil/пуст для kind=template/preset.
	//
	// **Не содержит** kind/ref/enabled/tag — все они метаданные записи. Тег
	// дописывается в тело при эмиссии конфига (resolve_dns.go).
	Body map[string]interface{} `json:"body,omitempty"`
}

// DNSRule — запись в state.dns.rules[] и в sections.dns.rules[].
type DNSRule struct {
	Kind DNSRuleKind `json:"kind"`

	// ID — необязательные метаданные другой стороны; лаунчер провозит.
	ID string `json:"id,omitempty"`

	// Ref — только для kind=preset, формат "<preset_id>".
	Ref string `json:"ref,omitempty"`

	// Name — необязательное имя правила (ONE_NAMESPACE §1): лаунчер его не
	// заполняет, но и не теряет.
	Name string `json:"name,omitempty"`

	// Enabled — toggle.
	Enabled bool `json:"enabled"`

	// Body — для kind=user полное тело sing-box dns rule (rule_set, server,
	// domain_*, ip_cidr, port, network, ...). nil/пуст для kind=preset.
	Body map[string]interface{} `json:"body,omitempty"`
}

// DNSOptions — раздел dns_options в state.json (SPEC 056-R-N).
//
// dns_* scalars (Strategy / Final / DefaultDomainResolver) дублируются
// здесь и в state.vars[]; для совместимости с template-substitute vars
// остаётся source of truth, а DNSOptions-scalars читаются как fallback
// (см. core/config_service.go::dnsConfigForUpdate).
//
// **Важно:** SPEC 056 решил оставить dns_* scalars в state.vars[] как единый
// KV-store. Поля Strategy/Final/DefaultDomainResolver здесь присутствуют для
// in-memory работы build pipeline, но **не сериализуются** если они zero-value
// (omitempty). На диск ходит вариант "хранится в vars[]".
//
// SPEC: IndependentCache УДАЛЕНО — deprecated в sing-box 1.14.0 (кэш всегда
// per-transport). Legacy state.json с этим ключом парсится без ошибок
// (unknown field ignored), новые state'ы поле не пишут.
type DNSOptions struct {
	Strategy              string `json:"strategy,omitempty"`
	Final                 string `json:"final,omitempty"`
	DefaultDomainResolver string `json:"default_domain_resolver,omitempty"`

	Servers []DNSServer `json:"servers,omitempty"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// ── Helpers ────────────────────────────────────────────────────────

// FindServerByTag возвращает индекс template/user-server с tag, или -1.
// (kind=preset идентифицируются через Ref, не Tag — для них используй FindServerByRef.)
func (d *DNSOptions) FindServerByTag(tag string) int {
	if d == nil {
		return -1
	}
	for i, s := range d.Servers {
		if (s.Kind == DNSServerKindTemplate || s.Kind == DNSServerKindUser) && s.Tag == tag {
			return i
		}
	}
	return -1
}

// FindServerByRef возвращает индекс preset-server с ref, или -1.
func (d *DNSOptions) FindServerByRef(ref string) int {
	if d == nil {
		return -1
	}
	for i, s := range d.Servers {
		if s.Kind == DNSServerKindPreset && s.Ref == ref {
			return i
		}
	}
	return -1
}

// FindRuleByRef возвращает индекс preset-rule с ref, или -1.
func (d *DNSOptions) FindRuleByRef(ref string) int {
	if d == nil {
		return -1
	}
	for i, r := range d.Rules {
		if r.Kind == DNSRuleKindPreset && r.Ref == ref {
			return i
		}
	}
	return -1
}

// IsEmpty — true если в DNSOptions нет ни одного поля (scalars + servers + rules).
// Используется sequence'ами omitempty / нормализации.
func (d *DNSOptions) IsEmpty() bool {
	if d == nil {
		return true
	}
	return d.Strategy == "" &&
		d.Final == "" &&
		d.DefaultDomainResolver == "" &&
		len(d.Servers) == 0 &&
		len(d.Rules) == 0
}
