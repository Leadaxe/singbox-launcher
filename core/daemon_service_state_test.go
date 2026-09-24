//go:build darwin

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/lxdclient"
)

// testServiceLayout — раскладка службы lx.11 во временном каталоге:
// копия — плоский файл <base>/Library/PrivilegedHelperTools/sing-box-lxd,
// legacy ранних lx.11 — <base>/Library/PrivilegedHelperTools/<label>, plist в
// <base>/LaunchDaemons, ядро лаунчера в <base>/data/bin. Владелец цепочки —
// текущий uid (root-owned файлы тест создать не может).
type testServiceLayout struct {
	daemonServiceLayout
	toolsDir     string
	launcherCore string
}

func newTestServiceLayout(t *testing.T) testServiceLayout {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "Library")
	tools := filepath.Join(root, "PrivilegedHelperTools")
	for _, dir := range []string{tools, filepath.Join(base, "LaunchDaemons"), filepath.Join(base, "data", "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// MkdirAll уважает umask: выставляем права цепочки явно.
	for _, dir := range []string{root, tools} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	l := testServiceLayout{
		daemonServiceLayout: daemonServiceLayout{
			PlistPath:  filepath.Join(base, "LaunchDaemons", daemonLaunchdLabel+".plist"),
			CorePath:   filepath.Join(tools, filepath.Base(daemonServiceCorePath())),
			LegacyPath: filepath.Join(tools, filepath.Base(daemonServiceLegacyCopyPath)),
			ChainRoot:  root,
			OwnerUID:   uint32(os.Getuid()),
		},
		toolsDir:     tools,
		launcherCore: filepath.Join(base, "data", "bin", "sing-box"),
	}
	writeTestFile(t, l.launcherCore, "core v1")
	return l
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeTestPlist — plist в форме, которую пишет `lxd --service=install`.
func writeTestPlist(t *testing.T, path, program string) {
	t.Helper()
	body := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + daemonLaunchdLabel + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + program + `</string>
        <string>lxd</string>
        <string>--state-dir</string>
        <string>/Library/Application Support/sing-box-lxd/state</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// classifyTestService — полный проход классификатора без процесса.
func classifyTestService(l testServiceLayout, hashes *fileHashCache) DaemonServiceCheck {
	c := inspectDaemonServiceDefinition(l.daemonServiceLayout)
	compareDaemonServiceFiles(&c, l.CorePath, l.launcherCore, hashes)
	return c
}

// TestDaemonServiceClassifier — вердикты SPEC 136 §4 на настоящих файлах:
// путь в plist, цепочка владения, sha копии против ядра лаунчера, кэш хэшей,
// паспорт работающего демона (lx.11 и старое ядро без полей).
func TestDaemonServiceClassifier(t *testing.T) {
	l := newTestServiceLayout(t)
	var hashes fileHashCache
	expect := func(t *testing.T, got DaemonServiceCheck, want DaemonServiceState) {
		t.Helper()
		if got.State != want {
			t.Fatalf("state = %s, want %s (detail: %s)", got.State, want, got.Detail)
		}
	}

	// NotInstalled: plist нет.
	expect(t, classifyTestService(l, &hashes), DaemonServiceNotInstalled)

	// Unsafe: служба на пользовательском ядре — ровно дыра SPEC 136 §1.
	writeTestPlist(t, l.PlistPath, l.launcherCore)
	c := classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceUnsafe)
	if c.ServicePath != l.launcherCore || c.CopyUsable() || !c.NeedsInstall() {
		t.Fatalf("unsafe: path=%q usable=%v needsInstall=%v", c.ServicePath, c.CopyUsable(), c.NeedsInstall())
	}

	// Unsafe: plist не разобрался.
	if err := os.WriteFile(l.PlistPath, []byte("not a plist"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)

	// Stale: plist на копию, копии нет (каталоги целы).
	writeTestPlist(t, l.PlistPath, l.CorePath)
	c = classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceStale)
	if !c.CopyMissing || c.CopyUsable() {
		t.Fatalf("missing copy: CopyMissing=%v usable=%v", c.CopyMissing, c.CopyUsable())
	}

	// Unsafe: на месте файла копии — каталог; причина говорит его убрать.
	if err := os.Mkdir(l.CorePath, 0o755); err != nil {
		t.Fatal(err)
	}
	c = classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceUnsafe)
	if !strings.Contains(c.Detail, "is a directory") || !strings.Contains(c.Detail, "remove it") {
		t.Fatalf("folder in place of the copy: %q", c.Detail)
	}
	if err := os.Remove(l.CorePath); err != nil {
		t.Fatal(err)
	}

	// Unsafe: plist на копию ранней раскладки lx.11 — каталог <label>/sing-box
	// или плоский <label>; причина — install, затем убрать остатки.
	writeTestFile(t, l.LegacyPath, "core v1")
	for _, legacyProgram := range []string{l.LegacyPath, filepath.Join(l.LegacyPath, "sing-box")} {
		writeTestPlist(t, l.PlistPath, legacyProgram)
		c = classifyTestService(l, &hashes)
		expect(t, c, DaemonServiceUnsafe)
		if !strings.Contains(c.Detail, "legacy layout") || !strings.Contains(c.Detail, "sudo rm -rf "+shellQuote(l.LegacyPath)) {
			t.Fatalf("legacy plist %s: %q", legacyProgram, c.Detail)
		}
	}
	if err := os.Remove(l.LegacyPath); err != nil {
		t.Fatal(err)
	}
	writeTestPlist(t, l.PlistPath, l.CorePath)

	// OK: копия = ядро лаунчера. Повтор с теми же файлами — из кэша.
	writeTestFile(t, l.CorePath, "core v1")
	writeTestFile(t, daemonServiceSidecarPath(l.CorePath), `{"version":"1.14.1-lx.11"}`)
	if v := readDaemonServiceSidecarVersion(l.CorePath); v != "1.14.1-lx.11" {
		t.Fatalf("sidecar %s: version %q", daemonServiceSidecarPath(l.CorePath), v)
	}
	c = classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceOK)
	if !c.CopyUsable() || c.CopySHA256 == "" || c.CopySHA256 != c.LauncherSHA256 {
		t.Fatalf("ok: usable=%v copy=%q launcher=%q", c.CopyUsable(), c.CopySHA256, c.LauncherSHA256)
	}
	// Содержимое одинаковое, но inode разные — два чтения.
	if hashes.computed != 2 {
		t.Fatalf("hashes computed %d times, want 2", hashes.computed)
	}
	computed := hashes.computed
	expect(t, classifyTestService(l, &hashes), DaemonServiceOK)
	if hashes.computed != computed {
		t.Fatalf("unchanged files were re-hashed: %d → %d", computed, hashes.computed)
	}

	// Stale: ядро лаунчера обновилось (скачивание), копия старая. Кэш видит
	// замену по ключу и перечитывает только изменившийся файл.
	writeTestFile(t, l.launcherCore, "core v2, a longer build")
	c = classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceStale)
	if c.CopyMissing || !c.CopyUsable() || c.CopySHA256 == c.LauncherSHA256 {
		t.Fatalf("stale: missing=%v usable=%v copy=%q launcher=%q", c.CopyMissing, c.CopyUsable(), c.CopySHA256, c.LauncherSHA256)
	}
	if hashes.computed != computed+1 {
		t.Fatalf("after replacing the launcher core: computed %d, want %d", hashes.computed, computed+1)
	}

	// Ядро лаунчера — симлинк на dev-сборку: сверяется цель ссылки.
	devBuild := filepath.Join(filepath.Dir(l.launcherCore), "sing-box-dev")
	writeTestFile(t, devBuild, "core v1")
	if err := os.Remove(l.launcherCore); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(devBuild, l.launcherCore); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceOK)

	// Unsafe: каталог помощников (родитель плоской копии) пишется группой.
	if err := os.Chmod(l.toolsDir, 0o775); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)
	if err := os.Chmod(l.toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Unsafe: верх цепочки пишется всеми.
	if err := os.Chmod(l.ChainRoot, 0o757); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)
	if err := os.Chmod(l.ChainRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// Unsafe: копия пишется группой.
	if err := os.Chmod(l.CorePath, 0o775); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)
	if err := os.Chmod(l.CorePath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Unsafe: чужой владелец цепочки (в проде — не root).
	foreign := l
	foreign.OwnerUID = l.OwnerUID + 1
	expect(t, classifyTestService(foreign, &hashes), DaemonServiceUnsafe)

	// Unsafe: копия — симлинк (на пользовательский файл).
	if err := os.Rename(l.CorePath, l.CorePath+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(devBuild, l.CorePath); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)
	if err := os.Remove(l.CorePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(l.CorePath+".real", l.CorePath); err != nil {
		t.Fatal(err)
	}
	c = classifyTestService(l, &hashes)
	expect(t, c, DaemonServiceOK)

	// launchd: файлы в порядке, но служба не загружена или не running —
	// NotRunning (лечится bootstrap, не install); launchd не ответил —
	// вердикта нет; поверх файлового вердикта launchd не судит.
	launchd := func(base DaemonServiceCheck, job launchdJob) DaemonServiceCheck {
		lc := base
		compareDaemonServiceLaunchd(&lc, job)
		return lc
	}
	expect(t, launchd(c, launchdJob{}), DaemonServiceOK)
	expect(t, launchd(c, launchdJob{Known: true, Loaded: true, State: "running"}), DaemonServiceOK)
	notLoaded := launchd(c, launchdJob{Known: true})
	expect(t, notLoaded, DaemonServiceNotRunning)
	if notLoaded.LaunchdState != launchdNotLoaded || !notLoaded.NeedsBootstrap() || notLoaded.NeedsInstall() || !notLoaded.CopyUsable() {
		t.Fatalf("not loaded: launchd %q bootstrap=%v install=%v usable=%v",
			notLoaded.LaunchdState, notLoaded.NeedsBootstrap(), notLoaded.NeedsInstall(), notLoaded.CopyUsable())
	}
	waiting := launchd(c, launchdJob{Known: true, Loaded: true, State: "spawn scheduled"})
	expect(t, waiting, DaemonServiceNotRunning)
	if waiting.LaunchdState != "spawn scheduled" {
		t.Fatalf("launchd state %q", waiting.LaunchdState)
	}
	staleCheck := c
	staleCheck.State = DaemonServiceStale
	expect(t, launchd(staleCheck, launchdJob{Known: true}), DaemonServiceStale)
	// Процесс не перебивает NotRunning (демон, запущенный руками, — не служба).
	nr := notLoaded
	compareDaemonServiceProcess(&nr, lxdclient.InfoData{Executable: l.CorePath, ExecutableSHA256: "ff"}, l.CorePath)
	expect(t, nr, DaemonServiceNotRunning)
	if cmd := daemonBootstrapCommand(); cmd != "sudo launchctl bootstrap system '/Library/LaunchDaemons/"+daemonLaunchdLabel+".plist'" {
		t.Fatalf("bootstrap command %q", cmd)
	}
	// Разбор `launchctl print`: state самой службы, не вложенных блоков.
	printed := "system/" + daemonLaunchdLabel + " = {\n\tactive count = 0\n\tendpoints = {\n\t\tstate = active\n\t}\n\tstate = not running\n\tpid = 0\n}\n"
	if got := parseLaunchctlPrintState([]byte(printed)); got != "not running" {
		t.Fatalf("parsed launchd state %q, want %q", got, "not running")
	}
	if got := parseLaunchctlPrintState([]byte("garbage")); got != "" {
		t.Fatalf("parsed launchd state from garbage: %q", got)
	}

	// Процесс: паспорт демона lx.11 (executable, executable_sha256).
	process := func(info lxdclient.InfoData, launcherVersion string) DaemonServiceCheck {
		pc := c
		pc.LauncherVersion = launcherVersion
		compareDaemonServiceProcess(&pc, info, l.CorePath)
		return pc
	}
	expect(t, process(lxdclient.InfoData{Executable: l.CorePath, ExecutableSHA256: c.CopySHA256}, ""), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: l.CorePath, ExecutableSHA256: "00" + c.CopySHA256[2:]}, ""), DaemonServiceProcessStale)
	expect(t, process(lxdclient.InfoData{Executable: l.launcherCore, ExecutableSHA256: c.CopySHA256}, ""), DaemonServiceProcessStale)
	// lx.11 сразу после старта: executable есть, executable_sha256 ещё
	// считается в фоне и пуст — «неизвестно», не ProcessStale; судит версия.
	expect(t, process(lxdclient.InfoData{Executable: l.CorePath, Version: "1.14.1-lx.11"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: l.CorePath, Version: "unknown"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Executable: l.CorePath, Version: "1.14.1-lx.10"}, "1.14.1-lx.11"), DaemonServiceProcessStale)
	// Старое ядро (lx.8/lx.10): полей нет — судит версия; dev-сборка не судит.
	expect(t, process(lxdclient.InfoData{Version: "1.14.1-lx.10"}, "1.14.1-lx.11"), DaemonServiceProcessStale)
	expect(t, process(lxdclient.InfoData{Version: "1.14.1-lx.11"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{Version: "unknown"}, "1.14.1-lx.11"), DaemonServiceOK)
	expect(t, process(lxdclient.InfoData{}, "1.14.1-lx.11"), DaemonServiceOK)

	// Вердикт по файлу сильнее вердикта по процессу.
	stale := c
	stale.State = DaemonServiceStale
	compareDaemonServiceProcess(&stale, lxdclient.InfoData{ExecutableSHA256: "ff"}, l.CorePath)
	expect(t, stale, DaemonServiceStale)
}

// TestDaemonServiceCommandQuoting — sudo-команды службы с путём, в котором
// пробел, апостроф и двойная кавычка: команда синтаксически верна для sh,
// разбирается ровно в задуманные аргументы и переживает литерал AppleScript,
// через который её получает Terminal. Uninstall идёт через копию, только
// когда она безопасна (SPEC 136 §5); вкладка Uninstall оставляет копию
// (`--keep-copy`), подсказка очистки данных — нет.
func TestDaemonServiceCommandQuoting(t *testing.T) {
	bin := "/Users/o'brien/My Apps/\"lx\" core/sing-box"
	commands := map[string][]string{
		daemonServiceCommand(bin, "lxd", "--service=install"): {bin, "lxd", "--service=install"},
		daemonUninstallCommandFor(bin, true, true):            {bin, "lxd", "--service=uninstall", "--keep-copy", "--purge"},
		daemonUninstallCommandFor(bin, false, true):           {bin, "lxd", "--service=uninstall", "--keep-copy"},
		daemonUninstallCommandFor(bin, true, false):           {bin, "lxd", "--service=uninstall", "--purge"},
	}
	wantInstall := `sudo '/Users/o'\''brien/My Apps/"lx" core/sing-box' lxd --service=install`
	if got := daemonServiceCommand(bin, "lxd", "--service=install"); got != wantInstall {
		t.Fatalf("install command:\n got %s\nwant %s", got, wantInstall)
	}
	for command, wantArgs := range commands {
		if out, err := exec.Command("sh", "-n", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("sh -n %q: %v (%s)", command, err, out)
		}
		// sudo подменён функцией, печатающей свои аргументы по строке.
		out, err := exec.Command("sh", "-c", `sudo() { printf '%s\n' "$@"; }; `+command).Output()
		if err != nil {
			t.Fatalf("sh -c %q: %v", command, err)
		}
		if got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); strings.Join(got, "|") != strings.Join(wantArgs, "|") {
			t.Fatalf("%q parsed as %q, want %q", command, got, wantArgs)
		}
		if osascript, err := exec.LookPath("osascript"); err == nil {
			out, err := exec.Command(osascript, "-e", "return "+appleScriptString(command)).Output()
			if err != nil {
				t.Fatalf("osascript literal of %q: %v", command, err)
			}
			if got := strings.TrimSuffix(string(out), "\n"); got != command {
				t.Fatalf("AppleScript literal round-trip:\n got %s\nwant %s", got, command)
			}
		}
	}

	// Uninstall и client add: копия — только когда plist на неё и она цела.
	l := newTestServiceLayout(t)
	if got := daemonServiceBinaryFor(l.daemonServiceLayout, l.launcherCore); got != l.launcherCore {
		t.Fatalf("no service: binary %s, want the launcher core", got)
	}
	writeTestPlist(t, l.PlistPath, l.launcherCore)
	if got := daemonServiceBinaryFor(l.daemonServiceLayout, l.launcherCore); got != l.launcherCore {
		t.Fatalf("unsafe service: binary %s, want the launcher core", got)
	}
	writeTestPlist(t, l.PlistPath, l.CorePath)
	if got := daemonServiceBinaryFor(l.daemonServiceLayout, l.launcherCore); got != l.launcherCore {
		t.Fatalf("missing copy: binary %s, want the launcher core", got)
	}
	writeTestFile(t, l.CorePath, "core v0")
	if got := daemonServiceBinaryFor(l.daemonServiceLayout, l.launcherCore); got != l.CorePath {
		t.Fatalf("safe copy: binary %s, want the copy", got)
	}
}

// TestServiceCoreVersionGate — сравнение версий ядра форка и граница
// «ядро умеет root-owned копию» (minCoreForRootOwnedService = lx.11):
// номер lx числом, пре-релиз младше релиза той же базы, неразборчивая
// версия — не умеет.
func TestServiceCoreVersionGate(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.14.1-lx.12-rc1", "1.14.1-lx.8", 1},
		{"1.14.1-lx.10", "1.14.1-lx.9", 1},
		{"1.14.1-lx.12-rc1", "1.14.1-lx.12", -1},
		{"1.14.1-lx.12-rc.1", "1.14.1-lx.12-rc.2", -1},
		{"1.14.1-lx.11", "1.14.1-lx.11", 0},
		{"v1.14.1-lx.11", "1.14.1-lx.11", 0},
		{"1.15.0-lx.1", "1.14.1-lx.40", 1},
		{"1.14.0-lx.40", "1.14.1-lx.1", -1},
	} {
		a, okA := parseCoreBuild(tc.a)
		b, okB := parseCoreBuild(tc.b)
		if !okA || !okB {
			t.Fatalf("parse %q=%v, %q=%v", tc.a, okA, tc.b, okB)
		}
		if got := compareCoreBuilds(a, b); got != tc.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := compareCoreBuilds(b, a); got != -tc.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", tc.b, tc.a, got, -tc.want)
		}
	}
	for version, want := range map[string]bool{
		"1.14.1-lx.8":      false,
		"1.14.1-lx.10":     false,
		"1.14.1-lx.11-rc1": false,
		"1.14.1-lx.11":     true,
		"1.14.1-lx.12-rc1": true,
		"1.14.1-lx.12":     true,
		"v1.14.1-lx.12":    true,
		"1.14.0-lx.40":     false,
		"1.15.0-lx.1":      true,
		"1.14.1":           false, // апстрим: lxd нет вовсе
		"unknown":          false, // dev-сборка
		"unnamed-dev":      false,
		"v-local-test":     false,
		"1.14.1-lx.":       false,
		"1.14.1-lx.12x":    false,
		"":                 false,
	} {
		if got := coreSupportsRootOwnedCopy(version); got != want {
			t.Fatalf("coreSupportsRootOwnedCopy(%q) = %v, want %v", version, got, want)
		}
	}
	if !coreSupportsRootOwnedCopy(constants.RequiredCoreVersion) {
		t.Fatalf("the pinned core %s must support the root-owned copy", constants.RequiredCoreVersion)
	}
}

// TestDaemonServiceCoreTooOld — живой дефект приёмки SPEC 136: служба на
// root-owned копии lx.12-rc1, ядро лаунчера lx.8 (до lx.11 install пишет в
// plist свой путь в DataDir). Ни один канал не отдаёт команду: плашка
// (вердикт без install/bootstrap), диалог после обновления ядра, модальное
// предупреждение и classic-гейт (daemonInstallCommandFor /
// privilegedCopyCommandFor), Debug API /daemon/commands. Ядро lx.12 при
// другой копии — Stale с командой.
func TestDaemonServiceCoreTooOld(t *testing.T) {
	const oldCore, newCore = "1.14.1-lx.8", "1.14.1-lx.12"
	l := newTestServiceLayout(t)
	var hashes fileHashCache
	noCommand := func(t *testing.T, c DaemonServiceCheck, blocked DaemonServiceState) {
		t.Helper()
		if c.State != DaemonServiceCoreTooOld || c.BlockedState != blocked {
			t.Fatalf("state %s (blocked %s), want core_too_old over %s (detail: %s)", c.State, c.BlockedState, blocked, c.Detail)
		}
		if c.NeedsInstall() || c.NeedsBootstrap() || c.InstallSupported() {
			t.Fatalf("core_too_old offers a command: install=%v bootstrap=%v supported=%v", c.NeedsInstall(), c.NeedsBootstrap(), c.InstallSupported())
		}
		if !strings.Contains(c.Detail, minCoreForRootOwnedService) || !strings.Contains(c.Detail, string(blocked)) {
			t.Fatalf("detail %q: want the minimum core and the blocked verdict", c.Detail)
		}
		if cmd := daemonCoreUpdatedCommand(c, l.launcherCore); cmd != "" {
			t.Fatalf("core update dialog command %q", cmd)
		}
		hint := DaemonServiceCoreHint(c.LauncherVersion)
		if !strings.Contains(hint, "v"+constants.RequiredCoreVersion) || strings.Contains(hint, "sudo") {
			t.Fatalf("hint %q: want Download v%s and no command", hint, constants.RequiredCoreVersion)
		}
	}

	// Копия lx.12-rc1 цела, ядро лаунчера — другой файл (lx.8).
	writeTestPlist(t, l.PlistPath, l.CorePath)
	writeTestFile(t, l.CorePath, "core lx.12-rc1")
	writeTestFile(t, daemonServiceSidecarPath(l.CorePath), `{"version":"1.14.1-lx.12-rc1"}`)
	writeTestFile(t, l.launcherCore, "core lx.8")
	c := classifyDaemonServiceFiles(l.daemonServiceLayout, l.launcherCore, oldCore, &hashes)
	noCommand(t, c, DaemonServiceStale)
	if !c.CopyUsable() || c.CopyVersion != "1.14.1-lx.12-rc1" || c.LauncherVersion != oldCore {
		t.Fatalf("usable=%v copy %q launcher %q", c.CopyUsable(), c.CopyVersion, c.LauncherVersion)
	}
	// Версия не читается (dev-сборка, нет ядра) — тот же отказ.
	for _, version := range []string{"", "unknown"} {
		noCommand(t, classifyDaemonServiceFiles(l.daemonServiceLayout, l.launcherCore, version, &hashes), DaemonServiceStale)
	}

	// Ядро lx.12, копия другая — Stale с командой install.
	c = classifyDaemonServiceFiles(l.daemonServiceLayout, l.launcherCore, newCore, &hashes)
	if c.State != DaemonServiceStale || !c.NeedsInstall() || c.BlockedState != "" {
		t.Fatalf("lx.12 core: state %s blocked %q install=%v", c.State, c.BlockedState, c.NeedsInstall())
	}
	wantInstall := daemonServiceCommand(l.launcherCore, "lxd", "--service=install")
	if cmd := daemonCoreUpdatedCommand(c, l.launcherCore); cmd != wantInstall {
		t.Fatalf("core update dialog command %q, want %q", cmd, wantInstall)
	}

	// Unsafe (plist на ядро лаунчера) — отказ сохраняет красный вердикт, а
	// копией такая служба не пользуется.
	writeTestPlist(t, l.PlistPath, l.launcherCore)
	c = classifyDaemonServiceFiles(l.daemonServiceLayout, l.launcherCore, oldCore, &hashes)
	noCommand(t, c, DaemonServiceUnsafe)
	if c.CopyUsable() || c.ServicePath != l.launcherCore {
		t.Fatalf("unsafe under core_too_old: usable=%v path %q", c.CopyUsable(), c.ServicePath)
	}

	// ProcessStale (файлы совпали, демон из другого образа) — тот же гейт.
	writeTestPlist(t, l.PlistPath, l.CorePath)
	writeTestFile(t, l.launcherCore, "core lx.12-rc1")
	c = classifyDaemonServiceFiles(l.daemonServiceLayout, l.launcherCore, oldCore, &hashes)
	if c.State != DaemonServiceOK {
		t.Fatalf("same files: state %s (%s)", c.State, c.Detail)
	}
	compareDaemonServiceProcess(&c, lxdclient.InfoData{Executable: l.CorePath, ExecutableSHA256: "ff"}, l.CorePath)
	gateServiceInstall(&c)
	noCommand(t, c, DaemonServiceProcessStale)

	// Команды: install (плашка, вкладка Install, модальное предупреждение)
	// и copy/install classic-гейта — только для ядра lx.11+.
	for _, withPlist := range []bool{false, true} {
		if withPlist {
			writeTestPlist(t, l.PlistPath, l.CorePath)
		} else if err := os.Remove(l.PlistPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if cmd, _, err := privilegedCopyCommandFor(l.daemonServiceLayout, l.launcherCore, oldCore); cmd != "" || err == nil {
			t.Fatalf("plist=%v: classic command %q, err %v", withPlist, cmd, err)
		}
		if cmd, _, err := privilegedCopyCommandFor(l.daemonServiceLayout, l.launcherCore, newCore); cmd == "" || err != nil {
			t.Fatalf("plist=%v: lx.12 classic command %q, err %v", withPlist, cmd, err)
		}
	}
	if cmd, err := daemonInstallCommandFor(l.launcherCore, oldCore); cmd != "" || err == nil {
		t.Fatalf("install command %q, err %v", cmd, err)
	}

	// Debug API /daemon/commands на настоящем «ядре»: версию лаунчер берёт
	// из `sing-box version`, install пуст, пока ядро старше lx.11.
	for version, wantInstall := range map[string]bool{oldCore: false, "unknown": false, newCore: true} {
		fake := filepath.Join(t.TempDir(), "sing-box")
		writeTestFile(t, fake, "#!/bin/sh\necho 'sing-box version "+version+"'\n")
		ac := &AppController{FileService: &services.FileService{SingboxPath: fake}}
		install := (&debugAPIDaemonWiring{ac: ac}).Commands().Install
		if got := install != ""; got != wantInstall {
			t.Fatalf("core %s: Debug API install command %q", version, install)
		}
		if wantInstall && install != daemonServiceCommand(fake, "lxd", "--service=install") {
			t.Fatalf("core %s: Debug API install command %q", version, install)
		}
	}
}
