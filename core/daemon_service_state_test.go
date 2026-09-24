//go:build darwin

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/internal/lxdclient"
)

// testServiceLayout — раскладка службы lx.11 во временном каталоге:
// <base>/Library/PrivilegedHelperTools/<label>/sing-box, plist в
// <base>/LaunchDaemons, ядро лаунчера в <base>/data/bin. Владелец цепочки —
// текущий uid (root-owned файлы тест создать не может).
type testServiceLayout struct {
	daemonServiceLayout
	helperDir    string
	launcherCore string
}

func newTestServiceLayout(t *testing.T) testServiceLayout {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "Library")
	helper := filepath.Join(root, "PrivilegedHelperTools", daemonLaunchdLabel)
	for _, dir := range []string{helper, filepath.Join(base, "LaunchDaemons"), filepath.Join(base, "data", "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// MkdirAll уважает umask: выставляем права цепочки явно.
	for _, dir := range []string{root, filepath.Dir(helper), helper} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	l := testServiceLayout{
		daemonServiceLayout: daemonServiceLayout{
			PlistPath: filepath.Join(base, "LaunchDaemons", daemonLaunchdLabel+".plist"),
			CorePath:  filepath.Join(helper, daemonServiceBinaryName),
			ChainRoot: root,
			OwnerUID:  uint32(os.Getuid()),
		},
		helperDir:    helper,
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

	// OK: копия = ядро лаунчера. Повтор с теми же файлами — из кэша.
	writeTestFile(t, l.CorePath, "core v1")
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

	// Unsafe: каталог службы пишется группой.
	if err := os.Chmod(l.helperDir, 0o775); err != nil {
		t.Fatal(err)
	}
	expect(t, classifyTestService(l, &hashes), DaemonServiceUnsafe)
	if err := os.Chmod(l.helperDir, 0o755); err != nil {
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
// когда она безопасна (SPEC 136 §5).
func TestDaemonServiceCommandQuoting(t *testing.T) {
	bin := "/Users/o'brien/My Apps/\"lx\" core/sing-box"
	commands := map[string][]string{
		daemonServiceCommand(bin, "lxd", "--service=install"): {bin, "lxd", "--service=install"},
		daemonUninstallCommandFor(bin, true):                  {bin, "lxd", "--service=uninstall", "--purge"},
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
