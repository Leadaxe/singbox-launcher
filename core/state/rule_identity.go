// File rule_identity.go — SPEC 063: identity правила = pure function over Rule.
//
// До SPEC 063 `state.Rule` хранил поле `ID string`, заполняемое из label на
// первой конверсии legacy → v6. Это поле было всегда вычислимо из других
// данных (`body.name` для inline/srs, `Ref` для preset) — pure redundancy.
//
// После SPEC 063 поле `ID` удалено; identity берётся через `StableRuleID(r)`.
// SPEC 127 (state v8): имя переехало из тела в поле записи `Name` — строка та
// же, алгоритм `sanitizeIDPart` не тронут, значит и id прежний.
// Единая точка истины для всех callsite'ов (build pipeline, UI slot lookup,
// orphan GC). Изменяешь алгоритм identity — изменяешь его здесь.
package state

import "strconv"

// StableRuleID — pure-function identity правила. Не stored, не serialized.
//
// Маршруты по Kind:
//
//	preset    → r.Ref           (template preset_id; уникален per template)
//	inline    → sanitize(r.Name)
//	srs       → sanitize(r.Name)
//	unknown kind / пустое имя → "unnamed"
//
// Возвращаемое значение SAFE для использования как key в map'ах и как часть
// sing-box tag'а (см. `"user:" + StableRuleID(r)` в build/rules_pipeline.go).
//
// Уникальность по списку rules валидируется на уровне UI add/edit (юзер не
// может создать два правила с одинаковым label) и в state validator
// (см. core/state/load.go).
func StableRuleID(r Rule) string {
	switch r.Kind {
	case RuleKindPreset:
		return r.Ref
	case RuleKindInline, RuleKindSrs:
		if r.Name == "" {
			return "unnamed"
		}
		return sanitizeIDPart(r.Name)
	default:
		return "unnamed"
	}
}

// SrsRuleSetTag — тег записи route.rule_set для i-го (с нуля) набора
// srs-правила с identity id. Единая точка для сборки (resolve_route.go) и
// legacy-проекции (load_v6.go): первый набор держит исторический тег
// "user:<id>", остальные — "user:<id>:2", "user:<id>:3", … Разделитель ':'
// не входит в алфавит sanitizeIDPart, поэтому тег второго набора правила
// "foo" не совпадёт с тегом правила "foo 2" ("user:foo-2").
func SrsRuleSetTag(id string, i int) string {
	if i <= 0 {
		return "user:" + id
	}
	return "user:" + id + ":" + strconv.Itoa(i+1)
}

// sanitizeIDPart — приводит произвольный label к безопасному identifier'у:
// alphanumeric + '-' + '_' остаются, ' ' → '-', остальное (включая не-ASCII)
// отбрасывается. Пустой результат → "rule".
//
// Поведение совпадает с прежним `sanitizeIDPart` из
// `ui/configurator/models/preset_ref_sync.go` (перенесён сюда вместе со
// `StableRuleID`). Менять алгоритм — означает менять identity для всех
// существующих state.json: переписать тесты + warn'нуть юзеров (не делать
// без миграции). На этом id висят теги `rule_set` в конфиге и имена файлов
// srs-кэша.
func sanitizeIDPart(s string) string {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, byte(r))
		} else if r == ' ' {
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "rule"
	}
	return string(out)
}

// CarryRuleMetadata возвращает свежеэмитированные правила с восстановленными
// метаданными, которые лаунчер не редактирует, а только провозит.
//
// Сегодня это `Rule.ID` — необязательное поле другой стороны (ONE_NAMESPACE §1,
// SPEC 127 §0): UI-модели его не хранят (ни `RuleState`, ни `PresetRefState`
// поля под него не имеют), поэтому эмиссия из модели отдаёт запись без `id`, и
// первое же сохранение визарда стирало бы чужие метаданные с диска.
//
// Сопоставление — по `StableRuleID`: это та же identity, по которой UI уже
// сопоставляет слоты оси с записями состояния (`RuleOrderFromAxis`). Одинаковые
// identity разбираются очередью в порядке следования, id уже стоящий на записи
// не перетирается, а лишние старые записи просто не находят пары.
func CarryRuleMetadata(fresh, prev []Rule) []Rule {
	if len(fresh) == 0 || len(prev) == 0 {
		return fresh
	}
	queue := make(map[string][]string, len(prev))
	for _, r := range prev {
		if r.ID == "" {
			continue
		}
		id := StableRuleID(r)
		queue[id] = append(queue[id], r.ID)
	}
	if len(queue) == 0 {
		return fresh
	}
	for i := range fresh {
		if fresh[i].ID != "" {
			continue
		}
		id := StableRuleID(fresh[i])
		q := queue[id]
		if len(q) == 0 {
			continue
		}
		fresh[i].ID = q[0]
		queue[id] = q[1:]
	}
	return fresh
}

// CarryDNSRuleMetadata — то же для DNS-правил: `ID` и `Name` — метаданные
// второй стороны, модель визарда их провозит полями `DNSUserRule`, но записи
// могут приехать и другим путём (импорт бэкапа, правка файла руками).
//
// Сопоставление — по позиции user-записей: у DNS-правил нет ни имени-identity,
// ни ref'а, а порядок пользовательских правил визард сохраняет (слоты
// `DNSRuleOrder`). Заполняются только пустые поля.
func CarryDNSRuleMetadata(fresh, prev []DNSRule) []DNSRule {
	if len(fresh) == 0 || len(prev) == 0 {
		return fresh
	}
	prevUser := make([]DNSRule, 0, len(prev))
	for _, r := range prev {
		if r.Kind == DNSRuleKindUser {
			prevUser = append(prevUser, r)
		}
	}
	if len(prevUser) == 0 {
		return fresh
	}
	next := 0
	for i := range fresh {
		if fresh[i].Kind != DNSRuleKindUser {
			continue
		}
		if next >= len(prevUser) {
			break
		}
		old := prevUser[next]
		next++
		if fresh[i].ID == "" {
			fresh[i].ID = old.ID
		}
		if fresh[i].Name == "" {
			fresh[i].Name = old.Name
		}
	}
	return fresh
}
