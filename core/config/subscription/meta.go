package subscription

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	v5 "singbox-launcher/core/state/v5"
)

// inlineCommentScanLimit — сколько первых строк тела сканировать в поисках
// `#header-name: value`. Достаточно с запасом: V2Board / Xboard
// эмитят 1-3 такие строки в самом начале.
const inlineCommentScanLimit = 100

// canonical-имена headers (RFC-стиль). http.Header.Get их нормализует
// до Title-case, поэтому достаточно объявить ровно эти ключи.
const (
	headerSubscriptionUserInfo   = "Subscription-Userinfo"
	headerProfileTitle           = "Profile-Title"
	headerProfileUpdateInterval  = "Profile-Update-Interval"
	headerSupportURL             = "Support-Url"
	headerProfileWebPageURL      = "Profile-Web-Page-Url"
	headerContentDisposition     = "Content-Disposition"
)

// ParseHeaders — извлекает метаданные подписки из HTTP response headers.
//
// Возвращает SubscriptionMeta с заполненными только header-derived полями
// (UserInfo / ProfileTitle / SupportURL / ...). Fetch history, preview
// заполняются вызывающим уровнем (fetcher.go).
//
// Headers контракт: см. SPEC 052 §"Headers контракт"
// (https://github.com/Leadaxe/LxBox/blob/main/docs/PROTOCOLS.md).
func ParseHeaders(h http.Header) v5.SubscriptionMeta {
	var m v5.SubscriptionMeta
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
	return m
}

// ParseInlineComments — сканит первые ~100 строк тела на `#header: value`.
//
// Останавливается при первой непустой строке без `#` префикса (это уже
// нодовая часть). Headers здесь те же что в HTTP, но без Title-case
// (case-insensitive matching).
func ParseInlineComments(body []byte) v5.SubscriptionMeta {
	var m v5.SubscriptionMeta
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
		}
	}
	return m
}

// MergeMeta мерджит два SubscriptionMeta: headers (HTTP) выигрывают,
// inline (#-comments в body) — fallback для пустых полей.
//
// Поля fetch history (LastFetchedAt, ErrorCount, ...) не трогаются —
// они никогда не приходят из header'ов.
func MergeMeta(headers, inline v5.SubscriptionMeta) v5.SubscriptionMeta {
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
	return out
}

// parseSubscriptionUserinfo разбирает значение subscription-userinfo:
//
//	upload=0; download=12345; total=999999; expire=1717171717
//
// Allows ";" or "," as разделитель, whitespace tolerant. Возвращает nil
// если ни одно поле не распозналось (malformed input).
func parseSubscriptionUserinfo(s string) *v5.UserInfo {
	if s == "" {
		return nil
	}
	ui := &v5.UserInfo{}
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
		if hasControlChars(str) {
			continue
		}
		return strings.TrimSpace(str)
	}
	return ""
}

func hasControlChars(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7F {
			return true
		}
	}
	return false
}

// parseContentDispositionFilename извлекает filename= из значения
// content-disposition:
//
//	attachment; filename="my profile.txt"
//	attachment; filename=plain.txt
//	attachment; filename*=UTF-8''My%20Profile.txt   (RFC 5987)
//
// Использует mime.ParseMediaType — он handle'ит и quoted, и raw,
// и filename* (UTF-8'')-форму (через automatic decode в mime).
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
