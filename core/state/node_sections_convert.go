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
	"bytes"
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

	// Нумеруются только БЕЗЫМЯННЫЕ правила и только друг относительно друга:
	// номер существует, чтобы их различать, и правило со своим именем в счёт
	// не идёт. Поэтому счётчик свой, а не индекс в общем списке: у конфига с
	// одним безымянным правилом и одним именованным номер не нужен вовсе.
	unnamed := 0
	for i, raw := range in.RouteRules {
		if body, err := nodeSectionFragmentBody(raw, in.NodeTag, "route.rules", i, true); err == nil && body != nil {
			if name, _ := body["name"].(string); strings.TrimSpace(name) == "" {
				unnamed++
			}
		}
	}

	num := NodeRuleDefaultNum
	unnamedSeen := 0
	for i, raw := range in.RouteRules {
		body, err := nodeSectionFragmentBody(raw, in.NodeTag, "route.rules", i, true)
		if err != nil {
			return nil, err
		}
		if body == nil {
			continue
		}
		if name, _ := body["name"].(string); strings.TrimSpace(name) == "" {
			unnamedSeen++
		}
		rule, err := nodeSectionRuleFromBody(body, unnamedSeen, unnamed, num)
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
//
// Тело записи (state v8) — правило sing-box КАК ЕСТЬ: проверки уже сделаны
// (`rule_set` — отказ, чужой `@var` — отказ), цели нет ни в каком виде →
// дописывается `"outbound":"@self"`. Имя — метаданные записи, из тела оно
// снимается (в конфиг `name` не уходил и раньше).
// unnamedNo / unnamedTotal — порядковый номер этого правила СРЕДИ БЕЗЫМЯННЫХ
// и их общее число; для правила со своим именем оба не смотрятся.
func nodeSectionRuleFromBody(body map[string]interface{}, unnamedNo, unnamedTotal, num int) (*Rule, error) {
	name, _ := body["name"].(string)
	named := strings.TrimSpace(name) != ""
	if !named {
		// Номер нужен только чтобы РАЗЛИЧАТЬ безымянные правила одного узла.
		// У единственного различать нечего, и «#1» там читается как обещание
		// второго, которого нет, — поэтому номер появляется начиная с двух.
		// Слово «rule» в списке правил ничего не добавляет, отсюда короткое
		// `#N`.
		name = SelfPlaceholderBraced
		if unnamedTotal > 1 {
			name = fmt.Sprintf("%s #%d", SelfPlaceholderBraced, unnamedNo)
		}
	}
	out := make(map[string]interface{}, len(body)+1)
	for k, v := range body {
		if named && k == "name" {
			continue
		}
		out[k] = v
	}
	outbound, _ := body["outbound"].(string)
	_, hasAction := body["action"]
	if outbound == "" && !hasAction {
		// Правило без цели — про этот узел: иначе оно ничего не значит.
		out["outbound"] = SelfPlaceholder
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	n := num
	return &Rule{
		Kind:    RuleKindInline,
		Name:    name,
		Enabled: true,
		Num:     &n,
		Body:    encoded,
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
	if _, err := r.DecodeBody(); err != nil {
		return nil, false
	}
	switch r.Kind {
	case RuleKindInline:
		// Тело записи УЖЕ правило sing-box — показываем его как есть, вместе
		// с порядком ключей, который написал пользователь.
		if len(bytes.TrimSpace(r.Body)) == 0 {
			return json.RawMessage(`{}`), true
		}
		return append(json.RawMessage(nil), r.Body...), true
	case RuleKindSrs:
		// srs-правило узла показывается тем, чем оно является: ссылками на
		// наборы плюс цель. Файлы наборов кладёт общий конвейер srs. ВСЕ
		// ссылки, а не первая: до v8 правило с тремя наборами эмитило один
		// (ловушка 20а карты SPEC 127).
		body, err := decodeRuleBodyMap(r.Body)
		if err != nil {
			return nil, false
		}
		obj := make(map[string]interface{}, len(body)+1)
		for k, v := range body {
			obj[k] = v
		}
		if len(r.Refs) == 1 {
			obj["rule_set"] = r.Refs[0]
		} else if len(r.Refs) > 1 {
			refs := make([]interface{}, 0, len(r.Refs))
			for _, u := range r.Refs {
				refs = append(refs, u)
			}
			obj["rule_set"] = refs
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, false
		}
		return raw, true
	default:
		return nil, false
	}
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
