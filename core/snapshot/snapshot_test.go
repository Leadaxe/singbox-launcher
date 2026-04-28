package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/internal/constants"
)

// layout создаёт временную exec-dir с поддиректориями bin/ и
// bin/wizard_states/. Возвращает execDir; пути готовы к чтению через
// platform.GetWizardTemplatePath / GetWizardStatePath / GetOutboundsCachePath /
// GetConfigPath.
func layout(t *testing.T) string {
	t.Helper()
	execDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(execDir, constants.BinDirName, constants.WizardStatesDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	return execDir
}

// writeFile записывает байты по «названию» (template/state/cache/config),
// разрешая путь через те же helpers, что использует production-код.
func writeFile(t *testing.T, execDir, name string, data []byte) {
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
		t.Fatalf("unknown file %q", name)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestBuild_AllFilesPresent — happy-path: 4 файла → все в Files, missing/errors пусты.
func TestBuild_AllFilesPresent(t *testing.T) {
	execDir := layout(t)
	writeFile(t, execDir, "template", []byte(`{"vars":[]}`))
	writeFile(t, execDir, "state", []byte(`{"version":4}`))
	writeFile(t, execDir, "cache", []byte(`{"version":1}`))
	writeFile(t, execDir, "config", []byte(`{"log":{"level":"info"}}`))

	snap := Build(execDir, "v-test", "1.13.x")

	for _, name := range []string{"template", "state", "cache", "config"} {
		if _, ok := snap.Files[name]; !ok {
			t.Errorf("Files[%q] missing", name)
		}
	}
	if len(snap.Missing) != 0 {
		t.Errorf("Missing must be empty: %v", snap.Missing)
	}
	if len(snap.Errors) != 0 {
		t.Errorf("Errors must be empty: %v", snap.Errors)
	}
	if snap.LauncherVersion != "v-test" || snap.SingboxVersion != "1.13.x" {
		t.Errorf("versions not propagated: %+v", snap)
	}
	if snap.CapturedAt == "" {
		t.Errorf("CapturedAt must be set")
	}
}

// TestBuild_MissingCache — частичная установка (без Update'а ещё не было).
func TestBuild_MissingCache(t *testing.T) {
	execDir := layout(t)
	writeFile(t, execDir, "template", []byte(`{}`))
	writeFile(t, execDir, "state", []byte(`{}`))
	writeFile(t, execDir, "config", []byte(`{}`))

	snap := Build(execDir, "v-test", "1.13.x")

	if len(snap.Missing) != 1 || snap.Missing[0] != "cache" {
		t.Errorf("Missing: want [cache], got %v", snap.Missing)
	}
	if _, ok := snap.Files["cache"]; ok {
		t.Errorf("Files[cache] must NOT be present")
	}
}

// TestBuild_AllMissing — чистая установка: ничего не настроено и не запускалось.
func TestBuild_AllMissing(t *testing.T) {
	execDir := layout(t)

	snap := Build(execDir, "v-test", "1.13.x")

	if len(snap.Missing) != 4 {
		t.Errorf("Missing: want 4 entries, got %d (%v)", len(snap.Missing), snap.Missing)
	}
	if len(snap.Files) != 0 {
		t.Errorf("Files must be empty (nil): %v", snap.Files)
	}
}

// TestBuild_CorruptJSON — битый файл попадает в Errors, не в Missing.
func TestBuild_CorruptJSON(t *testing.T) {
	execDir := layout(t)
	writeFile(t, execDir, "template", []byte(`{}`))
	writeFile(t, execDir, "state", []byte(`{}`))
	writeFile(t, execDir, "cache", []byte(`{}`))
	writeFile(t, execDir, "config", []byte(`{garbage`))

	snap := Build(execDir, "", "")

	if msg := snap.Errors["config"]; msg == "" {
		t.Errorf("Errors[config] must be set: %v", snap.Errors)
	}
	if _, ok := snap.Files["config"]; ok {
		t.Errorf("Files[config] must NOT be present when JSON invalid")
	}
	for _, name := range []string{"template", "state", "cache"} {
		if _, ok := snap.Files[name]; !ok {
			t.Errorf("Files[%q] must be present", name)
		}
	}
}

// TestBuild_JSONCConfigAccepted — config.json реально JSONC (с /** ... */
// маркерами), strict json.Valid его отбрасывал. Регрессия фикса:
// snapshot должен принимать JSONC, нормализуя через jsonc.ToJSON.
func TestBuild_JSONCConfigAccepted(t *testing.T) {
	execDir := layout(t)
	writeFile(t, execDir, "template", []byte(`{}`))
	writeFile(t, execDir, "state", []byte(`{}`))
	writeFile(t, execDir, "cache", []byte(`{}`))
	jsoncConfig := []byte(`{
/** @ParserConfig { "version": 4 } */
"log": { "level": "info" }
/** @ParserSTART */
,"outbounds": []
/** @ParserEND */
}`)
	writeFile(t, execDir, "config", jsoncConfig)

	snap := Build(execDir, "", "")

	if msg, bad := snap.Errors["config"]; bad {
		t.Fatalf("config errored, want accepted: %s", msg)
	}
	raw, ok := snap.Files["config"]
	if !ok {
		t.Fatal("Files[config] must be present after JSONC normalization")
	}
	// После нормализации это должен быть валидный strict-JSON.
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("normalized config must be strict JSON: %v\n%s", err, string(raw))
	}
	if _, has := parsed["log"]; !has {
		t.Error("expected normalized config to retain 'log' field")
	}
}

// TestBuild_NoRedaction — pin'им policy: secrets не маскируются.
// Если кто-то решит «добавить редакцию» — упадёт.
func TestBuild_NoRedaction(t *testing.T) {
	execDir := layout(t)
	cfg := `{"experimental":{"clash_api":{"secret":"deadbeef-secret"}},"outbounds":[{"type":"vless","password":"my-pass","uuid":"abcd"}]}`
	writeFile(t, execDir, "config", []byte(cfg))

	snap := Build(execDir, "", "")
	got := string(snap.Files["config"])
	for _, secret := range []string{"deadbeef-secret", "my-pass", "abcd"} {
		if !strings.Contains(got, secret) {
			t.Errorf("secret %q must pass through unredacted; got: %s", secret, got)
		}
	}
	if strings.Contains(got, "REDACTED") {
		t.Errorf("response must NOT contain 'REDACTED' marker")
	}
}

// TestBuild_FilesAreInlineJSON — содержимое файлов — inline-объект, не строка.
func TestBuild_FilesAreInlineJSON(t *testing.T) {
	execDir := layout(t)
	writeFile(t, execDir, "template", []byte(`{"answer":42}`))

	snap := Build(execDir, "", "")
	raw, ok := snap.Files["template"]
	if !ok {
		t.Fatal("template missing")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("must decode as object: %v; raw=%s", err, raw)
	}
	if v, _ := obj["answer"].(float64); v != 42 {
		t.Errorf("inline parse wrong: %+v", obj)
	}
}

// TestBuild_EmptyExecDir — несуществующая exec-dir → 4 в Missing, no panic.
// Защищаемся от программной ошибки вызывающего (передал пустую/несуществующую path).
func TestBuild_EmptyExecDir(t *testing.T) {
	snap := Build("/this/path/does/not/exist", "", "")
	if len(snap.Missing) != 4 {
		t.Errorf("Missing must list all 4 files for nonexistent dir, got: %v", snap.Missing)
	}
}
