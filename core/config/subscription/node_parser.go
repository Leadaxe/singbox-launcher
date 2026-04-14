// Package subscription provides parsing logic for various proxy node formats.
// It supports VLESS, VMess, Trojan, Shadowsocks, Hysteria2, SSH, SOCKS5, and WireGuard protocols, handling
// both direct links and subscription formats.
package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
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
		strings.HasPrefix(trimmed, "wireguard://") ||
		strings.HasPrefix(trimmed, "socks5://") ||
		strings.HasPrefix(trimmed, "socks://")
}

// MaxURILength defines the maximum allowed length for a proxy URI
const MaxURILength = 8192 // 8 KB - reasonable limit for proxy URIs

// ParseNode parses a single node URI and applies skip filters
func ParseNode(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
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
		fragment := ""
		if i := strings.Index(base64Part, "#"); i >= 0 {
			fragment = base64Part[i+1:]
			base64Part = base64Part[:i]
		}
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
		// VMess: base64(JSON) or legacy cleartext method:uuid@host:port (see parseVMessDecoded).
		if fragment != "" {
			if dec, err := url.PathUnescape(fragment); err == nil {
				fragment = dec
			}
		}
		return parseVMessDecoded(decoded, fragment, skipFilters)

	case strings.HasPrefix(uri, "vless://"):
		scheme = "vless"

	case strings.HasPrefix(uri, "trojan://"):
		scheme = "trojan"

	case strings.HasPrefix(uri, "ss://"):
		scheme = "ss"
		ssPart := strings.TrimPrefix(uri, "ss://")
		var fragSuffix string
		if i := strings.Index(ssPart, "#"); i >= 0 {
			fragSuffix = ssPart[i:]
			ssPart = ssPart[:i]
		}
		ssPart = strings.TrimSpace(ssPart)

		if atIdx := strings.Index(ssPart, "@"); atIdx > 0 {
			encodedUserinfo := ssPart[:atIdx]
			rest := ssPart[atIdx+1:]
			if dec, err := url.PathUnescape(encodedUserinfo); err == nil {
				encodedUserinfo = dec
			}
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
			uriToParse = "ss://" + rest + fragSuffix
		} else {
			// Legacy Shadowsocks URI: ss://base64("method:password@host:port")#tag (no userinfo@host before decoding).
			bare := ssPart
			if dec, err := url.PathUnescape(bare); err == nil {
				bare = dec
			}
			if decoded, err := decodeBase64WithPadding(bare); err != nil {
				debuglog.WarnLog("Parser: SS link is not SIP002 and legacy base64 decode failed: %v", err)
			} else {
				decStr := string(decoded)
				at := strings.Index(decStr, "@")
				if at > 0 {
					left := decStr[:at]
					right := strings.TrimSpace(decStr[at+1:])
					userinfoParts := strings.SplitN(left, ":", 2)
					if len(userinfoParts) == 2 && right != "" {
						ssMethod = strings.TrimSpace(userinfoParts[0])
						ssPassword = userinfoParts[1]
						if !isValidShadowsocksMethod(ssMethod) {
							debuglog.WarnLog("Parser: Invalid or unsupported Shadowsocks method '%s'. Skipping node.", ssMethod)
							return nil, fmt.Errorf("unsupported Shadowsocks encryption method: %s", ssMethod)
						}
						debuglog.DebugLog("Parser: Decoded legacy SS (method:password@host:port in one blob), host part length=%d", len(right))
						uriToParse = "ss://" + right + fragSuffix
					}
				}
			}
			if ssMethod == "" {
				debuglog.WarnLog("Parser: SS link is not in SIP002 format (no @ found): %s", uri)
			}
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

	case strings.HasPrefix(uri, "socks5://"):
		scheme = "socks5"
		defaultPort = 1080
	case strings.HasPrefix(uri, "socks://"):
		scheme = "socks"
		defaultPort = 1080

	case strings.HasPrefix(uri, "wireguard://"):
		return parseWireGuardURI(uri, skipFilters)

	default:
		return nil, fmt.Errorf("unsupported scheme")
	}

	// Parse URI
	parsedURL, err := url.Parse(uriToParse)
	hy2AuthPortList := ""
	if err != nil && scheme == "hysteria2" {
		if u, plist, recErr := hysteria2RecoverMultiPortAuthority(uriToParse); recErr == nil && u != nil {
			parsedURL, err, hy2AuthPortList = u, nil, plist
		}
	}
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
	// Validate SOCKS / SOCKS5: hostname required, user/password optional
	if (scheme == "socks" || scheme == "socks5") && parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("invalid socks URI: missing hostname")
	}

	// Extract components
	node := &configtypes.ParsedNode{
		Scheme: scheme,
		Server: parsedURL.Hostname(),
		Query:  parsedURL.Query(),
	}

	if scheme == "hysteria2" && hy2AuthPortList != "" {
		if ex := strings.TrimSpace(queryGetFold(node.Query, "mport")); ex != "" {
			node.Query.Set("mport", hy2AuthPortList+","+ex)
		} else {
			node.Query.Set("mport", hy2AuthPortList)
		}
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
		// Extract password for SSH, Trojan and SOCKS (user:password@server)
		if scheme == "ssh" || scheme == "trojan" || scheme == "socks" || scheme == "socks5" {
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
	// URL decode and validate UTF-8. Use PathUnescape (not QueryUnescape): in fragments '+' is literal;
	// QueryUnescape would turn '+' into space and corrupt names like "A+B".
	if node.Label != "" {
		if decoded, err := url.PathUnescape(node.Label); err == nil {
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

	node.Label = sanitizeForDisplay(node.Label)
	node.Label = textnorm.NormalizeProxyDisplay(node.Label)

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
//
// Invalid UTF-8 is repaired first: ranging over a broken string makes Go emit
// U+FFFD per bad subsequence, which then gets written into the label and shows
// as replacement glyphs in the UI. ToValidUTF8 drops invalid byte runs before the loop.
func sanitizeForDisplay(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToValidUTF8(s, "")
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
func getNodeValue(node *configtypes.ParsedNode, key string) string {
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

func shouldSkipNode(node *configtypes.ParsedNode, skipFilters []map[string]string) bool {
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

func buildOutbound(node *configtypes.ParsedNode) map[string]interface{} {
	outbound := make(map[string]interface{})
	outbound["tag"] = node.Tag
	// Use "shadowsocks" instead of "ss" for sing-box; "socks" outbound for socks5:// and socks:// URIs
	if node.Scheme == "ss" {
		outbound["type"] = "shadowsocks"
	} else if node.Scheme == "socks" || node.Scheme == "socks5" {
		outbound["type"] = "socks"
		outbound["version"] = "5"
	} else {
		outbound["type"] = node.Scheme
	}
	outbound["server"] = node.Server
	outbound["server_port"] = node.Port

	if node.Scheme == "vless" {
		outbound["uuid"] = node.UUID
		transport, hasTransport := uriTransportFromQuery(node.Query)
		if hasTransport {
			outbound["transport"] = transport
		}
		if node.Flow != "" {
			// Convert xtls-rprx-vision-udp443 to compatible format
			if node.Flow == "xtls-rprx-vision-udp443" {
				outbound["flow"] = "xtls-rprx-vision"
				outbound["packet_encoding"] = "xudp"
				outbound["server_port"] = 443
			} else {
				outbound["flow"] = node.Flow
			}
		} else if strings.TrimSpace(queryGetFold(node.Query, "pbk")) != "" && !hasTransport {
			// REALITY over plain TCP: URI often omits flow=; sing-box accepts xtls-rprx-vision (skip for ws/grpc/http/httpupgrade/…).
			outbound["flow"] = "xtls-rprx-vision"
		}
		if pe := strings.TrimSpace(queryGetFold(node.Query, "packetEncoding")); pe != "" {
			outbound["packet_encoding"] = pe
		}

		if tlsData, ok := vlessTLSFromNode(node); ok {
			outbound["tls"] = tlsData
		}
	} else if node.Scheme == "vmess" {
		outbound["uuid"] = node.UUID

		outbound["security"] = normalizeVMessSecurity(node.Query.Get("security"))

		if alterIDStr := node.Query.Get("alter_id"); alterIDStr != "" {
			if alterID, err := strconv.Atoi(alterIDStr); err == nil {
				outbound["alter_id"] = alterID
			}
		}

		network := strings.ToLower(strings.TrimSpace(node.Query.Get("network")))
		if network == "" {
			network = "tcp"
		}
		if network == "xhttp" {
			network = "httpupgrade"
		}

		switch {
		case network == "httpupgrade":
			tr := map[string]interface{}{"type": "httpupgrade"}
			if p := node.Query.Get("path"); p != "" {
				tr["path"] = p
			}
			h := queryGetFold(node.Query, "host")
			if h == "" {
				h = queryGetFold(node.Query, "sni")
			}
			if h != "" {
				tr["host"] = h
			}
			outbound["transport"] = tr

		case network == "h2":
			tr := map[string]interface{}{"type": "http"}
			if p := node.Query.Get("path"); p != "" {
				tr["path"] = p
			}
			hostStr := queryGetFold(node.Query, "host")
			if hostStr == "" {
				hostStr = queryGetFold(node.Query, "sni")
			}
			if hostStr == "" {
				hostStr = node.Server
			}
			if hostStr != "" {
				tr["host"] = []string{hostStr}
			}
			outbound["transport"] = tr

		case network == "ws" || network == "http" || network == "grpc":
			transport := make(map[string]interface{})
			transport["type"] = network

			if network == "grpc" {
				if path := node.Query.Get("path"); path != "" {
					transport["service_name"] = path
				}
			} else if path := node.Query.Get("path"); path != "" {
				transport["path"] = path
			}

			if network == "ws" {
				host := queryGetFold(node.Query, "host")
				if host == "" {
					host = queryGetFold(node.Query, "sni")
				}
				if host != "" {
					transport["headers"] = map[string]string{"Host": host}
				}
			}
			if network == "http" {
				if host := node.Query.Get("host"); host != "" {
					transport["host"] = []string{host}
				}
			}

			outbound["transport"] = transport
		}

		if node.Query.Get("tls_enabled") == "true" {
			tlsData := map[string]interface{}{
				"enabled": true,
			}

			sni := queryGetFold(node.Query, "sni")
			if sni == "" {
				sni = queryGetFold(node.Query, "peer")
			}
			if sni == "" {
				sni = node.Server
			}
			if sni != "" {
				tlsData["server_name"] = sni
			}

			if alpn := node.Query.Get("alpn"); alpn != "" {
				alpnList := strings.Split(alpn, ",")
				for i := range alpnList {
					alpnList[i] = strings.TrimSpace(alpnList[i])
				}
				tlsData["alpn"] = alpnList
			}

			if fp := NormalizeUTLSFingerprint(queryGetFold(node.Query, "fp")); fp != "" {
				tlsData["utls"] = map[string]interface{}{
					"enabled":     true,
					"fingerprint": fp,
				}
			}

			if tlsInsecureTrue(node.Query) {
				tlsData["insecure"] = true
			}

			outbound["tls"] = tlsData
		}
	} else if node.Scheme == "trojan" {
		outbound["password"] = node.UUID
		if t, ok := uriTransportFromQuery(node.Query); ok {
			outbound["transport"] = t
		}
		outbound["tls"] = trojanTLSFromNode(node)
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
	} else if node.Scheme == "socks" || node.Scheme == "socks5" {
		if node.UUID != "" {
			outbound["username"] = node.UUID
		}
		if password := node.Query.Get("password"); password != "" {
			outbound["password"] = password
		}
	}

	return outbound
}
