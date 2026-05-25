package subscription

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
