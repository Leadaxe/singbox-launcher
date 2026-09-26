// File parse.go — разбор строки отказа ядра (SPEC 132 волна 1, норма
// contract/docs/PARSING_PRINCIPLES.md §9.1-9.2).
//
// # Зачем отдельный пакет
//
// Разбор чистый: на входе текст ядра и множество финальных тегов собранного
// конфига, на выходе — назван ли узел и какой. Ни файлов, ни состояния, ни
// UI. Поэтому он живёт отдельно от core (который тянет за собой всё) и зовётся
// из трёх мест сразу: pre-start `check` classic-движка, `ApplyError.Message`
// демона и (будущей волной) текст отказа реального старта.
//
// # Нормативная форма (PARSING_PRINCIPLES §9.1)
//
//	initialize <outbound|endpoint>[<i>] <type>[<tag>]: <text>
//
// Индекс `<i>` ДИАГНОСТИЧЕСКИЙ: он нумерует позицию в собранном конфиге, а не
// узел состояния, и сопоставлять по нему запрещено (PARSING_PRINCIPLES §9.1).
//
// Ядра старше 1.14.1-lx.7 пишут ту же строку БЕЗ ` <type>[<tag>]`
// (`initialize outbound[26]: unknown uTLS fingerprint`) — это форма «без
// тега», и разбор обязан вернуть «узел не назван», а не угадывать.
//
// # Почему кандидаты, а не разрез
//
// `<tag>` и `<text>` разделяет `]: `, и ОБЕ стороны вправе содержать эту
// последовательность: тег — произвольная строка (пробелы, эмодзи, двоеточия,
// скобки), текст ядра — цепочка обёрток через `: `. Однозначного разреза по
// строке не существует. Поэтому берутся ВСЕ вхождения `]: ` справа налево —
// от самого длинного тега к самому короткому — и побеждает первый, который
// сопоставился с тегом собранного конфига. Сопоставление превращает остальные
// варианты в кандидатов, а не в догадки.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package corereject

import (
	"strings"
)

// Kind — вид записи конфига, который назвало ядро.
type Kind string

const (
	// KindOutbound — запись массива `outbounds`.
	KindOutbound Kind = "outbound"
	// KindEndpoint — запись массива `endpoints` (WireGuard, Tailscale).
	KindEndpoint Kind = "endpoint"
)

// Rejection — разобранный отказ ядра, назвавший узел.
type Rejection struct {
	// Kind — outbound или endpoint.
	Kind Kind
	// Index — `<i>` из строки. ТОЛЬКО диагностика: сопоставлять по нему
	// запрещено (PARSING_PRINCIPLES §9.1). -1 = не прочитан.
	Index int
	// Type — тип sing-box (`vless`, `wireguard`, …).
	Type string
	// Tag — ФИНАЛЬНЫЙ тег записи в собранном конфиге, сопоставившийся с
	// переданным множеством тегов.
	Tag string
	// Text — дословный текст ядра без префикса с тегом. Он же едет в
	// `disabled_reason` узла (PARSING_PRINCIPLES §9.4).
	Text string
}

const linePrefix = "initialize "

// TagSet — множество финальных тегов собранного конфига.
//
// Интерфейсом, а не картой: вызывающему обычно удобнее отдать готовую карту
// (`map[string]bool`), но тесты и будущий владелец сборки вправе отдать любой
// предикат. Функция-адаптер — HasTag.
type TagSet interface {
	Has(tag string) bool
}

// HasTag — TagSet из предиката.
type HasTag func(tag string) bool

// Has реализует TagSet.
func (f HasTag) Has(tag string) bool {
	if f == nil {
		return false
	}
	return f(tag)
}

// TagsOf — TagSet из карты тегов. nil-карта даёт множество, в котором нет
// ничего: страховка тогда не действует, и это правильный дефолт.
func TagsOf(tags map[string]bool) TagSet {
	return HasTag(func(tag string) bool {
		if tags == nil {
			return false
		}
		return tags[tag]
	})
}

// Parse ищет в выводе ядра строку отказа, называющую узел.
//
// out — вывод `sing-box check` целиком (многострочный: ядро пишет и баннер
// версии, и WARN-строки), либо одна строка `ApplyError.Message` демона. ANSI к
// этому моменту уже снят вызывающим (core.stripANSI).
//
// tags — финальные теги собранного конфига (outbounds + endpoints).
//
// ok=false означает ровно одно: «узел не назван» — ошибка не про узел, форма
// без тега, или ни один кандидат не сопоставился. Во всех трёх случаях
// страховка не действует (PARSING_PRINCIPLES §9.3), и вызывающий обязан вести себя как
// сегодня.
func Parse(out string, tags TagSet) (Rejection, bool) {
	if strings.TrimSpace(out) == "" || tags == nil {
		return Rejection{}, false
	}
	// Многострочный вывод: нужная строка может стоять где угодно — перед ней
	// идёт баннер ядра, после неё бывают хвосты. Первая подходящая побеждает:
	// ядро отказывается на ПЕРВОМ неприемлемом узле и второй строки такого
	// вида в одном отказе не пишет.
	//
	// Поиск ведётся не по началу строки, а по вхождению префикса: ядро иногда
	// заворачивает отказ в свою рамку (`FATAL[0000] ... initialize ...`), и
	// привязка к началу строки потеряла бы ровно тот случай, ради которого
	// разбор существует.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(strings.TrimRight(line, "\r"), " \t")
		at := strings.Index(line, linePrefix)
		if at < 0 {
			continue
		}
		if r, ok := parseLine(line[at:], tags); ok {
			return r, true
		}
	}
	return Rejection{}, false
}

// parseLine разбирает ОДНУ строку, начинающуюся с `initialize `.
func parseLine(line string, tags TagSet) (Rejection, bool) {
	rest := line[len(linePrefix):]

	// Шаг 1: фиксированный префикс слева — вид записи.
	var kind Kind
	switch {
	case strings.HasPrefix(rest, string(KindOutbound)+"["):
		kind = KindOutbound
	case strings.HasPrefix(rest, string(KindEndpoint)+"["):
		kind = KindEndpoint
	default:
		// `initialize inbound[0] tun: permission denied` и всё прочее —
		// ошибка не про узел.
		return Rejection{}, false
	}
	rest = rest[len(kind)+1:]

	// Индекс: десятичные цифры до `] `. Читается только ради диагностики.
	idx, rest, ok := takeIndex(rest)
	if !ok {
		return Rejection{}, false
	}
	if !strings.HasPrefix(rest, "] ") {
		// `initialize outbound[26]: unknown uTLS fingerprint` — форма ядер до
		// lx.7, без ` <type>[<tag>]`. Узел не назван.
		return Rejection{}, false
	}
	rest = rest[len("] "):]

	// Имя типа и открывающая скобка тега. Тип — идентификатор sing-box, в нём
	// `[` не бывает, поэтому первая же скобка и есть начало тега.
	open := strings.Index(rest, "[")
	if open <= 0 {
		return Rejection{}, false
	}
	nodeType := rest[:open]
	if strings.ContainsAny(nodeType, " \t") {
		// Между `] ` и `[` обязан стоять ровно один токен-тип. Пробел там —
		// строка другой формы, и дальше идти нельзя.
		return Rejection{}, false
	}
	rest = rest[open+1:]

	// Шаг 2-3: кандидаты по `]: `, справа налево, до первого сопоставившегося.
	for cut := strings.LastIndex(rest, "]: "); cut >= 0; cut = strings.LastIndex(rest[:cut], "]: ") {
		tag := rest[:cut]
		if !tags.Has(tag) {
			continue
		}
		return Rejection{
			Kind:  kind,
			Index: idx,
			Type:  nodeType,
			Tag:   tag,
			Text:  rest[cut+len("]: "):],
		}, true
	}
	// Ни один кандидат не сопоставился — узел не назван (PARSING_PRINCIPLES §9.3).
	return Rejection{}, false
}

// takeIndex снимает десятичные цифры слева. Возвращает число, остаток и
// признак того, что цифры вообще были.
func takeIndex(s string) (int, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return -1, s, false
	}
	n := 0
	for j := 0; j < i; j++ {
		// Защита от переполнения на абсурдно длинном числе: индекс
		// диагностический, и его потеря ничего не решает.
		if n > (1<<30)/10 {
			n = -1
			break
		}
		n = n*10 + int(s[j]-'0')
	}
	return n, s[i:], true
}
