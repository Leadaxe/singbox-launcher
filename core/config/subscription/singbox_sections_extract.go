// File singbox_sections_extract.go — извлечение секций узла из ЦЕЛОГО
// sing-box-конфига (SPEC 121 §6).
//
// # Зачем
//
// Пользователь вставляет как источник конфиг, в котором один узел и его
// связка: endpoint, DNS-сервер, привязанный к нему, DNS-правило на его домены
// и правило маршрута на его подсети. Без связки узел бесполезен, а импорт до
// сих пор выбрасывал `dns` и `route` целиком (singboxIgnoredSections) — и
// пользователь собирал их руками на трёх вкладках.
//
// # Правило извлечения, и почему оно такое узкое
//
// Секции берутся ТОЛЬКО когда в конфиге ровно ОДИН узел (не группа, не
// служебный тип) и он разобрался. Иначе непонятно, чей это DNS-сервер: связка
// принадлежит узлу, а не файлу, и раздать её нескольким узлам значило бы
// придумать за пользователя. Конфиг с двумя узлами ведёт себя как сегодня —
// `dns`/`route` игнорируются.
//
// Берётся только то, что СВЯЗАНО с узлом по ссылке:
//
//   - `dns.servers[]`, чей `detour` или `endpoint` равен тегу узла;
//   - `dns.rules[]`, чей `server` равен тегу одного из взятых серверов;
//   - `route.rules[]`, чей `outbound` равен тегу узла.
//
// Общие настройки (`dns.final`, `route.final`, `auto_detect_interface`,
// сервер `local-dns`) не берутся: это настройки конфига целиком, у них своё
// место в лаунчере, и подменять их вставкой одного узла нельзя.
//
// Ссылки на узел переписываются в `@self` — связка обязана пережить
// переименование узла.
//
// # Ограничение
//
// Порядок ключей внутри взятого фрагмента здесь НЕ сохраняется: импорт
// работает на `map[string]interface{}`, и порядок потерян ещё до этой точки
// (тем же свойством живёт и тело узла в carveSingboxJSONMulti). Значим он
// только при байтовом сравнении с эмиссией, а фрагменты секций ни с чем не
// сравниваются.
package subscription

import (
	"encoding/json"
	"sort"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// nodeSectionsSelfVar — плейсхолдер «финальный тег этого узла» (SPEC 121).
const nodeSectionsSelfVar = "@self"

// ExtractNodeSections вынимает секции единственного узла конфига.
//
// Чистая функция: ни сети, ни состояния, ни мутации входа (фрагменты
// копируются). Возвращает nil, если правило извлечения не выполнено.
//
// nodeTag — тег записи узла В КОНФИГЕ (не финальный тег лаунчера): по нему
// ищутся ссылки, и он же переписывается в `@self`.
func ExtractNodeSections(cfg map[string]interface{}, nodeTag string) *configtypes.NodeSections {
	if cfg == nil || strings.TrimSpace(nodeTag) == "" {
		return nil
	}

	out := &configtypes.NodeSections{}

	// Шаг 1: DNS-серверы, привязанные к узлу. Их локальные теги нужны шагу 2 —
	// правило берётся, только если ссылается на СВОЙ сервер; правило на
	// `local-dns` относится к конфигу, а не к узлу.
	ownServerTags := map[string]bool{}
	if dns, ok := cfg["dns"].(map[string]interface{}); ok {
		for _, srv := range jsonObjectList(dns["servers"]) {
			if !refersToNode(mapString(srv, "detour"), nodeTag) && !refersToNode(mapString(srv, "endpoint"), nodeTag) {
				continue
			}
			if tag := mapString(srv, "tag"); tag != "" {
				ownServerTags[tag] = true
			}
			if raw, ok := marshalNodeSectionFragment(srv, nodeTag); ok {
				out.DNSServers = append(out.DNSServers, raw)
			}
		}
		for _, rule := range jsonObjectList(dns["rules"]) {
			if srv := mapString(rule, "server"); srv == "" || !ownServerTags[srv] {
				continue
			}
			if raw, ok := marshalNodeSectionFragment(rule, nodeTag); ok {
				out.DNSRules = append(out.DNSRules, raw)
			}
		}
	}

	// Шаг 2: правила маршрута с целью-узлом. `rule_set` пропускается: в v1
	// секции наборов правил не объявляют и на них не ссылаются, а висячая
	// ссылка роняет конфиг целиком.
	if route, ok := cfg["route"].(map[string]interface{}); ok {
		for _, rule := range jsonObjectList(route["rules"]) {
			if !refersToNode(mapString(rule, "outbound"), nodeTag) {
				continue
			}
			if _, hasRuleSet := rule["rule_set"]; hasRuleSet {
				debuglog.WarnLog("Parser: node sections: route rule for %q references a rule set — skipped", nodeTag)
				continue
			}
			if raw, ok := marshalNodeSectionFragment(rule, nodeTag); ok {
				out.Rules = append(out.Rules, raw)
			}
		}
	}

	if out.IsEmpty() {
		return nil
	}
	return out
}

// SingleSectionCarrierTag — тег узла, которому принадлежит связка конфига, или
// "" если правило извлечения не выполнено.
//
// Условие — ровно одна запись в `outbounds[]`+`endpoints[]`, которая не
// группа и не служебный тип. Групповые и служебные записи не считаются
// узлами (те же предикаты, что у импорта): конфиг с одним vless и одним
// `selector` над ним — это по-прежнему конфиг об одном узле.
func SingleSectionCarrierTag(cfg map[string]interface{}) string {
	tag := ""
	count := 0
	for _, entry := range singboxAllEntries(cfg) {
		entryType := strings.ToLower(strings.TrimSpace(mapString(entry, "type")))
		if entryType == "" || IsSingboxServiceType(entryType) || IsSingboxGroupType(entryType) {
			continue
		}
		count++
		if count > 1 {
			return ""
		}
		tag = mapString(entry, "tag")
	}
	if count != 1 {
		return ""
	}
	return strings.TrimSpace(tag)
}

// marshalNodeSectionFragment — копия фрагмента с переписанной ссылкой на узел.
//
// Ссылка ищется по ЗНАЧЕНИЮ во всём фрагменте, а не в перечне полей: она может
// стоять в `outbound`, `detour`, `endpoint`, `server` — и следующее поле
// перечислять забудут.
func marshalNodeSectionFragment(obj map[string]interface{}, nodeTag string) (json.RawMessage, bool) {
	replaced := replaceJSONStringValue(copyJSONMap(obj), nodeTag, nodeSectionsSelfVar)
	raw, err := json.Marshal(replaced)
	if err != nil {
		debuglog.WarnLog("Parser: node sections: fragment not serializable: %v", err)
		return nil, false
	}
	return json.RawMessage(raw), true
}

// replaceJSONStringValue заменяет каждое строковое ЗНАЧЕНИЕ, равное from, на
// to. Ключи не трогаются: ключ — имя поля sing-box, а не ссылка.
func replaceJSONStringValue(v interface{}, from, to string) interface{} {
	switch t := v.(type) {
	case string:
		if t == from {
			return to
		}
		return t
	case []interface{}:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = replaceJSONStringValue(t[i], from, to)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = replaceJSONStringValue(val, from, to)
		}
		return out
	}
	return v
}

// jsonObjectList — список объектов из значения секции; чужие формы (не массив,
// не объекты внутри) молча пропускаются, как и везде в импорте.
func jsonObjectList(v interface{}) []map[string]interface{} {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if obj, ok := item.(map[string]interface{}); ok {
			out = append(out, obj)
		}
	}
	return out
}

// sortedNodeSectionKinds — виды извлечённых фрагментов для лога, в
// детерминированном порядке.
func sortedNodeSectionKinds(ns *configtypes.NodeSections) []string {
	if ns == nil {
		return nil
	}
	var out []string
	if len(ns.DNSServers) > 0 {
		out = append(out, "dns.servers")
	}
	if len(ns.DNSRules) > 0 {
		out = append(out, "dns.rules")
	}
	if len(ns.Rules) > 0 {
		out = append(out, "route.rules")
	}
	sort.Strings(out)
	return out
}

// refersToNode — ссылка указывает на узел-носитель: либо его тег, либо уже
// переписанный плейсхолдер `@self`. Второе — документ узла (SPEC 121 §5.1),
// который «Add server» и окно папки отдают общему парсеру как целый конфиг:
// без этого равенства связка из формы терялась бы на папочном пути.
func refersToNode(ref, nodeTag string) bool {
	return ref != "" && (ref == nodeTag || ref == nodeSectionsSelfVar)
}
