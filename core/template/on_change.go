package template

import (
	"encoding/json"
	"sort"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// onChangeMaxDepth — жёсткий предохранитель от бесконечной рекурсии сверх
// fixpoint-guard (защита на случай бага в шаблоне, который guard не ловит:
// например длинная ациклическая, но случайно бесконечная цепочка). Запас
// с большим отрывом от любой реалистичной цепочки on_change в шаблоне.
const onChangeMaxDepth = 32

// ApplyOnChange применяет `on_change.set` изменённой переменной `changed`
// (паритет с mobile _applyOnChange, settings_screen.dart:258-277; SPEC 103).
//
// Семантика (эталон LxBox):
//  1. У var `changed` может быть `on_change: {"set": {"@target": <#if>, ...}}`.
//  2. Для каждого target его `#if` вычисляется до скаляра (EvalIfScalar) В
//     КОНТЕКСТЕ уже НОВОГО значения `changed` — вызывающий обязан записать
//     новое значение в stateVars ДО вызова ApplyOnChange.
//  3. Вычисленный скаляр пишется в stateVars[target].
//  4. Если значение target реально изменилось — рекурсивно применяется
//     on_change САМОЙ target (цепочка). Если не изменилось — fixpoint-guard
//     обрывает рекурсию по этой ветке (защита от циклов A→B→A).
//
// stateVars мутируется на месте. Возвращает имена всех переменных, чьё
// значение было изменено (в порядке применения; может содержать дубликаты,
// если несколько veтвей цепочки сошлись на одной var — вызывающему обычно
// нужен факт «что-то поменялось», не уникальный список).
func ApplyOnChange(changed string, vars []TemplateVar, stateVars map[string]string, target TargetSpec) []string {
	var touched []string
	visited := make(map[string]int) // имя var -> сколько раз уже была целью в этой цепочке
	applyOnChangeRec(changed, vars, stateVars, target, visited, 0, &touched)
	return touched
}

func applyOnChangeRec(name string, vars []TemplateVar, stateVars map[string]string, target TargetSpec, visited map[string]int, depth int, touched *[]string) {
	if depth >= onChangeMaxDepth {
		debuglog.WarnLog("ApplyOnChange: превышена максимальная глубина (%d) на var %q — обрываю (возможный цикл в шаблоне)", onChangeMaxDepth, name)
		return
	}
	// Жёсткий предохранитель дополнительно к fixpoint-guard: одна и та же var
	// не может стать целью on_change более, чем len(vars)+1 раз в одной
	// цепочке — этого достаточно для любой ациклической топологии и отсекает
	// патологические случаи, которые fixpoint по значению не ловит (например
	// осциллирующие значения A→B→A с чередованием двух разных скаляров).
	maxVisits := len(vars) + 1
	visited[name]++
	if visited[name] > maxVisits {
		debuglog.WarnLog("ApplyOnChange: var %q встречена в цепочке on_change более %d раз — обрываю (цикл)", name, maxVisits)
		return
	}

	if stateVars == nil {
		return
	}
	v, ok := VarByName(vars, name)
	if !ok {
		return
	}
	// Канон — помеченный `#set`; легаси `set` читается бессрочно (SPEC 107).
	setRaw, ok := v.OnChange["#set"]
	if !ok {
		setRaw, ok = v.OnChange["set"]
	}
	if !ok || len(setRaw) == 0 {
		return
	}
	var set map[string]json.RawMessage
	if err := json.Unmarshal(setRaw, &set); err != nil {
		debuglog.WarnLog("ApplyOnChange: var %q on_change.set невалиден: %v", name, err)
		return
	}

	// Цели — в сортированном порядке ключей: обход map в Go случаен, а при
	// пересекающихся цепочках (#set двух целей сходятся в одну) результат
	// зависел бы от порядка. Dart-эталон LxBox упорядочен — расходиться нельзя.
	refs := make([]string, 0, len(set))
	for targetRef := range set {
		refs = append(refs, targetRef)
	}
	sort.Strings(refs)
	for _, targetRef := range refs {
		ifNode := set[targetRef]
		targetName := strings.TrimPrefix(strings.TrimSpace(targetRef), "@")
		if targetName == "" {
			continue
		}
		resolvedStr, ok := EvalIfScalar(ifNode, vars, stateVars, target)
		if !ok {
			continue // невычислимый #if (ветка не выбрана / не скаляр) — цель не трогаем
		}
		prev := stateVars[targetName]
		if prev == resolvedStr {
			continue // fixpoint: значение не изменилось — цепочка по этой ветке обрывается
		}
		stateVars[targetName] = resolvedStr
		*touched = append(*touched, targetName)
		applyOnChangeRec(targetName, vars, stateVars, target, visited, depth+1, touched)
	}
}
