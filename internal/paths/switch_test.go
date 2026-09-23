package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"singbox-launcher/internal/constants"
)

// TestSwitchPortable — переезд данных переключателем Portable в обе стороны
// (SPEC 135 §4.2) и раскладка, которую после него выберет Resolve.
func TestSwitchPortable(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	data := filepath.Join(root, "data")
	exe := filepath.Join(app, "singbox-launcher")
	home := filepath.Join(root, "home")
	env := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	always := func(string) bool { return true }

	write := func(p, body string, perm os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), perm); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		return string(b)
	}
	mustAbsent := func(p string) {
		t.Helper()
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s must be absent, lstat err=%v", p, err)
		}
	}
	resolveMode := func(t *testing.T) Mode {
		t.Helper()
		l, err := Resolve(exe, env, "linux", always)
		if err != nil {
			t.Fatal(err)
		}
		return l.Mode
	}

	sys := Layout{App: AppDir(app), Data: DataDir(data), Logs: LogDir(filepath.Join(data, "logs")), Mode: ModeSystem}
	dataBin := sys.Data.Bin()
	appBin := sys.App.Bin()
	statePath := filepath.Join(constants.WizardStatesDirName, constants.WizardStateFileName)

	write(filepath.Join(dataBin, statePath), "state", 0o644)
	write(filepath.Join(dataBin, "subscriptions", "sub.json"), "sub", 0o644)
	write(filepath.Join(dataBin, "settings.json"), "settings", 0o644)
	write(filepath.Join(dataBin, "daemon", "client.key"), "key", 0o600)
	if err := os.Chmod(filepath.Join(dataBin, "daemon"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(string(data), constants.MigratedFromMarkerFileName), "/old\n", 0o644)
	write(filepath.Join(data, "logs", "main.log"), "log", 0o644)
	write(filepath.Join(appBin, "wizard_template.json"), "shipped", 0o644)

	target := Layout{}

	t.Run("to portable", func(t *testing.T) {
		rep, err := SwitchToPortable(sys)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Leftover != "" {
			t.Fatalf("leftover %q", rep.Leftover)
		}
		if _, err := os.Stat(filepath.Join(app, constants.PortableMarkerFileName)); err != nil {
			t.Fatalf("marker: %v", err)
		}
		for rel, want := range map[string]string{
			statePath:                "state",
			"subscriptions/sub.json": "sub",
			"settings.json":          "settings",
			"daemon/client.key":      "key",
			"wizard_template.json":   "shipped",
		} {
			if got := read(filepath.Join(appBin, filepath.FromSlash(rel))); got != want {
				t.Fatalf("%s = %q, want %q", rel, got, want)
			}
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(filepath.Join(appBin, "daemon"))
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("daemon perm: %v %v", info, err)
			}
		}
		mustAbsent(dataBin)
		mustAbsent(filepath.Join(data, constants.MigratedFromMarkerFileName))
		if got := read(filepath.Join(data, "logs", "main.log")); got != "log" {
			t.Fatalf("logs touched: %q", got)
		}
		if m := resolveMode(t); m != ModePortable {
			t.Fatalf("mode %s, want portable", m)
		}
	})

	t.Run("to system", func(t *testing.T) {
		l, err := Resolve(exe, env, "linux", always)
		if err != nil {
			t.Fatal(err)
		}
		target, err = SystemDefault(exe, env, "linux", always)
		if err != nil {
			t.Fatal(err)
		}
		if target.Mode != ModeSystem || !strings.HasPrefix(string(target.Data), home) {
			t.Fatalf("system default %+v", target)
		}
		write(filepath.Join(app, "logs", "main.log"), "portable log", 0o644)

		rep, err := SwitchToSystem(l, target)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Leftover != "" {
			t.Fatalf("leftover %q", rep.Leftover)
		}
		if got := read(filepath.Join(target.Data.Bin(), statePath)); got != "state" {
			t.Fatalf("state = %q", got)
		}
		if got := read(filepath.Join(target.Data.Bin(), "wizard_template.json")); got != "shipped" {
			t.Fatalf("template = %q", got)
		}
		mustAbsent(filepath.Join(app, constants.PortableMarkerFileName))
		mustAbsent(appBin)
		if got := read(filepath.Join(app, "logs", "main.log")); got != "portable log" {
			t.Fatalf("app logs touched: %q", got)
		}
		if m := resolveMode(t); m != ModeSystem {
			t.Fatalf("mode %s, want system", m)
		}
	})

	t.Run("to system with undeletable leftover", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("permission bits do not block deletion here")
		}
		// Снова portable-раскладка: данные в app/bin, один подкаталог
		// без права записи — его содержимое не стереть.
		write(filepath.Join(appBin, statePath), "state2", 0o644)
		locked := filepath.Join(appBin, "locked")
		write(filepath.Join(locked, "f"), "x", 0o644)
		if err := os.Chmod(locked, 0o500); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(app, constants.PortableMarkerFileName)
		write(marker, "portable\n", 0o644)
		t.Cleanup(func() {
			matches, _ := filepath.Glob(filepath.Join(app, MovedBinPrefix+"*", "locked"))
			for _, m := range matches {
				_ = os.Chmod(m, 0o700)
			}
		})

		l, err := Resolve(exe, env, "linux", always)
		if err != nil || l.Mode != ModePortable {
			t.Fatalf("resolve %+v %v", l, err)
		}
		rep, err := SwitchToSystem(l, target)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(filepath.Base(rep.Leftover), MovedBinPrefix) || filepath.Dir(rep.Leftover) != app {
			t.Fatalf("leftover %q, want %s/%s*", rep.Leftover, app, MovedBinPrefix)
		}
		mustAbsent(appBin)
		mustAbsent(marker)
		if got := read(filepath.Join(target.Data.Bin(), statePath)); got != "state2" {
			t.Fatalf("state = %q", got)
		}
		if m := resolveMode(t); m != ModeSystem {
			t.Fatalf("mode %s, want system", m)
		}
	})

	t.Run("env layout refused", func(t *testing.T) {
		envLayout := Layout{App: AppDir(app), Data: DataDir(filepath.Join(root, "env")), Mode: ModeEnv}
		if _, err := SwitchToPortable(envLayout); err != ErrEnvLayout {
			t.Fatalf("to portable in env: %v", err)
		}
		if _, err := SwitchToSystem(envLayout, target); err != ErrEnvLayout {
			t.Fatalf("to system in env: %v", err)
		}
	})
}
