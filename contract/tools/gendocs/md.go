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
