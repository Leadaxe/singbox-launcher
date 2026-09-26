package backup

// Списки известных ключей формата 1.0 (SPEC 127 §6.2, W2.6; ловушка CODEMAP
// §7.26).
//
// # Почему рефлексией, а у 0.x — руками
//
// У 0.x список ключей НОРМАТИВЕН: это ровно таблица полей BACKUP.md §2, и
// выводить его из Go-структур значило бы объявить «схемой» текущую форму
// кода — любое внутреннее переименование молча меняло бы контракт. Списки
// 0.x поэтому и остаются написанными руками (file.go), без единой правки.
//
// У 1.0 нормативна ровно обратная вещь: файл ЕСТЬ сериализация состояния
// (П1), и «ключ, который знает модель» — это буквально json-тег поля. Список,
// переписанный сюда руками, был бы вторым источником истины и разъехался бы с
// первым же новым полем: поле поехало бы в файл, а импорт ругался бы на него
// `backup_unknown_field` — на СВОЙ ЖЕ файл.
//
// Списки для 1.0 и для 0.x обязаны быть разными (§7.26): у 0.x правило несёт
// `match`/`outbound`, у 1.0 — `body`/`refs`; общий список молчал бы там, где
// должен ругаться, и ругался бы на легаси-кейсы корпуса, у которых таких
// предупреждений в ожиданиях нет.

import (
	"encoding/json"
	"reflect"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// jsonKeys — json-имена полей структуры, включая встроенные.
//
// Встроенные разворачиваются (state.Source встраивает state.Node, и его ключи
// — такие же ключи записи); поле с `json:"-"` ключом не является: его в файле
// не бывает по построению (например, state.Source.Label — legacy-вход).
func jsonKeys(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" || !f.IsExported() {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if f.Anonymous && name == "" {
				walk(f.Type)
				continue
			}
			if name == "" {
				continue
			}
			out[name] = true
		}
	}
	walk(t)
	return out
}

var (
	root10Keys   = jsonKeys(reflect.TypeOf(Backup10{}))
	source10Keys = jsonKeys(reflect.TypeOf(Source10{}))
	// node10Keys — член папки: state.Node целиком (в файле у него те же
	// ключи, что у корневого узла, минус поля контейнера).
	node10Keys = jsonKeys(reflect.TypeOf(state.Node{}))
	rule10Keys = jsonKeys(reflect.TypeOf(state.Rule{}))
	dns10Keys  = jsonKeys(reflect.TypeOf(state.DNSOptions{}))
	// dnsServer10Keys и dnsRule10Keys объявлены порознь: у сервера есть
	// `tag`, у правила — `name`/`id`, и общий список пропустил бы чужое поле
	// в обе стороны. `vars` из общего списка сервера снят: ключ законен
	// только у записи шаблонного сервера (SPEC 129 Н1, ловушка Л5), и
	// рефлексия по типу этого не различает — его объявляет dnsServerKindKeys10.
	dnsServer10Keys = withoutKeys(jsonKeys(reflect.TypeOf(state.DNSServer{})), "vars")
	dnsRule10Keys   = jsonKeys(reflect.TypeOf(state.DNSRule{}))
	// sections10Keys — секции узла: `rules` и `dns`. Внутрь записей секции
	// обход спускается теми же списками, что у корневых, — форма одна
	// (NODE_SECTIONS.md §1), и вторая таблица разъехалась бы с первой.
	sections10Keys    = jsonKeys(reflect.TypeOf(state.NodeSections{}))
	sectionsDNS10Keys = jsonKeys(reflect.TypeOf(state.NodeSectionsDNS{}))
	// origin10Keys / detour10Keys / policy10Keys / update10Keys — вложенные
	// объекты записи источника.
	origin10Keys = jsonKeys(reflect.TypeOf(state.Origin{}))
	link10Keys   = jsonKeys(reflect.TypeOf(state.NodeLink{}))
	policy10Keys = jsonKeys(reflect.TypeOf(state.TagPolicy{}))
	update10Keys = jsonKeys(reflect.TypeOf(state.UpdateSpec{}))
	group10Keys  = jsonKeys(reflect.TypeOf(state.AutoGroup{}))
	// replace10Keys — свёртка формой состояния (контракт 1.1.78).
	replace10Keys = jsonKeys(reflect.TypeOf(state.FolderReplace{}))
	// auto10Keys — параметры автогруппы: одна каноническая форма и у свёртки,
	// и у Направления.
	auto10Keys = jsonKeys(reflect.TypeOf(configtypes.DirectionAuto{}))
)

// Поля стороны LxBox (TASKS_LXBOX.md §16.7, BACKUP.md §2 «Поля стороны
// LxBox»): объявлены в схеме с «Поддержка: LxBox», и лаунчер их игнорирует
// МОЛЧА — в состояние не кладёт (у его типов таких полей нет, декодер их
// пропускает) и неизвестными не называет (§1: объявленное чужое поле — не
// «непонятое»). Ключ внешней карты — `kind` записи, "*" — любой вид.
//
// Отдельными таблицами, а не полями типов: ключ файла, которого у лаунчера в
// модели нет и не будет, в state-типе был бы полем-призраком, которое тихо
// переезжало бы в state.json.
var (
	lxboxSourceKeys10 = map[string]map[string]bool{
		"subscription": {"detour_policy": true, "import_rules": true, "import_rules_enabled": true, "on_update_action": true},
		// tag_policy у сервера — ключ записи и так (Source10.TagPolicy); у
		// корневого сервера лаунчер его отбрасывает при разборе (decode10Source).
		"server": {"detour_policy": true},
		"folder": {"detour_policy": true, "ping_url": true, "ping_timeout_ms": true},
		"chain":  {"label": true},
	}
	lxboxGroupKeys10 = map[string]bool{"members_rule": true, "pool_badge": true}
	lxboxRuleKeys10  = map[string]map[string]bool{
		"*":      {"dns": true, "resolve": true},
		"srs":    {"update_interval_hours": true},
		"inline": {"verbatim": true},
	}
	lxboxDNSServerKeys10 = map[string]map[string]bool{
		"*": {"description": true},
	}
	// dnsServerKindKeys10 — ключи записи DNS-сервера, законные только у своего
	// вида: `vars` у `template` (SPEC 129). У `user`/`preset` ключ
	// называется backup_unknown_field.
	dnsServerKindKeys10 = map[string]map[string]bool{
		"template": {"vars": true},
	}
)

// withoutKeys — набор ключей без перечисленных.
func withoutKeys(base map[string]bool, drop ...string) map[string]bool {
	for _, k := range drop {
		delete(base, k)
	}
	return base
}

// mergeKindKeys — объединение таблиц «вид → ключи» (поля стороны LxBox и
// ключи своей стороны, законные у одного вида).
func mergeKindKeys(tables ...map[string]map[string]bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, t := range tables {
		for kind, keys := range t {
			if out[kind] == nil {
				out[kind] = map[string]bool{}
			}
			for k := range keys {
				out[kind][k] = true
			}
		}
	}
	return out
}

// scanUnknown10 обходит файл 1.0 и перечисляет всё, чего нет в модели.
//
// Обязанности те же, что у scanUnknown (П3/П6): `extensions` любой глубины —
// ОДИН warning на файл с перечнем записей, прочее — warning с полным путём.
// Различие только в наборах ключей и в форме корня: секция источников одна,
// а правила и DNS лежат записями состояния.
func scanUnknown10(data []byte) []Warning {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}

	sc := &unknownScan{}
	sc.object("", root, root10Keys)
	sc.nested(root, "exported_by", exportedByKeys)
	sc.nested(root, "route", routeKeys)

	if dns, ok := rawObject(root, "dns"); ok {
		sc.object("dns", dns, dns10Keys)
		sc.arrayKinds(dns, "dns.servers", "servers", dnsServer10Keys,
			mergeKindKeys(lxboxDNSServerKeys10, dnsServerKindKeys10), "tag", nil)
		sc.array(dns, "dns.rules", "rules", dnsRule10Keys, "name", nil)
	}

	sc.arrayKinds(root, "sources", "sources", source10Keys, lxboxSourceKeys10, "tag", func(where string, item map[string]json.RawMessage) {
		sc.scanSourceBody10(where, item)
		sc.array(item, where+".nodes", "nodes", node10Keys, "tag", sc.scanSourceBody10)
	})
	sc.array(root, "directions", "directions", directionKeys, "tag", sc.scanDirectionBody)
	sc.arrayKinds(root, "rules", "rules", rule10Keys, lxboxRuleKeys10, "name", nil)
	sc.array(root, "warp", "warp", warpKeys, "type", nil)

	return sc.warnings()
}

// scanSourceBody10 — вложенные уровни одной записи источника (или узла папки).
//
// Общий обход для корневой записи и для члена `nodes[]`: форма у них одна
// (state.Node внутри state.Source), и второй обход разъехался бы с первым.
// Внутрь `body` не спускаемся намеренно: это объект sing-box, его ключи ведёт
// ядро, а не таблица бэкапа.
func (sc *unknownScan) scanSourceBody10(where string, item map[string]json.RawMessage) {
	sc.nested2(item, where, "origin", origin10Keys)
	sc.nested2(item, where, "detour", link10Keys)
	sc.nested2(item, where, "tag_policy", policy10Keys)
	sc.nested2(item, where, "update", update10Keys)
	sc.nested2(item, where, "identity", identity10Keys)
	sc.array(item, where+".hops", "hops", link10Keys, "tag", nil)
	if group, ok := rawObject(item, "group"); ok {
		sc.object(joinPath(where, "group"), group, withKeys(group10Keys, lxboxGroupKeys10))
		sc.array(group, joinPath(where, "group")+".members", "members", link10Keys, "tag", nil)
		// Умолчание — ссылка той же формы, что член. Строкой (dev-форма)
		// обходить нечего: её терпит чтение группы (state.AutoGroup).
		sc.nested2(group, joinPath(where, "group"), "default", link10Keys)
	}
	if replace, ok := rawObject(item, "replace"); ok {
		sc.object(joinPath(where, "replace"), replace, replace10Keys)
		sc.nested2(replace, joinPath(where, "replace"), "auto", auto10Keys)
	}
	if sections, ok := rawObject(item, "sections"); ok {
		at := joinPath(where, "sections")
		sc.object(at, sections, sections10Keys)
		sc.array(sections, at+".rules", "rules", rule10Keys, "name", nil)
		if dns, ok := rawObject(sections, "dns"); ok {
			sc.object(at+".dns", dns, sectionsDNS10Keys)
			sc.array(dns, at+".dns.servers", "servers", dnsServer10Keys, "tag", nil)
			sc.array(dns, at+".dns.rules", "rules", dnsRule10Keys, "name", nil)
		}
	}
}

// arrayKinds — array с набором известных ключей по виду записи: base плюс
// поля стороны LxBox для её `kind` (и для "*").
func (sc *unknownScan) arrayKinds(parent map[string]json.RawMessage, where, key string, base map[string]bool, byKind map[string]map[string]bool, labelKey string, deeper func(string, map[string]json.RawMessage)) {
	raw, ok := parent[key]
	if !ok {
		return
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return
	}
	for i, item := range items {
		entry := where + "[" + entryLabel(item, labelKey, i) + "]"
		var kind string
		if rawKind, ok := item["kind"]; ok {
			_ = json.Unmarshal(rawKind, &kind)
		}
		sc.object(entry, item, withKeys(base, byKind["*"], byKind[kind]))
		if deeper != nil {
			deeper(entry, item)
		}
	}
}

// withKeys — объединение наборов ключей; без добавок — сам base.
func withKeys(base map[string]bool, extra ...map[string]bool) map[string]bool {
	n := 0
	for _, e := range extra {
		n += len(e)
	}
	if n == 0 {
		return base
	}
	out := make(map[string]bool, len(base)+n)
	for k := range base {
		out[k] = true
	}
	for _, e := range extra {
		for k := range e {
			out[k] = true
		}
	}
	return out
}

// identity10Keys — ключи объекта identity.
//
// Список тут ЯВНЫЙ, а не рефлексией: он перечисляет ключи КОНТРАКТА, включая
// mobile-only (device_os/ver_os/device_model), которые лаунчер не применяет —
// их считает сам импорт (importIdentity10) и выдаёт ОДИН
// backup_source_identity_dropped с перечнем. Не будь их здесь, та же потеря
// дала бы вдобавок backup_unknown_field — два предупреждения об одном.
//
// Рефлексия по state.SubscriptionIdentity дала бы сегодня тот же набор, но
// связала бы перечень ключей ФАЙЛА с составом полей состояния: сняли поле из
// структуры (лаунчеру оно не нужно) — и ключ файла молча стал «неизвестным».
var identity10Keys = map[string]bool{
	"user_agent": true, "hwid": true, "send_hwid": true,
	"hash_device_model": true, "device_os": true, "ver_os": true,
	"device_model": true,
}

// rawObject — вложенный объект записи; второй возврат = его там нет (или он
// не объект — тогда обходить нечего, а о типе скажет терпимый разбор).
func rawObject(parent map[string]json.RawMessage, key string) (map[string]json.RawMessage, bool) {
	raw, ok := parent[key]
	if !ok {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil, false
	}
	return obj, true
}
