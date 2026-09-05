// File node_document.go — разбор ДОКУМЕНТА узла (SPEC 121 §5.1).
//
// # Что такое документ узла
//
// Узел с секциями нельзя показать одним outbound-объектом: DNS-сервер,
// DNS-правило и правило маршрута живут рядом с телом, а не внутри него.
// Поэтому вкладка JSON узла принимает вторую форму — фрагмент конфига:
//
//	{
//	  "endpoints": [ { "type": "wireguard", "tag": "ts", … } ],
//	  "dns":   { "servers": [ … ], "rules": [ … ] },
//	  "route": { "rules":   [ … ] }
//	}
//
// Ровно одна запись в `outbounds[]`+`endpoints[]` становится ТЕЛОМ узла,
// остальное — его секциями. Документ намеренно НЕ является «конфигом
// целиком»: любой другой верхний ключ (`log`, `inbounds`, `experimental`) —
// ошибка с перечислением, потому что принять его молча значило бы обещать
// пользователю импорт, которого здесь нет.
//
// # Почему это чистая функция в core/config
//
// Тот же разбор нужен трём входам: вкладке JSON окна источника, форме
// «Add server» (SPEC 122) и вставке источника (`carveSingboxJSON`). Второй
// реализацией они разъехались бы на первой же правке правил — тот самый класс
// расхождений, который в этом проекте уже стоил трёх схем («эмиттер и парсер
// ходят парой»). Сети и состояния здесь нет.
//
// # Правила, которые применяет разбор
//
//  1. ссылка на РЕАЛЬНЫЙ тег записи узла внутри секций переписывается в
//     `@self` — пользователь вставляет готовый конфиг, а связка обязана
//     пережить переименование узла;
//  2. правило маршрута без `outbound` и без `action` получает
//     `"outbound": "@self"`: правило секции — про этот узел, иначе оно
//     ничего не значит;
//  3. `rule_set` в правиле — ошибка: в v1 секции наборов правил не
//     объявляют и на них не ссылаются, а висячая ссылка роняет конфиг целиком;
//  4. `@var`, кроме `@self`, — ошибка: у узла нет своего словаря переменных,
//     и на сборке строгая подстановка выбросила бы фрагмент молча для
//     пользователя.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"singbox-launcher/core/state"
)

// NodeDocumentSelfVar — плейсхолдер «финальный тег этого узла».
const NodeDocumentSelfVar = "@self"

// nodeDocTopKeys — верхние ключи, которые документ узла признаёт.
var nodeDocTopKeys = map[string]bool{
	"outbounds": true,
	"endpoints": true,
	"dns":       true,
	"route":     true,
}

// nodeDocDNSKeys / nodeDocRouteKeys — ключи, которые разбор читает внутри
// `dns` и `route`. Всё остальное (`final`, `strategy`,
// `default_domain_resolver`, `auto_detect_interface`) — настройки конфига
// ЦЕЛИКОМ, а не узла: принять их у одного узла значило бы дать ему править
// общие поля мимо вкладок, где они живут.
var (
	nodeDocDNSKeys   = map[string]bool{"servers": true, "rules": true}
	nodeDocRouteKeys = map[string]bool{"rules": true}
)

// nodeDocVarRe — любое `@имя` в строковом значении. Тот же алфавит, что у
// переменных шаблона (буквы, цифры, подчёркивание, точка).
var nodeDocVarRe = regexp.MustCompile(`@[A-Za-z_][A-Za-z0-9_.]*`)

// IsNodeDocument сообщает, выглядит ли текст документом узла, а не голым
// outbound-объектом.
//
// Признак ровно один и намеренно узкий: объект БЕЗ `type`, но хотя бы с одним
// ключом документа. Объект с `type` — тело узла и разбирается прежним путём
// (порядок проверок тот же, что в classifyJSONObjectBody: `type` раньше
// `outbounds`).
func IsNodeDocument(raw []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	if t, ok := probe["type"]; ok && len(bytes.TrimSpace(t)) > 0 && string(bytes.TrimSpace(t)) != "null" {
		return false
	}
	for k := range probe {
		if nodeDocTopKeys[k] {
			return true
		}
	}
	return false
}

// ParseNodeDocument разбирает документ узла в тело и секции.
//
// body сохраняет порядок ключей автора байт-в-байт (json.RawMessage +
// json.Compact, а не Unmarshal→Marshal): порядок полей тела значим — он
// сравнивается с выводом эмиттера.
//
// Возвращает ошибку и НИЧЕГО не меняет, если документ не годится: вызывающий
// показывает причину и оставляет узел прежним (тот же откат, что у
// applyServerBodyJSON).
func ParseNodeDocument(raw []byte) (json.RawMessage, *state.NodeSections, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("document: %w", err)
	}

	// Лишние верхние ключи — перечислением: «документ отвергнут» без имён
	// оставляет пользователя гадать, что именно здесь чужое.
	var extra []string
	for k := range doc {
		if !nodeDocTopKeys[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, nil, fmt.Errorf(
			"a node document carries only outbounds/endpoints, dns and route; unexpected key(s): %s",
			strings.Join(extra, ", "))
	}

	entries, err := nodeDocEntries(doc)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) != 1 {
		return nil, nil, fmt.Errorf(
			"a node document must carry exactly one node in outbounds/endpoints (found %d)", len(entries))
	}

	bodyRaw := entries[0]
	nodeTag := nodeDocTagOf(bodyRaw)

	var body bytes.Buffer
	if err := json.Compact(&body, bodyRaw); err != nil {
		return nil, nil, fmt.Errorf("node body: %w", err)
	}
	// Проверка та же, что у прежней формы вкладки: ядро не принимает
	// outbound без типа, и сказать это здесь дешевле, чем на sing-box check.
	var probe map[string]interface{}
	if err := json.Unmarshal(body.Bytes(), &probe); err != nil {
		return nil, nil, fmt.Errorf("node body: %w", err)
	}
	if t, _ := probe["type"].(string); strings.TrimSpace(t) == "" {
		return nil, nil, fmt.Errorf("the node object must have a non-empty \"type\" field")
	}

	sections := &state.NodeSections{}
	if err := nodeDocSubsection(doc, "dns", nodeDocDNSKeys, func(key string, list []json.RawMessage) error {
		switch key {
		case "servers":
			out, err := nodeDocFragments(list, nodeTag, "dns.servers", false)
			if err != nil {
				return err
			}
			sections.DNSServers = out
		case "rules":
			out, err := nodeDocFragments(list, nodeTag, "dns.rules", false)
			if err != nil {
				return err
			}
			sections.DNSRules = out
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if err := nodeDocSubsection(doc, "route", nodeDocRouteKeys, func(key string, list []json.RawMessage) error {
		out, err := nodeDocFragments(list, nodeTag, "route.rules", true)
		if err != nil {
			return err
		}
		sections.Rules = out
		return nil
	}); err != nil {
		return nil, nil, err
	}

	if sections.IsEmpty() {
		// Документ без секций — это просто тело, обёрнутое в конверт. Пустой
		// набор в состоянии был бы третьим состоянием поля (nil / пусто / есть).
		return json.RawMessage(body.Bytes()), nil, nil
	}
	return json.RawMessage(body.Bytes()), sections, nil
}

// nodeDocEntries — записи узла из `outbounds[]` и `endpoints[]` одним списком.
func nodeDocEntries(doc map[string]json.RawMessage) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for _, key := range []string{"outbounds", "endpoints"} {
		raw, ok := doc[key]
		if !ok {
			continue
		}
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("%s: expected an array of objects: %w", key, err)
		}
		out = append(out, list...)
	}
	return out, nil
}

// nodeDocSubsection разбирает `dns` / `route`: проверяет ключи внутри и отдаёт
// каждый известный список в apply.
func nodeDocSubsection(
	doc map[string]json.RawMessage,
	section string,
	allowed map[string]bool,
	apply func(key string, list []json.RawMessage) error,
) error {
	raw, ok := doc[section]
	if !ok {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("%s: expected an object: %w", section, err)
	}
	var extra []string
	for k := range obj {
		if !allowed[k] {
			extra = append(extra, section+"."+k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return fmt.Errorf("%q carries node fragments only; unexpected key(s): %s",
			section, strings.Join(extra, ", "))
	}
	for k := range allowed {
		listRaw, has := obj[k]
		if !has {
			continue
		}
		var list []json.RawMessage
		if err := json.Unmarshal(listRaw, &list); err != nil {
			return fmt.Errorf("%s.%s: expected an array of objects: %w", section, k, err)
		}
		if err := apply(k, list); err != nil {
			return err
		}
	}
	return nil
}

// nodeDocFragments прогоняет список фрагментов через правила §5.1: перепись
// реального тега в `@self`, дефолт `outbound` у правила маршрута, отказ на
// `rule_set` и на чужой `@var`.
//
// routeRule=true включает две проверки, которые касаются только правил
// маршрута; у DNS-фрагментов их нет.
func nodeDocFragments(list []json.RawMessage, nodeTag, where string, routeRule bool) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(list))
	for i, raw := range list {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("%s[%d]: expected an object: %w", where, i, err)
		}
		if routeRule {
			if _, has := obj["rule_set"]; has {
				return nil, fmt.Errorf(
					"%s[%d] references a rule set: node sections neither declare nor reference rule sets", where, i)
			}
		}
		// Перепись тега В ТЕКСТЕ фрагмента, а не по карте: ссылка на узел
		// может стоять в любом поле (`outbound`, `detour`, `endpoint`,
		// `server`), и перечислять их значило бы забыть следующее.
		replaced := raw
		if nodeTag != "" {
			var err error
			replaced, err = nodeDocReplaceTag(raw, nodeTag)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", where, i, err)
			}
		}
		if bad := nodeDocForeignVars(replaced); len(bad) > 0 {
			return nil, fmt.Errorf(
				"%s[%d] uses %s; a node section knows only %s",
				where, i, strings.Join(bad, ", "), NodeDocumentSelfVar)
		}
		if routeRule {
			var err error
			replaced, err = nodeDocDefaultOutbound(replaced)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", where, i, err)
			}
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, replaced); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", where, i, err)
		}
		out = append(out, json.RawMessage(append([]byte(nil), compact.Bytes()...)))
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// nodeDocReplaceTag заменяет КАЖДОЕ строковое значение, равное тегу узла, на
// `@self`.
//
// Обход по ТОКЕНАМ, а не текстовая замена: тег вида `ts` встречался бы
// подстрокой в чужих значениях («hosts», «.ts.net») и портил бы их. И не через
// map[string]interface{}: он потерял бы порядок ключей фрагмента, а он значим
// ровно так же, как у тела узла (CODEMAP §10 п. 21).
func nodeDocReplaceTag(raw json.RawMessage, nodeTag string) (json.RawMessage, error) {
	return nodeDocRewriteStrings(raw, func(s string) string {
		if s == nodeTag {
			return NodeDocumentSelfVar
		}
		return s
	})
}

// nodeDocRewriteStrings переписывает каждое строковое ЗНАЧЕНИЕ фрагмента,
// сохраняя порядок ключей.
//
// Реализация — потоковая перезапись через json.Decoder: он выдаёт токены в
// исходном порядке, а ключ объекта отличается от значения по чётности внутри
// `{…}`. Ключи не трогаются: ключ — имя поля sing-box, а не ссылка.
func nodeDocRewriteStrings(raw json.RawMessage, fn func(string) string) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Без HTML-escaping: тело узла и фрагменты хранятся так, как их написал
	// пользователь, а `<` вместо `<` — уже другая строка при байтовом
	// сравнении с выводом эмиттера.
	enc.SetEscapeHTML(false)
	// containers — стек: true = объект, false = массив. expectKey — следующий
	// строковый токен в объекте является ключом.
	var containers []bool
	expectKey := false
	needComma := false

	writeSep := func() {
		if needComma {
			buf.WriteByte(',')
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				writeSep()
				buf.WriteByte('{')
				containers = append(containers, true)
				expectKey = true
				needComma = false
				continue
			case '[':
				writeSep()
				buf.WriteByte('[')
				containers = append(containers, false)
				expectKey = false
				needComma = false
				continue
			case '}', ']':
				buf.WriteByte(byte(t))
				containers = containers[:len(containers)-1]
				needComma = true
				expectKey = len(containers) > 0 && containers[len(containers)-1]
				continue
			}
		case string:
			isKey := expectKey
			if isKey {
				writeSep()
				if err := nodeDocEncodeInline(enc, &buf, t); err != nil {
					return nil, err
				}
				buf.WriteByte(':')
				expectKey = false
				needComma = false
				continue
			}
			writeSep()
			if err := nodeDocEncodeInline(enc, &buf, fn(t)); err != nil {
				return nil, err
			}
		default:
			writeSep()
			if err := nodeDocEncodeInline(enc, &buf, tok); err != nil {
				return nil, err
			}
		}
		needComma = true
		if len(containers) > 0 && containers[len(containers)-1] {
			expectKey = true
		}
	}
	if len(containers) != 0 {
		return nil, fmt.Errorf("malformed JSON fragment")
	}
	return json.RawMessage(append([]byte(nil), buf.Bytes()...)), nil
}

// nodeDocEncodeInline пишет одно скалярное значение без завершающего перевода
// строки, который добавляет json.Encoder.
func nodeDocEncodeInline(enc *json.Encoder, buf *bytes.Buffer, v interface{}) error {
	start := buf.Len()
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encode дописывает '\n' — снимаем его, чтобы вывод остался компактным.
	if buf.Len() > start && buf.Bytes()[buf.Len()-1] == '\n' {
		buf.Truncate(buf.Len() - 1)
	}
	return nil
}

// nodeDocForeignVars — отсортированный список чужих `@var` во фрагменте.
func nodeDocForeignVars(raw json.RawMessage) []string {
	seen := map[string]bool{}
	if _, err := nodeDocRewriteStrings(raw, func(s string) string {
		for _, m := range nodeDocVarRe.FindAllString(s, -1) {
			if m != NodeDocumentSelfVar {
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

// nodeDocDefaultOutbound дописывает `"outbound": "@self"` правилу без цели.
//
// Делается ЗДЕСЬ, при сохранении, а не только на сборке: пользователь видит
// результат в теле, а сборка повторяет то же защитно (state мог приехать из
// бэкапа, написанного другой рукой).
func nodeDocDefaultOutbound(raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if _, has := obj["outbound"]; has {
		return raw, nil
	}
	if _, has := obj["action"]; has {
		return raw, nil
	}
	// Ключ дописывается В КОНЕЦ текста объекта, а не пересборкой карты:
	// Unmarshal→Marshal отсортировал бы поля правила по алфавиту, и тело
	// менялось бы на каждом сохранении.
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, fmt.Errorf("expected an object")
	}
	inner := bytes.TrimSpace(trimmed[1 : len(trimmed)-1])
	if len(inner) == 0 {
		return json.RawMessage(`{"outbound":"` + NodeDocumentSelfVar + `"}`), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	buf.Write(inner)
	buf.WriteString(`,"outbound":"` + NodeDocumentSelfVar + `"}`)
	return json.RawMessage(buf.Bytes()), nil
}

// nodeDocTagOf — тег записи узла из документа ("" если его нет).
func nodeDocTagOf(raw json.RawMessage) string {
	var probe struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.Tag)
}

// NodeBodyGoesToEndpoints — уедет ли тело узла в `endpoints[]`, а не в
// `outbounds[]`.
//
// Признак читается ТЕМ ЖЕ предикатом, что на эмиссии (EmitNodeJSONs →
// IsEndpointScheme): второй предикат разошёлся бы с первым, и документ
// показывал бы узел не в той секции, в которой он окажется в конфиге.
func NodeBodyGoesToEndpoints(body json.RawMessage) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return IsEndpointScheme(canonicalSchemeFromType(strings.ToLower(strings.TrimSpace(probe.Type))))
}

// RenderNodeDocument собирает документ из тела и секций узла — обратная
// операция ParseNodeDocument для отрисовки вкладки JSON.
//
// Секция выбирается той же схемой, что при эмиссии (EmitNodeJSONs): узел,
// который уедет в `endpoints[]`, и в документе показывается там же.
func RenderNodeDocument(body json.RawMessage, sections *state.NodeSections, isEndpoint bool) (string, error) {
	type dnsDoc struct {
		Servers []json.RawMessage `json:"servers,omitempty"`
		Rules   []json.RawMessage `json:"rules,omitempty"`
	}
	type routeDoc struct {
		Rules []json.RawMessage `json:"rules,omitempty"`
	}
	// json.RawMessage, а не карты: MarshalIndent переиндентирует вложенные
	// объекты, не трогая порядок полей внутри них (тот же приём, что в
	// unpackNodesDoc).
	doc := struct {
		Outbounds []json.RawMessage `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage `json:"endpoints,omitempty"`
		DNS       *dnsDoc           `json:"dns,omitempty"`
		Route     *routeDoc         `json:"route,omitempty"`
	}{}
	if isEndpoint {
		doc.Endpoints = []json.RawMessage{body}
	} else {
		doc.Outbounds = []json.RawMessage{body}
	}
	if sections != nil {
		if len(sections.DNSServers) > 0 || len(sections.DNSRules) > 0 {
			doc.DNS = &dnsDoc{Servers: sections.DNSServers, Rules: sections.DNSRules}
		}
		if len(sections.Rules) > 0 {
			doc.Route = &routeDoc{Rules: sections.Rules}
		}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
