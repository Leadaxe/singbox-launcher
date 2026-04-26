package locale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// MarkTemplateInstalled is a thin wrapper over LoadSettings/SaveSettings —
// most of the behavior is covered transitively, but we want explicit
// guarantees for the SPEC 046 invalidation contract: the version is written,
// other fields are not clobbered, and a no-op call is truly cheap.

func readSettings(t *testing.T, binDir string) Settings {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	return s
}

func TestMarkTemplateInstalled_WritesField(t *testing.T) {
	binDir := t.TempDir()
	if err := MarkTemplateInstalled(binDir, "v0.8.8"); err != nil {
		t.Fatalf("MarkTemplateInstalled: %v", err)
	}
	got := readSettings(t, binDir)
	if got.LastTemplateLauncherVersion != "v0.8.8" {
		t.Fatalf("LastTemplateLauncherVersion = %q, want %q", got.LastTemplateLauncherVersion, "v0.8.8")
	}
}

func TestMarkTemplateInstalled_PreservesOtherFields(t *testing.T) {
	binDir := t.TempDir()
	// Pre-seed with several unrelated settings — the helper must not lose
	// them.
	seed := Settings{
		Lang:                           "ru",
		PingTestURL:                    "https://example.com/ping",
		PingTestAllConcurrency:         8,
		SubscriptionAutoUpdateDisabled: true,
	}
	if err := SaveSettings(binDir, seed); err != nil {
		t.Fatalf("SaveSettings seed: %v", err)
	}
	if err := MarkTemplateInstalled(binDir, "v0.8.8"); err != nil {
		t.Fatalf("MarkTemplateInstalled: %v", err)
	}
	got := readSettings(t, binDir)
	if got.Lang != "ru" || got.PingTestURL != "https://example.com/ping" ||
		got.PingTestAllConcurrency != 8 || !got.SubscriptionAutoUpdateDisabled {
		t.Fatalf("MarkTemplateInstalled clobbered unrelated fields: %+v", got)
	}
	if got.LastTemplateLauncherVersion != "v0.8.8" {
		t.Fatalf("LastTemplateLauncherVersion = %q, want %q", got.LastTemplateLauncherVersion, "v0.8.8")
	}
}

func TestMarkTemplateInstalled_NoOpWhenSame(t *testing.T) {
	// When the recorded version already matches, Save should be skipped
	// (mtime-stable, useful for telemetry / fs-watch consumers).
	binDir := t.TempDir()
	if err := MarkTemplateInstalled(binDir, "v0.8.8"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	path := filepath.Join(binDir, "settings.json")
	st1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after first write: %v", err)
	}
	// Second call with same version — file should not be rewritten.
	if err := MarkTemplateInstalled(binDir, "v0.8.8"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	st2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after second write: %v", err)
	}
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatalf("expected no rewrite on idempotent call (mtime changed: %v vs %v)", st1.ModTime(), st2.ModTime())
	}
}
