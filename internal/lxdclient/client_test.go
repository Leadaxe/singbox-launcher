//go:build darwin

package lxdclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// httptest-сервер имитирует admin-плоскость демона без TLS (Config без пина),
// чтобы проверить маппинг кодов и разбор ошибок REST-клиента.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// httptest даёт http://127.0.0.1:PORT; клиент строит http:// когда пина нет.
	addr := srv.Listener.Addr().String()
	return New(Config{Addr: addr})
}

func TestStatusDecode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/status" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","active_sha256":"abc","last_good_sha256":"def","last_error":"","interrupted_apply":false}`))
	})
	info, err := c.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if info.Status != "started" || info.ActiveSHA != "abc" || info.LastGoodSHA != "def" {
		t.Fatalf("bad status decode: %+v", info)
	}
}

func TestApplyOK(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"applied":true,"active_sha256":"abc"}`))
	})
	if err := c.Apply([]byte(`{"log":{}}`)); err != nil {
		t.Fatalf("apply ok: %v", err)
	}
}

func TestApplyRejected422(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"applied":false,"rolled_back":false,"error":"bad config"}`))
	})
	err := c.Apply([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	applyErr, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("want *ApplyError, got %T", err)
	}
	if !applyErr.Rejected() {
		t.Error("Rejected() = false for 422")
	}
	if applyErr.RolledBack {
		t.Error("RolledBack = true for a plain reject")
	}
	if applyErr.Message != "bad config" {
		t.Errorf("message = %q", applyErr.Message)
	}
}

func TestApplyFailedRolledBack500(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"applied":false,"rolled_back":true,"error":"start failed"}`))
	})
	err := c.Apply([]byte(`{}`))
	applyErr, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("want *ApplyError, got %T (%v)", err, err)
	}
	if applyErr.Rejected() {
		t.Error("500 must not be Rejected()")
	}
	if !applyErr.RolledBack {
		t.Error("RolledBack = false for rolled_back:true")
	}
}

func TestStartStopRollbackCodes(t *testing.T) {
	// 404 на Start → ошибка (нет last-good).
	c404 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no config"}`))
	})
	if err := c404.Start(); err == nil {
		t.Error("Start on 404 must error")
	}
	// 200 → ok.
	c200 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stopped":true}`))
	})
	if err := c200.Stop(); err != nil {
		t.Errorf("Stop on 200: %v", err)
	}
}

func TestBearerHeaderSent(t *testing.T) {
	got := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"stopped":true}`))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{Addr: srv.Listener.Addr().String(), Secret: "sekret"})
	_ = c.Stop()
	if got != "Bearer sekret" {
		t.Fatalf("Authorization header = %q, want Bearer sekret", got)
	}
}

func TestActiveConfigEcho(t *testing.T) {
	body := `{"log":{"level":"info"}}`
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	got, err := c.ActiveConfig()
	if err != nil {
		t.Fatalf("active config: %v", err)
	}
	if string(got) != body {
		t.Fatalf("active config = %q, want %q", got, body)
	}
}

func TestTLSEnabledPredicate(t *testing.T) {
	if (Config{}).TLSEnabled() {
		t.Error("empty config should not enable TLS")
	}
	if !(Config{ServerFingerprint: "abc"}).TLSEnabled() {
		t.Error("fingerprint should enable TLS")
	}
}

func TestInfoDecode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/info" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.14.0-lx.23","state_dir":"/var/lib/lxd/state","listen":"127.0.0.1:9091","tls":true,"fingerprint":"ff00","pid":42,"uptime_seconds":7,"log_path":"/var/lib/lxd/lxd.log"}`))
	})
	info, err := c.Info()
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Version != "1.14.0-lx.23" || info.StateDir != "/var/lib/lxd/state" || !info.TLS || info.PID != 42 {
		t.Fatalf("bad info decode: %+v", info)
	}
}

func TestDetectChannel(t *testing.T) {
	// Plain HTTP-сервер (даже 401 — это валидный HTTP-ответ) → ChannelPlain.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(plain.Close)
	if got := DetectChannel(plain.Listener.Addr().String()); got != ChannelPlain {
		t.Fatalf("plain server detected as %v, want ChannelPlain", got)
	}

	// TLS-сервер: plain-запрос обрывается без HTTP-ответа → ChannelTLS.
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(tlsSrv.Close)
	if got := DetectChannel(tlsSrv.Listener.Addr().String()); got != ChannelTLS {
		t.Fatalf("tls server detected as %v, want ChannelTLS", got)
	}

	// Мёртвый порт → ChannelUnknown.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadAddr := dead.Listener.Addr().String()
	dead.Close()
	if got := DetectChannel(deadAddr); got != ChannelUnknown {
		t.Fatalf("dead port detected as %v, want ChannelUnknown", got)
	}
}
