package core

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
)

// withAppVersion temporarily overrides constants.AppVersion for a test scope
// so the function-under-test reads the version we want without us touching
// the global outside the closure.
func withAppVersion(t *testing.T, v string, fn func()) {
	t.Helper()
	prev := constants.AppVersion
	constants.AppVersion = v
	t.Cleanup(func() { constants.AppVersion = prev })
	fn()
}

// makeTempLauncherDir builds an exec-dir-shaped layout: <root>/bin/, with
// optional pre-existing wizard_template.json and bin/settings.json contents.
func makeTempLauncherDir(t *testing.T, withTemplate bool, settingsJSON string) string {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if withTemplate {
		if err := os.WriteFile(filepath.Join(binDir, "wizard_template.json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write template: %v", err)
		}
	}
	if settingsJSON != "" {
		if err := os.WriteFile(filepath.Join(binDir, "settings.json"), []byte(settingsJSON), 0o644); err != nil {
			t.Fatalf("write settings: %v", err)
		}
	}
	return root
}

func templateExists(t *testing.T, root string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, "bin", "wizard_template.json"))
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat template: %v", err)
	return false
}

func TestInvalidateTemplateIfStale_LegacyEmptyMarker_RemovesTemplate(t *testing.T) {
	// settings.json without last_template_launcher_version (legacy install).
	root := makeTempLauncherDir(t, true, `{"lang":"en"}`)
	withAppVersion(t, "v0.8.8", func() {
		if err := InvalidateTemplateIfStale(root); err != nil {
			t.Fatalf("invalidate: %v", err)
		}
		if templateExists(t, root) {
			t.Fatal("expected template to be removed (legacy empty marker)")
		}
	})
}

func TestInvalidateTemplateIfStale_OlderVersion_RemovesTemplate(t *testing.T) {
	root := makeTempLauncherDir(t, true, `{"lang":"en","last_template_launcher_version":"v0.8.7"}`)
	withAppVersion(t, "v0.8.8", func() {
		if err := InvalidateTemplateIfStale(root); err != nil {
			t.Fatalf("invalidate: %v", err)
		}
		if templateExists(t, root) {
			t.Fatal("expected template to be removed (last < current)")
		}
	})
}

func TestInvalidateTemplateIfStale_SameVersion_KeepsTemplate(t *testing.T) {
	root := makeTempLauncherDir(t, true, `{"lang":"en","last_template_launcher_version":"v0.8.8"}`)
	withAppVersion(t, "v0.8.8", func() {
		if err := InvalidateTemplateIfStale(root); err != nil {
			t.Fatalf("invalidate: %v", err)
		}
		if !templateExists(t, root) {
			t.Fatal("expected template to be kept (last == current)")
		}
	})
}

func TestInvalidateTemplateIfStale_NewerVersion_KeepsTemplate(t *testing.T) {
	// Downgrade scenario: last installed by a *newer* launcher than current.
	// Don't touch the file — user knows what they're doing.
	root := makeTempLauncherDir(t, true, `{"lang":"en","last_template_launcher_version":"v0.8.9"}`)
	withAppVersion(t, "v0.8.8", func() {
		if err := InvalidateTemplateIfStale(root); err != nil {
			t.Fatalf("invalidate: %v", err)
		}
		if !templateExists(t, root) {
			t.Fatal("expected template to be kept on downgrade")
		}
	})
}

func TestInvalidateTemplateIfStale_DevBuild_SkipsEntirely(t *testing.T) {
	root := makeTempLauncherDir(t, true, `{"lang":"en","last_template_launcher_version":"v0.8.7"}`)
	for _, v := range []string{"v-local-test", "unnamed-dev", "v0.8.7-3-gabc1234-dirty"} {
		v := v
		t.Run(v, func(t *testing.T) {
			withAppVersion(t, v, func() {
				if err := InvalidateTemplateIfStale(root); err != nil {
					t.Fatalf("invalidate: %v", err)
				}
				if !templateExists(t, root) {
					t.Fatal("expected dev build to leave template alone")
				}
			})
		})
	}
}

func TestInvalidateTemplateIfStale_NoTemplate_NoOp(t *testing.T) {
	// Fresh install: settings.json may or may not exist, no template file.
	// Should not error.
	root := makeTempLauncherDir(t, false, "")
	withAppVersion(t, "v0.8.8", func() {
		if err := InvalidateTemplateIfStale(root); err != nil {
			t.Fatalf("invalidate: %v", err)
		}
	})
}
