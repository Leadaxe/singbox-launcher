package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// NetworkRequestTimeout defines the timeout for network requests
const NetworkRequestTimeout = 30 * time.Second

// explainHTTPStatus converts a subscription-fetch HTTP status code into a
// short human-readable hint. Helps users triage the error without googling
// (common flaky cases: expired tokens → 401/403, provider moved URLs → 404,
// rate-limit → 429, provider outage → 5xx).
func explainHTTPStatus(code int) string {
	switch code {
	case 401:
		return "unauthorized — subscription token may have expired"
	case 403:
		return "forbidden — provider blocked this request (token, IP, or region)"
	case 404:
		return "not found — subscription URL may have moved"
	case 410:
		return "gone — this subscription URL is retired"
	case 429:
		return "rate limited — try again later"
	}
	if code >= 500 && code < 600 {
		return "server error — subscription provider is having issues"
	}
	if code >= 400 && code < 500 {
		return "client error"
	}
	return ""
}

// CreateHTTPClientFunc is a function variable that should be set to core.CreateHTTPClient
var CreateHTTPClientFunc func(timeout time.Duration) *http.Client

// IsNetworkErrorFunc is a function variable that should be set to core.IsNetworkError
var IsNetworkErrorFunc func(err error) bool

// GetNetworkErrorMessageFunc is a function variable that should be set to core.GetNetworkErrorMessage
var GetNetworkErrorMessageFunc func(err error) string

// SubscriptionRequestSettings — minimal settings surface needed by fetcher
// to build HWID-family request headers without importing internal/locale.
//
// Wired by core init (config_service.go) — same pattern as CreateHTTPClientFunc.
// The hook returns a snapshot (value copy of relevant fields) so concurrent
// settings edits don't race with an in-flight fetch.
type SubscriptionRequestSettings struct {
	HWID              string
	SendHWID          bool
	DeviceModelHashed bool
	// UserAgent — пользовательский UA для subscription requests. Пустое →
	// fallback на configtypes.BuildSubscriptionUserAgent (default). Полезно
	// когда провайдер режет наш default и принимает только v2rayN/Hiddify/etc.
	UserAgent string
}

// LoadSubscriptionSettingsFunc is set by core init to read bin/settings.json
// for the HWID-family headers. nil → fetcher sends no HWID headers (no-op
// for non-HWID providers; HWID-binding panels respond with empty body, but
// that's the same behavior as `subscription_send_hwid=false`).
var LoadSubscriptionSettingsFunc func() SubscriptionRequestSettings

// applySubscriptionRequestHeaders sets User-Agent + (when enabled) the four
// X-Hwid-family headers on an outbound subscription request. Centralized so
// FetchSubscriptionWithMeta and the legacy FetchSubscription wrapper stay
// in lockstep — see SPEC 061 §"Request headers".
func applySubscriptionRequestHeaders(req *http.Request) {
	// User-Agent: load settings ONCE here so we can let the user override
	// the default UA via Settings tab. Empty user value → default helper
	// (current launcher version + OS/arch). Both branches Set() once —
	// downstream HWID block re-uses the same snapshot, no double load.
	var s SubscriptionRequestSettings
	if LoadSubscriptionSettingsFunc != nil {
		s = LoadSubscriptionSettingsFunc()
	}
	ua := strings.TrimSpace(s.UserAgent)
	if ua == "" {
		ua = configtypes.BuildSubscriptionUserAgent()
	}
	req.Header.Set("User-Agent", ua)
	if !s.SendHWID || s.HWID == "" {
		return
	}
	req.Header.Set("X-Hwid", s.HWID)
	req.Header.Set("X-Device-OS", platform.DeviceOS())
	req.Header.Set("X-Ver-OS", platform.DeviceOSVersion())
	model := platform.DeviceModel()
	if s.DeviceModelHashed && model != "" {
		sum := sha256.Sum256([]byte(model))
		// 8 bytes = 16 hex chars — stable opaque ID per Remnawave HWID docs
		// "hashed model" mode (caller knows exactly the field width).
		model = hex.EncodeToString(sum[:8])
	}
	req.Header.Set("X-Device-Model", model)
}

// MaxSubscriptionResponseSize — лимит размера ответа от провайдера подписки.
// Защита от memory exhaustion на патологически больших телах.
const MaxSubscriptionResponseSize = 10 * 1024 * 1024 // 10 MB

// FetchResult — результат FetchSubscriptionWithMeta.
//
//   - Body — декодированное содержимое подписки (base64 → plain text URIs);
//   - RawBody — оригинальные байты ответа сервера ДО декодирования; именно
//     они кладутся в bin/subscriptions/<id>.raw, чтобы при следующем
//     Rebuild парсер мог повторно дёрнуть DecodeSubscriptionContent;
//   - Meta — заполненные header-derived поля (UserInfo, ProfileTitle, ...);
//     fetch history (LastFetchedAt и т.д.) — заполняет вызывающий слой;
//   - HTTPStatus — код ответа сервера (200 на success);
//   - RawBodyBytes — len(RawBody), pre-decoded размер для UI.
type FetchResult struct {
	Body         []byte
	RawBody      []byte
	Meta         state.SubscriptionMeta
	HTTPStatus   int
	RawBodyBytes int64
}

// FetchHTTPError — ошибка с не-200 status code; можно использовать
// errors.As для извлечения StatusCode при формировании meta.error_count.
//
// SPEC 061: на 403/410/429/5xx провайдер может приложить announce-headers
// («region blocked → @bot»). Парсим их в Announce поле — UI отрисует тот же
// actionable диалог, что и для FetchAnnounceError.
type FetchHTTPError struct {
	StatusCode int
	Hint       string
	Announce   *state.ProviderAnnounce
}

func (e *FetchHTTPError) Error() string {
	base := fmt.Sprintf("subscription server returned status %d", e.StatusCode)
	if e.Hint != "" {
		base += " (" + e.Hint + ")"
	}
	if e.Announce != nil && !e.Announce.IsEmpty() && e.Announce.Message != "" {
		base += ": " + e.Announce.Message
	}
	if e.Announce != nil && e.Announce.URL != "" {
		base += "  →  " + e.Announce.URL
	}
	return base
}

// FetchAnnounceError — провайдер вернул HTTP 200 с **пустым телом** и
// announce-headers, объясняющими причину (HWID-лимит, region-block,
// expired trial, etc.). Это не «empty subscription body» в смысле
// «пустая подписка» — это provider-side gate, и юзеру надо показать
// что именно сказал провайдер + куда идти.
//
// errors.As вытаскивает структуру для UI: показать диалог с
// message + кликабельной URL вместо плоской error-строки.
//
// Примеры в дикой природе:
//
//	NashVPN (Marzban-style):
//	    announce: base64:<RU-сообщение про лимит устройств>
//	    announce-url: https://t.me/nash_vpn_bot
//	    x-hwid-limit: true
//	    Body: 0 bytes
//
//	Sub-Store с истёкшим trial:
//	    announce: Your trial has expired. Renew at the link.
//	    announce-url: https://example.com/renew
//	    Body: 0 bytes
type FetchAnnounceError struct {
	Announce state.ProviderAnnounce
}

func (e *FetchAnnounceError) Error() string {
	a := e.Announce
	var b strings.Builder
	if a.ProfileTitle != "" {
		b.WriteString(a.ProfileTitle)
		b.WriteString(": ")
	}
	switch {
	case a.Message != "":
		b.WriteString(a.Message)
	case a.HWIDMaxDevicesReached || a.HWIDLimit:
		b.WriteString("subscription provider reports HWID/device limit reached")
	case a.HWIDNotSupported:
		b.WriteString("subscription provider says HWID identification is missing — check 'Send device identification' in Settings")
	default:
		b.WriteString("subscription provider returned empty body with announce header")
	}
	if a.URL != "" {
		b.WriteString("  →  ")
		b.WriteString(a.URL)
	}
	return b.String()
}

// FetchSubscriptionWithMeta — расширенная версия FetchSubscription,
// возвращающая raw body, decoded body и распарсенные header-derived
// поля subscription metadata.
//
// Производит:
//
//  1. HTTP GET с timeout / User-Agent SubscriptionParserClient;
//  2. чтение тела с лимитом MaxSubscriptionResponseSize;
//  3. ParseHeaders(resp.Header) → meta (header-derived поля);
//  4. ParseInlineComments(rawBody) → fallback meta;
//  5. MergeMeta(headers_meta, inline_meta);
//  6. DecodeSubscriptionContent(rawBody) → Body (base64 strip etc.);
//
// На любой ошибке (network/HTTP/decode) возвращает (*FetchResult с
// HTTPStatus заполненным если был ответ, без Body, без Meta) + error.
func FetchSubscriptionWithMeta(url string) (*FetchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	client := newHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	applySubscriptionRequestHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		if IsNetworkErrorFunc != nil && IsNetworkErrorFunc(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessageFunc(err))
		}
		return nil, fmt.Errorf("fetch subscription: %w", err)
	}
	defer func() {
		debuglog.RunAndLog("FetchSubscriptionWithMeta: close body", resp.Body.Close)
	}()

	result := &FetchResult{HTTPStatus: resp.StatusCode}

	if resp.StatusCode != http.StatusOK {
		httpErr := &FetchHTTPError{
			StatusCode: resp.StatusCode,
			Hint:       explainHTTPStatus(resp.StatusCode),
		}
		// SPEC 061: announce headers on non-200 (e.g. 403 + "region blocked,
		// see @bot"). Attach so UI can render the actionable dialog instead
		// of a flat "HTTP 403".
		if a := ParseAnnounce(resp.Header); !a.IsEmpty() {
			ac := a
			httpErr.Announce = &ac
		}
		return result, httpErr
	}

	limited := io.LimitReader(resp.Body, MaxSubscriptionResponseSize+1)
	rawBody, err := io.ReadAll(limited)
	if err != nil {
		return result, fmt.Errorf("read body: %w", err)
	}
	if len(rawBody) == 0 {
		// Provider gate: HTTP 200 + 0 байт + announce headers → не «пустая
		// подписка», а явное сообщение от провайдера (HWID-лимит, expired,
		// region-block, …). Сохраняем как FetchAnnounceError чтобы UI мог
		// показать декодированный текст + кликабельный URL вместо плоского
		// «empty subscription body».
		if a := ParseAnnounce(resp.Header); !a.IsEmpty() {
			return result, &FetchAnnounceError{Announce: a}
		}
		return result, fmt.Errorf("empty subscription body")
	}
	if int64(len(rawBody)) > MaxSubscriptionResponseSize {
		return result, fmt.Errorf("subscription body exceeds %d bytes", MaxSubscriptionResponseSize)
	}
	result.RawBody = rawBody
	result.RawBodyBytes = int64(len(rawBody))

	// Headers + inline + merge.
	headersMeta := ParseHeaders(resp.Header)
	inlineMeta := ParseInlineComments(rawBody)
	result.Meta = MergeMeta(headersMeta, inlineMeta)

	// Decoded body для парсера.
	decoded, decErr := DecodeSubscriptionContent(rawBody)
	if decErr != nil {
		return result, fmt.Errorf("decode subscription: %w", decErr)
	}
	result.Body = decoded
	return result, nil
}

// newHTTPClient — общая фабрика HTTP-клиента для подписок.
// Использует CreateHTTPClientFunc если задан (обходит system proxy
// настройки лаунчера), иначе fallback на дефолтный.
func newHTTPClient() *http.Client {
	if CreateHTTPClientFunc != nil {
		return CreateHTTPClientFunc(NetworkRequestTimeout)
	}
	return &http.Client{
		Timeout: NetworkRequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
}

// IsHTTPError — convenience-обёртка для callsite'ов, чтобы вытащить
// StatusCode из ошибки FetchSubscriptionWithMeta для записи в meta.
func IsHTTPError(err error) (*FetchHTTPError, bool) {
	var httpErr *FetchHTTPError
	if errors.As(err, &httpErr) {
		return httpErr, true
	}
	return nil, false
}

// IsAnnounceError — convenience-обёртка чтобы UI/CLI могли вытащить
// провайдерский announce + URL из ошибки и нарисовать actionable дialog
// (кликабельный URL, кнопка «открыть бота») вместо плоского текста.
func IsAnnounceError(err error) (*FetchAnnounceError, bool) {
	var ae *FetchAnnounceError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// FetchSubscription fetches subscription content from URL and decodes it.
//
// Deprecated: use FetchSubscriptionWithMeta — оно возвращает meta и
// raw body, нужные для SPEC 052 (cache + per-source metadata).
// Старый wrapper сохранён для backward-compat callsite'ов; будет удалён
// после Phase 7 cleanup.
func FetchSubscription(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	// Use simple HTTP client if CreateHTTPClientFunc not set
	var client *http.Client
	if CreateHTTPClientFunc != nil {
		client = CreateHTTPClientFunc(NetworkRequestTimeout)
	} else {
		client = &http.Client{
			Timeout: NetworkRequestTimeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// SPEC 061 §"Request headers": singbox-launcher/<v> (<os> <arch>) UA +
	// X-Hwid-family (when the user hasn't opted out via Settings).
	applySubscriptionRequestHeaders(req)

	resp, err := client.Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("FetchSubscription: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		if IsNetworkErrorFunc != nil && IsNetworkErrorFunc(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessageFunc(err))
		}
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription server returned status %d (%s)", resp.StatusCode, explainHTTPStatus(resp.StatusCode))
	}

	// Limit response size to prevent memory exhaustion
	const maxResponseSize = 10 * 1024 * 1024 // 10 MB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)

	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("FetchSubscription: failed to read subscription content: %w", err)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("FetchSubscription: subscription returned empty content")
	}

	// Check if content was truncated (exceeds max size)
	if len(content) > maxResponseSize {
		return nil, fmt.Errorf("FetchSubscription: subscription content too large (exceeds %d bytes)", maxResponseSize)
	}

	// Log preview of raw content for debugging
	const previewLen = 200
	preview := string(content)
	if len(preview) > previewLen {
		preview = preview[:previewLen] + "..."
	}
	debuglog.DebugLog("[DEBUG] FetchSubscription: Raw content preview (first %d bytes): %q", len(content), preview)

	// Use DecodeSubscriptionContent for decoding
	decoded, err := DecodeSubscriptionContent(content)
	if err != nil {
		return nil, fmt.Errorf("FetchSubscription: failed to decode subscription content: %w", err)
	}

	return decoded, nil
}
