package core

import "testing"

// fakeBackend — минимальная реализация CoreBackend для проверки
// диспетчеризации и учёта Close при setBackend.
type fakeBackend struct {
	mode     BackendMode
	started  int
	stopped  int
	restart  int
	exitStop bool
	closed   int
}

func (f *fakeBackend) Mode() BackendMode  { return f.mode }
func (f *fakeBackend) StartVPN(_ ...bool) { f.started++ }
func (f *fakeBackend) StopVPN()           { f.stopped++ }
func (f *fakeBackend) RestartVPN()        { f.restart++ }
func (f *fakeBackend) OnAppExit() bool    { return f.exitStop }
func (f *fakeBackend) Close()             { f.closed++ }

// fakeClashBackend добавляет ClashEndpoint (реализует clashEndpointSource).
type fakeClashBackend struct {
	fakeBackend
	base, token string
	ok          bool
}

func (f *fakeClashBackend) ClashEndpoint() (string, string, bool) {
	return f.base, f.token, f.ok
}

func TestDaemonClashEndpoint(t *testing.T) {
	ac := &AppController{}
	// Classic backend без ClashEndpoint → ok=false.
	ac.setBackend(&fakeBackend{mode: BackendClassic})
	if _, _, ok := ac.DaemonClashEndpoint(); ok {
		t.Fatal("classic backend must not provide a clash endpoint")
	}
	// Daemon-подобный backend отдаёт свой адрес.
	ac.setBackend(&fakeClashBackend{
		fakeBackend: fakeBackend{mode: BackendDaemon},
		base:        "http://127.0.0.1:9190", token: "tok", ok: true,
	})
	base, tok, ok := ac.DaemonClashEndpoint()
	if !ok || base != "http://127.0.0.1:9190" || tok != "tok" {
		t.Fatalf("daemon clash endpoint = (%q,%q,%v), want (http://127.0.0.1:9190,tok,true)", base, tok, ok)
	}
}

func TestBackendModeDefault(t *testing.T) {
	ac := &AppController{}
	// Без установленного backend BackendMode падает обратно на classic.
	if ac.BackendMode() != BackendClassic {
		t.Fatalf("default mode = %q, want classic", ac.BackendMode())
	}
}

func TestSetBackendClosesPrevious(t *testing.T) {
	ac := &AppController{}
	first := &fakeBackend{mode: BackendClassic}
	ac.setBackend(first)
	if ac.Backend() != first {
		t.Fatal("Backend() did not return the set backend")
	}
	second := &fakeBackend{mode: BackendDaemon}
	ac.setBackend(second)
	if first.closed != 1 {
		t.Fatalf("previous backend Close() called %d times, want 1", first.closed)
	}
	if ac.BackendMode() != BackendDaemon {
		t.Fatalf("mode after swap = %q, want daemon", ac.BackendMode())
	}
}

func TestSwitchBackendModeNoop(t *testing.T) {
	ac := &AppController{RunningState: &RunningState{}}
	ac.setBackend(&fakeBackend{mode: BackendClassic})
	// Тот же режим — no-op, не должно быть ошибки.
	if err := ac.SwitchBackendMode(BackendClassic); err != nil {
		t.Fatalf("noop switch errored: %v", err)
	}
}

func TestSwitchBackendModeBlockedWhileRunning(t *testing.T) {
	ac := &AppController{RunningState: &RunningState{}}
	ac.RunningState.controller = ac
	ac.setBackend(&fakeBackend{mode: BackendClassic})
	ac.RunningState.running = true // имитируем работающий VPN напрямую
	if err := ac.SwitchBackendMode(BackendDaemon); err == nil {
		t.Fatal("expected error switching mode while VPN is running")
	}
}
