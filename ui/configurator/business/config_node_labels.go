package business

import "strings"

// SPEC 095 D2/D3 — метки транспорта и security для подзаголовка узла.
//
// Правила один в один с LxBox (app/lib/models/config_node.dart,
// _deriveTransport / _deriveSecurity): подзаголовок должен читаться одинаково
// на телефоне и на десктопе, иначе пользователь сверяет два разных языка.

// tcpLikeProtocols — протоколы, которые без блока transport идут по голому TCP.
// Для них пустой transport означает "tcp", а не "транспорта нет".
var tcpLikeProtocols = map[string]struct{}{
	"vless": {}, "vmess": {}, "trojan": {}, "anytls": {},
}

// awgNumericKeys — базовые поля обфускации AmneziaWG 1.0.
var awgNumericKeys = []string{"jc", "jmin", "jmax", "s1", "s2", "h1", "h2", "h3", "h4"}

// awgSignatureKeys — signature-пакеты (CPS) AmneziaWG 1.5.
var awgSignatureKeys = []string{"i1", "i2", "i3", "i4", "i5"}

// awgMasqueradeKeys — masquerade-сахар: ядро разворачивает их в CPS-пакет,
// то есть по сути это уже 1.5.
var awgMasqueradeKeys = []string{"ip", "id", "ib"}

// deriveTransport возвращает метку транспорта.
//
// Правила:
//   - masque ходит поверх QUIC/h2, транспорт задан полем network; пусто → h3
//     (дефолт ядра);
//   - transport.type как есть, но "http" показывается как "h2" — так короче и
//     совпадает с тем, что пишут провайдеры;
//   - vless/vmess/trojan/anytls без transport → "tcp";
//   - иначе пусто (у групп, direct, wireguard транспорта нет).
func deriveTransport(nodeType string, raw map[string]interface{}) string {
	if nodeType == "masque" {
		if net := strings.TrimSpace(cfgNodeString(raw, "network")); net != "" {
			return net
		}
		return "h3"
	}

	if tr, ok := raw["transport"].(map[string]interface{}); ok {
		t := strings.TrimSpace(cfgNodeString(tr, "type"))
		if t != "" {
			if t == "http" {
				return "h2"
			}
			return t
		}
	}

	if _, tcpLike := tcpLikeProtocols[nodeType]; tcpLike {
		return "tcp"
	}
	return ""
}

// deriveSecurity возвращает метку защиты канала.
//
// Для WireGuard это уровень AmneziaWG, определяемый СТРУКТУРНО — по наличию
// полей, потому что явной версии в конфиге нет:
//
//	awg2    — ranged-заголовки h1–h4 вида "N-M" либо transport-padding s3/s4;
//	awg1.5  — signature-пакеты i1–i5;
//	awg     — только базовые jc/jmin/jmax/s1/s2 или одиночные h1–h4;
//	суффикс + — masquerade-поля ip/id/ib (ядро разворачивает их в CPS-пакет,
//	            то есть поднимает минимум до 1.5).
//
// Для остальных — TLS/Reality плюс +Vision, если включён xtls-rprx-vision.
func deriveSecurity(nodeType string, raw map[string]interface{}) string {
	if nodeType == "wireguard" {
		return deriveAWGLevel(raw)
	}

	tls, ok := raw["tls"].(map[string]interface{})
	if !ok {
		return ""
	}
	if enabled, _ := tls["enabled"].(bool); !enabled {
		return ""
	}

	base := "TLS"
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			base = "Reality"
		}
	}

	// Vision работает поверх TLS/Reality и только на голом TCP.
	if flow := cfgNodeString(raw, "flow"); strings.HasPrefix(flow, "xtls-rprx-vision") {
		return base + "+Vision"
	}
	return base
}

// deriveAWGLevel определяет уровень AmneziaWG по набору полей.
func deriveAWGLevel(raw map[string]interface{}) string {
	base := ""
	switch {
	case hasRangedHeader(raw) || hasAnyKey(raw, "s3", "s4"):
		base = "awg2"
	case hasAnyKey(raw, awgSignatureKeys...):
		base = "awg1.5"
	case hasAnyKey(raw, awgNumericKeys...):
		base = "awg"
	}

	if hasAnyKey(raw, awgMasqueradeKeys...) {
		// masquerade сам по себе = 1.5; на уже-2.0 остаётся 2.0.
		if base == "awg2" {
			return "awg2+"
		}
		return "awg1.5+"
	}
	return base
}

// hasRangedHeader сообщает, задан ли хоть один заголовок h1–h4 диапазоном
// («N-M»), а не числом. Диапазон появился в AmneziaWG 2.0.
func hasRangedHeader(raw map[string]interface{}) bool {
	for _, key := range []string{"h1", "h2", "h3", "h4"} {
		v, ok := raw[key]
		if !ok {
			continue
		}
		// Диапазон приезжает строкой; одиночное значение — числом.
		if s, isStr := v.(string); isStr && strings.Contains(s, "-") {
			return true
		}
	}
	return false
}

// hasAnyKey сообщает, есть ли в map хоть один из ключей.
func hasAnyKey(raw map[string]interface{}, keys ...string) bool {
	for _, k := range keys {
		if _, ok := raw[k]; ok {
			return true
		}
	}
	return false
}
