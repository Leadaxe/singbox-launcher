package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPurge — план и выполнение очистки (SPEC 135 §4.3) на реальных
// каталогах: состав плана, сохранность поставляемого и portable.txt,
// снятая отметка, системный каталог при portable.
func TestPurge(t *testing.T) {
	write := func(t *testing.T, p, data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	noEnv := func(string) string { return "" }
	probe := func(string) bool { return true }

	// systemFixture — системная раскладка после миграции из app/bin.
	type fixture struct{ root, app, data, logs, exe, moved, oldLogs, oldBin string }
	systemFixture := func(t *testing.T) (fixture, Layout) {
		root := t.TempDir()
		f := fixture{
			root:    root,
			app:     filepath.Join(root, "app"),
			data:    filepath.Join(root, "data"),
			logs:    filepath.Join(root, "logs"),
			moved:   filepath.Join(root, "app", "bin.moved-20260101-000000"),
			oldLogs: filepath.Join(root, "app", "logs"),
			oldBin:  filepath.Join(root, "app", "bin"),
		}
		f.exe = filepath.Join(f.app, "singbox-launcher")
		write(t, f.exe, "exe")
		write(t, filepath.Join(f.app, "portable.txt"), "")
		write(t, filepath.Join(f.oldBin, "wizard_template.json"), `{"shipped":true}`)
		write(t, filepath.Join(f.oldBin, "wizard_states", "state.json"), `{"old":true}`)
		write(t, filepath.Join(f.moved, "x"), "moved")
		write(t, filepath.Join(f.oldLogs, "old.log"), "old log line")
		write(t, filepath.Join(f.data, "bin", "wizard_states", "state.json"), `{"version":3}`)
		write(t, filepath.Join(f.data, "bin", "config.json"), `{}`)
		write(t, filepath.Join(f.data, ".migrated_from"), f.oldBin+"\n")
		write(t, filepath.Join(f.logs, "launcher.log"), "log line")
		l := Layout{App: AppDir(f.app), Data: DataDir(f.data), Logs: LogDir(f.logs), Mode: ModeSystem}
		return f, l
	}

	findItem := func(p PurgePlan, path string) *PurgeItem {
		for i := range p.Items {
			if filepath.Clean(p.Items[i].Path) == filepath.Clean(path) {
				return &p.Items[i]
			}
		}
		return nil
	}
	gone := func(t *testing.T, p string) {
		t.Helper()
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s: want removed, stat err=%v", p, err)
		}
	}
	kept := func(t *testing.T, p string) {
		t.Helper()
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: want kept: %v", p, err)
		}
	}

	t.Run("plan", func(t *testing.T) {
		f, l := systemFixture(t)
		p := BuildPurgePlan(l, f.exe, noEnv, "linux", probe)
		want := []struct {
			path string
			kind PurgeKind
			note string
		}{
			{f.data, PurgeData, ""},
			{f.logs, PurgeLogs, ""},
			{f.moved, PurgeLeftover, PurgeNoteMovedAway},
			{f.oldLogs, PurgeLeftover, PurgeNoteOldLogs},
			{f.oldBin, PurgeLeftover, PurgeNotePreMigrationApp},
		}
		if len(p.Items) != len(want) {
			t.Fatalf("items=%d want %d:\n%s", len(p.Items), len(want), p.Text())
		}
		for _, w := range want {
			it := findItem(p, w.path)
			if it == nil {
				t.Errorf("no item for %s:\n%s", w.path, p.Text())
				continue
			}
			if it.Kind != w.kind || it.Note != w.note || !it.Selected || it.Bytes <= 0 || it.Files <= 0 {
				t.Errorf("item %s: %+v, want kind=%s note=%q selected, bytes>0", w.path, *it, w.kind, w.note)
			}
		}
		if it := findItem(p, f.oldBin); it != nil && it.Files != 1 {
			t.Errorf("pre-migration item counts shipped template: files=%d want 1", it.Files)
		}
		for _, it := range p.Items {
			if filepath.Clean(it.Path) == filepath.Clean(f.app) || filepath.Base(it.Path) == "portable.txt" {
				t.Errorf("plan must not include %s", it.Path)
			}
		}
	})

	t.Run("execute all", func(t *testing.T) {
		f, l := systemFixture(t)
		rep := ExecutePurge(BuildPurgePlan(l, f.exe, noEnv, "linux", probe))
		if len(rep.Failed) != 0 {
			t.Fatalf("failed: %v", rep.Failed)
		}
		if len(rep.Removed) != 5 {
			t.Errorf("removed=%v want 5 items", rep.Removed)
		}
		for _, p := range []string{f.data, f.logs, f.moved, f.oldLogs, filepath.Join(f.oldBin, "wizard_states")} {
			gone(t, p)
		}
		for _, p := range []string{f.exe, filepath.Join(f.app, "portable.txt"), filepath.Join(f.oldBin, "wizard_template.json")} {
			kept(t, p)
		}
	})

	t.Run("logs unselected", func(t *testing.T) {
		f, l := systemFixture(t)
		p := BuildPurgePlan(l, f.exe, noEnv, "linux", probe)
		findItem(p, f.logs).Selected = false
		rep := ExecutePurge(p)
		if len(rep.Failed) != 0 {
			t.Fatalf("failed: %v", rep.Failed)
		}
		kept(t, filepath.Join(f.logs, "launcher.log"))
		gone(t, f.data)
	})

	t.Run("logs inside data unselected", func(t *testing.T) {
		f, l := systemFixture(t)
		l.Logs = LogDir(filepath.Join(f.data, "logs"))
		write(t, filepath.Join(string(l.Logs), "launcher.log"), "log line")
		p := BuildPurgePlan(l, f.exe, noEnv, "linux", probe)
		findItem(p, string(l.Logs)).Selected = false
		ExecutePurge(p)
		kept(t, filepath.Join(string(l.Logs), "launcher.log"))
		gone(t, filepath.Join(f.data, "bin"))
	})

	t.Run("portable with system folder", func(t *testing.T) {
		root := t.TempDir()
		app := filepath.Join(root, "app")
		home := filepath.Join(root, "home")
		exe := filepath.Join(app, "singbox-launcher")
		write(t, exe, "exe")
		write(t, filepath.Join(app, "portable.txt"), "")
		write(t, filepath.Join(app, "bin", "wizard_template.json"), `{}`)
		write(t, filepath.Join(app, "bin", "wizard_states", "state.json"), `{"version":3}`)
		sys := filepath.Join(home, ".local", "share", "singbox-launcher")
		write(t, filepath.Join(sys, "bin", "wizard_states", "state.json"), `{"stale":true}`)
		env := func(k string) string {
			switch k {
			case "HOME":
				return home
			case "LOCALAPPDATA":
				return filepath.Join(home, "AppData", "Local")
			}
			return ""
		}
		l := Layout{App: AppDir(app), Data: DataDir(app), Logs: LogDir(filepath.Join(app, "logs")), Mode: ModePortable}
		p := BuildPurgePlan(l, exe, env, "linux", probe)
		it := findItem(p, sys)
		if it == nil || it.Kind != PurgeLeftover || it.Note != PurgeNoteUnusedSystem {
			t.Fatalf("want unused system data folder %s:\n%s", sys, p.Text())
		}
		if d := findItem(p, filepath.Join(app, "bin")); d == nil || d.Kind != PurgeData {
			t.Fatalf("portable data item must be app/bin:\n%s", p.Text())
		}
		if rep := ExecutePurge(p); len(rep.Failed) != 0 {
			t.Fatalf("failed: %v", rep.Failed)
		}
		gone(t, sys)
		gone(t, filepath.Join(app, "bin", "wizard_states"))
		kept(t, filepath.Join(app, "bin", "wizard_template.json"))
		kept(t, filepath.Join(app, "portable.txt"))
		kept(t, exe)
	})
}
