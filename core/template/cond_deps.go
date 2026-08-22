package template

// Статическое извлечение зависимостей условия (SPEC 107 §8.1, D-066).
//
// Язык условий декларативен и полностью анализируем: множество переменных, от
// которых зависит узел, известно в момент загрузки шаблона — без вычисления и
// без динамического трекинга «что прочитали». Это и делает реактивный
// пересчёт дешёвым: индекс `переменная → узлы` строится один раз.
//
// Функция нормативна и проверяется общим корпусом (`corpus/template/deps/`) —
// Dart обязан давать тот же результат.

import (
	"encoding/json"
	"sort"
	"strings"
)

// CondDeps возвращает отсортированное множество имён переменных, от которых
// зависит условие (§5.1: сахар-список / cond-obj / предикат).
//
// Имена — без ведущего "@". Globals попадают под своими полными именами
// ("runtime.platform" и т. д.): на вкладке Target удалённой машины платформа
// меняется селектором, и узлы, гейтящиеся по ней, обязаны пересчитаться —
// это такие же зависимости, а не константы сессии.
//
// ВАЖНО: собираются имена ВСЕГО дерева, включая ветки, до которых вычисление
// не дойдёт из-за short-circuit. Завтра значение изменится, выбор ветки
// станет другим — подписка обязана существовать уже сегодня.
func CondDeps(cond interface{}) []string {
	set := make(map[string]struct{})
	collectCondDeps(cond, set)
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CondDepsJSON — CondDeps для условия в сыром JSON. Пустой вход и невалидный
// JSON дают пустой список: узел без разбираемого условия ни от чего не
// зависит (само условие при этом fail-closed вычислится в false).
func CondDepsJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return CondDeps(v)
}

// collectCondDeps обходит условие любой формы и глубины.
func collectCondDeps(cond interface{}, set map[string]struct{}) {
	switch x := cond.(type) {
	case []interface{}:
		// Сахар-список ≡ and.
		for _, e := range x {
			collectCondDeps(e, set)
		}
	case map[string]interface{}:
		// Ключевые слова читаются в обеих формах: канонической "#and"/"#or"
		// и легаси без "#" (SPEC 107).
		if raw, ok := condKey(x, "and"); ok {
			if list, isList := raw.([]interface{}); isList {
				for _, e := range list {
					collectCondDeps(e, set)
				}
			}
			return
		}
		if raw, ok := condKey(x, "or"); ok {
			if list, isList := raw.([]interface{}); isList {
				for _, e := range list {
					collectCondDeps(e, set)
				}
			}
			return
		}
		collectPredicateDeps(x, set)
	case string:
		// Предикат, записанный СТРОКОЙ с JSON (SPEC 097-форма
		// `"{\"@runtime.target\":\"local\"}"`). Встречается и внутри
		// #enable, не только в легаси if[] — разбираем здесь, чтобы форма
		// работала одинаково везде.
		if node, ok := parseJSONPredicateString(x); ok {
			collectCondDeps(node, set)
			return
		}
		noteVarName(x, set)
	}
}

// parseJSONPredicateString разбирает предикат, записанный строкой с JSON.
// Не-JSON строки (обычные "@var") возвращают ok=false.
func parseJSONPredicateString(s string) (interface{}, bool) {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "{") {
		return nil, false
	}
	var node interface{}
	if err := json.Unmarshal([]byte(t), &node); err != nil {
		return nil, false
	}
	return node, true
}

// collectPredicateDeps разбирает предикат: левую часть и все ссылки правой.
func collectPredicateDeps(p map[string]interface{}, set map[string]struct{}) {
	for k, v := range p {
		if k == "#not" {
			// Отрицание любого условия, не только предиката.
			collectCondDeps(v, set)
			continue
		}
		noteVarName(k, set) // левая часть {"@var": …}
		collectRHSDeps(v, set)
	}
}

// collectRHSDeps собирает ссылки правой части предиката (разрыв C8): равенство
// с другой переменной, элементы #in/#notIn, строковая форма #in ("@list"),
// паттерн #matches.
func collectRHSDeps(rhs interface{}, set map[string]struct{}) {
	switch r := rhs.(type) {
	case string:
		// "#notEmpty"/"#isEmpty" — не ссылки; всё прочее может быть "@var".
		if !strings.HasPrefix(r, "#") {
			noteVarName(r, set)
		}
	case []interface{}:
		for _, e := range r {
			collectRHSDeps(e, set)
		}
	case map[string]interface{}:
		for _, arg := range r { // #in / #notIn / #matches
			collectRHSDeps(arg, set)
		}
	}
}

// noteVarName кладёт имя в множество, если строка — ссылка вида "@name".
func noteVarName(ref string, set map[string]struct{}) {
	if !strings.HasPrefix(ref, "@") {
		return
	}
	name := ref[1:]
	if name == "" || strings.Contains(name, "@") {
		return
	}
	set[name] = struct{}{}
}
