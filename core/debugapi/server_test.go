package debugapi

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// fakeFacade lets tests drive the server without booting a whole controller.
type fakeFacade struct {
	running     bool
	proxies     []api.ProxyInfo
	active      string
	group       string
	version     string
	lastSuccess time.Time
	updateErr   error
	execDir     string // optional; only used by snapshot tests

	// state surface (SPEC 053/056/057/058 endpoints)
	stateValue    *state.State
	stateLoadErr  error
	saveStateErr  error
	savedState    *state.State
	templateValue *template.TemplateData
	templateErr   error
	logLevel      string
	logLevelSet   bool
	logLevelErr   error
	applyLevelErr error
	appliedLevel  string
}

func (f *fakeFacade) IsRunning() bool                    { return f.running }
func (f *fakeFacade) GetProxiesList() []api.ProxyInfo    { return f.proxies }
func (f *fakeFacade) GetActiveProxyName() string         { return f.active }
func (f *fakeFacade) GetSelectedClashGroup() string      { return f.group }
func (f *fakeFacade) GetSingboxVersion() string          { return f.version }
func (f *fakeFacade) GetConfigPath() string              { return "/tmp/config.json" }
func (f *fakeFacade) GetExecDir() string                 { return f.execDir }
func (f *fakeFacade) GetLauncherVersion() string         { return "v-test" }
func (f *fakeFacade) GetLastUpdateSucceededAt() time.Time { return f.lastSuccess }
func (f *fakeFacade) StartSingBox() error                { return nil }
func (f *fakeFacade) StopSingBox() error                 { return nil }
func (f *fakeFacade) UpdateSubscriptions() error         { return f.updateErr }
func (f *fakeFacade) PingAllProxies() error              { return nil }
func (f *fakeFacade) RebuildConfigIfDirty() error        { return nil }

func (f *fakeFacade) LoadState() (*state.State, error) {
	if f.stateLoadErr != nil {
		return nil, f.stateLoadErr
	}
	if f.stateValue == nil {
		return state.New(), nil
	}
	return f.stateValue, nil
}

func (f *fakeFacade) SaveState(s *state.State) error {
	if f.saveStateErr != nil {
		return f.saveStateErr
	}
	f.savedState = s
	f.stateValue = s
	return nil
}

func (f *fakeFacade) LoadTemplate() (*template.TemplateData, error) {
	if f.templateErr != nil {
		return nil, f.templateErr
	}
	if f.templateValue == nil {
		return &template.TemplateData{}, nil
	}
	return f.templateValue, nil
}

func (f *fakeFacade) ApplyLogLevelAndReload(level string) error {
	f.appliedLevel = level
	return f.applyLevelErr
}

func (f *fakeFacade) ReadCurrentLogLevel() (string, bool, error) {
	return f.logLevel, f.logLevelSet, f.logLevelErr
}

// freeLocalPort binds :0 then closes, returning the port. Good enough for
// server-under-test tests on a dev box.
func freeLocalPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestServerAuthAndState(t *testing.T) {
	port := freeLocalPort(t)
	ff := &fakeFacade{running: true, active: "JP-01", group: "auto-proxy-out", version: "1.12.12"}
	s, err := New(ff, port, "s3cr3t-token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	defer s.Stop()
	base := "http://127.0.0.1:" + itoa(port)

	// /ping is unauthenticated.
	resp, err := http.Get(base + "/ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("ping: status %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// /state without auth → 401.
	resp, err = http.Get(base + "/state")
	if err != nil {
		t.Fatalf("state noauth: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("state noauth: status %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// /state with wrong token → 401.
	req, _ := http.NewRequest("GET", base+"/state", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("state wrong: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("state wrong: status %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// /state with correct token → 200 + body contains "JP-01".
	req, _ = http.NewRequest("GET", base+"/state", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("state ok: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("state ok: status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"active_proxy":"JP-01"`) {
		t.Errorf("state body: %s", string(body))
	}

	// Action endpoints require POST.
	req, _ = http.NewRequest("GET", base+"/action/start", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("action get: %v", err)
	}
	if resp.StatusCode != 405 {
		t.Errorf("action GET: status %d, want 405", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestRebuildConfigEndpoint — POST /action/rebuild-config: 200 OK на успех,
// 405 на GET, 401 без токена, 500 на ошибку фасада.
func TestRebuildConfigEndpoint(t *testing.T) {
	port := freeLocalPort(t)
	ff := &fakeFacade{}
	srv, err := New(ff, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.Start()
	defer srv.Stop()

	base := "http://127.0.0.1:" + itoa(port)
	// Дать listener'у время.
	time.Sleep(50 * time.Millisecond)

	// 1. POST с токеном — 200.
	req, _ := http.NewRequest("POST", base+"/action/rebuild-config", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("rebuild-config: status %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `"ok":true`) {
		t.Errorf("rebuild-config body: %s", string(body))
	}

	// 2. GET — 405.
	req, _ = http.NewRequest("GET", base+"/action/rebuild-config", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != 405 {
		t.Errorf("rebuild-config GET: status %d, want 405", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 3. POST без токена — 401.
	req, _ = http.NewRequest("POST", base+"/action/rebuild-config", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("noauth: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("rebuild-config noauth: status %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestGenerateTokenUnique(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if a == b {
		t.Error("two generated tokens are equal — entropy broken")
	}
	if len(a) < 32 {
		t.Errorf("token unexpectedly short: %q", a)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := make([]byte, 0, 10)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
