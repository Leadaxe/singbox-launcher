package main

import (
	"strings"

	"singbox-launcher/core/config/registry"
)

// Полный словарь ссылки одной схемы.
//
// Отзыв владельца: раздел «Link parameters» на странице схемы показывал ТОЛЬКО
// собственные параметры схемы, а общие (sni, fp, pbk, sid, alpn, path, host,
// serviceName, mode …) лежали на _tls.md/_transports.md — читатель искал их на
// странице протокола и не находил. Теперь общие параметры разворачиваются
// сюда, сгруппированно, а общие страницы остаются справочником.
//
// Применимость выводится из тела схемы, а не из отдельного списка в реестре:
// параметры TLS/REALITY идут схеме тогда и только тогда, когда у неё есть поле
// `tls` (и, для конкретного поля, когда оно ей не запрещено через
// allowed_for/forbidden_for); параметры транспорта — когда есть поле
// `transport`, и только те варианты, которые в нём есть.

// linkGroup — подзаголовок раздела «Link parameters».
type linkGroup struct {
	title  string
	anchor string // часть якоря параметров группы: link-<anchor>-<name>
	note   string // строка под подзаголовком, если группе нужно пояснение
	items  []linkParam
}

// linkParam — один параметр ссылки вместе с адресом в теле.
type linkParam struct {
	name  string
	param *uriParam

	// bodyPaths — пути ПОЛЕЙ ТЕЛА, в которые параметр ложится, уже
	// приведённые к виду, в котором тело их знает: у транспортных параметров
	// реестр пишет `transport.path`, а тело держит `transport.ws.path`.
	bodyPaths []string
	anchor    string // якорь самого параметра
}

// schemeLinkParams собирает весь словарь ссылки схемы по группам.
// Пустой результат означает, что ссылочной формы у схемы нет вовсе.
func schemeLinkParams(raw *rawRegistry, scheme string, p *rawProtocol, body *registry.BodySchema) []linkGroup {
	if p == nil || p.URI == nil {
		return nil
	}

	var groups []linkGroup

	// Общее: то, что лежит не в query, а в самой ссылке.
	common := linkGroup{title: "Common", anchor: "common"}
	if p.URI.Userinfo != nil {
		common.items = append(common.items, mkParam(raw, scheme, "common", "userinfo", p.URI.Userinfo, nil))
	}
	// Хост и порт словарём не описаны — они и есть авторитет URI, — но читатель
	// ищет их здесь наравне с остальным, поэтому строки собираются из полей
	// тела, в которые авторитет разбирается.
	for _, pair := range []struct{ name, path, desc string }{
		{"host", "server", "The authority of the link: everything before `:` in `scheme://…@host:port`."},
		{"port", "server_port", "The authority of the link: everything after `:` in `scheme://…@host:port`."},
	} {
		if body == nil || body.Fields[pair.path] == nil {
			continue
		}
		common.items = append(common.items, mkParam(raw, scheme, "common", pair.name,
			&uriParam{DescEn: pair.desc}, []string{pair.path}))
	}
	if p.URI.Fragment != "" {
		common.items = append(common.items, mkParam(raw, scheme, "common", "#fragment",
			&uriParam{DescEn: "The part after `#`: the name the node is shown under. " +
				"It is not a body field — it is the node `" + p.URI.Fragment + "`."}, nil))
	}
	if len(common.items) > 0 {
		groups = append(groups, common)
	}

	// Собственные параметры схемы.
	own := linkGroup{title: "Protocol-specific", anchor: "proto"}
	declared := map[string]bool{}
	for _, name := range p.URI.queryOrder {
		param := p.URI.Query[name]
		if param == nil {
			continue
		}
		declared[name] = true
		own.items = append(own.items, mkParam(raw, scheme, "proto", name, param, nil))
	}
	if len(own.items) > 0 {
		groups = append(groups, own)
	}

	// Общие параметры TLS/REALITY — только если схема вообще носит TLS-блок.
	// Параметр, который схема объявила у себя (её словарь главнее: там свои
	// алиасы и своё описание), здесь не повторяется.
	if raw != nil && bodyHas(body, "tls") {
		tlsGroup := linkGroup{
			title:  "TLS / REALITY",
			anchor: "tls",
			note: "Shared across every scheme that carries a TLS block; the reference page is " +
				"[`_tls.md`](_tls.md).",
		}
		for _, name := range raw.tlsOrder {
			if declared[name] {
				continue
			}
			if param := raw.tlsParams[name]; param != nil && tlsParamApplies(raw, scheme, param) {
				tlsGroup.items = append(tlsGroup.items, mkParam(raw, scheme, "tls", name, param, nil))
			}
		}
		for _, name := range raw.realityOrder {
			if declared[name] {
				continue
			}
			if param := raw.realityParams[name]; param != nil && tlsParamApplies(raw, scheme, param) {
				tlsGroup.items = append(tlsGroup.items, mkParam(raw, scheme, "tls", name, param, nil))
			}
		}
		if len(tlsGroup.items) > 0 {
			groups = append(groups, tlsGroup)
		}
	}

	// Транспорты — только те варианты, что есть у схемы в теле.
	if raw != nil && bodyHas(body, "transport") {
		for _, tname := range raw.transportOrder {
			tr := raw.transports[tname]
			if tr == nil || len(tr.paramOrder) == 0 {
				continue
			}
			if !transportApplies(body, tname) {
				continue
			}
			g := linkGroup{
				title:  "Transport · `" + tname + "`",
				anchor: "tr-" + tname,
				note: "Read when the link says `type=" + tname + "`; the reference page is " +
					"[`_transports.md`](_transports.md).",
			}
			for _, name := range tr.paramOrder {
				param := tr.Params[name]
				if param == nil || declared[name] {
					continue
				}
				g.items = append(g.items, mkParam(raw, scheme, g.anchor, name, param,
					transportBodyPaths(raw, scheme, tname, param)))
			}
			if len(g.items) > 0 {
				groups = append(groups, g)
			}
		}
	}

	return groups
}

// mkParam готовит параметр к печати: якорь и адрес в теле.
func mkParam(raw *rawRegistry, scheme, group, name string, p *uriParam, paths []string) linkParam {
	if paths == nil && !p.DropAlways {
		paths = p.mapsToPaths()
	}
	return linkParam{
		name:      name,
		param:     p,
		bodyPaths: paths,
		anchor:    "link-" + group + "-" + slug(name),
	}
}

// transportBodyPaths переводит `maps_to` транспортного параметра в путь тела:
// реестр пишет его относительно блока транспорта (`transport.path`), а тело
// держит вариант отдельным сегментом (`transport.ws.path`).
func transportBodyPaths(raw *rawRegistry, scheme, variant string, p *uriParam) []string {
	raws := p.mapsToPaths()
	out := make([]string, 0, len(raws))
	for _, path := range raws {
		if rest := strings.TrimPrefix(path, "transport."); rest != path {
			out = append(out, "transport."+variant+"."+rest)
			continue
		}
		out = append(out, path)
	}
	return out
}

// tlsParamApplies отсеивает общий параметр TLS, чьё поле тела этой схеме
// запрещено: у naive ядро читает из tls только enabled/server_name/certificate,
// и обещать ему `fp=` или `pbk=` значит врать.
func tlsParamApplies(raw *rawRegistry, scheme string, p *uriParam) bool {
	paths := p.mapsToPaths()
	if len(paths) == 0 {
		// Параметр без адреса в теле (ech — всегда снимается) остаётся: он
		// встречается в ссылках, и читателю важно знать, что с ним будет.
		return true
	}
	for _, path := range paths {
		if f, _ := resolveBodyField(raw, scheme, path); f != nil {
			// Запрет ищется по ВСЕМУ пути, а не только у самого поля: у
			// `pbk` адрес tls.reality.public_key, а forbidden_for стоит на
			// блоке tls.reality.
			if forbiddenAncestor(raw, scheme, path) == nil {
				return true
			}
			continue
		}
		// Поля нет в теле этой схемы — параметр ей не адресован.
	}
	return false
}

// transportApplies говорит, есть ли у схемы такой вариант транспорта.
func transportApplies(body *registry.BodySchema, variant string) bool {
	if body == nil {
		return false
	}
	f := body.Fields["transport"]
	if f == nil {
		return false
	}
	_, ok := f.Variants[variant]
	return ok
}

func bodyHas(body *registry.BodySchema, name string) bool {
	return body != nil && body.Fields[name] != nil
}

// forbiddenAncestor — ближайшее поле по пути (само поле или любой его предок),
// которое запрещено этой схеме. Нужен там, где адрес в теле глубже запрета:
// forbidden_for стоит на блоке `tls.utls`, а параметр ссылки `fp` целится в
// `tls.utls.fingerprint`. Возвращает nil, если запрета по пути нет.
func forbiddenAncestor(raw *rawRegistry, scheme, path string) *registry.Field {
	cur := path
	for cur != "" {
		if f, _ := resolveBodyField(raw, scheme, cur); f != nil && !fieldAllowedFor(f, scheme) {
			return f
		}
		i := strings.LastIndex(cur, ".")
		if i < 0 {
			return nil
		}
		cur = cur[:i]
	}
	return nil
}

func fieldAllowedFor(f *registry.Field, scheme string) bool {
	if len(f.ForbiddenFor) > 0 && contains(f.ForbiddenFor, scheme) {
		return false
	}
	if len(f.AllowedFor) > 0 && !contains(f.AllowedFor, scheme) {
		return false
	}
	return true
}

// resolveBodyField ищет поле тела по пути и, если точного поля нет,
// поднимается к ближайшему предку. Точного поля не бывает у ключа карты:
// `transport.ws.headers.Host` — это ключ `Host` внутри поля `headers`, а
// правило значения и якорь есть только у самого `headers`.
func resolveBodyField(raw *rawRegistry, scheme, path string) (*registry.Field, string) {
	if raw == nil {
		return nil, ""
	}
	cur := path
	for cur != "" {
		if f := raw.bodyField(scheme, cur); f != nil {
			return f, cur
		}
		i := strings.LastIndex(cur, ".")
		if i < 0 {
			return nil, ""
		}
		cur = cur[:i]
	}
	return nil, ""
}

// slug — якорь из пути или имени. GitHub принимает в `id` буквы, цифры и
// дефис; точка и подчёркивание в путях тела встречаются постоянно, поэтому
// сводятся к дефису, а всё прочее выбрасывается.
func slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '.' || r == '_' || r == '-' || r == ' ' || r == '[' || r == ']':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// bodyAnchor — якорь поля тела на странице схемы. На него ссылается «Maps to»
// каждого параметра ссылки.
func bodyAnchor(path string) string {
	return "body-" + slug(path)
}

// anchorTag — сам якорь. Ставится инлайном в начале пункта списка: заголовком
// поле тела быть не может (их на странице под сотню), а инлайновый `<a id>`
// GitHub в списках рендерит и на него ссылается.
func anchorTag(id string) string {
	return "<a id=\"" + id + "\"></a>"
}

// hasSharedGroup — есть ли на странице группа общих параметров (TLS или
// транспорт). У wireguard и ssh их нет, и обещать их читателю нельзя.
func hasSharedGroup(groups []linkGroup) bool {
	for _, g := range groups {
		if g.anchor == "tls" || strings.HasPrefix(g.anchor, "tr-") {
			return true
		}
	}
	return false
}

// linkParamsByBodyPath — обратный индекс: какой параметр ссылки кладёт
// значение в это поле тела. Нужен строке «Set by link parameter(s): …» у поля.
func linkParamsByBodyPath(groups []linkGroup) map[string][]linkParam {
	out := map[string][]linkParam{}
	for _, g := range groups {
		for _, item := range g.items {
			for _, path := range item.bodyPaths {
				out[path] = append(out[path], item)
			}
		}
	}
	return out
}
