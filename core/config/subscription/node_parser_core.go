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
	"singbox-launcher/core/config/registry"
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
		strings.HasPrefix(trimmed, "socks4a://") ||
		strings.HasPrefix(trimmed, "socks4://") ||
		strings.HasPrefix(trimmed, "socks://") ||
		strings.HasPrefix(trimmed, "naive+https://") ||
		strings.HasPrefix(trimmed, "naive+quic://") ||
		strings.HasPrefix(trimmed, "proxy-http://") ||
		strings.HasPrefix(trimmed, "proxy-https://") ||
		strings.HasPrefix(trimmed, "proxy+http://") ||
		strings.HasPrefix(trimmed, "proxy+https://")
}

// MaxURILength — предел длины share-URI, из реестра контракта
// (registry/limits.json, max_uri_length), а не из константы кода.
//
// До SPEC 131 W2c Go держал 8192 против канонических 65536 у LxBox
// (ловушка Л18): длинная, но полностью валидная ссылка — Amnezia с
// контейнером, MASQUE с ключами — отбивалась на десктопе кодом uri_too_long
// и принималась на мобиле. Расхождение стояло в САМОМ НАЧАЛЕ конвейера, до
// всякого санитайза, и никакое правило реестра до такой ссылки не доезжало.
//
// var, а не const: значение читается с загрузкой реестра. Реестр вшит в
// бинарь, поэтому промах возможен лишь при порче сборки — там остаётся
// прежний потолок, чтобы разбор ссылок не остался вовсе без предела.
var MaxURILength = maxURILengthFromRegistry()

// maxURILengthDefault — запасной предел, если реестр не прочитался.
const maxURILengthDefault = 65536

func maxURILengthFromRegistry() int {
	reg, err := registry.Get()
	if err != nil {
		return maxURILengthDefault
	}
	if n, ok := reg.LimitInt("max_uri_length"); ok && n > 0 {
		return n
	}
	return maxURILengthDefault
}

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

	// Validate URI length
	if len(uri) > MaxURILength {
		return nil, fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), MaxURILength)
	}

	// SPEC 133: сначала спрашиваем ДВИЖОК реестра. Ведёт ли он эту ссылку,
	// решает секция схемы (`live: true` + её собственный detect), а не список
	// имён здесь. Не ведёт — идём прежним путём ниже; развилка временная и
	// исчезнет вместе с атрибутом, когда переведены будут все схемы.
	if node, engErr, handled := parseURIByEngine(uri, skipFilters); handled {
		return node, engErr
	}

	// awg://<base64 .conf>#label — панели заворачивают в ссылку целый wg-quick
	// (AmneziaWG 3.x по подписке). Форма key@host:port идёт штатной веткой ниже.
	if strings.HasPrefix(uri, "awg://") || strings.HasPrefix(uri, "wireguard://") {
		if node, ok, err := parseWGConfBase64Link(uri, skipFilters); ok {
			return node, err
		}
	}

	// Determine scheme
	scheme := ""
	uriToParse := uri
	defaultPort := 443 // Default port for most protocols

	// Determine scheme and handle protocol-specific parsing
	switch {
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
	if err != nil && scheme == "hysteria" {
		if u, plist, recErr := hysteria2RecoverMultiPortAuthority(uriToParse); recErr == nil && u != nil {
			parsedURL, err, hy2AuthPortList = u, nil, plist
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// Hysteria v1: хост обязателен, учётные данные живут в query (auth=),
	// поэтому userinfo здесь не требуем — в отличие от блока выше.
	if scheme == "hysteria" && parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("invalid hysteria URI: missing hostname")
	}

	// Extract components
	node := &configtypes.ParsedNode{
		Scheme: scheme,
		Server: parsedURL.Hostname(),
		Query:  parsedURL.Query(),
	}

	if scheme == "hysteria" && hy2AuthPortList != "" {
		if ex := strings.TrimSpace(queryGetFold(node.Query, "mport")); ex != "" {
			node.Query.Set("mport", hy2AuthPortList+","+ex)
		} else {
			node.Query.Set("mport", hy2AuthPortList)
		}
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

	// Учётные данные из userinfo. url.Parse уже снял percent-кодирование:
	// Username() отдаёт готовое значение, и повторный разбор портил бы
	// законные пароли ('+' становился пробелом, %XX декодировалось дважды).
	//
	// Воронка «userinfo-пароль в node.Query» отсюда УШЛА вместе с tuic —
	// последней схемой, которой она была нужна. Из-за неё у ssh «работало»
	// ненаписанное ?password= (QUIRKS Q133-49). У hysteria v1 учётные данные
	// живут в query (auth=), и второго компонента userinfo у неё нет.
	if parsedURL.User != nil {
		node.UUID = parsedURL.User.Username()
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

func buildOutbound(node *configtypes.ParsedNode) map[string]interface{} {
	outbound := make(map[string]interface{})
	// `ech=` не переводится никуда ни у одной схемы (D-122): Xray-форма несёт
	// чужой ключ. Помечаем один раз здесь, а не в каждой TLS-ветке.
	noteECHIgnored(node)
	outbound["tag"] = node.Tag
	// Переименований типа здесь больше нет: ss → "shadowsocks" и
	// socks*/version — свойство СХЕМЫ, и его объявляют `defaults` секций
	// (shadowsocks.json, socks.json). Обе схемы на движке, сюда не доходят.
	outbound["type"] = node.Scheme
	outbound["server"] = node.Server
	outbound["server_port"] = node.Port

	if node.Scheme == "hysteria" {
		buildHysteriaOutbound(node, outbound)
	}

	return outbound
}
