package template

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// isIntCastVar: legacy-список вар, которые кастуются в число ПО ИМЕНИ.
//
// Канон (TEMPLATE_LANG §2.2, разрыв C5) — каст по объявленному `type: "int"`,
// а не по имени: иначе новая числовая переменная с любым другим именем уезжает
// в конфиг строкой, и ядро отвергает весь конфиг на decode. Список остаётся
// временным fallback для шаблонов, где у этих вар тип не проставлен.
func isIntCastVar(name string) bool {
	switch name {
	case "tun_mtu", "mixed_listen_port", "proxy_in_listen_port", "urltest_tolerance":
		return true
	}
	return false
}

// intCastBounds — backstop диапазона для int-подстановки (TEMPLATE_LANG §2.2).
// Порт/tolerance/MTU вне uint16 роняют ядро на decode, поэтому значение
// клампится, а не уезжает как есть.
const (
	intCastMin = 0
	intCastMax = 65535
)

// wantsIntCast: переменная подставляется числом, если так объявлен её тип
// (канон) либо если её имя в legacy-списке (fallback для непроставленных типов).
func wantsIntCast(name, declaredType string) bool {
	switch strings.ToLower(strings.TrimSpace(declaredType)) {
	case "int", "number": // "number" — алиас чтения (TEMPLATE_LANG §2.2)
		return true
	}
	return isIntCastVar(name)
}

// runtimeGlobalPrefix — пространство имён runtime-globals в #if predicates (SPEC 067).
// Ссылка вида "@runtime.platform" / "@runtime.arch" / "@runtime.target"
// резолвится не из vars, а из TargetSpec — платформы и роли ЦЕЛЕВОЙ машины
// (SPEC 097; до него — из runtime.GOOS/GOARCH текущей). Namespace расширяемый:
// новые поля добавляются в runtimeGlobalFields + dispatch в lookupVarScalar.
const runtimeGlobalPrefix = "runtime."

// runtimeGlobalFields — известные поля namespace @runtime (без префикса).
// Все поля строковые: bare-форма предиката ("@runtime.x") для globals
// запрещена, сравнение только явное — {"@runtime.target": "remote"}.
var runtimeGlobalFields = map[string]struct{}{
	"platform": {},
	"arch":     {},
	"target":   {},
}

// isRuntimeGlobalRef true для имён вида "runtime.*" (после strip "@").
func isRuntimeGlobalRef(name string) bool {
	return strings.HasPrefix(name, runtimeGlobalPrefix)
}

// isKnownRuntimeGlobal true только для @runtime.<известное поле>.
func isKnownRuntimeGlobal(name string) bool {
	if !strings.HasPrefix(name, runtimeGlobalPrefix) {
		return false
	}
	_, ok := runtimeGlobalFields[name[len(runtimeGlobalPrefix):]]
	return ok
}

// SubstituteVarsInJSONCanon — подстановка по канону TEMPLATE_LANG §5 (D-011,
// разрыв C4). Отличается от обоих прежних режимов тем, что РАЗЛИЧАЕТ два
// случая, которые они путали:
//
//   - имя НЕ объявлено в vars (опечатка автора шаблона) — плейсхолдер "@name"
//     остаётся в выводе как есть + warning. Пустая строка тут маскировала бы
//     ошибку, а падение всего дерева — превращало опечатку в отказ конфига;
//   - имя объявлено, но значения нет (optional-var) — Dropped-каскад §5.1:
//     ключ удаляется из объекта, элемент — из массива, сборка продолжается.
//
// Возвращает дерево, отсортированный список кодов warning'ов без дублей и
// ошибку только на невалидном JSON. Ни один unresolved не роняет сборку —
// деградация пофрагментная. Параметры warning'ов — у
// SubstituteVarsInJSONCanonWarnings.
func SubstituteVarsInJSONCanon(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) ([]byte, []string, error) {
	out, warnings, err := SubstituteVarsInJSONCanonWarnings(data, vars, resolved, target)
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]bool, len(warnings))
	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		if seen[w.Code] {
			continue
		}
		seen[w.Code] = true
		codes = append(codes, w.Code)
	}
	sort.Strings(codes)
	return out, codes, nil
}

// SubstituteVarsInJSONCanonWarnings — та же подстановка по канону, но warning'и
// отдаются с параметрами по реестру (template_var_undeclared {name},
// template_unknown_directive {key}, template_int_* {name, value}), без дублей
// по паре (код, параметры). Порядок не нормирован: обход объекта идёт по map.
func SubstituteVarsInJSONCanonWarnings(data []byte, vars []TemplateVar, resolved map[string]ResolvedVar, target TargetSpec) (json.RawMessage, []TemplateWarning, error) {
	varTypes := make(map[string]string, len(vars))
	declared := make(map[string]bool, len(vars))
	for _, v := range vars {
		if v.Separator {
			continue
		}
		varTypes[v.Name] = v.Type
		declared[v.Name] = true
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root interface{}
	if err := dec.Decode(&root); err != nil {
		return nil, nil, err
	}

	ctx := &canonCtx{varTypes: varTypes, declared: declared, resolved: resolved, target: target}
	substituteWalkCanon(&root, ctx)
	// Dropped на самом верху дерева означает пустой конфиг — вырожденный
	// случай, отдаём пустой объект вместо sentinel'а наружу.
	if _, isDropped := root.(droppedValue); isDropped {
		root = map[string]interface{}{}
	}

	out, err := json.Marshal(root)
	return out, ctx.warnings, err
}

// enableKey — ключ гейта существования узла (SPEC 107, D-065/D-066).
// В отличие от #if суффиксы НЕ допускаются: один гейт на узел, композиция —
// через and/or внутри условия.
const enableKey = "#enable"

// condKey читает ключевое слово движка в КАНОНИЧЕСКОЙ помеченной форме
// ("#and", "#or", "#value", "#else") либо в легаси-форме без "#" (SPEC 107).
//
// Правило языка: "#" — ключевое слово движка, "@" — ссылка на переменную,
// всё остальное — данные. До SPEC 107 оно выполнялось лишь наполовину:
// "#not"/"#in"/"#matches" были помечены, а "and"/"or"/"value"/"else" — нет,
// хотя интерпретирует их тот же движок. Различать их по контексту умел только
// разборщик; человеку, пишущему шаблон, приходилось это помнить.
//
// Легаси-форма читается бессрочно — ни один существующий шаблон не ломается.
func condKey(m map[string]interface{}, word string) (interface{}, bool) {
	if v, ok := m["#"+word]; ok {
		return v, true
	}
	v, ok := m[word]
	return v, ok
}

// isIfKey сообщает, является ли ключ объекта условной конструкцией. JSON-объект
// не может нести два одинаковых ключа (при разборе в map выживает последний, а
// первое условие молча теряется), поэтому к `#if` разрешён произвольный суффикс:
// `#if`, `#if1`, `#if 2`, `#if tun-only` — все они равнозначны и позволяют
// повесить на один объект несколько независимых условий, попутно давая условию
// человекочитаемое имя (SPEC 103, D-045).
func isIfKey(k string) bool {
	return k == "#if" || strings.HasPrefix(k, "#if")
}

// ifKeysSorted возвращает условные ключи объекта в детерминированном порядке:
// несколько `#if…` на одном объекте применяются последовательно, и порядок
// обязан быть воспроизводимым (обход map в Go случаен).
func ifKeysSorted(m map[string]interface{}) []string {
	var keys []string
	for k := range m {
		if isIfKey(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// EvalIfScalar вычисляет одиночную конструкцию `{"#if": {...}}` до строкового
// скаляра ВНЕ дерева конфига (паритет с mobile evalIfScalar, if_engine.dart:
// 224-229; SPEC 103, §4.5/§4.6 TEMPLATE_LANG.md). Используется исключительно
// механизмом on_change.set (on_change.go) — переиспользует тот же движок
// предикатов и тот же канонический обходчик (selectIfBranchCanon,
// substituteWalkCanon), что и главный конфиг, чтобы не заводить второй парсер
// языка #if.
//
// resolved строится из vars+stateVars через ResolveTemplateVarsFor: on_change
// вызывается ПОСЛЕ того, как изменённая var уже записана в stateVars новым
// значением — резолвер обязан увидеть его.
//
// ok=false, если node — не объект ровно с одним ключом "#if", ветка не
// выбрана (false без else) или выбранное значение — не строка (массивы/
// объекты как результат on_change.set не поддерживаются: цель on_change —
// всегда var, а var — скаляр).
func EvalIfScalar(node json.RawMessage, vars []TemplateVar, stateVars map[string]string, target TargetSpec) (string, bool) {
	target = target.Normalized()
	var outer map[string]interface{}
	if err := json.Unmarshal(node, &outer); err != nil {
		debuglog.WarnLog("EvalIfScalar: invalid JSON: %v", err)
		return "", false
	}
	ifKeys := ifKeysSorted(outer)
	if len(outer) != 1 || len(ifKeys) != 1 {
		debuglog.WarnLog("EvalIfScalar: the node must be an object with exactly one key \"#if…\"")
		return "", false
	}
	rawBody, ok := outer[ifKeys[0]]
	if !ok {
		debuglog.WarnLog("EvalIfScalar: the node does not contain \"#if…\"")
		return "", false
	}
	body, ok := rawBody.(map[string]interface{})
	if !ok {
		debuglog.WarnLog("EvalIfScalar: #if body is not an object")
		return "", false
	}

	varTypes := make(map[string]string, len(vars))
	declared := make(map[string]bool, len(vars))
	for _, v := range vars {
		if !v.Separator {
			varTypes[v.Name] = v.Type
			declared[v.Name] = true
		}
	}
	resolved := ResolveTemplateVarsFor(vars, stateVars, nil, target)

	// Канонический обходчик (SPEC 143): та же грамматика #if и та же политика
	// unresolved, что у главного конфига. Предупреждения здесь — только в лог:
	// on_change срабатывает в UI при правке переменной, сборки и её отчёта в
	// этот момент нет.
	ctx := &canonCtx{varTypes: varTypes, declared: declared, resolved: resolved, target: target}
	defer func() {
		SortTemplateWarnings(ctx.warnings)
		for _, w := range ctx.warnings {
			debuglog.WarnLog("EvalIfScalar: %s %v", w.Code, w.Params)
		}
	}()
	branch, take := selectIfBranchCanon(ifKeys[0], body, ctx)
	if !take {
		return "", false
	}
	// Ветка может сама содержать @var-плейсхолдеры (как в конфиг-дереве) —
	// подставляем их тем же ходом, что и обычный #if в дереве конфига.
	substituteWalkCanon(&branch, ctx)
	if _, dropped := branch.(droppedValue); dropped {
		// Dropped (§5.1): значения нет — цель не трогаем, как у невыбранной ветки.
		return "", false
	}
	s, ok := branch.(string)
	if !ok {
		debuglog.WarnLog("EvalIfScalar: the chosen branch is not a string scalar (%T)", branch)
		return "", false
	}
	// Необъявленное имя в ветке канон оставляет плейсхолдером (§5.2) — в
	// состояние переменной такой литерал не пишется, цель не трогаем.
	for _, w := range ctx.warnings {
		if w.Code == warnVarUndeclared {
			return "", false
		}
	}
	return s, true
}

// selectIfBranch evaluates the condition and picks the value/else branch.
// Returns (branch, take=true) when a branch was selected; (nil, false) when
// condition is false and no else is present.
func selectIfBranch(body map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) (interface{}, bool) {
	cond := evaluateIfCondition(body, varTypes, resolved, target)
	if cond {
		val, hasVal := condKey(body, "value")
		if !hasVal {
			debuglog.WarnLog("substitute: #if missing required \"value\" field — skipping")
			return nil, false
		}
		return val, true
	}
	if elseVal, hasElse := condKey(body, "else"); hasElse {
		return elseVal, true
	}
	return nil, false
}

// evaluateIfCondition вычисляет тело #if (ключи and/or) — тонкая обёртка над
// evaluateCond: тело #if есть cond-obj плюс value/else (TEMPLATE_LANG §5.2).
//
// Defensive: оба ключа сразу → false + warn; ни одного → true + warn (тело #if
// без условия исторически трактовалось как безусловное; строгую форму держит
// канонический обходчик и load-валидатор).
func evaluateIfCondition(body map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	_, hasAnd := condKey(body, "and")
	_, hasOr := condKey(body, "or")
	if hasAnd && hasOr {
		debuglog.WarnLog("substitute: #if has both \"and\" and \"or\" — treating as false")
		return false
	}
	if !hasAnd && !hasOr {
		debuglog.WarnLog("substitute: #if has neither \"and\" nor \"or\" — treating as true")
		return true
	}
	return evaluateCondObj(body, varTypes, resolved, target)
}

// evaluateCond вычисляет ЛЮБОЕ условие языка (TEMPLATE_LANG §5.1, SPEC 107):
//
//	cond := pred-list | cond-obj | pred
//
// Сахар: голый список ≡ {"and": [...]}; одиночный предикат ≡ список из одного.
// Единая точка для #if, #enable и гейтов носителей — параллельных грамматик
// в языке нет.
func evaluateCond(cond interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	switch c := cond.(type) {
	case []interface{}:
		// Сахар-список: все истинны.
		return evaluatePredicateList(c, true, varTypes, resolved, target)
	case map[string]interface{}:
		if _, isAnd := condKey(c, "and"); isAnd {
			return evaluateCondObj(c, varTypes, resolved, target)
		}
		if _, isOr := condKey(c, "or"); isOr {
			return evaluateCondObj(c, varTypes, resolved, target)
		}
		// Объект без and/or — предикат ({"@var": …} / {"#not": …}).
		return evaluatePredicate(cond, varTypes, resolved, target)
	}
	// Строка — bare-предикат; всё прочее отвергнет evaluatePredicate.
	return evaluatePredicate(cond, varTypes, resolved, target)
}

// evaluateCondObj вычисляет объект-условие с ключом and или or. Ровно один из
// них; оба или ни одного — false (fail-closed, D-058: опечатка не должна
// молча включить узел).
func evaluateCondObj(c map[string]interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	andRaw, hasAnd := condKey(c, "and")
	orRaw, hasOr := condKey(c, "or")
	if hasAnd == hasOr {
		debuglog.WarnLog("substitute: cond must have exactly one of \"and\"/\"or\" — treating as false")
		return false
	}
	raw, isAnd := andRaw, true
	if hasOr {
		raw, isAnd = orRaw, false
	}
	list, ok := raw.([]interface{})
	if !ok {
		debuglog.WarnLog("substitute: cond and/or is not an array — treating as false")
		return false
	}
	return evaluatePredicateList(list, isAnd, varTypes, resolved, target)
}

// evaluatePredicateList short-circuits AND/OR over predicates.
func evaluatePredicateList(list []interface{}, isAnd bool, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	if isAnd {
		// Empty AND → vacuously true.
		for _, p := range list {
			// SPEC 107: элемент списка — предикат ИЛИ вложенный cond-obj
			// (снят запрет D-018), глубина не ограничена.
			if !evaluateCond(p, varTypes, resolved, target) {
				return false
			}
		}
		return true
	}
	// Empty OR → vacuously false.
	for _, p := range list {
		if evaluateCond(p, varTypes, resolved, target) {
			return true
		}
	}
	return false
}

// evaluatePredicate dispatches the 8 predicate forms (see SPEC 067).
// Recurses for #not.
func evaluatePredicate(p interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	switch pv := p.(type) {
	case string:
		// Предикат строкой с JSON (SPEC 097) — разбираем и вычисляем как узел.
		if node, ok := parseJSONPredicateString(pv); ok {
			return evaluateCond(node, varTypes, resolved, target)
		}
		// Bare "@var" → bool template var → scalar == "true".
		if !strings.HasPrefix(pv, "@") {
			debuglog.WarnLog("substitute: #if predicate bare string %q must start with @ — treating as false", pv)
			return false
		}
		name := strings.TrimPrefix(pv, "@")
		// Runtime globals not allowed in bare form (per SPEC).
		if isRuntimeGlobalRef(name) {
			debuglog.WarnLog("substitute: #if bare predicate %q is not allowed for runtime globals — treating as false", pv)
			return false
		}
		scalar, _, _, found := lookupVarScalar(name, resolved, target)
		if !found {
			debuglog.WarnLog("substitute: #if predicate references unknown var @%s — treating as false", name)
			return false
		}
		return strings.EqualFold(strings.TrimSpace(scalar), "true")
	case map[string]interface{}:
		if len(pv) != 1 {
			debuglog.WarnLog("substitute: #if predicate object must have exactly one key — treating as false")
			return false
		}
		for k, v := range pv {
			if k == "#not" {
				// SPEC 107: отрицание ЛЮБОГО условия, включая and/or.
				return !evaluateCond(v, varTypes, resolved, target)
			}
			if strings.HasPrefix(k, "@") {
				name := strings.TrimPrefix(k, "@")
				return evaluateVarPredicate(name, v, varTypes, resolved, target)
			}
			debuglog.WarnLog("substitute: #if predicate has unknown key %q — treating as false", k)
			return false
		}
	}
	debuglog.WarnLog("substitute: #if predicate has invalid shape — treating as false")
	return false
}

// evaluateVarPredicate dispatches RHS forms for {"@var": ...} predicates.
func evaluateVarPredicate(varName string, rhs interface{}, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	scalar, isList, list, found := lookupVarScalar(varName, resolved, target)
	if !found {
		debuglog.WarnLog("substitute: #if predicate references unknown var @%s — treating as false", varName)
		return false
	}
	switch r := rhs.(type) {
	case string:
		// "#notEmpty" / "#isEmpty" — no-arg predicate.
		if r == "#notEmpty" {
			return checkNotEmpty(scalar, isList, list, varName, varTypes)
		}
		if r == "#isEmpty" {
			return !checkNotEmpty(scalar, isList, list, varName, varTypes)
		}
		if strings.HasPrefix(r, "#") {
			debuglog.WarnLog("substitute: #if predicate has unknown no-arg form %q — treating as false", r)
			return false
		}
		// Literal equality — substitute @var if present.
		lit := substituteSimpleString(r, varTypes, resolved)
		return strings.TrimSpace(scalar) == lit
	case map[string]interface{}:
		if len(r) != 1 {
			debuglog.WarnLog("substitute: #if predicate RHS object must have exactly one key — treating as false")
			return false
		}
		for k, arg := range r {
			switch k {
			case "#in":
				return checkInList(scalar, arg, varTypes, resolved)
			case "#notIn":
				return !checkInList(scalar, arg, varTypes, resolved)
			case "#matches":
				return checkMatches(scalar, arg, varTypes, resolved)
			default:
				debuglog.WarnLog("substitute: #if predicate has unknown arg-form %q — treating as false", k)
				return false
			}
		}
	}
	debuglog.WarnLog("substitute: #if predicate RHS has invalid shape — treating as false")
	return false
}

// lookupVarScalar resolves @runtime.* globals (case-sensitive lower-case) and
// otherwise looks up the name in `resolved`. Returns (scalar, isList, list,
// found). Unknown @runtime.<field> → not found (defensive).
func lookupVarScalar(name string, resolved map[string]ResolvedVar, target TargetSpec) (string, bool, []string, bool) {
	if isRuntimeGlobalRef(name) {
		switch name[len(runtimeGlobalPrefix):] {
		case "platform":
			return target.GOOS, false, nil, true
		case "arch":
			return target.GOARCH, false, nil, true
		case "target":
			return target.TargetOrLocal(), false, nil, true
		default:
			debuglog.WarnLog("substitute: unknown runtime global @%s — treating as not found", name)
			return "", false, nil, false
		}
	}
	r, ok := resolved[name]
	if !ok {
		return "", false, nil, false
	}
	if len(r.List) > 0 || (r.Scalar == "" && r.List != nil) {
		return r.Scalar, true, r.List, true
	}
	return r.Scalar, false, nil, true
}

// checkNotEmpty applies the #notEmpty predicate semantics:
// text → len(trim(scalar)) > 0; text_list → len(list) > 0; bool → scalar == "true".
func checkNotEmpty(scalar string, isList bool, list []string, varName string, varTypes map[string]string) bool {
	if isList {
		return len(list) > 0
	}
	if typ, ok := varTypes[varName]; ok && typ == "bool" {
		return strings.EqualFold(strings.TrimSpace(scalar), "true")
	}
	if typ, ok := varTypes[varName]; ok && typ == "text_list" {
		return len(list) > 0
	}
	return len(strings.TrimSpace(scalar)) > 0
}

// checkInList tests whether scalar is in the args list. argsRaw may be either
// a JSON array of strings or a single "@text_list_var" reference.
func checkInList(scalar string, argsRaw interface{}, varTypes map[string]string, resolved map[string]ResolvedVar) bool {
	trimmed := strings.TrimSpace(scalar)
	// Single "@text_list_var" string — resolve to list.
	if s, ok := argsRaw.(string); ok {
		if strings.HasPrefix(s, "@") {
			name := strings.TrimPrefix(s, "@")
			if typ, exists := varTypes[name]; exists && typ == "text_list" {
				if r, rOK := resolved[name]; rOK {
					for _, item := range r.List {
						if item == trimmed {
							return true
						}
					}
					return false
				}
			}
		}
		debuglog.WarnLog("substitute: #if #in arg as string %q is not a @text_list_var — treating as false", s)
		return false
	}
	// Array form.
	list, ok := argsRaw.([]interface{})
	if !ok {
		debuglog.WarnLog("substitute: #if #in arg is not an array or @text_list_var — treating as false")
		return false
	}
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		lit := substituteSimpleString(s, varTypes, resolved)
		if lit == trimmed {
			return true
		}
	}
	return false
}

// checkMatches compiles the regex pattern (after @var substitution) and tests it
// against trimmed scalar.
func checkMatches(scalar string, patternRaw interface{}, varTypes map[string]string, resolved map[string]ResolvedVar) bool {
	pat, ok := patternRaw.(string)
	if !ok {
		debuglog.WarnLog("substitute: #if #matches pattern is not a string — treating as false")
		return false
	}
	pat = substituteSimpleString(pat, varTypes, resolved)
	re, err := regexp.Compile(pat)
	if err != nil {
		debuglog.WarnLog("substitute: #if #matches invalid regex %q: %v — treating as false", pat, err)
		return false
	}
	return re.MatchString(strings.TrimSpace(scalar))
}

// substituteSimpleString is used for controlled @var substitution inside
// predicate args (literal equality, #matches pattern, individual #in elements).
// If s == "@varname", resolves the value to its string form. Otherwise returns
// s as-is.
//
// Правила приведения — те же, что у подстановки в дерево (bool → "true"/
// "false", int — clamp по §2.2), но результат всегда строка: предикат
// сравнивает строки. Имя без значения сравнивается с "" — предикат по нему
// ложен, а не роняет вычисление; список (text_list) аргументом-скаляром не
// бывает, и ссылка остаётся как есть.
func substituteSimpleString(s string, varTypes map[string]string, resolved map[string]ResolvedVar) string {
	if !strings.HasPrefix(s, "@") {
		return s
	}
	name := strings.TrimPrefix(s, "@")
	if name == "" || strings.Contains(name, "@") {
		return s
	}
	r, ok := resolved[name]
	if !ok {
		return ""
	}
	typ := varTypes[name]
	if typ == "text_list" {
		return s
	}
	v := strings.TrimSpace(r.Scalar)
	if typ == "bool" {
		if v != "" && strings.EqualFold(v, "true") {
			return "true"
		}
		return "false"
	}
	if !wantsIntCast(name, typ) {
		return v
	}
	if v == "" {
		return "0"
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return v
	}
	if n < intCastMin {
		n = intCastMin
	}
	if n > intCastMax {
		n = intCastMax
	}
	return strconv.Itoa(n)
}
