package debugapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/grpc"
)

// fakeDaemonFacade — DaemonFacade без реального демона: проверяем контракт
// endpoint'ов, а не macOS-обвязку.
type fakeDaemonFacade struct {
	mode      string
	switchErr error
	paired    bool
}

func (f *fakeDaemonFacade) Status() DaemonStatus {
	return DaemonStatus{Paired: f.paired, Address: "127.0.0.1:19091", CoreSupportsLxd: true}
}
func (f *fakeDaemonFacade) Pair(invite, secret string) error { return nil }
func (f *fakeDaemonFacade) Unpair() error                    { return nil }
func (f *fakeDaemonFacade) SetAddress(addr string) error     { return nil }
func (f *fakeDaemonFacade) SetSecret(secret string) error    { return nil }
func (f *fakeDaemonFacade) Commands() DaemonCommands {
	return DaemonCommands{Install: "sudo sing-box lxd --service=install"}
}
func (f *fakeDaemonFacade) EngineMode() string { return f.mode }
func (f *fakeDaemonFacade) SwitchEngine(mode string) error {
	if f.switchErr != nil {
		return f.switchErr
	}
	f.mode = mode
	return nil
}
func (f *fakeDaemonFacade) AdminDo(method, path string, body []byte, contentType string) (int, []byte, string, error) {
	return 200, []byte(`{"status":"idle"}`), "application/json", nil
}
func (f *fakeDaemonFacade) GRPCConn() (*grpc.ClientConn, error) {
	return nil, errors.New("no daemon in tests")
}

func TestDaemonGroupContract(t *testing.T) {
	fd := &fakeDaemonFacade{mode: "classic"}
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{}, port, "remote-test-token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.EnableDaemon(fd)
	s.Start()
	t.Cleanup(s.Stop)
	base := "http://127.0.0.1:" + itoa(port)

	// capabilities: daemon включён, remote — нет.
	resp, body := authDo(t, http.MethodGet, base+"/", nil)
	var manifest struct {
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("manifest parse: %v", err)
	}
	if !manifest.Capabilities["daemon"] || manifest.Capabilities["remote"] {
		t.Errorf("capabilities = %v, want daemon:true remote:false", manifest.Capabilities)
	}

	// Статус отдаёт снимок фасада.
	resp, body = authDo(t, http.MethodGet, base+"/daemon/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var st DaemonStatus
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("status parse: %v", err)
	}
	if !st.CoreSupportsLxd || st.Address == "" {
		t.Errorf("status = %+v", st)
	}

	// Движок: невалидный режим — 400; валидный — переключает.
	resp, _ = authDo(t, http.MethodPost, base+"/daemon/engine", map[string]any{"mode": "warp9"})
	if resp.StatusCode != 400 {
		t.Errorf("engine bad mode: %d, want 400", resp.StatusCode)
	}
	resp, body = authDo(t, http.MethodPost, base+"/daemon/engine", map[string]any{"mode": "daemon"})
	if resp.StatusCode != 200 {
		t.Fatalf("engine switch: %d (%s)", resp.StatusCode, body)
	}
	if fd.mode != "daemon" {
		t.Errorf("facade mode = %q, want daemon", fd.mode)
	}

	// Работающий VPN — 409 (конфликт состояния, не 500).
	fd.switchErr = errors.New("stop the VPN before switching the core engine")
	resp, _ = authDo(t, http.MethodPost, base+"/daemon/engine", map[string]any{"mode": "classic"})
	if resp.StatusCode != 409 {
		t.Errorf("engine while running: %d, want 409", resp.StatusCode)
	}

	// Raw REST к локальному демону через фасад.
	resp, body = authDo(t, http.MethodPost, base+"/daemon/raw/rest",
		map[string]any{"method": "GET", "path": "/admin/status"})
	if resp.StatusCode != 200 {
		t.Fatalf("daemon raw rest: %d (%s)", resp.StatusCode, body)
	}
	var raw struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Status != 200 {
		t.Errorf("daemon raw rest = %s (err %v)", body, err)
	}
}
