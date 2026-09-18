package subscription

import (
	"singbox-launcher/core/config/configtypes"
)

// normalizeTuicHeartbeat turns a bare integer (seconds, which many TUIC clients
// emit) into a sing-box duration string ("10" → "10s"); a value already carrying
// a unit (e.g. "10s") is passed through unchanged.
func normalizeTuicHeartbeat(v string) string {
	if v == "" {
		return v
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return v // already has a unit/suffix
		}
	}
	return v + "s"
}

// buildTuicOutbound builds the outbound map for a TUIC v5 node.
//
// URI shape (de-facto client format, e.g. v2rayN / Nekobox):
//
//	tuic://uuid:password@host:port?congestion_control=&alpn=&udp_relay_mode=&allow_insecure=&sni=#name
//
// uuid is the userinfo username (node.UUID); password is the userinfo password,
// which ParseNode places into Query["password"]. TUIC always runs over QUIC, so
// a TLS block is mandatory.
func buildTuicOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	const scheme = "tuic"
	if node.UUID != "" {
		outbound["uuid"] = node.UUID
	}
	if pw := queryParamRaw(q, scheme, "password"); pw != "" {
		outbound["password"] = pw
	}

	// Значения enum'ов уезжают как есть: мусор снимет санитайзер кодами
	// tuic_congestion_invalid / tuic_udp_relay_mode_invalid, а дефолты ядра
	// (cubic, native) не материализуются (CANON §2.4, DRIFT §7.7).
	if cc := queryParam(q, scheme, "congestion_control"); cc != "" {
		outbound["congestion_control"] = cc
	}
	if urm := queryParam(q, scheme, "udp_relay_mode"); urm != "" {
		outbound["udp_relay_mode"] = urm
	}

	// Три написания одного флага (zero_rtt_handshake / reduce_rtt / zero_rtt)
	// объявлены в реестре как алиасы.
	if queryFlagTrue(q, scheme, "reduce_rtt") {
		outbound["zero_rtt_handshake"] = true
	}

	// heartbeat: голое число в ссылке — секунды, у ядра это duration-строка
	// (перевод формы записи, не решение о значении).
	if hb := queryParam(q, scheme, "heartbeat"); hb != "" {
		outbound["heartbeat"] = normalizeTuicHeartbeat(hb)
	}

	buildTuicTLS(node, outbound)
}

// buildTuicTLS builds the (mandatory) TLS block for a TUIC node.
func buildTuicTLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	const scheme = "tuic"
	tlsData := map[string]interface{}{"enabled": true}

	if sni := tlsServerNameFromQuery(q, scheme, node.Server); sni != "" {
		tlsData["server_name"] = sni
	}
	// ALPN и insecure (девять написаний) — общие: дефолт ["h3"] НЕ пишем,
	// это дефолт ядра (DRIFT §7.7).
	applyTLSQueryExtras(q, scheme, tlsData)
	// `fp` переводится в тело как есть и снимается реестром с кодом
	// tls_not_applicable_quic: ядро отпечаток поверх QUIC не применяет, но
	// молчать об этом (как было до SPEC 131) значит не сказать пользователю,
	// что параметр ссылки не сработал.
	applyTLSCamouflageFromQuery(q, scheme, tlsData)

	outbound["tls"] = tlsData
}
