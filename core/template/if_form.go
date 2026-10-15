package template

// Форма тела `#if…` (TEMPLATE_LANG §4.1, D-133, контракт 1.1.125).
//
// Единственное место движка, которое знает, как выглядит тело `#if`: обе
// формы разбираются здесь по типу значения, а все точки применения —
// map-spread, условный элемент массива, EvalIfScalar, default_value,
// валидатор загрузки, операция `#do` — работают с уже разобранным ifForm.
// Так короткая и длинная формы не могут разойтись в семантике.
//
//	массив  [cond, V] / [cond, V, E]                — короткая форма (канон)
//	объект  {"#and"|"#or": [...], "#value": V, "#else": E} — длинная форма (бессрочно)
//	иное                                            — ошибка формы

import "fmt"

// ifForm — разобранное тело `#if…`.
type ifForm struct {
	// Cond — условие языка §4.1–4.2 в форме, которую принимает evaluateCond.
	// У короткой формы — первый элемент как написан; у длинной — объект
	// `{"#and": [...]}` / `{"#or": [...]}`, собранный из тела.
	Cond     interface{}
	Value    interface{}
	HasValue bool
	Else     interface{}
	HasElse  bool
	// Short — тело было массивом.
	Short bool
}

// parseIfForm разбирает тело `#if…` по типу значения. Второе значение —
// текст ошибки формы ("" — форма валидна); валидатор загрузки ставит перед
// ним контекст, толерантный рантайм на любую ошибку отвечает
// template_unknown_directive и не выбирает ветку.
//
// Длинная форма проверяется структурно (ровно один из and/or, есть value):
// содержимое списка предикатов — дело валидатора (validateCondObjLists) и
// рантайма (evaluateCond), как и до короткой формы.
func parseIfForm(raw interface{}) (ifForm, string) {
	switch body := raw.(type) {
	case []interface{}:
		return parseIfShort(body)
	case map[string]interface{}:
		return parseIfLong(body)
	}
	return ifForm{}, "body must be an array [cond, value, else?] or an object with \"#value\""
}

// parseIfShort — короткая форма: позиционное чтение, 2–3 элемента, cond не
// массив, ветки не null.
func parseIfShort(body []interface{}) (ifForm, string) {
	if len(body) < 2 || len(body) > 3 {
		return ifForm{}, fmt.Sprintf("array form needs 2 or 3 elements [cond, value, else?], got %d", len(body))
	}
	if _, isList := body[0].([]interface{}); isList {
		return ifForm{}, "condition (element 0) must not be an array — several predicates go into {\"#and\": [...]} or {\"#or\": [...]}"
	}
	if body[0] == nil {
		return ifForm{}, "condition (element 0) must not be null"
	}
	if body[1] == nil {
		return ifForm{}, "value (element 1) must not be null"
	}
	f := ifForm{Cond: body[0], Value: body[1], HasValue: true, Short: true}
	if len(body) == 3 {
		if body[2] == nil {
			return ifForm{}, "else (element 2) must not be null"
		}
		f.Else, f.HasElse = body[2], true
	}
	return f, ""
}

// parseIfLong — длинная форма: ровно один из and/or (канон `#and`/`#or`,
// легаси без `#`), обязательный value, опциональный else.
func parseIfLong(body map[string]interface{}) (ifForm, string) {
	andRaw, hasAnd := condKey(body, "and")
	orRaw, hasOr := condKey(body, "or")
	if hasAnd && hasOr {
		return ifForm{}, "must have exactly one of \"and\" or \"or\" (not both)"
	}
	if !hasAnd && !hasOr {
		return ifForm{}, "must have one of \"and\" or \"or\""
	}
	val, hasVal := condKey(body, "value")
	if !hasVal {
		return ifForm{}, "missing required \"value\" field"
	}
	f := ifForm{Value: val, HasValue: true}
	if hasAnd {
		f.Cond = map[string]interface{}{"#and": andRaw}
	} else {
		f.Cond = map[string]interface{}{"#or": orRaw}
	}
	if elseVal, hasElse := condKey(body, "else"); hasElse {
		f.Else, f.HasElse = elseVal, true
	}
	return f, ""
}

// evalIfForm выбирает ветку: условие истинно → Value; ложно → Else, если
// есть. take=false — ветка не выбрана. Общий для всех точек применения.
func evalIfForm(f ifForm, varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) (interface{}, bool) {
	if evaluateCond(f.Cond, varTypes, resolved, target) {
		if !f.HasValue {
			return nil, false
		}
		return f.Value, true
	}
	if f.HasElse {
		return f.Else, true
	}
	return nil, false
}
