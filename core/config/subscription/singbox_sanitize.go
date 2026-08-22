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
	"hysteria2": {}, "tuic": {}, "masque": {},
}

// SanitizeSingboxOutboundMap приводит импортированный outbound к форме,
// которую ядро гарантированно принимает. Правит ob на месте.
//
// tag используется только в логах.
func SanitizeSingboxOutboundMap(ob map[string]interface{}, tag string) {
	if ob == nil {
		return
	}
	obType := strings.ToLower(strings.TrimSpace(mapString(ob, "type")))

	sanitizeSingboxMasqueLegacy(ob, obType, tag)
	sanitizeSingboxTLS(ob, obType, tag)
	sanitizeSingboxFlow(ob, tag)
	sanitizeSingboxPacketEncoding(ob, tag)
	sanitizeSingboxHysteria2Obfs(ob, obType, tag)
}

// sanitizeSingboxMasqueLegacy переводит masque-outbound из чужого диалекта
// (плоские `network`/`sni`/`skip_cert_verify`) в схему ядра SPEC 062: `vhttp`
// плюс вложенный `tls`. Ядро принимает и старую форму до lx.30, но смешивать
// нельзя: плоский `sni` рядом с `tls.server_name` — два источника имени, и при
// расхождении ядро падает fail-fast'ом.
//
// Новое имя всегда выигрывает: если импортируемый конфиг уже нёс `vhttp` или
// `tls.server_name`, legacy-дубликат просто снимается.
func sanitizeSingboxMasqueLegacy(ob map[string]interface{}, obType, tag string) {
	if obType != "masque" {
		return
	}

	if legacy := strings.TrimSpace(mapString(ob, "network")); legacy != "" {
		if strings.TrimSpace(mapString(ob, "vhttp")) == "" {
			ob["vhttp"] = legacy
		}
		delete(ob, "network")
		debuglog.DebugLog("Parser: singbox import %q: masque network → vhttp", tag)
	}

	sni := strings.TrimSpace(mapString(ob, "sni"))
	insecure, hasInsecure := ob["skip_cert_verify"].(bool)
	if sni == "" && !hasInsecure {
		return
	}
	delete(ob, "sni")
	delete(ob, "skip_cert_verify")

	tlsMap, _ := ob["tls"].(map[string]interface{})
	if tlsMap == nil {
		tlsMap = map[string]interface{}{}
	}
	if sni != "" {
		if _, has := tlsMap["server_name"]; !has {
			tlsMap["server_name"] = sni
		}
	}
	if insecure {
		if _, has := tlsMap["insecure"]; !has {
			tlsMap["insecure"] = true
		}
	}
	if len(tlsMap) > 0 {
		ob["tls"] = tlsMap
		debuglog.DebugLog("Parser: singbox import %q: masque flat sni/skip_cert_verify → tls block", tag)
	}
}

// sanitizeSingboxTLS чистит блок tls: uTLS allowlist, REALITY pbk/short_id,
// снятие uTLS/REALITY на QUIC-типах.
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
			delete(tlsMap, "reality")
			debuglog.DebugLog("Parser: singbox import %q: stripped reality from %s (QUIC)", tag, obType)
		}
	} else {
		sanitizeSingboxUTLS(tlsMap, tag)
		sanitizeSingboxReality(tlsMap, tag)
	}

	if len(tlsMap) == 0 {
		delete(ob, "tls")
	}
}

// sanitizeSingboxUTLS прогоняет fingerprint через allowlist sing-box.
func sanitizeSingboxUTLS(tlsMap map[string]interface{}, tag string) {
	utlsRaw, ok := tlsMap["utls"]
	if !ok {
		return
	}
	utlsMap, ok := utlsRaw.(map[string]interface{})
	if !ok {
		delete(tlsMap, "utls")
		return
	}
	fp := mapString(utlsMap, "fingerprint")
	normalized := utlsFingerprintOrFallback(fp)
	if normalized == "" {
		// Поле пустое (а не мусорное) — блока utls тут просто нет.
		delete(tlsMap, "utls")
		return
	}
	// Мусор канонизируется в chrome (D-029), а не снимает блок целиком: раньше
	// одна и та же нода получала utls на URI-пути и теряла его на импорте.
	utlsMap["fingerprint"] = normalized
}

// sanitizeSingboxReality валидирует public_key и чистит short_id.
func sanitizeSingboxReality(tlsMap map[string]interface{}, tag string) {
	realityRaw, ok := tlsMap["reality"]
	if !ok {
		return
	}
	realityMap, ok := realityRaw.(map[string]interface{})
	if !ok {
		delete(tlsMap, "reality")
		return
	}
	if enabled, ok := realityMap["enabled"].(bool); ok && !enabled {
		delete(tlsMap, "reality")
		return
	}

	pbk := mapString(realityMap, "public_key")
	if !isValidRealityPublicKey(pbk) {
		// pbk=enabled / true / мусор: ядро отвергает весь конфиг
		// ("invalid public_key"). Деградируем ноду до plain TLS.
		debuglog.WarnLog("Parser: singbox import %q: invalid REALITY public_key — degrading to plain TLS", tag)
		delete(tlsMap, "reality")
		return
	}

	if sid, ok := realityMap["short_id"]; ok {
		normalized := normalizeRealityShortID(toStringValue(sid))
		if normalized == "" {
			// Пустой short_id для REALITY легален; мусорный — нет.
			delete(realityMap, "short_id")
		} else {
			realityMap["short_id"] = normalized
		}
	}
}

// sanitizeSingboxFlow оставляет только xtls-rprx-vision и гасит flow при транспорте.
func sanitizeSingboxFlow(ob map[string]interface{}, tag string) {
	flowRaw, ok := ob["flow"]
	if !ok {
		return
	}
	flow := strings.TrimSpace(toStringValue(flowRaw))
	if flow == "" {
		delete(ob, "flow")
		return
	}
	if flow != "xtls-rprx-vision" {
		// none, deprecated xtls-rprx-direct/origin/splice, мусор —
		// всё это ядро либо отвергает, либо трактует неверно.
		debuglog.DebugLog("Parser: singbox import %q: dropping unsupported flow %q", tag, flow)
		delete(ob, "flow")
		return
	}
	// vision валиден только на голом TLS: с транспортом ядро отвергает узел.
	if _, hasTransport := ob["transport"]; hasTransport {
		debuglog.DebugLog("Parser: singbox import %q: dropping vision flow (transport present)", tag)
		delete(ob, "flow")
	}
}

// sanitizeSingboxPacketEncoding применяет allowlist sing-box.
func sanitizeSingboxPacketEncoding(ob map[string]interface{}, tag string) {
	peRaw, ok := ob["packet_encoding"]
	if !ok {
		return
	}
	pe := strings.ToLower(strings.TrimSpace(toStringValue(peRaw)))
	switch pe {
	case "xudp", "packetaddr":
		ob["packet_encoding"] = pe
	case "", "none":
		// "no special encoding" — эквивалентно отсутствию поля.
		delete(ob, "packet_encoding")
	default:
		// Неизвестное значение даёт панику в ядре (SPEC 049).
		debuglog.WarnLog("Parser: singbox import %q: unknown packet_encoding %q — dropping field", tag, pe)
		delete(ob, "packet_encoding")
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
	if !isValidHysteria2ObfsType(obfsType) {
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
