package template

// Канонический обход подстановки (TEMPLATE_LANG §5, D-011, разрыв C4).
//
// Отличие от legacy-обхода (substitute.go): здесь unresolved @var не роняет
// сборку и не маскируется пустой строкой, а даёт одно из двух — плейсхолдер
// (имя не объявлено = опечатка автора) или Dropped-каскад (имя объявлено,
// значения нет = optional-var). Реализация — модель Dart `if_engine.dart`,
// принятая каноном целиком.
//
// Legacy-обход остаётся до миграции вызывающих: preset-путь сегодня опирается
// на strict-семантику «unresolved → выбросить пресет целиком» (SPEC 067 фаза 8),
// и её замена — часть SPEC 106.

import (
	"sort"
	"strconv"
	"strings"
)

// droppedValue — sentinel «этого значения быть не должно» (§5.1).
// Родительский объект удаляет ключ, родительский массив — элемент.
type droppedValue struct{}

// canonCtx — контекст канонического обхода: объявления, значения, накопитель
// warning'ов. Коды warning'ов — из contract/registry/warnings.json.
type canonCtx struct {
	varTypes map[string]string
	declared map[string]bool
	resolved map[string]ResolvedVar
	target   TargetSpec
	warnings []string
}

// warn добавляет код warning'а без дублей — список кодов, а не журнал событий.
func (c *canonCtx) warn(code string) {
	for _, w := range c.warnings {
		if w == code {
			return
		}
	}
	c.warnings = append(c.warnings, code)
}

const (
	warnVarUndeclared    = "template_var_undeclared"
	warnUnknownDirective = "template_unknown_directive"
	warnIntClamped       = "template_int_clamped"
	warnIntInvalid       = "template_int_invalid"
)

// substituteWalkCanon обходит дерево на месте. После обхода значение может
// стать droppedValue — вызывающий обязан это обработать (удалить ключ/элемент).
func substituteWalkCanon(v *interface{}, ctx *canonCtx) {
	switch x := (*v).(type) {
	case map[string]interface{}:
		// SPEC 107: #enable — ПЕРВЫМ, до #if и до обхода детей. При false узел
		// исчезает целиком и внутри ничего не вычисляется (ни подстановок, ни
		// warning'ов). Обязательно ДО ветки warnUnknownDirective ниже: иначе
		// ключ выбрасывался бы как неизвестная директива, а узел оставался в
		// конфиге всегда — молчаливая противоположность задуманному.
		if raw, has := x[enableKey]; has {
			delete(x, enableKey)
			// Ссылка на необъявленное имя внутри гейта — опечатка автора:
			// предикат даст false (узел выпадет), но факт обязан быть виден
			// как warning (§5.2, разрыв N9).
			ctx.noteCondVars(raw)
			// Невалидная грамматика условия (оба and/or сразу, не-список,
			// неизвестная форма) — тоже warning: узел молча исчезает из
			// конфига, и без сигнала причину не найти.
			if !condFormValid(raw) {
				ctx.warn(warnUnknownDirective)
			}
			if !evaluateCond(raw, ctx.varTypes, ctx.resolved, ctx.target) {
				*v = droppedValue{}
				return
			}
		}
		// Pre-pass: управляющие конструкции (ключи на "#"), в порядке
		// сортировки — несколько именованных #if… применяются детерминированно
		// (D-045), одинаково с Dart (sort.Strings ↔ List.sort()).
		var ctrlKeys []string
		for k := range x {
			if strings.HasPrefix(k, "#") {
				ctrlKeys = append(ctrlKeys, k)
			}
		}
		sort.Strings(ctrlKeys)
		for _, k := range ctrlKeys {
			raw := x[k]
			if isIfKey(k) {
				handleIfMapSpreadCanon(x, k, raw, ctx)
				continue
			}
			// Неизвестная директива выбрасывается с warning (forward-compat,
			// §4.1) — чтобы шаблон новой версии не ронял старый движок.
			ctx.warn(warnUnknownDirective)
			delete(x, k)
		}
		// Обычный обход полей; Dropped удаляет ключ (§5.1).
		for k, val := range x {
			substituteWalkCanon(&val, ctx)
			if _, dropped := val.(droppedValue); dropped {
				delete(x, k)
				continue
			}
			x[k] = val
		}

	case []interface{}:
		out := make([]interface{}, 0, len(x))
		for _, elem := range x {
			// Условный элемент массива — объект РОВНО с одним ключом #if…
			// (§4.4). Объект с #if среди других ключей — обычный map-spread.
			if m, ok := elem.(map[string]interface{}); ok && len(m) == 1 {
				if ks := ifKeysSorted(m); len(ks) == 1 {
					if body, ok := m[ks[0]].(map[string]interface{}); ok {
						branch, take := handleIfArrayElementCanon(body, ctx)
						if !take {
							continue // условие false без else — элемент выпадает
						}
						if _, dropped := branch.(droppedValue); dropped {
							continue
						}
						// text_list в позиции элемента сплайсится в массив,
						// а не вкладывается: ["@a", {#if…}] → [a1, a2, …].
						if list, ok := branch.([]interface{}); ok && isSpliceable(m, ks[0], ctx) {
							out = append(out, list...)
							continue
						}
						out = append(out, branch)
						continue
					}
				}
			}
			// Голая ссылка "@text_list_var" в позиции элемента массива
			// СПЛАЙСИТСЯ в родительский массив, а не вкладывается в него:
			// шаблон пишет "address": ["@tun_address", {"#if": …}], и ядро
			// ждёт там плоский список CIDR. Без этого получается [[...]] —
			// ядро отвергает конфиг (`cannot unmarshal array into netip.Prefix`).
			if ref, ok := elem.(string); ok && strings.HasPrefix(ref, "@") {
				name := ref[1:]
				if name != "" && !strings.Contains(name, "@") && ctx.varTypes[name] == "text_list" {
					if list, ok := replacementCanon(name, ctx).([]interface{}); ok {
						out = append(out, list...)
						continue
					}
				}
			}
			val := elem
			substituteWalkCanon(&val, ctx)
			if _, dropped := val.(droppedValue); dropped {
				continue // Dropped-элемент укорачивает массив (§5.1)
			}
			out = append(out, val)
		}
		*v = out

	case string:
		if strings.HasPrefix(x, "@") {
			name := x[1:]
			if name != "" && !strings.Contains(name, "@") {
				*v = replacementCanon(name, ctx)
			}
		}
	}
}

// isSpliceable сообщает, была ли ветка условного элемента ссылкой на text_list —
// такой результат вливается в родительский массив, а не вкладывается в него.
func isSpliceable(m map[string]interface{}, ifKey string, ctx *canonCtx) bool {
	body, ok := m[ifKey].(map[string]interface{})
	if !ok {
		return false
	}
	for _, branchKey := range []string{"value", "else"} {
		if s, ok := body[branchKey].(string); ok && strings.HasPrefix(s, "@") {
			if ctx.varTypes[s[1:]] == "text_list" {
				return true
			}
		}
	}
	return false
}

// replacementCanon возвращает значение для плейсхолдера "@name" по канону §5.2.
func replacementCanon(name string, ctx *canonCtx) interface{} {
	// @runtime.* — не переменные шаблона, а globals таргета (desktop-расширение
	// §7.2); резолв тот же, что в предикатах.
	if isRuntimeGlobalRef(name) {
		if scalar, _, _, found := lookupVarScalar(name, ctx.resolved, ctx.target); found {
			return scalar
		}
		ctx.warn(warnVarUndeclared)
		return "@" + name
	}

	r, ok := ctx.resolved[name]
	if !ok {
		if !ctx.declared[name] {
			// Имя не объявлено — опечатка автора шаблона. Плейсхолдер остаётся
			// видимым: пустая строка спрятала бы ошибку, а падение сборки
			// превратило бы опечатку в отказ всего конфига (§5.2).
			ctx.warn(warnVarUndeclared)
			return "@" + name
		}
		// Объявлена, но значения нет — штатная optional-var, Dropped-каскад.
		return droppedValue{}
	}

	typ := ctx.varTypes[name]
	if typ == "text_list" {
		out := make([]interface{}, len(r.List))
		for i, s := range r.List {
			out[i] = s
		}
		return out
	}

	s := strings.TrimSpace(r.Scalar)
	if typ == "bool" {
		return s != "" && strings.EqualFold(s, "true")
	}
	if wantsIntCast(name, typ) {
		return intCastCanon(name, s, ctx)
	}
	return s
}

// intCastCanon — приведение к числу по §2.2: clamp в [0,65535], а не-число
// уезжает строкой (опечатка видна, а не маскируется нулём).
func intCastCanon(name, s string, ctx *canonCtx) interface{} {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		ctx.warn(warnIntInvalid)
		return s
	}
	if n < intCastMin {
		ctx.warn(warnIntClamped)
		return intCastMin
	}
	if n > intCastMax {
		ctx.warn(warnIntClamped)
		return intCastMax
	}
	return n
}

// handleIfMapSpreadCanon вычисляет #if среди полей объекта и вливает выбранную
// ветку в родителя (§4.3). Ключ #if удаляется всегда.
func handleIfMapSpreadCanon(parent map[string]interface{}, key string, rawBody interface{}, ctx *canonCtx) {
	defer delete(parent, key)
	body, ok := rawBody.(map[string]interface{})
	if !ok {
		ctx.warn(warnUnknownDirective)
		return
	}
	branch, take := selectIfBranchCanon(body, ctx)
	if !take {
		return // false без else — родитель остаётся как есть
	}
	obj, ok := branch.(map[string]interface{})
	if !ok {
		// Ветка не объект: влить в родителя нечего. Это и есть запрет
		// скалярного #if (D-044) — значение поля ведётся через on_change.
		ctx.warn(warnUnknownDirective)
		return
	}
	// Ветка обходится ЦЕЛИКОМ как объект — иначе вложенный в неё #if остаётся
	// нераскрытым (§4.1 разрешает вложение любой глубины), и поле, которое он
	// должен был дать, молча пропадает.
	var branchNode interface{} = obj
	substituteWalkCanon(&branchNode, ctx)
	expanded, ok := branchNode.(map[string]interface{})
	if !ok {
		return
	}
	// Ветка перетирает сиблингов при конфликте ключей (last-wins, §4.3).
	for k, val := range expanded {
		if _, dropped := val.(droppedValue); dropped {
			delete(parent, k)
			continue
		}
		parent[k] = val
	}
}

// handleIfArrayElementCanon вычисляет #if в позиции элемента массива (§4.4).
// take=false означает «элемент выпадает».
func handleIfArrayElementCanon(body map[string]interface{}, ctx *canonCtx) (interface{}, bool) {
	branch, take := selectIfBranchCanon(body, ctx)
	if !take {
		return nil, false
	}
	substituteWalkCanon(&branch, ctx)
	return branch, true
}

// selectIfBranchCanon вычисляет предикаты и возвращает выбранную ветку.
// take=false — условие ложно и ветки else нет.
//
// Движок предикатов (P1–P6, §4.2) переиспользуется из legacy-обхода целиком:
// он уже соответствует канону — все кейсы corpus/template/predicates проходят.
// Канон меняет только политику unresolved и Dropped-каскад, но не грамматику
// условий; своя копия предикатов дала бы два движка, расходящихся со временем.
func selectIfBranchCanon(body map[string]interface{}, ctx *canonCtx) (interface{}, bool) {
	// Ссылка на НЕобъявленное имя внутри предиката = опечатка автора: предикат
	// вычисляется в false (это делает сам движок), но факт обязан быть виден
	// как warning (§4.2, §5.2, разрыв N9). Имена собираются до вычисления —
	// протягивать накопитель через весь legacy-движок предикатов ради этого
	// не нужно.
	ctx.notePredicateVars(body)

	// Грамматика §4.1: ровно один из and/or. Legacy-движок на «ни одного»
	// отвечает TRUE — то есть невалидное условие ВКЛЮЧАЕТ ветку, вместо того
	// чтобы её пропустить. Канон здесь строгий: невалидная форма → false
	// (разрыв C3), чтобы опечатка в условии не втащила в конфиг блок, которого
	// автор не просил.
	_, hasAnd := condKey(body, "and")
	_, hasOr := condKey(body, "or")
	if hasAnd == hasOr { // оба или ни одного
		ctx.warn(warnUnknownDirective)
		if elseVal, has := condKey(body, "else"); has {
			return elseVal, true
		}
		return nil, false
	}
	if _, has := condKey(body, "value"); !has {
		// value обязателен (§4.1). Толерантный рантайм: пропуск + warning.
		ctx.warn(warnUnknownDirective)
		return nil, false
	}

	return selectIfBranch(body, ctx.varTypes, ctx.resolved, ctx.target)
}

// notePredicateVars обходит дерево предикатов #if и помечает warning'ом каждую
// ссылку на имя, которое не объявлено в vars и не является @runtime.*-global.
func (c *canonCtx) notePredicateVars(body map[string]interface{}) {
	for _, key := range []string{"and", "or"} {
		raw, _ := condKey(body, key)
		if list, ok := raw.([]interface{}); ok {
			for _, pred := range list {
				c.notePredicate(pred)
			}
		}
	}
}

// noteCondVars обходит ЛЮБОЕ условие (§5.1): сахар-список, cond-obj с and/or
// на любой глубине, одиночный предикат. Используется гейтом #enable, где
// условие приходит без обёртки-тела #if.
func (c *canonCtx) noteCondVars(cond interface{}) {
	switch x := cond.(type) {
	case []interface{}:
		for _, e := range x {
			c.notePredicate(e)
		}
	case map[string]interface{}:
		if _, isAnd := condKey(x, "and"); isAnd {
			c.notePredicateVars(x)
			return
		}
		if _, isOr := condKey(x, "or"); isOr {
			c.notePredicateVars(x)
			return
		}
		c.notePredicate(cond)
	default:
		c.notePredicate(cond)
	}
}

func (c *canonCtx) notePredicate(pred interface{}) {
	// Вложенный cond-obj как элемент списка (SPEC 107) — рекурсия, иначе
	// ссылки внутри вложенного and/or не попали бы в warning'и и в deps.
	if m, ok := pred.(map[string]interface{}); ok {
		_, hasAnd := condKey(m, "and")
		_, hasOr := condKey(m, "or")
		if hasAnd || hasOr {
			c.notePredicateVars(m)
			return
		}
	}
	switch p := pred.(type) {
	case string: // bare-форма "@var" (P1)
		c.noteVarRef(p)
	case map[string]interface{}:
		for k, v := range p {
			if k == "#not" { // P6 — отрицание вложенного предиката
				c.notePredicate(v)
				continue
			}
			c.noteVarRef(k) // левая часть {"@var": …} (P2–P5)
		}
	}
}

// noteVarRef помечает одну ссылку вида "@name", если такое имя не объявлено.
func (c *canonCtx) noteVarRef(ref string) {
	if !strings.HasPrefix(ref, "@") {
		return
	}
	name := ref[1:]
	if name == "" || isRuntimeGlobalRef(name) {
		return
	}
	if !c.declared[name] {
		c.warn(warnVarUndeclared)
	}
}

// condFormValid — поверхностная проверка формы условия (§5.1) для диагностики.
// Вычисление всё равно fail-closed; здесь решается только, поднимать ли
// warning о невалидной грамматике.
func condFormValid(cond interface{}) bool {
	switch c := cond.(type) {
	case string, []interface{}:
		return true
	case map[string]interface{}:
		_, hasAnd := condKey(c, "and")
		_, hasOr := condKey(c, "or")
		if hasAnd && hasOr {
			return false // ровно один из and/or
		}
		if hasAnd || hasOr {
			return true
		}
		return len(c) == 1 // предикат — ровно один ключ
	}
	return false // число, bool, null — не условие
}
