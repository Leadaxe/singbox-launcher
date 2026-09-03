// Package subscription provides parsing logic for various proxy node formats.
// It supports VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, SSH, SOCKS5, and WireGuard protocols, handling
// both direct links and subscription formats.
package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
		strings.HasPrefix(trimmed, "hysteria://") ||
		strings.HasPrefix(trimmed, "hy://") ||
		strings.HasPrefix(trimmed, "tuic://") ||
		strings.HasPrefix(trimmed, "anytls://") ||
		strings.HasPrefix(trimmed, "ssh://") ||
		strings.HasPrefix(trimmed, "wireguard://") ||
		strings.HasPrefix(trimmed, "awg://") ||
		strings.HasPrefix(trimmed, "masque://") ||
		strings.HasPrefix(trimmed, "vpn://") ||
		strings.HasPrefix(trimmed, "socks5://") ||
		strings.HasPrefix(trimmed, "socks://") ||
		strings.HasPrefix(trimmed, "naive+https://") ||
		strings.HasPrefix(trimmed, "naive+quic://") ||
		strings.HasPrefix(trimmed, "proxy-http://") ||
		strings.HasPrefix(trimmed, "proxy-https://") ||
		strings.HasPrefix(trimmed, "proxy+http://") ||
		strings.HasPrefix(trimmed, "proxy+https://")
}

// MaxURILength defines the maximum allowed length for a proxy URI
const MaxURILength = 8192 // 8 KB - reasonable limit for proxy URIs

// percentEncodeUserinfoSpaces percent-encodes raw spaces inside the userinfo
// segment of a proxy URI (between "://" and the authority's '@').
//
// Some public lists paste a promo login with a stray space —
// `vless://Telegramjoin:TurboConfigs @1.2.3.4:80?...` — and net/url refuses the
// whole URI with "invalid userinfo", dropping an otherwise usable node. A space
// is never meaningful there, so encoding it is lossless: callers unescape the
// userinfo anyway.
//
// Mirrors percentEncodeWGUserinfoSlashes (node_parser_wireguard.go), which
// solves the same class of problem for raw '/' in base64 keys.
func percentEncodeUserinfoSpaces(uri string) string {
	const sep = "://"
	si := strings.Index(uri, sep)
	if si < 0 {
		return uri
	}
	start := si + len(sep)
	rest := uri[start:]

	// Only the authority's '@' counts; a '@' inside the query or fragment
	// (a Telegram handle in the node name, say) is not a userinfo separator.
	at := strings.IndexByte(rest, '@')
	if at < 0 {
		return uri
	}
	if strings.ContainsAny(rest[:at], "?#") {
		return uri
	}

	userinfo := rest[:at]
	if !strings.Contains(userinfo, " ") {
		return uri
	}
	return uri[:start] + strings.ReplaceAll(userinfo, " ", "%20") + uri[start+at:]
}

// ParseNode parses a single node URI and applies skip filters
func ParseNode(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	// Amnezia vpn:// (compressed profile JSON, SPEC 075) is dispatched before the
	// generic length guard: such links wrap a whole profile and routinely exceed
	// MaxURILength; parseAmneziaVPNLink enforces its own size caps.
	if strings.HasPrefix(uri, "vpn://") {
		return parseAmneziaVPNLink(uri, skipFilters)
	}

	// MASQUE (CONNECT-IP / WARP) wraps full key material in the URI like
	// wireguard://; dispatch to its own parser (builds the endpoint directly).
	// Requires core >= lx.2 (masque outbound; launcher pins lx.3).
	if strings.HasPrefix(uri, "masque://") {
		return parseMasqueURI(uri, skipFilters)
	}

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

	case strings.HasPrefix(uri, "hysteria://"), strings.HasPrefix(uri, "hy://"):
		// Hysteria v1 (ядро: type "hysteria"). Отдельный протокол, не «старая
		// запись hysteria2»: учётные данные в query (auth=), obfs — плоская
		// строка, bandwidth согласуется с сервером. Схема hy:// — короткий
		// алиас клиентов 1.x, нормализуем к hysteria:// для net/url.
		scheme = "hysteria"
		defaultPort = 443
		if strings.HasPrefix(uri, "hy://") {
			uriToParse = strings.Replace(uri, "hy://", "hysteria://", 1)
		} else {
			uriToParse = uri
		}

	case strings.HasPrefix(uri, "tuic://"):
		// TUIC v5 (uuid:password@host:port). Runs over QUIC; default port 443.
		scheme = "tuic"

	case strings.HasPrefix(uri, "anytls://"):
		// AnyTLS (password@host:port). Single credential in userinfo like Trojan;
		// always over TLS; default port 443.
		scheme = "anytls"

	case strings.HasPrefix(uri, "ssh://"):
		scheme = "ssh"
		defaultPort = 22 // Default port for SSH

	case strings.HasPrefix(uri, "socks5://"):
		// ВНИМАНИЕ: алиас НЕ канонизируется в "socks" (CANON §1), хотя канон
		// схемы — именно "socks". Причина не в контракте, а в дефолтном теге:
		// он строится из схемы (`fmt.Sprintf("%s-%s-%d", scheme, ...)`, :551),
		// и канонизация переименовала бы socks5-host-1080 → socks-host-1080
		// у ВСЕХ существующих узлов. Тег входит в identity-хеш и в ключи
		// disabled-отметок — переименование сбросило бы пользовательские
		// отметки и порвало ссылки detour/цепочек. Расхождение с Dart
		// остаётся в корпусе как per-app override (docs/IDENTITY.md §4a-C).
		scheme = "socks5"
		defaultPort = 1080
	case strings.HasPrefix(uri, "socks://"):
		scheme = "socks"
		defaultPort = 1080

	case strings.HasPrefix(uri, "wireguard://"), strings.HasPrefix(uri, "awg://"):
		// AmneziaWG (SPEC 073): awg:// is an alias — same endpoint shape as
		// wireguard:// plus promoted obfuscation params (jc/jmin/.../i1-i5),
		// handled inside parseWireGuardURI via applyAWGFields. Normalize the
		// scheme so net/url parses it; node.Scheme stays "wireguard" (AWG is a
		// superset of the WG endpoint — keeps GenerateEndpointJSON guard happy).
		wgURI := uri
		if strings.HasPrefix(uri, "awg://") {
			wgURI = strings.Replace(uri, "awg://", "wireguard://", 1)
		}
		return parseWireGuardURI(wgURI, skipFilters)

	case strings.HasPrefix(uri, "naive+https://"), strings.HasPrefix(uri, "naive+quic://"):
		// NaïveProxy URI (de-facto spec: DuckSoft 2020).
		// Replace "naive+xxx" prefix with "https" so net/url can parse the rest;
		// transport mode (HTTP/2 vs QUIC) is remembered in node.Query["quic"].
		scheme = "naive"
		defaultPort = 443
		if strings.HasPrefix(uri, "naive+quic://") {
			uriToParse = strings.Replace(uri, "naive+quic://", "https://", 1)
		} else {
			uriToParse = strings.Replace(uri, "naive+https://", "https://", 1)
		}

	case strings.HasPrefix(uri, "proxy-http://"), strings.HasPrefix(uri, "proxy-https://"),
		strings.HasPrefix(uri, "proxy+http://"), strings.HasPrefix(uri, "proxy+https://"):
		// HTTP(S) CONNECT proxy (SPEC 103 §9.B6; LxBox http_parser.dart). Custom
		// scheme instead of bare http(s):// — those are intercepted upstream as
		// subscription URLs. Has its own parser (dispatched below, mirrors
		// masque/vpn): userinfo, TLS-by-suffix and headers don't fit the generic
		// net/url branch below.
		return parseHTTPProxyURI(uri, skipFilters)

	default:
		return nil, fmt.Errorf("unsupported scheme")
	}

	// Public lists sometimes paste a raw space into the userinfo
	// (`vless://Telegramjoin:TurboConfigs @host:port`), usually a stray
	// separator in a promo login. net/url rejects it outright with
	// "invalid userinfo", so the whole node was lost. Percent-encode spaces in
	// that segment before parsing; everything downstream reads the userinfo
	// through PathUnescape / QueryUnescape and gets the original value back.
	uriToParse = percentEncodeUserinfoSpaces(uriToParse)

	// Parse URI
	parsedURL, err := url.Parse(uriToParse)
	hy2AuthPortList := ""
	if err != nil && (scheme == "hysteria2" || scheme == "hysteria") {
		if u, plist, recErr := hysteria2RecoverMultiPortAuthority(uriToParse); recErr == nil && u != nil {
			parsedURL, err, hy2AuthPortList = u, nil, plist
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// Validate VLESS/Trojan/SSH/TUIC/AnyTLS URI format (must have hostname and userinfo)
	if scheme == "vless" || scheme == "trojan" || scheme == "ssh" || scheme == "tuic" || scheme == "anytls" {
		if parsedURL.Hostname() == "" {
			return nil, fmt.Errorf("invalid %s URI: missing hostname", scheme)
		}
		if parsedURL.User == nil || parsedURL.User.Username() == "" {
			return nil, fmt.Errorf("invalid %s URI: missing userinfo (UUID/password/user)", scheme)
		}
	}
	// Hysteria v1: хост обязателен, учётные данные живут в query (auth=),
	// поэтому userinfo здесь не требуем — в отличие от блока выше.
	if scheme == "hysteria" && parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("invalid hysteria URI: missing hostname")
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

	if (scheme == "hysteria2" || scheme == "hysteria") && hy2AuthPortList != "" {
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

	// Extract port (defaultPort was set in scheme detection). Out-of-range
	// ports are a node-level error: sing-box check rejects the whole config
	// over a single bad server_port, so the node must degrade here instead.
	node.Port = defaultPort
	if port := parsedURL.Port(); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid port %q in URI", port)
		}
		node.Port = p
	}

	// Extract UUID/user
	// For hysteria2, password is in username part of userinfo (hysteria2://password@server:port)
	// For SSH and Trojan, password can be in userinfo (user:password@server:port)
	// url.Parse has already percent-decoded the userinfo: Username()/Password()
	// return the plain values. Re-decoding them (historically via QueryUnescape)
	// corrupted legal credentials — '+' became a space and literal %XX sequences
	// were decoded a second time.
	if parsedURL.User != nil {
		node.UUID = parsedURL.User.Username()
		// Extract password for SSH, Trojan, SOCKS, Naive and TUIC (user:password@server)
		if scheme == "ssh" || scheme == "trojan" || scheme == "socks" || scheme == "socks5" || scheme == "naive" || scheme == "tuic" {
			if password, hasPassword := parsedURL.User.Password(); hasPassword {
				node.Query.Set("password", password)
			}
		}
	}

	// Naive-specific: remember transport mode (HTTP/2 vs QUIC) from the original
	// scheme prefix, and strip the `padding` query param which has no sing-box
	// equivalent and would otherwise leak into logs as an "unknown option".
	if scheme == "naive" {
		if strings.HasPrefix(uri, "naive+quic://") {
			node.Query.Set("quic", "true")
		}
		if node.Query.Has("padding") {
			debuglog.WarnLog("Parser: naive: 'padding' URI parameter has no sing-box equivalent, ignoring (value=%q)", node.Query.Get("padding"))
			node.Query.Del("padding")
			node.AddWarning(WarnNaivePaddingIgnored)
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

	// For some formats, label might be in the path.
	//
	// Из userinfo метка НЕ берётся: там лежат учётные данные, а не имя.
	// vless/vmess — UUID, tuic — UUID, wireguard/masque — приватный ключ,
	// ss/trojan — пароль, ssh/socks — имя пользователя. Прежняя ветка
	// подставляла всё это в Label, и узел без `#fragment` получал в имя
	// свой же секрет: имя едет в UI, логи, скриншоты поддержки и бэкап,
	// то есть значение утекало за пределы локального файла (в отличие от
	// секретов в state.json, которые там by design). Продуктово оно тоже
	// бесполезно — «11111111-1111-…» ничего не говорит пользователю.
	// Пустой Label ниже разворачивается в `scheme-server-port`
	// (generateDefaultTag) — осмысленное имя без секрета.
	// Паритет с LxBox: та сторона userinfo в метку не берёт вовсе.
	if node.Label == "" && parsedURL.Path != "" && parsedURL.Path != "/" {
		node.Label = strings.TrimPrefix(parsedURL.Path, "/")
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

// decodeBase64WithPadding attempts to decode base64 string with automatic padding.
// Thin wrapper over the shared DecodeBase64Multi helper (encoding_utils.go),
// which tries the same four variants in the same order.
func decodeBase64WithPadding(s string) ([]byte, error) {
	decoded, _, err := DecodeBase64Multi(s)
	return decoded, err
}

// validateAndFixUTF8 validates and fixes invalid UTF-8 in a string.
// Returns fixed string and true if valid, or original string and false if unfixable.
// Thin wrapper over the shared FixUTF8String helper (utf8_utils.go).
func validateAndFixUTF8(s string) (string, bool) {
	return FixUTF8String(s)
}

// validateAndFixUTF8Bytes validates and fixes invalid UTF-8 in bytes.
// Returns fixed string and true if valid, or empty string and false if unfixable.
// Thin wrapper over the shared FixUTF8Bytes helper (utf8_utils.go).
func validateAndFixUTF8Bytes(b []byte) (string, bool) {
	return FixUTF8Bytes(b)
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

// matchesPattern checks if a value matches a pattern (supports regex and negation).
// Delegates to the shared configtypes.MatchesPattern so subscription skip-filters and
// selector filters stay byte-equivalent (see core/config/configtypes/matcher.go).
func matchesPattern(value, pattern string) bool {
	return configtypes.MatchesPattern(value, pattern)
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

// noteWSEarlyDataConverted ставит info-код, если early data приехала из
// Xray-хвоста `?ed=N` В ПУТИ: путь из ссылки попал в конфиг не буквально, а
// разложенным на два поля.
//
// Именно из пути, а не из плоских `ed`/`eh` в query (вторая форма, SPEC 103
// §9.E): там ссылка НЕ вводит в заблуждение — параметры названы своими
// именами и переносятся один в один, преобразовывать нечего. Проверка идёт
// по исходному пути, а не по результату: uriTransportFromQuery — чистый
// построитель транспорта, узла он не знает, и менять его сигнатуру ради
// info-кода несоразмерно.
func noteWSEarlyDataConverted(node *configtypes.ParsedNode, transport map[string]interface{}) {
	if node == nil || transport == nil {
		return
	}
	if _, ok := transport["max_early_data"]; !ok {
		return
	}
	rawPath := queryGetFold(node.Query, "path")
	if _, maxED := splitWSEarlyData(decodeResidualPercent(rawPath)); maxED > 0 {
		node.AddWarning(WarnWSEarlyDataEDConverted)
	}
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
			noteWSEarlyDataConverted(node, transport)
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
		}
		if pe := strings.TrimSpace(queryGetFold(node.Query, "packetEncoding")); pe != "" {
			// sing-box accepts only "xudp" / "packetaddr" (or empty/omitted).
			// Some xray-style subscriptions emit packetEncoding=none for
			// nodes that don't need xtls — semantically equivalent to
			// omitting the field. v1.13.x sing-box panics on any other
			// value (see SPEC 049 upstream report), so filter to the
			// allow-list and drop everything else with a warning.
			switch strings.ToLower(pe) {
			case "xudp", "packetaddr":
				outbound["packet_encoding"] = strings.ToLower(pe)
			case "none":
				// silently drop — common, by-design "no special encoding"
			default:
				debuglog.WarnLog("Parser: unknown packetEncoding %q in %s URI %s — dropping field", pe, node.Scheme, node.Tag)
				node.AddWarning(WarnPacketEncodingUnknown)
			}
		}

		// VLESS post-quantum encryption layer (lx SPEC 032, core option/vless.go
		// Encryption). `none` and the empty value mean "layer off" — the field is
		// then omitted, matching LxBox and CANON §2.4. Dropping it outright made
		// such a node unusable on desktop while it worked on mobile (SPEC 103).
		if enc := strings.TrimSpace(queryGetFold(node.Query, "encryption")); enc != "" && !strings.EqualFold(enc, "none") {
			outbound["encryption"] = enc
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

		switch {
		case network == "xhttp":
			// sing-box-lx xhttp transport (SPEC 071). Distinct from httpupgrade.
			tr := xhttpTransportFromQuery(node.Query)
			if _, ok := tr["host"]; !ok {
				if h := queryGetFold(node.Query, "sni"); h != "" {
					tr["host"] = h
				}
			}
			outbound["transport"] = tr

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
				if network == "ws" {
					// split Xray's `?ed=N` early-data tail out of the path (issue #96)
					if applyWSEarlyData(transport, path) {
						node.AddWarning(WarnWSEarlyDataEDConverted)
					}
				} else {
					transport["path"] = path
				}
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

			if fp := utlsFingerprintFromQuery(node.Query, "fp", "fingerprint"); fp != "" {
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
			noteWSEarlyDataConverted(node, t)
		}
		if tlsData, ok := trojanTLSFromNode(node); ok {
			outbound["tls"] = tlsData
		}
	} else if node.Scheme == "ss" {
		if method := node.Query.Get("method"); method != "" {
			outbound["method"] = method
		}
		if password := node.Query.Get("password"); password != "" {
			outbound["password"] = password
		}
	} else if node.Scheme == "hysteria2" {
		buildHysteria2Outbound(node, outbound)
	} else if node.Scheme == "hysteria" {
		buildHysteriaOutbound(node, outbound)
	} else if node.Scheme == "tuic" {
		buildTuicOutbound(node, outbound)
	} else if node.Scheme == "anytls" {
		buildAnyTLSOutbound(node, outbound)
	} else if node.Scheme == "ssh" {
		buildSSHOutbound(node, outbound)
	} else if node.Scheme == "naive" {
		buildNaiveOutbound(node, outbound)
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
