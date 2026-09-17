package nodeflow

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config/registry"
)

// coerce приводит значение к типу поля из реестра (SPEC 131 §3.2, ловушка
// Л8: JSON-карта несёт float64 и []interface{}, а эмиттеру ассерты типов
// запрещены — приводит только эта функция).
//
// Второй результат false — значение к типу не сводится; вызывающий исполняет
// on_invalid.
func coerce(f *registry.Field, raw interface{}) (interface{}, bool) {
	if raw == nil {
		return nil, false
	}
	switch f.Type {
	case "string", "enum":
		v, ok := asString(raw)
		if !ok {
			return nil, false
		}
		return normalize(f.Normalize, v), true
	case "int":
		return asInt(raw)
	case "uint16":
		v, ok := asInt(raw)
		if !ok {
			return nil, false
		}
		n := v.(int)
		if n < 0 || n > 65535 {
			return nil, false
		}
		return n, true
	case "bool":
		return asBool(raw)
	case "duration":
		return asDuration(raw)
	case "awg_range":
		return asAWGRange(raw)
	case "int_array":
		return asIntArray(raw)
	case "listable_string":
		return asListable(f, raw)
	case "string_array":
		return asStringArray(f, raw)
	case "object":
		m, ok := asObject(raw)
		if !ok {
			return nil, false
		}
		return m, true
	case "array":
		l, ok := asSlice(raw)
		if !ok {
			return nil, false
		}
		return l, true
	}
	// Тип, которого код не знает: реестр уехал вперёд — пропускаем как есть,
	// не выдумывая приведения.
	return raw, true
}

// asAWGRange — тип AWGRange форка (option/wireguard_awg.go): ЧИСЛО либо
// строка "min-max", из которой ядро выбирает значение на каждое рукопожатие.
// Обе формы ядро принимает (проверено `sing-box check` на 1.14.1-lx.4), и
// форма сохраняется как приехала: число, записанное строкой, сменило бы
// смысл поля на «диапазон из одного значения».
func asAWGRange(raw interface{}) (interface{}, bool) {
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		lo, hi, isRange := strings.Cut(s, "-")
		if !isRange {
			// Голое число строкой — форму не переписываем, лишь проверяем.
			if _, err := strconv.ParseUint(s, 10, 64); err != nil {
				return nil, false
			}
			return s, true
		}
		if _, err := strconv.ParseUint(strings.TrimSpace(lo), 10, 64); err != nil {
			return nil, false
		}
		if _, err := strconv.ParseUint(strings.TrimSpace(hi), 10, 64); err != nil {
			return nil, false
		}
		return s, true
	}
	v, ok := asInt(raw)
	if !ok {
		return nil, false
	}
	if n, isInt := v.(int); isInt && n < 0 {
		return nil, false
	}
	return v, true
}

// asIntArray — массив целых (peers[].reserved: ровно три байта).
func asIntArray(raw interface{}) (interface{}, bool) {
	var src []interface{}
	switch v := raw.(type) {
	case []int:
		out := make([]int, len(v))
		copy(out, v)
		return out, true
	case []interface{}:
		src = v
	default:
		return nil, false
	}
	out := make([]int, 0, len(src))
	for _, item := range src {
		n, ok := asInt(item)
		if !ok {
			return nil, false
		}
		out = append(out, n.(int))
	}
	return out, true
}

// asObject принимает карту в любой из форм, в которых она приезжает: после
// round-trip через JSON это map[string]interface{}, а прямо от URI-парсера —
// нередко map[string]string (transport.headers, node_parser_transport.go:215)
// или map[string]listable. Ассерт ровно одной формы молча ронял бы поле —
// ловушка Л8 ровно про это.
func asObject(raw interface{}) (map[string]interface{}, bool) {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v, true
	case map[string]string:
		out := make(map[string]interface{}, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out, true
	case map[string][]string:
		out := make(map[string]interface{}, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out, true
	}
	return nil, false
}

// asSlice — то же для массивов: peers приезжают []map[string]interface{}
// (node_parser_wireguard.go:216), а после JSON — []interface{}.
func asSlice(raw interface{}) ([]interface{}, bool) {
	switch v := raw.(type) {
	case []interface{}:
		return v, true
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		return out, true
	}
	return nil, false
}

func normalize(mode, v string) string {
	switch mode {
	case "trim":
		return strings.TrimSpace(v)
	case "lower":
		return strings.ToLower(v)
	case "trim_lower":
		return strings.ToLower(strings.TrimSpace(v))
	}
	return v
}

func asString(raw interface{}) (string, bool) {
	switch v := raw.(type) {
	case string:
		return v, true
	}
	return "", false
}

// asInt принимает число без дробной части и строку-число.
func asInt(raw interface{}) (interface{}, bool) {
	switch v := raw.(type) {
	case float64:
		if v != float64(int64(v)) {
			return nil, false
		}
		return int(v), true
	case float32:
		if float64(v) != float64(int64(v)) {
			return nil, false
		}
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil, false
		}
		return int(n), true
	case string:
		s := strings.TrimSpace(v)
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, false
		}
		return n, true
	}
	return nil, false
}

// asBool принимает bool и строки true/false/1/0 — так приезжают параметры
// ссылок (allowInsecure=1, insecure="true").
func asBool(raw interface{}) (interface{}, bool) {
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	case float64:
		switch v {
		case 1:
			return true, true
		case 0:
			return false, true
		}
	}
	return nil, false
}

// asDuration — только строка формата ядра.
//
// badoption.Duration в ядре читает ИСКЛЮЧИТЕЛЬНО JSON-строку
// (sing/common/json/badoption/duration.go: UnmarshalJSON разбирает string и
// зовёт my_time.ParseDuration), число даёт ошибку разбора на весь конфиг.
// Поэтому число сюда не приводится: молчаливая замена 30 → "30s" угадывала
// бы единицу измерения за источник. Набор единиц шире стандартного Go: к
// ns/us/ms/s/m/h добавлен d (сутки), и голый "0" легален.
func asDuration(raw interface{}) (interface{}, bool) {
	s, ok := raw.(string)
	if !ok {
		return nil, false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	if !durationOK(s) {
		return nil, false
	}
	return s, true
}

func durationOK(s string) bool {
	body := s
	if body[0] == '+' || body[0] == '-' {
		body = body[1:]
	}
	if body == "0" {
		return true
	}
	if body == "" {
		return false
	}
	// Единица d ядром поддержана, стандартным time.ParseDuration — нет:
	// заменяем сутки часами только для проверки синтаксиса.
	probe := replaceDayUnit(body)
	if probe == "" {
		return false
	}
	_, err := time.ParseDuration(probe)
	return err == nil
}

// replaceDayUnit переписывает суффикс d в h, умножая число на 24; на любой
// неожиданности возвращает пустую строку (значение будет отвергнуто).
func replaceDayUnit(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		start := i
		for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
			i++
		}
		num := s[start:i]
		unitStart := i
		for i < len(s) && !(s[i] >= '0' && s[i] <= '9') && s[i] != '.' {
			i++
		}
		unit := s[unitStart:i]
		if unit == "d" {
			if num == "" {
				return ""
			}
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return ""
			}
			b.WriteString(strconv.FormatFloat(f*24, 'f', -1, 64))
			b.WriteString("h")
			continue
		}
		b.WriteString(num)
		b.WriteString(unit)
	}
	return b.String()
}

// asListable — строка или массив строк; ФОРМА СОХРАНЯЕТСЯ. Канонизация
// формы не задача конвейера: ядро принимает обе, а переписывание строки в
// массив ломало бы сравнение с исходником (SPEC 131 §3.3).
func asListable(f *registry.Field, raw interface{}) (interface{}, bool) {
	switch v := raw.(type) {
	case string:
		return normalize(f.Normalize, v), true
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, normalize(f.Normalize, item))
		}
		return out, true
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, normalize(f.Normalize, s))
		}
		return out, true
	}
	return nil, false
}

// asStringArray — только массив (certificate_public_key_sha256, allowed_ips).
func asStringArray(f *registry.Field, raw interface{}) (interface{}, bool) {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, normalize(f.Normalize, item))
		}
		return out, true
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, normalize(f.Normalize, s))
		}
		return out, true
	}
	return nil, false
}
