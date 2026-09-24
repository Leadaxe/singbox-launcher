package paths

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// migFile — файл унаследованной раскладки: путь относительно app, режим, содержимое.
type migFile struct {
	rel  string
	perm os.FileMode
	data []byte
}

// legacyFixture строит унаследованную раскладку app/bin (+ app/logs вне bin).
func legacyFixture(t *testing.T, app string) []migFile {
	t.Helper()
	srs := bytes.Repeat([]byte("srs-payload-"), 700) // ~8 КБ
	files := []migFile{
		{"bin/wizard_states/state.json", 0o644, []byte(`{"version":3,"selected":"proxy-a"}`)},
		{"bin/subscriptions/x.txt", 0o644, []byte("vless://example\n")},
		{"bin/rule-sets/geo.srs", 0o644, srs},
		{"bin/settings.json", 0o644, []byte(`{"lang":"ru"}`)},
		{"bin/daemon/key.pem", 0o600, []byte("-----BEGIN KEY-----\nabc\n")},
		{"bin/sing-box", 0o755, []byte{0x7f, 'E'}},
		{"logs/launcher.log", 0o644, []byte("old log\n")},
	}
	for _, f := range files {
		p := filepath.Join(app, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, f.data, f.perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, f.perm); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"bin/daemon", "bin/remote-daemons"} {
		p := filepath.Join(app, filepath.FromSlash(d))
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// snapshot — дерево root: относительный путь → режим и содержимое.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return out
	}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		v := info.Mode().String()
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			v += " " + string(b)
		}
		out[filepath.ToSlash(rel)] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameSnapshot(t *testing.T, what string, want, got map[string]string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d entries, want %d", what, len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %s changed", what, k)
		}
	}
}

func migLayout(app, data string, mode Mode) Layout {
	return Layout{App: AppDir(app), Data: DataDir(data), Logs: LogDir(filepath.Join(data, "logs")), Mode: mode}
}

func put(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMigrateLegacyData(t *testing.T) {
	var logged []string
	logf := func(format string, args ...interface{}) {
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	t.Run("happy", func(t *testing.T) {
		app, data := t.TempDir(), filepath.Join(t.TempDir(), "SingboxLauncher")
		files := legacyFixture(t, app)
		before := snapshot(t, app)
		logged = nil

		res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), logf)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Migrated || res.DataHadState || res.Source != filepath.Join(app, "bin") {
			t.Fatalf("result = %+v", res)
		}
		if res.Report.Files == 0 || res.Report.Skipped != 0 {
			t.Fatalf("report = %+v", res.Report)
		}
		if len(logged) != 1 || !strings.HasPrefix(logged[0], "migration: ") || !strings.Contains(logged[0], " skipped=0") {
			t.Fatalf("log = %q", logged)
		}

		for _, f := range files {
			if !strings.HasPrefix(f.rel, "bin/") {
				continue
			}
			if got := mustRead(t, filepath.Join(data, filepath.FromSlash(f.rel))); got != string(f.data) {
				t.Fatalf("%s content differs", f.rel)
			}
		}
		if _, err := os.Stat(filepath.Join(data, "logs", "launcher.log")); !os.IsNotExist(err) {
			t.Fatalf("logs must not be copied: %v", err)
		}
		if runtime.GOOS != "windows" {
			for rel, want := range map[string]os.FileMode{
				"bin/daemon":         0o700,
				"bin/remote-daemons": 0o700,
				"bin/daemon/key.pem": 0o600,
				"bin/sing-box":       0o755,
			} {
				fi, err := os.Stat(filepath.Join(data, filepath.FromSlash(rel)))
				if err != nil {
					t.Fatal(err)
				}
				if fi.Mode().Perm() != want {
					t.Fatalf("%s perm = %o, want %o", rel, fi.Mode().Perm(), want)
				}
			}
		}
		sameSnapshot(t, "source", before, snapshot(t, app))
		if got := mustRead(t, filepath.Join(data, ".migrated_from")); got != filepath.Join(app, "bin")+"\n" {
			t.Fatalf(".migrated_from = %q", got)
		}
		if _, err := os.Stat(filepath.Join(data, "bin.migrating")); !os.IsNotExist(err) {
			t.Fatalf("bin.migrating left behind: %v", err)
		}

		t.Run("idempotent", func(t *testing.T) {
			put(t, filepath.Join(data, "bin", "settings.json"), `{"lang":"en"}`)
			after := snapshot(t, data)
			res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), logf)
			if err != nil {
				t.Fatal(err)
			}
			if res.Migrated || !res.DataHadState {
				t.Fatalf("result = %+v", res)
			}
			sameSnapshot(t, "data", after, snapshot(t, data))
		})
	})

	t.Run("no source", func(t *testing.T) {
		app, data := t.TempDir(), t.TempDir()
		put(t, filepath.Join(data, "keep.txt"), "x")
		before := snapshot(t, data)
		res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.Migrated || res.Source != "" {
			t.Fatalf("result = %+v", res)
		}
		sameSnapshot(t, "data", before, snapshot(t, data))
	})

	t.Run("wrong mode", func(t *testing.T) {
		for _, mode := range []Mode{ModePortable, ModeLegacy} {
			app, data := t.TempDir(), filepath.Join(t.TempDir(), "d")
			legacyFixture(t, app)
			res, err := MigrateLegacyData(migLayout(app, data, mode), nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.Migrated {
				t.Fatalf("%s: migrated", mode)
			}
			if _, err := os.Stat(data); !os.IsNotExist(err) {
				t.Fatalf("%s: data dir created", mode)
			}
		}
	})

	t.Run("existing dst", func(t *testing.T) {
		app, data := t.TempDir(), t.TempDir()
		legacyFixture(t, app)
		// Как после EnsureDirectories и первого старта без state.json.
		if err := os.MkdirAll(filepath.Join(data, "bin", "rule-sets"), 0o755); err != nil {
			t.Fatal(err)
		}
		put(t, filepath.Join(data, "bin", "settings.json"), `{"fresh":true}`)
		res, err := MigrateLegacyData(migLayout(app, data, ModeEnv), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Migrated {
			t.Fatalf("result = %+v", res)
		}
		if got := mustRead(t, filepath.Join(data, "bin", "settings.json")); got != `{"lang":"ru"}` {
			t.Fatalf("settings.json = %q, want source copy", got)
		}
		if _, err := os.Stat(filepath.Join(data, "bin", "rule-sets", "geo.srs")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(data, "bin.migrating")); !os.IsNotExist(err) {
			t.Fatalf("bin.migrating left behind: %v", err)
		}
	})

	t.Run("stale tmp", func(t *testing.T) {
		app, data := t.TempDir(), t.TempDir()
		legacyFixture(t, app)
		put(t, filepath.Join(data, "bin.migrating", "junk.txt"), "junk")
		res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Migrated {
			t.Fatalf("result = %+v", res)
		}
		for _, p := range []string{
			filepath.Join(data, "bin.migrating"),
			filepath.Join(data, "bin", "junk.txt"),
		} {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Fatalf("%s must not exist: %v", p, err)
			}
		}
		if _, err := os.Stat(filepath.Join(data, "bin", "wizard_states", "state.json")); err != nil {
			t.Fatal(err)
		}
	})

	// Свежий lock другого экземпляра — миграция пропускается; брошенный
	// (старше TTL) — сносится, миграция идёт, lock после неё убран.
	t.Run("lock", func(t *testing.T) {
		app, data := t.TempDir(), t.TempDir()
		legacyFixture(t, app)
		lock := filepath.Join(data, migrationLockName)
		put(t, lock, "1\n")
		res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), nil)
		if err != nil || res.Migrated || !res.Busy {
			t.Fatalf("fresh lock: %+v %v", res, err)
		}
		old := time.Now().Add(-2 * migrationLockTTL)
		if err := os.Chtimes(lock, old, old); err != nil {
			t.Fatal(err)
		}
		res, err = MigrateLegacyData(migLayout(app, data, ModeSystem), nil)
		if err != nil || !res.Migrated {
			t.Fatalf("stale lock: %+v %v", res, err)
		}
		if _, err := os.Stat(lock); !os.IsNotExist(err) {
			t.Fatalf("lock must be removed: %v", err)
		}
	})

	t.Run("unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("chmod 000 does not deny read here")
		}
		app, data := t.TempDir(), t.TempDir()
		legacyFixture(t, app)
		secret := filepath.Join(app, "bin", "subscriptions", "secret.txt")
		put(t, secret, "hidden")
		if err := os.Chmod(secret, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(secret, 0o644)

		res, err := MigrateLegacyData(migLayout(app, data, ModeSystem), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Migrated || res.Report.Skipped != 1 {
			t.Fatalf("result = %+v", res)
		}
		if len(res.Report.SkippedExamples) == 0 || !strings.Contains(res.Report.SkippedExamples[0], "subscriptions/secret.txt") {
			t.Fatalf("examples = %v", res.Report.SkippedExamples)
		}
		for _, rel := range []string{"subscriptions/x.txt", "wizard_states/state.json", "rule-sets/geo.srs", "daemon/key.pem"} {
			if _, err := os.Stat(filepath.Join(data, "bin", filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := os.Stat(filepath.Join(data, "bin", "subscriptions", "secret.txt")); !os.IsNotExist(err) {
			t.Fatalf("secret.txt must be skipped: %v", err)
		}
	})
}

func TestCopyTree(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privilege on windows")
		}
		src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
		put(t, filepath.Join(src, "a.txt"), "a")
		if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
			t.Fatal(err)
		}
		rep, err := CopyTree(src, dst)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Files != 2 || rep.Skipped != 0 {
			t.Fatalf("report = %+v", rep)
		}
		fi, err := os.Lstat(filepath.Join(dst, "link"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("link copied as %s, want symlink", fi.Mode())
		}
		if target, _ := os.Readlink(filepath.Join(dst, "link")); target != "a.txt" {
			t.Fatalf("link target = %q", target)
		}
	})

	// Корень-симлинк: разворачивается, копируется содержимое цели (раньше
	// Walk принимал корень за ссылку и продвижение падало на пустом dirs).
	t.Run("symlink root", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privilege on windows")
		}
		real, base := t.TempDir(), t.TempDir()
		put(t, filepath.Join(real, "wizard_states", "state.json"), "s")
		src := filepath.Join(base, "bin")
		if err := os.Symlink(real, src); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(base, "out")
		rep, err := CopyTree(src, dst)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Files != 1 || rep.Skipped != 0 {
			t.Fatalf("report = %+v", rep)
		}
		if got := mustRead(t, filepath.Join(dst, "wizard_states", "state.json")); got != "s" {
			t.Fatalf("state = %q", got)
		}
		if fi, err := os.Lstat(dst); err != nil || !fi.IsDir() {
			t.Fatalf("dst must be a real dir: %v %v", fi, err)
		}
	})

	t.Run("src not a dir", func(t *testing.T) {
		base := t.TempDir()
		file := filepath.Join(base, "f")
		put(t, file, "x")
		if _, err := CopyTree(file, filepath.Join(base, "out")); err == nil {
			t.Fatal("want error for file source")
		}
		if _, err := CopyTree(filepath.Join(base, "missing"), filepath.Join(base, "out2")); err == nil {
			t.Fatal("want error for missing source")
		}
		if _, err := os.Stat(filepath.Join(base, "out.migrating")); !os.IsNotExist(err) {
			t.Fatalf("tmp created for bad source: %v", err)
		}
	})
}
