// File node_sections.go — секции узла в формате состояния (SPEC 121 §10).
//
// # Что это
//
// Узел может нести с собой куски конфига, без которых он бесполезен:
// DNS-сервер, привязанный к нему, DNS-правило на его домены, правила маршрута
// на его подсети. Хранятся они НЕ сырыми фрагментами sing-box, а записями
// самого лаунчера — теми же `Rule` / `DNSServer` / `DNSRule`, что лежат в
// корне состояния:
//
//	"sections": {
//	  "rules": [ {"kind":"inline","enabled":true,"order_num":945,
//	              "body":{"name":"@{self} network",
//	                      "match":{"ip_cidr":["100.64.0.0/10"]},"outbound":"@self"}} ],
//	  "dns": {
//	    "servers": [ {"kind":"user","enabled":true,"tag":"@{self}-dns",
//	                  "type":"tailscale","endpoint":"@self"} ],
//	    "rules":   [ {"kind":"user","enabled":true,
//	                  "domain_suffix":[".ts.net"],"server":"@{self}-dns"} ]
//	  }
//	}
//
// Так у каждой записи появляется своя позиция на оси и свой тумблер, а сборка
// перестаёт «разворачивать» секции: она просто дописывает эти записи к общим
// спискам и дальше работает существующий конвейер (SPEC 121 §10.2).
//
// # Плейсхолдер
//
// `@self` целой строкой и `@{self}` внутри строки — единственная переменная,
// которую знает секция. Подстановка живёт рядом (selfvar.go, SubstituteSelf) и
// происходит на сборке: финальный тег узла до эмиссии неизвестен.
//
// # Виды записей
//
// Правила — только `inline` и `srs`: вид `preset` у узла означал бы ссылку на
// шаблон, которого на чужой машине может не быть. DNS-записи — только `user`:
// `template`/`preset` — тонкие ссылки, и у узла им ссылаться не на что.
// Запись чужого вида отбрасывается при чтении с WarnLog.
package state

import (
	"encoding/json"
	"fmt"

	"singbox-launcher/internal/debuglog"
)

// SelfPlaceholder / SelfPlaceholderBraced — плейсхолдер «финальный тег этого
// узла». Первая форма занимает строку целиком, вторая встраивается в неё
// (`@{self}-dns`, `@{self} network`).
const (
	SelfPlaceholder       = "@self"
	SelfPlaceholderBraced = "@{self}"
)

// NodeSections — фрагмент состояния, который узел носит с собой.
type NodeSections struct {
	// Rules — записи route-правил (виды inline | srs) со своими enabled и
	// order_num. Каждая встаёт на общую ось отдельной строкой.
	Rules []Rule `json:"rules,omitempty"`
	// DNS — DNS-серверы и DNS-правила узла (вид user). nil = их нет.
	DNS *NodeSectionsDNS `json:"dns,omitempty"`
}

// NodeSectionsDNS — DNS-половина секций узла.
type NodeSectionsDNS struct {
	Servers []DNSServer `json:"servers,omitempty"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// IsEmpty — набор не несёт ни одной записи.
func (ns *NodeSections) IsEmpty() bool {
	if ns == nil {
		return true
	}
	if len(ns.Rules) > 0 {
		return false
	}
	return ns.DNS.IsEmpty()
}

// IsEmpty — DNS-половина пуста.
func (d *NodeSectionsDNS) IsEmpty() bool {
	return d == nil || (len(d.Servers) == 0 && len(d.Rules) == 0)
}

// HasRules — у набора есть правила маршрута.
func (ns *NodeSections) HasRules() bool {
	return ns != nil && len(ns.Rules) > 0
}

// DNSServers / DNSRules — удобный доступ к DNS-половине без проверки на nil.
func (ns *NodeSections) DNSServers() []DNSServer {
	if ns == nil || ns.DNS == nil {
		return nil
	}
	return ns.DNS.Servers
}

func (ns *NodeSections) DNSRules() []DNSRule {
	if ns == nil || ns.DNS == nil {
		return nil
	}
	return ns.DNS.Rules
}

// SetDNS кладёт списки в DNS-половину, нормализуя пустое в nil.
func (ns *NodeSections) SetDNS(servers []DNSServer, rules []DNSRule) {
	if ns == nil {
		return
	}
	if len(servers) == 0 && len(rules) == 0 {
		ns.DNS = nil
		return
	}
	ns.DNS = &NodeSectionsDNS{Servers: servers, Rules: rules}
}

// Clone — глубокая копия набора (тела правил и карт DNS не разделяются с
// оригиналом: редактор владеет своей копией, SPEC 117).
func (ns *NodeSections) Clone() *NodeSections {
	if ns == nil {
		return nil
	}
	out := &NodeSections{}
	for _, r := range ns.Rules {
		out.Rules = append(out.Rules, cloneRule(r))
	}
	if ns.DNS != nil {
		dns := &NodeSectionsDNS{}
		for _, s := range ns.DNS.Servers {
			dns.Servers = append(dns.Servers, DNSServer{
				Kind: s.Kind, Tag: s.Tag, Ref: s.Ref, Enabled: s.Enabled,
				Body: cloneJSONMap(s.Body),
			})
		}
		for _, r := range ns.DNS.Rules {
			dns.Rules = append(dns.Rules, DNSRule{
				Kind: r.Kind, Ref: r.Ref, Enabled: r.Enabled, Body: cloneJSONMap(r.Body),
			})
		}
		out.DNS = dns
	}
	return out
}

// CloneRule / CloneDNSServer / CloneDNSRule — копии записей для тех, кто
// уносит их за пределы состояния (экспорт бэкапа 1.0 пишет в файл СНИМОК
// момента, и общий с состоянием указатель сделал бы файл окном в живые
// данные).
//
// Публичные обёртки, а не переименование внутренних: внутри пакета копии
// зовутся десятком мест, и смена имени была бы шумом без смысла.
func CloneRule(r Rule) Rule { return cloneRule(r) }

func CloneDNSServer(s DNSServer) DNSServer {
	out := s
	out.Body = cloneJSONMap(s.Body)
	return out
}

func CloneDNSRule(r DNSRule) DNSRule {
	out := r
	out.Body = cloneJSONMap(r.Body)
	return out
}

// cloneRule — копия записи правила с отдельным телом.
func cloneRule(r Rule) Rule {
	out := r
	if r.Num != nil {
		v := *r.Num
		out.Num = &v
	}
	if len(r.Refs) > 0 {
		out.Refs = append([]string(nil), r.Refs...)
	}
	if len(r.Vars) > 0 {
		vars := make(map[string]string, len(r.Vars))
		for k, v := range r.Vars {
			vars[k] = v
		}
		out.Vars = vars
	}
	if len(r.Body) > 0 {
		out.Body = append(json.RawMessage(nil), r.Body...)
	}
	return out
}

// cloneJSONMap — поверхностно-глубокая копия карты тела (значения приезжают из
// JSON и переиспользуются только на чтение; копируется верхний уровень).
func cloneJSONMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// NormalizeNodeSections приводит поле к канону: чужие виды записей выбрасываются,
// пустой набор — в nil, и у любого вида узла, кроме server, секций не остаётся
// вовсе.
//
// Зовётся и на чтении состояния, и перед записью: пустой объект `{}` в
// state.json был бы третьим состоянием поля (nil / пусто / есть), а разбирать
// его пришлось бы каждому потребителю.
func (n *Node) NormalizeNodeSections() {
	if n == nil {
		return
	}
	if n.Kind != SourceKindServer {
		n.Sections = nil
		return
	}
	if n.Sections != nil {
		n.Sections.dropForeignKinds(n.Tag)
	}
	if n.Sections.IsEmpty() {
		n.Sections = nil
	}
}

// dropForeignKinds снимает записи видов, которых у секции быть не может.
func (ns *NodeSections) dropForeignKinds(nodeTag string) {
	if ns == nil {
		return
	}
	rules := ns.Rules[:0]
	for _, r := range ns.Rules {
		switch r.Kind {
		case RuleKindInline, RuleKindSrs:
			rules = append(rules, r)
		default:
			debuglog.WarnLog("node sections: node %q carries a %q rule — node sections know only inline and srs; entry dropped", nodeTag, string(r.Kind))
		}
	}
	if len(rules) == 0 {
		ns.Rules = nil
	} else {
		ns.Rules = rules
	}
	if ns.DNS == nil {
		return
	}
	servers := ns.DNS.Servers[:0]
	for _, s := range ns.DNS.Servers {
		if s.Kind == DNSServerKindUser {
			servers = append(servers, s)
			continue
		}
		debuglog.WarnLog("node sections: node %q carries a %q DNS server — node sections know only user entries; entry dropped", nodeTag, string(s.Kind))
	}
	dnsRules := ns.DNS.Rules[:0]
	for _, r := range ns.DNS.Rules {
		if r.Kind == DNSRuleKindUser {
			dnsRules = append(dnsRules, r)
			continue
		}
		debuglog.WarnLog("node sections: node %q carries a %q DNS rule — node sections know only user entries; entry dropped", nodeTag, string(r.Kind))
	}
	if len(servers) == 0 && len(dnsRules) == 0 {
		ns.DNS = nil
		return
	}
	ns.DNS.Servers = servers
	ns.DNS.Rules = dnsRules
}

// normalizeSectionsOfSources прогоняет NormalizeNodeSections по всему дереву
// источников: корневые узлы и члены контейнеров.
func normalizeSectionsOfSources(sources []Source) {
	for i := range sources {
		sources[i].Node.NormalizeNodeSections()
		for j := range sources[i].Nodes {
			sources[i].Nodes[j].NormalizeNodeSections()
		}
	}
}

// ReadNodeSections разбирает объект `sections`, написанный человеком:
// вкладка JSON узла принимает хранимую форму обратно (SPEC 121 §10.4).
//
// Виды записей проверяются здесь, а не молча отбрасываются: в тексте, который
// пишет человек, `kind: preset` — опечатка, и сказать о ней надо вслух.
func ReadNodeSections(raw json.RawMessage) (*NodeSections, error) {
	var out NodeSections
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("sections: %w", err)
	}
	for i, r := range out.Rules {
		switch r.Kind {
		case RuleKindInline, RuleKindSrs:
			if _, err := r.DecodeBody(); err != nil {
				return nil, fmt.Errorf("sections.rules[%d]: %w", i, err)
			}
		default:
			return nil, fmt.Errorf(
				"sections.rules[%d] has kind %q; node sections know only %q and %q",
				i, string(r.Kind), string(RuleKindInline), string(RuleKindSrs))
		}
	}
	for i, srv := range out.DNSServers() {
		if srv.Kind != DNSServerKindUser {
			return nil, fmt.Errorf(
				"sections.dns.servers[%d] has kind %q; node sections know only %q",
				i, string(srv.Kind), string(DNSServerKindUser))
		}
	}
	for i, r := range out.DNSRules() {
		if r.Kind != DNSRuleKindUser {
			return nil, fmt.Errorf(
				"sections.dns.rules[%d] has kind %q; node sections know only %q",
				i, string(r.Kind), string(DNSRuleKindUser))
		}
	}
	if out.IsEmpty() {
		return nil, nil
	}
	return &out, nil
}
