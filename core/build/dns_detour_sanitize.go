// File dns_detour_sanitize.go — `dns.detour` как полноправное ребро
// outbound-графа (SPEC 118 W4; features/directions.md §9).
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
// # Почему снимается ключ, а не сервер
//
// Здесь строгость иная, чем у detour УЗЛА, и намеренно. У узла detour —
// управление анонимностью: снять его значило бы отправить трафик напрямую
// молча, поэтому выбрасывается носитель (fail-closed). У DNS-сервера detour
// — это «через какой канал резолвить»; сняв ключ, ядро резолвит напрямую —
// ровно то, что делает и сервер вовсе без detour (и что штатно делает
// `detour: direct-out`, который сборка снимает сама). Выбросить же сам
// DNS-сервер значило бы оставить конфиг без резолвера и уронить весь DNS.
package build

import (
	"encoding/json"

	"singbox-launcher/internal/debuglog"
)

// SanitizeDNSDetours снимает у DNS-серверов `detour`, указывающий на тег,
// которого нет в финальном конфиге.
//
// finalTags — множество ВСЕХ outbound-тегов итогового конфига (шаблонные +
// сгенерированные), уже вычищенное граф-санитайзером: узлы, выпавшие
// fail-closed, из него удалены, и ссылка на такой узел здесь честно
// читается висячей.
//
// Возвращает секцию как есть при любой неожиданности (не объект, нет
// servers, битый JSON): DNS-секцию формируют шаблон и пресеты, и
// переписывать её ради несделанной правки нельзя — это меняло бы порядок
// ключей на ровном месте.
func SanitizeDNSDetours(dnsRaw json.RawMessage, finalTags map[string]bool) json.RawMessage {
	if len(dnsRaw) == 0 || len(finalTags) == 0 {
		return dnsRaw
	}
	var dnsObj map[string]json.RawMessage
	if err := json.Unmarshal(dnsRaw, &dnsObj); err != nil {
		return dnsRaw
	}
	rawServers, ok := dnsObj["servers"]
	if !ok {
		return dnsRaw
	}
	var servers []map[string]interface{}
	if err := json.Unmarshal(rawServers, &servers); err != nil {
		return dnsRaw
	}

	changed := false
	kept := make([]map[string]interface{}, 0, len(servers))
	// droppedTags — серверы, выброшенные ЦЕЛИКОМ: их обязаны пережить
	// правила, которые на них ссылались (иначе конфиг падает на «dns server
	// not found»).
	droppedTags := make(map[string]bool)
	for i := range servers {
		tag, _ := servers[i]["tag"].(string)

		// SPEC 121: `endpoint` — второе ребро «DNS-сервер → outbound». В
		// отличие от detour, снять здесь нечего: сервер, чей endpoint исчез
		// (tailscale-резолвер без своего endpoint'а), невалиден без него, и
		// ядро откажется стартовать так же. Поэтому политика тут как у detour
		// УЗЛА — выбрасывается носитель.
		if ep, ok := servers[i]["endpoint"].(string); ok && ep != "" && !finalTags[ep] {
			debuglog.WarnLog("build: DNS server %q points at endpoint %q missing from the final config — "+
				"the server is dropped whole (an endpoint-less server is invalid, the key cannot just be removed)", tag, ep)
			if tag != "" {
				droppedTags[tag] = true
			}
			changed = true
			continue
		}

		if detour, ok := servers[i]["detour"].(string); ok && detour != "" && !finalTags[detour] {
			debuglog.WarnLog("build: DNS server %q has detour target %q missing from the final config — key removed "+
				"(otherwise the core will not start); resolution will go direct", tag, detour)
			delete(servers[i], "detour")
			changed = true
		}
		kept = append(kept, servers[i])
	}
	if !changed {
		return dnsRaw
	}

	// Выброс сервера делает висячими ссылки на него: `dns.rules[].server`,
	// `dns.final`, `domain_resolver` и состав групп. Починка идёт ЗДЕСЬ ЖЕ, а
	// не полагается на repairDanglingDNSRefs внутри MergePresetsIntoDNS — тот
	// отработал раньше этого прохода и о новой потере не знает.
	if len(droppedTags) > 0 {
		var err error
		kept, err = repairAfterServerDrop(dnsObj, kept)
		if err != nil {
			debuglog.WarnLog("build: DNS repair after dropping %d server(s) failed: %v — section left as is", len(droppedTags), err)
			return dnsRaw
		}
	}

	encoded, err := json.Marshal(kept)
	if err != nil {
		return dnsRaw
	}
	dnsObj["servers"] = encoded
	out, err := json.Marshal(dnsObj)
	if err != nil {
		return dnsRaw
	}
	return out
}

// repairAfterServerDrop чинит ссылки, повисшие после выброса DNS-серверов
// целиком (SPEC 121 §4 п. 5).
//
// Переиспользует те же pruneDNSGroupMembers / repairDanglingDNSRefs, что
// работают внутри MergePresetsIntoDNS: вторая реализация тех же правил
// разъехалась бы с первой на первой же правке. Цена — перегон servers/rules
// через []interface{} и обратно; порядок при этом сохраняется, ключи внутри
// тел не переупорядочиваются (карты и так не хранят порядок — секция уже
// собрана маршалингом карты выше по конвейеру).
func repairAfterServerDrop(dnsObj map[string]json.RawMessage, servers []map[string]interface{}) ([]map[string]interface{}, error) {
	var rules []interface{}
	_, hadRules := dnsObj["rules"]
	if hadRules {
		if err := json.Unmarshal(dnsObj["rules"], &rules); err != nil {
			return nil, err
		}
	}

	// `final` и `domain_resolver` живут на уровне секции — их читает
	// repairDanglingDNSRefs, поэтому корень тоже нужен картой.
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
	list = pruneDNSGroupMembers(list)
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
			return nil, err
		}
		dnsObj["rules"] = encoded
	}
	if final, ok := root["final"].(string); ok && final != "" {
		encoded, err := json.Marshal(final)
		if err != nil {
			return nil, err
		}
		dnsObj["final"] = encoded
	} else if _, had := dnsObj["final"]; had {
		delete(dnsObj, "final")
	}
	return out, nil
}
