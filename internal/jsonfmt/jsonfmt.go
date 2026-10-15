// Package jsonfmt печатает JSON в компактном каноне шаблона: короткие
// контейнеры — в одну строку, длинные — по элементу на строку.
//
// Правило одно и рекурсивное: объект или массив печатается в строку
// `{"a": 1, "b": [1, 2]}` / `["x", "y"]`, если эта строка (без отступа и без
// ключа перед ней) не длиннее MaxInlineWidth символов, иначе — многострочно с
// отступом в два пробела, и к каждому дочернему контейнеру применяется то же
// правило. Порядок ключей и литералы чисел сохраняются как в исходнике,
// строки кодируются без HTML-экранирования, не-ASCII остаётся как есть.
// Формат идемпотентен: FormatCompact(FormatCompact(x)) == FormatCompact(x).
package jsonfmt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// MaxInlineWidth — предельная длина контейнера в одну строку, в символах.
const MaxInlineWidth = 100

const indentUnit = "  "

// value — узел упорядоченного дерева JSON. У скаляра заполнен lit (готовый
// литерал), у объекта — keys и items (keys[i] — закодированный ключ items[i]),
// у массива — только items.
type value struct {
	kind  byte // 's' скаляр, 'o' объект, 'a' массив
	lit   string
	keys  []string
	items []*value
	flat  string // кэш однострочной формы контейнера
}

// FormatCompact разбирает raw и печатает его в компактном каноне с одним
// переводом строки в конце. Невалидный JSON (в том числе лишние данные после
// значения) — ошибка.
func FormatCompact(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	root, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("jsonfmt: unexpected data after top-level value")
		}
		return nil, fmt.Errorf("jsonfmt: %w", err)
	}
	var b strings.Builder
	write(&b, root, "")
	b.WriteByte('\n')
	return []byte(b.String()), nil
}

func parseValue(dec *json.Decoder) (*value, error) {
	tok, err := dec.Token()
	if err != nil {
		if err == io.EOF {
			return nil, errors.New("jsonfmt: unexpected end of input")
		}
		return nil, fmt.Errorf("jsonfmt: %w", err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return parseObject(dec)
		case '[':
			return parseArray(dec)
		}
		return nil, fmt.Errorf("jsonfmt: unexpected delimiter %q", t)
	case string:
		lit, err := encodeString(t)
		if err != nil {
			return nil, err
		}
		return &value{kind: 's', lit: lit}, nil
	case json.Number:
		return &value{kind: 's', lit: t.String()}, nil
	case bool:
		if t {
			return &value{kind: 's', lit: "true"}, nil
		}
		return &value{kind: 's', lit: "false"}, nil
	case nil:
		return &value{kind: 's', lit: "null"}, nil
	}
	return nil, fmt.Errorf("jsonfmt: unexpected token %v", tok)
}

func parseObject(dec *json.Decoder) (*value, error) {
	v := &value{kind: 'o'}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("jsonfmt: %w", err)
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("jsonfmt: object key expected, got %v", tok)
		}
		encKey, err := encodeString(key)
		if err != nil {
			return nil, err
		}
		item, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		v.keys = append(v.keys, encKey)
		v.items = append(v.items, item)
	}
	return v, closeDelim(dec, '}')
}

func parseArray(dec *json.Decoder) (*value, error) {
	v := &value{kind: 'a'}
	for dec.More() {
		item, err := parseValue(dec)
		if err != nil {
			return nil, err
		}
		v.items = append(v.items, item)
	}
	return v, closeDelim(dec, ']')
}

func closeDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("jsonfmt: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("jsonfmt: expected %q, got %v", want, tok)
	}
	return nil
}

// encodeString — JSON-литерал строки без HTML-экранирования (`<`, `>`, `&`
// остаются как есть) и без экранирования не-ASCII.
func encodeString(s string) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", fmt.Errorf("jsonfmt: encode string: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// flatForm — однострочная форма узла; у контейнера считается один раз.
func (v *value) flatForm() string {
	if v.kind == 's' {
		return v.lit
	}
	if v.flat != "" {
		return v.flat
	}
	var b strings.Builder
	opening, closing := byte('['), byte(']')
	if v.kind == 'o' {
		opening, closing = '{', '}'
	}
	b.WriteByte(opening)
	for i, item := range v.items {
		if i > 0 {
			b.WriteString(", ")
		}
		if v.kind == 'o' {
			b.WriteString(v.keys[i])
			b.WriteString(": ")
		}
		b.WriteString(item.flatForm())
	}
	b.WriteByte(closing)
	v.flat = b.String()
	return v.flat
}

// write печатает узел, начиная с текущей позиции строки; indent — отступ
// строки, на которой узел начался (по нему встаёт закрывающая скобка).
func write(b *strings.Builder, v *value, indent string) {
	flat := v.flatForm()
	if v.kind == 's' || len(v.items) == 0 || utf8.RuneCountInString(flat) <= MaxInlineWidth {
		b.WriteString(flat)
		return
	}
	opening, closing := "[", "]"
	if v.kind == 'o' {
		opening, closing = "{", "}"
	}
	inner := indent + indentUnit
	b.WriteString(opening)
	b.WriteByte('\n')
	for i, item := range v.items {
		b.WriteString(inner)
		if v.kind == 'o' {
			b.WriteString(v.keys[i])
			b.WriteString(": ")
		}
		write(b, item, inner)
		if i < len(v.items)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent)
	b.WriteString(closing)
}
