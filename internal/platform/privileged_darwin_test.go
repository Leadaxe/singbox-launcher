package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestPrivilegedStartCommand — команда старта ядра под root (SPEC 137 §3)
// без root: тело — синтаксически верный sh; все утилиты — root-owned файлы
// по абсолютным путям; argv, прогнанный как есть (без AEWP) с поддельным
// ядром в пути с пробелом и апострофом и с отравленным окружением
// (`BASH_FUNC_echo%%`, чужой PATH), печатает ровно два PID — шелла (тот же
// процесс, что запущен: env делает exec) и ядра, — запускает ядро из
// каталога bin с `run -c <конфиг>`, чистым PATH и без функций окружения,
// дописывает его вывод в лог и выходит только после ядра.
func TestPrivilegedStartCommand(t *testing.T) {
	if out, err := exec.Command(privilegedShell, "-n", "-c", privilegedStartBody).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v (%s)", err, out)
	}
	for _, tool := range []string{privilegedEnvTool, privilegedShell, privilegedKillTool, privilegedPkillTool, privilegedRmTool} {
		fi, err := os.Lstat(tool)
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !filepath.IsAbs(tool) || !fi.Mode().IsRegular() || !ok || st.Uid != 0 || fi.Mode().Perm()&0o022 != 0 {
			t.Fatalf("%s must be a root-owned regular file without group/other write (mode %v)", tool, fi.Mode())
		}
	}

	base := filepath.Join(t.TempDir(), "o'brien data")
	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(base, "copy dir", "sing-box")
	if err := os.MkdirAll(filepath.Dir(core), 0o755); err != nil {
		t.Fatal(err)
	}
	// Поддельное ядро: печатает cwd, аргументы и окружение, ждёт и
	// отмечается «done» — по этой строке видно, что шелл дождался ядра.
	fake := "#!/bin/sh\n" +
		"echo \"cwd=$(/bin/pwd -P)\"\n" +
		"echo \"args=$*\"\n" +
		"/usr/bin/env\n" +
		"/bin/sleep 0.3\n" +
		"echo done\n"
	if err := os.WriteFile(core, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(base, "logs dir", "sing-box.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("previous line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool, args := PrivilegedStartArgs(core, binDir, "config.json", logPath)
	if tool != privilegedEnvTool || len(args) != 10 || args[0] != "-i" || args[1] != privilegedSafePath ||
		args[2] != privilegedShell || args[3] != "-c" || args[4] != privilegedStartBody || args[5] != PrivilegedStartName ||
		args[6] != binDir || args[7] != core || args[8] != "config.json" || args[9] != logPath {
		t.Fatalf("unexpected command: %s %q", tool, args)
	}
	if !strings.Contains(PrivilegedPkillPattern, PrivilegedStartName) {
		t.Fatalf("pkill pattern %q does not match the shell name %q", PrivilegedPkillPattern, PrivilegedStartName)
	}

	cmd := exec.Command(tool, args...)
	cmd.Env = append(os.Environ(),
		"BASH_FUNC_echo%%=() { printf 'HIJACK\\n'; }",
		"PATH=/nonexistent:"+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout must be exactly two PID lines, got %q", out)
	}
	shellPID, err1 := strconv.Atoi(lines[0])
	corePID, err2 := strconv.Atoi(lines[1])
	if err1 != nil || err2 != nil || shellPID != cmd.Process.Pid || corePID <= 0 || corePID == shellPID {
		t.Fatalf("PIDs %q: want shell %d and a separate core PID", lines, cmd.Process.Pid)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(data)
	resolvedBin, err := filepath.EvalSymlinks(binDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"previous line\n",
		"cwd=" + resolvedBin + "\n",
		"args=run -c config.json\n",
		privilegedSafePath + "\n",
		"done\n",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log lacks %q:\n%s", want, logText)
		}
	}
	for _, leaked := range []string{"BASH_FUNC", "HIJACK", "/nonexistent"} {
		if strings.Contains(logText, leaked) || strings.Contains(string(out), leaked) {
			t.Fatalf("launcher environment leaked into the root shell (%s):\n%s", leaked, logText)
		}
	}
}
