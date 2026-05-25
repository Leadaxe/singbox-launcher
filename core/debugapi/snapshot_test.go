package debugapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/snapshot"
	"singbox-launcher/internal/constants"
)

// snapshotLayout создаёт временную exec-dir с поддиректориями bin/ и
// bin/wizard_states/. Возвращает execDir; путь готов к чтению через
// platform.GetWizardTemplatePath / GetWizardStatePath / GetOutboundsCachePath /
// GetConfigPath.
func snapshotLayout(t *testing.T) string {
	t.Helper()
	execDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(execDir, constants.BinDirName, constants.WizardStatesDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	return execDir
}

// writeSnapshotFile записывает байты по «названию» (template/state/cache/config),
// разрешая путь через те же helpers, что использует production-код.
func writeSnapshotFile(t *testing.T, execDir, name string, data []byte) {
	t.Helper()
	var path string
	switch name {
	case "template":
		path = filepath.Join(execDir, constants.BinDirName, constants.WizardTemplateFileName)
	case "state":
		path = filepath.Join(execDir, constants.BinDirName, constants.WizardStatesDirName, constants.WizardStateFileName)
	case "cache":
		path = filepath.Join(execDir, constants.BinDirName, constants.OutboundsCacheFileName)
	case "config":
		path = filepath.Join(execDir, constants.BinDirName, constants.ConfigFileName)
	default:
		t.Fatalf("unknown snapshot file %q", name)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// snapshotServer стартует сервер с переданным execDir; возвращает базовый URL и token.
func snapshotServer(t *testing.T, execDir string) (string, string) {
	t.Helper()
	port := freeLocalPort(t)
	ff := &fakeFacade{version: "1.13.x", execDir: execDir}
	const tok = "snapshot-token"
	s, err := New(ff, port, tok)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.Start()
	t.Cleanup(s.Stop)
	return "http://127.0.0.1:" + itoa(port), tok
}

// snapshotGET делает GET /debug/snapshot с переданным токеном; возвращает status и распарсенное тело.
func snapshotGET(t *testing.T, base, token string) (int, snapshot.Snapshot) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/debug/snapshot", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, snapshot.Snapshot{}
	}
	var out snapshot.Snapshot
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal body: %v\n--- body ---\n%s", err, body)
	}
	return resp.StatusCode, out
}

// TestSnapshot_AllFilesPresent — все четыре файла на диске → все в files; missing/errors пусты.
func TestSnapshot_AllFilesPresent(t *testing.T) {
	execDir := snapshotLayout(t)
	writeSnapshotFile(t, execDir, "template", []byte(`{"vars":[]}`))
	writeSnapshotFile(t, execDir, "state", []byte(`{"version":4,"vars":[]}`))
	writeSnapshotFile(t, execDir, "cache", []byte(`{"version":1}`))
	writeSnapshotFile(t, execDir, "config", []byte(`{"log":{"level":"info"}}`))

	base, tok := snapshotServer(t, execDir)
	st, resp := snapshotGET(t, base, tok)
	if st != http.StatusOK {
		t.Fatalf("status: want 200, got %d", st)
	}
	for _, name := range []string{"template", "state", "cache", "config"} {
		if _, ok := resp.Files[name]; !ok {
			t.Errorf("files[%q] missing in response: %+v", name, resp)
		}
	}
	if len(resp.Missing) != 0 {
		t.Errorf("Missing should be empty: %v", resp.Missing)
	}
	if len(resp.Errors) != 0 {
		t.Errorf("Errors should be empty: %v", resp.Errors)
	}
	if resp.LauncherVersion != "v-test" {
		t.Errorf("LauncherVersion: want v-test, got %q", resp.LauncherVersion)
	}
	if resp.SingboxVersion != "1.13.x" {
		t.Errorf("SingboxVersion: want 1.13.x, got %q", resp.SingboxVersion)
	}
	if resp.CapturedAt == "" {
		t.Errorf("CapturedAt must be non-empty")
	}
}

// TestSnapshot_MissingCache — без cache → cache в Missing, остальные на месте.
func TestSnapshot_MissingCache(t *testing.T) {
	execDir := snapshotLayout(t)
	writeSnapshotFile(t, execDir, "template", []byte(`{}`))
	writeSnapshotFile(t, execDir, "state", []byte(`{}`))
	writeSnapshotFile(t, execDir, "config", []byte(`{}`))

	base, tok := snapshotServer(t, execDir)
	st, resp := snapshotGET(t, base, tok)
	if st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	if len(resp.Missing) != 1 || resp.Missing[0] != "cache" {
		t.Errorf("Missing: want [cache], got %v", resp.Missing)
	}
	if _, ok := resp.Files["cache"]; ok {
		t.Errorf("Files[cache] must NOT be present when missing")
	}
}

// TestSnapshot_AllMissing — пустая bin-директория → все четыре в Missing, Files nil/пустой.
func TestSnapshot_AllMissing(t *testing.T) {
	execDir := snapshotLayout(t)
	base, tok := snapshotServer(t, execDir)
	st, resp := snapshotGET(t, base, tok)
	if st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	if len(resp.Missing) != 4 {
		t.Errorf("Missing: want 4 entries, got %d (%v)", len(resp.Missing), resp.Missing)
	}
	if len(resp.Files) != 0 {
		t.Errorf("Files: want empty, got %v", resp.Files)
	}
}

// TestSnapshot_CorruptJSON — битый JSON в одном файле → попадает в Errors, остальные на месте.
func TestSnapshot_CorruptJSON(t *testing.T) {
	execDir := snapshotLayout(t)
	writeSnapshotFile(t, execDir, "template", []byte(`{}`))
	writeSnapshotFile(t, execDir, "state", []byte(`{}`))
	writeSnapshotFile(t, execDir, "cache", []byte(`{}`))
	writeSnapshotFile(t, execDir, "config", []byte(`{not valid json`))

	base, tok := snapshotServer(t, execDir)
	st, resp := snapshotGET(t, base, tok)
	if st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	if msg := resp.Errors["config"]; msg == "" {
		t.Errorf("Errors[config] must be set, got: %v", resp.Errors)
	}
	if _, ok := resp.Files["config"]; ok {
		t.Errorf("Files[config] must NOT be present when JSON invalid")
	}
	for _, name := range []string{"template", "state", "cache"} {
		if _, ok := resp.Files[name]; !ok {
			t.Errorf("Files[%q] must be present", name)
		}
	}
}

// TestSnapshot_NoRedaction — намеренный pin: секреты НЕ маскируются.
// Если кто-то решит «добавить редакцию для безопасности» — тест упадёт.
func TestSnapshot_NoRedaction(t *testing.T) {
	execDir := snapshotLayout(t)
	cfg := `{"experimental":{"clash_api":{"secret":"deadbeef-secret"}},"outbounds":[{"type":"vless","password":"my-vless-pass","uuid":"abcd-uuid"}]}`
	writeSnapshotFile(t, execDir, "config", []byte(cfg))

	base, tok := snapshotServer(t, execDir)
	st, resp := snapshotGET(t, base, tok)
	if st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	got := string(resp.Files["config"])
	for _, secret := range []string{"deadbeef-secret", "my-vless-pass", "abcd-uuid"} {
		if !strings.Contains(got, secret) {
			t.Errorf("expected raw secret %q in response, got: %s", secret, got)
		}
	}
	if strings.Contains(got, "REDACTED") {
		t.Errorf("response must NOT contain 'REDACTED' marker (no-redaction policy): %s", got)
	}
}

// TestSnapshot_FilesAreInlineJSON — файлы возвращаются как inline-JSON-объекты,
// а не как строки-в-строках. Проверяем что распарсенный шаблон — это объект,
// а не string scalar.
func TestSnapshot_FilesAreInlineJSON(t *testing.T) {
	execDir := snapshotLayout(t)
	writeSnapshotFile(t, execDir, "template", []byte(`{"answer":42}`))

	base, tok := snapshotServer(t, execDir)
	_, resp := snapshotGET(t, base, tok)
	raw, ok := resp.Files["template"]
	if !ok {
		t.Fatal("template missing")
	}
	var asObj map[string]any
	if err := json.Unmarshal(raw, &asObj); err != nil {
		t.Fatalf("template should decode as JSON object, got error %v; raw=%s", err, raw)
	}
	if v, _ := asObj["answer"].(float64); v != 42 {
		t.Errorf("inline parse failed: %+v", asObj)
	}
}

// TestSnapshot_AuthRequired — без Authorization → 401.
func TestSnapshot_AuthRequired(t *testing.T) {
	execDir := snapshotLayout(t)
	base, _ := snapshotServer(t, execDir)
	resp, err := http.Get(base + "/debug/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", resp.StatusCode)
	}
}

// TestSnapshot_GETOnly — POST → 405.
func TestSnapshot_GETOnly(t *testing.T) {
	execDir := snapshotLayout(t)
	base, tok := snapshotServer(t, execDir)
	req, _ := http.NewRequest(http.MethodPost, base+"/debug/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status: want 405, got %d", resp.StatusCode)
	}
}
