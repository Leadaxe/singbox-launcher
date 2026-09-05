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
//     и на сборке подстановка оставила бы её строкой в конфиге.
//
// # Хранимая форма (SPEC 121 §10)
//
// Наружу разбор отдаёт state.NodeSections — записи ЛАУНЧЕРА (Rule вида
// inline, DNSServer/DNSRule вида user), а не сырые фрагменты sing-box.
// Перевод одного фрагмента в запись живёт в NodeSectionsFromSingbox и зовётся
// отсюда, из ExtractNodeSections (целый конфиг как источник) и из
// конструктора Tailscale: второй реализации правил перевода быть не должно.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/state"
)

// NodeDocumentSelfVar — плейсхолдер «финальный тег этого узла».
// Алиас на state.SelfPlaceholder: строка одна, объявление — рядом с типом.
const NodeDocumentSelfVar = state.SelfPlaceholder

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

	var dnsServers, dnsRules, routeRules []json.RawMessage
	if err := nodeDocSubsection(doc, "dns", nodeDocDNSKeys, func(key string, list []json.RawMessage) error {
		switch key {
		case "servers":
			dnsServers = list
		case "rules":
			dnsRules = list
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if err := nodeDocSubsection(doc, "route", nodeDocRouteKeys, func(key string, list []json.RawMessage) error {
		routeRules = list
		return nil
	}); err != nil {
		return nil, nil, err
	}

	sections, err := state.NodeSectionsFromSingbox(state.SingboxNodeFragments{
		NodeTag:    nodeTag,
		DNSServers: dnsServers,
		DNSRules:   dnsRules,
		RouteRules: routeRules,
	})
	if err != nil {
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
// Секции рисуются в ХРАНИМОЙ форме (SPEC 121 §10.4): пользователь видит ровно
// те записи, которые лежат в состоянии, вместе с их `enabled` и `order_num`.
// Показывать их sing-box-фрагментами значило бы прятать половину полей и
// терять их на следующем сохранении.
//
// Секция тела выбирается той же схемой, что при эмиссии (EmitNodeJSONs): узел,
// который уедет в `endpoints[]`, и в документе показывается там же.
func RenderNodeDocument(body json.RawMessage, sections *state.NodeSections, isEndpoint bool) (string, error) {
	// json.RawMessage у тела, а не карта: MarshalIndent переиндентирует
	// вложенный объект, не трогая порядок полей внутри него (тот же приём,
	// что в unpackNodesDoc).
	doc := struct {
		Outbounds []json.RawMessage   `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage   `json:"endpoints,omitempty"`
		Sections  *state.NodeSections `json:"sections,omitempty"`
	}{}
	if isEndpoint {
		doc.Endpoints = []json.RawMessage{body}
	} else {
		doc.Outbounds = []json.RawMessage{body}
	}
	if !sections.IsEmpty() {
		doc.Sections = sections
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
