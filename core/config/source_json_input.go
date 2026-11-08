// File source_json_input.go — JSON, пришедший источником одной записи
// (вкладка JSON/Source, поле Origin, Regen из правленого текста): вид по
// реестру и текст, который ляжет в origin.raw (контракт 1.1.165,
// PARSING_PRINCIPLES §11 п.2).
//
// Источник хранится в своём диалекте: голое тело — как набрано; документ —
// обрезанный до узловых массивов (`outbounds`/`endpoints` у sing-box,
// `outbounds` у Xray), остальное (`route`, `dns`, `routing`, `log`…) не
// хранится; массив — форма ВВОДА: при правке одного узла в источник идёт
// первый элемент. О несохранённом остатке вызывающий сообщает одним
// сообщением (RestDropped).
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// JSONSourceInput — разобранный ввод.
type JSONSourceInput struct {
	// Kind — вид происхождения (state.OriginKindSingbox | OriginKindXray).
	Kind string
	// SourceKind — имя вида реестра (source_kinds.json), по которому выбрана
	// форма; для диагностики.
	SourceKind string
	// Source — текст, который станет origin.raw: тело, обрезанный документ
	// или первый элемент массива (компактный JSON).
	Source string
	// RestDropped — во вводе было что-то кроме сохранённого: другие ключи
	// документа или элементы массива после первого.
	RestDropped bool
}

// Узловые массивы документа по диалекту: всё прочее при сохранении
// выкидывается (§11 п.2).
var nodeArraysByKind = map[string][]string{
	state.OriginKindSingbox: {"outbounds", "endpoints"},
	state.OriginKindXray:    {"outbounds"},
}

// PrepareJSONSourceInput — ввод JSON → вид и текст источника. Текст, который
// реестр JSON-диалектом не считает (ссылка, INI, битый JSON), — ошибка.
func PrepareJSONSourceInput(text string) (*JSONSourceInput, error) {
	trimmed := strings.TrimSpace(text)
	if !subscription.IsJSONOriginText(trimmed) {
		if trimmed == "" {
			return nil, fmt.Errorf("the input is empty")
		}
		var probe interface{}
		if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
			return nil, fmt.Errorf("the input is not valid JSON: %w", err)
		}
		return nil, fmt.Errorf("the input is not a JSON object or array")
	}
	kind, sourceKind := subscription.ClassifyOriginText(trimmed)
	if !state.IsJSONOriginKind(kind) {
		return nil, fmt.Errorf("the JSON is not a node source the registry knows (sing-box or Xray)")
	}
	in := &JSONSourceInput{Kind: kind, SourceKind: sourceKind}
	element := []byte(trimmed)
	if trimmed[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(element, &list); err != nil {
			return nil, fmt.Errorf("array: %w", err)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("the array carries no node")
		}
		element = list[0]
		in.RestDropped = len(list) > 1
	}
	source, restDropped, err := trimDocumentToNodeArrays(element, kind)
	if err != nil {
		return nil, err
	}
	in.Source = source
	in.RestDropped = in.RestDropped || restDropped
	return in, nil
}

// trimDocumentToNodeArrays — объект документа → объект только из узловых
// массивов своего диалекта; голое тело (объект без узловых массивов)
// возвращается компактным как есть.
func trimDocumentToNodeArrays(element json.RawMessage, kind string) (string, bool, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(element, &obj); err != nil || obj == nil {
		return "", false, fmt.Errorf("node source: %w", err)
	}
	arrays := nodeArraysByKind[kind]
	kept := make(map[string]json.RawMessage, len(arrays))
	for _, k := range arrays {
		if v, ok := obj[k]; ok {
			kept[k] = v
		}
	}
	if len(kept) == 0 {
		var compact bytes.Buffer
		if err := json.Compact(&compact, element); err != nil {
			return "", false, fmt.Errorf("node body: %w", err)
		}
		return compact.String(), false, nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, k := range arrays {
		v, ok := kept[k]
		if !ok {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, _ := json.Marshal(k)
		buf.Write(keyJSON)
		buf.WriteByte(':')
		if err := json.Compact(&buf, v); err != nil {
			return "", false, fmt.Errorf("node document: %w", err)
		}
	}
	buf.WriteByte('}')
	return buf.String(), len(obj) > len(kept), nil
}
