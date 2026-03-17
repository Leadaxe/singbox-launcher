// Package subscription provides parsing logic for various proxy node formats.
// It supports VLESS, VMess, Trojan, Shadowsocks, Hysteria2, and SSH protocols, handling
// both direct links and subscription formats.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
)

// IsDirectLink checks if the input string is a direct proxy link (vless://, vmess://, wireguard://, etc.)
func IsDirectLink(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "vless://") ||
		strings.HasPrefix(trimmed, "vmess://") ||
		strings.HasPrefix(trimmed, "trojan://") ||
		strings.HasPrefix(trimmed, "ss://") ||
		strings.HasPrefix(trimmed, "hysteria2://") ||
		strings.HasPrefix(trimmed, "hy2://") ||
		strings.HasPrefix(trimmed, "ssh://") ||
		strings.HasPrefix(trimmed, "wireguard://")
}

// MaxURILength defines the maximum allowed length for a proxy URI
const MaxURILength = 8192 // 8 KB - reasonable limit for proxy URIs

// ParseNode parses a single node URI and applies skip filters
func ParseNode(uri string, skipFilters []map[string]string) (*config.ParsedNode, error) {
	// Validate URI length
	if len(uri) > MaxURILength {
		return nil, fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), MaxURILength)
	}

	// Determine scheme
	scheme := ""
	uriToParse := uri
	defaultPort := 443              // Default port for most protocols
	var ssMethod, ssPassword string // For SS links: method and password extracted from base64

	// Determine scheme and handle protocol-specific parsing
	switch {
	case strings.HasPrefix(uri, "vmess://"):
		base64Part := strings.TrimPrefix(uri, "vmess://")
		decoded, err := decodeBase64WithPadding(base64Part)
		if err != nil {
			uriPreview := uri
			if len(uriPreview) > 50 {
				uriPreview = uriPreview[:50] + "..."
			}
			debuglog.ErrorLog("Parser: Failed to decode VMESS base64 (uri length: %d, base64 length: %d): %v. URI: %s. Skipping node.",
				len(uri), len(base64Part), err, uriPreview)
			return nil, fmt.Errorf("failed to decode VMESS base64: %w", err)
		}
		if len(decoded) == 0 {
			debuglog.ErrorLog("Parser: VMESS decoded content is empty. Skipping node.")
			return nil, fmt.Errorf("VMESS decoded content is empty")
		}
		var vmessConfig map[string]interface{}
		if err := json.Unmarshal(decoded, &vmessConfig); err != nil {
			debuglog.ErrorLog("Parser: Failed to parse VMESS JSON (decoded length: %d): %v. Skipping node.", len(decoded), err)
			return nil, fmt.Errorf("failed to parse VMESS JSON: %w", err)
		}
		// VMess uses base64-encoded JSON format instead of standard URI format,
		// so it requires separate parsing logic and cannot use the common URI parser.
		// All other protocols (VLESS, Trojan, SS, Hysteria2, SSH) use standard URI format
		// and are processed through the common parsing path below.
		return parseVMessJSON(vmessConfig, skipFilters)

	case strings.HasPrefix(uri, "vless://"):
		scheme = "vless"

	case strings.HasPrefix(uri, "trojan://"):
		scheme = "trojan"

	case strings.HasPrefix(uri, "ss://"):
		scheme = "ss"
		ssPart := strings.TrimPrefix(uri, "ss://")
		if atIdx := strings.Index(ssPart, "@"); atIdx > 0 {
			encodedUserinfo := ssPart[:atIdx]
			rest := ssPart[atIdx+1:]
			decoded, err := decodeBase64WithPadding(encodedUserinfo)
			if err != nil {
				debuglog.ErrorLog("Parser: Failed to decode SS base64 userinfo. Encoded: %s, Error: %v", encodedUserinfo, err)
			} else {
				decodedStr := string(decoded)
				userinfoParts := strings.SplitN(decodedStr, ":", 2)
				if len(userinfoParts) == 2 {
					ssMethod = userinfoParts[0]
					ssPassword = userinfoParts[1]
					debuglog.DebugLog("Parser: Successfully extracted SS credentials: method=%s, password length=%d", ssMethod, len(ssPassword))
					if !isValidShadowsocksMethod(ssMethod) {
						debuglog.WarnLog("Parser: Invalid or unsupported Shadowsocks method '%s'. Skipping node.", ssMethod)
						return nil, fmt.Errorf("unsupported Shadowsocks encryption method: %s", ssMethod)
					}
				} else {
					debuglog.ErrorLog("Parser: SS decoded userinfo doesn't contain ':' separator. Decoded: %s", decodedStr)
				}
			}
			uriToParse = "ss://" + rest
		} else {
			debuglog.WarnLog("Parser: SS link is not in SIP002 format (no @ found): %s", uri)
		}

	case strings.HasPrefix(uri, "hysteria2://"), strings.HasPrefix(uri, "hy2://"):
		scheme = "hysteria2"
		// Handle both hysteria2:// and hy2:// schemes (hy2 is official short form)
		// Normalize to hysteria2:// for parsing
		uriToParse = uri
		var base64Part string
		if strings.HasPrefix(uri, "hy2://") {
			base64Part = strings.TrimPrefix(uri, "hy2://")
			uriToParse = strings.Replace(uri, "hy2://", "hysteria2://", 1)
		} else {
			base64Part = strings.TrimPrefix(uri, "hysteria2://")
		}

		// Try to decode base64 (some Hysteria2 links are base64-encoded)
		decoded, err := decodeBase64WithPadding(base64Part)
		if err == nil && len(decoded) > 0 {
			decodedStr, valid := validateAndFixUTF8Bytes(decoded)
			if !valid {
				debuglog.ErrorLog("Parser: Decoded base64 contains invalid UTF-8 that cannot be fixed. Skipping node.")
				return nil, fmt.Errorf("decoded base64 contains invalid UTF-8")
			}
			if decodedStr != string(decoded) {
				debuglog.DebugLog("Parser: Fixed invalid UTF-8 in decoded base64 Hysteria2 link")
			}
			if strings.Contains(decodedStr, "@") {
				uriToParse = "hysteria2://" + decodedStr
				debuglog.DebugLog("Parser: Successfully decoded base64 Hysteria2 link")
			}
		}

	case strings.HasPrefix(uri, "ssh://"):
		scheme = "ssh"
		defaultPort = 22 // Default port for SSH

	case strings.HasPrefix(uri, "wireguard://"):
		return parseWireGuardURI(uri, skipFilters)

	default:
		return nil, fmt.Errorf("unsupported scheme")
	}

	// Parse URI
	parsedURL, err := url.Parse(uriToParse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// Validate VLESS/Trojan/SSH URI format (must have hostname and userinfo)
	if scheme == "vless" || scheme == "trojan" || scheme == "ssh" {
		if parsedURL.Hostname() == "" {
			return nil, fmt.Errorf("invalid %s URI: missing hostname", scheme)
		}
		if parsedURL.User == nil || parsedURL.User.Username() == "" {
			return nil, fmt.Errorf("invalid %s URI: missing userinfo (UUID/password/user)", scheme)
		}
	}

	// Extract components
	node := &config.ParsedNode{
		Scheme: scheme,
		Server: parsedURL.Hostname(),
		Query:  parsedURL.Query(),
	}

	// For SS, store method and password in Query (if extracted during parsing)
	if scheme == "ss" {
		if ssMethod == "" || ssPassword == "" {
			debuglog.ErrorLog("Parser: SS link missing method or password. URI: %s", uri)
			return nil, fmt.Errorf("SS link missing required method or password")
		}
		node.Query.Set("method", ssMethod)
		node.Query.Set("password", ssPassword)
	}

	// Extract port (defaultPort was set in scheme detection)
	node.Port = defaultPort
	if port := parsedURL.Port(); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			node.Port = p
		}
	}

	// Extract UUID/user
	// For hysteria2, password is in username part of userinfo (hysteria2://password@server:port)
	// For SSH and Trojan, password can be in userinfo (user:password@server:port)
	if parsedURL.User != nil {
		node.UUID = parsedURL.User.Username()
		// URL decode the username (password) if it contains encoded characters
		if decoded, err := url.QueryUnescape(node.UUID); err == nil && decoded != node.UUID {
			node.UUID = decoded
		}
		// Extract password for SSH and Trojan (user:password@server)
		if scheme == "ssh" || scheme == "trojan" {
			if password, hasPassword := parsedURL.User.Password(); hasPassword {
				if decodedPassword, err := url.QueryUnescape(password); err == nil {
					node.Query.Set("password", decodedPassword)
				} else {
					node.Query.Set("password", password)
				}
			}
		}
	}

	// Extract fragment (label)
	node.Label = parsedURL.Fragment
	// URL decode and validate UTF-8
	if node.Label != "" {
		if decoded, err := url.QueryUnescape(node.Label); err == nil {
			node.Label = decoded
		}

		// Validate and fix UTF-8 encoding
		fixed, valid := validateAndFixUTF8(node.Label)
		if !valid {
			debuglog.ErrorLog("Parser: Fragment contains invalid UTF-8 that cannot be fixed: %q. Skipping node.", parsedURL.Fragment)
			return nil, fmt.Errorf("fragment contains invalid UTF-8: %q", parsedURL.Fragment)
		}

		if fixed != node.Label {
			debuglog.DebugLog("Parser: Fixed invalid UTF-8 in fragment: %q -> %q", parsedURL.Fragment, fixed)
			node.Label = fixed
		}

		// Sanitize control characters (NUL and other C0 controls) which may
		// cause GUI toolkits or serializers to misbehave. Remove runes < 0x20
		// (except common whitespace) and delete DEL (0x7F).
		node.Label = sanitizeForDisplay(node.Label)
	}

	// For some formats, label might be in path or userinfo
	if node.Label == "" {
		// Try to extract from path (some formats use path for label)
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			node.Label = strings.TrimPrefix(parsedURL.Path, "/")
		} else if parsedURL.User != nil && scheme != "hysteria2" {
			// Some formats encode label in username (but not for hysteria2, where it's the password)
			node.Label = parsedURL.User.Username()
		}
	}

	// Extract tag and comment from label
	node.Tag, node.Comment = extractTagAndComment(node.Label)

	// Generate tag if missing
	if node.Tag == "" {
		node.Tag = generateDefaultTag(scheme, node.Server, node.Port)
		node.Comment = node.Tag
	}

	// Normalize flag
	node.Tag = normalizeFlagTag(node.Tag)

	// Extract flow
	node.Flow = parsedURL.Query().Get("flow")

	// Apply skip filters
	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Node should be skipped
	}

	// Build outbound JSON based on scheme
	node.Outbound = buildOutbound(node)

	return node, nil
}

// Private helper functions (migrated from parser.go)

// decodeBase64WithPadding attempts to decode base64 string with automatic padding
// Uses the shared tryDecodeBase64 function from core package
// Note: This creates a dependency on core package, but we can't import it due to circular dependency
// So we keep a local implementation that matches the logic
func decodeBase64WithPadding(s string) ([]byte, error) {
	// Try URL-safe base64 without padding first (most common)
	if decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, nil
	}

	// Try standard base64 without padding
	if decoded, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, nil
	}

	// Try URL-safe base64 with padding
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}

	// Try standard base64 with padding
	return base64.StdEncoding.DecodeString(s)
}

// isValidShadowsocksMethod checks if the encryption method is supported by sing-box
// This prevents invalid methods (like binary data) from causing sing-box to crash
// Only methods supported by sing-box are allowed (see sing-box documentation)
func isValidShadowsocksMethod(method string) bool {
	validMethods := map[string]bool{
		// 2022 edition (modern, best security)
		"2022-blake3-aes-128-gcm":       true,
		"2022-blake3-aes-256-gcm":       true,
		"2022-blake3-chacha20-poly1305": true,
		// AEAD ciphers
		"none":                    true,
		"aes-128-gcm":             true,
		"aes-192-gcm":             true,
		"aes-256-gcm":             true,
		"chacha20-ietf-poly1305":  true,
		"xchacha20-ietf-poly1305": true,
	}
	return validMethods[method]
}

// isValidHysteria2ObfsType checks if the obfs type is supported by sing-box for Hysteria2
// According to sing-box documentation, only "salamander" is supported
func isValidHysteria2ObfsType(obfsType string) bool {
	return obfsType == "salamander"
}

// validateAndFixUTF8 validates and fixes invalid UTF-8 in a string
// Returns fixed string and true if valid, or original string and false if unfixable
func validateAndFixUTF8(s string) (string, bool) {
	if utf8.ValidString(s) {
		return s, true
	}
	fixed := strings.ToValidUTF8(s, "")
	if utf8.ValidString(fixed) {
		return fixed, true
	}
	return s, false
}

// validateAndFixUTF8Bytes validates and fixes invalid UTF-8 in bytes
// Returns fixed string and true if valid, or empty string and false if unfixable
func validateAndFixUTF8Bytes(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), true
	}
	fixed := strings.ToValidUTF8(string(b), "")
	if utf8.ValidString(fixed) {
		return fixed, true
	}
	return "", false
}

// sanitizeForDisplay removes control characters that are unsafe for UI
// and other consumers (notably NUL). It removes runes in the C0 control
// range (U+0000..U+001F) and DEL (U+007F). Keeps common whitespace
// characters (tab, newline, carriage return) if present.
func sanitizeForDisplay(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		// Keep tab/newline/carriage return
		if r == '\t' || r == '\n' || r == '\r' {
			b.WriteRune(r)
			continue
		}
		// Skip C0 controls and DEL
		if r >= 0 && r <= 0x1F {
			continue
		}
		if r == 0x7F {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func extractTagAndComment(label string) (tag, comment string) {
	tag = strings.TrimSpace(label)

	// Comment is the part after | separator
	if idx := strings.Index(label, "|"); idx >= 0 {
		comment = strings.TrimSpace(label[idx+1:])
	} else {
		comment = tag // If no |, use full label as comment
	}
	return tag, comment
}

func normalizeFlagTag(tag string) string {
	return strings.ReplaceAll(tag, "🇪🇳", "🇬🇧")
}

// generateDefaultTag generates a default tag for a node when tag is missing
func generateDefaultTag(scheme, server string, port int) string {
	return fmt.Sprintf("%s-%s-%d", scheme, server, port)
}

// getNodeValue extracts a value from node by key (supports nested keys with dots)
func getNodeValue(node *config.ParsedNode, key string) string {
	switch key {
	case "tag":
		return node.Tag
	case "host":
		return node.Server
	case "label":
		return node.Label
	case "scheme":
		return node.Scheme
	case "fragment":
		return node.Label // fragment == label
	case "comment":
		return node.Comment
	case "flow":
		return node.Flow
	default:
		return ""
	}
}

// matchesPattern checks if a value matches a pattern (supports regex and negation)
func matchesPattern(value, pattern string) bool {
	// Negation literal: !literal
	if strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!/") {
		literal := strings.TrimPrefix(pattern, "!")
		return value != literal
	}

	// Negation regex: !/regex/i
	if strings.HasPrefix(pattern, "!/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "!/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return !re.MatchString(value)
	}

	// Regex: /regex/i
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			debuglog.WarnLog("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return re.MatchString(value)
	}

	// Literal match (case-sensitive)
	return value == pattern
}

func shouldSkipNode(node *config.ParsedNode, skipFilters []map[string]string) bool {
	for _, filter := range skipFilters {
		allKeysMatch := true
		for key, pattern := range filter {
			value := getNodeValue(node, key)
			if !matchesPattern(value, pattern) {
				allKeysMatch = false
				break
			}
		}
		if allKeysMatch {
			return true // Skip node
		}
	}
	return false // Don't skip
}

// parseWireGuardURI parses wireguard:// URI into ParsedNode with sing-box endpoint in Outbound.
// Format: wireguard://<PRIVATE_KEY>@<SERVER_IP>:<PORT>?publickey=...&address=...&allowedips=...
// Required query: publickey, address, allowedips. Optional: mtu, keepalive, presharedkey, listenport, name, dns.
func parseWireGuardURI(uri string, skipFilters []map[string]string) (*config.ParsedNode, error) {
	debuglog.DebugLog("parseWireGuardURI: start")
	if len(uri) > MaxURILength {
		debuglog.DebugLog("parseWireGuardURI: error URI length exceeded")
		return nil, fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), MaxURILength)
	}
	// Extract fragment from raw URI; url.Parse may not set Fragment for non-standard schemes.
	fragmentFromRaw := ""
	if i := strings.LastIndex(uri, "#"); i >= 0 {
		fragmentFromRaw = strings.TrimSpace(uri[i+1:])
	}
	parsedURL, err := url.Parse(uri)
	if err != nil {
		debuglog.DebugLog("parseWireGuardURI: error parse URL: %v", err)
		return nil, fmt.Errorf("failed to parse wireguard URI: %w", err)
	}
	if parsedURL.Hostname() == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing hostname")
		return nil, fmt.Errorf("invalid wireguard URI: missing hostname")
	}
	if parsedURL.User == nil || parsedURL.User.Username() == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing private key (userinfo)")
		return nil, fmt.Errorf("invalid wireguard URI: missing private key (userinfo)")
	}
	// Use PathUnescape so + in base64 is preserved (QueryUnescape would turn + into space and break the key)
	privateKey, err := url.PathUnescape(parsedURL.User.Username())
	if err != nil {
		privateKey = parsedURL.User.Username()
	}
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return nil, fmt.Errorf("invalid wireguard URI: empty private key")
	}
	// Validate base64 private key (optional but recommended)
	if _, err := base64.StdEncoding.DecodeString(privateKey); err != nil {
		if _, err2 := base64.URLEncoding.DecodeString(privateKey); err2 != nil {
			debuglog.DebugLog("parseWireGuardURI: warning private key may not be valid base64")
		}
	}

	port := 51820
	if p := parsedURL.Port(); p != "" {
		if pi, err := strconv.Atoi(p); err == nil {
			port = pi
		}
	}

	q := parsedURL.Query()
	// Preserve + in base64 (query parser would decode + as space)
	publicKey := queryParamPreservePlus(parsedURL, "publickey")
	if publicKey == "" {
		publicKey = q.Get("publickey")
	}
	addressParam := q.Get("address")
	allowedipsParam := q.Get("allowedips")
	if publicKey == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing publickey")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter publickey")
	}
	if addressParam == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing address")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter address")
	}
	if allowedipsParam == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing allowedips")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter allowedips")
	}

	addressDecoded, _ := url.QueryUnescape(addressParam)
	allowedipsDecoded, _ := url.QueryUnescape(allowedipsParam)
	addressList := splitAndTrim(addressDecoded, ",")
	allowedipsList := splitAndTrim(allowedipsDecoded, ",")
	if len(addressList) == 0 || len(allowedipsList) == 0 {
		return nil, fmt.Errorf("invalid wireguard URI: address or allowedips empty after parse")
	}

	mtu := 1420
	if m := q.Get("mtu"); m != "" {
		if mi, err := strconv.Atoi(m); err == nil {
			mtu = mi
		}
	}
	listenport := 0
	if lp := q.Get("listenport"); lp != "" {
		if lpi, err := strconv.Atoi(lp); err == nil {
			listenport = lpi
		}
	}
	name := q.Get("name")
	if name == "" {
		name = "singbox-wg0"
	}
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}

	peer := map[string]interface{}{
		"address":      parsedURL.Hostname(),
		"port":         port,
		"public_key":   publicKey,
		"allowed_ips":  allowedipsList,
	}
	if keepalive := q.Get("keepalive"); keepalive != "" {
		if ki, err := strconv.Atoi(keepalive); err == nil {
			peer["persistent_keepalive_interval"] = ki
		}
	}
	if psk := queryParamPreservePlus(parsedURL, "presharedkey"); psk != "" {
		peer["pre_shared_key"] = psk
	} else if psk := q.Get("presharedkey"); psk != "" {
		peer["pre_shared_key"] = psk
	}

	endpoint := map[string]interface{}{
		"type":        "wireguard",
		"tag":         "", // set below after tag is computed
		"name":        name,
		"system":      false,
		"mtu":         mtu,
		"address":     addressList,
		"private_key": privateKey,
		"peers":       []map[string]interface{}{peer},
	}
	if listenport != 0 {
		endpoint["listen_port"] = listenport
	}

	label := parsedURL.Fragment
	if label == "" && fragmentFromRaw != "" {
		label = fragmentFromRaw
	}
	if label == "" {
		label = name
	}
	if decoded, err := url.QueryUnescape(label); err == nil {
		label = decoded
	}
	label = sanitizeForDisplay(label)
	tag, comment := extractTagAndComment(label)
	if tag == "" {
		tag = generateDefaultTag("wireguard", parsedURL.Hostname(), port)
		comment = tag
	}
	tag = normalizeFlagTag(tag)
	endpoint["tag"] = tag

	node := &config.ParsedNode{
		Scheme:   "wireguard",
		Tag:      tag,
		Server:   parsedURL.Hostname(),
		Port:     port,
		Label:    label,
		Comment:  comment,
		Query:    q,
		Outbound: endpoint,
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	debuglog.DebugLog("parseWireGuardURI: success tag=%s", node.Tag)
	return node, nil
}

// queryParamPreservePlus returns the first value for key in u.RawQuery, decoded with PathUnescape.
// This preserves '+' in base64 (QueryUnescape decodes '+' as space and would break keys).
func queryParamPreservePlus(u *url.URL, key string) string {
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if i := strings.Index(pair, "="); i >= 0 {
			k := strings.TrimSpace(pair[:i])
			if k != key {
				continue
			}
			val := pair[i+1:]
			if d, err := url.PathUnescape(val); err == nil {
				return d
			}
			return val
		}
	}
	return ""
}

func splitAndTrim(s string, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildOutbound(node *config.ParsedNode) map[string]interface{} {
	outbound := make(map[string]interface{})
	outbound["tag"] = node.Tag
	// Use "shadowsocks" instead of "ss" for sing-box
	if node.Scheme == "ss" {
		outbound["type"] = "shadowsocks"
	} else {
		outbound["type"] = node.Scheme
	}
	outbound["server"] = node.Server
	outbound["server_port"] = node.Port

	if node.Scheme == "vless" {
		outbound["uuid"] = node.UUID
		if node.Flow != "" {
			// Convert xtls-rprx-vision-udp443 to compatible format
			if node.Flow == "xtls-rprx-vision-udp443" {
				outbound["flow"] = "xtls-rprx-vision"
				outbound["packet_encoding"] = "xudp"
				outbound["server_port"] = 443
			} else {
				outbound["flow"] = node.Flow
			}
		}

		// Build TLS structure with correct field order
		sni := node.Query.Get("sni")
		if sni == "" {
			sni = node.Server // Fallback to server hostname
		}
		fp := node.Query.Get("fp")
		if fp == "" {
			fp = "random"
		}
		pbk := node.Query.Get("pbk")
		sid := node.Query.Get("sid")

		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
		}

		if pbk != "" {
			tlsData["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": pbk,
				"short_id":   sid,
			}
		}

		outbound["tls"] = tlsData
	} else if node.Scheme == "vmess" {
		outbound["uuid"] = node.UUID

		if security := node.Query.Get("security"); security != "" {
			outbound["security"] = security
		} else {
			outbound["security"] = "auto" // default
		}

		if alterIDStr := node.Query.Get("alter_id"); alterIDStr != "" {
			if alterID, err := strconv.Atoi(alterIDStr); err == nil {
				outbound["alter_id"] = alterID
			}
		}

		network := node.Query.Get("network")
		if network == "" {
			network = "tcp" // default
		}

		if network == "ws" || network == "http" || network == "grpc" {
			transport := make(map[string]interface{})
			transport["type"] = network

			if path := node.Query.Get("path"); path != "" {
				transport["path"] = path
			}

			if network == "ws" || network == "http" {
				if host := node.Query.Get("host"); host != "" {
					headers := map[string]string{"Host": host}
					transport["headers"] = headers
				}
			}

			outbound["transport"] = transport
		}

		if node.Query.Get("tls_enabled") == "true" {
			tlsData := map[string]interface{}{
				"enabled": true,
			}

			if sni := node.Query.Get("sni"); sni != "" {
				tlsData["server_name"] = sni
			}

			if alpn := node.Query.Get("alpn"); alpn != "" {
				alpnList := strings.Split(alpn, ",")
				for i := range alpnList {
					alpnList[i] = strings.TrimSpace(alpnList[i])
				}
				tlsData["alpn"] = alpnList
			}

			if fp := node.Query.Get("fp"); fp != "" {
				tlsData["utls"] = map[string]interface{}{
					"enabled":     true,
					"fingerprint": fp,
				}
			}

			if node.Query.Get("insecure") == "true" {
				tlsData["insecure"] = true
			}

			outbound["tls"] = tlsData
		}
	} else if node.Scheme == "trojan" {
		outbound["password"] = node.UUID
	} else if node.Scheme == "ss" {
		if method := node.Query.Get("method"); method != "" {
			outbound["method"] = method
		}
		if password := node.Query.Get("password"); password != "" {
			outbound["password"] = password
		}
	} else if node.Scheme == "hysteria2" {
		buildHysteria2Outbound(node, outbound)
	} else if node.Scheme == "ssh" {
		buildSSHOutbound(node, outbound)
	}

	return outbound
}

// buildHysteria2Outbound builds outbound configuration for Hysteria2 protocol
func buildHysteria2Outbound(node *config.ParsedNode, outbound map[string]interface{}) {
	// Password is required (stored in UUID field from userinfo)
	if node.UUID != "" {
		outbound["password"] = node.UUID
	} else {
		debuglog.WarnLog("Parser: Hysteria2 link missing password. URI might be invalid.")
	}

	// Optional: ports range (mport parameter) - converted to server_ports array for sing-box 1.9+
	// Format: "27200-28000" or "27200:28000" -> ["27200:28000"]
	if mport := node.Query.Get("mport"); mport != "" {
		// Convert mport format (can be "27200-28000" or "27200:28000") to sing-box format
		// sing-box expects array of port ranges in format "start:end"
		portRange := strings.ReplaceAll(mport, "-", ":")
		serverPorts := []string{portRange}
		outbound["server_ports"] = serverPorts
		// hop_interval is optional, default is "30s" in sing-box, so we can omit it
		// or set it explicitly if needed in the future
	}

	// Optional: obfs (obfuscation)
	if obfs := node.Query.Get("obfs"); obfs != "" {
		// Validate obfs type to prevent sing-box crashes
		if !isValidHysteria2ObfsType(obfs) {
			debuglog.WarnLog("Parser: Invalid or unsupported Hysteria2 obfs type '%s'. Only 'salamander' is supported. Skipping obfs.", obfs)
		} else {
			obfsConfig := map[string]interface{}{
				"type": obfs,
			}
			if obfsPassword := node.Query.Get("obfs-password"); obfsPassword != "" {
				obfsConfig["password"] = obfsPassword
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
func buildHysteria2TLS(node *config.ParsedNode, outbound map[string]interface{}) {
	sni := node.Query.Get("sni")

	// Handle insecure parameter (can be "1" or "true")
	insecure := node.Query.Get("insecure") == "true" || node.Query.Get("insecure") == "1"
	skipCertVerify := node.Query.Get("skip-cert-verify") == "true" || node.Query.Get("skip-cert-verify") == "1"

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

	if insecure || skipCertVerify {
		tlsData["insecure"] = true
	}

	// Handle ALPN parameter (for hysteria2, typically "h3")
	if alpn := node.Query.Get("alpn"); alpn != "" {
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}

	outbound["tls"] = tlsData
}

// buildSSHOutbound builds outbound configuration for SSH protocol
func buildSSHOutbound(node *config.ParsedNode, outbound map[string]interface{}) {
	// User is required (stored in UUID field from userinfo)
	if node.UUID != "" {
		outbound["user"] = node.UUID
	} else {
		outbound["user"] = "root" // Default user for SSH
		debuglog.WarnLog("Parser: SSH link missing user, using default 'root'")
	}

	// Password is optional (can be in query params from userinfo)
	if password := node.Query.Get("password"); password != "" {
		outbound["password"] = password
	}

	// Private key (inline) - if provided, takes precedence over private_key_path
	if privateKey := node.Query.Get("private_key"); privateKey != "" {
		// URL decode if needed
		if decoded, err := url.QueryUnescape(privateKey); err == nil {
			outbound["private_key"] = decoded
		} else {
			outbound["private_key"] = privateKey
		}
	} else if privateKeyPath := node.Query.Get("private_key_path"); privateKeyPath != "" {
		// Private key path
		if decoded, err := url.QueryUnescape(privateKeyPath); err == nil {
			outbound["private_key_path"] = decoded
		} else {
			outbound["private_key_path"] = privateKeyPath
		}
	}

	// Private key passphrase
	if passphrase := node.Query.Get("private_key_passphrase"); passphrase != "" {
		if decoded, err := url.QueryUnescape(passphrase); err == nil {
			outbound["private_key_passphrase"] = decoded
		} else {
			outbound["private_key_passphrase"] = passphrase
		}
	}

	// Host key (can be multiple, comma-separated)
	if hostKey := node.Query.Get("host_key"); hostKey != "" {
		// Split by comma if multiple keys provided
		hostKeys := strings.Split(hostKey, ",")
		// Trim spaces and decode each key
		decodedKeys := make([]string, 0, len(hostKeys))
		for _, key := range hostKeys {
			key = strings.TrimSpace(key)
			if key != "" {
				if decoded, err := url.QueryUnescape(key); err == nil {
					decodedKeys = append(decodedKeys, decoded)
				} else {
					decodedKeys = append(decodedKeys, key)
				}
			}
		}
		if len(decodedKeys) > 0 {
			outbound["host_key"] = decodedKeys
		}
	}

	// Host key algorithms (can be multiple, comma-separated)
	if algorithms := node.Query.Get("host_key_algorithms"); algorithms != "" {
		algList := strings.Split(algorithms, ",")
		for i := range algList {
			algList[i] = strings.TrimSpace(algList[i])
		}
		// Remove empty strings
		filteredAlgs := make([]string, 0, len(algList))
		for _, alg := range algList {
			if alg != "" {
				filteredAlgs = append(filteredAlgs, alg)
			}
		}
		if len(filteredAlgs) > 0 {
			outbound["host_key_algorithms"] = filteredAlgs
		}
	}

	// Client version
	if clientVersion := node.Query.Get("client_version"); clientVersion != "" {
		if decoded, err := url.QueryUnescape(clientVersion); err == nil {
			outbound["client_version"] = decoded
		} else {
			outbound["client_version"] = clientVersion
		}
	}
}

// parseVMessJSON parses VMess configuration from decoded JSON.
// VMess protocol uses base64-encoded JSON format (vmess://base64(json)) instead of
// standard URI format used by other protocols (vless://, trojan://, ssh://, etc.).
// This is why VMess requires separate parsing logic and cannot use the common
// URI parsing path that other protocols share.
func parseVMessJSON(vmessConfig map[string]interface{}, skipFilters []map[string]string) (*config.ParsedNode, error) {
	node := &config.ParsedNode{
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
		node.Label = ps
		node.Tag, node.Comment = extractTagAndComment(ps)
		node.Tag = normalizeFlagTag(node.Tag)
	} else {
		node.Tag = generateDefaultTag("vmess", node.Server, node.Port)
		node.Comment = node.Tag
	}

	if scy, ok := vmessConfig["scy"].(string); ok && scy != "" {
		node.Query.Set("security", scy)
	} else {
		node.Query.Set("security", "auto")
	}

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
