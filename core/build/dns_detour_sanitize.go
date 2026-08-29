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
	for i := range servers {
		detour, ok := servers[i]["detour"].(string)
		if !ok || detour == "" {
			continue
		}
		if finalTags[detour] {
			continue
		}
		tag, _ := servers[i]["tag"].(string)
		debuglog.WarnLog("build: у DNS-сервера %q цель detour %q отсутствует в финальном конфиге — ключ снят "+
			"(иначе ядро не стартует); резолв пойдёт напрямую", tag, detour)
		delete(servers[i], "detour")
		changed = true
	}
	if !changed {
		return dnsRaw
	}

	encoded, err := json.Marshal(servers)
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

// DNSDetourTags — теги, на которые ссылаются `detour` DNS-серверов секции.
//
// Нужен реестру известных целей: тег, на который смотрит DNS, — такая же
// ссылка, как цель правила, и переименование обязано её переписывать
// (features/directions.md §9).
func DNSDetourTags(dnsRaw json.RawMessage) []string {
	if len(dnsRaw) == 0 {
		return nil
	}
	var dnsObj struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(dnsRaw, &dnsObj); err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(dnsObj.Servers))
	var out []string
	for _, srv := range dnsObj.Servers {
		detour, ok := srv["detour"].(string)
		if !ok || detour == "" {
			continue
		}
		if _, dup := seen[detour]; dup {
			continue
		}
		seen[detour] = struct{}{}
		out = append(out, detour)
	}
	return out
}
