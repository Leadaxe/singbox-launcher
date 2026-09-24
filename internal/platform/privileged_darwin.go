//go:build darwin
// +build darwin

package platform

/*
#cgo LDFLAGS: -framework Security -framework Foundation

#include <stdlib.h>
#include <Security/Security.h>
#include <stdio.h>
#include <unistd.h>

// We use AuthorizationExecuteWithPrivileges (deprecated but still supported) to prompt for password and run sing-box for TUN.
// A single AuthorizationRef is kept and reused while privilegedAuthReuse (Go side) is true, so the user is prompted
// for password only once per app session; otherwise the Go side frees it after every call.
// If the child prints decimal PIDs on the first two lines of stdout (shell PID, then sing-box PID), they are set; otherwise 0.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
static AuthorizationRef g_privilegedAuthRef = NULL;

static int runWithPrivileges(const char *path, char **args, int argCount, pid_t *outScriptPid, pid_t *outSingboxPid) {
	*outScriptPid = 0;
	*outSingboxPid = 0;
	if (g_privilegedAuthRef == NULL) {
		OSStatus status = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment,
			kAuthorizationFlagInteractionAllowed | kAuthorizationFlagExtendRights,
			&g_privilegedAuthRef);
		if (status != errAuthorizationSuccess) {
			return (int)status;
		}
	}

	FILE *pipe = NULL;
	OSStatus status = AuthorizationExecuteWithPrivileges(g_privilegedAuthRef, path,
		kAuthorizationFlagDefaults, args, &pipe);
	// Do not free g_privilegedAuthRef here: the Go side decides (privilegedAuthReuse)

	if (status != errAuthorizationSuccess) {
		return (int)status;
	}
	if (pipe) {
		char buf[32];
		if (fgets(buf, (int)sizeof(buf), pipe)) {
			long p = strtol(buf, NULL, 10);
			if (p > 0)
				*outScriptPid = (pid_t)p;
		}
		if (fgets(buf, (int)sizeof(buf), pipe)) {
			long p = strtol(buf, NULL, 10);
			if (p > 0)
				*outSingboxPid = (pid_t)p;
		}
		fclose(pipe);
	}
	return 0;
}

void freePrivilegedAuthorization(void) {
	if (g_privilegedAuthRef != NULL) {
		AuthorizationFree(g_privilegedAuthRef, kAuthorizationFlagDestroyRights);
		g_privilegedAuthRef = NULL;
	}
}
#pragma clang diagnostic pop
*/
import "C"

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"singbox-launcher/internal/debuglog"
)

// Имена привилегированного запуска (macOS TUN).
const (
	// PrivilegedStartName — $0 root-шелла старта ядра: по нему pgrep/pkill
	// находят обёртку (PrivilegedPkillPattern), как находили прежний скрипт.
	PrivilegedStartName = "start-singbox-privileged"
	// PrivilegedLegacyScriptName — скрипт старта до SPEC 137 (лежал в
	// <Data>/bin). Больше не пишется и не исполняется; лаунчер его удаляет.
	PrivilegedLegacyScriptName = PrivilegedStartName + ".sh"
	PrivilegedPidFileName      = "singbox.pid"
	PrivilegedPkillPattern     = "sing-box run|" + PrivilegedStartName
)

// Что исполняет root (SPEC 137 §3): только root-owned файлы по абсолютным
// путям — копия ядра (путь передаёт core) и системные утилиты. Ничего из
// каталога данных, бандла или PATH.
const (
	privilegedEnvTool   = "/usr/bin/env"
	privilegedShell     = "/bin/sh"
	privilegedKillTool  = "/bin/kill"
	privilegedPkillTool = "/usr/bin/pkill"
	privilegedRmTool    = "/bin/rm"
	// privilegedSafePath — единственная переменная окружения root-шелла.
	// AEWP передаёт инструменту окружение лаунчера, а его задаёт
	// пользователь: PATH решал бы, какой `rm` запустит root, а /bin/sh
	// (bash) подхватывает функции из `BASH_FUNC_<имя>%%` и подменил бы ими
	// даже `echo` постоянного тела. `env -i` отрезает всё это.
	privilegedSafePath = "PATH=/usr/bin:/bin:/usr/sbin:/sbin"
)

// privilegedAuthReuse — время жизни авторизации AEWP (SPEC 137 §6).
//
// true — вариант А (текущий): одна авторизация на сессию лаунчера. Root
// исполняет только root-owned копию ядра после сверки sha и системные
// утилиты с фиксированными аргументами, подменять нечего; пароль — раз
// за сессию.
//
// false — вариант Б: ссылка освобождается после каждого вызова, пароль
// спрашивается на каждый старт, остановку, рестарт при применении конфига,
// снятие TUN и авто-рестарт после падения.
const privilegedAuthReuse = true

// privilegedStartBody — тело root-шелла старта ядра (SPEC 137 §3):
// константа, пути приходят позиционными аргументами — $1 каталог bin,
// $2 копия ядра, $3 имя конфига, $4 лог ядра. Первые две строки stdout —
// PID шелла и PID ядра (их читает runWithPrivileges); затем stdout уходит
// в лог, шелл ждёт ядро, и его выход — выход ядра (WaitForPrivilegedExit).
const privilegedStartBody = `echo $$
cd "$1" || exit 1
"$2" run -c "$3" >>"$4" 2>&1 &
echo $!
exec >>"$4" 2>&1
wait`

// privilegedMu — один вызов AEWP за раз: ссылка авторизации — глобальная
// в C, и при варианте Б её освобождение не должно пересечься с чужим
// вызовом. Второй вызов ждёт, пока первый не получит PID или не выйдет
// (для kill/pkill/rm — доли секунды), а не рисует второй диалог пароля.
var privilegedMu sync.Mutex

// RunWithPrivileges runs the given tool with elevated privileges using the macOS
// Security framework. The user is prompted for their password (once per session
// while privilegedAuthReuse is true). It returns as soon as the child closes its
// stdout or has printed two lines; if those lines are decimal PIDs (shell PID,
// then sing-box PID), they are returned. Otherwise 0, 0.
//
// toolPath must be an absolute path of a root-owned file, and args are passed
// as argv without a shell (SPEC 137): callers go through StartPrivilegedCore,
// KillPrivilegedProcess, KillPrivilegedByPattern and RemoveWithPrivileges.
func RunWithPrivileges(toolPath string, args []string) (scriptPID, singboxPID int, err error) {
	cPath := C.CString(toolPath)
	defer C.free(unsafe.Pointer(cPath))

	// Build NULL-terminated array of C strings for arguments
	cArgs := make([]*C.char, 0, len(args)+1)
	for _, a := range args {
		cArgs = append(cArgs, C.CString(a))
	}
	defer func() {
		for _, p := range cArgs {
			C.free(unsafe.Pointer(p))
		}
	}()
	// NULL terminator
	cArgs = append(cArgs, nil)
	cArgsPtr := &cArgs[0]

	privilegedMu.Lock()
	var cScriptPid, cSingboxPid C.pid_t
	code := C.runWithPrivileges(cPath, cArgsPtr, C.int(len(args)), &cScriptPid, &cSingboxPid)
	if !privilegedAuthReuse {
		C.freePrivilegedAuthorization()
	}
	privilegedMu.Unlock()
	if code != 0 {
		return 0, 0, fmt.Errorf("privileged execution failed with status %d (authorization may have been cancelled)", code)
	}
	return int(cScriptPid), int(cSingboxPid), nil
}

// PrivilegedStartArgs — инструмент и argv AEWP для старта ядра corePath с
// TUN (SPEC 137 §3): `/usr/bin/env -i PATH=… /bin/sh -c <тело> <имя>
// <bin> <ядро> <конфиг> <лог>`. env заменяет себя шеллом через exec — PID
// для Wait4 тот же. Вынесено отдельно ради теста тела без root.
func PrivilegedStartArgs(corePath, binDir, configName, logPath string) (tool string, args []string) {
	return privilegedEnvTool, []string{
		"-i", privilegedSafePath,
		privilegedShell, "-c", privilegedStartBody,
		PrivilegedStartName, binDir, corePath, configName, logPath,
	}
}

// StartPrivilegedCore запускает под root ядро corePath (root-owned копию —
// её проверяет core до вызова) с конфигом configName из binDir и логом
// logPath. Возвращает PID шелла-обёртки и PID ядра.
func StartPrivilegedCore(corePath, binDir, configName, logPath string) (shellPID, corePID int, err error) {
	tool, args := PrivilegedStartArgs(corePath, binDir, configName, logPath)
	return RunWithPrivileges(tool, args)
}

// KillPrivilegedProcess sends SIGTERM to the shell and sing-box PIDs
// (`/bin/kill`, no shell) and removes the pid file. The launcher writes the
// pid file as the user, so it removes it the same way, not as root. Darwin only.
func KillPrivilegedProcess(scriptPID, singboxPID int, pidFile string) error {
	args := []string{"-TERM"}
	for _, pid := range []int{scriptPID, singboxPID} {
		// PID <= 0 не передаём: `kill 0` — вся группа процессов.
		if pid > 0 {
			args = append(args, strconv.Itoa(pid))
		}
	}
	if len(args) > 1 {
		if _, _, err := RunWithPrivileges(privilegedKillTool, args); err != nil {
			return err
		}
	}
	if pidFile != "" {
		if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
			debuglog.WarnLog("KillPrivilegedProcess: remove %s: %v", pidFile, err)
		}
	}
	return nil
}

// KillPrivilegedByPattern — SIGTERM всем процессам привилегированного
// запуска по командной строке (`/usr/bin/pkill -f PrivilegedPkillPattern`,
// без шелла): ядро `sing-box run` и шелл-обёртка. Для диалога «Sing-Box
// already running» и Kill в Diagnostics. Darwin only.
func KillPrivilegedByPattern() error {
	_, _, err := RunWithPrivileges(privilegedPkillTool, []string{"-TERM", "-f", PrivilegedPkillPattern})
	return err
}

// RemoveWithPrivileges удаляет пути под root (`/bin/rm -rf --`, без шелла):
// файлы, которые ядро с TUN оставило root-owned (кэш, логи). Darwin only.
func RemoveWithPrivileges(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"-rf", "--"}, paths...)
	_, _, err := RunWithPrivileges(privilegedRmTool, args)
	return err
}

// WaitForPrivilegedExit waits for the process pid to exit (reaps it to avoid zombie). Darwin only.
func WaitForPrivilegedExit(pid int) {
	if pid <= 0 {
		return
	}
	var status syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &status, 0, nil)
}

// FreePrivilegedAuthorization releases the cached AuthorizationRef so the next RunWithPrivileges will prompt again.
// Call on app exit (e.g. GracefulExit) to avoid leaving the ref alive.
func FreePrivilegedAuthorization() {
	privilegedMu.Lock()
	defer privilegedMu.Unlock()
	C.freePrivilegedAuthorization()
}
