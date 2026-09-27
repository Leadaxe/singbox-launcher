//go:build darwin

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCoreScript создаёт исполняемый скрипт, изображающий ядро: он
// отвечает на те сабкоманды, которые указаны в supports. Копии реального
// ядра и root не нужны: проба способностей только запускает бинарь.
func fakeCoreScript(t *testing.T, dir, name string, supports map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("case \"$1 $2\" in\n")
	for pattern, out := range supports {
		b.WriteString("  \"" + pattern + "\") echo " + shellSingleQuote(out) + "; exit 0;;\n")
	}
	b.WriteString("esac\n")
	b.WriteString("echo \"unknown command\" >&2\nexit 1\n")
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// shellSingleQuote — минимальное экранирование для тестового скрипта.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// TestCoreSupportsServiceCopy — способность копировать себя проверяется у
// самого бинаря, а не по строке версии. Кастомная сборка без `-lx.N` не
// может быть оценена по имени, но её CLI виден напрямую (SPEC 143).
func TestCoreSupportsServiceCopy(t *testing.T) {
	dir := t.TempDir()

	withCopy := fakeCoreScript(t, dir, "core-with-copy", map[string]string{
		"lxd --service=copy": "Usage: sing-box lxd --service=copy  install a root-owned copy",
	})
	withoutLxd := fakeCoreScript(t, dir, "core-no-lxd", map[string]string{})
	withLxdNoCopy := fakeCoreScript(t, dir, "core-lxd-no-copy", map[string]string{
		"lxd --service=copy": "unknown command \"lxd\" for \"sing-box\"",
	})

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "core advertises the copy subcommand", path: withCopy, want: true},
		{name: "core without lxd cannot copy itself", path: withoutLxd, want: false},
		{name: "core that reports unknown command", path: withLxdNoCopy, want: false},
		{name: "missing path", path: filepath.Join(dir, "absent"), want: false},
		{name: "empty path", path: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coreSupportsServiceCopy(tt.path); got != tt.want {
				t.Fatalf("coreSupportsServiceCopy(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestCoreSupportsServiceCopy_RealCoreShape — ядро без lxd (форма
// пользовательской сборки: только run/check/version) не должно считаться
// умеющим копию. Это регрессия на исходную жалобу: лаунчер предлагал
// команды, которые такое ядро выполнить не может.
func TestCoreSupportsServiceCopy_RealCoreShape(t *testing.T) {
	dir := t.TempDir()
	// Форма вывода настоящего ядра без поддержки lxd.
	core := filepath.Join(dir, "sing-box")
	script := "#!/bin/sh\n" +
		"echo 'Error: unknown command \"lxd\" for \"sing-box\"' >&2\n" +
		"echo \"Run 'sing-box --help' for usage.\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(core, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if coreSupportsServiceCopy(core) {
		t.Fatal("a core without lxd must not be reported as able to copy itself")
	}
}

// TestPrivilegedCopyCommandFor_ClassicWithoutLxd — главное требование
// SPEC 143 §5.2: classic TUN не должен требовать от ядра поддержки lxd.
// Кастомное ядро без lxd обязано получить рабочую команду синхронизации
// копии, а не предложение переустановить официальное.
func TestPrivilegedCopyCommandFor_ClassicWithoutLxd(t *testing.T) {
	dir := t.TempDir()
	core := fakeCoreScript(t, dir, "sing-box", map[string]string{})
	copyPath := filepath.Join(dir, "PrivilegedHelperTools", "sing-box-lxd")

	// Служба НЕ установлена (plist отсутствует) — путь classic-копии.
	l := daemonServiceLayout{
		PlistPath: filepath.Join(dir, "absent.plist"),
		CorePath:  copyPath,
		ChainRoot: dir,
		OwnerUID:  0,
	}

	command, viaService, err := privilegedCopyCommandFor(l, core, "1.15.0-jiejie-masquerade.5")
	if err != nil {
		t.Fatalf("a custom core without lxd must still get a copy command for classic TUN: %v", err)
	}
	if viaService {
		t.Fatal("no service is installed, so the command must not be a service install")
	}
	if command == "" {
		t.Fatal("empty command")
	}
	// Копию кладёт лаунчер: install/cp в защищённый каталог, от root.
	for _, want := range []string{"sudo", "/bin/cp", core, copyPath, "root:wheel", "0755"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q lacks %q", command, want)
		}
	}
	// И не просит у ядра lxd, которого у него нет: ядро не запускается ни с
	// какой lxd-сабкомандой. (Имя цели sing-box-lxd — это имя файла копии,
	// а не вызов lxd.)
	if strings.Contains(command, core+" lxd") || strings.Contains(command, "lxd --service") {
		t.Fatalf("command %q must not require lxd support from the core", command)
	}
}

// TestPrivilegedCopyCommandFor_ForkCoreUsesOwnCopy — ядро форка с
// поддержкой копии продолжает пользоваться своим `--service=copy`:
// штатный путь SPEC 137 не меняется.
func TestPrivilegedCopyCommandFor_ForkCoreUsesOwnCopy(t *testing.T) {
	dir := t.TempDir()
	core := fakeCoreScript(t, dir, "sing-box", map[string]string{
		"lxd --service=copy": "Usage: sing-box lxd --service=copy",
	})
	l := daemonServiceLayout{
		PlistPath: filepath.Join(dir, "absent.plist"),
		CorePath:  filepath.Join(dir, "sing-box-lxd"),
		ChainRoot: dir,
		OwnerUID:  0,
	}
	command, viaService, err := privilegedCopyCommandFor(l, core, "1.15.0-lx.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if viaService {
		t.Fatal("no service is installed")
	}
	if !strings.Contains(command, "--service=copy") {
		t.Fatalf("a fork core keeps using its own copy subcommand, got %q", command)
	}
}

// TestPrivilegedCopyCommandFor_ServiceStillGatedByVersion — установка
// СЛУЖБЫ остаётся гейтом по версии: это протокол демона, и кастомное
// ядро без lxd службу установить не может, даже если classic-копия
// теперь доступна.
func TestPrivilegedCopyCommandFor_ServiceStillGatedByVersion(t *testing.T) {
	dir := t.TempDir()
	core := fakeCoreScript(t, dir, "sing-box", map[string]string{})
	plist := filepath.Join(dir, "com.leadaxe.sing-box-lxd.plist")
	if err := os.WriteFile(plist, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := daemonServiceLayout{
		PlistPath: plist,
		CorePath:  filepath.Join(dir, "sing-box-lxd"),
		ChainRoot: dir,
		OwnerUID:  0,
	}
	command, viaService, err := privilegedCopyCommandFor(l, core, "1.15.0-jiejie-masquerade.5")
	if err == nil {
		t.Fatalf("installing the daemon service must stay gated by version, got command %q", command)
	}
	if viaService != true {
		t.Fatal("the refusal must still report that a service is defined")
	}
}

// TestPrivilegedCopyCommandFor_OldForkReleaseStillRefused — опознанный, но
// слишком старый релиз форка (1.14.1-lx.8) команды не получает: его версия
// обещает свою раскладку копии, и подменять её копией от лаунчера нельзя.
// Здесь сохраняется прежнее поведение SPEC 137.
func TestPrivilegedCopyCommandFor_OldForkReleaseStillRefused(t *testing.T) {
	dir := t.TempDir()
	core := fakeCoreScript(t, dir, "sing-box", map[string]string{})
	l := daemonServiceLayout{
		PlistPath: filepath.Join(dir, "absent.plist"),
		CorePath:  filepath.Join(dir, "sing-box-lxd"),
		ChainRoot: dir,
		OwnerUID:  0,
	}
	command, _, err := privilegedCopyCommandFor(l, core, "1.14.1-lx.8")
	if err == nil {
		t.Fatalf("an old numbered fork release must be refused, got command %q", command)
	}
	if command != "" {
		t.Fatalf("command must be empty for an old fork release, got %q", command)
	}
}

// TestPrivilegedCopyCommandFor_UnrecognizedVersionGetsCommand — кастомная
// сборка без `-lx.N` (как 1.15.0-jiejie-masquerade.5) команду получает:
// classic TUN не требует lxd, а версия о способностях не говорит — решает
// проба бинаря.
func TestPrivilegedCopyCommandFor_UnrecognizedVersionGetsCommand(t *testing.T) {
	dir := t.TempDir()
	core := fakeCoreScript(t, dir, "sing-box", map[string]string{})
	l := daemonServiceLayout{
		PlistPath: filepath.Join(dir, "absent.plist"),
		CorePath:  filepath.Join(dir, "sing-box-lxd"),
		ChainRoot: dir,
		OwnerUID:  0,
	}
	command, viaService, err := privilegedCopyCommandFor(l, core, "1.15.0-jiejie-masquerade.5")
	if err != nil {
		t.Fatalf("an unrecognized custom version must still get a copy command: %v", err)
	}
	if viaService {
		t.Fatal("no service is installed")
	}
	if command == "" {
		t.Fatal("empty command")
	}
}

// TestRootCopyInstallCommand — команда копии атомарна и не оставляет
// полузаписанный бинарь на месте цели: копия идёт во временный файл в том
// же каталоге и переименовывается.
func TestRootCopyInstallCommand(t *testing.T) {
	src := "/Users/dev core/bin/sing-box" // пробел — проверка кавычек
	dst := "/Library/PrivilegedHelperTools/sing-box-lxd"
	cmd := rootCopyInstallCommand(src, dst)

	for _, want := range []string{
		shellQuote(src),
		shellQuote(dst),
		shellQuote(dst + ".new"),
		"/bin/mkdir -p",
		"/bin/cp -f",
		"/usr/sbin/chown root:wheel",
		"/bin/chmod 0755",
		"/bin/mv -f",
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command %q lacks %q", cmd, want)
		}
	}
	// Временный файл — рядом с целью, иначе mv не атомарен между томами.
	if filepath.Dir(dst+".new") != filepath.Dir(dst) {
		t.Fatal("the temp file must live in the destination directory for an atomic rename")
	}
	// Путь с пробелом обязан быть закавычен.
	if strings.Contains(cmd, "cp -f "+src+" ") {
		t.Fatalf("the source path with a space is not quoted: %q", cmd)
	}
}
