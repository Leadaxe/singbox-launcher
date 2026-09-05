// File node_sections_convert.go — единственный перевод sing-box-фрагментов в
// хранимую форму секций узла и обратно (SPEC 121 §10.3, §10.4).
//
// # Зачем одна функция
//
// Секции узла приезжают из трёх мест: вкладка JSON окна источника
// (ParseNodeDocument), целый sing-box-конфиг как источник (ExtractNodeSections)
// и конструктор «Add server → Tailscale». Все три отдают одно и то же —
// sing-box-документ, — и все три обязаны получить одинаковые записи состояния.
// Вторая реализация правил перевода разошлась бы с первой на первой же правке
// («эмиттер и парсер ходят парой»), поэтому перевод живёт ЗДЕСЬ, а вызывающие
// только подают ему списки.
//
// # Правила перевода
//
//   - `dns.servers[i]` → DNSServer вида `user`: тег уезжает в поле Tag,
//     остальное тело — в Body как есть (`endpoint`, `accept_default_resolvers`
//     и любое незнакомое поле переживают перевод: типизация потеряла бы их);
//   - `dns.rules[i]` → DNSRule вида `user`, тело целиком;
//   - `route.rules[i]` → Rule вида `inline`: `outbound` из правила
//     (или `@self`, если его не было и нет `action`), `match` — всё
//     остальное, `name` — из `@{self}` и порядкового номера, чтобы строка
//     Rules называлась узлом, а не «unnamed»;
//   - `rule_set` в правиле — отказ: секции наборов правил не объявляют и на
//     них не ссылаются, а висячая ссылка роняет конфиг целиком;
//   - `@var`, кроме `@self`/`@{self}`, — отказ: словаря переменных у узла нет.
package state

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SingboxNodeFragments — вход перевода: куски sing-box-документа одного узла.
type SingboxNodeFragments struct {
	// NodeTag — РЕАЛЬНЫЙ тег записи узла в документе. Каждое строковое
	// значение, равное ему, переписывается в `@self`: связка обязана пережить
	// переименование узла. Пусто — переписывать нечего.
	NodeTag string
	// DNSServers / DNSRules / RouteRules — тела фрагментов как в документе.
	DNSServers []json.RawMessage
	DNSRules   []json.RawMessage
	RouteRules []json.RawMessage
}

// NodeSectionsFromSingbox переводит фрагменты в записи состояния.
//
// Ошибка = документ не годится целиком: вызывающий показывает причину и
// оставляет узел прежним. Частичного приёма здесь нет намеренно — принять
// половину связки значило бы отдать пользователю узел, который «почти
// работает».
func NodeSectionsFromSingbox(in SingboxNodeFragments) (*NodeSections, error) {
	out := &NodeSections{}

	var servers []DNSServer
	for i, raw := range in.DNSServers {
		body, err := nodeSectionFragmentBody(raw, in.NodeTag, "dns.servers", i, false)
		if err != nil {
			return nil, err
		}
		if body == nil {
			continue
		}
		tag, _ := body["tag"].(string)
		// Тег живёт полем записи, а не в теле: сериализатор DNSServer пишет
		// его на верхнем уровне и полю верит больше, чем карте.
		delete(body, "tag")
		servers = append(servers, DNSServer{
			Kind:    DNSServerKindUser,
			Tag:     tag,
			Enabled: true,
			Body:    body,
		})
	}

	var dnsRules []DNSRule
	for i, raw := range in.DNSRules {
		body, err := nodeSectionFragmentBody(raw, in.NodeTag, "dns.rules", i, false)
		if err != nil {
			return nil, err
		}
		if body == nil {
			continue
		}
		dnsRules = append(dnsRules, DNSRule{
			Kind:    DNSRuleKindUser,
			Enabled: true,
			Body:    body,
		})
	}
	out.SetDNS(servers, dnsRules)

	num := NodeRuleDefaultNum
	for i, raw := range in.RouteRules {
		body, err := nodeSectionFragmentBody(raw, in.NodeTag, "route.rules", i, true)
		if err != nil {
			return nil, err
		}
		if body == nil {
			continue
		}
		rule, err := nodeSectionRuleFromBody(body, i, num)
		if err != nil {
			return nil, fmt.Errorf("route.rules[%d]: %w", i, err)
		}
		out.Rules = append(out.Rules, *rule)
		// Правила одного узла встают на оси подряд: у каждой строки своя
		// позиция, и раздать им один номер значило бы отдать порядок
		// тай-брейку.
		num++
	}

	if out.IsEmpty() {
		return nil, nil
	}
	return out, nil
}

// nodeSectionRuleFromBody — одно sing-box-правило как запись `inline`.
func nodeSectionRuleFromBody(body map[string]interface{}, idx, num int) (*Rule, error) {
	outbound, _ := body["outbound"].(string)
	_, hasAction := body["action"]
	if outbound == "" && !hasAction {
		// Правило без цели — про этот узел: иначе оно ничего не значит.
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
	name, _ := body["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s rule %d", SelfPlaceholderBraced, idx+1)
	} else {
		delete(match, "name")
	}
	encoded, err := json.Marshal(InlineBody{Name: name, Match: match, Outbound: outbound})
	if err != nil {
		return nil, err
	}
	n := num
	return &Rule{
		Kind:     RuleKindInline,
		Enabled:  true,
		OrderNum: &n,
		Body:     encoded,
	}, nil
}

// nodeSectionFragmentBody — общий путь одного фрагмента: перепись реального
// тега в `@self`, отказ на чужой `@var` и на `rule_set`, разбор в карту.
//
// Возвращает (nil, nil) для пустого фрагмента: пустой объект не ошибка, но и
// записью состояния он не становится.
func nodeSectionFragmentBody(raw json.RawMessage, nodeTag, where string, idx int, routeRule bool) (map[string]interface{}, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s[%d]: expected an object: %w", where, idx, err)
	}
	if routeRule {
		if _, has := probe["rule_set"]; has {
			return nil, fmt.Errorf(
				"%s[%d] references a rule set: node sections neither declare nor reference rule sets", where, idx)
		}
	}
	// Перепись тега В ТЕКСТЕ фрагмента, а не по карте: ссылка на узел может
	// стоять в любом поле (`outbound`, `detour`, `endpoint`, `server`), и
	// перечислять их значило бы забыть следующее.
	replaced := raw
	if nodeTag != "" {
		out, err := rewriteJSONStringValues(raw, func(s string) string {
			if s == nodeTag {
				return SelfPlaceholder
			}
			return s
		})
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", where, idx, err)
		}
		replaced = out
	}
	if bad := nodeSectionForeignVars(replaced); len(bad) > 0 {
		return nil, fmt.Errorf("%s[%d] uses %s; a node section knows only %s",
			where, idx, strings.Join(bad, ", "), SelfPlaceholder)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(replaced, &body); err != nil {
		return nil, fmt.Errorf("%s[%d]: expected an object: %w", where, idx, err)
	}
	if len(body) == 0 {
		return nil, nil
	}
	return body, nil
}

// NodeSectionsToSingbox — обратный перевод: записи состояния в куски
// sing-box-документа для показа на вкладке JSON узла.
//
// Плейсхолдер НЕ подставляется: пользователь правит документ и должен видеть
// `@self` там, где он его написал, — иначе следующее сохранение запекло бы в
// секцию сегодняшний тег.
func NodeSectionsToSingbox(sections *NodeSections) SingboxNodeFragments {
	var out SingboxNodeFragments
	if sections == nil {
		return out
	}
	for _, s := range sections.DNSServers() {
		body := map[string]interface{}{}
		for k, v := range s.Body {
			body[k] = v
		}
		if s.Tag != "" {
			body["tag"] = s.Tag
		}
		if raw, err := json.Marshal(body); err == nil {
			out.DNSServers = append(out.DNSServers, raw)
		}
	}
	for _, r := range sections.DNSRules() {
		if raw, err := json.Marshal(r.Body); err == nil {
			out.DNSRules = append(out.DNSRules, raw)
		}
	}
	for _, r := range sections.Rules {
		raw, ok := nodeSectionRuleToSingbox(r)
		if !ok {
			continue
		}
		out.RouteRules = append(out.RouteRules, raw)
	}
	return out
}

// nodeSectionRuleToSingbox — запись правила как sing-box-объект.
func nodeSectionRuleToSingbox(r Rule) (json.RawMessage, bool) {
	body, err := r.DecodeBody()
	if err != nil {
		return nil, false
	}
	obj := map[string]interface{}{}
	switch b := body.(type) {
	case *InlineBody:
		for k, v := range b.Match {
			obj[k] = v
		}
		if b.Outbound != "" {
			obj["outbound"] = b.Outbound
		}
	case *SrsBody:
		// srs-правило узла показывается тем, чем оно является: ссылкой на
		// набор плюс цель. Файл набора кладёт общий конвейер srs.
		if b.SrsURL != "" {
			obj["srs_url"] = b.SrsURL
		}
		if b.Outbound != "" {
			obj["outbound"] = b.Outbound
		}
	default:
		return nil, false
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// nodeSectionVarRe — любое `@имя` в строковом значении. Тот же алфавит, что у
// переменных шаблона (буквы, цифры, подчёркивание, точка). `@{self}` ловится
// как `@` без имени и в список чужих не попадает — он проверяется отдельно.
var nodeSectionVarRe = regexp.MustCompile(`@[A-Za-z_][A-Za-z0-9_.]*`)

// nodeSectionForeignVars — отсортированный список чужих `@var` во фрагменте.
func nodeSectionForeignVars(raw []byte) []string {
	seen := map[string]bool{}
	if _, err := rewriteJSONStringValues(raw, func(s string) string {
		for _, m := range nodeSectionVarRe.FindAllString(s, -1) {
			if m != SelfPlaceholder {
				seen[m] = true
			}
		}
		return s
	}); err != nil {
		return nil
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
