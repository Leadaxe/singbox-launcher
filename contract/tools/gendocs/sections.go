package main

import (
	"sort"
	"strings"

	"singbox-launcher/core/config/registry"
)

// Файл держит три последних раздела страницы схемы: «Diagnosed problems»,
// «Replacements» и «Degradation». Все три выводятся ИЗ ПРАВИЛ реестра, а не из
// отдельного текстового поля: свободный список `degrade[]` из реестра снят
// именно потому, что расходился с правилами, по которым конвейер работает на
// самом деле.

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
			line := "`" + u.path + "` — " + u.via
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
		add(codeOr(f.Code, "unknown_key"), "forbidden for `"+scheme+"`", actionRemoved)
		return
	}
	if len(f.AllowedFor) > 0 && !contains(f.AllowedFor, scheme) {
		add(codeOr(f.Code, "unknown_key"), "not allowed for `"+scheme+"`", actionRemoved)
		return
	}

	if f.OnInvalid != nil {
		add(f.OnInvalid.Code, "on_invalid: "+f.OnInvalid.Action, onInvalidAction(f.OnInvalid))
	}
	for _, a := range f.Advisory {
		if len(a.Except) > 0 {
			add(a.Code, "advisory except "+scalarList(a.Except), actionKept)
			continue
		}
		add(a.Code, "advisory "+scalarList(a.Values), actionKept)
	}
	if dw := f.DefaultWhen; dw != nil && dw.Absent && dw.Code != "" {
		add(dw.Code, "default_when absent", "filled in with "+scalar(dw.Value))
	}
	if f.NormalizeCode != "" {
		add(f.NormalizeCode, "normalize "+f.Normalize, "value cleaned up")
	}
	for _, c := range f.Conflicts {
		add(codeOr(c.Code, "field_conflict"), "conflicts with `"+c.With+"`", actionRemoved)
	}
	for _, rq := range f.Requires {
		add(codeOr(rq.Code, "field_requires"), "requires `"+rq.Path+"`", actionRemoved)
	}
	if fw := f.ForbiddenWhen; fw != nil {
		add(codeOr(fw.Code, "field_conflict"), "forbidden when `"+fw.Path+"` is set", actionRemoved)
	}
	if f.Required && f.OnInvalid == nil {
		add("field_missing", "required", actionNodeDropped)
	}
	if f.Code != "" && len(f.ForbiddenFor) == 0 && len(f.AllowedFor) == 0 {
		add(f.Code, "code", "")
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
			line := "`" + path + "` — when absent, filled in with " + scalar(dw.Value)
			if dw.Code != "" {
				line += " → " + warnLink(dw.Code, "../")
			}
			*out = append(*out, line)
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
	if len(f.Conflicts) > 0 || len(f.Requires) > 0 || f.ForbiddenWhen != nil {
		put(actionRemoved, "`"+path+"` — conflicts with another field of the same node")
	}
	if dw := f.DefaultWhen; dw != nil && dw.Absent {
		put("replaced", "`"+path+"` — absent value is filled in with "+scalar(dw.Value))
	}
	if g := bodyGate(f); g != "" {
		put("gated", "`"+path+"` — "+strings.TrimPrefix(g, "Requires: "))
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

func contains(items []string, s string) bool {
	for _, it := range items {
		if it == s {
			return true
		}
	}
	return false
}
