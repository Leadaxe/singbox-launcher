package debugapi

// Гейт мажора схемы состояния (SPEC 118 §4.G, Т10).
//
// Проверяется свойство, а не текст: операции, которые ПЕРЕНОСЯТ состояние
// (copy-from) или правят его ПО ЧАСТЯМ (PATCH /state/rules, /state/dns),
// отказываются работать с файлом чужого мажора и называют обе версии; при
// совпадении мажора работают как раньше. Чтение (GET) не гейтуется никогда —
// диагностика обязана оставаться доступной именно в момент расхождения.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// writeMachineStateOfSchema кладёт машине state-файл ЗАДАННОГО мажора.
//
// Файл собирается вручную, а не через state.Save: Save всегда пишет текущую
// схему, и «состояние из будущего» через него невыразимо — ровно то, что здесь
// и требуется подделать.
func writeMachineStateOfSchema(t *testing.T, execDir, id string, major int) string {
	t.Helper()
	path := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `{"meta":{"version":` + strconv.Itoa(major) + `,"schema":"future"},"sources":[],"rules":[]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

// seedMachines кладёт в реестр несколько машин разом: copy-from требует и
// исходника, и приёмника, а seedMachine пишет файл реестра целиком.
func seedMachines(t *testing.T, execDir string, ids []string, addr string) {
	t.Helper()
	binDir := platform.GetBinDir(execDir)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, `{"id":"`+id+`","name":"`+id+`","addr":"`+addr+`"}`)
	}
	raw := "[" + strings.Join(parts, ",") + "]"
	if err := os.WriteFile(filepath.Join(binDir, "remote-daemons.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}

// mismatchBody — разобранный отказ гейта.
type mismatchBody struct {
	Error     string `json:"error"`
	Found     int    `json:"schema_found"`
	Supported int    `json:"schema_supported"`
}

// requireMismatch — отказ обязан быть 409 и назвать ОБЕ версии. Именно обе:
// одна версия не отвечает на вопрос «кого обновлять».
func requireMismatch(t *testing.T, what string, resp *http.Response, body []byte, wantFound int) {
	t.Helper()
	requireMismatchStatus(t, what, resp.StatusCode, body, wantFound)
}

// requireMismatchStatus — та же проверка по (статус, тело): локальные
// endpoint'ы ходят через doJSON, который отдаёт именно эту пару.
func requireMismatchStatus(t *testing.T, what string, status int, body []byte, wantFound int) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("%s: status %d (%s), want 409", what, status, body)
	}
	var got mismatchBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%s: parse body %s: %v", what, body, err)
	}
	if got.Found != wantFound {
		t.Errorf("%s: schema_found = %d, want %d", what, got.Found, wantFound)
	}
	if got.Supported != state.SchemaMajor {
		t.Errorf("%s: schema_supported = %d, want %d", what, got.Supported, state.SchemaMajor)
	}
	// Обе версии обязаны быть и в человекочитаемом тексте: пользователь видит
	// сообщение, а не JSON-поля.
	if !strings.Contains(got.Error, "v"+strconv.Itoa(wantFound)) ||
		!strings.Contains(got.Error, "v"+strconv.Itoa(state.SchemaMajor)) {
		t.Errorf("%s: текст отказа не называет обе версии: %q", what, got.Error)
	}
}

// §4.G: PATCH /state/rules и /state/dns удалённой машины, чей state написан
// БОЛЕЕ НОВОЙ схемой, отказывают; GET той же машины продолжает читаться.
func TestRemoteStatePatchRefusesNewerSchema(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)
	future := state.SchemaMajor + 1
	writeMachineStateOfSchema(t, execDir, "router", future)

	resp, body := authDo(t, http.MethodPatch, base+"/remote/machines/router/state/rules",
		map[string]any{"mode": "replace", "rules": []any{}})
	requireMismatch(t, "PATCH state/rules", resp, body, future)

	resp, body = authDo(t, http.MethodPatch, base+"/remote/machines/router/state/dns",
		map[string]any{"servers": []any{}, "rules": []any{}})
	requireMismatch(t, "PATCH state/dns", resp, body, future)

	// Файл не тронут: отказ обязан быть отказом, а не половиной записи.
	path := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "router")
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if v, verr := state.SchemaVersionOfBytes(after); verr != nil || v != future {
		t.Fatalf("файл переписан отказавшим PATCH'ем: version=%d err=%v", v, verr)
	}
}

// §4.G, вторая половина: при СОВПАДЕНИИ мажора те же PATCH'и работают как
// раньше. Без этой половины гейт легко «пройти», запретив всё подряд.
func TestRemoteStatePatchWorksOnMatchingSchema(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)

	path := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "router")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := state.New().Save(path); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	resp, body := authDo(t, http.MethodPatch, base+"/remote/machines/router/state/rules",
		map[string]any{"mode": "replace", "rules": []any{}})
	if resp.StatusCode != 200 {
		t.Fatalf("PATCH state/rules на своей схеме: status %d (%s)", resp.StatusCode, body)
	}
	resp, body = authDo(t, http.MethodPatch, base+"/remote/machines/router/state/dns",
		map[string]any{"servers": []any{}, "rules": []any{}})
	if resp.StatusCode != 200 {
		t.Fatalf("PATCH state/dns на своей схеме: status %d (%s)", resp.StatusCode, body)
	}
}

// §4.G: диагностика не гейтуется. GET /state/full машины из будущего обязан
// отвечать: закрыть чтение в момент расхождения — значит отнять у пользователя
// единственный способ увидеть, ЧТО именно разошлось.
func TestRemoteStateGetNotGatedBySchema(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachine(t, execDir, "router", addr)
	writeMachineStateOfSchema(t, execDir, "router", state.SchemaMajor+1)

	resp, body := authDo(t, http.MethodGet, base+"/remote/machines/router/state/rules", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET state/rules на чужой схеме: status %d (%s), чтение гейтить нельзя", resp.StatusCode, body)
	}
}

// §4.G: /profile/copy-from гейтуется по ИСХОДНИКУ — переносится именно его
// файл. Расхождение = 409 с обеими версиями, а приёмник не тронут.
func TestRemoteProfileCopyFromRefusesNewerSchema(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachines(t, execDir, []string{"src", "dst"}, addr)
	future := state.SchemaMajor + 1
	writeMachineStateOfSchema(t, execDir, "src", future)

	resp, body := authDo(t, http.MethodPost, base+"/remote/machines/dst/profile/copy-from",
		map[string]any{"source_id": "src", "overwrite": true})
	requireMismatch(t, "copy-from", resp, body, future)

	dstPath := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "dst")
	if _, err := os.Stat(dstPath); !os.IsNotExist(err) {
		t.Fatalf("отказавший copy-from всё равно создал state приёмника (%v)", err)
	}
}

// §4.G, вторая половина для copy-from: на своей схеме перенос идёт.
func TestRemoteProfileCopyFromWorksOnMatchingSchema(t *testing.T) {
	daemon := httptest.NewServer(fakeDaemonMux(nil))
	defer daemon.Close()
	addr := strings.TrimPrefix(daemon.URL, "http://")

	base, execDir, _ := newRemoteTestServer(t)
	seedMachines(t, execDir, []string{"src", "dst"}, addr)

	srcPath := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "src")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := state.New().Save(srcPath); err != nil {
		t.Fatalf("seed src state: %v", err)
	}

	resp, body := authDo(t, http.MethodPost, base+"/remote/machines/dst/profile/copy-from",
		map[string]any{"source_id": "src", "overwrite": true})
	if resp.StatusCode != 200 {
		t.Fatalf("copy-from на своей схеме: status %d (%s)", resp.StatusCode, body)
	}
	dstPath := platform.GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, "dst")
	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("copy-from не создал state приёмника: %v", err)
	}
}

// §4.G, локальная сторона: PATCH /state/rules и /state/dns своей машины
// гейтуются той же проверкой.
//
// Гейт не про удалённые машины как таковые — он про ЧАСТИЧНУЮ ПРАВКУ чужой
// формы, а состояние из будущего локально появляется тем же способом: откат
// на старую сборку после апгрейда. Пропустить здесь значило бы дать ей
// урезать собственный state.json.
func TestLocalStatePatchRefusesNewerSchema(t *testing.T) {
	execDir := t.TempDir()
	path := platform.GetWizardStatePath(execDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	future := state.SchemaMajor + 1
	raw := `{"meta":{"version":` + strconv.Itoa(future) + `,"schema":"future"},"sources":[],"rules":[]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	ff := &fakeFacade{execDir: execDir, stateValue: state.New()}
	base, _ := newTestServer(t, ff)

	status, body := doJSON(t, authedReq(t, "PATCH", base+"/state/rules", []byte(`{"mode":"replace","rules":[]}`)), nil)
	requireMismatchStatus(t, "локальный PATCH state/rules", status, body, future)
	if ff.savedState != nil {
		t.Errorf("отказавший PATCH всё равно сохранил состояние")
	}

	status, body = doJSON(t, authedReq(t, "PATCH", base+"/state/dns", []byte(`{"servers":[],"rules":[]}`)), nil)
	requireMismatchStatus(t, "локальный PATCH state/dns", status, body, future)
	if ff.savedState != nil {
		t.Errorf("отказавший PATCH всё равно сохранил состояние")
	}
}
