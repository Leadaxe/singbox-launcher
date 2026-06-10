package subscription

import (
	"net/url"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// isValidTuicCongestionControl reports whether cc is a congestion controller
// sing-box accepts for TUIC. Anything else is dropped (sing-box rejects unknown
// values at load time).
func isValidTuicCongestionControl(cc string) bool {
	switch cc {
	case "cubic", "new_reno", "bbr":
		return true
	default:
		return false
	}
}

// tuicQueryFlagTrue reports whether a boolean-ish query flag is enabled
// ("1"/"true"/"yes", case-insensitive). queryGetFold makes the key lookup
// case-insensitive but not separator-insensitive, so callers pass the exact key
// (e.g. both "allow_insecure" and "skip-cert-verify").
func tuicQueryFlagTrue(q url.Values, key string) bool {
	v := strings.ToLower(strings.TrimSpace(queryGetFold(q, key)))
	return v == "1" || v == "true" || v == "yes"
}

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
	if node.UUID != "" {
		outbound["uuid"] = node.UUID
	} else {
		debuglog.WarnLog("Parser: TUIC link missing uuid. URI might be invalid.")
	}
	if pw := node.Query.Get("password"); pw != "" {
		outbound["password"] = pw
	} else {
		debuglog.WarnLog("Parser: TUIC link missing password. URI might be invalid.")
	}

	// congestion_control (optional): cubic | new_reno | bbr
	if cc := strings.ToLower(strings.TrimSpace(queryGetFold(node.Query, "congestion_control"))); cc != "" {
		if isValidTuicCongestionControl(cc) {
			outbound["congestion_control"] = cc
		} else {
			debuglog.WarnLog("Parser: unsupported TUIC congestion_control %q (want cubic/new_reno/bbr), dropping.", cc)
		}
	}

	// udp_relay_mode (optional): native | quic
	if urm := strings.ToLower(strings.TrimSpace(queryGetFold(node.Query, "udp_relay_mode"))); urm != "" {
		if urm == "native" || urm == "quic" {
			outbound["udp_relay_mode"] = urm
		} else {
			debuglog.WarnLog("Parser: unsupported TUIC udp_relay_mode %q (want native/quic), dropping.", urm)
		}
	}

	// zero-RTT handshake: accept the sing-box key and the common reduce_rtt alias.
	if tuicQueryFlagTrue(node.Query, "zero_rtt_handshake") || tuicQueryFlagTrue(node.Query, "reduce_rtt") {
		outbound["zero_rtt_handshake"] = true
	}

	// heartbeat (optional): duration; bare integers are treated as seconds.
	if hb := strings.TrimSpace(queryGetFold(node.Query, "heartbeat")); hb != "" {
		outbound["heartbeat"] = normalizeTuicHeartbeat(hb)
	}

	buildTuicTLS(node, outbound)
}

// buildTuicTLS builds the (mandatory) TLS block for a TUIC node.
func buildTuicTLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	tlsData := map[string]interface{}{
		"enabled": true,
	}

	// Set SNI if provided and looks like a hostname/IP (skip emoji/invalid),
	// otherwise fall back to the server address.
	sni := queryGetFold(q, "sni")
	if sni != "" && sni != "🔒" && (strings.Contains(sni, ".") || strings.Contains(sni, ":")) {
		tlsData["server_name"] = sni
	} else if node.Server != "" {
		tlsData["server_name"] = node.Server
	}

	// insecure: tlsInsecureTrue covers insecure/allowInsecure; TUIC subscriptions
	// commonly use the snake_case allow_insecure / skip-cert-verify spellings,
	// which queryGetFold does NOT fold to those keys — check them explicitly.
	if tlsInsecureTrue(q) ||
		tuicQueryFlagTrue(q, "allow_insecure") ||
		tuicQueryFlagTrue(q, "skip-cert-verify") ||
		tuicQueryFlagTrue(q, "skipCertVerify") {
		tlsData["insecure"] = true
	}

	fp := NormalizeUTLSFingerprint(queryGetFold(q, "fp"))
	if fp == "" {
		fp = NormalizeUTLSFingerprint(queryGetFold(q, "fingerprint"))
	}
	if fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}

	// ALPN (TUIC default is ["h3"]; subscriptions often pass e.g. "h3,spdy/3.1").
	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}

	outbound["tls"] = tlsData
}
