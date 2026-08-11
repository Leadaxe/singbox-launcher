//go:build darwin

package lxdclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	emptypb "google.golang.org/protobuf/types/known/emptypb"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// E2E-прогон против НАСТОЯЩЕГО демона `sing-box lxd`.
//
// Активация: LXD_E2E_BIN=/path/to/sing-box (сборка с with_lx_command);
// без переменной тест скипается — обычный CI его не гоняет.
//
// Демон поднимается собственным дочерним процессом на loopback-порту с
// временным state-dir и гасится строго по PID своего процесса (никаких
// pkill по имени — на машине может жить боевой sing-box).
const e2eListen = "127.0.0.1:29417"

// e2eConfig — минимальный конфиг без TUN (никаких привилегий): mixed-inbound
// на loopback, selector + urltest группы для проверки gRPC-наблюдаемости.
const e2eConfig = `{
  "log": {"level": "debug"},
  "inbounds": [
    {"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 29418}
  ],
  "outbounds": [
    {"type": "direct", "tag": "direct-a"},
    {"type": "direct", "tag": "direct-b"},
    {"type": "selector", "tag": "proxy-out", "outbounds": ["direct-a", "direct-b"], "default": "direct-a"},
    {"type": "urltest", "tag": "auto", "outbounds": ["direct-a", "direct-b"]}
  ]
}`

func TestDaemonEndToEnd(t *testing.T) {
	singboxBin := os.Getenv("LXD_E2E_BIN")
	if singboxBin == "" {
		t.Skip("LXD_E2E_BIN is not set — skipping live daemon e2e")
	}

	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	// Новая модель владения (ревизия 057): дом демона — state-dir, все его
	// настройки в daemon.json. Демон запускается голым `lxd --state-dir` и
	// обязан подхватить listen/tls/secret из своего файла.
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "e2e-test-secret"
	daemonJSON := `{"listen":"` + e2eListen + `","tls":true,"secret":"` + secret + `"}`
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.json"), []byte(daemonJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(singboxBin, "lxd", "--state-dir", stateDir)
	// cwd демона → временный каталог: иначе он наследует каталог пакета и
	// пишет туда рантайм-файлы (cache.db), засоряя рабочее дерево.
	cmd.Dir = tempDir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() {
		// Только свой PID — see memory: никогда не убивать sing-box по имени.
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	})

	// --- Сопряжение прод-путём оператора: `lxd client add` сам находит
	// listen/secret в daemon.json указанного state-dir и минтит приглашение.
	var invite string
	deadline := time.Now().Add(15 * time.Second)
	for {
		out, err := exec.Command(singboxBin, "lxd", "client", "add",
			"--state-dir", stateDir, "--name", "e2e-launcher").CombinedOutput()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if candidate := strings.TrimSpace(line); strings.Contains(candidate, "#") {
					if _, parseErr := ParseInvite(candidate); parseErr == nil {
						invite = candidate
					}
				}
			}
			if invite != "" {
				break
			}
			t.Fatalf("client add succeeded but printed no invite: %s", out)
		}
		if time.Now().After(deadline) {
			t.Fatalf("client add: %v (%s)", err, out)
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Logf("invite: %s", invite)

	parsed, err := ParseInvite(invite)
	if err != nil {
		t.Fatalf("parse invite: %v", err)
	}
	identity, err := LoadOrCreateIdentity(filepath.Join(tempDir, "launcher-id"))
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	// БЕЗ Secret: доверенный клиентский сертификат — полный мандат
	// mTLS-плоскости (bearer-or-cert); секрет демона лаунчер не знает.
	client := New(Config{
		Addr:              parsed.Addr,
		ServerFingerprint: parsed.ServerFingerprint,
		Identity:          identity,
	})
	if err := client.Enroll(parsed.Code, "e2e-launcher"); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	// --- Паспорт демона: /admin/info доступен по одному сертификату и
	// отдаёт настоящий state-dir (источник каталога для cache_file).
	passport, err := client.Info()
	if err != nil {
		t.Fatalf("info after enroll (cert-only): %v", err)
	}
	if passport.StateDir != stateDir {
		t.Fatalf("info.state_dir = %q, want %q", passport.StateDir, stateDir)
	}
	if !passport.TLS || passport.Fingerprint == "" {
		t.Fatalf("info must report tls+fingerprint, got %+v", passport)
	}
	t.Logf("daemon passport: version=%s state_dir=%s", passport.Version, passport.StateDir)

	// Одноразовость кода: второй enroll обязан упасть.
	if err := client.Enroll(parsed.Code, "e2e-launcher-2"); err == nil {
		t.Fatal("second enroll with the same code must fail")
	}

	// --- REST: status → idle; apply → started; config echo; stop; start.
	info, err := client.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if info.Status != "idle" {
		t.Fatalf("fresh daemon status = %q, want idle", info.Status)
	}

	if err := client.Apply([]byte(e2eConfig)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	info, err = client.Status()
	if err != nil {
		t.Fatalf("status after apply: %v", err)
	}
	if info.Status != "started" {
		t.Fatalf("status after apply = %q (last_error=%q), want started", info.Status, info.LastError)
	}

	activeConfig, err := client.ActiveConfig()
	if err != nil {
		t.Fatalf("active config: %v", err)
	}
	var parsedConfig map[string]any
	if err := json.Unmarshal(activeConfig, &parsedConfig); err != nil {
		t.Fatalf("active config is not JSON: %v", err)
	}

	// Битый конфиг → 422 rejected, работающий инстанс не тронут.
	if err := client.Apply([]byte(`{"inbounds": [{"type": "no-such-type"}]}`)); err == nil {
		t.Fatal("apply of a broken config must fail")
	} else {
		var applyErr *ApplyError
		if !errors.As(err, &applyErr) || !applyErr.Rejected() {
			t.Fatalf("broken config: want 422 rejected, got %v", err)
		}
	}
	if info, _ = client.Status(); info.Status != "started" {
		t.Fatalf("status after rejected apply = %q, want started", info.Status)
	}

	// --- gRPC: версия, статус-стрим, группы, selector, url-test, пул, логи.
	conn, err := client.DialGRPC()
	if err != nil {
		t.Fatalf("dial grpc: %v", err)
	}
	defer conn.Close()
	grpcClient := daemonpb.NewStartedServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	version, err := grpcClient.GetVersion(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetVersion over mTLS: %v", err)
	}
	t.Logf("daemon core version: %s", version.GetVersion())

	statusStream, err := grpcClient.SubscribeServiceStatus(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("SubscribeServiceStatus: %v", err)
	}
	first, err := statusStream.Recv()
	if err != nil {
		t.Fatalf("status stream recv: %v", err)
	}
	if first.GetStatus() != daemonpb.ServiceStatus_STARTED {
		t.Fatalf("initial stream status = %v, want STARTED", first.GetStatus())
	}

	groups, err := grpcClient.GetGroups(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetGroups: %v", err)
	}
	var selectorFound, urltestFound bool
	for _, g := range groups.GetGroup() {
		switch g.GetTag() {
		case "proxy-out":
			selectorFound = true
			if len(g.GetItems()) != 2 {
				t.Fatalf("proxy-out items = %d, want 2", len(g.GetItems()))
			}
		case "auto":
			urltestFound = true
		}
	}
	if !selectorFound || !urltestFound {
		t.Fatalf("groups missing: selector=%v urltest=%v", selectorFound, urltestFound)
	}

	if _, err := grpcClient.SelectOutbound(ctx, &daemonpb.SelectOutboundRequest{GroupTag: "proxy-out", OutboundTag: "direct-b"}); err != nil {
		t.Fatalf("SelectOutbound: %v", err)
	}
	groups, err = grpcClient.GetGroups(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("GetGroups after select: %v", err)
	}
	for _, g := range groups.GetGroup() {
		if g.GetTag() == "proxy-out" && g.GetSelected() != "direct-b" {
			t.Fatalf("selected = %q, want direct-b", g.GetSelected())
		}
	}

	// URL-тест узла: направляем на локальный endpoint демона? Внешней сети
	// может не быть — допускаем и успех, и ошибку теста, главное что RPC
	// отвечает (не транспортный сбой).
	urlTestResp, err := grpcClient.URLTestOutbound(ctx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: "direct-a",
		Link:        "https://www.gstatic.com/generate_204",
		Timeout:     5000,
	})
	if err != nil {
		t.Fatalf("URLTestOutbound transport error: %v", err)
	}
	t.Logf("urltest direct-a: delay=%d error=%q", urlTestResp.GetDelay(), urlTestResp.GetError())

	pool, err := grpcClient.GetPool(ctx, &daemonpb.GetPoolRequest{GroupTag: "auto"})
	if err != nil {
		t.Fatalf("GetPool: %v", err)
	}
	t.Logf("pool slots: %d", len(pool.GetSlots()))

	logStream, err := grpcClient.SubscribeLog(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("SubscribeLog: %v", err)
	}
	logBatch, err := logStream.Recv()
	if err != nil {
		t.Fatalf("log stream recv: %v", err)
	}
	t.Logf("log backlog: %d message(s)", len(logBatch.GetMessages()))

	// --- Канал переживает смену конфига: повторный apply при живом стриме.
	if err := client.Apply([]byte(strings.Replace(e2eConfig, "29418", "29419", 1))); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	// Стрим обязан доставить переходы без переподключения; ждём возврата в STARTED.
	sawStarted := false
	streamDeadline := time.Now().Add(10 * time.Second)
	for !sawStarted && time.Now().Before(streamDeadline) {
		st, err := statusStream.Recv()
		if err != nil {
			t.Fatalf("status stream died across apply: %v", err)
		}
		t.Logf("stream status: %v", st.GetStatus())
		if st.GetStatus() == daemonpb.ServiceStatus_STARTED {
			sawStarted = true
		}
	}
	if !sawStarted {
		t.Fatal("stream never returned to STARTED after apply")
	}

	// --- Stop/Start жизненного цикла.
	if err := client.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if info, _ = client.Status(); info.Status == "started" {
		t.Fatal("status still started after stop")
	}
	if err := client.Start(); err != nil {
		t.Fatalf("start from last-good: %v", err)
	}
	if info, _ = client.Status(); info.Status != "started" {
		t.Fatalf("status after start = %q, want started", info.Status)
	}
}
