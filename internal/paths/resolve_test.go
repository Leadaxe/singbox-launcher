package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"singbox-launcher/internal/constants"
)

func TestResolveMatrix(t *testing.T) {
	const (
		dataEnv = "/env/data"
		logEnv  = "/env/logs"
		home    = "/home/u"
	)
	join := filepath.Join
	portableAt := func(app string, m Mode) Layout {
		return Layout{App: AppDir(app), Data: DataDir(app), Logs: LogDir(join(app, "logs")), Mode: m}
	}
	relAbs, err := filepath.Abs("rel/data")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		goos    string
		bundle  bool // exe внутри X.app/Contents/MacOS
		marker  bool // portable.txt рядом с бинарём
		legacy  bool // bin/wizard_states/state.json рядом с бинарём
		probe   bool
		env     map[string]string
		want    func(app string) Layout
		wantErr bool
	}{
		{
			name: "env data only", goos: "linux", marker: true,
			env: map[string]string{constants.EnvDataDir: dataEnv},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: dataEnv, Logs: LogDir(join(dataEnv, "logs")), Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir}}
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
			name: "env log only, data by marker", goos: "linux", marker: true,
			env: map[string]string{constants.EnvLogDir: logEnv},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(app), Logs: logEnv, Mode: ModeEnv, EnvSource: []string{constants.EnvLogDir}}
			},
		},
		{
			name: "env log only, data by xdg default", goos: "linux",
			env: map[string]string{constants.EnvLogDir: logEnv, "HOME": home},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join(home, ".local", "share", "singbox-launcher")), Logs: logEnv, Mode: ModeEnv, EnvSource: []string{constants.EnvLogDir}}
			},
		},
		{
			name: "env both", goos: "windows",
			env: map[string]string{constants.EnvDataDir: dataEnv, constants.EnvLogDir: logEnv, "LOCALAPPDATA": "/lad"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: dataEnv, Logs: logEnv, Mode: ModeEnv, EnvSource: []string{constants.EnvDataDir, constants.EnvLogDir}}
			},
		},
		{
			name: "marker wins over legacy", goos: "linux", marker: true, legacy: true, probe: true,
			env:  map[string]string{"HOME": home},
			want: func(app string) Layout { return portableAt(app, ModePortable) },
		},
		{
			name: "marker on windows", goos: "windows", marker: true,
			env:  map[string]string{"LOCALAPPDATA": "/lad"},
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
			env: map[string]string{"HOME": home, "XDG_DATA_HOME": "/xd", "XDG_STATE_HOME": "/xs"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join("/xd", "singbox-launcher")), Logs: LogDir(join("/xs", "singbox-launcher", "logs")), Mode: ModeSystem}
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
			env: map[string]string{"XDG_DATA_HOME": "/xd", "XDG_STATE_HOME": "/xs"},
			want: func(app string) Layout {
				return Layout{App: AppDir(app), Data: DataDir(join("/xd", "singbox-launcher")), Logs: LogDir(join("/xs", "singbox-launcher", "logs")), Mode: ModeSystem}
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
			env := func(k string) string { return tc.env[k] }
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
