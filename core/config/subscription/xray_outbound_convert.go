package subscription

// Xray-вход: общие выемки УРОВНЯ ДОКУМЕНТА (SPEC 133).
//
// Конвертеры протоколов отсюда СНЯТЫ — элемент (и звено цепочки) разбирает
// движок реестра (xray_element_engine.go). Осталось то, что читает не тело
// узла, а запись документа: скаляры карты и ссылка на соседний outbound из
// sockopt. Уровень документа на движок ещё не переведён — это следующая
// волна кампании, и вместе с ней уедет и этот файл.

import (
	"fmt"
	"strings"
)

// xrayMapString returns string value for key in m.
func xrayMapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	case fmt.Stringer:
		return strings.TrimSpace(s.String())
	default:
		return strings.TrimSpace(fmt.Sprint(s))
	}
}

// xrayJSONInt coerces JSON-decoded numbers to int.
func xrayJSONInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

// xraySockoptDialerRef returns dialerProxy or dialer from streamSettings.sockopt (Xray).
func xraySockoptDialerRef(streamSettings map[string]interface{}) string {
	if streamSettings == nil {
		return ""
	}
	sockopt, _ := streamSettings["sockopt"].(map[string]interface{})
	if sockopt == nil {
		return ""
	}
	if s := xrayMapString(sockopt, "dialerProxy"); s != "" {
		return s
	}
	return xrayMapString(sockopt, "dialer")
}
