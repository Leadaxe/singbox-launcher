package linkmap

import (
	"strings"

	"singbox-launcher/core/config/registry"
)

// Файл исполняет УРОВЕНЬ ДОКУМЕНТА таблицы видов источника
// (contract/registry/source_kinds.json): detect → unwrap → redetect →
// elements. Ниже уровнем стоит exec.go, собирающий узел из одного элемента.
//
// Имён схем, протоколов и форматов здесь нет: и «что это за текст», и «как
// его разрезать» приезжают из таблицы. Единственное, что движок знает по
// имени, — РАСПАКОВЩИКИ (`unwrap`), и они переданы вызывающим извне: формат
// контейнера предикатом не выражается, но вызывается он по объявленному
// имени, а не веткой в сниффере.

// Unwrapper — распаковщик оболочки документа, зарегистрированный по имени
// (значение атрибута `unwrap`).
//
// Результат — один или несколько текстов: base64-обёртка даёт один,
// контейнерный экспорт — по тексту на контейнер. Ошибка означает «оболочка
// не снялась»: вид источника отвергается, и выбор продолжается со следующего
// кандидата.
type Unwrapper func(text string) ([]string, error)

// SourceElement — один элемент документа: то, из чего маппер собирает узел.
type SourceElement struct {
	// Text — текст элемента (строка ссылки, ini-текст). Пуст у элемента,
	// приехавшего значением JSON.
	Text string
	// Value — разобранное значение JSON; nil у текстового элемента.
	Value interface{}
	// InArray — имя массива, из которого взят элемент ("outbounds",
	// "endpoints"): различает уровень у sing-box и попадает в detect секции.
	InArray string
}

// SourceResult — исход классификации документа.
type SourceResult struct {
	// Kind — победивший вид источника. Пуст, если не совпало ничего и ветки
	// `default` в таблице нет.
	Kind registry.SourceKind
	// Text — текст, которым документ оказался ПОСЛЕ всех распаковок; именно
	// его судил победивший detect.
	Text string
	// Elements — элементы в порядке обхода.
	Elements []SourceElement
	// SkippedLines — сколько строк пропущено нарезкой "lines" (пустые и
	// комментарии): наблюдаемый выход, из него собирается отчёт разбора.
	SkippedLines int
	// UnwrapDepth — сколько оболочек снято.
	UnwrapDepth int
	// Matched — имена ВСЕХ сработавших видов на последнем детекте. Линтер
	// требует, чтобы на корпусе их было ровно один: два — тихое перекрытие.
	Matched []string
	// ByDefault — победила ветка «всё остальное».
	ByDefault bool
	// Recognized — вид найден (хотя бы default-веткой).
	Recognized bool
	// UnwrapFailed — имя распаковщика, который не справился либо не
	// зарегистрирован; вид из-за этого отвергнут. Для лога, а не для UI.
	UnwrapFailed string
}

// ClassifySource определяет вид источника и режет документ на элементы.
//
// Распаковка рекурсивна: подписка base64 внутри base64 раскрывается, пока
// не кончится `max_unwrap_depth`. Предел объявлен таблицей, а не кодом, и
// сегодняшнему поведению он не равен — у рукописного декодера предела нет
// вовсе (DELTAS D133-35).
func ClassifySource(set *registry.MapperSet, text string, unwrappers map[string]Unwrapper) SourceResult {
	res := SourceResult{Text: text}
	spec := set.SourceKinds()
	if spec == nil {
		return res
	}
	maxDepth := spec.MaxUnwrapDepth

	cur := text
	for depth := 0; ; depth++ {
		content := NewContent(cur)
		kind, sel := SelectSource(set, content)
		res.Text = cur
		res.Matched = sel.Matched
		res.ByDefault = sel.ByDefault
		if sel.Index < 0 {
			res.Recognized = false
			return res
		}
		res.Kind = kind
		res.Recognized = true

		if kind.Unwrap == "" {
			res.Elements, res.SkippedLines = extractElements(kind, content, nil)
			return res
		}

		// Оболочка есть — снимаем. Глубина считается по СНЯТЫМ оболочкам:
		// предел 2 означает «base64 внутри base64, дальше нет».
		if depth >= maxDepth && maxDepth > 0 {
			// Предел исчерпан: вид оставляем объявленным (человеку важно
			// знать, ЧЕМ текст оказался), но элементов не даём — иначе
			// глубина не ограничивала бы ничего.
			res.UnwrapFailed = kind.Unwrap
			return res
		}
		un, ok := unwrappers[kind.Unwrap]
		if !ok {
			res.UnwrapFailed = kind.Unwrap
			return res
		}
		texts, err := un(cur)
		if err != nil || len(texts) == 0 {
			res.UnwrapFailed = kind.Unwrap
			return res
		}

		if !kind.Redetect {
			// Вид элемента известен заранее: распаковщик обещает тексты
			// объявленного маппером вида, и судить их заново незачем.
			res.UnwrapDepth = depth + 1
			res.Elements, res.SkippedLines = extractElements(kind, content, texts)
			return res
		}

		// `requires_after_unwrap` — правдоподобие распакованного: без него
		// алфавит base64 ловил бы обычную строку ссылок без спецсимволов.
		joined := strings.Join(texts, "\n")
		if kind.RequiresAfterUnwrap != nil && !Matches(kind.RequiresAfterUnwrap, NewContent(joined)) {
			// Промежуточный слой ТОЙ ЖЕ оболочки: base64 внутри base64
			// распаковывается в чистый алфавит base64 — ни `://`, ни `{`,
			// ни `[Interface]` в нём нет, и правдоподобие проваливается,
			// хотя документа мы ещё просто не достигли. Отличает этот
			// случай от «обёртки не было» повторный детект: распакованное
			// снова совпало с предикатом той же оболочки. Продолжаем цикл —
			// сверху его сторожит `max_unwrap_depth`, ради которого предел
			// и объявлен таблицей (DELTAS D133-35).
			again, againSel := SelectSource(set, NewContent(joined))
			if kind.Redetect && againSel.Index >= 0 && again.SourceKind == kind.SourceKind {
				res.UnwrapDepth = depth + 1
				cur = joined
				continue
			}
			// Распакованное на документ не похоже — обёртки не было, и
			// текст судится как есть: ветка отвергается ЦЕЛИКОМ, а не
			// заменяет документ мусором.
			//
			// Победителем объявляется вид, выигравший ПОВТОРНЫЙ суд, а не
			// отвергнутая обёртка: её имя уехало бы в лог и в ожидания
			// корпуса, объявляя обёрткой текст, который ею не был. Вместе с
			// видом переезжают и Matched/ByDefault: иначе в них остаётся
			// след отвергнутой обёртки, и лог сообщает одно, а имя вида —
			// другое.
			alt, altSel, elems, skipped, ok := reselectWithout(set, kind, cur)
			if !ok {
				res.Recognized = false
				res.Kind = registry.SourceKind{}
				res.Matched = nil
				res.ByDefault = false
				return res
			}
			res.Kind = alt
			res.Matched = altSel.Matched
			res.ByDefault = altSel.ByDefault
			res.Elements, res.SkippedLines = elems, skipped
			return res
		}
		res.UnwrapDepth = depth + 1
		cur = joined
	}
}

// reselectWithout судит текст ЗАНОВО, исключив отвергнутый вид, и возвращает
// победителя вместе с его элементами.
//
// Нужен единственному случаю: обёртка опознана предикатом, но распакованное
// содержимое не прошло `requires_after_unwrap`, то есть обёртки не было.
// Текст обязан достаться следующему кандидату — и достаться ПОД ЕГО ИМЕНЕМ:
// вид источника читают человек в логе и ожидания корпуса, и назвать обёрткой
// текст, который ею не оказался, значит соврать обоим.
func reselectWithout(
	set *registry.MapperSet,
	skip registry.SourceKind,
	text string,
) (registry.SourceKind, SelectResult, []SourceElement, int, bool) {
	kinds := set.SourceKindsByPriority()
	cands := make([]Candidate, 0, len(kinds))
	rest := make([]registry.SourceKind, 0, len(kinds))
	for _, k := range kinds {
		if k.SourceKind == skip.SourceKind {
			continue
		}
		cands = append(cands, SourceCandidate{Kind: k})
		rest = append(rest, k)
	}
	content := NewContent(text)
	sel := Select(cands, content)
	if sel.Index < 0 {
		return registry.SourceKind{}, sel, nil, 0, false
	}
	won := rest[sel.Index]
	// Второй оболочки быть не может: отвергнутый вид исключён, а других
	// обёрток на один текст таблица не объявляет. Объявит — элементов не
	// будет, и раннер таблицы это поймает счётчиком.
	elems, skipped := extractElements(won, content, nil)
	return won, sel, elems, skipped, true
}

// extractElements исполняет выражение `elements`.
//
// Грамматика выражения (MAPPER_ENGINE §2): "$self" | "lines" | "texts" |
// путь с `[]` | несколько путей через "+". Разбирает его движок, но НАЗЫВАЕТ
// пути таблица: ни "outbounds", ни "endpoints" в коде не упомянуты.
func extractElements(kind registry.SourceKind, c *Content, texts []string) ([]SourceElement, int) {
	expr := strings.TrimSpace(kind.Elements)
	if expr == "" {
		return nil, 0
	}
	var out []SourceElement
	skipped := 0
	for _, term := range strings.Split(expr, "+") {
		term = strings.TrimSpace(term)
		switch term {
		case "":
			continue
		case "$self":
			if v := c.JSON(); v != nil {
				out = append(out, SourceElement{Text: c.Text(), Value: v})
				continue
			}
			out = append(out, SourceElement{Text: c.Text()})
		case "lines":
			lines, n := splitLines(c.Text(), kind.LineCommentPrefixes)
			skipped += n
			for _, line := range lines {
				out = append(out, SourceElement{Text: line})
			}
		case "texts":
			for _, t := range texts {
				if strings.TrimSpace(t) == "" {
					skipped++
					continue
				}
				out = append(out, SourceElement{Text: t})
			}
		default:
			out = append(out, collectPath(c.JSON(), term)...)
		}
	}
	return out, skipped
}

// splitLines режет текст на непустые не-комментарные строки, возвращая ещё и
// число пропущенных.
func splitLines(text string, commentPrefixes []string) ([]string, int) {
	var out []string
	skipped := 0
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			continue
		}
		if hasAnyPrefix(line, commentPrefixes) {
			skipped++
			continue
		}
		out = append(out, line)
	}
	return out, skipped
}

// collectPath собирает ВСЕ значения по пути с `[]`.
//
// Отличается от lookupPath, который отдаёт одно значение: нарезка обязана
// обойти каждый элемент каждого массива, а не первый. Имя последнего
// массива в пути становится InArray элемента — им detect секции отличает
// outbound от endpoint, не зная этих имён из кода.
func collectPath(v interface{}, path string) []SourceElement {
	if v == nil {
		return nil
	}
	arrPath, rest, split := cutArrayPath(path)
	if !split {
		found, ok := lookupPath(v, path)
		if !ok {
			return nil
		}
		return []SourceElement{{Value: found}}
	}
	node := v
	if arrPath != "" {
		found, ok := lookupPath(v, arrPath)
		if !ok {
			return nil
		}
		node = found
	}
	arr, ok := node.([]interface{})
	if !ok {
		return nil
	}
	name := lastPathSegment(arrPath)
	var out []SourceElement
	for _, elem := range arr {
		if rest == "" {
			if elem == nil {
				continue
			}
			out = append(out, SourceElement{Value: elem, InArray: name})
			continue
		}
		out = append(out, collectPath(elem, rest)...)
	}
	return out
}

// lastPathSegment — последний точечный сегмент пути ("a.b" → "b").
func lastPathSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}
