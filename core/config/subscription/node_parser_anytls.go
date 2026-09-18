package subscription

import (
	"singbox-launcher/core/config/configtypes"
)

// buildAnyTLSOutbound builds the outbound map for an AnyTLS node.
//
// URI shape (de-facto client format, e.g. sing-box / NekoBox / v2rayN):
//
//	anytls://password@host:port?insecure=1&sni=&alpn=&fp=&udp=#name
//
// The single credential (password) is the userinfo username, like Trojan
// (ParseNode stores it in node.UUID). AnyTLS always runs over TLS, so the TLS
// block is mandatory. Optional session-pool tuning maps to the sing-box
// idle_session_* / min_idle_session fields.
func buildAnyTLSOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	const scheme = "anytls"
	// Единственный кредентиал лежит в userinfo (ParseNode кладёт его в UUID).
	if node.UUID != "" {
		outbound["password"] = node.UUID
	}

	// Голое число на длительностях — секунды (форма записи, как у tuic).
	if v := queryParam(q, scheme, "idle_session_check_interval"); v != "" {
		outbound["idle_session_check_interval"] = normalizeTuicHeartbeat(v)
	}
	if v := queryParam(q, scheme, "idle_session_timeout"); v != "" {
		outbound["idle_session_timeout"] = normalizeTuicHeartbeat(v)
	}
	// Значение уезжает как есть: не-число снимет санитайзер кодом
	// anytls_min_idle_invalid (реестр, anytls.body.min_idle_session).
	if v := queryParam(q, scheme, "min_idle_session"); v != "" {
		outbound["min_idle_session"] = v
	}

	buildAnyTLSTLS(node, outbound)
}

// buildAnyTLSTLS собирает обязательный TLS-блок anytls.
//
// Форма повторяет vless: тот же дефолт отпечатка `random` у пустого fp
// (D-009) и тот же гейт блока reality по наличию pbk. Значения полей не
// судятся — этим занят санитайзер.
func buildAnyTLSTLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	const scheme = "anytls"
	tlsData := map[string]interface{}{"enabled": true}

	if sni := tlsServerNameFromQuery(q, scheme, node.Server); sni != "" {
		tlsData["server_name"] = sni
	}
	tlsData["utls"] = map[string]interface{}{
		"enabled":     true,
		"fingerprint": utlsFingerprintOrDefault(q, scheme, "random"),
	}
	if pbk := queryParam(q, scheme, "pbk"); pbk != "" {
		reality := map[string]interface{}{
			"enabled":    true,
			"public_key": pbk,
		}
		if sid := queryParam(q, scheme, "sid"); sid != "" {
			reality["short_id"] = sid
		}
		if ks := queryParam(q, scheme, "key_share"); ks != "" {
			reality["key_share"] = ks
		}
		tlsData["reality"] = reality
	}
	applyTLSQueryExtras(q, scheme, tlsData)

	outbound["tls"] = tlsData
}
