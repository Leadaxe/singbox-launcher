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
// The raw first line is copied to outFirstLine: a refusing command explains itself there instead of a PID.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
static AuthorizationRef g_privilegedAuthRef = NULL;

static int runWithPrivileges(const char *path, char **args, int argCount, pid_t *outScriptPid, pid_t *outSingboxPid,
	char *outFirstLine, int outFirstLineLen) {
	*outScriptPid = 0;
	*outSingboxPid = 0;
	if (outFirstLineLen > 0)
		outFirstLine[0] = 0;
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
		if (outFirstLineLen > 0 && fgets(outFirstLine, outFirstLineLen, pipe)) {
			long p = strtol(outFirstLine, NULL, 10);
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
	"strings"
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
	// PrivilegedCopyName — имя файла root-owned копии ядра
	// (/Library/PrivilegedHelperTools/sing-box-lxd, SPEC 136/137, ядро lx.12):
	// процесс ядра под root зовётся так (12 символов — p_comm не усекает).
	// Путь копии в core строится от этого имени.
	PrivilegedCopyName = "sing-box-lxd"
	// PrivilegedPkillPattern — командные строки привилегированного запуска
	// для pgrep/pkill -f: ядро лаунчера (`sing-box run`), копия
	// (`sing-box-lxd run`; «sing-box run» её не ловит — после sing-box идёт
	// «-lxd») и root-шелл обёртки. Демон службы (`… lxd --state-dir`) под
	// шаблон не попадает.
	PrivilegedPkillPattern = "sing-box run|" + PrivilegedCopyName + " run|" + PrivilegedStartName
)

// IsPrivilegedCoreProcessName — имя процесса — это root-owned копия ядра.
func IsPrivilegedCoreProcessName(name string) bool {
	return name == PrivilegedCopyName
}

// Что исполняет root (SPEC 137 §3): только root-owned файлы по абсолютным
// путям — копия ядра (путь передаёт core) и системные утилиты. Ничего из
// каталога данных, бандла или PATH.
const (
	privilegedEnvTool   = "/usr/bin/env"
	privilegedShell     = "/bin/sh"
	privilegedKillTool  = "/bin/kill"
	privilegedPkillTool = "/usr/bin/pkill"
	// privilegedSafePath — единственная переменная окружения root-шелла.
	// AEWP передаёт инструменту окружение лаунчера, а его задаёт
	// пользователь: PATH решал бы, какой `rm` запустит root, а /bin/sh
	// (bash) подхватывает функции из `BASH_FUNC_<имя>%%` и подменил бы ими
	// даже `echo` постоянного тела. `env -i` отрезает всё это.
	privilegedSafePath = "PATH=/usr/bin:/bin:/usr/sbin:/sbin"
)

// Лог ядра, запущенного под root (SPEC 137.1): root не пишет по путям
// пользователя, вывод ядра идёт в root-owned каталог, лаунчер его только
// читает. Не /Library/Application Support/sing-box-lxd: тот 0700, и без
// root его не прочитать. Файл принадлежит пользователю лаунчера, 0600:
// другие локальные учётные записи его не читают, а подменить его нельзя —
// каталог root-owned.
const (
	// PrivilegedLogDir — каталог лога (root:wheel 0755), его создаёт и
	// проверяет тело старта.
	PrivilegedLogDir = "/Library/Logs/sing-box-lxd"
	// privilegedLogName — файл лога classic-ядра (пользователь лаунчера,
	// 0600); .old — прошлый, тот же владелец.
	privilegedLogName = "classic.log"
	// privilegedLogRotateBytes — порог ротации при старте: как у лога ядра
	// в каталоге пользователя (maxLogFileSize в core/services).
	privilegedLogRotateBytes = 2 * 1024 * 1024
	// privilegedMinUserUID — первый uid обычной учётной записи macOS: лог
	// отдаётся только такому владельцу (строка — её вставляет тело).
	privilegedMinUserUID = "501"
)

// PrivilegedCoreLogPath — файл, куда пишет вывод ядро, запущенное под root.
func PrivilegedCoreLogPath() string {
	return PrivilegedLogDir + "/" + privilegedLogName
}

// privilegedAuthReuse — время жизни авторизации AEWP (SPEC 137 §6).
//
// true — вариант А (текущий): одна авторизация на сессию лаунчера. Root
// исполняет только root-owned копию ядра после сверки sha и системные
// утилиты с фиксированными аргументами, подменять нечего; пароль — раз
// за сессию.
//
// false — вариант Б: ссылка освобождается после каждого вызова, пароль
// спрашивается на каждый старт, остановку, рестарт при применении конфига,
// Kill и авто-рестарт после падения (снятие TUN root не требует, 137.1).
const privilegedAuthReuse = true

// privilegedStartBody — тело root-шелла старта ядра (SPEC 137 §3, 137.1):
// константа, пути приходят позиционными аргументами — $1 каталог bin,
// $2 копия ядра, $3 имя конфига, $4 каталог лога, $5 ожидаемый владелец
// каталога лога (uid; 0 в проде), $6 uid пользователя лаунчера — владелец
// файла лога, $7 порог ротации в байтах.
//
// До первого PID тело готовит лог, ничего не следуя по симлинкам: $6 —
// только цифры, не меньше 501, пользователь существует (`/usr/bin/id`);
// каталог не симлинк, создаётся и проверяется как каталог владельца $5,
// 0755; файл, если есть, — обычный файл этого пользователя или root,
// отдаётся пользователю (0600), больше порога — уезжает в .old с тем же
// владельцем; новый файл — тоже пользователю, 0600. Отказ — строка
// «refused: <причина>» вместо PID (её читает RunWithPrivileges), и ядро не
// стартует. Затем первые две строки stdout — PID шелла и PID ядра; stdout
// шелла уходит в лог, шелл ждёт ядро, и его выход — выход ядра
// (WaitForPrivilegedExit).
const privilegedStartBody = `umask 022
d="$4"
f="$d/` + privilegedLogName + `"
u="$6"
case "$u" in ''|*[!0-9]*) echo "refused: invalid uid '$u'"; exit 1;; esac
if [ "$u" -lt ` + privilegedMinUserUID + ` ]; then echo "refused: uid $u is not a regular user"; exit 1; fi
g="$(/usr/bin/id -g "$u" 2>/dev/null)" || { echo "refused: no user with uid $u"; exit 1; }
case "$g" in ''|*[!0-9]*) echo "refused: no group for uid $u"; exit 1;; esac
if [ -L "$d" ]; then echo "refused: $d is a symbolic link"; exit 1; fi
/bin/mkdir -p "$d" || { echo "refused: cannot create $d"; exit 1; }
if [ "$(/usr/bin/stat -f '%u:%HT' "$d")" != "$5:Directory" ]; then echo "refused: $d is not a directory owned by uid $5"; exit 1; fi
/bin/chmod 0755 "$d" || { echo "refused: cannot chmod $d"; exit 1; }
if [ -e "$f" ] || [ -L "$f" ]; then
  o="$(/usr/bin/stat -f '%u:%HT' "$f")"
  if [ "$o" != "$u:Regular File" ] && [ "$o" != "0:Regular File" ]; then echo "refused: $f is not a regular file owned by uid $u or root"; exit 1; fi
  /bin/chmod 0600 "$f" && /usr/sbin/chown "$u:$g" "$f" || { echo "refused: cannot hand $f to uid $u"; exit 1; }
  if [ "$(/usr/bin/stat -f %z "$f")" -gt "$7" ]; then /bin/mv -f "$f" "$f.old" || { echo "refused: cannot rotate $f"; exit 1; }; fi
fi
: >>"$f" || { echo "refused: cannot open $f"; exit 1; }
/bin/chmod 0600 "$f" && /usr/sbin/chown "$u:$g" "$f" || { echo "refused: cannot hand $f to uid $u"; exit 1; }
cd "$1" || { echo "refused: cannot enter $1"; exit 1; }
echo $$
"$2" run -c "$3" >>"$f" 2>&1 &
echo $!
exec >>"$f" 2>&1
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
// KillPrivilegedProcess and KillPrivilegedByPattern. A non-PID first line of
// stdout (a refusal reason) comes back as the error.
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
	var firstLine [512]C.char
	code := C.runWithPrivileges(cPath, cArgsPtr, C.int(len(args)), &cScriptPid, &cSingboxPid, &firstLine[0], C.int(len(firstLine)))
	if !privilegedAuthReuse {
		C.freePrivilegedAuthorization()
	}
	privilegedMu.Unlock()
	if code != 0 {
		return 0, 0, fmt.Errorf("privileged execution failed with status %d (authorization may have been cancelled)", code)
	}
	// Вместо PID — строка: команда отказалась и объяснила почему (тело
	// старта: «refused: …»). kill/pkill в stdout не пишут — там пусто.
	if cScriptPid == 0 {
		if msg := strings.TrimSpace(C.GoString(&firstLine[0])); msg != "" {
			return 0, 0, fmt.Errorf("%s", msg)
		}
	}
	return int(cScriptPid), int(cSingboxPid), nil
}

// PrivilegedStartArgs — инструмент и argv AEWP для старта ядра corePath с
// TUN (SPEC 137 §3): `/usr/bin/env -i PATH=… /bin/sh -c <тело> <имя>
// <bin> <ядро> <конфиг> <каталог лога> <владелец каталога> <uid
// пользователя> <порог>`. env заменяет себя шеллом через exec — PID для
// Wait4 тот же. Каталог лога и его владелец — параметры ради теста тела без
// root; прод — StartPrivilegedCore.
func PrivilegedStartArgs(corePath, binDir, configName, logDir string, logDirOwnerUID, userUID int, rotateBytes int64) (tool string, args []string) {
	return privilegedEnvTool, []string{
		"-i", privilegedSafePath,
		privilegedShell, "-c", privilegedStartBody,
		PrivilegedStartName, binDir, corePath, configName,
		logDir, strconv.Itoa(logDirOwnerUID), strconv.Itoa(userUID), strconv.FormatInt(rotateBytes, 10),
	}
}

// StartPrivilegedCore запускает под root ядро corePath (root-owned копию —
// её проверяет core до вызова) с конфигом configName из binDir. Вывод ядра —
// в PrivilegedCoreLogPath (каталог root-owned, файл — пользователю лаунчера
// 0600, SPEC 137.1). Возвращает PID
// шелла-обёртки и PID ядра; отказ тела — ошибка с его причиной.
func StartPrivilegedCore(corePath, binDir, configName string) (shellPID, corePID int, err error) {
	tool, args := PrivilegedStartArgs(corePath, binDir, configName, PrivilegedLogDir, 0, os.Getuid(), privilegedLogRotateBytes)
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
