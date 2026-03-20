package subscription

import (
	"net/url"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// queryGetFold returns the first value for a query key, matching case-insensitively.
// Subscriptions use allowinsecure=0, AllowInsecure=1, etc.
func queryGetFold(q url.Values, name string) string {
	for k, vs := range q {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// normalizePercentDecodeLoop applies URL-unescape until stable (fixes multiply-encoded alpn, etc.).
func normalizePercentDecodeLoop(s string) string {
	for {
		dec, err := url.QueryUnescape(s)
		if err != nil || dec == s {
			break
		}
		s = dec
	}
	return s
}

func tlsInsecureTrue(q url.Values) bool {
	for _, key := range []string{"insecure", "allowInsecure", "allowinsecure"} {
		v := strings.TrimSpace(strings.ToLower(queryGetFold(q, key)))
		if v == "1" || v == "true" || v == "yes" {
			return true
		}
	}
	return false
}

// normalizeUTLSFingerprint maps subscription variants to sing-box utls names (lowercase).
func normalizeUTLSFingerprint(fp string) string {
	fp = strings.TrimSpace(strings.ToLower(fp))
	if fp == "" {
		return ""
	}
	return fp
}

// plaintextVLESSPorts are common subscription ports where TLS is typically off (plain HTTP / CF HTTP).
var plaintextVLESSPorts = map[int]struct{}{
	80: {}, 8080: {}, 8880: {}, 2052: {}, 2082: {}, 2086: {}, 2095: {},
}

func shouldVLESSSkipTLSForPort(port int) bool {
	_, ok := plaintextVLESSPorts[port]
	return ok
}

// uriTransportFromQuery builds sing-box V2Ray transport for VLESS/Trojan from URI query.
// See: https://sing-box.sagernet.org/configuration/shared/v2ray-transport/
func uriTransportFromQuery(q url.Values) (map[string]interface{}, bool) {
	typ := strings.ToLower(strings.TrimSpace(queryGetFold(q, "type")))
	headerType := strings.ToLower(strings.TrimSpace(queryGetFold(q, "headerType")))

	// Xray: TCP/raw with HTTP header camouflage → sing-box "http" transport (not plain TCP).
	if (typ == "raw" || typ == "tcp") && headerType == "http" {
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	}

	switch typ {
	case "ws":
		t := map[string]interface{}{"type": "ws"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["headers"] = map[string]string{"Host": host}
		}
		return t, true
	case "grpc":
		t := map[string]interface{}{"type": "grpc"}
		sn := queryGetFold(q, "serviceName")
		if sn == "" {
			sn = queryGetFold(q, "service_name")
		}
		if sn != "" {
			t["service_name"] = sn
		} else if p := queryGetFold(q, "path"); p != "" {
			t["service_name"] = p
		}
		return t, true
	case "http":
		// HTTP transport: "host" is a list in sing-box (not a plain Host header).
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	case "xhttp":
		// Xray sharing label "xhttp" → sing-box "httpupgrade" (host/path/headers only; no "mode" in schema).
		t := map[string]interface{}{"type": "httpupgrade"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = host
		}
		return t, true
	case "raw", "tcp", "":
		return nil, false
	default:
		return nil, false
	}
}

func applyTLSQueryExtras(q url.Values, tlsData map[string]interface{}) {
	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpn = normalizePercentDecodeLoop(alpn)
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}
	if tlsInsecureTrue(q) {
		tlsData["insecure"] = true
	}
}

// vlessTLSFromNode returns sing-box tls map for VLESS and whether TLS should be included.
func vlessTLSFromNode(node *configtypes.ParsedNode) (map[string]interface{}, bool) {
	q := node.Query
	sec := strings.ToLower(strings.TrimSpace(queryGetFold(q, "security")))
	pbk := strings.TrimSpace(queryGetFold(q, "pbk"))

	if sec == "none" {
		return nil, false
	}

	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = node.Server
	}
	fp := normalizeUTLSFingerprint(queryGetFold(q, "fp"))
	if fp == "" {
		fp = "random"
	}

	if pbk != "" {
		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": pbk,
				"short_id":   queryGetFold(q, "sid"),
			},
		}
		applyTLSQueryExtras(q, tlsData)
		return tlsData, true
	}

	if sec == "reality" {
		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
		}
		applyTLSQueryExtras(q, tlsData)
		return tlsData, true
	}

	if sec == "" && shouldVLESSSkipTLSForPort(node.Port) {
		return nil, false
	}

	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
		"utls": map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		},
	}
	applyTLSQueryExtras(q, tlsData)
	return tlsData, true
}

// trojanTLSFromNode returns TLS config for Trojan (WebSocket/raw over TLS).
func trojanTLSFromNode(node *configtypes.ParsedNode) map[string]interface{} {
	q := node.Query
	sec := strings.ToLower(strings.TrimSpace(queryGetFold(q, "security")))
	if sec == "none" {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = queryGetFold(q, "host")
	}
	if sni == "" {
		sni = node.Server
	}

	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
	}
	if fp := normalizeUTLSFingerprint(queryGetFold(q, "fp")); fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	applyTLSQueryExtras(q, tlsData)
	return tlsData
}
