package subscription

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// uuidRE — relaxed UUIDv4 form check (8-4-4-4-12 hex). We don't validate
// version/variant bits because tests can override via fakeSubscriptionSettings.
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// withSubscriptionSettings swaps LoadSubscriptionSettingsFunc for one test and
// restores it on cleanup. Several tests verify request-side headers, so the
// helper avoids each one having to do the boilerplate save+defer dance.
func withSubscriptionSettings(t *testing.T, s SubscriptionRequestSettings) {
	t.Helper()
	prev := LoadSubscriptionSettingsFunc
	LoadSubscriptionSettingsFunc = func() SubscriptionRequestSettings { return s }
	t.Cleanup(func() { LoadSubscriptionSettingsFunc = prev })
}

// TestFetchSubscriptionWithMeta_HappyPath — V2Board-like response с headers + body.
func TestFetchSubscriptionWithMeta_HappyPath(t *testing.T) {
	body := "vless://uuid@host:443#tokyo\nvless://uuid@host:443#fra\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Errorf("missing User-Agent")
		}
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=1717171717")
		w.Header().Set("Profile-Title", "TestSub")
		w.Header().Set("Profile-Update-Interval", "12")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res, err := FetchSubscriptionWithMeta(srv.URL)
	if err != nil {
		t.Fatalf("FetchSubscriptionWithMeta: %v", err)
	}
	if res.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus = %d, want 200", res.HTTPStatus)
	}
	if string(res.RawBody) != body {
		t.Errorf("RawBody mismatch: got %q, want %q", res.RawBody, body)
	}
	if res.RawBodyBytes != int64(len(body)) {
		t.Errorf("RawBodyBytes = %d, want %d", res.RawBodyBytes, len(body))
	}
	if res.Meta.ProfileTitle != "TestSub" {
		t.Errorf("ProfileTitle = %q", res.Meta.ProfileTitle)
	}
	if res.Meta.UserInfo == nil || res.Meta.UserInfo.TotalBytes != 100 {
		t.Errorf("UserInfo: %+v", res.Meta.UserInfo)
	}
	if res.Meta.ProfileUpdateIntervalHours != 12 {
		t.Errorf("ProfileUpdateIntervalHours = %d", res.Meta.ProfileUpdateIntervalHours)
	}
	// Body — decoded (для plain text это identity).
	if !strings.Contains(string(res.Body), "vless://uuid@host:443#tokyo") {
		t.Errorf("Body missing nodes: %q", res.Body)
	}
}

// TestFetchSubscriptionWithMeta_InlineFallback — headers пустые, метаданные
// идут из #-comments внутри body.
func TestFetchSubscriptionWithMeta_InlineFallback(t *testing.T) {
	body := "#profile-title: Inline\n" +
		"#subscription-userinfo: upload=1; download=2; total=3; expire=4\n" +
		"vless://uuid@host:443#tokyo\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res, err := FetchSubscriptionWithMeta(srv.URL)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Meta.ProfileTitle != "Inline" {
		t.Errorf("ProfileTitle = %q, want Inline", res.Meta.ProfileTitle)
	}
	if res.Meta.UserInfo == nil || res.Meta.UserInfo.TotalBytes != 3 {
		t.Errorf("UserInfo: %+v", res.Meta.UserInfo)
	}
}

// TestFetchSubscriptionWithMeta_HTTPError — non-200 status → FetchHTTPError.
func TestFetchSubscriptionWithMeta_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	res, err := FetchSubscriptionWithMeta(srv.URL)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var httpErr *FetchHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *FetchHTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want 403", httpErr.StatusCode)
	}
	if res == nil || res.HTTPStatus != http.StatusForbidden {
		t.Errorf("result HTTPStatus: %+v", res)
	}

	if extracted, ok := IsHTTPError(err); !ok || extracted.StatusCode != 403 {
		t.Errorf("IsHTTPError extraction failed")
	}
}

// TestFetchSubscriptionWithMeta_EmptyBody — пустое тело → ошибка.
func TestFetchSubscriptionWithMeta_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := FetchSubscriptionWithMeta(srv.URL)
	if err == nil {
		t.Fatalf("expected empty-body error")
	}
	// No announce headers → plain "empty subscription body" error,
	// NOT a FetchAnnounceError.
	if _, ok := IsAnnounceError(err); ok {
		t.Errorf("unexpected FetchAnnounceError for empty body without announce headers")
	}
}

// TestFetchSubscriptionWithMeta_AnnounceError — provider gate: HTTP 200 +
// empty body + announce headers → wraps the announce info into a
// FetchAnnounceError so UI/CLI can render the provider's message + URL.
func TestFetchSubscriptionWithMeta_AnnounceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Announce", "base64:0JLRiyDQtNC+0YHRgtC40LPQvdGD0LvQuCDQu9C40LzQuNGC0LAg0YPRgdGC0YDQvtC50YHRgtCyIQ==")
		w.Header().Set("Announce-Url", "https://t.me/nash_vpn_bot")
		w.Header().Set("X-Hwid-Limit", "true")
		w.Header().Set("Profile-Title", "base64:TmFzaFZQTg==")
		w.WriteHeader(http.StatusOK)
		// no body
	}))
	defer srv.Close()

	_, err := FetchSubscriptionWithMeta(srv.URL)
	if err == nil {
		t.Fatalf("expected error")
	}
	ae, ok := IsAnnounceError(err)
	if !ok {
		t.Fatalf("expected FetchAnnounceError, got %T: %v", err, err)
	}
	if !ae.Announce.HWIDLimit {
		t.Errorf("HWIDLimit = false")
	}
	if ae.Announce.URL != "https://t.me/nash_vpn_bot" {
		t.Errorf("URL = %q", ae.Announce.URL)
	}
	if ae.Announce.ProfileTitle != "NashVPN" {
		t.Errorf("ProfileTitle = %q", ae.Announce.ProfileTitle)
	}
	if !strings.HasPrefix(ae.Announce.Message, "Вы достиг") {
		t.Errorf("Message decoded prefix wrong: %q", ae.Announce.Message)
	}
	// Error() should contain the title, message preview and URL so a flat
	// UI label still surfaces something useful.
	msg := err.Error()
	for _, want := range []string{"NashVPN", "Вы достиг", "https://t.me/nash_vpn_bot"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing substring %q", msg, want)
		}
	}
	// errors.As + sentinel errors.Is plumbing — defensive smoke.
	var sentinel *FetchAnnounceError
	if !errors.As(err, &sentinel) {
		t.Errorf("errors.As(*FetchAnnounceError) failed")
	}
}

// TestFetchSubscription_UserAgentFormat — SPEC 061 Phase 2: UA должно быть
// `LxBox/<v> (desktop; <os>)`, не legacy `SubscriptionParserClient`. Bare
// `singbox` (без дефиса) недопустим: панели, матчащие UA по подстроке, иначе
// принимают его за не-sing-box клиента и отдают JSON-конфиг вместо списка
// подписки (см. BuildSubscriptionUserAgent).
func TestFetchSubscription_UserAgentFormat(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://u@h:443#a\n"))
	}))
	defer srv.Close()

	if _, err := FetchSubscriptionWithMeta(srv.URL); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.HasPrefix(gotUA, "LxBox/") {
		t.Errorf("UA = %q, want prefix LxBox/", gotUA)
	}
	if strings.Contains(gotUA, "singbox") {
		t.Errorf("UA = %q, must not contain bare 'singbox' (panels mis-route it)", gotUA)
	}
	if !strings.Contains(gotUA, "desktop") {
		t.Errorf("UA = %q, want 'desktop' variant tag", gotUA)
	}
	if !strings.Contains(gotUA, "(") || !strings.Contains(gotUA, ")") {
		t.Errorf("UA = %q, want platform suffix in parens", gotUA)
	}
}

// TestFetchSubscription_HWIDHeadersRoundTrip — when settings opt in to send
// HWID (default), all 4 X-* headers land on the wire with plausible values.
func TestFetchSubscription_HWIDHeadersRoundTrip(t *testing.T) {
	const fakeHWID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	withSubscriptionSettings(t, SubscriptionRequestSettings{
		HWID:              fakeHWID,
		SendHWID:          true,
		DeviceModelHashed: false,
	})

	var gotHWID, gotOS, gotVer, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHWID = r.Header.Get("X-Hwid")
		gotOS = r.Header.Get("X-Device-Os") // Go canonicalizes to Title-case
		gotVer = r.Header.Get("X-Ver-Os")
		gotModel = r.Header.Get("X-Device-Model")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://u@h:443#a\n"))
	}))
	defer srv.Close()

	if _, err := FetchSubscriptionWithMeta(srv.URL); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !uuidRE.MatchString(gotHWID) {
		t.Errorf("X-Hwid = %q, want UUIDv4 form", gotHWID)
	}
	switch gotOS {
	case "macOS", "windows", "linux":
		// ok
	default:
		t.Errorf("X-Device-OS = %q, want macOS/windows/linux", gotOS)
	}
	if gotVer == "" {
		t.Errorf("X-Ver-OS empty")
	}
	if gotModel == "" {
		t.Errorf("X-Device-Model empty")
	}
}

// TestFetchSubscription_HWIDHeadersDisabled — explicit opt-out: none of the
// 4 X-Hwid-family headers may appear (provider must see request as
// "anonymous" — same fingerprint as a static-page fetcher).
func TestFetchSubscription_HWIDHeadersDisabled(t *testing.T) {
	withSubscriptionSettings(t, SubscriptionRequestSettings{
		HWID:     "any",
		SendHWID: false,
	})

	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://u@h:443#a\n"))
	}))
	defer srv.Close()

	if _, err := FetchSubscriptionWithMeta(srv.URL); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, h := range []string{"X-Hwid", "X-Device-Os", "X-Ver-Os", "X-Device-Model"} {
		if v := headers.Get(h); v != "" {
			t.Errorf("header %q leaked on opt-out: %q", h, v)
		}
	}
}

// TestFetchSubscription_HTTPErrorWithAnnounce — SPEC 061: non-200 + announce
// headers (e.g. 403 + "region blocked, see @bot") attaches the parsed
// announce to FetchHTTPError and Error() includes the message + URL.
func TestFetchSubscription_HTTPErrorWithAnnounce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Announce", "Region blocked. Contact support.")
		w.Header().Set("Announce-Url", "https://t.me/support_bot")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := FetchSubscriptionWithMeta(srv.URL)
	if err == nil {
		t.Fatalf("expected error")
	}
	httpErr, ok := IsHTTPError(err)
	if !ok {
		t.Fatalf("expected *FetchHTTPError, got %T", err)
	}
	if httpErr.Announce == nil {
		t.Fatalf("Announce nil on 403 with announce headers")
	}
	if httpErr.Announce.Message != "Region blocked. Contact support." {
		t.Errorf("Message = %q", httpErr.Announce.Message)
	}
	if httpErr.Announce.URL != "https://t.me/support_bot" {
		t.Errorf("URL = %q", httpErr.Announce.URL)
	}
	msg := err.Error()
	for _, want := range []string{"403", "Region blocked", "https://t.me/support_bot"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

// TestFetchSubscription_HWIDHeadersHashedModel — hashed mode replaces the
// raw model string with sha256(model)[:16] = exactly 16 lowercase hex chars.
func TestFetchSubscription_HWIDHeadersHashedModel(t *testing.T) {
	withSubscriptionSettings(t, SubscriptionRequestSettings{
		HWID:              "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		SendHWID:          true,
		DeviceModelHashed: true,
	})

	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotModel = r.Header.Get("X-Device-Model")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vless://u@h:443#a\n"))
	}))
	defer srv.Close()

	if _, err := FetchSubscriptionWithMeta(srv.URL); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(gotModel) != 16 {
		t.Errorf("hashed X-Device-Model = %q (len %d), want 16 hex chars", gotModel, len(gotModel))
	}
	for _, c := range gotModel {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("hashed model has non-hex byte %q in %q", c, gotModel)
			break
		}
	}
}
