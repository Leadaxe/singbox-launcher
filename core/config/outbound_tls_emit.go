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
)

// emitOutboundTLSJSON строит `{...}` для tls. Второе значение false —
// блок опускается целиком (нет карты или enabled:false, SPEC 045).
func emitOutboundTLSJSON(outbound map[string]interface{}) (string, bool) {
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
		parts = append(parts, fmt.Sprintf(`"reality":{%s}`, strings.Join(realityParts, ",")))
	}

	return "{" + strings.Join(parts, ",") + "}", true
}
