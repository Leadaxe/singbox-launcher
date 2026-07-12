package subscription

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
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
	if node.UUID != "" {
		outbound["password"] = node.UUID
	} else {
		debuglog.WarnLog("Parser: AnyTLS link missing password (userinfo). URI might be invalid.")
	}

	// Session-pool tuning (optional). Bare integers on the idle-session durations
	// are treated as seconds, matching the TUIC heartbeat convention.
	if v := strings.TrimSpace(queryGetFold(node.Query, "idle_session_check_interval")); v != "" {
		outbound["idle_session_check_interval"] = normalizeTuicHeartbeat(v)
	}
	if v := strings.TrimSpace(queryGetFold(node.Query, "idle_session_timeout")); v != "" {
		outbound["idle_session_timeout"] = normalizeTuicHeartbeat(v)
	}
	if v := strings.TrimSpace(queryGetFold(node.Query, "min_idle_session")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			outbound["min_idle_session"] = n
		} else {
			debuglog.WarnLog("Parser: AnyTLS min_idle_session %q is not a non-negative integer, dropping.", v)
		}
	}

	buildAnyTLSTLS(node, outbound)
}

// buildAnyTLSTLS builds the (mandatory) TLS block for an AnyTLS node. It mirrors
// buildTuicTLS: server_name from sni (or the server address), insecure across the
// common spellings, uTLS fingerprint, and ALPN.
func buildAnyTLSTLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	tlsData := map[string]interface{}{"enabled": true}

	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = queryGetFold(q, "peer")
	}
	if sni != "" && sni != "🔒" && (strings.Contains(sni, ".") || strings.Contains(sni, ":")) {
		tlsData["server_name"] = sni
	} else if node.Server != "" {
		tlsData["server_name"] = node.Server
	}

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

	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}

	outbound["tls"] = tlsData
}
