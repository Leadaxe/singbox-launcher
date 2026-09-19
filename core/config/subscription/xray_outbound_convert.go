package subscription

// Xray-вход: общие выемки УРОВНЯ ДОКУМЕНТА (SPEC 133).
//
// Конвертеры протоколов отсюда СНЯТЫ — элемент разбирает движок реестра
// (xray_element_engine.go). Осталось то, что читает не тело узла, а запись
// документа: скаляры карты, ссылка на соседний outbound из sockopt и сборка
// ХОПА цепочки из socks-элемента. Уровень документа на движок ещё не
// переведён — это следующая волна кампании, и вместе с ней уедет и этот файл.

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
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

// xrayBuildJumpFromSocksOutbound builds ParsedJump from Xray socks outbound (settings.servers[0]).
func xrayBuildJumpFromSocksOutbound(ob map[string]interface{}, jumpTag string) (*configtypes.ParsedJump, error) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return nil, fmt.Errorf("missing socks settings")
	}
	servers, _ := settings["servers"].([]interface{})
	if len(servers) == 0 {
		return nil, fmt.Errorf("missing socks servers")
	}
	s0, ok := servers[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid socks server")
	}
	addr := xrayMapString(s0, "address")
	port := xrayJSONInt(s0["port"])
	if addr == "" || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid socks address/port")
	}

	jump := &configtypes.ParsedJump{
		Tag:      jumpTag,
		Scheme:   "socks",
		Server:   addr,
		Port:     port,
		Outbound: map[string]interface{}{"version": "5"},
	}
	users, _ := s0["users"].([]interface{})
	if len(users) > 0 {
		if u0, ok := users[0].(map[string]interface{}); ok {
			user := xrayMapString(u0, "user")
			pass := xrayMapString(u0, "pass")
			if user != "" {
				jump.Outbound["username"] = user
			}
			if pass != "" {
				jump.Outbound["password"] = pass
			}
		}
	}
	return jump, nil
}
