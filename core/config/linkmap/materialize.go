package linkmap

import (
	"strconv"
	"strings"
)

// MaterializeContext переносит значения ОТ ВЫЗЫВАЮЩЕГО (источник
// `context.<путь>`, контракт 1.1.63) в САМ ini-текст — по записям секции, а
// не кодом распаковщика (контракт 1.1.72).
//
// Зачем текст, а не тело: распаковщик контейнера отдаёт узлу происхождение, и
// оно обязано быть самодостаточным — пересборка узла из origin.raw идёт уже
// без контейнера, и всё, что контейнер знал о тексте, к тому моменту должно
// лежать в тексте. Поэтому правила `context.*` исполняются ОДИН раз, здесь,
// а тело потом строится из готового текста обычным путём без контекста.
//
// Два общих приёма, оба — из объявлений таблицы:
//
//   - запись с `substitute` и источником `ini.<Секция>.<Ключ>`: каждая строка
//     этого ключа в секции получает подставленное значение; неразрешённый
//     плейсхолдер снимается, строка без единого элемента удаляется;
//   - запись, чьи источники — только `context.*`, с путём `maps_to` и
//     выполненным `when`: если у того же пути есть запись с источником
//     `ini.<Секция>.<Ключ>` и этого ключа в тексте нет, строка
//     `<Ключ> = <значение>` вставляется сразу за заголовком секции. Явный
//     ключ в тексте приоритетнее — ровно как при разборе, где запись из ini
//     объявлена раньше.
//
// Текст не ini-формы (ссылка, обёрнутый пейлоад) и пустой контекст
// возвращаются как есть: дописывать строку в base64 нечем и незачем.
func MaterializeContext(plan *Plan, text string, context interface{}) string {
	if plan == nil || plan.Mapper == nil || context == nil {
		return text
	}
	space, form, err := UnwrapURI(plan, text)
	if err != nil || form.Space != "ini" || len(form.Decode) > 0 {
		return text
	}
	space.SetContext(context)
	st := &execState{
		plan:       plan,
		space:      space,
		form:       form,
		writtenBy:  map[string]*writeMark{},
		schemeVals: map[string]interface{}{},
		res:        &Result{Body: map[string]interface{}{}},
	}

	entries := make([]Entry, 0, len(plan.Selectors)+len(plan.Rest))
	entries = append(entries, plan.Selectors...)
	entries = append(entries, plan.Rest...)

	prefixes := plan.Mapper.IniDialect.Prefixes()
	out := text
	for i := range entries {
		p := entries[i].Param
		if p == nil {
			continue
		}
		names := p.Source.ForForm(form.ID)
		if p.Substitute != nil {
			for _, name := range names {
				if section, key, ok := iniSourceKey(name); ok {
					out = rewriteINIKey(out, prefixes, section, key, func(v string) (string, bool) {
						return st.substitute(p.Substitute, v)
					})
				}
			}
			continue
		}
		if !allContextSources(names) || p.MapsTo == nil || p.MapsTo.Path == "" {
			continue
		}
		st.curParam = p
		if !st.whenHolds(p.When) {
			continue
		}
		val, _, found := st.lookupSource(p)
		val = strings.TrimSpace(val)
		if !found || val == "" {
			continue
		}
		if p.Type == "int" {
			n, convErr := strconv.Atoi(val)
			if convErr != nil {
				continue
			}
			val = strconv.Itoa(n)
		}
		section, key, ok := iniTargetOf(entries, p.MapsTo.Path, form.ID)
		if !ok {
			continue
		}
		if _, has := space.Lookup("ini." + section + "." + key); has {
			continue
		}
		out = insertINIKey(out, section, key+" = "+val)
	}
	return out
}

// iniSourceKey разбирает имя источника `ini.<Секция>.<Ключ>`.
func iniSourceKey(name string) (string, string, bool) {
	if !strings.HasPrefix(name, "ini.") || strings.HasPrefix(name, "ini.$") {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, "ini.")
	idx := strings.Index(rest, ".")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// allContextSources — все источники записи лежат в слое `context.*`.
func allContextSources(names []string) bool {
	if len(names) == 0 {
		return false
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "context.") {
			return false
		}
	}
	return true
}

// iniTargetOf — ключ ini, которым текст сам задаёт путь тела `path`: первая
// запись секции с этим `maps_to` и источником `ini.<Секция>.<Ключ>`.
func iniTargetOf(entries []Entry, path, formID string) (string, string, bool) {
	for i := range entries {
		q := entries[i].Param
		if q == nil || q.MapsTo == nil || q.MapsTo.Path != path {
			continue
		}
		for _, name := range q.Source.ForForm(formID) {
			if section, key, ok := iniSourceKey(name); ok {
				return section, key, true
			}
		}
	}
	return "", "", false
}

// splitINILines режет текст на строки, запоминая, был ли перевод строки CRLF.
func splitINILines(text string) ([]string, string) {
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), nl
}

// iniHeader — имя секции строки-заголовка в нижнем регистре.
func iniHeader(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "[") {
		return "", false
	}
	return strings.ToLower(strings.Trim(t, "[]")), true
}

// rewriteINIKey пропускает значение каждой строки `key` секции `section`
// через fn: новое значение пишется на место старого (имя ключа и отступ —
// как у автора), false от fn удаляет строку. Без изменений текст
// возвращается байт в байт.
func rewriteINIKey(text string, prefixes []string, section, key string, fn func(string) (string, bool)) string {
	lines, nl := splitINILines(text)
	out := make([]string, 0, len(lines))
	current := ""
	changed := false
	for _, line := range lines {
		if name, ok := iniHeader(line); ok {
			current = name
			out = append(out, line)
			continue
		}
		t := strings.TrimSpace(line)
		idx := strings.Index(line, "=")
		if current != strings.ToLower(section) || t == "" || hasAnyPrefix(t, prefixes) || idx < 0 ||
			!strings.EqualFold(strings.TrimSpace(line[:idx]), key) {
			out = append(out, line)
			continue
		}
		old := strings.TrimSpace(line[idx+1:])
		nv, keep := fn(old)
		if !keep {
			changed = true
			continue
		}
		if nv != old {
			changed = true
			line = strings.TrimRight(line[:idx], " \t") + " = " + nv
		}
		out = append(out, line)
	}
	if !changed {
		return text
	}
	return strings.Join(out, nl)
}

// insertINIKey вставляет строку сразу за ПЕРВЫМ заголовком секции.
func insertINIKey(text, section, line string) string {
	lines, nl := splitINILines(text)
	for i, l := range lines {
		if name, ok := iniHeader(l); ok && name == strings.ToLower(section) {
			out := make([]string, 0, len(lines)+1)
			out = append(out, lines[:i+1]...)
			out = append(out, line)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, nl)
		}
	}
	return text
}
