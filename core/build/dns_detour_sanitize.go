// File dns_detour_sanitize.go — `dns.detour` как полноправное ребро
// outbound-графа (SPEC 118 W4; features/directions.md §9) и вторая линия
// fail-closed DNS (SPEC 129 Н10).
//
// # Зачем отдельный проход
//
// Граф-санитайзер (outbound_graph_sanitize.go) обходит рёбра ВНУТРИ секций
// outbounds/endpoints: detour узла, члены групп, позиции цепочек. Ребро
// «DNS-сервер → outbound» в этот обход не попадало вовсе: DNS-секция
// собирается позже и из другого материала. В итоге висячий `detour` у
// DNS-сервера — переименовали Направление, свернули папку, выключили
// источник — доезжал до ядра, и ядро отказывалось стартовать: пользователь
// получал «конфиг собрался» в лаунчере и мёртвый VPN в реальности.
//
// Правило блока (build-graph-sanitizer): новое ребро outbound-графа — в
// общий проход, а не в частную проверку. Проход здесь и есть его место: он
// работает по тому же множеству ФИНАЛЬНЫХ тегов, что и остальной санитайзер,
// и вызывается из той же точки сборки.
//
// # Почему выбрасывается сервер, а не ключ (SPEC 129)
//
// До SPEC 129 висячий detour снимался, и сервер резолвил НАПРЯМУЮ — «ровно то,
// что делает сервер вовсе без detour». Это была тихая утечка: google_udp,
// настроенный через удалённое Направление, после переименования Направления
// уходил мимо VPN без единого слова в UI. У DNS-сервера detour — такая же ручка
// анонимности, как у узла (detour-failclosed-uniform): носитель выпадает
// целиком, а ссылки на него закрываются, а не уводятся в прямой резолв —
//
//   - DNS-правило на выпавший сервер становится `action: reject` с тем же
//     условием: пользователь хотел эти домены через конкретный маршрут, и
//     сломанный резолв виден, а утечка — нет;
//   - `dns.final` снимается, последним правилом встаёт `{"action":"reject"}`:
//     без final ядро взяло бы первый сервер списка — у шаблонов это системный
//     резолвер;
//   - `domain_resolver` (DNS-серверов, route, узлов) заменяется резолвером по
//     умолчанию шаблона или первым пригодным сервером: адрес сервера
//     резолвится ДО туннеля, пользовательских доменов там нет, а без
//     резолвера ядро не стартует.
package build

import (
	"encoding/json"
	"net"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// dnsFailClosed — итог второй линии для секций, которые собираются после
// dns (SPEC 129 Н10): теги DNS-серверов, выпавших из-за висячего detour (и
// групп, опустевших от этого), и резолвер, которым заменяется ссылка
// `domain_resolver` на выпавший сервер.
type dnsFailClosed struct {
	dropped map[string]bool
	// resolver — замена; пусто — заменить нечем, ключ снимается.
	resolver string
	// fallback — следующий пригодный сервер после resolver: им заменяется
	// ссылка у самого сервера-замены, которому сослаться на себя нельзя.
	fallback string
}

// active — сработала ли вторая линия.
func (f *dnsFailClosed) active() bool { return f != nil && len(f.dropped) > 0 }

// SanitizeDNSDetours — вторая линия fail-closed DNS без итога для других
// секций: сервер с висячим detour или endpoint выпадает, ссылки на него
// чинятся. Обёртка над sanitizeDNSSection для вызывающих, которым итог не
// нужен.
func SanitizeDNSDetours(dnsRaw json.RawMessage, finalTags map[string]bool) json.RawMessage {
	out, _ := sanitizeDNSSection(dnsRaw, finalTags, "")
	return out
}

// sanitizeDNSSection выбрасывает DNS-серверы, чьи `detour` или `endpoint`
// указывают на тег, которого нет в финальном конфиге, и чинит ссылки на них.
//
// finalTags — множество ВСЕХ outbound-тегов итогового конфига (шаблонные +
// сгенерированные), уже вычищенное граф-санитайзером: узлы, выпавшие
// fail-closed, из него удалены, и ссылка на такой узел здесь честно
// читается висячей. defaultResolver — резолвер по умолчанию шаблона
// (TemplateData.DefaultDomainResolver), первый кандидат на замену.
//
// Возвращает секцию как есть при любой неожиданности (не объект, нет
// servers, битый JSON): DNS-секцию формируют шаблон и пресеты, и
// переписывать её ради несделанной правки нельзя — это меняло бы порядок
// ключей на ровном месте.
func sanitizeDNSSection(dnsRaw json.RawMessage, finalTags map[string]bool, defaultResolver string) (json.RawMessage, *dnsFailClosed) {
	if len(dnsRaw) == 0 || len(finalTags) == 0 {
		return dnsRaw, nil
	}
	var dnsObj map[string]json.RawMessage
	if err := json.Unmarshal(dnsRaw, &dnsObj); err != nil {
		return dnsRaw, nil
	}
	rawServers, ok := dnsObj["servers"]
	if !ok {
		return dnsRaw, nil
	}
	var servers []map[string]interface{}
	if err := json.Unmarshal(rawServers, &servers); err != nil {
		return dnsRaw, nil
	}

	kept := make([]map[string]interface{}, 0, len(servers))
	// byEndpoint / byDetour — серверы, выброшенные ЦЕЛИКОМ. Их обязаны
	// пережить правила, которые на них ссылались (иначе конфиг падает на «dns
	// server not found»), но лечатся ссылки по-разному (Н10).
	byEndpoint := make(map[string]bool)
	byDetour := make(map[string]bool)
	for i := range servers {
		tag, _ := servers[i]["tag"].(string)

		// SPEC 121: `endpoint` — второе ребро «DNS-сервер → outbound»: сервер,
		// чей endpoint исчез (tailscale-резолвер без своего endpoint'а),
		// невалиден без него, и ядро откажется стартовать так же.
		if ep, ok := servers[i]["endpoint"].(string); ok && ep != "" && !finalTags[ep] {
			debuglog.WarnLog("build: DNS server %q points at endpoint %q missing from the final config — "+
				"the server is dropped whole (an endpoint-less server is invalid, the key cannot just be removed)", tag, ep)
			byEndpoint[tag] = true
			continue
		}

		if detour, ok := servers[i]["detour"].(string); ok && detour != "" && !finalTags[detour] {
			debuglog.WarnLog("build: DNS server %q routes through %q, which is missing from the final config — "+
				"the server is dropped (fail-closed: resolving it directly would bypass the chosen route); "+
				"rules aimed at it now reject their queries", tag, detour)
			byDetour[tag] = true
			continue
		}
		kept = append(kept, servers[i])
	}
	if len(byEndpoint) == 0 && len(byDetour) == 0 {
		return dnsRaw, nil
	}

	// Выброс сервера делает висячими ссылки на него: `dns.rules[].server`,
	// `dns.final`, `domain_resolver` и состав групп. Починка идёт ЗДЕСЬ ЖЕ, а
	// не полагается на repairDanglingDNSRefs внутри MergePresetsIntoDNS — тот
	// отработал раньше этого прохода и о новой потере не знает.
	kept, fc, err := repairAfterServerDrop(dnsObj, kept, byDetour, defaultResolver)
	if err != nil {
		debuglog.WarnLog("build: DNS repair after dropping %d server(s) failed: %v — section left as is",
			len(byEndpoint)+len(byDetour), err)
		return dnsRaw, nil
	}

	encoded, err := json.Marshal(kept)
	if err != nil {
		return dnsRaw, nil
	}
	dnsObj["servers"] = encoded
	out, err := json.Marshal(dnsObj)
	if err != nil {
		return dnsRaw, nil
	}
	return out, fc
}

// repairAfterServerDrop чинит ссылки, повисшие после выброса DNS-серверов
// целиком (SPEC 121 §4 п. 5, SPEC 129 Н10).
//
// failClosed — теги, выпавшие второй линией (висячий detour). Группа,
// опустевшая в этом проходе, выпадает вслед за участниками и лечится так же.
// Ссылки на них закрываются (reject, заглушка final, замена резолвера);
// прочие висячие ссылки — серверы без endpoint — прежним механизмом
// repairDanglingDNSRefs, общим с MergePresetsIntoDNS: вторая реализация тех же
// правил разъехалась бы с первой на первой же правке. Цена — перегон
// servers/rules через []interface{} и обратно; порядок при этом сохраняется.
func repairAfterServerDrop(dnsObj map[string]json.RawMessage, servers []map[string]interface{}, failClosed map[string]bool, defaultResolver string) ([]map[string]interface{}, *dnsFailClosed, error) {
	var rules []interface{}
	_, hadRules := dnsObj["rules"]
	if hadRules {
		if err := json.Unmarshal(dnsObj["rules"], &rules); err != nil {
			return nil, nil, err
		}
	}

	// `final` живёт на уровне секции — его читает repairDanglingDNSRefs,
	// поэтому корень тоже нужен картой.
	root := make(map[string]interface{}, len(dnsObj))
	if rawFinal, ok := dnsObj["final"]; ok {
		var final string
		if err := json.Unmarshal(rawFinal, &final); err == nil {
			root["final"] = final
		}
	}

	list := make([]interface{}, 0, len(servers))
	for i := range servers {
		list = append(list, servers[i])
	}
	before := dnsServerTags(list)
	list = pruneDNSGroupMembers(list)

	var fc *dnsFailClosed
	if len(failClosed) > 0 {
		after := dnsServerTags(list)
		dropped := make(map[string]bool, len(failClosed))
		for tag := range failClosed {
			dropped[tag] = true
		}
		for tag := range before {
			if !after[tag] {
				dropped[tag] = true // группа, опустевшая вслед за участниками
			}
		}
		primary, fallback := pickHealDNSResolver(list, defaultResolver, dropped)
		fc = &dnsFailClosed{dropped: dropped, resolver: primary, fallback: fallback}

		rules = rejectDNSRulesOnDropped(rules, dropped)
		if final, _ := root["final"].(string); final != "" && dropped[final] {
			debuglog.WarnLog("dns: final %q was dropped (fail-closed) — key removed, a final reject rule keeps the "+
				"remaining queries away from the first server", final)
			delete(root, "final")
			rules = append(rules, map[string]interface{}{"action": "reject"})
			hadRules = true
		}
		for _, raw := range list {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			healDNSServerResolver(m, fc)
		}
	}
	rules = repairDanglingDNSRefs(root, list, rules)

	out := make([]map[string]interface{}, 0, len(list))
	for _, raw := range list {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, m)
	}

	if hadRules {
		encoded, err := json.Marshal(rules)
		if err != nil {
			return nil, nil, err
		}
		dnsObj["rules"] = encoded
	}
	if final, ok := root["final"].(string); ok && final != "" {
		encoded, err := json.Marshal(final)
		if err != nil {
			return nil, nil, err
		}
		dnsObj["final"] = encoded
	} else if _, had := dnsObj["final"]; had {
		delete(dnsObj, "final")
	}
	return out, fc, nil
}

// dnsServerTags — теги списка серверов.
func dnsServerTags(list []interface{}) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, raw := range list {
		if m, ok := raw.(map[string]interface{}); ok {
			if tag, _ := m["tag"].(string); tag != "" {
				out[tag] = true
			}
		}
	}
	return out
}

// dnsRuleRouteKeys — ключи правила, осмысленные только у маршрутного
// действия (`route`/`evaluate`, опции маршрута ядра): у `reject` ядро
// отвергает их как неизвестные.
var dnsRuleRouteKeys = []string{
	"server", "tag", "speculative", "race", "timeout", "strategy",
	"disable_cache", "disable_optimistic_cache", "rewrite_ttl",
	"client_subnet", "remove_client_subnet",
}

// rejectDNSRulesOnDropped — правило, направлявшее запрос на выпавший сервер,
// остаётся со своим условием и отказывает (SPEC 129 Н10, ответ LxBox №4).
// Вложенные правила логического действия не несут — сервер стоит на
// верхнем уровне, другого места искать не нужно.
func rejectDNSRulesOnDropped(rules []interface{}, dropped map[string]bool) []interface{} {
	for _, raw := range rules {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		srv, _ := m["server"].(string)
		if srv == "" || !dropped[srv] {
			continue
		}
		for _, k := range dnsRuleRouteKeys {
			delete(m, k)
		}
		m["action"] = "reject"
		debuglog.WarnLog("dns: rule aimed at dropped server %q now rejects its queries (fail-closed)", srv)
	}
	return rules
}

// pickHealDNSResolver — чем заменить ссылку `domain_resolver` на выпавший
// сервер: резолвер по умолчанию шаблона, если он в конфиге, иначе первый
// оставшийся сервер не fakeip/hosts (тот же выбор, что у LxBox
// heal_dangling_dns_resolvers). Пусто — заменить нечем.
//
// Второе значение — следующий пригодный сервер после первого: замена для
// самого сервера-замены (норма SPEC 129, сверено с LxBox: «следующий
// пригодный, иначе ключ снимается»).
func pickHealDNSResolver(list []interface{}, defaultResolver string, dropped map[string]bool) (string, string) {
	suitable := func(m map[string]interface{}) bool {
		switch m["type"] {
		case "fakeip", "hosts":
			return false
		}
		return true
	}
	var order []string
	hasDefault := false
	for _, raw := range list {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "" || dropped[tag] || !suitable(m) {
			continue
		}
		if defaultResolver != "" && tag == defaultResolver {
			hasDefault = true
			continue
		}
		order = append(order, tag)
	}
	if hasDefault {
		order = append([]string{defaultResolver}, order...)
	}
	switch len(order) {
	case 0:
		return "", ""
	case 1:
		return order[0], ""
	}
	return order[0], order[1]
}

// healDNSServerResolver — `domain_resolver` DNS-сервера на выпавший сервер.
// У сервера с доменным адресом ссылка заменяется (без резолвера ядро не
// поднимет транспорт), у сервера с IP или без адреса — снимается: резолвер
// ему не нужен.
func healDNSServerResolver(m map[string]interface{}, fc *dnsFailClosed) {
	target, isObject := resolverTarget(m["domain_resolver"])
	if target == "" || !fc.dropped[target] {
		return
	}
	tag, _ := m["tag"].(string)
	addr, _ := m["server"].(string)
	replacement := fc.resolver
	if replacement == tag {
		replacement = fc.fallback
	}
	if dnsAddressIsDomain(addr) && replacement != "" && replacement != tag {
		setResolverTarget(m, "domain_resolver", replacement, isObject)
		debuglog.WarnLog("dns: server %q: domain_resolver %q was dropped — replaced with %q", tag, target, replacement)
		return
	}
	delete(m, "domain_resolver")
	debuglog.WarnLog("dns: server %q: domain_resolver %q was dropped — key removed", tag, target)
}

// healResolverKey — ключ-резолвер (`default_domain_resolver` у route,
// `domain_resolver` у узла) на выпавший сервер: замена, а заменить нечем —
// ключ снимается. Возвращает true, если объект изменён.
func (f *dnsFailClosed) healResolverKey(m map[string]interface{}, key, where string) bool {
	if !f.active() || m == nil {
		return false
	}
	target, isObject := resolverTarget(m[key])
	if target == "" || !f.dropped[target] {
		return false
	}
	if f.resolver != "" {
		setResolverTarget(m, key, f.resolver, isObject)
		debuglog.WarnLog("build: %s: %s %q was dropped (fail-closed) — replaced with %q", where, key, target, f.resolver)
		return true
	}
	delete(m, key)
	debuglog.WarnLog("build: %s: %s %q was dropped and no DNS server can replace it — key removed", where, key, target)
	return true
}

// resolverTarget — тег из значения ключа-резолвера: строка или объект
// `{server, strategy, …}` (обе формы ядра).
func resolverTarget(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, false
	case map[string]interface{}:
		s, _ := t["server"].(string)
		return s, true
	}
	return "", false
}

// setResolverTarget — записать тег в ключ-резолвер, не теряя форму значения.
func setResolverTarget(m map[string]interface{}, key, tag string, isObject bool) {
	if isObject {
		if obj, ok := m[key].(map[string]interface{}); ok {
			obj["server"] = tag
			return
		}
	}
	m[key] = tag
}

// dnsAddressIsDomain — адрес сервера — имя, а не IP-литерал.
func dnsAddressIsDomain(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	addr = strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
	return net.ParseIP(addr) == nil
}

// healResolversInEntries — `domain_resolver` узлов (outbounds/endpoints) на
// выпавший сервер. Записи без такой ссылки возвращаются теми же байтами.
func (f *dnsFailClosed) healResolversInEntries(entries []json.RawMessage, section string) []json.RawMessage {
	if !f.active() || len(entries) == 0 {
		return entries
	}
	var out []json.RawMessage
	for i, raw := range entries {
		if !strings.Contains(string(raw), "domain_resolver") {
			if out != nil {
				out = append(out, raw)
			}
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			if out != nil {
				out = append(out, raw)
			}
			continue
		}
		tag, _ := m["tag"].(string)
		if !f.healResolverKey(m, "domain_resolver", section+" "+tag) {
			if out != nil {
				out = append(out, raw)
			}
			continue
		}
		encoded, err := json.Marshal(m)
		if err != nil {
			if out != nil {
				out = append(out, raw)
			}
			continue
		}
		if out == nil {
			out = append([]json.RawMessage(nil), entries[:i]...)
		}
		out = append(out, encoded)
	}
	if out == nil {
		return entries
	}
	return out
}

// healResolversInSection — то же для секции шаблона (массив объектов
// outbounds/endpoints) или route (объект). Нетронутая секция — теми же
// байтами.
func (f *dnsFailClosed) healResolversInSection(raw json.RawMessage, section string) json.RawMessage {
	if !f.active() || len(raw) == 0 || !strings.Contains(string(raw), "domain_resolver") {
		return raw
	}
	if section == "route" {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return raw
		}
		if !f.healResolverKey(m, "default_domain_resolver", "route") {
			return raw
		}
		out, err := json.Marshal(m)
		if err != nil {
			return raw
		}
		return out
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return raw
	}
	healed := f.healResolversInEntries(list, section)
	if len(healed) == len(list) {
		same := true
		for i := range healed {
			if string(healed[i]) != string(list[i]) {
				same = false
				break
			}
		}
		if same {
			return raw
		}
	}
	out, err := json.Marshal(healed)
	if err != nil {
		return raw
	}
	return out
}
