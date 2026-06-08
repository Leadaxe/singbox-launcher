package subscription

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"mime"
	"net/http"
	"singbox-launcher/core/state"
	"strconv"
	"strings"
	"unicode/utf8"
)

// inlineCommentScanLimit — сколько первых строк тела сканировать в поисках
// `#header-name: value`. Достаточно с запасом: V2Board / Xboard
// эмитят 1-3 такие строки в самом начале.
const inlineCommentScanLimit = 100

// canonical-имена headers (RFC-стиль). http.Header.Get их нормализует
// до Title-case, поэтому достаточно объявить ровно эти ключи.
const (
	headerSubscriptionUserInfo  = "Subscription-Userinfo"
	headerProfileTitle          = "Profile-Title"
	headerProfileUpdateInterval = "Profile-Update-Interval"
	headerSupportURL            = "Support-Url"
	headerProfileWebPageURL     = "Profile-Web-Page-Url"
	headerContentDisposition    = "Content-Disposition"

	// Announce-headers — провайдер шлёт их вместе с **пустым телом**, когда
	// он не может вернуть подписку (HWID-лимит / region-block / expired
	// trial / etc.). Без этого декода юзер видит только generic «empty
	// subscription body» и не понимает что делать.
	//
	// Видел в дикой природе (Marzban / Sub-Store / NashVPN-style панели):
	//
	//   announce: base64:<UTF-8 message for the user>
	//   announce-url: https://t.me/some_support_bot
	//   x-hwid-limit: true
	headerAnnounce    = "Announce"
	headerAnnounceURL = "Announce-Url"

	// HWID-family (Remnawave / Marzneshin / Marzban). Truthy values:
	// "true" / "1" / "yes" / "on" (case-insensitive). See SPEC 061 §4.
	headerHWIDActive            = "X-Hwid-Active"
	headerHWIDNotSupported      = "X-Hwid-Not-Supported"
	headerHWIDMaxDevicesReached = "X-Hwid-Max-Devices-Reached"
	headerHWIDLimit             = "X-Hwid-Limit" // legacy alias of MaxDevicesReached
)

// truthyHeaderValue — common parse for `true`/`1`/`yes`/`on` (case-insensitive).
// Used by every X-Hwid-* boolean and by ParseInlineComments for the few keys
// that map onto bool fields.
func truthyHeaderValue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// ProviderAnnounce is re-exported from state for callsites that historically
// referenced subscription.ProviderAnnounce. The struct lives in state so
// SubscriptionMeta can carry it without circular imports — see
// state/provider_announce.go for full semantics.
type ProviderAnnounce = state.ProviderAnnounce

// ParseAnnounce — вытаскивает provider-announce headers (см. константы выше)
// из ответа. Возвращает «пустой» ProviderAnnounce если ни одного нет —
// caller через IsEmpty решает, нужно ли заворачивать в специальную ошибку.
//
// Декодит `Announce` через тот же `decodeProfileTitle` helper (handles
// `base64:` prefix + auto-detect bare base64), потому что провайдеры
// шлют announce ровно тем же способом.
//
// Legacy alias mirroring: HWIDLimit (Marzban / v2RayTun) and
// HWIDMaxDevicesReached (Remnawave) describe the same condition. When either
// header is set we populate **both** flags so UI code can use whichever
// field is more readable in context.
func ParseAnnounce(h http.Header) state.ProviderAnnounce {
	var a state.ProviderAnnounce
	if h == nil {
		return a
	}
	if v := strings.TrimSpace(h.Get(headerAnnounce)); v != "" {
		a.Message = decodeProfileTitle(v)
	}
	if v := strings.TrimSpace(h.Get(headerAnnounceURL)); v != "" {
		a.URL = v
	}
	if truthyHeaderValue(h.Get(headerHWIDActive)) {
		a.HWIDActive = true
	}
	if truthyHeaderValue(h.Get(headerHWIDNotSupported)) {
		a.HWIDNotSupported = true
	}
	limit := truthyHeaderValue(h.Get(headerHWIDLimit))
	maxReached := truthyHeaderValue(h.Get(headerHWIDMaxDevicesReached))
	if limit || maxReached {
		// Aliased pair: mirror to both fields so callers can read either.
		a.HWIDLimit = true
		a.HWIDMaxDevicesReached = true
	}
	if v := strings.TrimSpace(h.Get(headerProfileTitle)); v != "" {
		a.ProfileTitle = decodeProfileTitle(v)
	}
	return a
}

// ParseHeaders — извлекает метаданные подписки из HTTP response headers.
//
// Возвращает SubscriptionMeta с заполненными только header-derived полями
// (UserInfo / ProfileTitle / SupportURL / ...). Fetch history, preview
// заполняются вызывающим уровнем (fetcher.go).
//
// Headers контракт: см. SPEC 052 §"Headers контракт"
// (https://github.com/Leadaxe/LxBox/blob/main/docs/PROTOCOLS.md).
func ParseHeaders(h http.Header) state.SubscriptionMeta {
	var m state.SubscriptionMeta
	if h == nil {
		return m
	}

	if v := strings.TrimSpace(h.Get(headerSubscriptionUserInfo)); v != "" {
		if ui := parseSubscriptionUserinfo(v); ui != nil {
			m.UserInfo = ui
		}
	}
	if v := strings.TrimSpace(h.Get(headerProfileTitle)); v != "" {
		m.ProfileTitle = decodeProfileTitle(v)
	}
	if v := strings.TrimSpace(h.Get(headerProfileUpdateInterval)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			m.ProfileUpdateIntervalHours = n
		}
	}
	if v := strings.TrimSpace(h.Get(headerSupportURL)); v != "" {
		m.SupportURL = v
	}
	if v := strings.TrimSpace(h.Get(headerProfileWebPageURL)); v != "" {
		m.ProfileWebPageURL = v
	}
	if v := strings.TrimSpace(h.Get(headerContentDisposition)); v != "" {
		if name := parseContentDispositionFilename(v); name != "" {
			m.ContentDispositionFilename = name
		}
	}
	// SPEC 061: announce headers also surface on a contentful 200 — the
	// subscription works but the provider has a message ("trial expiring in
	// 3 days", "device limit warning"). UI shows a 📢 badge in that case.
	if ann := ParseAnnounce(h); !ann.IsEmpty() {
		annCopy := ann
		m.ProviderAnnounce = &annCopy
	}
	return m
}

// ParseInlineComments — сканит первые ~100 строк тела на `#header: value`.
//
// Останавливается при первой непустой строке без `#` префикса (это уже
// нодовая часть). Headers здесь те же что в HTTP, но без Title-case
// (case-insensitive matching).
func ParseInlineComments(body []byte) state.SubscriptionMeta {
	var m state.SubscriptionMeta
	if len(body) == 0 {
		return m
	}

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)

	count := 0
	for scanner.Scan() {
		count++
		if count > inlineCommentScanLimit {
			break
		}
		line := strings.TrimRight(scanner.Text(), "\r\t ")
		if line == "" {
			// blank lines в начале допустимы — продолжаем
			continue
		}
		if !strings.HasPrefix(line, "#") {
			// первая non-empty non-comment строка → дальше тело
			break
		}
		// strip ведущий "#" и пробельные символы
		raw := strings.TrimLeft(line[1:], "\t ")
		// формат: <header>: <value>
		colon := strings.IndexByte(raw, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(raw[:colon])
		value := strings.TrimSpace(raw[colon+1:])
		if name == "" || value == "" {
			continue
		}

		switch strings.ToLower(name) {
		case "subscription-userinfo":
			if ui := parseSubscriptionUserinfo(value); ui != nil {
				m.UserInfo = ui
			}
		case "profile-title":
			m.ProfileTitle = decodeProfileTitle(value)
		case "profile-update-interval":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				m.ProfileUpdateIntervalHours = n
			}
		case "support-url":
			m.SupportURL = value
		case "profile-web-page-url":
			m.ProfileWebPageURL = value
		case "content-disposition":
			if name := parseContentDispositionFilename(value); name != "" {
				m.ContentDispositionFilename = name
			}
		// SPEC 061 §5: announce / announce-url are valid inline too —
		// static hosting (Gist / GitHub Pages) can't set HTTP headers but
		// providers still want to surface notices through them.
		// HWID-family flags are deliberately NOT mirrored here — they only
		// make sense for server-side device counting, which static hosts
		// don't have.
		case "announce":
			if m.ProviderAnnounce == nil {
				m.ProviderAnnounce = &state.ProviderAnnounce{}
			}
			m.ProviderAnnounce.Message = decodeProfileTitle(value)
		case "announce-url":
			if m.ProviderAnnounce == nil {
				m.ProviderAnnounce = &state.ProviderAnnounce{}
			}
			m.ProviderAnnounce.URL = value
		}
	}
	// Drop the announce pointer if no field stuck (keeps state JSON tidy).
	if m.ProviderAnnounce != nil && m.ProviderAnnounce.IsEmpty() {
		m.ProviderAnnounce = nil
	}
	return m
}

// MergeMeta мерджит два SubscriptionMeta: headers (HTTP) выигрывают,
// inline (#-comments в body) — fallback для пустых полей.
//
// Поля fetch history (LastFetchedAt, ErrorCount, ...) не трогаются —
// они никогда не приходят из header'ов.
func MergeMeta(headers, inline state.SubscriptionMeta) state.SubscriptionMeta {
	out := headers // copy by value
	if out.UserInfo == nil && inline.UserInfo != nil {
		out.UserInfo = inline.UserInfo
	}
	if out.ProfileTitle == "" {
		out.ProfileTitle = inline.ProfileTitle
	}
	if out.ProfileUpdateIntervalHours == 0 {
		out.ProfileUpdateIntervalHours = inline.ProfileUpdateIntervalHours
	}
	if out.SupportURL == "" {
		out.SupportURL = inline.SupportURL
	}
	if out.ProfileWebPageURL == "" {
		out.ProfileWebPageURL = inline.ProfileWebPageURL
	}
	if out.ContentDispositionFilename == "" {
		out.ContentDispositionFilename = inline.ContentDispositionFilename
	}
	if out.ProviderAnnounce == nil && inline.ProviderAnnounce != nil {
		out.ProviderAnnounce = inline.ProviderAnnounce
	}
	return out
}

// parseSubscriptionUserinfo разбирает значение subscription-userinfo:
//
//	upload=0; download=12345; total=999999; expire=1717171717
//
// Allows ";" or "," as разделитель, whitespace tolerant. Возвращает nil
// если ни одно поле не распозналось (malformed input).
func parseSubscriptionUserinfo(s string) *state.UserInfo {
	if s == "" {
		return nil
	}
	ui := &state.UserInfo{}
	any := false
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == ','
	})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		eq := strings.IndexByte(p, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(p[:eq]))
		val := strings.TrimSpace(p[eq+1:])
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "upload":
			ui.UploadBytes = n
			any = true
		case "download":
			ui.DownloadBytes = n
			any = true
		case "total":
			ui.TotalBytes = n
			any = true
		case "expire":
			ui.ExpireUnix = n
			any = true
		}
	}
	if !any {
		return nil
	}
	return ui
}

// decodeProfileTitle определяет, является ли строка base64-encoded UTF-8
// и возвращает плоский UTF-8 string.
//
// Эвристика:
//  1. Префикс "base64:" → strip + decode (RawStdEncoding допускает оба
//     варианта с padding и без).
//  2. Без префикса: если строка проходит base64-decode и результат —
//     корректный UTF-8 без управляющих символов — используем decoded.
//     Иначе — original.
//
// Не паникует на malformed input — fallback на исходную строку.
func decodeProfileTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if strings.HasPrefix(s, "base64:") {
		raw := strings.TrimPrefix(s, "base64:")
		if dec := tryBase64Decode(raw); dec != "" {
			return dec
		}
		return s
	}

	// Auto-detect: некоторые провайдеры эмитят base64 без префикса.
	// Гристика: длина >= 4, состоит только из base64-alphabet, decode
	// даёт valid UTF-8 без control-chars.
	if looksLikeBase64(s) {
		if dec := tryBase64Decode(s); dec != "" {
			return dec
		}
	}
	return s
}

// looksLikeBase64 — быстрая проверка: только base64-alphabet и длина >= 4.
func looksLikeBase64(s string) bool {
	if len(s) < 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '+', c == '/', c == '=', c == '-', c == '_':
			// ok
		default:
			return false
		}
	}
	return true
}

// tryBase64Decode пробует все 4 варианта (Std/URL × padded/raw).
// Возвращает "" если ни один не дал valid UTF-8 без control-chars.
func tryBase64Decode(s string) string {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		raw, err := enc.DecodeString(s)
		if err != nil {
			continue
		}
		str := string(raw)
		if !utf8.ValidString(str) {
			continue
		}
		// Reject если decoded — байтовая каша с control-chars
		// (false-positive base64-detect на не-base64 строках).
		if HasControlChars(str) {
			continue
		}
		return strings.TrimSpace(str)
	}
	return ""
}

// parseContentDispositionFilename извлекает filename= из значения
// content-disposition:
//
//	attachment; filename="my profile.txt"
//	attachment; filename=plain.txt
//	attachment; filename*=UTF-8''My%20Profile.txt   (RFC 5987)
//
// Использует mime.ParseMediaType — он handle'ит и quoted, и raw,
// и filename* (UTF-8”)-форму (через automatic decode в mime).
func parseContentDispositionFilename(s string) string {
	if s == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(s)
	if err != nil {
		return ""
	}
	if name, ok := params["filename"]; ok && name != "" {
		return name
	}
	return ""
}
