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
		for rt.Kind() == reflect.Ptr {
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
	// в обе стороны.
	dnsServer10Keys = jsonKeys(reflect.TypeOf(state.DNSServer{}))
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
	// fold10Keys — свёртка формой контракта (Fold), а не state-ного replace:
	// это одно из исключений тонкого слоя (§6.0).
	fold10Keys = jsonKeys(reflect.TypeOf(Fold{}))
	// auto10Keys — параметры автогруппы: одна каноническая форма и у свёртки,
	// и у Направления.
	auto10Keys = jsonKeys(reflect.TypeOf(configtypes.DirectionAuto{}))
)

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
		sc.array(dns, "dns.servers", "servers", dnsServer10Keys, "tag", nil)
		sc.array(dns, "dns.rules", "rules", dnsRule10Keys, "name", nil)
	}

	sc.array(root, "sources", "sources", source10Keys, "tag", func(where string, item map[string]json.RawMessage) {
		sc.scanSourceBody10(where, item)
		sc.array(item, where+".nodes", "nodes", node10Keys, "tag", sc.scanSourceBody10)
	})
	sc.array(root, "directions", "directions", directionKeys, "tag", sc.scanDirectionBody)
	sc.array(root, "rules", "rules", rule10Keys, "name", nil)
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
		sc.object(joinPath(where, "group"), group, group10Keys)
		sc.array(group, joinPath(where, "group")+".members", "members", link10Keys, "tag", nil)
	}
	if fold, ok := rawObject(item, "fold"); ok {
		sc.object(joinPath(where, "fold"), fold, fold10Keys)
		sc.nested2(fold, joinPath(where, "fold"), "auto", auto10Keys)
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
