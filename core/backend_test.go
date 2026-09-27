package core

import (
	"testing"

	"singbox-launcher/api"
	"singbox-launcher/core/services"
)

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

// fakeOwnTransportBackend — daemon-подобный бэкенд со своим транспортом
// (реализует ownTransportSource, как DaemonBackend).
type fakeOwnTransportBackend struct {
	fakeBackend
	own services.ProxyTransport
}

func (f *fakeOwnTransportBackend) ownProxyTransport() services.ProxyTransport { return f.own }

// markerTransport — транспорт-метка: сравнивается по указателю.
type markerTransport struct{ name string }

func (*markerTransport) GroupProxies(string) ([]api.ProxyInfo, string, error) { return nil, "", nil }
func (*markerTransport) SwitchProxy(string, string) error                     { return nil }
func (*markerTransport) Delay(string) (int64, error)                          { return 0, nil }

// TestLocalProxyTransportIgnoresMachineOverride — пока на экране вкладка
// Remote, в APIService стоит транспорт удалённой машины. Область Local всё
// равно обязана получить транспорт СВОЕГО ядра: иначе команды панели Local и
// её окон (выключатель WireGuard, переключение узла) уходят на роутер.
func TestLocalProxyTransportIgnoresMachineOverride(t *testing.T) {
	machine := &markerTransport{name: "machine"}

	t.Run("daemon", func(t *testing.T) {
		own := &markerTransport{name: "own"}
		ac := &AppController{APIService: &services.APIService{}}
		ac.setBackend(&fakeOwnTransportBackend{fakeBackend: fakeBackend{mode: BackendDaemon}, own: own})
		ac.APIService.SetTransport(machine) // вкладка Remote с выбранной машиной

		if got := ac.LocalProxyTransport(); got != services.ProxyTransport(own) {
			t.Fatalf("Local got %v, want own daemon transport", got)
		}
		if got, ok := ac.OwnDaemonTransport(); !ok || got != services.ProxyTransport(own) {
			t.Fatalf("OwnDaemonTransport = (%v,%v), want own daemon transport", got, ok)
		}
		// Цель Remote без подключённой машины не подхватывает транспорт,
		// стоящий в APIService: источник машины — только её выбор.
		if ac.ChainsAvailable(CoreIn(services.ScopeRemote)) || ac.DaemonPoolAvailable(CoreIn(services.ScopeRemote)) {
			t.Fatal("Remote target must not read the transport from APIService")
		}
	})

	t.Run("classic", func(t *testing.T) {
		ac := &AppController{APIService: &services.APIService{Enabled: true, BaseURL: "http://127.0.0.1:9090", Token: "tok"}}
		ac.setBackend(&fakeBackend{mode: BackendClassic})
		ac.APIService.SetTransport(machine) // classic-клиент с подключённой машиной

		ct, ok := ac.LocalProxyTransport().(services.ClashTransport)
		if !ok {
			t.Fatalf("Local got %T, want Clash HTTP of own core", ac.LocalProxyTransport())
		}
		if ct.BaseURL != "http://127.0.0.1:9090" || ct.Token != "tok" {
			t.Fatalf("Local Clash endpoint = %+v, want own config endpoint", ct)
		}
		if _, ok := ac.OwnDaemonTransport(); ok {
			t.Fatal("classic backend has no own daemon transport")
		}
	})
}
