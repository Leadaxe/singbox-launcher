// File rule_identity.go — SPEC 063: identity правила = pure function over Rule.
//
// До SPEC 063 `state.Rule` хранил поле `ID string`, заполняемое из label на
// первой конверсии legacy → v6. Это поле было всегда вычислимо из других
// данных (`body.name` для inline/srs, `Ref` для preset) — pure redundancy.
//
// После SPEC 063 поле `ID` удалено; identity берётся через `StableRuleID(r)`.
// Единая точка истины для всех callsite'ов (build pipeline, UI slot lookup,
// orphan GC). Изменяешь алгоритм identity — изменяешь его здесь.
package state

import "strconv"

// StableRuleID — pure-function identity правила. Не stored, не serialized.
//
// Маршруты по Kind:
//
//	preset    → r.Ref           (template preset_id; уникален per template)
//	inline    → sanitize(body.name)
//	srs       → sanitize(body.name)
//	unknown / undecodable body / empty name → "unnamed"
//
// Возвращаемое значение SAFE для использования как key в map'ах и как часть
// sing-box tag'а (см. `"user:" + StableRuleID(r)` в build/rules_pipeline.go).
//
// Уникальность по списку rules валидируется на уровне UI add/edit (юзер не
// может создать два правила с одинаковым label) и в state validator
// (см. core/state/load.go).
func StableRuleID(r Rule) string {
	if r.Kind == RuleKindPreset {
		return r.Ref
	}
	body, err := r.DecodeBody()
	if err != nil {
		return "unnamed"
	}
	var name string
	switch b := body.(type) {
	case *InlineBody:
		name = b.Name
	case *SrsBody:
		name = b.Name
	default:
		return "unnamed"
	}
	if name == "" {
		return "unnamed"
	}
	return sanitizeIDPart(name)
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
// без миграции).
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
