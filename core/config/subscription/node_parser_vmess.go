package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/textnorm"
)

// normalizeVMessSecurity maps subscription / JSON values to sing-box vmess outbound security.
// See: https://sing-box.sagernet.org/configuration/outbound/vmess/
func normalizeVMessSecurity(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" || s == "null" || s == "undefined" {
		return "auto"
	}
	switch s {
	case "auto", "none", "zero", "aes-128-gcm", "chacha20-poly1305", "aes-128-ctr":
		return s
	case "chacha20-ietf-poly1305":
		return "chacha20-poly1305"
	default:
		return "auto"
	}
}

func vmessStringField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			return v
		}
	}
	return ""
}

// parseVMessJSON parses VMess configuration from decoded JSON.
// VMess protocol uses base64-encoded JSON format (vmess://base64(json)) instead of
// standard URI format used by other protocols (vless://, trojan://, ssh://, etc.).
// This is why VMess requires separate parsing logic and cannot use the common
// URI parsing path that other protocols share.
func parseVMessJSON(vmessConfig map[string]interface{}, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	node := &configtypes.ParsedNode{
		Scheme: "vmess",
		Query:  make(url.Values),
	}

	var missingFields []string

	if add, ok := vmessConfig["add"].(string); ok && add != "" {
		node.Server = add
	} else {
		missingFields = append(missingFields, "add")
	}

	if port, ok := vmessConfig["port"].(float64); ok {
		node.Port = int(port)
	} else if portStr, ok := vmessConfig["port"].(string); ok {
		if p, err := strconv.Atoi(portStr); err == nil {
			node.Port = p
		} else {
			missingFields = append(missingFields, "port (invalid format)")
		}
	} else {
		missingFields = append(missingFields, "port")
	}

	if id, ok := vmessConfig["id"].(string); ok && id != "" {
		node.UUID = id
	} else {
		missingFields = append(missingFields, "id")
	}

	if len(missingFields) > 0 {
		return nil, fmt.Errorf("missing required fields: %v", missingFields)
	}

	if ps, ok := vmessConfig["ps"].(string); ok && ps != "" {
		ps = sanitizeForDisplay(ps)
		ps = textnorm.NormalizeProxyDisplay(ps)
		node.Label = ps
		node.Tag, node.Comment = extractTagAndComment(ps)
		node.Tag = normalizeFlagTag(node.Tag)
	} else {
		node.Tag = generateDefaultTag("vmess", node.Server, node.Port)
		node.Comment = node.Tag
	}

	secRaw := vmessStringField(vmessConfig, "scy", "security")
	node.Query.Set("security", normalizeVMessSecurity(secRaw))

	if aid, ok := vmessConfig["aid"].(string); ok && aid != "" && aid != "0" {
		node.Query.Set("alter_id", aid)
	} else if aidNum, ok := vmessConfig["aid"].(float64); ok && aidNum != 0 {
		node.Query.Set("alter_id", strconv.Itoa(int(aidNum)))
	}

	net := ""
	if netVal, ok := vmessConfig["net"].(string); ok && netVal != "" {
		net = netVal
		if net == "xhttp" {
			net = "ws"
		}
		node.Query.Set("network", net)
	} else {
		net = "tcp"
		node.Query.Set("network", net)
	}

	if path, ok := vmessConfig["path"].(string); ok && path != "" {
		node.Query.Set("path", path)
	}

	if host, ok := vmessConfig["host"].(string); ok && host != "" {
		node.Query.Set("host", host)
	}

	if tls, ok := vmessConfig["tls"].(string); ok && tls == "tls" {
		node.Query.Set("tls_enabled", "true")

		sni := ""
		if sniVal, ok := vmessConfig["sni"].(string); ok && sniVal != "" {
			sni = sniVal
		} else if host, ok := vmessConfig["host"].(string); ok && host != "" {
			sni = host
		} else {
			sni = node.Server
		}
		node.Query.Set("sni", sni)

		if alpn, ok := vmessConfig["alpn"].(string); ok && alpn != "" {
			node.Query.Set("alpn", alpn)
		}

		if fp, ok := vmessConfig["fp"].(string); ok && fp != "" {
			node.Query.Set("fp", fp)
		}

		if insecure, ok := vmessConfig["insecure"].(string); ok && insecure == "1" {
			node.Query.Set("insecure", "true")
		}
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Skip node
	}

	node.Outbound = buildOutbound(node)
	return node, nil
}
