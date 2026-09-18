package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// mdCell готовит значение к ячейке таблицы: вертикальная черта ломает разметку,
// перевод строки — тоже.
func mdCell(s string) string {
	if s == "" {
		return "—"
	}
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// mdText — значение в обычной строке списка. В отличие от ячейки таблицы,
// вертикальную черту здесь экранировать не надо: она ничего не ломает.
func mdText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// aliasName — имя алиаса без пояснения. Реестр местами пишет рядом с именем
// человеческую заметку в скобках или после пробела («packetencoding (любой
// регистр — …)», «sni (fallback)»). Документация английская, и заметка в ней
// не нужна — нужно само имя, под которым параметр встречается в подписках.
//
// Отдельный случай — запись, которая именем не является вовсе, а описывает
// форму фразой («the '?ed=N' suffix of path»). Обрезка по первому пробелу
// давала из неё алиас `the`; такая запись отбрасывается целиком.
func aliasName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " ("); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if !looksLikeParamName(s) {
		return ""
	}
	return s
}

// looksLikeParamName — годится ли строка на имя параметра. Имена параметров в
// подписках — идентификаторы: буквы, цифры, `-`, `_`, `.`; всё прочее (кавычка,
// артикль, знак препинания) означает, что это фраза, а не имя.
func looksLikeParamName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}
	// Английское слово-артикль в списке алиасов — остаток обрезанной фразы.
	switch strings.ToLower(s) {
	case "the", "a", "an", "any", "same", "as", "or":
		return false
	}
	return true
}

// aliasNames чистит список алиасов и выкидывает пустые.
func aliasNames(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if n := aliasName(it); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// list — пункт списка с вложенными строками-атрибутами. Заменяет широкие
// таблицы: девять колонок на GitHub уезжают за экран, и описание поля
// оказывается там, куда не доскроллить.
type list struct {
	b *strings.Builder

	// anchors — ставить ли якорь у каждого поля тела. Якоря нужны и на
	// странице схемы (туда ведут ссылки «Maps to» из словаря ссылки), и на
	// странице общей суб-схемы (туда ведёт словарь кодов). Уникальны они в
	// пределах одного документа, а страницы разные.
	anchors bool

	// byPath — обратный индекс «поле тела → параметры ссылки, которые сюда
	// кладут». Пустой на страницах, где словаря ссылки нет.
	byPath map[string][]linkParam

	// scheme — схема, чью страницу мы печатаем; пусто на странице общей
	// суб-схемы. Список чужих схем в `allowed_for`/`forbidden_for` осмыслен
	// только там, где схема заранее не известна.
	scheme string
}

// setByLink — параметры ссылки, кладущие значение в это поле, ссылками на их
// пункты выше по странице.
func (l *list) setByLink(path string) string {
	if len(l.byPath) == 0 {
		return ""
	}
	items := l.byPath[path]
	if len(items) == 0 {
		return ""
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if seen[it.name] {
			continue
		}
		seen[it.name] = true
		out = append(out, "[`"+it.name+"`](#"+it.anchor+")")
	}
	return strings.Join(out, ", ")
}

// item открывает пункт: заголовок с именем поля и его описанием.
func (l *list) item(head, desc string) {
	l.b.WriteString("- " + head)
	if d := mdText(desc); d != "" {
		l.b.WriteString(" — " + d)
	}
	l.b.WriteString("\n")
}

// attr — вложенная строка-атрибут. Пустые не выводятся вовсе: строка
// «Default: — · Required: —» не несёт ничего, кроме шума.
func (l *list) attr(parts ...string) {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return
	}
	l.b.WriteString("  - " + mdText(strings.Join(kept, " · ")) + "\n")
}

func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + s + "`"
}

func codeList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it == "" {
			out = append(out, "`\"\"`")
			continue
		}
		out = append(out, code(it))
	}
	return strings.Join(out, ", ")
}

// scalar печатает значение реестра (default, values, on_invalid.value) так,
// как оно лежит в JSON: пустая строка видна явно, число без экспоненты.
func scalar(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		if t == "" {
			return "`\"\"`"
		}
		return code(t)
	case bool:
		return code(strconv.FormatBool(t))
	case float64:
		if t == float64(int64(t)) {
			return code(strconv.FormatInt(int64(t), 10))
		}
		return code(strconv.FormatFloat(t, 'g', -1, 64))
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, scalar(item))
		}
		return strings.Join(parts, ", ")
	case map[string]interface{}:
		// Составное значение (лимит-объект) печатается как JSON, а не
		// Go-шным map[...] — ключи сортируются самим кодировщиком.
		enc, err := json.Marshal(t)
		if err != nil {
			return code(fmt.Sprint(v))
		}
		return code(string(enc))
	}
	return code(fmt.Sprint(v))
}

func scalarList(items []interface{}) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, scalar(it))
	}
	return strings.Join(parts, ", ")
}

// warnLink — ссылка на код в warnings.md. Якорь ставится генератором там же,
// поэтому имя якоря = сам код.
func warnLink(code, prefix string) string {
	if code == "" {
		return ""
	}
	return "[`" + code + "`](" + prefix + "warnings.md#" + code + ")"
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// table собирает таблицу markdown; пустая — не печатается вовсе.
type table struct {
	head []string
	rows [][]string
}

func (t *table) add(cells ...string) {
	row := make([]string, len(cells))
	for i, c := range cells {
		row[i] = mdCell(c)
	}
	t.rows = append(t.rows, row)
}

func (t *table) render(b *strings.Builder) {
	if len(t.rows) == 0 {
		return
	}
	b.WriteString("| " + strings.Join(t.head, " | ") + " |\n")
	sep := make([]string, len(t.head))
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString("|" + strings.Join(sep, "|") + "|\n")
	for _, r := range t.rows {
		b.WriteString("| " + strings.Join(r, " | ") + " |\n")
	}
	b.WriteString("\n")
}
