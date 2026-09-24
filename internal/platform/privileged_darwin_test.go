package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestPrivilegedStartCommand — команда старта ядра под root (SPEC 137 §3,
// 137.1) без root: тело — синтаксически верный sh; всё, что оно и AEWP
// исполняют, — root-owned файлы по абсолютным путям. argv, прогнанный как
// есть (без AEWP, владелец каталога лога — свой uid вместо 0), с поддельным
// ядром в пути с пробелом и апострофом и с отравленным окружением
// (`BASH_FUNC_echo%%`, чужой PATH): создаёт каталог лога 0755, ротирует
// большой лог в .old, печатает ровно два PID — шелла (тот же процесс: env
// делает exec) и ядра, — запускает ядро из каталога bin с `run -c
// <конфиг>`, чистым PATH и без функций окружения, пишет его вывод в лог
// пользователя 0600 (и .old — 0600) и выходит только после ядра. Отказ с
// причиной вместо PID, ядро не стартует: uid пользователя не число, меньше
// 501 или без учётной записи; симлинк на месте каталога или файла лога;
// чужой владелец каталога.
func TestPrivilegedStartCommand(t *testing.T) {
	if out, err := exec.Command(privilegedShell, "-n", "-c", privilegedStartBody).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v (%s)", err, out)
	}
	uid := os.Getuid()
	if minUID, _ := strconv.Atoi(privilegedMinUserUID); uid < minUID {
		t.Skipf("uid %d is below %s: the body hands the log only to a regular user", uid, privilegedMinUserUID)
	}
	for _, tool := range []string{privilegedEnvTool, privilegedShell, privilegedKillTool, privilegedPkillTool,
		"/usr/bin/stat", "/bin/mkdir", "/bin/chmod", "/bin/mv", "/usr/sbin/chown", "/usr/bin/id"} {
		fi, err := os.Lstat(tool)
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !filepath.IsAbs(tool) || !fi.Mode().IsRegular() || !ok || st.Uid != 0 || fi.Mode().Perm()&0o022 != 0 {
			t.Fatalf("%s must be a root-owned regular file without group/other write (mode %v)", tool, fi.Mode())
		}
	}
	if !strings.HasPrefix(PrivilegedCoreLogPath(), PrivilegedLogDir+"/") || !strings.HasPrefix(PrivilegedLogDir, "/Library/Logs/") {
		t.Fatalf("privileged log %q must live in a root-owned /Library/Logs folder", PrivilegedCoreLogPath())
	}

	base := filepath.Join(t.TempDir(), "o'brien data")
	binDir := filepath.Join(base, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Имя поддельного ядра — как у плоской копии: по нему pgrep/pkill
	// находят ядро под root.
	core := filepath.Join(base, "copy dir", PrivilegedCopyName)
	if err := os.MkdirAll(filepath.Dir(core), 0o755); err != nil {
		t.Fatal(err)
	}
	// Поддельное ядро: печатает cwd, аргументы и окружение, ждёт и
	// отмечается «done» — по этой строке видно, что шелл дождался ядра.
	fake := "#!/bin/sh\n" +
		"echo \"cwd=$(/bin/pwd -P)\"\n" +
		"echo \"args=$*\"\n" +
		"/usr/bin/env\n" +
		"/bin/sleep 2\n" +
		"echo done\n"
	if err := os.WriteFile(core, []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(base, "Logs dir", "sing-box-lxd")
	logPath := filepath.Join(logDir, privilegedLogName)
	const rotate = 64

	// run — argv как есть; userUID != "" подменяет аргумент uid пользователя
	// (проверка валидации в теле).
	run := func(t *testing.T, dirOwnerUID int, userUID string) (string, *exec.Cmd) {
		t.Helper()
		tool, args := PrivilegedStartArgs(core, binDir, "config.json", logDir, dirOwnerUID, uid, rotate)
		if userUID != "" {
			args[11] = userUID
		}
		cmd := exec.Command(tool, args...)
		cmd.Env = append(os.Environ(),
			"BASH_FUNC_echo%%=() { printf 'HIJACK\\n'; }",
			"PATH=/nonexistent:"+os.Getenv("PATH"))
		out, err := cmd.Output()
		if err != nil && !strings.HasPrefix(string(out), "refused: ") {
			t.Fatalf("run: %v (%s)", err, out)
		}
		return string(out), cmd
	}

	tool, args := PrivilegedStartArgs(core, binDir, "config.json", logDir, 0, uid, rotate)
	if tool != privilegedEnvTool || len(args) != 13 || args[0] != "-i" || args[1] != privilegedSafePath ||
		args[2] != privilegedShell || args[3] != "-c" || args[4] != privilegedStartBody || args[5] != PrivilegedStartName ||
		args[6] != binDir || args[7] != core || args[8] != "config.json" || args[9] != logDir ||
		args[10] != "0" || args[11] != strconv.Itoa(uid) || args[12] != strconv.Itoa(rotate) {
		t.Fatalf("unexpected command: %s %q", tool, args)
	}

	// Отказы по uid пользователя: не число, системный, несуществующий.
	for _, bad := range []string{"abc", "5o1", "-1", "500", "0", "4000000000"} {
		if out, _ := run(t, uid, bad); !strings.HasPrefix(out, "refused: ") || strings.Contains(out, "done") {
			t.Fatalf("user uid %q: %q", bad, out)
		}
	}
	if _, err := os.Lstat(logDir); !os.IsNotExist(err) {
		t.Fatalf("a refused uid must stop before the log folder is touched: %v", err)
	}
	if !strings.Contains(PrivilegedPkillPattern, PrivilegedStartName) {
		t.Fatalf("pkill pattern %q does not match the shell name %q", PrivilegedPkillPattern, PrivilegedStartName)
	}

	// Отказы до старта ядра: симлинк на месте каталога лога.
	if err := os.MkdirAll(filepath.Dir(logDir), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(base, "decoy")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(decoy, logDir); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, uid, ""); !strings.HasPrefix(out, "refused: ") || strings.Contains(out, "done") {
		t.Fatalf("symlinked log folder: %q", out)
	}
	if err := os.Remove(logDir); err != nil {
		t.Fatal(err)
	}
	// Чужой владелец каталога (в проде — не root).
	if out, _ := run(t, uid+1, ""); !strings.HasPrefix(out, "refused: ") {
		t.Fatalf("foreign owner: %q", out)
	}
	// Симлинк на месте файла лога.
	if err := os.Symlink(filepath.Join(decoy, "target"), logPath); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, uid, ""); !strings.HasPrefix(out, "refused: ") {
		t.Fatalf("symlinked log file: %q", out)
	}
	if _, err := os.Stat(filepath.Join(decoy, "target")); !os.IsNotExist(err) {
		t.Fatalf("the refused start wrote through the symlink: %v", err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}

	// Старт: лог больше порога уезжает в .old.
	if err := os.Chmod(logDir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(strings.Repeat("x", rotate+1)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Пока ядро работает, `pgrep -f PrivilegedPkillPattern` (тот же шаблон у
	// pkill Kill-кнопок) находит и ядро-копию, и шелл обёртки.
	pgrepCh := make(chan string, 1)
	go func() {
		time.Sleep(400 * time.Millisecond)
		found, _ := exec.Command("/usr/bin/pgrep", "-f", PrivilegedPkillPattern).Output()
		pgrepCh <- string(found)
	}()
	out, cmd := run(t, uid, "")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout must be exactly two PID lines, got %q", out)
	}
	shellPID, err1 := strconv.Atoi(lines[0])
	corePID, err2 := strconv.Atoi(lines[1])
	if err1 != nil || err2 != nil || shellPID != cmd.Process.Pid || corePID <= 0 || corePID == shellPID {
		t.Fatalf("PIDs %q: want shell %d and a separate core PID", lines, cmd.Process.Pid)
	}
	matched := strings.Fields(<-pgrepCh)
	for _, pid := range []int{shellPID, corePID} {
		hit := false
		for _, m := range matched {
			if m == strconv.Itoa(pid) {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("pgrep -f %q found %v, missing PID %d", PrivilegedPkillPattern, matched, pid)
		}
	}
	for path, want := range map[string]os.FileMode{logDir: 0o755, logPath: 0o600, logPath + ".old": 0o600} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := fi.Sys().(*syscall.Stat_t)
		if fi.Mode().Perm() != want || st == nil || int(st.Uid) != uid {
			t.Fatalf("%s: mode %04o, want %04o, owned by the launcher user %d", path, fi.Mode().Perm(), want, uid)
		}
	}
	if old, err := os.ReadFile(logPath + ".old"); err != nil || !strings.HasPrefix(string(old), "xxx") {
		t.Fatalf("the oversized log was not rotated to .old: %v", err)
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
		if strings.Contains(logText, leaked) || strings.Contains(out, leaked) {
			t.Fatalf("launcher environment leaked into the root shell (%s):\n%s", leaked, logText)
		}
	}
}
