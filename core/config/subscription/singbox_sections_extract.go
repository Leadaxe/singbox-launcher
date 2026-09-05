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
// # Хранимая форма
//
// Собранные фрагменты переводит в записи состояния ОДНА общая функция
// (state.NodeSectionsFromSingbox) — та же, которой пользуется вкладка JSON
// узла и конструктор Tailscale. Здесь только отбор «что связано с узлом».
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
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

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

	var picked state.SingboxNodeFragments
	picked.NodeTag = nodeTag

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
			if raw, ok := marshalNodeSectionFragment(srv); ok {
				picked.DNSServers = append(picked.DNSServers, raw)
			}
		}
		for _, rule := range jsonObjectList(dns["rules"]) {
			if srv := mapString(rule, "server"); srv == "" || !ownServerTags[srv] {
				continue
			}
			if raw, ok := marshalNodeSectionFragment(rule); ok {
				picked.DNSRules = append(picked.DNSRules, raw)
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
			if raw, ok := marshalNodeSectionFragment(rule); ok {
				picked.RouteRules = append(picked.RouteRules, raw)
			}
		}
	}

	sections, err := state.NodeSectionsFromSingbox(picked)
	if err != nil {
		// Отбор здесь уже отсеял всё, что перевод считает отказом (rule_set),
		// но конфиг пишет человек: остаётся чужая `@var` в строке. Узел
		// приезжает без секций, а причина называется вслух.
		debuglog.WarnLog("Parser: node sections for %q not taken: %v", nodeTag, err)
		return nil
	}
	if sections.IsEmpty() {
		return nil
	}
	raw, err := json.Marshal(sections)
	if err != nil {
		debuglog.WarnLog("Parser: node sections for %q not serializable: %v", nodeTag, err)
		return nil
	}
	return &configtypes.NodeSections{Raw: raw}
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

// marshalNodeSectionFragment — сериализованная копия фрагмента.
//
// Перепись ссылки на узел в `@self` здесь НЕ делается: её делает общий
// перевод (state.NodeSectionsFromSingbox по полю NodeTag) — по значению во
// всём фрагменте, потому что ссылка может стоять в `outbound`, `detour`,
// `endpoint`, `server`, и следующее поле перечислять забудут.
func marshalNodeSectionFragment(obj map[string]interface{}) (json.RawMessage, bool) {
	raw, err := json.Marshal(copyJSONMap(obj))
	if err != nil {
		debuglog.WarnLog("Parser: node sections: fragment not serializable: %v", err)
		return nil, false
	}
	return json.RawMessage(raw), true
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

// nodeSectionEntryCount — сколько записей несёт извлечённая связка (для
// счётчика результата и лога).
func nodeSectionEntryCount(ns *configtypes.NodeSections) int {
	decoded := decodeNodeSections(ns)
	if decoded == nil {
		return 0
	}
	return len(decoded.DNSServers()) + len(decoded.DNSRules()) + len(decoded.Rules)
}

// decodeNodeSections разбирает непрозрачный блок сборочной формы обратно в
// записи состояния.
func decodeNodeSections(ns *configtypes.NodeSections) *state.NodeSections {
	if ns.IsEmpty() {
		return nil
	}
	var decoded state.NodeSections
	if err := json.Unmarshal(ns.Raw, &decoded); err != nil {
		return nil
	}
	return &decoded
}

// sortedNodeSectionKinds — виды извлечённых фрагментов для лога, в
// детерминированном порядке.
func sortedNodeSectionKinds(ns *configtypes.NodeSections) []string {
	decoded := decodeNodeSections(ns)
	if decoded == nil {
		return nil
	}
	var out []string
	if len(decoded.DNSServers()) > 0 {
		out = append(out, "dns.servers")
	}
	if len(decoded.DNSRules()) > 0 {
		out = append(out, "dns.rules")
	}
	if len(decoded.Rules) > 0 {
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
	return ref != "" && (ref == nodeTag || ref == state.SelfPlaceholder)
}
