package nodeflow

// Авторское тело: реестр сообщает, не правит (контракт 1.1.87,
// PARSING_PRINCIPLES §10; решение владельца 27.09.2026, LxBox §577).
//
// Здесь ЕДИНСТВЕННАЯ точка, которая решает, применять ли к телу узла правку
// по правилу реестра: Decide. Её зовут все шаги, меняющие тело по реестру —
// санитайзер при материализации (AuthoredResult), починки на сборке
// (RepairsFor) и уступка detour (config.yieldBodyToDetour,
// config.yieldToBuildDetour). Обычное тело правится всегда; авторское —
// только жёстким правилом (`core_rejects` реестра), остальные правила дают
// код с признаком applied: false.

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/registry"
)

// RuleIsHard — жёсткое ли правило, давшее запись w, у схемы scheme.
//
// Жёсткие (PARSING_PRINCIPLES §10.3): узел без строкового непустого `type`
// (field_missing на пути `type`), узловой гейт ядра, правило или связь с
// `core_rejects`.
func RuleIsHard(scheme string, w Warning) bool {
	if w.Code == "field_missing" && w.Path == "type" {
		return true
	}
	reg, err := registry.Get()
	if err != nil {
		return true // без реестра судить нечем — поведение прежнее
	}
	return reg.RuleCoreRejects(scheme, w.Code, w.Path)
}

// Decide — применять ли правку, о которой говорит запись w, к телу узла.
//
// Обычное тело (authored == false) правится всегда, запись возвращается как
// есть. Авторское — только жёстким правилом; мягкое правило тело не трогает,
// а запись получает applied: false.
func Decide(scheme string, authored bool, w Warning) (bool, Warning) {
	if !authored || RuleIsHard(scheme, w) {
		return true, w
	}
	return false, w.NotApplied()
}

// AuthoredResult — исход санитайзера для АВТОРСКОГО тела raw.
//
// Тело — raw как написано, без `tag` и `detour` (их назначает сборка) и
// с правками только жёстких правил: значение по пути записи берётся из
// res.Clean (или снимается, если там его нет). Коды — те же и в том же
// порядке, мягкие с applied: false. Отказ по узлу остаётся только
// жёстким; мягкий становится записью с applied: false.
func AuthoredResult(scheme string, raw map[string]interface{}, res Result) Result {
	body := map[string]interface{}{}
	if raw != nil {
		body = deepCopyMap(raw)
	}
	delete(body, "tag")
	delete(body, "detour")
	out := Result{Clean: body}
	for _, w := range res.Warnings {
		apply, w2 := Decide(scheme, true, w)
		if apply && w.Path != "" {
			patchFromClean(body, res.Clean, w.Path)
		}
		out.Warnings = append(out.Warnings, w2)
	}
	for _, p := range res.HardPaths {
		patchFromClean(body, res.Clean, p)
	}
	if res.Drop != nil {
		if apply, _ := Decide(scheme, true, *res.Drop); apply {
			d := *res.Drop
			out.Drop = &d
		} else {
			marked := false
			for i := range out.Warnings {
				if out.Warnings[i].Code == res.Drop.Code && out.Warnings[i].Path == res.Drop.Path {
					marked = true
				}
			}
			if !marked {
				out.Warnings = append(out.Warnings, res.Drop.NotApplied())
			}
		}
	}
	return out
}

// RepairsFor — починки реестра (Repairs) через точку решения: у авторского
// тела применяются только жёсткие, мягкие возвращаются с applied: false и
// тело не трогают.
func RepairsFor(scheme string, authored bool, m map[string]interface{}) (map[string]interface{}, []Warning) {
	fixed, notes := Repairs(scheme, m)
	if !authored || len(notes) == 0 {
		return fixed, notes
	}
	body := deepCopyMap(m)
	out := make([]Warning, 0, len(notes))
	for _, w := range notes {
		apply, w2 := Decide(scheme, true, w)
		if apply && w.Path != "" {
			patchFromClean(body, fixed, w.Path)
		}
		out = append(out, w2)
	}
	return body, out
}

// patchFromClean переносит в body значение пути path из clean; нет
// значения в clean — путь в body снимается.
//
// Индекс элемента в скобках (`peers[0].port`) — отдельный сегмент; правка
// самого элемента (`server_ports[0]`: элемент снят) переносится массивом
// целиком — после снятия индексы остальных элементов сдвинуты.
func patchFromClean(body, clean map[string]interface{}, path string) {
	parts := strings.Split(strings.NewReplacer("[", ".", "]", "").Replace(path), ".")
	for len(parts) > 1 && isIndexPart(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	if v, ok := lookupBodyPath(clean, parts); ok {
		setBodyPath(body, parts, deepCopyValue(v))
		return
	}
	deleteBodyPath(body, parts)
}

func lookupBodyPath(v interface{}, parts []string) (interface{}, bool) {
	cur := v
	for _, p := range parts {
		switch c := cur.(type) {
		case map[string]interface{}:
			next, ok := c[p]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func setBodyPath(m map[string]interface{}, parts []string, v interface{}) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		m[parts[0]] = v
		return
	}
	switch next := m[parts[0]].(type) {
	case map[string]interface{}:
		setBodyPath(next, parts[1:], v)
	case []interface{}:
		i, err := strconv.Atoi(parts[1])
		if err != nil || i < 0 || i >= len(next) {
			return
		}
		if len(parts) == 2 {
			next[i] = v
			return
		}
		if inner, ok := next[i].(map[string]interface{}); ok {
			setBodyPath(inner, parts[2:], v)
		}
	default:
		inner := map[string]interface{}{}
		m[parts[0]] = inner
		setBodyPath(inner, parts[1:], v)
	}
}

func deleteBodyPath(m map[string]interface{}, parts []string) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		delete(m, parts[0])
		return
	}
	switch next := m[parts[0]].(type) {
	case map[string]interface{}:
		deleteBodyPath(next, parts[1:])
	case []interface{}:
		i, err := strconv.Atoi(parts[1])
		if err != nil || i < 0 || i >= len(next) {
			return
		}
		if len(parts) == 2 {
			m[parts[0]] = append(append([]interface{}{}, next[:i]...), next[i+1:]...)
			return
		}
		if inner, ok := next[i].(map[string]interface{}); ok {
			deleteBodyPath(inner, parts[2:])
		}
	}
}

func isIndexPart(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
