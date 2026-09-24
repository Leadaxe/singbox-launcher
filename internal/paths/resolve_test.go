package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"singbox-launcher/internal/constants"
)

func TestResolveMatrix(t *testing.T) {
	// Корни фикстур — абсолютные на хосте: на Windows «/env/data» без тома
	// не IsAbs (Resolve доводит его до «D:\env\data», XDG отбрасывает).
	hostAbs := func(p string) string {
		a, err := filepath.Abs(filepath.FromSlash(p))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	var (
		dataEnv = hostAbs("/env/data")
		logEnv  = hostAbs("/env/logs")
		xdgData = hostAbs("/xd")
		xdgStat = hostAbs("/xs")
	)
	const home = "/home/u"
	join := filepath.Join
	portableAt := func(app string, m Mode) Layout {
		return Layout{App: AppDir(app), Data: DataDir(app), Logs: LogDir(join(app, "logs")), Mode: m}
	}
	relAbs, err := filepath.Abs("rel/data")
	if err != nil {
		t.Fatal(err)
	}

	systemAt := func(app string, markerIgnored bool) Layout {
		return Layout{App: AppDir(app), Data: DataDir(join("/lad", "singbox-launcher")), Logs: LogDir(join("/lad", "singbox-launcher", "logs")), Mode: ModeSystem, MarkerIgnored: markerIgnored}
	}
	// Защищённые каталоги Windows (SPEC 139 §7) задаются относительно AppDir
	// фикстуры: он — временный каталог хоста.
	parentAs := func(name string) func(app string) map[string]string {
		return func(app string) map[string]string { return map[string]string{name: filepath.Dir(app)} }
	}

	cases := []struct {
		name   string
		goos   string
		bundle bool // exe внутри X.app/Contents/MacOS
		marker bool // portable.txt рядом с бинарём
		legacy bool // bin/wizard_states/state.json рядом с бинарём
		probe  bool
		env    map[string]string
		// extraEnv — переменные, зависящие от AppDir (защищённые каталоги).
		extraEnv func(app string) map[string]string
		want     func(app string) Layout
		wantErr  bool
	}{
		{
			name: "env data only", goos: "linux", marker: true,
			env: map[string]string{constants.EnvDataDir: dataEnv},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(dataEnv), Logs: LogDir(join(dataEnv, "logs")), Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir}}
			},
		},
		{
			name: "env data relative is made absolute", goos: "linux",
			env: map[string]string{constants.EnvDataDir: "rel/data"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(relAbs), Logs: LogDir(join(relAbs, "logs")), Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir}}
			},
		},
		{
			name: "env log only, data by marker", goos: "linux", marker: true, probe: true,
			env: map[string]string{constants.EnvLogDir: logEnv},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(app), Logs: LogDir(logEnv), Mode: ModeEnv, EnvSource: []string{constants.EnvLogDir}}
			},
		},
		{
			name: "env log only, data by xdg default", goos: "linux",
			env: map[string]string{constants.EnvLogDir: logEnv, "HOME": home},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, ".local", "share", "singbox-launcher")), Logs: LogDir(logEnv), Mode: ModeEnv, EnvSource: []string{constants.EnvLogDir}}
			},
		},
		{
			name: "env both", goos: "windows",
			env: map[string]string{constants.EnvDataDir: dataEnv, constants.EnvLogDir: logEnv, "LOCALAPPDATA": "/lad"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(dataEnv), Logs: LogDir(logEnv), Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir, constants.EnvLogDir}}
			},
		},
		{
			name: "marker wins over legacy", goos: "linux", marker: true, legacy: true, probe: true,
			env:  map[string]string{"HOME": home},
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{
			name: "marker on windows", goos: "windows", marker: true, probe: true,
			env:  map[string]string{"LOCALAPPDATA": "/lad", "ProgramFiles": `C:\Program Files`},
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		// SPEC 139 §7: AppDir под защищённым каталогом Windows не пишется
		// пользователем, даже если проба проходит (повышенный экземпляр):
		// маркер и Legacy игнорируются, раскладка — системная.
		{
			name: "marker under Program Files, probe passes", goos: "windows", marker: true, probe: true,
			env: map[string]string{"LOCALAPPDATA": "/lad"}, extraEnv: parentAs("ProgramFiles"),
			want: func(app string) Layout { return systemAt(app, true) },
		},
		{
			name: "legacy under Program Files (x86), case-insensitive", goos: "windows", legacy: true, probe: true,
			env: map[string]string{"LOCALAPPDATA": "/lad"},
			extraEnv: func(app string) map[string]string {
				return map[string]string{"ProgramFiles(x86)": strings.ToUpper(filepath.Dir(app))}
			},
			want: func(app string) Layout { return systemAt(app, false) },
		},
		{
			name: "marker and legacy at ProgramW6432 root", goos: "windows", marker: true, legacy: true, probe: true,
			env: map[string]string{"LOCALAPPDATA": "/lad"},
			extraEnv: func(app string) map[string]string {
				return map[string]string{"ProgramW6432": app + string(filepath.Separator)}
			},
			want: func(app string) Layout { return systemAt(app, true) },
		},
		{
			name: "legacy under SystemRoot", goos: "windows", legacy: true, probe: true,
			env: map[string]string{"LOCALAPPDATA": "/lad"}, extraEnv: parentAs("SystemRoot"),
			want: func(app string) Layout { return systemAt(app, false) },
		},
		{
			name: "protected prefix without directory boundary", goos: "windows", marker: true, probe: true,
			env: map[string]string{"LOCALAPPDATA": "/lad"},
			extraEnv: func(app string) map[string]string {
				return map[string]string{"ProgramFiles": app[:len(app)-1]}
			},
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{
			name: "marker in read-only folder on windows", goos: "windows", marker: true, probe: false,
			env:  map[string]string{"LOCALAPPDATA": "/lad"},
			want: func(app string) Layout { return systemAt(app, true) },
		},
		{
			name: "marker in read-only folder on linux", goos: "linux", marker: true, probe: false,
			env: map[string]string{"HOME": home},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, ".local", "share", "singbox-launcher")), Logs: LogDir(join(home, ".local", "state", "singbox-launcher", "logs")), Mode: ModeSystem, MarkerIgnored: true}
			},
		},
		{
			name: "protected dirs do not apply off windows", goos: "linux", marker: true, probe: true,
			env: map[string]string{"HOME": home}, extraEnv: parentAs("ProgramFiles"),
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{
			name: "legacy writable", goos: "windows", legacy: true, probe: true,
			env:  map[string]string{"LOCALAPPDATA": "/lad"},
			want: func(app string) Layout { return portableAt(app, ModeLegacy) },
		},
		{
			name: "legacy not writable falls to system", goos: "linux", legacy: true, probe: false,
			env: map[string]string{"HOME": home},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, ".local", "share", "singbox-launcher")), Logs: LogDir(join(home, ".local", "state", "singbox-launcher", "logs")), Mode: ModeSystem}
			},
		},
		{
			name: "linux xdg set", goos: "linux",
			env: map[string]string{"HOME": home, "XDG_DATA_HOME": xdgData, "XDG_STATE_HOME": xdgStat},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(xdgData, "singbox-launcher")), Logs: LogDir(join(xdgStat, "singbox-launcher", "logs")), Mode: ModeSystem}
			},
		},
		{
			name: "linux relative xdg ignored", goos: "freebsd",
			env: map[string]string{"HOME": home, "XDG_DATA_HOME": "xd"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, ".local", "share", "singbox-launcher")), Logs: LogDir(join(home, ".local", "state", "singbox-launcher", "logs")), Mode: ModeSystem}
			},
		},
		{
			name: "linux xdg set, no home", goos: "linux",
			env: map[string]string{"XDG_DATA_HOME": xdgData, "XDG_STATE_HOME": xdgStat},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(xdgData, "singbox-launcher")), Logs: LogDir(join(xdgStat, "singbox-launcher", "logs")), Mode: ModeSystem}
			},
		},
		{name: "linux no home", goos: "linux", env: map[string]string{"XDG_DATA_HOME": "/xd"}, wantErr: true},
		{
			name: "darwin app bundle ignores marker and legacy", goos: "darwin", bundle: true, marker: true, legacy: true, probe: true,
			env: map[string]string{"HOME": home},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, "Library", "Application Support", "singbox-launcher")), Logs: LogDir(join(home, "Library", "Logs", "singbox-launcher")), Mode: ModeSystem}
			},
		},
		{name: "darwin app bundle no home", goos: "darwin", bundle: true, wantErr: true},
		{
			name: "darwin bare binary", goos: "darwin",
			env:  map[string]string{"HOME": home},
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{
			name: "darwin bare binary legacy", goos: "darwin", legacy: true, probe: true,
			env:  map[string]string{"HOME": home},
			want: func(app string) Layout { return portableAt(app, ModeLegacy) },
		},
		{
			name: "windows localappdata", goos: "windows",
			env: map[string]string{"LOCALAPPDATA": "/lad", "USERPROFILE": "/up"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join("/lad", "singbox-launcher")), Logs: LogDir(join("/lad", "singbox-launcher", "logs")), Mode: ModeSystem}
			},
		},
		{
			name: "windows userprofile fallback", goos: "windows",
			env: map[string]string{"USERPROFILE": "/up"},
			want: func(app string) Layout {
				d := join("/up", "AppData", "Local", "singbox-launcher")
				return Layout{App: AppDir(app), Data: DataDir(d), Logs: LogDir(join(d, "logs")), Mode: ModeSystem}
			},
		},
		{
			name: "windows no roots, writable", goos: "windows", probe: true,
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{name: "windows no roots, not writable", goos: "windows", probe: false, wantErr: true},
		{name: "windows no roots, under Program Files", goos: "windows", probe: true, extraEnv: parentAs("ProgramFiles"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := t.TempDir()
			if tc.bundle {
				app = join(app, "Lx.app", "Contents", "MacOS")
				if err := os.MkdirAll(app, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.marker {
				writeFile(t, join(app, constants.PortableMarkerFileName))
			}
			if tc.legacy {
				writeFile(t, join(app, "bin", "wizard_states", "state.json"))
			}
			exe := join(app, "singbox-launcher")
			vars := map[string]string{}
			for k, v := range tc.env {
				vars[k] = v
			}
			if tc.extraEnv != nil {
				for k, v := range tc.extraEnv(app) {
					vars[k] = v
				}
			}
			env := func(k string) string { return vars[k] }
			probe := func(dir string) bool {
				if dir != app {
					t.Errorf("probe called for %q, want AppDir %q", dir, app)
				}
				return tc.probe
			}

			got, err := Resolve(exe, env, tc.goos, probe)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.want(app); !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}

	// -handoff (SPEC 139 §5): раскладка родителя для повышенного экземпляра
	// идёт мимо Resolve; App — каталог своего exe.
	t.Run("handoff", func(t *testing.T) {
		app := t.TempDir()
		exe := join(app, "singbox-launcher")
		valid := "4242|system|" + dataEnv + "|" + logEnv
		want := Layout{App: AppDir(app), Data: DataDir(dataEnv), Logs: LogDir(logEnv), Mode: ModeSystem}

		got, pid, err := ParseHandoff(valid, exe)
		if err != nil || pid != 4242 || !reflect.DeepEqual(got, want) {
			t.Fatalf("ParseHandoff(%q) = %+v, %d, %v; want %+v, 4242", valid, got, pid, err, want)
		}
		if back := got.Handoff(pid); back != valid {
			t.Errorf("Handoff round trip = %q, want %q", back, valid)
		}

		// portable.txt рядом с бинарём при System — маркер игнорирован у
		// родителя, пометка восстанавливается.
		writeFile(t, join(app, constants.PortableMarkerFileName))
		want.MarkerIgnored = true
		if got, _, err := ParseHandoff(valid, exe); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("with marker: got %+v, %v; want %+v", got, err, want)
		}

		for _, bad := range []string{
			"",
			"4242|system|" + dataEnv,
			"0|system|" + dataEnv + "|" + logEnv,
			"-1|system|" + dataEnv + "|" + logEnv,
			"pid|system|" + dataEnv + "|" + logEnv,
			"4242|nomad|" + dataEnv + "|" + logEnv,
			"4242|system|rel/data|" + logEnv,
			"4242|system|" + dataEnv + "|rel/logs",
		} {
			if l, _, err := ParseHandoff(bad, exe); err == nil {
				t.Errorf("ParseHandoff(%q) = %+v, want error", bad, l)
			}
		}
	})
}

func TestIsAppBundle(t *testing.T) {
	cases := []struct {
		exe  string
		goos string
		want bool
	}{
		{"/Applications/Lx.app/Contents/MacOS/singbox-launcher", "darwin", true},
		{"/Users/u/Downloads/My Lx.app/Contents/MacOS/bin/x", "darwin", true},
		{"/Applications/Lx.app/Contents/MacOS/singbox-launcher", "linux", false},
		{"/Users/u/singbox-launcher/singbox-launcher", "darwin", false},
		{"/Users/u/.app/Contents/MacOS/x", "darwin", false},
		{"/Users/u/Lx.app/Contents/Resources/x", "darwin", false},
	}
	for _, tc := range cases {
		if got := IsAppBundle(tc.exe, tc.goos); got != tc.want {
			t.Errorf("IsAppBundle(%q, %q) = %v, want %v", tc.exe, tc.goos, got, tc.want)
		}
	}
}

func TestProbeWritable(t *testing.T) {
	dir := t.TempDir()
	if !ProbeWritable(dir) {
		t.Error("TempDir: want writable")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("probe left files behind: %v", entries)
	}

	file := filepath.Join(dir, "file")
	writeFile(t, file)
	if ProbeWritable(file) {
		t.Error("regular file: want not writable")
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows не запрещает запись в каталог по биту 0555 (атрибут read-only каталогу не мешает)")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if ProbeWritable(ro) {
		t.Error("0555 dir: want not writable")
	}
}

func TestLogLine(t *testing.T) {
	l := Layout{App: "/a", Data: "/d", Logs: "/l", Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir, constants.EnvLogDir}}
	want := "layout: mode=env app=/a data=/d logs=/l env=SINGBOX_LAUNCHER_DATA_DIR,SINGBOX_LAUNCHER_LOG_DIR"
	if got := l.LogLine(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
