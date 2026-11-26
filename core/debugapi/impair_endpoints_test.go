package debugapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	daemonpb "singbox-launcher/internal/daemonpb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// impairFakeCore — StartedService ядра с одним impair-узлом "imp": запоминает,
// что ему прислали, и отвечает статусом или ошибкой ядра.
type impairFakeCore struct {
	daemonpb.UnimplementedStartedServiceServer
	gotEntry    string
	gotSchedule *daemonpb.ScheduleImpairRequest
}

func (c *impairFakeCore) check(tag string) error {
	switch tag {
	case "imp":
		return nil
	case "direct-out":
		return status.Error(codes.InvalidArgument, "direct-out is not an impair outbound")
	case "stopped":
		return status.Error(codes.FailedPrecondition, "service is not started")
	}
	return status.Error(codes.NotFound, "outbound not found: "+tag)
}

func (c *impairFakeCore) GetImpairStatus(_ context.Context, r *daemonpb.ImpairStatusRequest) (*daemonpb.ImpairStatus, error) {
	if err := c.check(r.GetOutboundTag()); err != nil {
		return nil, err
	}
	return &daemonpb.ImpairStatus{Json: `{"tag":"imp","link_params":{"rate":"1mbit"}}`}, nil
}

func (c *impairFakeCore) SetImpair(_ context.Context, r *daemonpb.SetImpairRequest) (*daemonpb.ImpairStatus, error) {
	if err := c.check(r.GetOutboundTag()); err != nil {
		return nil, err
	}
	c.gotEntry = r.GetEntry()
	return &daemonpb.ImpairStatus{Json: `{"tag":"imp"}`}, nil
}

func (c *impairFakeCore) ScheduleImpair(_ context.Context, r *daemonpb.ScheduleImpairRequest) (*daemonpb.ImpairStatus, error) {
	if err := c.check(r.GetOutboundTag()); err != nil {
		return nil, err
	}
	c.gotSchedule = r
	return &daemonpb.ImpairStatus{Json: `{"tag":"imp"}`}, nil
}

// impairDaemonFacade — фасад демона, чей gRPC ведёт в фейковое ядро.
type impairDaemonFacade struct {
	fakeDaemonFacade
	addr string
}

func (f *impairDaemonFacade) GRPCConn() (*grpc.ClientConn, error) {
	return grpc.NewClient(f.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// TestImpairEndpointsPassThrough — маршруты /impair/* передают запрос ядру как
// есть (запись расписания, entries/start/repeat) и отдают его статус; ошибки
// ядра становятся HTTP-кодами: нет тега — 404, не impair — 400, сервис не
// запущен — 409; неверное тело отсекается до ядра (400).
func TestImpairEndpointsPassThrough(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	core := &impairFakeCore{}
	gs := grpc.NewServer()
	daemonpb.RegisterStartedServiceServer(gs, core)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	port := freeLocalPort(t)
	s, err := New(&fakeFacade{}, port, "remote-test-token")
	if err != nil {
		t.Fatal(err)
	}
	s.EnableDaemon(&impairDaemonFacade{fakeDaemonFacade: fakeDaemonFacade{mode: "daemon"}, addr: lis.Addr().String()})
	s.Start()
	t.Cleanup(s.Stop)
	base := "http://127.0.0.1:" + itoa(port)

	resp, body := authDo(t, http.MethodGet, base+"/impair/imp", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d %s", resp.StatusCode, body)
	}
	var st map[string]any
	if err := json.Unmarshal(body, &st); err != nil || st["tag"] != "imp" {
		t.Errorf("status body = %s", body)
	}

	resp, body = authDo(t, http.MethodPost, base+"/impair/imp/set",
		map[string]any{"link_params": map[string]any{"outage": true, "reset": true}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set: %d %s", resp.StatusCode, body)
	}
	if core.gotEntry != `{"link_params":{"outage":true,"reset":true}}` {
		t.Errorf("entry sent to the core = %s", core.gotEntry)
	}

	resp, body = authDo(t, http.MethodPost, base+"/impair/imp/schedule", map[string]any{
		"entries": []any{map[string]any{"at": "0s", "link_params": map[string]any{"profile": "lte-walk"}}},
		"start":   "first_use", "repeat": "60s",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schedule: %d %s", resp.StatusCode, body)
	}
	if g := core.gotSchedule; g == nil || g.GetEntries() != `[{"at":"0s","link_params":{"profile":"lte-walk"}}]` || g.GetStart() != "first_use" || g.GetRepeat() != "60s" {
		t.Errorf("schedule sent to the core = %+v", g)
	}
	resp, _ = authDo(t, http.MethodPost, base+"/impair/imp/schedule", map[string]any{"entries": []any{}})
	if resp.StatusCode != http.StatusOK || core.gotSchedule.GetEntries() != "[]" {
		t.Errorf("reset schedule: %d, entries %q", resp.StatusCode, core.gotSchedule.GetEntries())
	}

	for path, want := range map[string]int{
		"/impair/missing":    http.StatusNotFound,
		"/impair/direct-out": http.StatusBadRequest,
		"/impair/stopped":    http.StatusConflict,
	} {
		if resp, body := authDo(t, http.MethodGet, base+path, nil); resp.StatusCode != want {
			t.Errorf("GET %s = %d, want %d (%s)", path, resp.StatusCode, want, body)
		}
	}
	if resp, _ := authDo(t, http.MethodPost, base+"/impair/imp/schedule", map[string]any{"entries": "x"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad entries = %d, want 400", resp.StatusCode)
	}
	if resp, _ := authDo(t, http.MethodPost, base+"/impair/imp/set", []any{1}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("non-object entry = %d, want 400", resp.StatusCode)
	}
}
