package main

import (
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Файл держит три последних раздела страницы схемы: «Diagnosed problems»,
// «Replacements» и «Degradation». Все три выводятся ИЗ ПРАВИЛ реестра, а не из
// отдельного текстового поля: свободный список `degrade[]` из реестра снят
// именно потому, что расходился с правилами, по которым конвейер работает на
// самом деле.

// conditionPhrase — английская запись условия применимости правила значения
// (`when.any_set`).
//
// Условие называет РОД узла набором полей, и печатать здесь все два десятка
// имён значило бы утопить правило в списке: человек читает страницу, чтобы
// понять, к нему ли это относится, а не чтобы сверить набор. Поэтому — первые
// несколько имён и «and N more», а полный набор виден в самом реестре.
func conditionPhrase(c *registry.Condition) string {
	if c == nil {
		return ""
	}
	if len(c.AnySet) == 0 {
		return valuesPhrase(c.Values)
	}
	const shown = 3
	names := c.AnySet
	head := names
	tail := 0
	if len(names) > shown {
		head, tail = names[:shown], len(names)-shown
	}
	out := " when any of " + codeList(head) + " is set"
	if tail > 0 {
		out += " (and " + strconv.Itoa(tail) + " more)"
	}
	if v := valuesPhrase(c.Values); v != "" {
		out += "," + v
	}
	return out
}

// valuesPhrase — предикаты условия по значению путей (контракт 1.1.56):
// «when `tls.reality.enabled` is true», операторы in / not_in — списком.
func valuesPhrase(values map[string]interface{}) string {
	if len(values) == 0 {
		return ""
	}
	paths := make([]string, 0, len(values))
	for p := range values {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		want := values[p]
		if op, ok := want.(map[string]interface{}); ok {
			if list, ok := op["in"].([]interface{}); ok {
				parts = append(parts, code(p)+" is one of "+scalarList(list))
				continue
			}
			if list, ok := op["not_in"].([]interface{}); ok {
				parts = append(parts, code(p)+" is none of "+scalarList(list))
				continue
			}
			continue
		}
		parts = append(parts, code(p)+" is "+scalar(want))
	}
	if len(parts) == 0 {
		return ""
	}
	return " when " + strings.Join(parts, " and ")
}

// maxWhenPhrase — английская запись условного потолка: что заменяется, когда
// и с каким кодом.
func maxWhenPhrase(mw *registry.MaxWhen, linkPrefix string) string {
	if mw == nil {
		return ""
	}
	out := "Above " + scalar(mw.Max) + ": replaced with " + scalar(mw.Max) +
		conditionPhrase(mw.When)
	if mw.Code != "" {
		out += " → " + warnLink(mw.Code, linkPrefix)
	}
	if len(mw.ExceptSources) > 0 {
		// Исключение по входу — не деталь реализации, а то, что человек
		// увидит как разное поведение на разных импортах. Молчать о нём
		// нельзя: узел из подписки и тот же узел JSON-ом ведут себя по-разному.
		out += ". From " + codeList(mw.ExceptSources) +
			": kept as written, with a note"
		if mw.NoteCode != "" {
			out += " → " + warnLink(mw.NoteCode, linkPrefix)
		}
	}
	return out
}

// lowerFirst опускает первую букву фразы: одна и та же формулировка встаёт и
// самостоятельным пунктом («Above 1280: …»), и продолжением строки после тире.
func lowerFirst(s string) string {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return s
	}
	return string(s[0]-'A'+'a') + s[1:]
}

// renderSchemeWarnings — раздел «Diagnosed problems».
//
// Раздел обязан быть самодостаточным: человек открыл страницу из-за одного
// узла, и он не должен обходить четыре общие суб-схемы, чтобы узнать, что
// именно случилось. Поэтому ссылочные поля (`tls`, `transport`, `multiplex`,
// `dialer`) РАЗВОРАЧИВАЮТСЯ: коды суб-схемы попадают в список этой схемы с
// полным путём поля.
//
// Учитываются `allowed_for`/`forbidden_for`: поле, запрещённое этой схеме,
// даёт код запрета (`tls_field_unsupported_naive` у naive), а не свои обычные
// коды — иначе страница naive обещала бы диагностику, которой там нет.
func renderSchemeWarnings(b *strings.Builder, raw *rawRegistry, scheme string, body *registry.BodySchema) {
	b.WriteString("## Diagnosed problems\n\n")
	if body == nil {
		b.WriteString("No body schema in the registry, so nothing is diagnosed on the fields.\n\n")
		return
	}

	found := map[string][]usage{}
	for _, name := range body.Order {
		f := body.Fields[name]
		if f == nil {
			continue
		}
		collectSchemeUsages(found, scheme, name, f)
	}

	if len(found) == 0 {
		b.WriteString("No field of this scheme raises a code of its own.\n\n")
		return
	}

	b.WriteString("Every code that can be raised on a node of this scheme, including the ones " +
		"coming from the shared TLS, transport, multiplex and dialer sub-schemas. " +
		"Follow a code for what it means and what to do about it.\n\n")

	codes := make([]string, 0, len(found))
	for c := range found {
		codes = append(codes, c)
	}
	sort.Strings(codes)

	l := &list{b: b}
	for _, c := range codes {
		l.item(warnLink(c, "../"), "")
		for _, u := range found[c] {
			// Путь — якорной ссылкой на само поле выше по странице: код без
			// поля не говорит, что именно чинить.
			line := "[`" + u.path + "`](#" + bodyAnchor(u.path) + ") — " + u.via
			if u.action != "" {
				line += " → " + u.action
			}
			l.attr(line)
		}
	}
	b.WriteString("\n")
}

// collectSchemeUsages обходит поле схемы вглубь и складывает коды с путями.
//
// Обход свой, а не buildUsageIndex: тот строит обратный индекс по СЫРЫМ файлам
// (иначе поле суб-схемы попало бы в него по разу на каждую из 16 схем), а
// здесь нужна ровно противоположная вещь — разрешённое тело ОДНОЙ схемы, где
// суб-схема уже подставлена и видно, какие её поля этой схеме запрещены.
func collectSchemeUsages(out map[string][]usage, scheme, path string, f *registry.Field) {
	add := func(code, via, action string) {
		if code == "" {
			return
		}
		out[code] = append(out[code], usage{scheme: scheme, path: path, via: via, action: action})
	}

	// Поле, запрещённое этой схеме, снимается целиком: собственные правила
	// значения к нему уже не применяются, и показывать их — врать.
	if len(f.ForbiddenFor) > 0 && contains(f.ForbiddenFor, scheme) {
		add(forbiddenCode(f, scheme), "not supported by `"+scheme+"`", actionRemoved)
		return
	}
	if len(f.AllowedFor) > 0 && !contains(f.AllowedFor, scheme) {
		add(forbiddenCode(f, scheme), "not supported by `"+scheme+"`", actionRemoved)
		return
	}

	if f.OnInvalid != nil {
		add(f.OnInvalid.Code, "the value does not fit the field", onInvalidAction(f.OnInvalid))
		if f.OnInvalid.ElseCode != "" {
			add(f.OnInvalid.ElseCode, "an object arrived without a usable `"+f.OnInvalid.Key+"` member", actionRemoved)
		}
	}
	for _, a := range f.Advisory {
		if len(a.Except) > 0 {
			add(a.Code, "the value is anything except "+scalarList(a.Except), actionKept)
			continue
		}
		add(a.Code, "the value is "+scalarList(a.Values), actionKept)
	}
	if dw := f.DefaultWhen; dw != nil && dw.Absent && dw.Code != "" {
		add(dw.Code, "the field is absent", "filled in with "+scalar(dw.Value))
	}
	if mw := f.MaxWhen; mw != nil {
		add(mw.Code, "the value is above "+scalar(mw.Max)+conditionPhrase(mw.When),
			"replaced with "+scalar(mw.Max))
		if mw.NoteCode != "" {
			add(mw.NoteCode, "the value is above "+scalar(mw.Max)+conditionPhrase(mw.When)+
				", but the body came from "+codeList(mw.ExceptSources), actionKept)
		}
	}
	if f.NormalizeCode != "" {
		add(f.NormalizeCode, "the value had to be cleaned up ("+f.Normalize+")", "value cleaned up")
	}
	for _, c := range f.Conflicts {
		add(codeOr(c.Code, "field_conflict"), "conflicts with `"+c.With+"`"+unlessPhrase(c), actionRemoved)
	}
	for _, rq := range f.Requires {
		if rq.Set != nil {
			add(codeOr(rq.Code, "field_requires"), "set without `"+rq.Path+"`"+unlessPhrase(rq),
				"`"+rq.Path+"` filled in with "+scalar(rq.Set))
			continue
		}
		add(codeOr(rq.Code, "field_requires"), "set without `"+rq.Path+"`"+unlessPhrase(rq), actionRemoved)
	}
	if cw := f.CoerceWhen; cw != nil {
		add(cw.Code, "the value is "+scalarList(cw.Values)+conditionPhrase(cw.When), "replaced with "+scalar(cw.Value))
	}
	if oh := f.OnHopRequired; oh != nil {
		add(oh.Code, "a hop at position 2 or later requires this path", "not stripped")
	}
	if fi := f.ItemForbidden; fi != nil {
		add(fi.Code, "a list item is "+scalarList(fi.Values), "item removed")
	}
	if f.Required && f.OnInvalid == nil {
		add("field_missing", "required and missing", actionNodeDropped)
	}
	if f.Code != "" && len(f.ForbiddenFor) == 0 && len(f.AllowedFor) == 0 {
		add(f.Code, "the field is present", "")
	}

	// Варианты (транспорты) и вложенные объекты — тем же порядком, с полным
	// путём: именно он делает страницу самодостаточной.
	if len(f.Variants) > 0 {
		names := make([]string, 0, len(f.Variants))
		for v := range f.Variants {
			names = append(names, v)
		}
		sort.Strings(names)
		for _, v := range names {
			collectNestedUsages(out, scheme, joinPath(path, v), f.Variants[v])
		}
		return
	}
	collectNestedUsages(out, scheme, path, f)
}

func collectNestedUsages(out map[string][]usage, scheme, path string, f *registry.Field) {
	inner := f
	if len(inner.Fields) == 0 && inner.Items != nil {
		inner = inner.Items
	}
	if len(inner.Fields) == 0 {
		return
	}
	names := inner.Order
	if len(names) == 0 {
		for k := range inner.Fields {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		if child := inner.Fields[name]; child != nil {
			collectSchemeUsages(out, scheme, joinPath(path, name), child)
		}
	}
}

// renderReplacements — раздел «Replacements»: всё, что конвейер ПОДМЕНЯЕТ, а не
// снимает. Четыре источника подмен, и все четыре человеку выглядят одинаково
// («я написал одно, в теле другое»), поэтому собраны в один раздел:
//
//   - алиасы имён — тот же смысл, другое написание;
//   - normalize — то же значение, приведённое к форме ядра;
//   - coerce — негодное значение заменено годным;
//   - default_when — отсутствующее значение дописано;
//   - переводы маппера — структурные решения, которые санитайзер принять не
//     может в принципе (секция `mapper` реестра).
func renderReplacements(b *strings.Builder, raw *rawRegistry, scheme string, p *rawProtocol) {
	b.WriteString("## Replacements\n\n")

	var (
		aliasRows   []string
		valueRows   []string
		mapperItems []mapperRule
	)

	if p != nil && p.URI != nil {
		for _, name := range p.URI.queryOrder {
			param := p.URI.Query[name]
			if param == nil {
				continue
			}
			if names := aliasNames(param.Aliases); len(names) > 0 {
				aliasRows = append(aliasRows, codeList(names)+" → `"+name+"`")
			}
		}
	}

	if body, ok := schemeBody(raw, scheme); ok {
		collectReplacements(&valueRows, "", body.Order, body.Fields)
	}

	if raw != nil {
		mapperItems = raw.mapperFor(scheme)
	}

	if len(aliasRows) == 0 && len(valueRows) == 0 && len(mapperItems) == 0 {
		b.WriteString("Nothing is silently replaced for this scheme.\n\n")
		return
	}

	if len(aliasRows) > 0 {
		b.WriteString("**Link parameter names.** The same parameter is spelled differently by " +
			"different clients; the left spelling is read as the right one.\n\n")
		l := &list{b: b}
		for _, r := range aliasRows {
			l.item(r, "")
		}
		b.WriteString("\n")
	}

	if len(valueRows) > 0 {
		b.WriteString("**Values.** What the sanitizer does to a value before it reaches the " +
			"node body.\n\n")
		l := &list{b: b}
		for _, r := range valueRows {
			l.item(r, "")
		}
		b.WriteString("\n")
	}

	if len(mapperItems) > 0 {
		b.WriteString("**Structural translations.** Decisions taken while the link is being " +
			"read, before any value is judged: whether a block exists at all, where a field " +
			"comes from, or how one input becomes several fields. The sanitizer sees a " +
			"finished body and cannot take them.\n\n")
		l := &list{b: b}
		for _, m := range mapperItems {
			head := ""
			switch {
			case m.From != "" && m.To != "":
				head = mapperSide(m.From) + " → " + mapperSide(m.To)
			case m.From != "":
				head = mapperSide(m.From)
			case m.To != "":
				head = mapperSide(m.To)
			default:
				head = code(m.ID)
			}
			l.item(head, m.DescEn)
			attr := "Kind: " + code(m.Kind)
			if m.Code != "" {
				attr += " · " + warnLink(m.Code, "../")
			}
			l.attr(attr)
		}
		b.WriteString("\n")
	}
}

// collectReplacements собирает подмены значений вглубь тела.
func collectReplacements(out *[]string, prefix string, order []string, fields map[string]*registry.Field) {
	names := order
	if len(names) == 0 {
		for k := range fields {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		f := fields[name]
		if f == nil {
			continue
		}
		path := joinPath(prefix, name)

		if f.Normalize != "" {
			line := "`" + path + "` — normalized: " + code(f.Normalize)
			if f.NormalizeCode != "" {
				line += " → " + warnLink(f.NormalizeCode, "../")
			}
			*out = append(*out, line)
		}
		if oi := f.OnInvalid; oi != nil && oi.Action == "coerce" {
			line := "`" + path + "` — an invalid value is replaced with " + scalar(oi.Value)
			if oi.Code != "" {
				line += " → " + warnLink(oi.Code, "../")
			}
			*out = append(*out, line)
		}
		if dw := f.DefaultWhen; dw != nil && dw.Absent {
			line := "`" + path + "` — when absent, filled in with " + scalar(dw.Value) +
				conditionPhrase(dw.When)
			if dw.Code != "" {
				line += " → " + warnLink(dw.Code, "../")
			}
			*out = append(*out, line)
		}
		if mw := f.MaxWhen; mw != nil {
			*out = append(*out, "`"+path+"` — "+lowerFirst(maxWhenPhrase(mw, "../")))
		}
		if cw := f.CoerceWhen; cw != nil {
			*out = append(*out, "`"+path+"` — "+scalarList(cw.Values)+" is replaced with "+scalar(cw.Value)+
				conditionPhrase(cw.When)+" → "+warnLink(cw.Code, "../"))
		}
		for _, rq := range f.Requires {
			if rq.Set != nil {
				*out = append(*out, "`"+path+"` — without `"+rq.Path+"`, it is filled in with "+scalar(rq.Set)+
					" → "+warnLink(codeOr(rq.Code, "field_requires"), "../"))
			}
		}
		if names := fieldAliasNames(f.Aliases); len(names) > 0 {
			*out = append(*out, "`"+path+"` — also read from "+codeList(names))
		}

		if len(f.Variants) > 0 {
			vnames := make([]string, 0, len(f.Variants))
			for v := range f.Variants {
				vnames = append(vnames, v)
			}
			sort.Strings(vnames)
			for _, v := range vnames {
				if vf := f.Variants[v]; vf != nil {
					collectReplacements(out, joinPath(path, v), vf.Order, vf.Fields)
				}
			}
			continue
		}
		inner := f
		if len(inner.Fields) == 0 && inner.Items != nil {
			inner = inner.Items
		}
		if len(inner.Fields) > 0 {
			collectReplacements(out, path, inner.Order, inner.Fields)
		}
	}
}

// renderDegradation — раздел «Degradation»: чем кончается негодное значение,
// сгруппированно по ИСХОДУ, а не по полю.
//
// Раздел выводится из тех же правил, что и разделы выше; отдельного списка в
// реестре у него нет и быть не должно — свободный `degrade[]` расходился с
// правилами, по которым конвейер работает.
func renderDegradation(b *strings.Builder, scheme string, body *registry.BodySchema) {
	b.WriteString("## Degradation\n\n")
	if body == nil {
		b.WriteString("No body schema in the registry.\n\n")
		return
	}

	buckets := map[string][]string{}
	for _, name := range body.Order {
		if f := body.Fields[name]; f != nil {
			collectDegradation(buckets, scheme, name, f)
		}
	}

	// Порядок — по тяжести исхода: сперва то, из-за чего узла не будет вовсе.
	order := []struct{ key, head string }{
		{actionNodeDropped, "The node is dropped"},
		{actionRemoved, "The field is removed, the node lives on"},
		{"replaced", "The value is replaced, the node lives on"},
		{actionKept, "Kept as is, with a notice"},
		{"gated", "Left out when the running core is too old"},
	}

	wrote := false
	for _, o := range order {
		rows := buckets[o.key]
		if len(rows) == 0 {
			continue
		}
		sort.Strings(rows)
		b.WriteString("**" + o.head + "**\n\n")
		l := &list{b: b}
		for _, r := range rows {
			l.item(r, "")
		}
		b.WriteString("\n")
		wrote = true
	}
	if !wrote {
		b.WriteString("Nothing degrades on this scheme: every field is taken as it comes.\n\n")
	}
}

func collectDegradation(out map[string][]string, scheme, path string, f *registry.Field) {
	put := func(bucket, line string) {
		out[bucket] = append(out[bucket], line)
	}

	if len(f.ForbiddenFor) > 0 && contains(f.ForbiddenFor, scheme) {
		put(actionRemoved, "`"+path+"` — not supported by this protocol")
		return
	}
	if len(f.AllowedFor) > 0 && !contains(f.AllowedFor, scheme) {
		put(actionRemoved, "`"+path+"` — not supported by this protocol")
		return
	}

	if oi := f.OnInvalid; oi != nil {
		switch oi.Action {
		case "drop_node":
			put(actionNodeDropped, "`"+path+"` — invalid value")
		case "coerce":
			put("replaced", "`"+path+"` — invalid value becomes "+scalar(oi.Value))
		case "unwrap":
			put("replaced", "`"+path+"` — an object takes the value of its `"+oi.Key+"` member")
			put(actionRemoved, "`"+path+"` — an object without `"+oi.Key+"`, or another invalid value")
		default:
			put(actionRemoved, "`"+path+"` — invalid value")
		}
	}
	if f.Required && f.OnInvalid == nil {
		put(actionNodeDropped, "`"+path+"` — required and missing")
	}
	if len(f.Advisory) > 0 {
		put(actionKept, "`"+path+"` — accepted, but worth knowing about")
	}
	if len(f.Conflicts) > 0 || len(f.Requires) > 0 {
		put(actionRemoved, "`"+path+"` — conflicts with another field of the same node")
	}
	for _, rq := range f.Requires {
		if rq.Set != nil {
			put("replaced", "`"+rq.Path+"` — filled in with "+scalar(rq.Set)+" when `"+path+"` needs it")
		}
	}
	if cw := f.CoerceWhen; cw != nil {
		put("replaced", "`"+path+"` — "+scalarList(cw.Values)+" is replaced with "+scalar(cw.Value)+conditionPhrase(cw.When))
	}
	if dw := f.DefaultWhen; dw != nil && dw.Absent {
		put("replaced", "`"+path+"` — absent value is filled in with "+scalar(dw.Value)+
			conditionPhrase(dw.When))
	}
	if mw := f.MaxWhen; mw != nil {
		put("replaced", "`"+path+"` — a value above "+scalar(mw.Max)+
			" is replaced with "+scalar(mw.Max)+conditionPhrase(mw.When))
		if mw.NoteCode != "" {
			put(actionKept, "`"+path+"` — a value above "+scalar(mw.Max)+
				" coming from "+codeList(mw.ExceptSources)+" is kept, with a note")
		}
	}
	if g := bodyGate(f); g != "" {
		put("gated", "`"+path+"` — needs "+strings.TrimPrefix(g, "Only written when: "))
	}
	if a := f.OnCoreUnsupported; a != nil {
		put("gated", "`"+path+"` — on a core that lacks it the whole node is dropped ("+code(a.Code)+")")
	}
	if rf := f.RangeForm; rf != nil && rf.OnCoreUnsupported != nil {
		put("gated", "`"+path+"` as a range `N-M` — on a core that lacks it the whole node is dropped ("+code(rf.OnCoreUnsupported.Code)+")")
	}

	if len(f.Variants) > 0 {
		names := make([]string, 0, len(f.Variants))
		for v := range f.Variants {
			names = append(names, v)
		}
		sort.Strings(names)
		for _, v := range names {
			collectDegradationNested(out, scheme, joinPath(path, v), f.Variants[v])
		}
		return
	}
	collectDegradationNested(out, scheme, path, f)
}

func collectDegradationNested(out map[string][]string, scheme, path string, f *registry.Field) {
	inner := f
	if len(inner.Fields) == 0 && inner.Items != nil {
		inner = inner.Items
	}
	if len(inner.Fields) == 0 {
		return
	}
	names := inner.Order
	if len(names) == 0 {
		for k := range inner.Fields {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	for _, name := range names {
		if child := inner.Fields[name]; child != nil {
			collectDegradation(out, scheme, joinPath(path, name), child)
		}
	}
}

// schemeBody — разрешённое тело схемы, если оно у неё есть.
func schemeBody(raw *rawRegistry, scheme string) (*registry.BodySchema, bool) {
	if raw == nil || raw.reg == nil {
		return nil, false
	}
	body, ok := raw.reg.Body(scheme)
	if !ok || body == nil {
		return nil, false
	}
	return body, true
}

// mapperSide — сторона перевода (`from`/`to`) в готовом для markdown виде.
//
// Записи секции `mapper` — фразы, а не значения полей: часть из них уже несёт
// собственную разметку («`security=none`», «no `tls` block at all»). Обернуть
// такую фразу в кавычки ещё раз значит получить вложенные обратные кавычки,
// которые markdown не рисует. Оборачиваем только голый текст.
func mapperSide(s string) string {
	if strings.Contains(s, "`") {
		return s
	}
	return code(s)
}

// fieldAliasNames — алиасы поля тела. В реестре они записаны двумя формами:
// списком имён и одиночной строкой (у поля с единственным чужим написанием),
// поэтому тип в схеме открытый.
func fieldAliasNames(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return aliasNames([]string{t})
	case []string:
		return aliasNames(t)
	case []interface{}:
		items := make([]string, 0, len(t))
		for _, it := range t {
			if s, ok := it.(string); ok {
				items = append(items, s)
			}
		}
		return aliasNames(items)
	}
	return nil
}

func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

// forbiddenCode — код запрета поля для конкретной схемы. Тот же выбор, что
// делает санитайзер (core/config/nodeflow/sanitize.go forbiddenCode): у поля
// кроме общего `code` есть словарь `forbidden_codes`, потому что исход у
// разных схем разный — naive теряет настройку (warning), QUIC-протокол
// избавляется от бессмыслицы (info).
func forbiddenCode(f *registry.Field, scheme string) string {
	if c, ok := f.ForbiddenCodes[scheme]; ok && c != "" {
		return c
	}
	return codeOr(f.Code, "unknown_key")
}

func contains(items []string, s string) bool {
	for _, it := range items {
		if it == s {
			return true
		}
	}
	return false
}
