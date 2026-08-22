package template

import (
	"encoding/json"
	"sort"
	"strings"
)

// EvalIf reports whether an if/if_or guard is satisfied for the given vars:
// every name in ifList must resolve to "true" (case-insensitive), AND ifOrList
// is empty OR at least one of its names is "true". A leading "@" on a name is
// stripped before lookup (SPEC 067 canonical form).
//
// This is the single source of truth for preset / rule / outbound if/if_or
// activation, shared by the build pipeline (core/build) and the configurator UI
// (ui/configurator) so server-side emit and the UI preview never diverge.
func EvalIf(ifList, ifOrList []string, varsMap map[string]string) bool {
	ok, _ := EvalIfWithReason(ifList, ifOrList, varsMap)
	return ok
}

// EvalIfWithReason is EvalIf plus a human-readable reason when the guard fails
// ("if=<name>" or "if_or=<sorted,names>"); the reason is "" when it passes.
// The if_or names in the reason are sorted (a COPY — the caller's slice is never
// mutated) for deterministic, test-stable output.
func EvalIfWithReason(ifList, ifOrList []string, varsMap map[string]string) (bool, string) {
	for _, name := range ifList {
		if !strings.EqualFold(strings.TrimSpace(varsMap[strings.TrimPrefix(name, "@")]), "true") {
			return false, "if=" + name
		}
	}
	if len(ifOrList) > 0 {
		anyTrue := false
		for _, name := range ifOrList {
			if strings.EqualFold(strings.TrimSpace(varsMap[strings.TrimPrefix(name, "@")]), "true") {
				anyTrue = true
				break
			}
		}
		if !anyTrue {
			sorted := append([]string(nil), ifOrList...)
			sort.Strings(sorted)
			return false, "if_or=" + strings.Join(sorted, ",")
		}
	}
	return true, ""
}

// ---------------------------------------------------------------------------
// Гейт носителя: #enable + легаси-алиасы (SPEC 107 §7, D-065/D-066)
// ---------------------------------------------------------------------------

// GateCond — нормализованное условие носителя (переменной, фрагмента пресета,
// params). Собирается ОДИН РАЗ при декодировании: ни одна точка применения не
// знает о старых формах, канон и легаси сходятся здесь.
//
// nil означает «гейта нет» — носитель безусловен.
type GateCond struct {
	// Cond — условие языка §5.1 в разобранном виде (то же, что тело #enable).
	Cond interface{}
	// Invalid — форма не разобрана. Fail-closed: гейт не пройден (D-058).
	Invalid bool
}

// NormalizeGate сводит #enable и легаси-формы в одно условие.
//
//	enableRaw — значение ключа "#enable" (nil, если ключа не было);
//	ifList    — легаси "if"  → and-список;
//	ifOrList  — легаси "if_or" → or-список.
//
// Элемент легаси-списка может быть не только именем "@var", но и СТРОКОЙ С
// JSON-ПРЕДИКАТОМ (`"{\"@runtime.target\":\"local\"}"`, SPEC 097) — такие
// строки разбираются в предикат. Без этого гейты Settings по таргету сломались
// бы молча (ловушка SPEC 107 §11.3).
//
// Несколько источников на одном носителе объединяются через and.
func NormalizeGate(enableRaw interface{}, ifList, ifOrList []string) *GateCond {
	var parts []interface{}

	if len(ifList) > 0 {
		parts = append(parts, map[string]interface{}{"and": legacyListToPredicates(ifList)})
	}
	if len(ifOrList) > 0 {
		parts = append(parts, map[string]interface{}{"or": legacyListToPredicates(ifOrList)})
	}
	if enableRaw != nil {
		parts = append(parts, enableRaw)
	}

	switch len(parts) {
	case 0:
		return nil
	case 1:
		return &GateCond{Cond: parts[0]}
	default:
		return &GateCond{Cond: map[string]interface{}{"and": parts}}
	}
}

// NormalizeGateJSON — NormalizeGate для носителя в сыром JSON: сам достаёт
// "#enable". Ошибка разбора JSON → Invalid (fail-closed).
func NormalizeGateJSON(data []byte, ifList, ifOrList []string) *GateCond {
	var probe struct {
		Enable interface{} `json:"#enable"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return &GateCond{Invalid: true}
	}
	return NormalizeGate(probe.Enable, ifList, ifOrList)
}

// legacyListToPredicates превращает элементы легаси-списка в предикаты.
func legacyListToPredicates(list []string) []interface{} {
	out := make([]interface{}, 0, len(list))
	for _, raw := range list {
		trimmed := strings.TrimSpace(raw)
		// SPEC 097: элемент может быть JSON-предикатом, записанным строкой.
		if strings.HasPrefix(trimmed, "{") {
			var node interface{}
			if err := json.Unmarshal([]byte(trimmed), &node); err == nil {
				out = append(out, node)
				continue
			}
			// Не разобралось — оставляем как есть: предикат отвергнет форму,
			// гейт даст false (fail-closed), а не молча пройдёт.
		}
		// Легаси-совместимость: старый EvalIf принимал имя БЕЗ "@"
		// (`"if": ["gateway_mode"]`), потому что делал TrimPrefix. Движок
		// предикатов требует канонический "@". Дописываем префикс сами —
		// иначе шаблоны, писавшиеся до канона, молча получили бы false на
		// каждом гейте (и все их фрагменты исчезли бы из конфига).
		if trimmed != "" && !strings.HasPrefix(trimmed, "@") && !strings.HasPrefix(trimmed, "{") {
			out = append(out, "@"+trimmed)
			continue
		}
		out = append(out, raw)
	}
	return out
}

// Satisfied вычисляет гейт по разрешённым переменным. nil-гейт → true.
func (g *GateCond) Satisfied(varTypes map[string]string, resolved map[string]ResolvedVar, target TargetSpec) bool {
	if g == nil {
		return true
	}
	if g.Invalid {
		return false
	}
	return evaluateCond(g.Cond, varTypes, resolved, target)
}

// SatisfiedVars — вычисление по плоской карте «имя → строковое значение»
// (build-путь пресетов: там значения уже плоские, типов нет).
func (g *GateCond) SatisfiedVars(varsMap map[string]string, target TargetSpec) bool {
	if g == nil {
		return true
	}
	if g.Invalid {
		return false
	}
	resolved := make(map[string]ResolvedVar, len(varsMap))
	for k, v := range varsMap {
		resolved[k] = ResolvedVar{Scalar: v}
	}
	return evaluateCond(g.Cond, nil, resolved, target)
}

// Deps — имена переменных, от которых зависит гейт (SPEC 107 §8.1): вход в
// реактивный индекс.
func (g *GateCond) Deps() []string {
	if g == nil || g.Invalid {
		return nil
	}
	return CondDeps(g.Cond)
}

// GateKey — имя ключа гейта в JSON.
const GateKey = enableKey
