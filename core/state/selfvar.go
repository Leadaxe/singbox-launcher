// File selfvar.go — подстановка плейсхолдера узла (SPEC 121 §10.1).
//
// # Что подставляется
//
// Секции узла пишутся до того, как известен его финальный тег: тег зависит от
// тег-политики контейнера и складывается только на эмиссии. Поэтому запись
// секции ссылается на свой узел плейсхолдером:
//
//	"@self"        — строка целиком: `"outbound": "@self"`, `"endpoint": "@self"`
//	"@{self}"      — внутри строки:  `"tag": "@{self}-dns"`, `"name": "@{self} network"`
//
// # Почему свой обход, а не движок @var пресетов
//
// `template.SubstituteVarsInJSONStrict` — строгий: он объявляет ошибкой любое
// `@имя`, которого нет в словаре, и роняет фрагмент целиком. У секции узла
// словаря нет вовсе, а строки её тел пишет пользователь: домен `@example`,
// путь, комментарий — всё это уехало бы в отказ. Здесь правило другое и
// намеренно узкое: заменяются ровно две формы, всё прочее — данные.
//
// # Единственная точка
//
// Подстановка живёт здесь и только здесь: и сборка (инъекция записей в
// route/dns), и UI (подписи строк вкладки DNS) зовут одну функцию. Вторая
// реализация разошлась бы с первой на первой же правке правил — тот же класс
// расхождений, который в этом проекте уже стоил трёх схем.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// errMalformedJSONFragment — вход не является цельным JSON-документом.
var errMalformedJSONFragment = errors.New("malformed JSON fragment")

// Константы плейсхолдера объявлены рядом с типом секций (node_sections.go):
// SelfPlaceholder и SelfPlaceholderBraced.

// SubstituteSelf подставляет финальный тег узла во все строковые ЗНАЧЕНИЯ
// JSON-документа.
//
// Ключи объектов не трогаются: ключ — имя поля sing-box, а не ссылка. Порядок
// ключей сохраняется (потоковая перезапись через json.Decoder), потому что
// тела фрагментов сравниваются с выводом эмиттера байт в байт.
//
// Пустой finalTag (узел без имени — вырожденный случай) возвращает вход как
// есть: подстановка пустой строкой дала бы `"outbound": ""`, то есть висячую
// ссылку вместо честной.
func SubstituteSelf(raw []byte, finalTag string) []byte {
	if len(raw) == 0 || finalTag == "" {
		return raw
	}
	if !bytes.Contains(raw, []byte("@")) {
		return raw
	}
	out, err := rewriteJSONStringValues(raw, func(s string) string {
		return SubstituteSelfInString(s, finalTag)
	})
	if err != nil {
		// Битый JSON здесь не чинится: вызывающий получает вход обратно и
		// разбирается с ним своим путём (разбор всё равно упадёт и назовёт
		// причину точнее, чем это место).
		return raw
	}
	return out
}

// SubstituteSelfInString — подстановка в ОДНОЙ строке.
//
// Порядок значим: `@{self}` заменяется ПЕРВЫМ. Иначе строка `@{self}-dns`
// сначала осталась бы нетронутой (подстроки `@self` в ней нет), а потом
// проверка «строка целиком» её тоже не узнала бы.
//
// Форма `@self` подставляется ТОЛЬКО когда занимает строку целиком: внутри
// строки её пришлось бы отделять от следующего символа, и `@selfish` стало бы
// `<тег>ish`. Для вставки в строку есть скобочная форма.
func SubstituteSelfInString(s, finalTag string) string {
	if finalTag == "" {
		return s
	}
	if strings.Contains(s, SelfPlaceholderBraced) {
		s = strings.ReplaceAll(s, SelfPlaceholderBraced, finalTag)
	}
	if s == SelfPlaceholder {
		return finalTag
	}
	return s
}

// SubstituteSelfInMap — подстановка в карте тела (DNS-запись состояния хранит
// тело картой, а не сырым JSON).
//
// Возвращает НОВУЮ карту: вход принадлежит состоянию, и править его на месте
// значило бы запечь финальный тег в state.json.
func SubstituteSelfInMap(in map[string]interface{}, finalTag string) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = substituteSelfInValue(v, finalTag)
	}
	return out
}

func substituteSelfInValue(v interface{}, finalTag string) interface{} {
	switch t := v.(type) {
	case string:
		return SubstituteSelfInString(t, finalTag)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = substituteSelfInValue(t[i], finalTag)
		}
		return out
	case map[string]interface{}:
		return SubstituteSelfInMap(t, finalTag)
	default:
		return v
	}
}

// rewriteJSONStringValues переписывает каждое строковое ЗНАЧЕНИЕ документа,
// сохраняя порядок ключей.
//
// Реализация — потоковая перезапись через json.Decoder: он выдаёт токены в
// исходном порядке, а ключ объекта отличается от значения по чётности внутри
// `{…}`. Прогон через map[string]interface{} порядок бы потерял.
func rewriteJSONStringValues(raw []byte, fn func(string) string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Без HTML-escaping: тела хранятся так, как их написал пользователь, а
	// `<` вместо `<` — уже другая строка при байтовом сравнении.
	enc.SetEscapeHTML(false)

	// containers — стек: true = объект, false = массив.
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
			if expectKey {
				writeSep()
				if err := encodeJSONScalarInline(enc, &buf, t); err != nil {
					return nil, err
				}
				buf.WriteByte(':')
				expectKey = false
				needComma = false
				continue
			}
			writeSep()
			if err := encodeJSONScalarInline(enc, &buf, fn(t)); err != nil {
				return nil, err
			}
		default:
			writeSep()
			if err := encodeJSONScalarInline(enc, &buf, tok); err != nil {
				return nil, err
			}
		}
		needComma = true
		if len(containers) > 0 && containers[len(containers)-1] {
			expectKey = true
		}
	}
	if len(containers) != 0 {
		return nil, errMalformedJSONFragment
	}
	return append([]byte(nil), buf.Bytes()...), nil
}

// encodeJSONScalarInline пишет одно скалярное значение без завершающего
// перевода строки, который добавляет json.Encoder.
func encodeJSONScalarInline(enc *json.Encoder, buf *bytes.Buffer, v interface{}) error {
	start := buf.Len()
	if err := enc.Encode(v); err != nil {
		return err
	}
	if buf.Len() > start && buf.Bytes()[buf.Len()-1] == '\n' {
		buf.Truncate(buf.Len() - 1)
	}
	return nil
}
