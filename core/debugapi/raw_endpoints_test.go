package debugapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestResolveGRPCMethod(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
		kind    string
	}{
		{"/daemon.StartedService/GetGroups", false, "unary"},
		{"daemon.StartedService/GetGroups", false, "unary"},
		{"daemon.StartedService.GetGroups", false, "unary"},
		{"/daemon.StartedService/SubscribeLog", false, "server_stream"},
		{"/daemon.StartedService/ProvideUSBDevices", false, "bidi_stream"},
		{"/daemon.ManagedService/StopService", false, "unary"},
		{"/daemon.StartedService/NoSuchMethod", true, ""},
		{"/daemon.NoSuchService/GetGroups", true, ""},
		// Туннель ведёт к демону: чужие пакеты не резолвятся, даже если их
		// дескрипторы есть в бинаре.
		{"/google.protobuf.Empty/Whatever", true, ""},
		{"garbage", true, ""},
		{"", true, ""},
	}
	for _, tc := range cases {
		m, err := resolveGRPCMethod(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("resolve(%q): expected error, got %v", tc.in, m.FullName())
			}
			continue
		}
		if err != nil {
			t.Errorf("resolve(%q): %v", tc.in, err)
			continue
		}
		if got := grpcMethodKind(m); got != tc.kind {
			t.Errorf("resolve(%q): kind %q, want %q", tc.in, got, tc.kind)
		}
	}
}

func TestGRPCMethodsDiscovery(t *testing.T) {
	base, _, _ := newRemoteTestServer(t)

	resp, body := authDo(t, http.MethodGet, base+"/grpc/methods", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("/grpc/methods: status %d", resp.StatusCode)
	}
	var out struct {
		Methods []struct {
			Full   string `json:"full"`
			Kind   string `json:"kind"`
			Input  string `json:"input"`
			Output string `json:"output"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(out.Methods) == 0 {
		t.Fatal("no methods discovered — daemonpb descriptors not registered?")
	}
	byFull := map[string]string{}
	for _, m := range out.Methods {
		if !hasPrefix(m.Full, "/daemon.") {
			t.Errorf("foreign method leaked into discovery: %s", m.Full)
		}
		byFull[m.Full] = m.Kind
	}
	for full, kind := range map[string]string{
		"/daemon.StartedService/GetGroups":    "unary",
		"/daemon.StartedService/SubscribeLog": "server_stream",
		"/daemon.ManagedService/StopService":  "unary",
	} {
		if got := byFull[full]; got != kind {
			t.Errorf("%s: kind %q, want %q", full, got, kind)
		}
	}
}

// hasPrefix — локальный алиас, чтобы не тянуть strings ради одного вызова в
// проверках теста.
func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func TestRawGRPCValidation(t *testing.T) {
	base, execDir, _ := newRemoteTestServer(t)
	// Намеренно мёртвый порт: валидация обязана отработать до похода в сеть.
	seedMachine(t, execDir, "router", "127.0.0.1:1")

	// method обязателен.
	resp, _ := authDo(t, http.MethodPost, base+"/remote/machines/router/raw/grpc",
		map[string]any{})
	if resp.StatusCode != 400 {
		t.Errorf("empty method: status %d, want 400", resp.StatusCode)
	}

	// Неизвестный метод — 400 без похода в сеть.
	resp, _ = authDo(t, http.MethodPost, base+"/remote/machines/router/raw/grpc",
		map[string]any{"method": "/daemon.StartedService/Nope"})
	if resp.StatusCode != 400 {
		t.Errorf("unknown method: status %d, want 400", resp.StatusCode)
	}

	// client/bidi-stream — 501, тоже без сети (резолв раньше dial).
	resp, body := authDo(t, http.MethodPost, base+"/remote/machines/router/raw/grpc",
		map[string]any{"method": "/daemon.StartedService/ProvideUSBDevices"})
	if resp.StatusCode != 501 {
		t.Errorf("bidi method: status %d (%s), want 501", resp.StatusCode, body)
	}
}
