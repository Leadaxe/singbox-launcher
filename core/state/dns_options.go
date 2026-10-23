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
//     диске у preset только {kind, ref, enabled}, у template ещё значения
//     переменных сервера `vars` (SPEC 129): {kind, tag, enabled, vars?}.
//     В `vars` — только объявленные сервером имена и только значения,
//     отличные от умолчания шаблона (нормы Н2/Н4, record_vars.go).
//  3. kind=user — полное тело sing-box в `body`.
//
// См. SPECS/056-R-N-DNS_SCHEMA_REDESIGN/SPEC.md, SPECS/127-F-N-ONE_NAMESPACE_V8/.
package state

import "strings"

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

// DNSServer — запись в state.dns.servers[].
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

	// Vars — значения переменных шаблонного сервера по ЛОКАЛЬНЫМ именам его
	// объявления (`outbound`, `dns_ip`), только kind=template (SPEC 129).
	// Отсутствие ключа = «следовать шаблону»: умолчание не пишется (Н4),
	// необъявленное имя снимается (Н2), значение хранится подрезанным (Н3).
	// До SPEC 129 значения жили корневыми `vars.dns_<tag>_<var>`; их
	// переносит сюда ApplyRecordVars.
	Vars map[string]string `json:"vars,omitempty"`

	// Body — для kind=user полное тело sing-box DNS-сервера (type, server,
	// server_port, tls, detour, ...). nil/пуст для kind=template/preset.
	//
	// **Не содержит** kind/ref/enabled/tag — все они метаданные записи. Тег
	// дописывается в тело при эмиссии конфига (resolve_dns.go).
	Body map[string]interface{} `json:"body,omitempty"`
}

// DNSRule — запись в state.dns.rules[].
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

	// TemplateCatalog — теги каталога (`dns_options.servers` шаблона) в
	// порядке шаблона (SPEC 173, контракт 1.1.135, D-139). Одно поле на две
	// формы с одним смыслом «какой каталог этот список уже видел»:
	//   - в состоянии — каталог, которым список засеян. nil — посева ещё не
	//     было (состояние до SPEC 173 или без шаблона): записи kind=template
	//     дописывает SeedDNSTemplateCatalog. Тег каталога без записи после
	//     посева — сервер удалён пользователем, а не «умолчание шаблона»;
	//   - в бэкапе (`dns.template_catalog`) — каталог шаблона источника на
	//     момент экспорта; по нему импорт снимает удалённое источником
	//     (BACKUP.md §9 п. 5).
	TemplateCatalog []string `json:"template_catalog,omitempty"`

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

// FindTemplateServer возвращает индекс записи kind=template с tag, или -1.
func (d *DNSOptions) FindTemplateServer(tag string) int {
	if d == nil {
		return -1
	}
	for i, s := range d.Servers {
		if s.Kind == DNSServerKindTemplate && s.Tag == tag {
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

// ── Удерживающие ссылки (SPEC 173) ────────────────────────────────

// Порядок значимости удерживающих ссылок — порядок словаря `used_by`
// контракта (BACKUP.md §9 п. 5): при нескольких ссылках называется первая.
const (
	dnsHoldFinal = iota
	dnsHoldResolver
	dnsHoldDNSRule
	dnsHoldRule
	dnsHoldGroup
	dnsHoldServer
	dnsHoldPreset
	dnsHoldRequired
)

// Имена переменных состояния, в которых живут DNS-скаляры (источник истины;
// поля DNSOptions — запасное значение, core/config_service_context.go).
const (
	dnsFinalVar    = "dns_final"
	dnsResolverVar = "dns_default_domain_resolver"
)

// DNSServerHolders — кто держит каждый тег DNS-сервера: тег → первая
// удерживающая ссылка в форме `used_by` контракта (`final`,
// `default_domain_resolver`, `dns_rule:<server>`, `rule:<name>`,
// `group:<tag>`, `server:<tag>`, `preset:<id>`, `required`). Тега без
// ссылок в карте нет.
//
// Одна функция на вкладку DNS (Delete недоступен, подпись «Used by …») и на
// импорт бэкапа (снятие удалённого источником шаблонного сервера): правило
// «что держит сервер» обязано быть одним, иначе импорт снял бы то, что UI
// удалить не даёт. Перечень закрытый (контракт 1.1.135/1.1.136):
//   - `dns_final` / `dns_default_domain_resolver` (переменные и поля секции);
//   - `server` DNS-правила списка `dns.rules` (правила пресетов — через
//     пресет ниже, правил шаблона у лаунчера нет);
//   - объект `dns` маршрутного правила с непустым `serverTag` — независимо
//     от `enabled`: снятие необратимо, а выключенная опция, включённая
//     потом, указала бы в пустоту;
//   - сервер-группа списка (члены `servers`) и `domain_resolver` другого
//     сервера списка; у шаблонного — по телу шаблона, `@имя` разрешается
//     значением записи или умолчанием (decls.DNSServerLinks);
//   - `server` DNS-правил включённого пресета (decls.PresetDNSRuleServers);
//   - `required: true` у сервера шаблона.
//
// decls nil — шаблона нет: ссылок тела шаблона, пресетов и `required` не
// видно, остаются ссылки самого состояния.
func DNSServerHolders(s *State, decls *RecordVarDecls) map[string]string {
	if s == nil {
		return nil
	}
	best := map[string]int{}
	used := map[string]string{}
	hold := func(tag string, kind int, usedBy string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if cur, ok := best[tag]; ok && cur <= kind {
			return
		}
		best[tag] = kind
		used[tag] = usedBy
	}

	for _, v := range s.Vars {
		switch v.Name {
		case dnsFinalVar:
			hold(v.Value, dnsHoldFinal, "final")
		case dnsResolverVar:
			hold(v.Value, dnsHoldResolver, "default_domain_resolver")
		}
	}
	hold(s.DNS.Final, dnsHoldFinal, "final")
	hold(s.DNS.DefaultDomainResolver, dnsHoldResolver, "default_domain_resolver")

	for _, r := range s.DNS.Rules {
		if r.Kind != DNSRuleKindUser {
			continue
		}
		if server, _ := r.Body["server"].(string); strings.TrimSpace(server) != "" {
			hold(server, dnsHoldDNSRule, "dns_rule:"+strings.TrimSpace(server))
		}
	}

	for _, r := range s.Rules {
		if r.DNS == nil || strings.TrimSpace(r.DNS.ServerTag) == "" {
			continue
		}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			name = "rule"
		}
		hold(r.DNS.ServerTag, dnsHoldRule, "rule:"+name)
	}

	for _, srv := range s.DNS.Servers {
		switch srv.Kind {
		case DNSServerKindUser:
			if t, _ := srv.Body["type"].(string); t == "group" {
				members, _ := srv.Body["servers"].([]interface{})
				for _, m := range members {
					if tag, _ := m.(string); tag != srv.Tag {
						hold(tag, dnsHoldGroup, "group:"+srv.Tag)
					}
				}
			}
			if tag := dnsResolverTarget(srv.Body["domain_resolver"]); tag != srv.Tag {
				hold(tag, dnsHoldServer, "server:"+srv.Tag)
			}
		case DNSServerKindTemplate:
			if decls == nil {
				continue
			}
			links := decls.DNSServerLinks[srv.Tag]
			for _, raw := range links.Group {
				for _, tag := range decls.dnsServerLinkValues(srv, raw) {
					if tag != srv.Tag {
						hold(tag, dnsHoldGroup, "group:"+srv.Tag)
					}
				}
			}
			for _, tag := range decls.dnsServerLinkValues(srv, links.DomainResolver) {
				if tag != srv.Tag {
					hold(tag, dnsHoldServer, "server:"+srv.Tag)
				}
			}
		}
	}

	if decls != nil {
		for _, r := range s.Rules {
			if r.Kind != RuleKindPreset || !r.Enabled || r.Ref == "" {
				continue
			}
			for _, raw := range decls.PresetDNSRuleServers[r.Ref] {
				hold(presetLinkValue(r, decls.Presets[r.Ref], raw), dnsHoldPreset, "preset:"+r.Ref)
			}
		}
		for tag, req := range decls.DNSServerRequired {
			if req {
				hold(tag, dnsHoldRequired, "required")
			}
		}
	}
	return used
}

// dnsResolverTarget — тег из `domain_resolver`: строка или объект {server}.
func dnsResolverTarget(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]interface{}:
		s, _ := t["server"].(string)
		return strings.TrimSpace(s)
	}
	return ""
}

// dnsServerLinkValues — теги, на которые указывает ссылка тела шаблонного
// сервера: литерал как есть, `@имя` — значение записи или умолчание
// объявления (список — построчно).
func (d *RecordVarDecls) dnsServerLinkValues(srv DNSServer, raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "@") {
		return []string{raw}
	}
	name := raw[1:]
	value, ok := srv.Vars[name]
	if !ok {
		decl, declared := d.DNSServerVarDecl(srv.Tag, name)
		if !declared {
			return nil
		}
		value = decl.Default
	}
	return splitLinkList(value)
}

// presetLinkValue — значение `server` DNS-правила пресета: литерал как есть,
// `@имя` — значение правила или умолчание объявления пресета.
func presetLinkValue(r Rule, declared []RecordVarDecl, raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "@") {
		return raw
	}
	name := raw[1:]
	if v, ok := r.Vars[name]; ok {
		return strings.TrimSpace(v)
	}
	for _, decl := range declared {
		if decl.Name == name {
			return decl.Default
		}
	}
	return ""
}

func splitLinkList(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}
