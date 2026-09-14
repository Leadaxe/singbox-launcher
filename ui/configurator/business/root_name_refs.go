// File root_name_refs.go — ссылки на КОРНЕВОЕ ИМЯ: перепись при
// переименовании и гашение при удалении (NODE_LINK.md §6).
//
// Корневое имя — тег верхнего узла (server, chain, auto), Направления и его
// `-auto`, свёртки папки и её `-auto`. На него смотрят два класса ссылок:
//
//   - NodeLink без folder_id — detour корневых записей и членов папок,
//     позиции цепочек, члены корневых групп (реестр editNodeLinks,
//     node_move.go);
//   - ссылки ПО ИМЕНИ строкой — цели правил, route.final, опции
//     (addOutbounds), умолчание селектора (options.default) и литеральное
//     умолчание отбора (preferredDefault) Направлений — в теле и в USER-патче,
//     outbound-переменные пресетов, detour DNS-серверов.
//
// Операций над именем четыре — переименование Направления, свёртки и верхнего
// узла, удаление верхнего узла, — и отличаются они только тем, что делают с
// найденной ссылкой. Поэтому обход один (editRootNameRefs): список мест
// обязан оставаться полным, и появившаяся ссылка на корневое имя добавляется
// сюда, а не отдельной правкой в одной из операций (тот же принцип, что у
// граф-санитайзера сборки).
package business

import (
	"encoding/json"
	"strings"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// rootRefAction — что операция делает с найденной ссылкой на имя.
type rootRefAction int

const (
	// rootRefMiss — ссылка не на это имя.
	rootRefMiss rootRefAction = iota
	// rootRefRename — ссылка переписывается на новое имя.
	rootRefRename
	// rootRefClear — цели больше нет: NodeLink гаснет, элемент списка
	// addOutbounds уходит. Одиночная цель (правило, route.final, переменная
	// пресета, detour DNS, умолчание Направления) остаётся как есть и только
	// называется: удаление Направления оставляет такие цели висеть, и
	// удаление узла повторяет ровно это (решение владельца 15.09.2026) —
	// новую цель выбирает пользователь, а штатная загрузка чистит правило
	// без цели сама.
	rootRefClear
	// rootRefName — ссылка не трогается, только называется (перенос узла в
	// папку: адрес у ссылок по имени переписать нельзя).
	rootRefName
)

// rootRefDecide — решение операции по одному имени: действие и замена (для
// rootRefRename).
type rootRefDecide func(name string) (string, rootRefAction)

// rootRenames — решение «переименовать» по карте «старое имя → новое».
//
// Карта, а не пара: переименование Направления несёт и тег, и его `-auto`, и
// правка обязана пройти ОДНИМ проходом — последовательные проходы по парам
// переписали бы `x` → `x-auto` и тут же `x-auto` → `x-auto-auto`.
func rootRenames(renames map[string]string) rootRefDecide {
	return func(name string) (string, rootRefAction) {
		if next, ok := renames[name]; ok {
			return next, rootRefRename
		}
		return name, rootRefMiss
	}
}

// rootNameIs — решение act для одного имени tag.
func rootNameIs(tag string, act rootRefAction) rootRefDecide {
	return func(name string) (string, rootRefAction) {
		if name == tag {
			return name, act
		}
		return name, rootRefMiss
	}
}

// editRootNameRefs — ЕДИНЫЙ обход ссылок на корневые имена.
//
// Возвращает имена задетых (источники — как в списке Sources, записи — как
// их зовут в своих вкладках; без повторов, в порядке обхода) и число задетых
// ссылок.
func editRootNameRefs(model *wizardmodels.WizardModel, decide rootRefDecide) ([]string, int) {
	if model == nil {
		return nil, 0
	}
	var names []string
	seen := map[string]bool{}
	note := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	count := 0
	// single — одиночная цель: переписывается при rename, иначе только
	// называется (см. rootRefClear).
	single := func(v *string, holder string) {
		next, act := decide(*v)
		if act == rootRefMiss {
			return
		}
		if act == rootRefRename {
			*v = next
		}
		count++
		note(holder)
	}

	// 1. NodeLink корневого пространства — реестром узловых ссылок. Член
	// группы ВНУТРИ контейнера без folder_id адресует свой контейнер, а не
	// корень (NODE_LINK.md §5.1 № 8), и корневое имя его не касается.
	sources, linked := editNodeLinks(model, func(link corestate.NodeLink, space string) (corestate.NodeLink, linkEdit) {
		if strings.TrimSpace(link.FolderID) != "" || space != "" {
			return link, linkKeep
		}
		next, act := decide(link.Tag)
		switch act {
		case rootRefRename:
			return corestate.NodeLink{Tag: next}, linkReplace
		case rootRefClear:
			return corestate.NodeLink{}, linkDrop
		}
		return link, linkKeep
	})
	count += linked
	for _, name := range sources {
		note(name)
	}

	// 2. Цели правил.
	for _, rs := range model.CustomRules {
		if rs != nil {
			single(&rs.SelectedOutbound, firstNonEmptyRefName(rs.Rule.Label, rs.Rule.Description))
		}
	}

	// 3. Маршрут по умолчанию. Два места хранения одного значения
	// (SelectedFinalOutbound + SettingsVars["route_final"]) синхронны по
	// построению — правим оба, иначе одно перебьёт другое на сохранении.
	single(&model.SelectedFinalOutbound, "route.final")
	if model.SettingsVars != nil {
		if v, ok := model.SettingsVars["route_final"]; ok {
			if next, act := decide(v); act == rootRefRename {
				model.SettingsVars["route_final"] = next
			}
		}
	}

	// 4. Направления: опции (addOutbounds), умолчание селектора
	// (options.default) и литеральное умолчание отбора (preferredDefault) —
	// в теле собственной записи и в USER-патче любой записи. У записи-ссылки
	// на шаблон или пресет тело живёт в базе (в модели оно пусто) и не
	// трогается; USER-патч — правка пользователя поверх базы, и ссылка в нём
	// такая же, как в теле. Патч пресета не трогается: его пересобирает sync.
	for i := range model.GlobalOutbounds {
		d := &model.GlobalOutbounds[i]
		hits := 0
		if d.Ref == "" {
			var n int
			d.AddOutbounds, n = editNameList(d.AddOutbounds, decide)
			hits += n
			d.Options, n = editOptionsDefault(d.Options, decide)
			hits += n
			d.PreferredDefault, n = editDefaultLiteral(d.PreferredDefault, decide)
			hits += n
		}
		for u := range d.Updates {
			up := &d.Updates[u]
			if up.Ref != configtypes.RefUser || up.Patch == nil {
				continue
			}
			hits += editUserPatch(up, decide)
		}
		if hits > 0 {
			count += hits
			note(d.Tag)
		}
	}

	// 5. Outbound-переменные пресетов (preset.vars[].type == "outbound").
	//
	// Тип переменной не проверяем: значение, совпавшее с корневым именем, и
	// есть ссылка на него.
	for _, ref := range model.PresetRefs {
		if ref == nil {
			continue
		}
		for varName, val := range ref.Vars {
			next, act := decide(val)
			if act == rootRefMiss {
				continue
			}
			if act == rootRefRename {
				ref.Vars[varName] = next
			}
			count++
			note(firstNonEmptyRefName(ref.Ref, "preset"))
		}
	}

	// 6. detour DNS-серверов.
	count += editDNSDetours(model, decide, note)

	return names, count
}

// editNameList — список имён (addOutbounds): переименование на месте,
// гашение выносит элемент, называние оставляет как есть. Возвращает список и
// число задетых элементов; незадетый список возвращается тем же срезом.
func editNameList(list []string, decide rootRefDecide) ([]string, int) {
	hits := 0
	out := list[:0:0]
	for _, name := range list {
		next, act := decide(name)
		switch act {
		case rootRefMiss:
			out = append(out, name)
			continue
		case rootRefRename:
			out = append(out, next)
		case rootRefClear:
			// Элемент уходит вместе с целью: Направление живёт остальным
			// составом.
		default:
			out = append(out, name)
		}
		hits++
	}
	if hits == 0 {
		return list, 0
	}
	return out, hits
}

// editOptionsDefault — `options.default` селектора: одиночная цель по имени.
// Переписывается копией карты — карта могла делиться с буфером формы.
func editOptionsDefault(opts map[string]interface{}, decide rootRefDecide) (map[string]interface{}, int) {
	def, ok := opts["default"].(string)
	if !ok || def == "" {
		return opts, 0
	}
	next, act := decide(def)
	switch act {
	case rootRefMiss:
		return opts, 0
	case rootRefRename:
		out := copyStringKeyMap(opts)
		out["default"] = next
		return out, 1
	}
	return opts, 1
}

// editDefaultLiteral — литеральное умолчание отбора (см.
// directionDefaultLiteral): одиночная цель по имени.
func editDefaultLiteral(pd map[string]interface{}, decide rootRefDecide) (map[string]interface{}, int) {
	name, negated, ok := directionDefaultLiteral(pd)
	if !ok {
		return pd, 0
	}
	next, act := decide(name)
	switch act {
	case rootRefMiss:
		return pd, 0
	case rootRefRename:
		return withDirectionDefaultLiteral(pd, next, negated), 1
	}
	return pd, 1
}

// editUserPatch — те же три места в USER-патче Направления: `addOutbounds`
// (после JSON — []interface{}), `options.default`, `preferredDefault`.
// Возвращает число задетых ссылок; изменённые поля патча заменяются новыми
// значениями, а не правятся на месте.
func editUserPatch(up *configtypes.OutboundUpdate, decide rootRefDecide) int {
	hits := 0
	switch list := up.Patch["addOutbounds"].(type) {
	case []string:
		next, n := editNameList(list, decide)
		if n > 0 {
			up.Patch["addOutbounds"] = next
			hits += n
		}
	case []interface{}:
		names := make([]string, 0, len(list))
		for _, item := range list {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		if len(names) == len(list) {
			next, n := editNameList(names, decide)
			if n > 0 {
				arr := make([]interface{}, len(next))
				for k, name := range next {
					arr[k] = name
				}
				up.Patch["addOutbounds"] = arr
				hits += n
			}
		}
	}
	if opts, ok := up.Patch["options"].(map[string]interface{}); ok {
		if next, n := editOptionsDefault(opts, decide); n > 0 {
			up.Patch["options"] = next
			hits += n
		}
	}
	if pd, ok := up.Patch["preferredDefault"].(map[string]interface{}); ok {
		if next, n := editDefaultLiteral(pd, decide); n > 0 {
			up.Patch["preferredDefault"] = next
			hits += n
		}
	}
	return hits
}

// copyStringKeyMap — поверхностная копия карты.
func copyStringKeyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// directionDefaultLiteral — имя, которое умолчание Направления называет
// ЛИТЕРАЛОМ: `X` или `!X` (язык паттернов, configtypes/matcher.go).
//
// Только литерал — ссылка на узел по имени. Регулярка `/…/` — отбор по
// финальному тегу, а не ссылка (NODE_LINK.md §4.3), и операции над именем её
// не трогают: правка регулярки поменяла бы выбор и для других узлов.
func directionDefaultLiteral(pd map[string]interface{}) (name string, negated bool, ok bool) {
	raw, _ := pd["tag"].(string)
	if raw == "" || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "!/") {
		return "", false, false
	}
	if strings.HasPrefix(raw, "!") {
		return strings.TrimPrefix(raw, "!"), true, true
	}
	return raw, false, true
}

// withDirectionDefaultLiteral — копия умолчания с литералом на новое имя.
// Копия, а не правка на месте: карта может делиться с буфером формы.
func withDirectionDefaultLiteral(pd map[string]interface{}, name string, negated bool) map[string]interface{} {
	out := copyStringKeyMap(pd)
	if negated {
		name = "!" + name
	}
	out["tag"] = name
	return out
}

// editDNSDetours — поле detour в DNS-серверах.
//
// Серверы хранятся сырым JSON (форма записи задаётся шаблоном и ядром, а не
// нашей структурой), поэтому правим точечно: разбираем в map, меняем одно
// поле, собираем обратно. Сервер, который не разобрался или не ссылается на
// имя, остаётся байт-в-байт прежним — переписывать чужой JSON целиком ради
// несделанной правки значит менять форматирование и порядок ключей на ровном
// месте.
func editDNSDetours(model *wizardmodels.WizardModel, decide rootRefDecide, note func(string)) int {
	count := 0
	for i, raw := range model.DNSServers {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		rawDetour, ok := obj["detour"]
		if !ok {
			continue
		}
		var detour string
		if err := json.Unmarshal(rawDetour, &detour); err != nil {
			continue
		}
		next, act := decide(detour)
		if act == rootRefMiss {
			continue
		}
		var tag string
		_ = json.Unmarshal(obj["tag"], &tag)
		if act == rootRefRename {
			encoded, err := json.Marshal(next)
			if err != nil {
				continue
			}
			obj["detour"] = encoded
			updated, err := json.Marshal(obj)
			if err != nil {
				continue
			}
			model.DNSServers[i] = updated
		}
		count++
		note(firstNonEmptyRefName(tag, "DNS server"))
	}
	return count
}

// RenameRootNodeRefs переписывает ссылки на ВЕРХНИЙ узел (server, chain),
// чей тег сменился с oldTag на newTag, и возвращает имена задетых источников
// и записей.
//
// Решение владельца 15.09.2026: переименование верхнего узла ведёт ссылки за
// ним, как у узла контейнера (NODE_LINK.md §6 правило 1), — прежний сброс
// оставлял маршрут без хопа там, где узел никуда не делся. Переписываются все
// виды: NodeLink `{tag: old}` и ссылки по имени.
//
// Выбор в селекторах живого ядра адресован финальным тегом и после
// переименования протухает — предупреждает вызывающий UI
// (showStaleSelectionDialog).
func RenameRootNodeRefs(model *wizardmodels.WizardModel, oldTag, newTag string) []string {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if model == nil || oldTag == "" || newTag == "" || oldTag == newTag || rootNameOwnedElsewhere(model, oldTag) {
		return nil
	}
	names, _ := editRootNameRefs(model, rootRenames(map[string]string{oldTag: newTag}))
	InvalidateNodePool(model)
	return names
}

// RootNodeTagTaken — занят ли тег в корневом пространстве кем-то, кроме самого
// узла с тегом exceptTag: верхним узлом, Направлением или его `-auto`,
// свёрткой или её двойником, служебным тегом шаблона или пресета.
//
// Гард переименования верхнего узла: ссылки идут за узлом на новое имя
// (RenameRootNodeRefs), и занятое имя увело бы их на ЧУЖУЮ цель. В отличие от
// DirectionTagTaken, пара `<tag>-auto` здесь не проверяется: двойника у узла
// нет, и чужое `X-auto` имени `X` у узла не мешает.
func RootNodeTagTaken(model *wizardmodels.WizardModel, tag, exceptTag string) bool {
	tag = strings.TrimSpace(tag)
	exceptTag = strings.TrimSpace(exceptTag)
	if model == nil || tag == "" || tag == exceptTag {
		return false
	}
	owners := ModelTagOwners(model)
	delete(owners, exceptTag)
	_, taken := owners[tag]
	return taken
}

// ClearRootNodeRefs гасит ссылки на удалённый ВЕРХНИЙ узел (server, chain,
// auto) и возвращает имена задетых источников и записей.
//
// Решение владельца 15.09.2026: ссылки, которые узел использовали, гаснут
// вместе с ним — detour снимается, позиция выходит из цепочки, член — из
// группы, опция — из addOutbounds Направления. Одиночные цели по имени
// остаются висеть, как при удалении Направления, и только называются
// (rootRefClear). Вызывающий зовёт это ПОСЛЕ удаления узла из модели.
func ClearRootNodeRefs(model *wizardmodels.WizardModel, tag string) []string {
	tag = strings.TrimSpace(tag)
	if model == nil || tag == "" || rootNameOwnedElsewhere(model, tag) {
		return nil
	}
	names, _ := editRootNameRefs(model, rootNameIs(tag, rootRefClear))
	InvalidateNodePool(model)
	return names
}

// rootNameOwnedElsewhere — носит ли имя ещё кто-то в корне: верхний узел,
// Направление или его `-auto`, свёртка или её `-auto`.
//
// Операции над именем верхнего узла зовутся ПОСЛЕ правки модели: узел уже
// переименован или удалён. Если имя и после неё занято, оно было общим у двух
// владельцев — состояние, о котором и так говорит гард сборки, — и ссылки по
// нему сборка разрешит к оставшемуся. Переписать или погасить их значило бы
// задеть ссылки ЧУЖОЙ цели (NODE_LINK.md §6 правило 3), поэтому операция их не
// трогает.
func rootNameOwnedElsewhere(model *wizardmodels.WizardModel, name string) bool {
	for _, tag := range ModelRootNodeTags(model) {
		if tag == name {
			return true
		}
	}
	for _, tag := range ModelReplaceTags(model) {
		if tag == name {
			return true
		}
	}
	for i := range model.GlobalOutbounds {
		d := &model.GlobalOutbounds[i]
		if d.Tag == name || (d.Auto != nil && d.Tag != "" && d.AutoTag() == name) {
			return true
		}
	}
	return false
}

// RenameFoldRefs переписывает ссылки на свёртку папки (`replace.tag`), чей
// тег сменился правкой источника, и возвращает число переписанных ссылок.
//
// Свёртка — корневое имя, как Направление, и переименование идёт тем же
// обходом (NODE_LINK.md §6 правило 1): иначе правило, route.final, позиция
// цепочки или detour повисли бы на имени, которого в конфиге больше нет.
// Двойник `<tag>-auto` переименовывается, только если он есть и до, и после
// правки (режим both): у свёртки без него `<tag>-auto` может быть чужим
// именем, а у исчезнувшего двойника вести ссылку некуда.
//
// Выбор в селекторах живого ядра по прежнему тегу протухает — об этом
// предупреждает вызывающий (staleSelectionAfterEdit).
func RenameFoldRefs(model *wizardmodels.WizardModel, before, after *corestate.FolderReplace) int {
	if model == nil || before == nil || after == nil {
		return 0
	}
	oldTag := strings.TrimSpace(before.Tag)
	newTag := strings.TrimSpace(after.Tag)
	if oldTag == "" || newTag == "" || oldTag == newTag {
		return 0
	}
	renames := map[string]string{oldTag: newTag}
	if before.Mode == corestate.FolderReplaceBoth && after.Mode == corestate.FolderReplaceBoth {
		renames[oldTag+"-auto"] = newTag + "-auto"
	}
	_, count := editRootNameRefs(model, rootRenames(renames))
	if count > 0 {
		InvalidateNodePool(model)
	}
	return count
}
