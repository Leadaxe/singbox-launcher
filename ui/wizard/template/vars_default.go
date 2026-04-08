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
	Scalar      string
	PerPlatform map[string]string // нормализованные ключи в нижнем регистре
}

// IsEmpty true, если нет ни скаляра, ни карты.
func (v VarDefaultValue) IsEmpty() bool {
	return strings.TrimSpace(v.Scalar) == "" && len(v.PerPlatform) == 0
}

// ForPlatform возвращает значение по умолчанию для goos/goarch.
func (v VarDefaultValue) ForPlatform(goos, goarch string) string {
	if len(v.PerPlatform) > 0 {
		for _, k := range defaultValueKeyOrder(goos, goarch) {
			if val, ok := v.PerPlatform[k]; ok {
				val = strings.TrimSpace(val)
				if val != "" {
					return val
				}
			}
		}
	}
	return strings.TrimSpace(v.Scalar)
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
		v.PerPlatform = make(map[string]string, len(m))
		for k, val := range m {
			sk := strings.ToLower(strings.TrimSpace(k))
			if sk == "" {
				continue
			}
			v.PerPlatform[sk] = defaultValueStringify(val)
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
