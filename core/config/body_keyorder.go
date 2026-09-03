// File body_keyorder.go — работа с телом узла (Node.Body) БЕЗ потери порядка
// ключей (SPEC 118 W4, риск Р1).
//
// Тело узла v7 — это ровно то, что эмиттер лаунчера кладёт в config.json,
// минус `tag` и `detour` (их владелец — модель: тег живёт в Node.Tag,
// маршрут — в NodeLink). Значит на сборке достаточно вернуть эти два ключа
// на их прежние места — и байты совпадут с тем, что писал старый движок,
// который эмитил узел из URI на каждой сборке.
//
// Отсюда требование: порядок ключей тела ОБЯЗАН сохраняться. encoding/json
// сортирует ключи map'ов, поэтому пересборка через map (как делал каркас W2)
// давала стабильный, но ЧУЖОЙ порядок — и байт-эквивалентность эталонов
// ломалась на каждом узле. Здесь порядок ведётся явным списком ключей, а
// значения провозятся сырыми (json.RawMessage) — они не переформатируются
// вовсе.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// orderedJSONObject — JSON-объект с сохранённым порядком ключей и сырыми
// значениями.
type orderedJSONObject struct {
	keys   []string
	values map[string]json.RawMessage
}

// decodeOrderedJSONObject разбирает объект, запоминая порядок ключей.
//
// Дубли ключей (битое тело) схлопываются: побеждает последнее значение, место
// в порядке остаётся за первым вхождением — ровно так же поступает
// encoding/json при разборе в map.
func decodeOrderedJSONObject(raw []byte) (*orderedJSONObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("body is not a JSON object: %w", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("body is not a JSON object")
	}

	obj := &orderedJSONObject{values: make(map[string]json.RawMessage, 8)}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("body: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("body: non-string object key")
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, fmt.Errorf("body: key %q: %w", key, err)
		}
		if _, dup := obj.values[key]; !dup {
			obj.keys = append(obj.keys, key)
		}
		obj.values[key] = val
	}
	if _, err := dec.Token(); err != nil { // закрывающая скобка
		return nil, fmt.Errorf("body: %w", err)
	}
	return obj, nil
}

// delete убирает ключ вместе с его местом в порядке.
func (o *orderedJSONObject) delete(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	kept := o.keys[:0]
	for _, k := range o.keys {
		if k != key {
			kept = append(kept, k)
		}
	}
	o.keys = kept
}

// setFirst ставит ключ ПЕРВЫМ (место `tag` в эмиссии лаунчера).
func (o *orderedJSONObject) setFirst(key string, val json.RawMessage) {
	o.delete(key)
	o.keys = append([]string{key}, o.keys...)
	o.values[key] = val
}

// setLast ставит ключ ПОСЛЕДНИМ (место `detour` в эмиссии лаунчера).
func (o *orderedJSONObject) setLast(key string, val json.RawMessage) {
	o.delete(key)
	o.keys = append(o.keys, key)
	o.values[key] = val
}

// has — есть ли такой ключ.
func (o *orderedJSONObject) has(key string) bool {
	_, ok := o.values[key]
	return ok
}

// stringValue — строковое значение ключа ("" — нет ключа или не строка).
func (o *orderedJSONObject) stringValue(key string) string {
	raw, ok := o.values[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// encode собирает объект обратно — компактно, в сохранённом порядке.
func (o *orderedJSONObject) encode() json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			continue
		}
		b.Write(keyJSON)
		b.WriteByte(':')
		b.Write(o.values[k])
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}

// marshalJSONStringRaw — строка как json.RawMessage (для setFirst/setLast).
func marshalJSONStringRaw(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(b)
}
