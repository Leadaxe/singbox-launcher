// Package subscription provides parsing logic for various proxy node formats.
// It supports VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC, SSH, SOCKS5, and WireGuard protocols, handling
// both direct links and subscription formats.
package subscription

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
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

	// Рукописных парсеров ссылок больше НЕТ: последней ушла wireguard/awg, и
	// обе её формы — `key@host:port` и целый wg-quick под base64 — ведёт
	// секция реестра двумя `forms` одной таблицей записей.
	//
	// Сюда попадает только текст, который не опознала ни одна секция.
	return nil, ErrUnsupportedScheme
}

// ErrUnsupportedScheme — последний отказ ParseNode: схему строки не ведёт ни
// одна секция реестра.
//
// Сторожевая переменная, а не строка на месте: отбраковке нужен МАШИННЫЙ код
// (`scheme_unsupported`, D-088), а различать «эту схему мы не знаем вовсе» от
// прочих отказов разбора по тексту ошибки нельзя — текст у каждой стороны
// свой. Обёртки над ней (`%w`) сохраняют признак для errors.Is.
var ErrUnsupportedScheme = errors.New("unsupported scheme")

// Private helper functions (migrated from parser.go)

// validateAndFixUTF8 validates and fixes invalid UTF-8 in a string.
// Returns fixed string and true if valid, or original string and false if unfixable.
// Thin wrapper over the shared FixUTF8String helper (utf8_utils.go).
func validateAndFixUTF8(s string) (string, bool) {
	return FixUTF8String(s)
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
