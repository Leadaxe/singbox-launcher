package services

import (
	"testing"

	"singbox-launcher/api"
)

type stubTransport struct {
	group    string
	switched [2]string
	delayed  string
}

func (s *stubTransport) GroupProxies(group string) ([]api.ProxyInfo, string, error) {
	s.group = group
	return []api.ProxyInfo{{Name: "node-a"}}, "node-a", nil
}
func (s *stubTransport) SwitchProxy(group, name string) error {
	s.switched = [2]string{group, name}
	return nil
}
func (s *stubTransport) Delay(name string) (int64, error) {
	s.delayed = name
	return 42, nil
}

func TestWireTransportUsesOverride(t *testing.T) {
	svc := &APIService{Enabled: false} // Clash отключён…
	stub := &stubTransport{}
	svc.SetTransport(stub) // …но override установлен

	got, err := svc.wireTransport()
	if err != nil {
		t.Fatalf("wireTransport with override errored: %v", err)
	}
	if got != stub {
		t.Fatal("wireTransport did not return the override")
	}
	if svc.TransportOverride() != stub {
		t.Fatal("TransportOverride getter mismatch")
	}
}

func TestWireTransportClashFallback(t *testing.T) {
	svc := &APIService{Enabled: true, BaseURL: "http://127.0.0.1:9090", Token: "tok"}
	got, err := svc.wireTransport()
	if err != nil {
		t.Fatalf("wireTransport errored: %v", err)
	}
	ct, ok := got.(ClashTransport)
	if !ok {
		t.Fatalf("want ClashTransport, got %T", got)
	}
	if ct.BaseURL != "http://127.0.0.1:9090" || ct.Token != "tok" {
		t.Fatalf("clash transport carried wrong endpoint: %+v", ct)
	}
}

func TestWireTransportDisabledNoOverride(t *testing.T) {
	svc := &APIService{Enabled: false}
	if _, err := svc.wireTransport(); err == nil {
		t.Fatal("expected error when clash disabled and no override")
	}
}

func TestSetTransportNilRestoresClash(t *testing.T) {
	svc := &APIService{Enabled: true, BaseURL: "http://x", Token: "t"}
	svc.SetTransport(&stubTransport{})
	svc.SetTransport(nil)
	got, err := svc.wireTransport()
	if err != nil {
		t.Fatalf("errored after clearing override: %v", err)
	}
	if _, ok := got.(ClashTransport); !ok {
		t.Fatalf("want ClashTransport after nil override, got %T", got)
	}
}

// SwitchProxyVia использует ЯВНО переданный транспорт, минуя wireTransport
// (нужно для UI-remote-override SPEC 064).
func TestSwitchProxyViaUsesGivenTransport(t *testing.T) {
	svc := &APIService{Enabled: false} // wireTransport упал бы (clash off)
	svc.LastSelectedProxyByGroup = map[string]string{}
	explicit := &stubTransport{}
	if err := svc.SwitchProxyVia(explicit, "grp", "node-x"); err != nil {
		t.Fatalf("SwitchProxyVia: %v", err)
	}
	if explicit.switched != [2]string{"grp", "node-x"} {
		t.Fatalf("explicit transport SwitchProxy got %v", explicit.switched)
	}
	if svc.GetActiveProxyName() != "node-x" {
		t.Fatalf("active proxy = %q, want node-x", svc.GetActiveProxyName())
	}
	if svc.GetLastSelectedProxyForGroup("grp") != "node-x" {
		t.Error("last-selected not recorded")
	}
}

// SwitchProxy идёт через транспорт-override, а не через api.SwitchProxy.
func TestAPIServiceSwitchProxyUsesTransport(t *testing.T) {
	svc := &APIService{Enabled: false}
	svc.LastSelectedProxyByGroup = map[string]string{}
	stub := &stubTransport{}
	svc.SetTransport(stub)
	if err := svc.SwitchProxy("proxy-out", "node-b"); err != nil {
		t.Fatalf("SwitchProxy: %v", err)
	}
	if stub.switched != [2]string{"proxy-out", "node-b"} {
		t.Fatalf("transport SwitchProxy got %v", stub.switched)
	}
	if svc.GetActiveProxyName() != "node-b" {
		t.Fatalf("active proxy = %q, want node-b", svc.GetActiveProxyName())
	}
}
