// File node_document.go — разбор ДОКУМЕНТА узла (SPEC 121 §5.1, SPEC 144).
//
// # Что такое документ узла
//
// Вкладка JSON узла принимает, кроме голого тела, фрагмент конфига:
//
//	{ "endpoints": [ { "type": "wireguard", "tag": "ts", … } ] }
//
// Ровно одна запись в `outbounds[]`+`endpoints[]` становится ТЕЛОМ узла.
// Ключи `dns`, `route` и `sections` принимаются и ОТБРАСЫВАЮТСЯ: секций у
// узла нет (контракт 1.1.85, NODE_SECTIONS.md; LxBox §575), а вызывающий
// сообщает пользователю, что остальное содержимое документа не сохранено.
// Любой другой верхний ключ (`log`, `inbounds`, `experimental`) — ошибка с
// перечислением: принять его молча значило бы обещать импорт, которого здесь
// нет.
//
// # Почему это чистая функция в core/config
//
// Тот же разбор нужен трём входам: вкладке JSON окна источника, форме
// «Add server» (SPEC 122) и вставке источника (`carveSingboxJSON`). Второй
// реализацией они разъехались бы на первой же правке правил. Сети и
// состояния здесь нет.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// nodeDocTopKeys — верхние ключи, которые документ узла признаёт.
var nodeDocTopKeys = map[string]bool{
	"outbounds": true,
	"endpoints": true,
	"dns":       true,
	"route":     true,
	"sections":  true,
}

// nodeDocDroppedKeys — признаваемые ключи, содержимое которых не сохраняется
// (секции узла упразднены, контракт 1.1.85). Порядок — порядок сообщения.
var nodeDocDroppedKeys = []string{"dns", "route", "sections"}

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

// ParseNodeDocument разбирает документ узла в тело.
//
// body сохраняет порядок ключей автора байт-в-байт (json.RawMessage +
// json.Compact, а не Unmarshal→Marshal): порядок полей тела значим — он
// сравнивается с выводом эмиттера.
//
// dropped — присутствовавшие в документе ключи `dns`/`route`/`sections`,
// содержимое которых отброшено; вызывающий сообщает о них пользователю.
//
// Возвращает ошибку и НИЧЕГО не меняет, если документ не годится: вызывающий
// показывает причину и оставляет узел прежним (тот же откат, что у
// applyServerBodyJSON).
func ParseNodeDocument(raw []byte) (body json.RawMessage, dropped []string, err error) {
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
			"a node document carries only outbounds/endpoints; unexpected key(s): %s",
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

	var buf bytes.Buffer
	if err := json.Compact(&buf, entries[0]); err != nil {
		return nil, nil, fmt.Errorf("node body: %w", err)
	}
	// Проверка та же, что у прежней формы вкладки: ядро не принимает
	// outbound без типа, и сказать это здесь дешевле, чем на sing-box check.
	var probe map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &probe); err != nil {
		return nil, nil, fmt.Errorf("node body: %w", err)
	}
	if t, _ := probe["type"].(string); strings.TrimSpace(t) == "" {
		return nil, nil, fmt.Errorf("the node object must have a non-empty \"type\" field")
	}

	for _, k := range nodeDocDroppedKeys {
		if v, ok := doc[k]; ok && !isEmptyJSONValue(v) {
			dropped = append(dropped, k)
		}
	}
	return json.RawMessage(buf.Bytes()), dropped, nil
}

// isEmptyJSONValue — null, пустой объект или пустой массив.
func isEmptyJSONValue(raw json.RawMessage) bool {
	t := string(bytes.TrimSpace(raw))
	return t == "" || t == "null" || t == "{}" || t == "[]"
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

// RenderNodeDocument собирает документ из тела узла — обратная операция
// ParseNodeDocument для отрисовки вкладки JSON.
//
// Секция тела выбирается той же схемой, что при эмиссии (EmitNodeJSONs): узел,
// который уедет в `endpoints[]`, и в документе показывается там же.
func RenderNodeDocument(body json.RawMessage, isEndpoint bool) (string, error) {
	// json.RawMessage у тела, а не карта: MarshalIndent переиндентирует
	// вложенный объект, не трогая порядок полей внутри него (тот же приём,
	// что в unpackNodesDoc).
	doc := struct {
		Outbounds []json.RawMessage `json:"outbounds,omitempty"`
		Endpoints []json.RawMessage `json:"endpoints,omitempty"`
	}{}
	if isEndpoint {
		doc.Endpoints = []json.RawMessage{body}
	} else {
		doc.Outbounds = []json.RawMessage{body}
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
