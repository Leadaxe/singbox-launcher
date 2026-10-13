package lxdclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestClientCommands — именованные команды демона (SPEC 128 форка) поверх
// plain-h2c клиента с Bearer: список 200/404, запуск 202/409, 404 по имени,
// разбор exit_code: null, пустое тело POST и отказ клиента от плохого имени.
func TestClientCommands(t *testing.T) {
	var unsupported atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
			t.Errorf("%s %s: Authorization = %q", r.Method, r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		if unsupported.Load() {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`404 page not found`))
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/commands":
			_, _ = w.Write([]byte(`{"commands":[
				{"name":"echo","command":"echo hi","state":"done","started_at":"2026-10-12T10:00:00Z","finished_at":"2026-10-12T10:00:01Z","exit_code":0,"client":"singbox-launcher"},
				{"name":"update","command":"/usr/bin/sing-box lxd update","state":"idle","started_at":"","finished_at":"","exit_code":null,"client":""}]}`))
		case "GET /admin/commands/update":
			_, _ = w.Write([]byte(`{"name":"update","command":"/usr/bin/sing-box lxd update","state":"running","started_at":"2026-10-12T10:00:00Z","finished_at":"","exit_code":null,"client":"singbox-launcher","log":"downloading…\n"}`))
		case "POST /admin/commands/echo":
			if ct := r.Header.Get("Content-Type"); ct != "" {
				t.Errorf("POST Content-Type = %q, want none", ct)
			}
			if body, _ := io.ReadAll(r.Body); len(body) != 0 {
				t.Errorf("POST body = %q, want empty", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"name":"echo","command":"echo hi","state":"running","started_at":"2026-10-12T10:00:00Z","finished_at":"","exit_code":null,"client":"singbox-launcher"}`))
		case "POST /admin/commands/update":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"name":"update","command":"/usr/bin/sing-box lxd update","state":"running","started_at":"2026-10-12T09:59:00Z","finished_at":"","exit_code":null,"client":"operator"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no such command"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := New(Config{Addr: srv.Listener.Addr().String(), Secret: "s3cret"})
	ctx := context.Background()

	list, err := c.CommandsCtx(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Name != "echo" || list[0].ExitCode == nil || *list[0].ExitCode != 0 ||
		list[1].Name != "update" || list[1].ExitCode != nil || list[1].State != CommandIdle {
		t.Fatalf("list decode: %+v", list)
	}

	got, err := c.CommandCtx(ctx, "update")
	if err != nil || got.State != CommandRunning || got.ExitCode != nil || got.Log != "downloading…\n" {
		t.Fatalf("get update: %+v, %v", got, err)
	}

	started, err := c.RunCommandCtx(ctx, "echo")
	if err != nil || started.State != CommandRunning || started.Client != "singbox-launcher" {
		t.Fatalf("run echo (202): %+v, %v", started, err)
	}

	_, err = c.RunCommandCtx(ctx, "update")
	var running *ErrCommandRunning
	if !errors.As(err, &running) || running.Current.Client != "operator" || running.Current.StartedAt != "2026-10-12T09:59:00Z" {
		t.Fatalf("run update (409): %v", err)
	}

	if _, err := c.CommandCtx(ctx, "gone"); !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("get gone: %v, want ErrCommandNotFound", err)
	}
	if _, err := c.RunCommandCtx(ctx, "gone"); !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("run gone: %v, want ErrCommandNotFound", err)
	}

	for _, bad := range []string{"", "../info", "Update", "a/b", "-x"} {
		if _, err := c.RunCommandCtx(ctx, bad); err == nil {
			t.Errorf("RunCommandCtx(%q) accepted", bad)
		}
	}

	unsupported.Store(true)
	if _, err := c.CommandsCtx(ctx); !errors.Is(err, ErrCommandsUnsupported) {
		t.Fatalf("list on an old core: %v, want ErrCommandsUnsupported", err)
	}
}
