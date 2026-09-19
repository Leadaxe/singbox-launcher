package nodeflow

import (
	"encoding/base64"
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
		return asDuration(f, raw)
	case "awg_range":
		return asAWGRange(f, raw)
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
//
// Границы — uint32: это MagicHeader и тайминги ядра, и значение шире 2^32-1
// оно отвергает разбором, роняя весь конфиг. Разбор в uint64 пропускал бы их.
//
// normalize: "range_order" СВОПАЕТ перевёрнутую пару ("40-10" → "10-40").
// Флаг именно у поля, а не у типа: у magic-заголовков h1–h4 порядок границ
// смысла не несёт (ядро выбирает значение ИЗ диапазона, и [10,40] = [40,10]),
// а у таймингов AWG 3.x перевёрнутая пара — опечатка человека, которую он
// обязан увидеть, и там своп запрещён (SPEC 123 §2 «Политика ошибок»).
func asAWGRange(f *registry.Field, raw interface{}) (interface{}, bool) {
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		loStr, hiStr, isRange := strings.Cut(s, "-")
		if !isRange {
			// Голое число строкой — форму не переписываем, лишь проверяем.
			if _, err := strconv.ParseUint(s, 10, 32); err != nil {
				return nil, false
			}
			return s, true
		}
		lo, err := strconv.ParseUint(strings.TrimSpace(loStr), 10, 32)
		if err != nil {
			return nil, false
		}
		hi, err := strconv.ParseUint(strings.TrimSpace(hiStr), 10, 32)
		if err != nil {
			return nil, false
		}
		if hi < lo {
			if f.Normalize != "range_order" {
				// Своп не разрешён — пара негодна, решает on_invalid поля.
				return nil, false
			}
			lo, hi = hi, lo
		}
		return strconv.FormatUint(lo, 10) + "-" + strconv.FormatUint(hi, 10), true
	}
	v, ok := asInt(raw)
	if !ok {
		return nil, false
	}
	if n, isInt := v.(int); isInt && (n < 0 || int64(n) > 0xFFFFFFFF) {
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
	case "hex_only":
		// Оставить только hex-цифры, A-F → a-f (REALITY short_id).
		//
		// Публичные списки кладут в sid моджибейк (UTF-8, прочитанный как
		// Latin-1 → U+00C2), пробелы и пунктуацию; ядро декодирует значение
		// через encoding/hex и падает на любой не-hex руне. Чистка — это
		// перевод написания, а не подгонка значения: ДЛИНУ полученного
		// результата проверяет max/len_parity, и нечётный или слишком
		// длинный sid снимается целиком, а не обрезается (D-032: обрезка
		// дала бы валидную форму с ЧУЖИМ идентификатором).
		var b strings.Builder
		b.Grow(len(v))
		for _, r := range strings.ToValidUTF8(strings.TrimSpace(v), "") {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
				b.WriteRune(r)
			case r >= 'A' && r <= 'F':
				b.WriteRune(r - 'A' + 'a')
			}
		}
		return b.String()
	case "cidr_prefix":
		// Голый адрес получает префикс «весь хост»: ядро требует CIDR
		// (`netip.ParsePrefix("172.16.0.2"): no '/'` — фатал на весь
		// конфиг), а подписки пишут адрес и так, и так. Семейство видно по
		// двоеточию: /32 для IPv4, /128 для IPv6.
		//
		// Перевод написания, а не суждение: значение с префиксом и мусор
		// уезжают как есть, годность судит format cidr.
		t := strings.TrimSpace(v)
		if t == "" || strings.Contains(t, "/") {
			return v
		}
		if strings.Contains(t, ":") {
			return t + "/128"
		}
		return t + "/32"
	case "base64_std":
		// Четыре написания одних и тех же 32 байт → одно. Ядро декодирует
		// ключи WireGuard исключительно base64.StdEncoding, а панели пишут
		// тот же ключ url-safe и без паддинга.
		//
		// Канонизация обязательна не только ради ядра: ОДИН И ТОТ ЖЕ ключ в
		// разных написаниях давал бы узлу два разных identity-хеша
		// (DELTAS D133-22). О годности не судит — значение, которое не
		// декодируется или декодируется не в 32 байта, возвращается КАК
		// ЕСТЬ, и его судит format base64_32 с своим on_invalid.
		for _, enc := range []*base64.Encoding{
			base64.StdEncoding, base64.URLEncoding,
			base64.RawStdEncoding, base64.RawURLEncoding,
		} {
			raw, err := enc.DecodeString(v)
			if err != nil || len(raw) != 32 {
				continue
			}
			return base64.StdEncoding.EncodeToString(raw)
		}
		return v
	case "base64_rawurl":
		// Зеркало base64_std для ядра, которое декодирует ключ ТОЛЬКО
		// base64.RawURLEncoding (REALITY public_key: common/tls/reality_client.go
		// зовёт RawURLEncoding.DecodeString). Написание со «+», «/» и «=»
		// формат base64_32 проходит — 32 байта после декода там правда, —
		// а ядро отвечает «decode public_key: illegal base64 data» отказом
		// ВСЕГО конфига.
		//
		// О годности не судит, как и base64_std: значение, которое не
		// декодируется ни одним алфавитом, возвращается КАК ЕСТЬ, и его
		// судит format со своим on_invalid.
		for _, enc := range []*base64.Encoding{
			base64.RawURLEncoding, base64.URLEncoding,
			base64.RawStdEncoding, base64.StdEncoding,
		} {
			raw, err := enc.DecodeString(v)
			if err != nil || len(raw) != 32 {
				continue
			}
			return base64.RawURLEncoding.EncodeToString(raw)
		}
		return v
	case "duration_bare_seconds":
		// Голое число — это СЕКУНДЫ: живая конвенция панелей
		// (`heartbeat=10`, `tcpKeepAliveIdle: 30`), а ядро ждёт единицу
		// измерения и на голом числе валит весь конфиг. Значение с уже
		// написанной единицей не трогаем: это перевод диалекта, а не
		// нормализация величины.
		//
		// Правило ЗНАЧЕНИЯ, поэтому живёт в теле, а не в маппере: голые
		// секунды приезжают не только из ссылки, но и из xray-sockopt и из
		// тела в форме ядра (SPEC 133, ответ LxBox §24.33).
		t := strings.TrimSpace(v)
		if t == "" {
			return v
		}
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				return v
			}
		}
		return t + "s"
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
// Поэтому число сюда не приводится САМО ПО СЕБЕ: молчаливая замена 30 → "30s"
// угадывала бы единицу измерения за источник. Угадывание объявляется полем:
// normalize duration_bare_seconds говорит, что у ЭТОГО поля голое число —
// секунды (конвенция панелей: heartbeat=10, tcpKeepAliveIdle: 30). Набор
// единиц шире стандартного Go: к ns/us/ms/s/m/h добавлен d (сутки), и голый
// "0" легален.
//
// Число из JSON (float64/int) тоже проходит через нормализатор: у входа
// xray/singbox голые секунды приезжают ЧИСЛОМ, а не строкой, и требовать
// строку значило бы читать правило только со ссылки.
func asDuration(f *registry.Field, raw interface{}) (interface{}, bool) {
	s, ok := raw.(string)
	if !ok {
		if f == nil || f.Normalize != "duration_bare_seconds" {
			return nil, false
		}
		// Только для объявившего поля: иначе любое число у duration тихо
		// стало бы секундами вопреки докстроке выше.
		n, okNum := asInt(raw)
		if !okNum {
			return nil, false
		}
		s = strconv.Itoa(n.(int))
	}
	s = normalize(f.Normalize, s)
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
