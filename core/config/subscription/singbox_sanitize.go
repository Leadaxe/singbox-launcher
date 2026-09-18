package subscription

import (
	"strings"

	"singbox-launcher/internal/debuglog"
)

// SPEC 094 A2 / Р2 — санитайзы над ГОТОВОЙ sing-box outbound map.
//
// Импорт sing-box JSON не разбирает outbound на поля и не собирает обратно:
// входная map уже имеет тот же формат, который эмитит лаунчер. Вместо
// реэмиссии она прогоняется через те же проверки, что и URI-путь, — иначе
// один битый узел из чужого конфига валит весь config.json (ядро отвергает
// конфиг целиком на unknown field / невалидном значении).
//
// Каждая функция правит map на месте и возвращает признак «что-то изменено»
// только там, где вызывающему это важно для лога.

// singboxServiceTypes — служебные типы, которые не являются узлами.
// Собственный набор, намеренно не переиспользующий Xray-список
// (у Xray это freedom/blackhole/loopback, у sing-box — direct/block/dns).
var singboxServiceTypes = map[string]struct{}{
	"direct": {}, "block": {}, "dns": {},
}

// singboxGroupTypes — типы outbound-групп.
var singboxGroupTypes = map[string]struct{}{
	"selector": {}, "urltest": {},
}

// IsSingboxServiceType сообщает, является ли тип служебным (не узел).
func IsSingboxServiceType(t string) bool {
	_, ok := singboxServiceTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// IsSingboxGroupType сообщает, является ли тип группой (selector/urltest).
func IsSingboxGroupType(t string) bool {
	_, ok := singboxGroupTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// quicOutboundTypes — типы, работающие поверх QUIC. Для них ядро не умеет
// uTLS/REALITY: STDConfig() возвращает ошибку, а QUIC-путь фолбэчит именно
// на него, и нода становится мёртвой.
// masque: h3 несёт TLS внутри QUIC, а ядро (SPEC 062 §1.3) для него игнорирует
// utls/reality/ech с предупреждением — снимаем их здесь, как у остальных QUIC.
var quicOutboundTypes = map[string]struct{}{
	"hysteria": {}, "hysteria2": {}, "tuic": {}, "masque": {},
}

// SanitizeSingboxOutboundMap приводит импортированный outbound к форме,
// которую ядро гарантированно принимает. Правит ob на месте.
//
// tag используется только в логах.
//
// Возвращает коды применённых деградаций (contract/registry/warnings.json).
// Санитайзер работает с сырой map и узла не знает, а код обязан оказаться
// НА УЗЛЕ: лог читает только тот, кто в него смотрит, а конверт узла едет
// в UI и в LxBox — обе стороны должны сообщать об одной деградации
// одинаково. Вызывающий вешает возвращённое через node.AddWarning.
func SanitizeSingboxOutboundMap(ob map[string]interface{}, tag string) []string {
	if ob == nil {
		return nil
	}
	obType := strings.ToLower(strings.TrimSpace(mapString(ob, "type")))

	sanitizeSingboxMasqueLegacy(ob, obType, tag)
	sanitizeSingboxTLS(ob, obType, tag)
	sanitizeSingboxHysteria2Obfs(ob, obType, tag)
	sanitizeSingboxHysteriaObfs(ob, obType, tag)
	return nil
}

// sanitizeSingboxMasqueLegacy СТРИПАЕТ у masque-outbound ключи чужого
// диалекта (плоские `network`/`sni`/`skip_cert_verify`) — без переноса
// значений. Legacy-приём снесён контрактом 0.8.0 (D-078): значения больше
// не читаются; узел живёт на канонических `vhttp` + вложенном `tls`, а при
// их отсутствии — на дефолтах.
//
// Удалять ключи всё равно обязательно, просто убрать функцию нельзя:
// плоский `sni` рядом с `tls.server_name` — два источника имени, и при
// расхождении ядро падает fail-fast'ом; протащенный legacy-ключ ронял бы
// конфиг целиком вместо тихой деградации одного узла.
func sanitizeSingboxMasqueLegacy(ob map[string]interface{}, obType, tag string) {
	if obType != "masque" {
		return
	}
	stripped := false
	for _, k := range []string{"network", "sni", "skip_cert_verify"} {
		if _, has := ob[k]; has {
			delete(ob, k)
			stripped = true
		}
	}
	if stripped {
		debuglog.DebugLog("Parser: singbox import %q: masque legacy flat keys stripped (0.8.0, values ignored)", tag)
	}
}

// sanitizeSingboxTLS чистит блок tls: uTLS allowlist, REALITY pbk/short_id,
// key_share, снятие uTLS/REALITY на QUIC-типах.
//
// Возвращает код деградации (или "") — прокидывает наружу код из
// sanitizeSingboxReality, вешать его здесь не на что.
func sanitizeSingboxTLS(ob map[string]interface{}, obType, tag string) {
	tlsRaw, ok := ob["tls"]
	if !ok {
		return
	}
	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok {
		// tls не объект — ядро отвергнет конфиг; безопаснее снять поле.
		debuglog.WarnLog("Parser: singbox import %q: tls is not an object — dropping field", tag)
		delete(ob, "tls")
		return
	}

	// Явный tls:{enabled:false} роняет ядра 1.14.0-lx.5..lx.18 SIGSEGV'ом при
	// первом dial (SPEC 045). Блок в этом случае не нужен вовсе.
	if enabled, ok := tlsMap["enabled"].(bool); ok && !enabled {
		delete(ob, "tls")
		return
	}

	if _, isQUIC := quicOutboundTypes[obType]; isQUIC {
		// SPEC 094 A2: на QUIC срезаем utls и reality целиком.
		if _, had := tlsMap["utls"]; had {
			delete(tlsMap, "utls")
			debuglog.DebugLog("Parser: singbox import %q: stripped utls from %s (QUIC)", tag, obType)
		}
		if _, had := tlsMap["reality"]; had {
			// key_share уезжает вместе с блоком и кода не даёт: снят не он,
			// а весь REALITY (policy.quic_strip).
			delete(tlsMap, "reality")
			debuglog.DebugLog("Parser: singbox import %q: stripped reality from %s (QUIC)", tag, obType)
		}
	}

	if len(tlsMap) == 0 {
		delete(ob, "tls")
	}
}

// Дефолт полосы Hysteria v1 здесь БОЛЬШЕ НЕ ПОДСТАВЛЯЕТСЯ: его подставляет
// реестр (default_when у hysteria.body.up_mbps / down_mbps, SPEC 131 W2d) —
// одинаково для ссылки, JSON-тела и Xray-объекта. Прежде та же константа 100
// лежала здесь, в URI-парсере и в Xray-конвертере тремя копиями, и узел
// получал её не на всех дорогах.

// sanitizeSingboxHysteriaObfs приводит obfs узла Hysteria v1 к форме ядра.
//
// У v1 obfs — ПЛОСКАЯ строка-секрет (option/hysteria.go), а у hysteria2 —
// объект {type,password}. Провайдеры, конвертирующие конфиги автоматически,
// иногда кладут в v1 объект от v2: ядро отвергает такой outbound на разборе
// («json: cannot unmarshal object into ... string») и это роняет ВЕСЬ конфиг,
// а не одну ноду. Достаём секрет, если он там есть, иначе снимаем блок.
func sanitizeSingboxHysteriaObfs(ob map[string]interface{}, obType, tag string) {
	if obType != "hysteria" {
		return
	}
	obfsRaw, ok := ob["obfs"]
	if !ok {
		return
	}
	switch v := obfsRaw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			delete(ob, "obfs")
		}
	case map[string]interface{}:
		password := strings.TrimSpace(mapString(v, "password"))
		if password == "" {
			debuglog.WarnLog("Parser: singbox import %q: hysteria obfs object without password — dropping obfs", tag)
			delete(ob, "obfs")
			return
		}
		debuglog.WarnLog("Parser: singbox import %q: hysteria obfs given as object — using its password as the obfs string", tag)
		ob["obfs"] = password
	default:
		debuglog.WarnLog("Parser: singbox import %q: hysteria obfs of unexpected shape — dropping obfs", tag)
		delete(ob, "obfs")
	}
}

// sanitizeSingboxHysteria2Obfs снимает обфускацию с неподдерживаемым типом.
func sanitizeSingboxHysteria2Obfs(ob map[string]interface{}, obType, tag string) {
	if obType != "hysteria2" {
		return
	}
	obfsRaw, ok := ob["obfs"]
	if !ok {
		return
	}
	obfsMap, ok := obfsRaw.(map[string]interface{})
	if !ok {
		delete(ob, "obfs")
		return
	}
	obfsType := strings.ToLower(strings.TrimSpace(mapString(obfsMap, "type")))
	if obfsType == "" {
		delete(ob, "obfs")
		return
	}
	// Набор типов — из реестра (hysteria2.body.obfs.type); прежде он жил
	// константой в парсере ссылок, и та копия ушла в W2d.
	if obfsType != "salamander" && obfsType != "gecko" {
		// "unknown obfs type" — fatal для всего конфига.
		debuglog.WarnLog("Parser: singbox import %q: unsupported hysteria2 obfs %q — dropping obfs", tag, obfsType)
		delete(ob, "obfs")
		return
	}
	if strings.TrimSpace(mapString(obfsMap, "password")) == "" {
		// "missing obfs password" — тоже fatal.
		debuglog.WarnLog("Parser: singbox import %q: hysteria2 obfs without password — dropping obfs", tag)
		delete(ob, "obfs")
		return
	}
	obfsMap["type"] = obfsType
}

// mapString возвращает строковое поле map или "".
func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// toStringValue приводит значение к строке, если это строка; иначе "".
// Числовые/булевы значения в этих полях не легальны и должны быть отброшены.
func toStringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}
