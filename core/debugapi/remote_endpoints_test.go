package debugapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// newRemoteTestServer поднимает Debug API с remote-группой поверх реестра в
// temp-каталоге. Возвращает base URL и execDir для подкладывания файлов.
func newRemoteTestServer(t *testing.T) (base, execDir string, srv *Server) {
	t.Helper()
	execDir = t.TempDir()
	registry := services.NewRemoteRegistry(execDir)
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{}, port, "remote-test-token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.EnableRemote(&RemoteAPI{
		Registry: registry,
		Pool:     services.NewTransportPool(registry),
		ExecDir:  execDir,
	})
	s.Start()
	t.Cleanup(s.Stop)
	return "http://127.0.0.1:" + itoa(port), execDir, s
}

// authDo — авторизованный запрос к серверу-под-тестом.
func authDo(t *testing.T, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer remote-test-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(resp.Body)
	return resp, data.Bytes()
}

// seedMachine кладёт запись машины в реестр напрямую (Pair требует живого
// enroll — в тестах канал заменяет httptest-демон, доверие не нужно).
func seedMachine(t *testing.T, execDir, id, addr string) {
	t.Helper()
	binDir := platform.GetBinDir(execDir)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	raw := fmt.Sprintf(`[{"id":%q,"name":%q,"addr":%q}]`, id, id, addr)
	if err := os.WriteFile(filepath.Join(binDir, "remote-daemons.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}

func TestRemoteGroupCapabilitiesAndRegistry(t *testing.T) {
	base, _, _ := newRemoteTestServer(t)

	// Манифест объявляет remote-группу, daemon выключен.
	resp, body := authDo(t, http.MethodGet, base+"/", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("manifest: status %d", resp.StatusCode)
	}
	var manifest struct {
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("manifest parse: %v", err)
	}
	if !manifest.Capabilities["remote"] {
		t.Errorf("capabilities.remote = false, want true")
	}
	if manifest.Capabilities["daemon"] {
		t.Errorf("capabilities.daemon = true, want false (not enabled)")
	}

	// Пустой реестр — пустой список.
	resp, body = authDo(t, http.MethodGet, base+"/remote/machines", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("machines: status %d (%s)", resp.StatusCode, body)
	}
	var list struct {
		Machines []machineView `json:"machines"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("machines parse: %v", err)
	}
	if len(list.Machines) != 0 {
		t.Errorf("machines: %d entries, want 0", len(list.Machines))
	}

	// Неизвестная машина — 404 на любом её endpoint'е.
	for _, path := range []string{
		"/remote/machines/ghost",
		"/remote/machines/ghost/health",
		"/remote/machines/ghost/state/full",
	} {
		resp, _ = authDo(t, http.MethodGet, base+path, nil)
		if resp.StatusCode != 404 {
			t.Errorf("GET %s: status %d, want 404", path, resp.StatusCode)
		}
	}

	// Битое приглашение — 400 (вина запроса), а не 5xx «машина недоступна».
	resp, body = authDo(t, http.MethodPost, base+"/remote/machines",
		map[string]any{"invite": "not-an-invite", "name": "x"})
	if resp.StatusCode != 400 {
		t.Errorf("pair with bad invite: status %d (%s), want 400", resp.StatusCode, body)
	}
}

// fakeDaemonMux — admin-плоскость демона для тестов (plain h2c).
func fakeDaemonMux(applied *[]byte) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"started","active_sha256":"aaa","last_good_sha256":"aaa"}`))
	})
	mux.HandleFunc("/admin/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"test","state_dir":"/tmp/lxd-test"}`))
	})
	mux.HandleFunc("/admin/resources", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resources":[]}`))
	})
	mux.HandleFunc("/admin/apply", func(w http.ResponseWriter, r *http.Request) {
		raw := new(bytes.Buffer)
		_, _ = raw.ReadFrom(r.Body)
		if applied != nil {
			*applied = raw.Bytes()
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func TestRemoteRawRESTHealthAndDeploy(t *testing.T) {
	var applied []byte
	daemon := httptest.NewServer(fakeDaemonMux(&applied))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)

	// Health — машина отвечает, паспорт закеширован.
	resp, body := authDo(t, http.MethodGet, base+"/remote/machines/router/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("health: status %d (%s)", resp.StatusCode, body)
	}
	var health struct {
		Reachable  bool   `json:"reachable"`
		CoreStatus string `json:"core_status"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("health parse: %v", err)
	}
	if !health.Reachable || health.CoreStatus != "started" {
		t.Errorf("health = %+v, want reachable started", health)
	}

	// Raw REST passthrough: статус и тело демона доезжают как данные.
	resp, body = authDo(t, http.MethodPost, base+"/remote/machines/router/raw/rest",
		map[string]any{"method": "GET", "path": "/admin/status"})
	if resp.StatusCode != 200 {
		t.Fatalf("raw rest: status %d (%s)", resp.StatusCode, body)
	}
	var raw struct {
		Status int `json:"status"`
		Body   struct {
			Status string `json:"status"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("raw rest parse: %v (%s)", err, body)
	}
	if raw.Status != 200 || raw.Body.Status != "started" {
		t.Errorf("raw rest = %+v, want daemon 200/started", raw)
	}

	// Raw REST: путь не с "/" — 400.
	resp, _ = authDo(t, http.MethodPost, base+"/remote/machines/router/raw/rest",
		map[string]any{"method": "GET", "path": "admin/status"})
	if resp.StatusCode != 400 {
		t.Errorf("raw rest bad path: status %d, want 400", resp.StatusCode)
	}

	// Deploy без собранного конфига — 404 c подсказкой про Configure.
	resp, body = authDo(t, http.MethodPost, base+"/remote/machines/router/deploy", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("deploy w/o config: status %d (%s), want 404", resp.StatusCode, body)
	}

	// Кладём собранный конфиг машины и деплоим.
	cfgPath := platform.GetRemoteConfigPathFor(execDir, "router")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir machine dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"log":{"level":"warn"}}`), 0o644); err != nil {
		t.Fatalf("write built config: %v", err)
	}
	resp, body = authDo(t, http.MethodPost, base+"/remote/machines/router/deploy", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("deploy: status %d (%s)", resp.StatusCode, body)
	}
	var deploy struct {
		OK        bool   `json:"ok"`
		ConfigSHA string `json:"config_sha"`
	}
	if err := json.Unmarshal(body, &deploy); err != nil {
		t.Fatalf("deploy parse: %v", err)
	}
	if !deploy.OK || len(deploy.ConfigSHA) != 64 {
		t.Errorf("deploy = %+v, want ok + sha256", deploy)
	}
	if !bytes.Contains(applied, []byte(`"warn"`)) {
		t.Errorf("daemon received %q, want the built config", applied)
	}
}

func TestRemoteUIOverrideEndpoints(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	// Сервер без UI-хуков: группа отвечает 503, а не 404/500.
	base, execDir, srv := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)
	resp, _ := authDo(t, http.MethodGet, base+"/remote/ui", nil)
	if resp.StatusCode != 503 {
		t.Fatalf("ui state headless: %d, want 503", resp.StatusCode)
	}
	resp, _ = authDo(t, http.MethodPost, base+"/remote/machines/router/ui/connect", nil)
	if resp.StatusCode != 503 {
		t.Fatalf("ui connect headless: %d, want 503", resp.StatusCode)
	}

	// Подключаем фейковые хуки — «UI создан».
	var (
		connectedID string
		active      bool
	)
	srv.remote.UIConnect = func(id string) error { connectedID, active = id, true; return nil }
	srv.remote.UIDisconnect = func() error { connectedID, active = "", false; return nil }
	srv.remote.UIState = func() (string, string, bool, error) { return connectedID, connectedID, active, nil }

	// Недоступная машина: 502, override не тронут.
	seedMachine(t, execDir, "dead", "127.0.0.1:1")
	resp, body := authDo(t, http.MethodPost, base+"/remote/machines/dead/ui/connect", nil)
	if resp.StatusCode != 502 {
		t.Fatalf("connect dead: %d (%s), want 502", resp.StatusCode, body)
	}
	if active {
		t.Fatal("connect dead: override switched despite 502")
	}

	// seedMachine перезаписывает реестр целиком — вернём живую машину.
	seedMachine(t, execDir, "router", addr)
	resp, body = authDo(t, http.MethodPost, base+"/remote/machines/router/ui/connect", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("connect: %d (%s)", resp.StatusCode, body)
	}
	if connectedID != "router" || !active {
		t.Fatalf("hook state after connect = (%q,%v), want (router,true)", connectedID, active)
	}
	var conn struct {
		OK         bool   `json:"ok"`
		CoreStatus string `json:"core_status"`
	}
	if err := json.Unmarshal(body, &conn); err != nil || !conn.OK || conn.CoreStatus != "started" {
		t.Errorf("connect response = %s (err %v)", body, err)
	}

	// Состояние отражает подключение.
	resp, body = authDo(t, http.MethodGet, base+"/remote/ui", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("ui state: %d", resp.StatusCode)
	}
	var st struct {
		Connected bool   `json:"connected"`
		MachineID string `json:"machine_id"`
	}
	if err := json.Unmarshal(body, &st); err != nil || !st.Connected || st.MachineID != "router" {
		t.Errorf("ui state = %s (err %v), want connected router", body, err)
	}

	// Disconnect идемпотентен.
	for i := 0; i < 2; i++ {
		resp, _ = authDo(t, http.MethodPost, base+"/remote/ui/disconnect", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("disconnect #%d: %d", i+1, resp.StatusCode)
		}
	}
	if active || connectedID != "" {
		t.Errorf("hook state after disconnect = (%q,%v), want empty", connectedID, active)
	}
}

func TestRemoteMachineStateMirror(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)

	// Состояния ещё нет: GET — 404 (fresh machine), PATCH — тоже 404.
	resp, _ := authDo(t, http.MethodGet, base+"/remote/machines/router/state/full", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("state/full fresh: status %d, want 404", resp.StatusCode)
	}
	resp, _ = authDo(t, http.MethodPatch, base+"/remote/machines/router/state/rules",
		map[string]any{"mode": "replace", "rules": []any{}})
	if resp.StatusCode != 404 {
		t.Fatalf("patch rules fresh: status %d, want 404", resp.StatusCode)
	}

	// Кладём state машины и патчим его через API.
	statePath := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "router")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := state.New().Save(statePath); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	rule := map[string]any{
		"kind":    "inline",
		"enabled": true,
		"body":    map[string]any{"name": "t", "match": map[string]any{"domain_suffix": []string{"x.com"}}, "outbound": "reject"},
	}
	resp, body := authDo(t, http.MethodPatch, base+"/remote/machines/router/state/rules",
		map[string]any{"mode": "replace", "rules": []any{rule}})
	if resp.StatusCode != 200 {
		t.Fatalf("patch rules: status %d (%s)", resp.StatusCode, body)
	}

	resp, body = authDo(t, http.MethodGet, base+"/remote/machines/router/state/rules", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get rules: status %d", resp.StatusCode)
	}
	var rules struct {
		Rules []json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(body, &rules); err != nil {
		t.Fatalf("rules parse: %v", err)
	}
	if len(rules.Rules) != 1 {
		t.Errorf("rules after patch: %d, want 1", len(rules.Rules))
	}

	// Машинный state отдельный: локальный /state/rules его не видит.
	resp, body = authDo(t, http.MethodGet, base+"/state/rules", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("local rules: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &rules); err != nil {
		t.Fatalf("local rules parse: %v", err)
	}
	if len(rules.Rules) != 0 {
		t.Errorf("local rules leaked machine rules: %d entries", len(rules.Rules))
	}
}
