package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig кладёт config.json во временный каталог. Фикстуры —
// синтетические: ни реальных узлов, ни секретов пользователя.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadClashAPIConfig_States — вердикт разбора experimental.clash_api.
// Пустой secret — это состояние «без аутентификации», а не ошибка
// конфигурации: ядро поднимает такой API на петле (SPEC 143).
func TestLoadClashAPIConfig_States(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		wantState APIConfigState
		wantURL   string
		wantToken string
		wantAuth  bool
		wantErr   bool
	}{
		{
			name:      "secret present",
			config:    `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","secret":"s3cr3t-value"}}}`,
			wantState: APIConfigOK,
			wantURL:   "http://127.0.0.1:9090",
			wantToken: "s3cr3t-value",
			wantAuth:  true,
		},
		{
			name:      "empty secret on loopback is no-auth, not an error",
			config:    `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090"}}}`,
			wantState: APIConfigNoAuth,
			wantURL:   "http://127.0.0.1:9090",
			wantToken: "",
			wantAuth:  false,
		},
		{
			name:      "explicit empty secret string",
			config:    `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","secret":""}}}`,
			wantState: APIConfigNoAuth,
			wantURL:   "http://127.0.0.1:9090",
			wantAuth:  false,
		},
		{
			name:      "empty secret on localhost name",
			config:    `{"experimental":{"clash_api":{"external_controller":"localhost:9090"}}}`,
			wantState: APIConfigNoAuth,
			wantURL:   "http://localhost:9090",
			wantAuth:  false,
		},
		{
			name:      "empty secret on IPv6 loopback",
			config:    `{"experimental":{"clash_api":{"external_controller":"[::1]:9090"}}}`,
			wantState: APIConfigNoAuth,
			wantURL:   "http://[::1]:9090",
			wantAuth:  false,
		},
		{
			// Без secret наружу выставлять нельзя: состояние «без
			// аутентификации» остаётся, но пользователь получает
			// предупреждение, а не молчаливое отключение.
			name:      "empty secret but not loopback",
			config:    `{"experimental":{"clash_api":{"external_controller":"0.0.0.0:9090"}}}`,
			wantState: APIConfigNoAuth,
			wantURL:   "http://0.0.0.0:9090",
			wantAuth:  false,
		},
		{
			name:      "no clash_api section",
			config:    `{"experimental":{"cache_file":{"enabled":true}}}`,
			wantState: APIConfigMissing,
		},
		{
			name:      "no experimental section",
			config:    `{"outbounds":[]}`,
			wantState: APIConfigMissing,
		},
		{
			name:      "empty external_controller",
			config:    `{"experimental":{"clash_api":{"external_controller":""}}}`,
			wantState: APIConfigMissing,
		},
		{
			// Адрес без порта не примет http.Client — это ошибка адреса,
			// её надо показать, а не считать «не настроено».
			name:      "controller without port",
			config:    `{"experimental":{"clash_api":{"external_controller":"127.0.0.1"}}}`,
			wantState: APIConfigInvalid,
		},
		{
			name:      "controller with url scheme",
			config:    `{"experimental":{"clash_api":{"external_controller":"http://127.0.0.1:9090"}}}`,
			wantState: APIConfigInvalid,
		},
		{
			// jsonc: комментарии — штатный формат конфига, они снимаются
			// до json.Unmarshal.
			name:      "jsonc with comments",
			config:    "{\n// clash api\n\"experimental\":{\"clash_api\":{\"external_controller\":\"127.0.0.1:9090\"}}\n}",
			wantState: APIConfigNoAuth,
			wantURL:   "http://127.0.0.1:9090",
		},
		{
			// Известное ограничение зависимости: jsonc.ToJSON снимает
			// комментарии, но оставляет висячие запятые, поэтому config с
			// хвостовой запятой не разбирается. Фиксируем как есть — это
			// отдельная тема, не часть SPEC 143.
			name:      "trailing comma is not repaired by jsonc",
			config:    `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090"},},}`,
			wantState: APIConfigUnreadable,
			wantErr:   true,
		},
		{
			name:      "unparseable json",
			config:    `{"experimental":`,
			wantState: APIConfigUnreadable,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			cfg, err := LoadClashAPIConfig(path)
			if tt.wantErr && err == nil {
				t.Fatalf("want error, got state %q", cfg.State)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.State != tt.wantState {
				t.Fatalf("state = %q, want %q (detail %q)", cfg.State, tt.wantState, cfg.Detail)
			}
			if cfg.BaseURL != tt.wantURL {
				t.Fatalf("baseURL = %q, want %q", cfg.BaseURL, tt.wantURL)
			}
			if cfg.Token != tt.wantToken {
				t.Fatalf("token = %q, want %q", cfg.Token, tt.wantToken)
			}
			if cfg.RequiresAuth() != tt.wantAuth {
				t.Fatalf("RequiresAuth = %v, want %v", cfg.RequiresAuth(), tt.wantAuth)
			}
			if got := cfg.Enabled(); got != tt.wantState.Enabled() {
				t.Fatalf("Enabled = %v, want %v", got, tt.wantState.Enabled())
			}
		})
	}
}

// TestLoadClashAPIConfig_MissingFile — отсутствующий config.json (холодный
// старт) — не паника и не «валидный API».
func TestLoadClashAPIConfig_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	cfg, err := LoadClashAPIConfig(path)
	if err == nil {
		t.Fatalf("want error for a missing file, got state %q", cfg.State)
	}
	if cfg.State != APIConfigUnreadable {
		t.Fatalf("state = %q, want %q", cfg.State, APIConfigUnreadable)
	}
	if cfg.Enabled() {
		t.Fatal("a missing config must not enable the API")
	}
}

// TestSetAuthHeader — пустой token ⇒ заголовка Authorization нет вовсе.
// Отправлять `Bearer ` с пустым токеном нельзя: это неверный запрос и
// ложный след в логе ядра.
func TestSetAuthHeader(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantValue string
		wantSet   bool
	}{
		{name: "empty token omits the header", token: "", wantSet: false},
		{name: "non-empty token sets Bearer", token: "abc123", wantValue: "Bearer abc123", wantSet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			SetAuthHeader(h, tt.token)
			got := h.Get("Authorization")
			if tt.wantSet {
				if got != tt.wantValue {
					t.Fatalf("Authorization = %q, want %q", got, tt.wantValue)
				}
				return
			}
			if got != "" {
				t.Fatalf("Authorization must be absent for an empty token, got %q", got)
			}
			if _, present := h["Authorization"]; present {
				t.Fatal("Authorization key must not be present at all for an empty token")
			}
		})
	}
}

// TestTestAPIConnection_AuthFailure — 401/403 от ядра — это ErrAPIAuth,
// отличимое от «соединение отвергнуто» и от «неверный адрес».
func TestTestAPIConnection_AuthFailure(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		wantAuth   bool
		wantErrAny bool
	}{
		{name: "200 is success", status: http.StatusOK},
		{name: "401 is an auth error", status: http.StatusUnauthorized, wantAuth: true, wantErrAny: true},
		{name: "403 is an auth error", status: http.StatusForbidden, wantAuth: true, wantErrAny: true},
		{name: "500 is not an auth error", status: http.StatusInternalServerError, wantErrAny: true},
		{name: "404 is not an auth error", status: http.StatusNotFound, wantErrAny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/version" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				if tt.status == http.StatusOK {
					_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.15.0-test"})
				}
			}))
			defer srv.Close()

			err := TestAPIConnection(srv.URL, "some-secret")
			if tt.wantErrAny && err == nil {
				t.Fatal("want an error")
			}
			if !tt.wantErrAny && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if IsAuthError(err) != tt.wantAuth {
				t.Fatalf("IsAuthError(%v) = %v, want %v", err, IsAuthError(err), tt.wantAuth)
			}
		})
	}
}

// TestTestAPIConnection_EmptySecretSendsNoAuth — при пустом secret запрос
// уходит без Authorization, и API без аутентификации отвечает 200.
// Это и есть сценарий, который раньше был недостижим: API отключался
// ещё на разборе конфига.
func TestTestAPIConnection_EmptySecretSendsNoAuth(t *testing.T) {
	var sawAuth string
	var sawAuthPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		_, sawAuthPresent = r.Header["Authorization"]
		// Ядро без secret: запрос без заголовка принимается, с заголовком — 401.
		if sawAuthPresent {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"1.15.0-test"}`))
	}))
	defer srv.Close()

	if err := TestAPIConnection(srv.URL, ""); err != nil {
		t.Fatalf("unauthenticated loopback API must be reachable: %v", err)
	}
	if sawAuthPresent {
		t.Fatalf("no Authorization header must be sent for an empty secret, got %q", sawAuth)
	}
	_ = sawAuth
}

// TestGetProxiesInGroup_EmptySecretSendsNoAuth — тот же контракт на пути
// списка узлов (именно он наполняет список в UI).
func TestGetProxiesInGroup_EmptySecretSendsNoAuth(t *testing.T) {
	var authPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, authPresent = r.Header["Authorization"]
		if authPresent {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"proxies":{"proxy-out":{"type":"Selector","now":"node-a","all":["node-a","node-b"]}}}`))
	}))
	defer srv.Close()

	proxies, now, err := GetProxiesInGroup(srv.URL, "", "proxy-out")
	if err != nil {
		t.Fatalf("GetProxiesInGroup with an empty secret: %v", err)
	}
	if authPresent {
		t.Fatal("no Authorization header must be sent for an empty secret")
	}
	// Второе значение — выбранный сейчас узел, а не имя группы.
	if now != "node-a" {
		t.Fatalf("selected proxy = %q, want node-a", now)
	}
	if len(proxies) != 2 {
		t.Fatalf("got %d proxies, want 2", len(proxies))
	}
}
