package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// VarDefaultValue — default_value в vars: строка или объект платформа → значение.
// В JSON: скаляр (строка/число/bool) или объект {"win7":"gvisor","default":"system"}.
// Ключи без учёта регистра. Объект: win7 (только windows/386), затем GOOS (как в platforms), затем default.
// Документация под linux/darwin/windows; см. docs/CREATE_WIZARD_TEMPLATE.md (и _RU).
type VarDefaultValue struct {
	Scalar string
	// PerPlatform: нормализованные ключи (нижний регистр) → значение. Значение —
	// строка ИЛИ #if-дерево (map[string]interface{}), вычисляемое в ForTarget по
	// @runtime.* globals (SPEC 067). Единственный спец-ключ "#if" = top-level выражение.
	PerPlatform map[string]interface{}
}

// IsEmpty true, если нет ни скаляра, ни карты.
func (v VarDefaultValue) IsEmpty() bool {
	return strings.TrimSpace(v.Scalar) == "" && len(v.PerPlatform) == 0
}

// ForTarget возвращает значение по умолчанию для целевой машины. Узел значения
// может быть #if-выражением (только @runtime.* globals) — оно вычисляется здесь.
//
// Обёртка над ForTargetIn без vars-контекста — для вызывающих, которым
// доступ к user-vars в default_value не нужен.
func (v VarDefaultValue) ForTarget(target TargetSpec) string {
	return v.ForTargetIn(target, nil, nil)
}

// ForTargetIn — то же с доступом к user-vars в предикатах #if (SPEC 097).
//
// resolved содержит vars, объявленные РАНЬШЕ этой var по списку template.vars
// (однопроходный резолв в ResolveTemplateVarsFor): default_value может
// ссылаться bare-формой на bool-var выше себя — например proxy_in_listen
// дефолтится по @gateway_mode. Forward-ссылка (на var ниже по списку) не
// найдётся в resolved и даст false с warning — это осознанный контракт,
// а не гонка: порядок объявления в шаблоне и есть порядок вычисления.
func (v VarDefaultValue) ForTargetIn(target TargetSpec, varTypes map[string]string, resolved map[string]ResolvedVar) string {
	target = target.Normalized()
	if len(v.PerPlatform) > 0 {
		// Top-level #if: default_value == {"#if": {...}}.
		if node, ok := v.PerPlatform["#if"]; ok && len(v.PerPlatform) == 1 {
			return resolveDefaultNode(map[string]interface{}{"#if": node}, target, varTypes, resolved)
		}
		for _, k := range defaultValueKeyOrder(target.GOOS, target.GOARCH) {
			if val, ok := v.PerPlatform[k]; ok {
				if s := resolveDefaultNode(val, target, varTypes, resolved); s != "" {
					return s
				}
			}
		}
	}
	return strings.TrimSpace(v.Scalar)
}

// resolveDefaultNode разрешает узел default_value:
//   - строка → trimmed;
//   - объект {"#if": {...}} → вычисляет #if (@runtime.* globals + user-vars,
//     объявленные раньше — см. ForTargetIn) и рекурсивно разрешает ветку;
//   - число/bool → строковое представление; иначе → "".
func resolveDefaultNode(node interface{}, target TargetSpec, varTypes map[string]string, resolved map[string]ResolvedVar) string {
	switch x := node.(type) {
	case string:
		return strings.TrimSpace(x)
	case map[string]interface{}:
		body, ok := x["#if"]
		if !ok {
			return ""
		}
		bodyMap, ok := body.(map[string]interface{})
		if !ok {
			return ""
		}
		branch, take := selectIfBranch(bodyMap, varTypes, resolved, target)
		if !take {
			return ""
		}
		return resolveDefaultNode(branch, target, varTypes, resolved)
	case nil:
		return ""
	default:
		return defaultValueStringify(node)
	}
}

// defaultValueKeyOrder задаёт перебор ключей объекта default_value: как platforms — только GOOS,
// плюс псевдоним win7 (только windows/386), затем default. GOARCH в именах ключей не используется.
func defaultValueKeyOrder(goos, goarch string) []string {
	goos = strings.ToLower(strings.TrimSpace(goos))
	goarch = strings.ToLower(strings.TrimSpace(goarch))
	var keys []string
	if goos == "windows" && goarch == "386" {
		keys = append(keys, "win7")
	}
	if goos != "" {
		keys = append(keys, goos)
	}
	keys = append(keys, "default")
	return keys
}

// UnmarshalJSON принимает строку, число, bool, объект string→значение или null.
func (v *VarDefaultValue) UnmarshalJSON(data []byte) error {
	*v = VarDefaultValue{}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	switch data[0] {
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("default_value: %w", err)
		}
		v.Scalar = s
		return nil
	case '{':
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("default_value object: %w", err)
		}
		if len(m) == 0 {
			return nil
		}
		v.PerPlatform = make(map[string]interface{}, len(m))
		for k, val := range m {
			sk := strings.ToLower(strings.TrimSpace(k))
			if sk == "" {
				continue
			}
			// #if-дерево сохраняем как есть (вычислится в ForTarget по @runtime.*);
			// скаляры приводим к строке.
			if tree, ok := val.(map[string]interface{}); ok {
				v.PerPlatform[sk] = tree
			} else {
				v.PerPlatform[sk] = defaultValueStringify(val)
			}
		}
		return nil
	default:
		// число, bool
		var n json.Number
		if err := json.Unmarshal(data, &n); err == nil && n != "" {
			v.Scalar = n.String()
			return nil
		}
		var b bool
		if err := json.Unmarshal(data, &b); err == nil {
			if b {
				v.Scalar = "true"
			} else {
				v.Scalar = "false"
			}
			return nil
		}
		return fmt.Errorf("default_value: want string, number, bool, or object, got %s", string(truncateForErr(data, 40)))
	}
}

// defaultValueStringify renders an ALREADY-DECODED default_value
// (interface{} from json.Unmarshal of the vars map) to a string.
//
// Intentionally distinct from readJSONLiteralAsString (vars_resolve.go),
// which takes raw json.RawMessage. Two differences are load-bearing: (1)
// input type — decoded value here vs raw bytes there; (2) the default branch
// here is a fmt.Sprint catch-all (default_value is trusted template content,
// so a best-effort string is fine), whereas the raw reader strictly returns
// "" for non-literals. Do NOT merge the two (SPEC 069 §5.4 — verified
// intentional divergence, not accidental duplication).
func defaultValueStringify(val interface{}) string {
	switch x := val.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(x, 'f', -1, 64))
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func truncateForErr(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return append(append([]byte{}, b[:n]...), '.', '.', '.')
}

// MarshalJSON: объект или строка (для симметрии тестов).
func (v VarDefaultValue) MarshalJSON() ([]byte, error) {
	if len(v.PerPlatform) > 0 {
		return json.Marshal(v.PerPlatform)
	}
	if strings.TrimSpace(v.Scalar) != "" {
		return json.Marshal(v.Scalar)
	}
	return []byte("null"), nil
}
