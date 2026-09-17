// File outbound_tls_emit.go — эмиссия блока tls outbound'а.
//
// Allowlist по OutboundTLSOptions ядра (sing-box option/tls.go): всё, что
// ядро знает, обязано пережить разбор → эмиссию. До выноса сюда встроенный
// блок генератора знал семь полей, и sing-box-импорт терял certificate,
// alpn/пины (приезжали []interface{}, а ассерт ждал []string), версии,
// client_*, fragment — LxBox #140: naive с certificate после сохранения
// оставался без сертификата.
//
// Не эмитится намеренно:
//   - ech — ядро собрано без with_ech (D-006, registry/tls.json), блок
//     уронил бы конфиг;
//   - неизвестные ключи — ядро отвергает unknown field на всём конфиге,
//     а карта tls приходит и от URI-парсеров, и от чужого JSON.
package config

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
)

// naiveTLSKeys — единственные TLS-поля, которые ядро читает у naive
// (protocol/naive/outbound.go NewOutbound): всё остальное — disable_sni,
// insecure, alpn, версии, cipher_suites, curve_preferences, client_*,
// fragment, kernel_*, utls, reality — ядро отвергает фаталом на ВЕСЬ конфиг,
// а certificate_public_key_sha256 молча не применяет (пиннинг, которого
// нет). Режем здесь, а не в санитайзере импорта: эмиттер — последний рубеж
// и для URI-, и для JSON-пути. Норма §22 TASKS_LXBOX, паритет с LxBox.
var naiveTLSKeys = map[string]struct{}{
	"enabled": {}, "server_name": {}, "certificate": {}, "certificate_path": {},
}

// emitOutboundTLSJSON строит `{...}` для tls узла схемы scheme. Второе
// значение false — блок опускается целиком (нет карты или enabled:false,
// SPEC 045).
func emitOutboundTLSJSON(scheme string, outbound map[string]interface{}) (string, bool) {
	if outbound == nil {
		return "", false
	}
	tlsData, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		return "", false
	}
	if enabled, ok := tlsData["enabled"].(bool); ok && !enabled {
		// Omit the block entirely instead of emitting `"tls":{"enabled":false}`.
		// Both mean "dial plain TCP", but the explicit disabled form crashes
		// sing-box cores 1.14.0-lx.5..lx.18 on the first dial of a
		// trojan/vless node (nil TLS config wrapped in a live dialer —
		// SPEC 045). Backstop for nodes that reach the generator without
		// going through the URI parsers.
		return "", false
	}

	if scheme == "naive" {
		filtered := make(map[string]interface{}, len(naiveTLSKeys))
		for k, v := range tlsData {
			if _, keep := naiveTLSKeys[k]; keep {
				filtered[k] = v
			} else {
				debuglog.WarnLog("Generator: naive %q: tls.%s is not supported by the core — dropped", mapStringValue(outbound, "tag"), k)
			}
		}
		tlsData = filtered
	}

	var parts []string
	addBool := func(key string, onlyTrue bool) {
		if v, ok := tlsData[key].(bool); ok && (v || !onlyTrue) {
			parts = append(parts, fmt.Sprintf(`"%s":%v`, key, v))
		}
	}
	addString := func(key string) {
		if v, ok := tlsData[key].(string); ok && v != "" {
			parts = append(parts, fmt.Sprintf(`"%s":%s`, key, marshalJSONString(v)))
		}
	}
	// Listable[string] ядра: строка или массив. Форму сохраняем как приехала —
	// человек, набравший certificate строкой, увидит после сохранения строку.
	addListable := func(key string) {
		switch v := tlsData[key].(type) {
		case string:
			if v != "" {
				parts = append(parts, fmt.Sprintf(`"%s":%s`, key, marshalJSONString(v)))
			}
		default:
			if list := tolerantStringSlice(v); len(list) > 0 {
				listJSON, _ := json.Marshal(list)
				parts = append(parts, fmt.Sprintf(`"%s":%s`, key, string(listJSON)))
			}
		}
	}

	// Порядок = порядок полей OutboundTLSOptions.
	addBool("enabled", false)
	addBool("disable_sni", true)
	addString("server_name")
	addBool("insecure", true)
	addListable("alpn")
	addString("min_version")
	addString("max_version")
	addListable("cipher_suites")
	addListable("curve_preferences")
	addListable("certificate")
	addString("certificate_path")
	// Пин сертификата (hysteria2 `pinSHA256=`): без эмиссии узел с
	// самоподписанным сертификатом получал обычную CA-проверку и не
	// подключался, хотя URI содержал всё нужное (SPEC 103, фаза 2).
	addListable("certificate_public_key_sha256")
	addListable("client_certificate")
	addString("client_certificate_path")
	addListable("client_key")
	addString("client_key_path")
	addBool("fragment", true)
	addString("fragment_fallback_delay")
	addBool("record_fragment", true)
	// kTLS ядро принимает только на Linux («kTLS is only supported on
	// Linux» — и это отказ ВСЕГО конфига, а не узла). На других ОС поле
	// опускается: узел живёт без ускорения, конфиг живёт.
	if runtime.GOOS == "linux" {
		addBool("kernel_tx", true)
		addBool("kernel_rx", true)
	}

	if utls, ok := tlsData["utls"].(map[string]interface{}); ok {
		var utlsParts []string
		if utlsEnabled, ok := utls["enabled"].(bool); ok {
			utlsParts = append(utlsParts, fmt.Sprintf(`"enabled":%v`, utlsEnabled))
		}
		// Emit fingerprint only when sing-box knows the name. An unknown value
		// (e.g. fp=HelloChrome_120 from a raw uTLS identifier) aborts config
		// load for every outbound; omitting the key lets sing-box pick its
		// default Chrome hello instead.
		if fingerprint, ok := utls["fingerprint"].(string); ok {
			if fingerprint = subscription.NormalizeUTLSFingerprint(fingerprint); fingerprint != "" {
				utlsParts = append(utlsParts, fmt.Sprintf(`"fingerprint":%s`, marshalJSONString(fingerprint)))
			}
		}
		parts = append(parts, fmt.Sprintf(`"utls":{%s}`, strings.Join(utlsParts, ",")))
	}

	if reality, ok := tlsData["reality"].(map[string]interface{}); ok {
		var realityParts []string
		if realityEnabled, ok := reality["enabled"].(bool); ok {
			realityParts = append(realityParts, fmt.Sprintf(`"enabled":%v`, realityEnabled))
		}
		if publicKey, ok := reality["public_key"].(string); ok {
			realityParts = append(realityParts, fmt.Sprintf(`"public_key":%s`, marshalJSONString(publicKey)))
		}
		if shortID, ok := reality["short_id"].(string); ok {
			realityParts = append(realityParts, fmt.Sprintf(`"short_id":%s`, marshalJSONString(shortID)))
		}
		// key_share — SPEC 089 ядра (sing-box-lx ≥ 1.14.1-lx.4): "" = как
		// несёт отпечаток, "classical" = без X25519MLKEM768, "hybrid" =
		// гибрид обязателен. Enum ядра закрытый: любое другое значение —
		// ошибка загрузки ВСЕГО конфига, поэтому мусор не эмитится (узел
		// живёт без поля), а не передаётся дальше.
		if keyShare, ok := reality["key_share"].(string); ok && keyShare != "" {
			if keyShare == "hybrid" || keyShare == "classical" {
				realityParts = append(realityParts, fmt.Sprintf(`"key_share":%s`, marshalJSONString(keyShare)))
			} else {
				debuglog.WarnLog("Generator: %q: tls.reality.key_share %q is not hybrid/classical — dropped", mapStringValue(outbound, "tag"), keyShare)
			}
		}
		parts = append(parts, fmt.Sprintf(`"reality":{%s}`, strings.Join(realityParts, ",")))
	}

	return "{" + strings.Join(parts, ",") + "}", true
}

// mapStringValue — строковое поле карты или "".
func mapStringValue(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
