package linkmap

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Трасса движка — общий с LxBox формат для МЕХАНИЧЕСКОЙ сверки Go↔Dart
// диффом. Норма формата — contract/docs/MAPPER_ENGINE.md, приложение
// «Трасса»; менять его в одностороннем порядке нельзя.
//
// JSON Lines: одна строка = событие, ключи в ФИКСИРОВАННОМ порядке, без
// времени и адресов памяти. Порядок событий = порядок исполнения и он
// нормативен.
//
// Коллектор опционален и по умолчанию выключен: nil-приёмник не стоит
// ничего, поэтому вызовы Add можно ставить безусловно.

// Стадии (stage).
const (
	StageDocDetect  = "doc_detect"
	StageUnwrap     = "unwrap"
	StageElemDetect = "elem_detect"
	StageLex        = "lex"
	StageField      = "field"
	StageSets       = "sets"
	StageDefault    = "default"
	StageLabel      = "label"
	StageUnknown    = "unknown"
	StageResult     = "result"
)

// Действия (act).
const (
	ActWrite    = "write"
	ActSkip     = "skip"
	ActRemove   = "remove"
	ActOverride = "override"
	ActKeep     = "keep"
)

// Причины (why). Значения с двоеточием собираются функциями ниже.
const (
	WhyNone               = "-"
	WhyWhenFalse          = "when_false"
	WhyEmpty              = "empty"
	WhyNotDeclared        = "not_declared"
	WhyDefault            = "default"
	WhyMaterializeDefault = "materialize_default"
)

// WhyLowerPriority — запись проиграла другой записи в тот же путь.
func WhyLowerPriority(entry string) string { return "lower_priority:" + entry }

// WhyAliasOf — значение взято под неканоническим написанием.
func WhyAliasOf(canon string) string { return "alias_of:" + canon }

// Event — одно событие трассы.
//
// Поля raw/val/path могут быть null, поэтому это interface{}, а не string:
// различие «пустая строка» и «значения не было» в сверке значимо.
type Event struct {
	N      int
	Stage  string
	Mapper string
	Entry  string
	Src    string
	Raw    interface{}
	Val    interface{}
	Path   interface{}
	Act    string
	Why    string
}

// Trace — коллектор событий. Нулевой указатель — выключенная трасса:
// все методы на nil безопасны и не делают ничего.
type Trace struct {
	events []Event
	// order — порядок ключей тела (body.order) для канонической
	// сериализации итогового события.
	order []string
}

// NewTrace включает сбор трассы. bodyOrder — порядок ключей тела из
// реестра; по нему сериализуется итоговое событие.
func NewTrace(bodyOrder []string) *Trace {
	return &Trace{order: append([]string{}, bodyOrder...)}
}

// Enabled сообщает, собирается ли трасса.
func (t *Trace) Enabled() bool { return t != nil }

// Add записывает событие. Номер проставляется коллектором: он обязан
// отражать порядок ИСПОЛНЕНИЯ, а не порядок вызова из разных мест.
func (t *Trace) Add(e Event) {
	if t == nil {
		return
	}
	e.N = len(t.events) + 1
	if e.Src == "" {
		e.Src = "-"
	}
	if e.Why == "" {
		e.Why = WhyNone
	}
	t.events = append(t.events, e)
}

// Events — собранные события.
func (t *Trace) Events() []Event {
	if t == nil {
		return nil
	}
	return t.events
}

// Bytes сериализует трассу в JSON Lines по канону приложения «Трасса».
func (t *Trace) Bytes() []byte {
	if t == nil {
		return nil
	}
	var buf bytes.Buffer
	for i := range t.events {
		writeEvent(&buf, &t.events[i], t.order)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// String — трасса текстом.
func (t *Trace) String() string {
	if t == nil {
		return ""
	}
	return string(t.Bytes())
}

// writeEvent пишет одно событие с ключами в ФИКСИРОВАННОМ порядке.
//
// Ручная сборка, а не json.Marshal структуры: Marshal пишет поля в порядке
// объявления, но молча экранирует HTML и не умеет упорядочивать вложенные
// карты по body.order. Оба отличия дали бы ложные расхождения при сверке.
func writeEvent(buf *bytes.Buffer, e *Event, order []string) {
	buf.WriteString(`{"n":`)
	buf.WriteString(strconv.Itoa(e.N))
	writeKeyString(buf, "stage", e.Stage)
	writeKeyString(buf, "mapper", e.Mapper)
	writeKeyString(buf, "entry", e.Entry)
	writeKeyString(buf, "src", e.Src)
	writeKeyValue(buf, "raw", e.Raw, order)
	writeKeyValue(buf, "val", e.Val, order)
	writeKeyValue(buf, "path", e.Path, order)
	writeKeyString(buf, "act", e.Act)
	writeKeyString(buf, "why", e.Why)
	buf.WriteByte('}')
}

func writeKeyString(buf *bytes.Buffer, key, val string) {
	buf.WriteString(`,"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	writeJSONString(buf, val)
}

func writeKeyValue(buf *bytes.Buffer, key string, val interface{}, order []string) {
	buf.WriteString(`,"`)
	buf.WriteString(key)
	buf.WriteString(`":`)
	WriteCanonicalJSON(buf, val, order)
}

// WriteCanonicalJSON пишет значение по канону сверки:
//
//   - без пробелов;
//   - ключи карт — по body.order, остальные лексикографически;
//   - числа без экспоненты;
//   - не-ASCII как есть (UTF-8), \u только для управляющих;
//   - БЕЗ HTML-экранирования < > &.
func WriteCanonicalJSON(buf *bytes.Buffer, v interface{}, order []string) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case string:
		writeJSONString(buf, t)
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case float64:
		buf.WriteString(formatNumber(t))
	case []interface{}:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			WriteCanonicalJSON(buf, item, order)
		}
		buf.WriteByte(']')
	case []string:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, item)
		}
		buf.WriteByte(']')
	case map[string]interface{}:
		buf.WriteByte('{')
		for i, k := range orderedKeys(t, order) {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			WriteCanonicalJSON(buf, t[k], order)
		}
		buf.WriteByte('}')
	default:
		// Незнакомый тип не должен молча стать мусором: отдаём его через
		// обычный Marshal, но без HTML-экранирования.
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(t)
		// Encode дописывает \n — снимаем.
		if b := buf.Bytes(); len(b) > 0 && b[len(b)-1] == '\n' {
			buf.Truncate(buf.Len() - 1)
		}
	}
}

// orderedKeys — ключи карты: сперва перечисленные в body.order (в его
// порядке), затем остальные лексикографически.
//
// Свободные объекты (headers и подобные) в body.order не входят и потому
// упорядочиваются лексикографически — как и требует норма.
func orderedKeys(m map[string]interface{}, order []string) []string {
	out := make([]string, 0, len(m))
	seen := make(map[string]bool, len(m))
	for _, k := range order {
		if _, ok := m[k]; ok && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(m))
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// formatNumber печатает число БЕЗ экспоненты.
//
// JSON-разбор отдаёт любое число как float64, и strconv по умолчанию
// напечатал бы 1e+06 — у Dart вывод был бы другим, и сверка падала бы на
// числах, которые равны.
func formatNumber(f float64) string {
	if f == float64(int64(f)) && f < 1e15 && f > -1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// writeJSONString пишет строку по канону: не-ASCII как есть, \u только для
// управляющих символов, без HTML-экранирования.
func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(`\u`)
				const hex = "0123456789abcdef"
				buf.WriteByte('0')
				buf.WriteByte('0')
				buf.WriteByte(hex[(r>>4)&0xf])
				buf.WriteByte(hex[r&0xf])
				continue
			}
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
}

// ResultEvent — итоговое событие: тело, метка, источник тела.
//
// Кладётся ПОСЛЕДНЕЙ строкой трассы (норма приложения «Трасса»).
func (t *Trace) ResultEvent(mapper string, body map[string]interface{}, label, bodySource string) {
	if t == nil {
		return
	}
	t.Add(Event{
		Stage:  StageResult,
		Mapper: mapper,
		Entry:  "-",
		Src:    "-",
		Raw:    nil,
		Val: map[string]interface{}{
			"body":        body,
			"label":       label,
			"body_source": bodySource,
		},
		Path: nil,
		Act:  ActKeep,
		Why:  WhyNone,
	})
}

// TrimTrailingNewline снимает финальный перевод строки — удобно тесту,
// сравнивающему трассу с golden-файлом.
func TrimTrailingNewline(s string) string { return strings.TrimRight(s, "\n") }
