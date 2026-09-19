package subscription

// Чтение значений из ТЕЛА узла: мелкие приведения, которыми пользуются
// несколько мест пакета.
//
// Файл появился при снятии рукописных эмиттеров share-URI (SPEC 133): эти
// функции жили в `shareuri_helpers.go` и `shareuri_wireguard.go` рядом с
// эмиссией, но принадлежат не ей — их читают разбор AmneziaWG, проверка
// «ссылка несёт приватный ключ» и сборка AWG3. Оставить их в удалённых
// файлах значило бы держать эмиттеры ради четырёх функций.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// mapGetString — значение поля тела строкой, каким бы числовым типом оно ни
// приехало: тело бывает разобрано из JSON (float64) и построено кодом (int).
func mapGetString(m map[string]interface{}, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

// mapGetInt — то же для целого.
func mapGetInt(m map[string]interface{}, k string) int {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

// splitAndTrim режет строку по разделителю, снимая пробелы и пустые куски.
func splitAndTrim(s string, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// wireGuardPeerMaps приводит массив пиров к срезу карт.
//
// Тело бывает построено кодом (`[]map[string]interface{}`) и разобрано из
// JSON (`[]interface{}`); читающему нужен один вид.
func wireGuardPeerMaps(ep map[string]interface{}) ([]map[string]interface{}, error) {
	v, ok := ep["peers"]
	if !ok {
		return nil, fmt.Errorf("missing peers")
	}
	if typed, ok := v.([]map[string]interface{}); ok {
		if len(typed) == 0 {
			return nil, fmt.Errorf("peers must be a non-empty array")
		}
		return typed, nil
	}
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("peers must be a non-empty array")
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid peer objects")
	}
	return out, nil
}
