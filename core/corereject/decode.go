// File decode.go — вторая нормативная форма отказа ядра: ошибка
// ДЕКОДИРОВАНИЯ конфига (контракт 1.1.121, PARSING_PRINCIPLES §9.1-§9.4).
//
// # Откуда форма
//
// Тело узла, которое ядро не может даже декодировать (неизвестный ключ,
// значение не того типа), валит старт раньше, чем появится структура
// outbound'а, и тега в строке нет:
//
//	start or reload service: decode config: outbounds[12].network: json: unknown field "network"
//
// Путь `outbounds[12].network` пишет JSON-декодер `sing/common/json`
// (contextjson): сегменты склеены точкой, последний отделён `: `.
//
// # Почему здесь индекс сопоставлять МОЖНО
//
// В форме initialize индекс диагностический и сопоставлять по нему запрещено.
// В форме декодирования ядро декодировало ровно тот массив, который приложение
// записало, поэтому `<i>` однозначно указывает на запись ОТПРАВЛЕННОГО
// конфига. Это сопоставление, а не перебор (§9.3 запрещает перебор). Индекс
// вне массива — узел не назван.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package corereject

import (
	"encoding/json"
	"strings"
)

// SentConfig — финальные теги отправленного ядру конфига В ПОРЯДКЕ МАССИВОВ.
// Порядок обязателен: форма декодирования называет запись индексом.
type SentConfig struct {
	Outbounds []string
	Endpoints []string
}

// SentConfigFromJSON снимает теги outbounds[] и endpoints[] с собранного
// конфига в порядке массивов. Нечитаемый JSON даёт пустой SentConfig: форма
// декодирования тогда никого не назовёт, и это правильный дефолт.
func SentConfigFromJSON(data []byte) SentConfig {
	var raw struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		} `json:"outbounds"`
		Endpoints []struct {
			Tag string `json:"tag"`
		} `json:"endpoints"`
	}
	if len(data) == 0 || json.Unmarshal(data, &raw) != nil {
		return SentConfig{}
	}
	out := SentConfig{
		Outbounds: make([]string, 0, len(raw.Outbounds)),
		Endpoints: make([]string, 0, len(raw.Endpoints)),
	}
	for _, o := range raw.Outbounds {
		out.Outbounds = append(out.Outbounds, o.Tag)
	}
	for _, e := range raw.Endpoints {
		out.Endpoints = append(out.Endpoints, e.Tag)
	}
	return out
}

// ParseSent — разбор отказа ядра по обеим нормативным формам §9.1: сначала
// initialize (сопоставление по тегу), при «не назван» — форма декодирования
// (индекс → тег по отправленному конфигу).
//
// В обеих формах узел считается названным только когда финальный тег есть в
// tags (§9.3: тег без узла сопоставленным не считается).
func ParseSent(out string, tags TagSet, sent SentConfig) (Rejection, bool) {
	if r, ok := Parse(out, tags); ok {
		return r, true
	}
	if strings.TrimSpace(out) == "" || tags == nil {
		return Rejection{}, false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(strings.TrimRight(line, "\r"), " \t")
		r, found := parseDecodeLine(line, sent)
		if !found {
			continue
		}
		// Первое сошедшееся вхождение побеждает (§9.1). Его тег не наш —
		// узел не назван, дальше не ищем: второго отказа в одной ошибке
		// декодер не пишет.
		if !tags.Has(r.Tag) {
			return Rejection{}, false
		}
		return r, true
	}
	return Rejection{}, false
}

// decodeKinds — `<kind>` формы декодирования: имена массивов конфига (путь
// декодера), а не слова формы initialize.
var decodeKinds = []struct {
	marker string
	kind   Kind
}{
	{"outbounds[", KindOutbound},
	{"endpoints[", KindEndpoint},
}

// parseDecodeLine ищет в строке первое вхождение `outbounds[`/`endpoints[`
// слева направо, за которым сходится грамматика
// `<kind>[<i>](.<path>)?: <text>` и индекс попадает в массив отправленного
// конфига. found=false — в строке такого вхождения нет.
func parseDecodeLine(line string, sent SentConfig) (Rejection, bool) {
	pos := 0
	for pos < len(line) {
		at, kind, marker := nextDecodeMarker(line, pos)
		if at < 0 {
			return Rejection{}, false
		}
		pos = at + 1
		rest := line[at+len(marker):]
		idx, rest, ok := takeIndex(rest)
		if !ok || idx < 0 || !strings.HasPrefix(rest, "]") {
			continue
		}
		rest = rest[1:]
		var path, text string
		switch {
		case strings.HasPrefix(rest, ": "):
			text = rest[2:]
		case strings.HasPrefix(rest, "."):
			cut := pathEnd(rest[1:])
			if cut <= 0 {
				continue
			}
			path = rest[1 : 1+cut]
			text = rest[1+cut+len(": "):]
		default:
			continue
		}
		arr := sent.Outbounds
		if kind == KindEndpoint {
			arr = sent.Endpoints
		}
		if idx >= len(arr) || arr[idx] == "" {
			// Индекс вне массива отправленного конфига (или запись без
			// тега) — вхождение не сходится, ищем следующее.
			continue
		}
		reason := text
		if path != "" {
			reason = path + ": " + text
		}
		return Rejection{
			Kind:  kind,
			Index: idx,
			Tag:   arr[idx],
			Path:  path,
			Text:  reason,
		}, true
	}
	return Rejection{}, false
}

// nextDecodeMarker — ближайшее с позиции from вхождение любого маркера.
func nextDecodeMarker(line string, from int) (int, Kind, string) {
	best, bestKind, bestMarker := -1, Kind(""), ""
	for _, k := range decodeKinds {
		i := strings.Index(line[from:], k.marker)
		if i < 0 {
			continue
		}
		i += from
		if best < 0 || i < best {
			best, bestKind, bestMarker = i, k.kind, k.marker
		}
	}
	return best, bestKind, bestMarker
}

// pathEnd — длина пути до разделителя `: `. Экранированные ключи декодера
// (`["a: b"]`) пропускаются целиком, чтобы `: ` внутри имени ключа не резал
// путь. -1 — разделителя нет.
func pathEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], `["`):
			j := i + 2
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				return -1
			}
			i = j // `"`; закрывающая `]` пройдёт обычным шагом
		case strings.HasPrefix(s[i:], ": "):
			return i
		}
	}
	return -1
}
