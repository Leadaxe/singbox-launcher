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
	"strings"

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

	// legacyRaw — сырой объект `sections` СТАРОЙ формы (волны 1–2), если файл
	// приехал с ней. Перевод отложен: `enabled` и `order_num` правил живут в
	// записи `kind=node` внутри `rules[]` состояния, а на уровне одного узла
	// её не видно. Доперевод делает MigrateLegacyNodeSections после разбора
	// всего файла; поле не сериализуется и в памяти не живёт дольше загрузки.
	legacyRaw json.RawMessage
}

// UnmarshalJSON — чтение обеих форм (SPEC 121 §10.3).
//
// Старая форма распознаётся по ключам и откладывается целиком: перевести её
// здесь нельзя, не увидев `rules[]` состояния.
func (ns *NodeSections) UnmarshalJSON(data []byte) error {
	if LegacyNodeSectionsShape(data) {
		ns.legacyRaw = append(json.RawMessage(nil), data...)
		return nil
	}
	type plain NodeSections
	var tmp plain
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	ns.Rules = tmp.Rules
	ns.DNS = tmp.DNS
	return nil
}

// HasLegacyShape — набор ещё не переведён из старой формы.
func (ns *NodeSections) HasLegacyShape() bool {
	return ns != nil && len(ns.legacyRaw) > 0
}

// NodeSectionsDNS — DNS-половина секций узла.
type NodeSectionsDNS struct {
	Servers []DNSServer `json:"servers,omitempty"`
	Rules   []DNSRule   `json:"rules,omitempty"`
}

// IsEmpty — набор не несёт ни одной записи.
//
// Непереведённая старая форма пустой НЕ считается: иначе нормализация чтения
// снесла бы её раньше, чем до неё дошёл конвертер.
func (ns *NodeSections) IsEmpty() bool {
	if ns == nil {
		return true
	}
	if len(ns.Rules) > 0 || len(ns.legacyRaw) > 0 {
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

// cloneRule — копия записи правила с отдельным телом.
func cloneRule(r Rule) Rule {
	out := r
	if r.OrderNum != nil {
		v := *r.OrderNum
		out.OrderNum = &v
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
	if ns == nil || len(ns.legacyRaw) > 0 {
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
// Старая форма (волны 1–2) читается тем же конвертером, что у state.json:
// пользователь мог скопировать секции из прежней версии.
//
// Виды записей проверяются здесь, а не молча отбрасываются: в тексте, который
// пишет человек, `kind: preset` — опечатка, и сказать о ней надо вслух.
func ReadNodeSections(raw json.RawMessage) (*NodeSections, error) {
	if LegacyNodeSectionsShape(raw) {
		converted, ok := ConvertLegacyNodeSections(raw, "", nil, nil)
		if !ok {
			return nil, fmt.Errorf("sections: cannot be read")
		}
		return converted, nil
	}
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

// ── Чтение старого формата (SPEC 121 §10.3) ────────────────────────

// legacyNodeSections — форма волн 1–2: сырые фрагменты sing-box.
type legacyNodeSections struct {
	DNSServers []json.RawMessage `json:"dns_servers"`
	DNSRules   []json.RawMessage `json:"dns_rules"`
	Rules      []json.RawMessage `json:"rules"`
	// RuleNum — только у бэкапа старой формы: позиция единственного якоря.
	RuleNum *float64 `json:"rule_num"`
}

// LegacyNodeSectionsShape сообщает, написан ли объект `sections` в СТАРОЙ
// форме (волны 1–2): сырые фрагменты sing-box вместо записей состояния.
//
// Признак: ключи `dns_servers`/`dns_rules` (новая форма их не знает вовсе) или
// `rules` как массив объектов БЕЗ `kind` — у записи состояния он обязателен.
func LegacyNodeSectionsShape(raw json.RawMessage) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	if _, ok := probe["dns_servers"]; ok {
		return true
	}
	if _, ok := probe["dns_rules"]; ok {
		return true
	}
	if _, ok := probe["rule_num"]; ok {
		return true
	}
	rulesRaw, ok := probe["rules"]
	if !ok {
		return false
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(rulesRaw, &list); err != nil {
		return false
	}
	for _, item := range list {
		if _, hasKind := item["kind"]; !hasKind {
			return true
		}
	}
	return false
}

// ConvertLegacyNodeSections переводит старую форму в новую (SPEC 121 §10.3).
//
// Перевод обязан сохранить эмитируемый конфиг байт-в-байт по смыслу, поэтому
// он повторяет то, что делала неявная префиксация волн 1–2:
//
//   - локальный тег DNS-сервера `X` → `@{self}:X` (раньше сборка ставила
//     `<финальный тег>:X`);
//   - `server: X` в DNS-правиле, равный локальному тегу сервера этой же
//     секции, → `@{self}:X`; чужое имя не трогается;
//   - sing-box-правило → запись `inline`: `match` = все поля, кроме
//     `outbound`/`action`, `outbound` = было (или `@self`, если не было),
//     `name` = `@{self} rule N`;
//   - `enabled`/`order_num` правил берутся из записи `kind=node` этого узла
//     (anchorEnabled/anchorNum); их нет → `true` / NodeRuleDefaultNum.
//
// Правило с `rule_set` в старой форме сборка отбрасывала — перевод делает то
// же и называет это в WarnLog.
func ConvertLegacyNodeSections(raw json.RawMessage, nodeTag string, anchorEnabled *bool, anchorNum *int) (*NodeSections, bool) {
	var legacy legacyNodeSections
	if err := json.Unmarshal(raw, &legacy); err != nil {
		debuglog.WarnLog("node sections: node %q carries sections in the old shape that cannot be read (%v) — sections dropped", nodeTag, err)
		return nil, false
	}
	return convertLegacyNodeSections(&legacy, nodeTag, anchorEnabled, anchorNum), true
}

func convertLegacyNodeSections(legacy *legacyNodeSections, nodeTag string, anchorEnabled *bool, anchorNum *int) *NodeSections {
	if legacy == nil {
		return nil
	}
	out := &NodeSections{}

	enabled := true
	if anchorEnabled != nil {
		enabled = *anchorEnabled
	}
	num := NodeRuleDefaultNum
	if anchorNum != nil {
		num = *anchorNum
	} else if legacy.RuleNum != nil {
		num = int(*legacy.RuleNum)
	}

	// Локальные теги серверов собираются ДО префиксации: по ним DNS-правило
	// узнаёт «свой» сервер — ровно как это делала сборка волн 1–2.
	localServerTags := make(map[string]bool, len(legacy.DNSServers))
	var servers []DNSServer
	for i, rawSrv := range legacy.DNSServers {
		body, ok := legacyFragmentBody(rawSrv, nodeTag, "dns_servers", i)
		if !ok {
			continue
		}
		tag, _ := body["tag"].(string)
		if tag != "" {
			localServerTags[tag] = true
			tag = SelfPlaceholderBraced + ":" + tag
		}
		// Тег живёт в поле Tag записи, а не в теле: сериализатор DNSServer
		// пишет его на верхнем уровне и предпочитает поле карте.
		delete(body, "tag")
		servers = append(servers, DNSServer{
			Kind:    DNSServerKindUser,
			Tag:     tag,
			Enabled: true,
			Body:    body,
		})
	}

	var dnsRules []DNSRule
	for i, rawRule := range legacy.DNSRules {
		body, ok := legacyFragmentBody(rawRule, nodeTag, "dns_rules", i)
		if !ok {
			continue
		}
		if srv, _ := body["server"].(string); srv != "" && localServerTags[srv] {
			body["server"] = SelfPlaceholderBraced + ":" + srv
		}
		dnsRules = append(dnsRules, DNSRule{Kind: DNSRuleKindUser, Enabled: true, Body: body})
	}
	out.SetDNS(servers, dnsRules)

	for i, rawRule := range legacy.Rules {
		body, ok := legacyFragmentBody(rawRule, nodeTag, "rules", i)
		if !ok {
			continue
		}
		if _, hasRuleSet := body["rule_set"]; hasRuleSet {
			debuglog.WarnLog("node sections: node %q rules[%d] references a rule set — node sections neither declare nor reference rule sets; entry dropped", nodeTag, i)
			continue
		}
		outbound, _ := body["outbound"].(string)
		_, hasAction := body["action"]
		if outbound == "" && !hasAction {
			outbound = SelfPlaceholder
		}
		match := make(map[string]interface{}, len(body))
		for k, v := range body {
			switch k {
			case "outbound", "action":
				continue
			}
			match[k] = v
		}
		inline := InlineBody{
			Name:     fmt.Sprintf("%s rule %d", SelfPlaceholderBraced, i+1),
			Match:    match,
			Outbound: outbound,
		}
		encoded, err := json.Marshal(inline)
		if err != nil {
			debuglog.WarnLog("node sections: node %q rules[%d] cannot be converted (%v) — entry dropped", nodeTag, i, err)
			continue
		}
		n := num
		out.Rules = append(out.Rules, Rule{
			Kind:     RuleKindInline,
			Enabled:  enabled,
			OrderNum: &n,
			Body:     encoded,
		})
		// Соседние правила одного узла встают подряд: у якоря волн 1–2 был
		// один номер на весь набор, а здесь у каждой записи свой.
		num++
	}

	if out.IsEmpty() {
		return nil
	}
	return out
}

// MigrateLegacyNodeSections доводит перевод старой формы до конца на уровне
// всего состояния (SPEC 121 §10.3).
//
// Здесь и только здесь видно обе половины: секции лежат у узла, а `enabled` и
// `order_num` для них — в записи `kind=node` внутри `rules[]`. Запись после
// перевода удаляется: вида `node` больше нет, и оставить её значило бы
// уронить загрузку на неизвестном виде.
//
// Идемпотентна: состояние без старой формы и без записей `kind=node` не
// меняется вовсе.
func MigrateLegacyNodeSections(s *State) {
	if s == nil {
		return
	}
	anchors, anchorSeen := legacyNodeAnchors(s.Rules)

	visit := func(n *Node, link NodeLink) {
		if n == nil || !n.Sections.HasLegacyShape() {
			return
		}
		raw := n.Sections.legacyRaw
		var enabled *bool
		var num *int
		if a, ok := anchors[link]; ok {
			enabled, num = a.enabled, a.num
		}
		converted, ok := ConvertLegacyNodeSections(raw, link.Tag, enabled, num)
		if !ok {
			n.Sections = nil
			return
		}
		n.Sections = converted
		debuglog.InfoLog("node sections: node %q converted from the pre-%s storage shape", link.Tag, "0.13 launcher")
	}

	for i := range s.Sources {
		src := &s.Sources[i]
		if src.Kind == SourceKindServer {
			visit(&src.Node, NodeLink{Tag: src.NodeTagOrLabel()})
		}
		for j := range src.Nodes {
			visit(&src.Nodes[j], NodeLink{FolderID: src.ID, Tag: src.Nodes[j].Tag})
		}
	}

	// Записи упразднённого вида снимаются независимо от того, нашлись ли им
	// секции: вид `node` больше не существует, и DecodeBody на нём падает.
	if anchorSeen {
		s.Rules = dropLegacyNodeAnchors(s.Rules)
	}
}

// legacyNodeAnchorState — то, что якорь волн 1–2 отдаёт переводу.
type legacyNodeAnchorState struct {
	enabled *bool
	num     *int
}

// legacyNodeAnchors собирает записи упразднённого вида `node` по ссылке на
// узел. Второй возврат — встретилась ли хоть одна такая запись.
func legacyNodeAnchors(rules []Rule) (map[NodeLink]legacyNodeAnchorState, bool) {
	out := map[NodeLink]legacyNodeAnchorState{}
	seen := false
	for _, r := range rules {
		if r.Kind != legacyRuleKindNode {
			continue
		}
		seen = true
		var body struct {
			FolderID string `json:"folder_id"`
			Tag      string `json:"tag"`
		}
		if err := json.Unmarshal(r.Body, &body); err != nil || body.Tag == "" {
			continue
		}
		enabled := r.Enabled
		st := legacyNodeAnchorState{enabled: &enabled}
		if r.OrderNum != nil {
			n := *r.OrderNum
			st.num = &n
		}
		out[NodeLink{FolderID: body.FolderID, Tag: body.Tag}] = st
	}
	return out, seen
}

// dropLegacyNodeAnchors убирает записи упразднённого вида из списка правил.
func dropLegacyNodeAnchors(rules []Rule) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.Kind == legacyRuleKindNode {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// legacyRuleKindNode — упразднённый вид правила волн 1–2 (SPEC 121 §10.1).
// Живёт одной константой только ради чтения старых файлов.
const legacyRuleKindNode RuleKind = "node"

// legacyFragmentBody разбирает один сырой фрагмент старой формы в карту.
func legacyFragmentBody(raw json.RawMessage, nodeTag, section string, idx int) (map[string]interface{}, bool) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, false
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		debuglog.WarnLog("node sections: node %q %s[%d] is not a JSON object — entry dropped", nodeTag, section, idx)
		return nil, false
	}
	return body, true
}
