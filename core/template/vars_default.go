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
	// строка ИЛИ #if-дерево (map[string]interface{}), вычисляемое в ForPlatform по
	// @runtime.* globals (SPEC 067). Единственный спец-ключ "#if" = top-level выражение.
	PerPlatform map[string]interface{}
}

// IsEmpty true, если нет ни скаляра, ни карты.
func (v VarDefaultValue) IsEmpty() bool {
	return strings.TrimSpace(v.Scalar) == "" && len(v.PerPlatform) == 0
}

// ForPlatform возвращает значение по умолчанию для goos/goarch. Узел значения
// может быть #if-выражением (только @runtime.* globals) — оно вычисляется здесь.
func (v VarDefaultValue) ForPlatform(goos, goarch string) string {
	if len(v.PerPlatform) > 0 {
		// Top-level #if: default_value == {"#if": {...}}.
		if node, ok := v.PerPlatform["#if"]; ok && len(v.PerPlatform) == 1 {
			return resolveDefaultNode(map[string]interface{}{"#if": node}, goos, goarch)
		}
		for _, k := range defaultValueKeyOrder(goos, goarch) {
			if val, ok := v.PerPlatform[k]; ok {
				if s := resolveDefaultNode(val, goos, goarch); s != "" {
					return s
				}
			}
		}
	}
	return strings.TrimSpace(v.Scalar)
}

// resolveDefaultNode разрешает узел default_value:
//   - строка → trimmed;
//   - объект {"#if": {...}} → вычисляет #if (только @runtime.* globals, без user-vars
//     и без resolved-map) и рекурсивно разрешает выбранную ветку;
//   - число/bool → строковое представление; иначе → "".
func resolveDefaultNode(node interface{}, goos, goarch string) string {
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
		branch, take := selectIfBranch(bodyMap, nil, nil, goos, goarch)
		if !take {
			return ""
		}
		return resolveDefaultNode(branch, goos, goarch)
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
			// #if-дерево сохраняем как есть (вычислится в ForPlatform по @runtime.*);
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
