package nodeflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Emit сериализует чистую карту в тело узла в порядке body.order реестра —
// то есть в порядке структур ядра (SPEC 131 §3.3).
//
// Ни одного per-scheme switch и ни одного ассерта типа (.(int)/.([]string)):
// значения уже приведены санитайзером, а кодируются они через encoding/json,
// поэтому неожиданный тип даёт ошибку, а не молчаливую потерю поля (ловушка
// Л8). Записываются только ключи, которые есть в карте; у tristate-полей
// пустое значение пишется явно, а не опускается.
func Emit(scheme string, clean map[string]interface{}) ([]byte, error) {
	reg, err := registry.Get()
	if err != nil {
		return nil, err
	}
	body, ok := reg.Body(scheme)
	if !ok {
		return nil, fmt.Errorf("nodeflow: схема %q неизвестна реестру", scheme)
	}
	var buf bytes.Buffer
	if err := emitObject(&buf, body.Order, body.Fields, clean); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func emitObject(buf *bytes.Buffer, order []string, fields map[string]*registry.Field, m map[string]interface{}) error {
	buf.WriteByte('{')
	first := true
	for _, name := range order {
		v, ok := m[name]
		if !ok {
			continue
		}
		f := fields[name]
		if !first {
			buf.WriteByte(',')
		}
		first = false
		if err := writeString(buf, name); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := emitValue(buf, f, v); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func emitValue(buf *bytes.Buffer, f *registry.Field, v interface{}) error {
	if f == nil {
		return writeJSON(buf, v)
	}
	switch inner := v.(type) {
	case map[string]interface{}:
		order, fields := emitShape(f, inner)
		if order == nil {
			// Свободная карта (headers) — пишем как есть; порядок ключей
			// задаёт encoding/json (алфавитный), ядру он безразличен.
			return writeJSON(buf, inner)
		}
		return emitObject(buf, order, fields, inner)
	case []interface{}:
		if f.Items == nil || len(f.Items.Fields) == 0 {
			return writeJSON(buf, inner)
		}
		buf.WriteByte('[')
		for i, item := range inner {
			if i > 0 {
				buf.WriteByte(',')
			}
			m, ok := item.(map[string]interface{})
			if !ok {
				if err := writeJSON(buf, item); err != nil {
					return err
				}
				continue
			}
			if err := emitObject(buf, f.Items.Order, f.Items.Fields, m); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	}
	return writeJSON(buf, v)
}

// emitShape выбирает порядок и описание полей для вложенного объекта: у
// вариантного узла (transport) — по значению дискриминатора, иначе — своё.
// nil-order означает свободную карту.
func emitShape(f *registry.Field, m map[string]interface{}) ([]string, map[string]*registry.Field) {
	if len(f.Variants) > 0 {
		disc, _ := m[f.Discriminator].(string)
		v := f.Variants[strings.TrimSpace(disc)]
		if v == nil {
			return nil, nil
		}
		// Дискриминатор пишется первым: у структур ядра type идёт головой.
		order := make([]string, 0, len(v.Order)+1)
		order = append(order, f.Discriminator)
		order = append(order, v.Order...)
		fields := make(map[string]*registry.Field, len(v.Fields)+1)
		for k, fl := range v.Fields {
			fields[k] = fl
		}
		fields[f.Discriminator] = &registry.Field{Type: "string"}
		return order, fields
	}
	if len(f.Fields) == 0 {
		return nil, nil
	}
	return f.Order, f.Fields
}

// writeJSON кодирует значение через encoding/json без экранирования HTML:
// тело узла уезжает в config.json, где &, < и > в путях и заголовках должны
// остаться собой.
func writeJSON(buf *bytes.Buffer, v interface{}) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	b := tmp.Bytes()
	// Encode дописывает перевод строки — компактный JSON его не носит.
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	buf.Write(b)
	return nil
}

func writeString(buf *bytes.Buffer, s string) error {
	return writeJSON(buf, s)
}
