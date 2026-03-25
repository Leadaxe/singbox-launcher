package subscription

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
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

// parseVMessDecoded decodes VMess after base64: standard JSON or legacy cleartext (method:uuid@host:port).
func parseVMessDecoded(decoded []byte, fragmentLabel string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	s := string(decoded)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("vmess decoded payload is empty")
	}

	var vmessConfig map[string]interface{}
	if err := json.Unmarshal(decoded, &vmessConfig); err != nil || vmessConfig == nil {
		return parseVMessLegacyCleartext(s, fragmentLabel, skipFilters)
	}
	return parseVMessJSON(vmessConfig, skipFilters)
}

// splitVMessHostPort parses host:port or [ipv6]:port after "@" in legacy VMess strings.
func splitVMessHostPort(hostport string) (host string, port int, err error) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", 0, fmt.Errorf("empty host:port")
	}
	if strings.HasPrefix(hostport, "[") {
		closeBracket := strings.Index(hostport, "]:")
		if closeBracket < 0 {
			return "", 0, fmt.Errorf("invalid bracketed host:port")
		}
		host = hostport[1:closeBracket]
		portStr := hostport[closeBracket+2:]
		p, e := strconv.Atoi(portStr)
		if e != nil {
			return "", 0, e
		}
		return host, p, nil
	}
	i := strings.LastIndex(hostport, ":")
	if i <= 0 || i == len(hostport)-1 {
		return "", 0, fmt.Errorf("missing port in host:port")
	}
	host = hostport[:i]
	portStr := hostport[i+1:]
	p, e := strconv.Atoi(portStr)
	if e != nil {
		return "", 0, e
	}
	return host, p, nil
}

func parseVMessLegacyCleartext(s, fragmentLabel string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	mainPart, rawQuery, hasQuery := strings.Cut(s, "?")
	mainPart = strings.TrimSpace(mainPart)
	var qvals url.Values
	if hasQuery {
		q, err := url.ParseQuery(strings.TrimSpace(rawQuery))
		if err != nil {
			return nil, fmt.Errorf("vmess legacy: bad query: %w", err)
		}
		qvals = q
	}

	at := strings.Index(mainPart, "@")
	if at < 0 {
		return nil, fmt.Errorf("vmess legacy: expected method:uuid@host:port")
	}
	userinfo := mainPart[:at]
	hp := mainPart[at+1:]
	parts := strings.SplitN(userinfo, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("vmess legacy: bad userinfo")
	}
	method, uuid := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if method == "" || uuid == "" {
		return nil, fmt.Errorf("vmess legacy: empty method or uuid")
	}
	host, port, err := splitVMessHostPort(hp)
	if err != nil {
		return nil, fmt.Errorf("vmess legacy: %w", err)
	}

	node := &configtypes.ParsedNode{
		Scheme: "vmess",
		Query:  make(url.Values),
		Server: host,
		Port:   port,
		UUID:   uuid,
	}
	node.Query.Set("security", normalizeVMessSecurity(method))
	for k, vs := range qvals {
		if len(vs) > 0 {
			node.Query.Set(k, vs[0])
		}
	}
	if node.Query.Get("network") == "" {
		if t := queryGetFold(node.Query, "type"); t != "" {
			node.Query.Set("network", strings.ToLower(strings.TrimSpace(t)))
		}
	}

	switch strings.ToLower(strings.TrimSpace(queryGetFold(node.Query, "tls"))) {
	case "1", "true", "tls":
		node.Query.Set("tls_enabled", "true")
	}

	label := strings.TrimSpace(fragmentLabel)
	if label != "" {
		if dec, err := url.PathUnescape(label); err == nil {
			label = dec
		}
		label = sanitizeForDisplay(label)
		label = textnorm.NormalizeProxyDisplay(label)
		node.Label = label
		node.Tag, node.Comment = extractTagAndComment(label)
		node.Tag = normalizeFlagTag(node.Tag)
	} else {
		node.Tag = generateDefaultTag("vmess", node.Server, node.Port)
		node.Comment = node.Tag
	}

	debuglog.DebugLog("Parser: VMess legacy cleartext parsed host=%s port=%d", host, port)

	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	node.Outbound = buildOutbound(node)
	return node, nil
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

	net := "tcp"
	if netVal, ok := vmessConfig["net"].(string); ok && strings.TrimSpace(netVal) != "" {
		n := strings.ToLower(strings.TrimSpace(netVal))
		switch n {
		case "xhttp", "httpupgrade":
			net = "httpupgrade"
		case "h2":
			net = "h2"
		default:
			net = n
		}
	}
	node.Query.Set("network", net)

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

	// Legacy VMess net=h2 is HTTP/2 over TLS; sing-box uses transport "http" (no separate "h2" type).
	if net == "h2" {
		if tlsStr := vmessStringField(vmessConfig, "tls"); tlsStr != "tls" {
			node.Query.Set("tls_enabled", "true")
			sni := vmessStringField(vmessConfig, "sni")
			if sni == "" {
				sni = vmessStringField(vmessConfig, "host")
			}
			if sni == "" {
				sni = node.Server
			}
			node.Query.Set("sni", sni)
			if alpn := vmessStringField(vmessConfig, "alpn"); alpn != "" {
				node.Query.Set("alpn", alpn)
			}
			if fp := vmessStringField(vmessConfig, "fp"); fp != "" {
				node.Query.Set("fp", fp)
			}
			if insecure, ok := vmessConfig["insecure"].(string); ok && insecure == "1" {
				node.Query.Set("insecure", "true")
			}
		}
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Skip node
	}

	node.Outbound = buildOutbound(node)
	return node, nil
}
