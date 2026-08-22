package subscription

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// isValidHysteria2ObfsType checks if the obfs type is supported by the core.
// sing-box-lx implements both salamander and gecko (protocol/hysteria2/outbound.go);
// LxBox already accepts gecko, so rejecting it here silently stripped working
// obfuscation from desktop nodes (SPEC 103, D-016(а)).
func isValidHysteria2ObfsType(obfsType string) bool {
	return obfsType == "salamander" || obfsType == "gecko"
}

// buildHysteria2Outbound builds outbound configuration for Hysteria2 protocol
func buildHysteria2Outbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	// Password is required (stored in UUID field from userinfo)
	if node.UUID != "" {
		outbound["password"] = node.UUID
	} else {
		debuglog.WarnLog("Parser: Hysteria2 link missing password. URI might be invalid.")
	}

	// Optional: mport / ports query — Hysteria2 multi-port (comma-separated ports and ranges, hyphen in URI).
	// See https://v2.hysteria.network/docs/advanced/Port-Hopping/
	mport := strings.TrimSpace(queryGetFold(node.Query, "mport"))
	if mport == "" {
		mport = strings.TrimSpace(queryGetFold(node.Query, "ports"))
	}
	if sp := hysteria2MportSpecToSingBoxServerPorts(mport); len(sp) > 0 {
		outbound["server_ports"] = sp
	}

	// Optional: obfs (obfuscation)
	if obfs := node.Query.Get("obfs"); obfs != "" {
		obfsPassword := node.Query.Get("obfs-password")
		switch {
		case !isValidHysteria2ObfsType(obfs):
			node.AddWarning(WarnObfsUnknown)
			debuglog.WarnLog("Parser: Invalid or unsupported Hysteria2 obfs type '%s'. Supported: salamander, gecko. Skipping obfs.", obfs)
		case obfsPassword == "":
			node.AddWarning(WarnObfsPasswordMissing)
			// The core refuses to start a node whose obfs block has no password
			// ("missing obfs password") and that kills the whole config, so drop
			// the obfs block instead of the config (SPEC 103, D-016(г)).
			debuglog.WarnLog("Parser: Hysteria2 obfs '%s' has no obfs-password — dropping obfs (the core would reject the whole config).", obfs)
		default:
			obfsConfig := map[string]interface{}{
				"type":     obfs,
				"password": obfsPassword,
			}
			// Gecko carries optional packet-size bounds flat inside obfs
			// (option/hysteria2.go, Hysteria2ObfsGecko); same spelling as LxBox.
			if obfs == "gecko" {
				if v := strings.TrimSpace(queryGetFold(node.Query, "obfs-min-packet-size")); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						obfsConfig["min_packet_size"] = n
					}
				}
				if v := strings.TrimSpace(queryGetFold(node.Query, "obfs-max-packet-size")); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						obfsConfig["max_packet_size"] = n
					}
				}
			}
			outbound["obfs"] = obfsConfig
		}
	}

	// Optional: bandwidth (up/down in Mbps)
	if up := node.Query.Get("upmbps"); up != "" {
		if upMBps, err := strconv.Atoi(up); err == nil {
			outbound["up_mbps"] = upMBps
		}
	}
	if down := node.Query.Get("downmbps"); down != "" {
		if downMBps, err := strconv.Atoi(down); err == nil {
			outbound["down_mbps"] = downMBps
		}
	}

	// TLS settings (required for hysteria2)
	buildHysteria2TLS(node, outbound)
}

// buildHysteria2TLS builds TLS configuration for Hysteria2
func buildHysteria2TLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	sni := queryGetFold(q, "sni")

	// Always enable TLS for hysteria2 (required by protocol)
	tlsData := map[string]interface{}{
		"enabled": true,
	}

	// Set SNI if provided and valid (skip emoji or invalid values)
	// SNI is valid if it contains dot (hostname) or colon (IPv6)
	if sni != "" && sni != "🔒" && (strings.Contains(sni, ".") || strings.Contains(sni, ":")) {
		tlsData["server_name"] = sni
	} else if node.Server != "" {
		tlsData["server_name"] = node.Server
	}

	if tlsInsecureTrue(q) {
		tlsData["insecure"] = true
	} else if queryGetFold(q, "skip-cert-verify") == "true" || queryGetFold(q, "skip-cert-verify") == "1" {
		tlsData["insecure"] = true
	}

	// No utls on QUIC: hysteria2 runs over QUIC, which doesn't use uTLS
	// ClientHello fingerprints at all — `fp=` here is subscription noise. The
	// Xray-JSON path already stripped it; the URI path used to keep it, so the
	// same node differed between paths and between desktop and mobile
	// (SPEC 103, D-033).

	if pin := strings.TrimSpace(queryGetFold(q, "pinSHA256")); pin != "" {
		tlsData["certificate_public_key_sha256"] = []string{pin}
	}

	// Handle ALPN parameter (for hysteria2, typically "h3")
	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}

	outbound["tls"] = tlsData
}
