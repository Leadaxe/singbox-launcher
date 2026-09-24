//go:build darwin

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"singbox-launcher/internal/platform"
)

// TestPrivilegedCoreCopyGate — гейт привилегированного старта classic
// (SPEC 137 §4) на настоящих файлах в раскладке SPEC 136 от своего uid:
// нет копии, цепочка владения, sha копии против ядра лаунчера (кэш, симлинк
// на dev-сборку), нет ядра лаунчера; и выбор одной sudo-команды — copy без
// службы, install при plist службы — с квотингом пути, который разбирает sh.
func TestPrivilegedCoreCopyGate(t *testing.T) {
	l := newTestServiceLayout(t)
	var hashes fileHashCache
	gate := func(t *testing.T, launcherCore string, want privilegedCopyState) privilegedCopyCheck {
		t.Helper()
		c := checkPrivilegedCoreCopy(l.daemonServiceLayout, launcherCore, &hashes)
		if c.State != want {
			t.Fatalf("state = %s, want %s (detail: %s)", c.State, want, c.Detail)
		}
		return c
	}

	// Нет ядра лаунчера — сравнивать не с чем.
	gate(t, "", privilegedCopyNoCore)
	gate(t, filepath.Join(filepath.Dir(l.launcherCore), "absent"), privilegedCopyNoCore)

	// Нет каталога помощников, затем нет самой копии: создать недостающее под
	// root-owned родителем пользователь не может — это «missing», не «unsafe».
	if err := os.Remove(l.toolsDir); err != nil {
		t.Fatal(err)
	}
	gate(t, l.launcherCore, privilegedCopyMissing)
	if err := os.Mkdir(l.toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(l.toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := gate(t, l.launcherCore, privilegedCopyMissing)
	if c.LauncherSHA256 == "" || c.CopySHA256 != "" {
		t.Fatalf("missing: launcher sha %q, copy sha %q", c.LauncherSHA256, c.CopySHA256)
	}
	// Копии нет, а от ранней lx.11 остался каталог <label>/ — он не
	// используется, причина советует его убрать.
	if err := os.Mkdir(l.LegacyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if c = gate(t, l.launcherCore, privilegedCopyMissing); !strings.Contains(c.Detail, "legacy layout") ||
		!strings.Contains(c.Detail, "sudo rm -rf "+shellQuote(l.LegacyPath)) {
		t.Fatalf("missing copy with a legacy folder: %q", c.Detail)
	}
	if err := os.Remove(l.LegacyPath); err != nil {
		t.Fatal(err)
	}

	// Unsafe: на месте файла копии — каталог.
	if err := os.Mkdir(l.CorePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if c = gate(t, l.launcherCore, privilegedCopyUnsafe); !strings.Contains(c.Detail, "remove it") {
		t.Fatalf("folder in place of the copy: %q", c.Detail)
	}
	if err := os.Remove(l.CorePath); err != nil {
		t.Fatal(err)
	}

	// OK: копия = ядро лаунчера; стартует копия. Повтор — из кэша.
	writeTestFile(t, l.CorePath, "core v1")
	c = gate(t, l.launcherCore, privilegedCopyOK)
	if c.CorePath != l.CorePath || c.CopySHA256 == "" || c.CopySHA256 != c.LauncherSHA256 {
		t.Fatalf("ok: core %q copy %q launcher %q", c.CorePath, c.CopySHA256, c.LauncherSHA256)
	}
	computed := hashes.computed
	gate(t, l.launcherCore, privilegedCopyOK)
	if hashes.computed != computed {
		t.Fatalf("unchanged files were re-hashed: %d → %d", computed, hashes.computed)
	}

	// Outdated: ядро лаунчера обновилось (скачивание), копия старая.
	writeTestFile(t, l.launcherCore, "core v2, a longer build")
	c = gate(t, l.launcherCore, privilegedCopyOutdated)
	if c.CopySHA256 == "" || c.LauncherSHA256 == "" || c.CopySHA256 == c.LauncherSHA256 ||
		!strings.Contains(c.Detail, shortSHA(c.CopySHA256)) || !strings.Contains(c.Detail, shortSHA(c.LauncherSHA256)) {
		t.Fatalf("outdated: copy %q launcher %q detail %q", c.CopySHA256, c.LauncherSHA256, c.Detail)
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
	// TempDir на macOS лежит под симлинком /var → /private/var.
	resolvedDev, err := filepath.EvalSymlinks(devBuild)
	if err != nil {
		t.Fatal(err)
	}
	if c = gate(t, l.launcherCore, privilegedCopyOK); c.LauncherCore != resolvedDev {
		t.Fatalf("launcher core resolved to %q, want %q", c.LauncherCore, resolvedDev)
	}

	// Unsafe: каталог помощников пишется группой; копия пишется всеми.
	if err := os.Chmod(l.toolsDir, 0o775); err != nil {
		t.Fatal(err)
	}
	gate(t, l.launcherCore, privilegedCopyUnsafe)
	if err := os.Chmod(l.toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(l.CorePath, 0o757); err != nil {
		t.Fatal(err)
	}
	gate(t, l.launcherCore, privilegedCopyUnsafe)
	if err := os.Chmod(l.CorePath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Unsafe: чужой владелец цепочки (в проде — не root).
	foreign := l.daemonServiceLayout
	foreign.OwnerUID = l.OwnerUID + 1
	if c := checkPrivilegedCoreCopy(foreign, l.launcherCore, &hashes); c.State != privilegedCopyUnsafe {
		t.Fatalf("foreign owner: state %s, want unsafe", c.State)
	}

	// Unsafe: копия — симлинк на файл пользователя с тем же содержимым.
	if err := os.Rename(l.CorePath, l.CorePath+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(devBuild, l.CorePath); err != nil {
		t.Fatal(err)
	}
	gate(t, l.launcherCore, privilegedCopyUnsafe)
	if err := os.Remove(l.CorePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(l.CorePath+".real", l.CorePath); err != nil {
		t.Fatal(err)
	}
	gate(t, l.launcherCore, privilegedCopyOK)

	// Имя плоской копии — то же, что знает platform: по нему pgrep/pkill
	// находят ядро под root, а pid-файл отличает его от чужого PID.
	copyName := filepath.Base(daemonServiceCorePath())
	if copyName != platform.PrivilegedCopyName || daemonServiceCorePath() != "/Library/PrivilegedHelperTools/sing-box-lxd" ||
		daemonServiceSidecarPath(daemonServiceCorePath()) != "/Library/PrivilegedHelperTools/sing-box-lxd.install.json" {
		t.Fatalf("copy %s, platform name %s", daemonServiceCorePath(), platform.PrivilegedCopyName)
	}
	// «sing-box run» копию не ловит — ради неё в шаблоне своя альтернатива.
	if regexp.MustCompile("sing-box run").MatchString(daemonServiceCorePath() + " run -c config.json") {
		t.Fatal(`"sing-box run" must not match the copy's command line`)
	}
	pattern := regexp.MustCompile(platform.PrivilegedPkillPattern)
	for cmdline, want := range map[string]bool{
		daemonServiceCorePath() + " run -c config.json":                                         true,
		"/Users/u/Library/Application Support/singbox-launcher/bin/sing-box run -c config.json": true,
		"/bin/sh -c … " + platform.PrivilegedStartName + " /x/bin":                              true,
		daemonServiceCorePath() + " lxd --state-dir /Library/Application Support/sing-box-lxd":  false,
		"/Users/u/bin/sing-box check -c config.json":                                            false,
	} {
		if got := pattern.MatchString(cmdline); got != want {
			t.Fatalf("pkill pattern on %q: %v, want %v", cmdline, got, want)
		}
	}
	for name, want := range map[string]bool{copyName: true, "sing-box": false, "com.leadaxe.sing": false, "sing-box-lx": false} {
		if got := platform.IsPrivilegedCoreProcessName(name); got != want {
			t.Fatalf("IsPrivilegedCoreProcessName(%q) = %v, want %v", name, got, want)
		}
	}

	// Команда: без службы — copy, при plist службы — install (она обновляет
	// ту же копию). Путь с пробелом и апострофом разбирается sh ровно в
	// задуманные аргументы.
	bin := "/Users/o'brien/My Apps/sing-box"
	for _, tc := range []struct {
		withPlist   bool
		wantService bool
		wantArgs    []string
	}{
		{false, false, []string{bin, "lxd", "--service=copy"}},
		{true, true, []string{bin, "lxd", "--service=install"}},
	} {
		if tc.withPlist {
			writeTestPlist(t, l.PlistPath, l.CorePath)
		}
		command, viaService := privilegedCopyCommandFor(l.daemonServiceLayout, bin)
		if viaService != tc.wantService {
			t.Fatalf("plist=%v: viaService %v, want %v", tc.withPlist, viaService, tc.wantService)
		}
		if out, err := exec.Command("sh", "-n", "-c", command).CombinedOutput(); err != nil {
			t.Fatalf("sh -n %q: %v (%s)", command, err, out)
		}
		out, err := exec.Command("sh", "-c", `sudo() { printf '%s\n' "$@"; }; `+command).Output()
		if err != nil {
			t.Fatalf("sh -c %q: %v", command, err)
		}
		if got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); strings.Join(got, "|") != strings.Join(tc.wantArgs, "|") {
			t.Fatalf("%q parsed as %q, want %q", command, got, tc.wantArgs)
		}
	}
}
