//go:build windows && !386

package core

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Служба демона на Windows (SPEC 141 §5), платформенная часть менеджера:
// имя службы SCM, рендер команды для показа и Copy (PowerShell), исполнение
// команды через runas (platform.RunElevated, окно UAC) с ожиданием кода
// выхода, операции Install / Start / Fresh invite / Uninstall / Copy с
// автосопряжением по файлу --invite-out. Общее — daemon_manager.go.
//
// Этап API (SPEC 141, фаза 1): типы и сигнатуры зафиксированы
// (API_WINDOWS.md), тела операций — заглушки errDaemonWindowsPending;
// движок закрыт daemonEngineAvailable, поэтому в рантайме до них доходит
// только «службы нет».

// errDaemonWindowsPending — ответ заглушек, пока нет ядра со службой Windows.
var errDaemonWindowsPending = errors.New("daemon mode on Windows arrives with core v1.14.2-lx.2")

// errDaemonCommandStillRunning — команда под runas не завершилась за
// daemonRunWaitTimeout; процесс не убивается (SPEC 141 §5.2 п. 3).
var errDaemonCommandStillRunning = errors.New("the administrator command is still running")

const (
	// daemonServiceName — имя службы у SCM (SPEC 141 §3 п. 1).
	daemonServiceName = "sing-box-lxd"
	// daemonRunWaitTimeout — ожидание выхода команды под runas (§5.2 п. 3).
	daemonRunWaitTimeout = 120 * time.Second
	// daemonClientNamePrefix — имя клиента на пользователя:
	// singbox-launcher-<user> (§5.3 «Имя клиента»).
	daemonClientNamePrefix = "singbox-launcher-"
)

// DaemonOpsElevated — на Windows операции службы исполняет лаунчер через
// runas (одно окно UAC), ждёт кода выхода и возвращает итог в
// DaemonRunResult. Кнопка — «Run as administrator» (NotRunning — «Start
// the service»).
const DaemonOpsElevated = true

// daemonFallbackRuntimeDir — TODO(SPEC 141 фаза 2): <ProgramData>\sing-box-lxd\state
// через windows.KnownFolderPath (FOLDERID_ProgramData); константа станет
// функцией и на macOS.
const daemonFallbackRuntimeDir = ""

// daemonEngineAvailable — TODO(SPEC 141 фаза 2): гейт по версии ядра
// (≥ 1.14.2-lx.2) вместо безусловного отказа; пока лаунчер на Windows
// остаётся на classic.
func daemonEngineAvailable() error { return errDaemonWindowsPending }

// daemonServiceCommand — рендер команды для показа и Copy (SPEC 141 §5.1):
// PowerShell `& '<путь>' <args>`; одинарная кавычка внутри → ''. Аргументы
// без пробелов и спецсимволов PowerShell идут как есть, прочие — в кавычках.
// Исполняется не строка, а argv (runDaemonCommandElevated).
func daemonServiceCommand(binary string, args ...string) string {
	parts := make([]string, 0, len(args)+2)
	parts = append(parts, "&", powerShellQuote(binary))
	for _, a := range args {
		if powerShellBareArg(a) {
			parts = append(parts, a)
		} else {
			parts = append(parts, powerShellQuote(a))
		}
	}
	return strings.Join(parts, " ")
}

// powerShellQuote — строковый литерал PowerShell в одинарных кавычках.
func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// powerShellBareArg — аргумент можно не заключать в кавычки: непустой,
// только буквы, цифры и -_=.:\/.
func powerShellBareArg(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(`-_=.:\/`, r):
		default:
			return false
		}
	}
	return true
}

// daemonScExe — %SystemRoot%\System32\sc.exe по системному каталогу.
func daemonScExe() string {
	dir, err := windows.GetSystemDirectory()
	if err != nil || dir == "" {
		dir = `C:\Windows\System32`
	}
	return filepath.Join(dir, "sc.exe")
}

// daemonBootstrapCommand — NotRunning: `sc.exe start sing-box-lxd` под runas
// (START у Authenticated Users нет, SPEC 141 §3 п. 1a, §13 п. 4).
func daemonBootstrapCommand() string {
	return daemonServiceCommand(daemonScExe(), "start", daemonServiceName)
}

// DaemonKickstartCommand — на Windows команды нет (SPEC 141 §5.1): install и
// copy сами перезапускают службу.
func (ac *AppController) DaemonKickstartCommand() string { return "" }

// DaemonShowSecretCommand — TODO(SPEC 141 фаза 2): чтение "secret" из
// <ProgramData>\sing-box-lxd\state\daemon.json (SYSTEM + Administrators) —
// команда для консоли администратора, только Copy.
func (ac *AppController) DaemonShowSecretCommand() string { return "" }

// daemonClientName — TODO(SPEC 141 фаза 2): singbox-launcher-<user>, <user>
// — SAM account name в нижнем регистре, символы вне [a-z0-9_-] → _, всё
// имя не длиннее 64 (§5.3). Уходит в --invite-name install и --name
// client add.
func daemonClientName() string { return strings.TrimSuffix(daemonClientNamePrefix, "-") }

// runDaemonCommandElevated — TODO(SPEC 141 фаза 2): cmd через
// platform.RunElevated (runas, ElevatedShowHidden, рабочий каталог — каталог
// бинаря), Wait(daemonRunWaitTimeout), Close. Ошибки:
// platform.ErrElevationCancelled — отказ UAC; errDaemonCommandStillRunning —
// таймаут; прочие — ShellExecuteExW. exitCode — код выхода процесса.
func runDaemonCommandElevated(cmd DaemonCommand) (exitCode int, err error) {
	_ = cmd
	return 0, errDaemonWindowsPending
}

// DaemonInstallOrUpdate — TODO(SPEC 141 фаза 2): гейт версии → install
// `lxd --service=install --invite-out <Data>\bin\daemon\invite-<rnd>.txt
// --invite-name <клиент>` ядром лаунчера под runas → код 0: чтение файла,
// PairDaemonWithInvite, удаление файла, warnings сайдкара; код 1:
// `--service=status` без прав → ≠ 0 — вердикт; 0 и файла нет — NoInvite с
// FreshInvite; иной код — строка «failed» с Command для Copy. Затем пересчёт
// классификатора (Service).
func (ac *AppController) DaemonInstallOrUpdate() DaemonRunResult {
	return daemonOpPending(DaemonOpInstall)
}

// DaemonStartService — TODO(SPEC 141 фаза 2): `sc.exe start sing-box-lxd`
// под runas (NotRunning), затем пересчёт классификатора.
func (ac *AppController) DaemonStartService() DaemonRunResult {
	return daemonOpPending(DaemonOpStart)
}

// DaemonFreshInvite — TODO(SPEC 141 фаза 2): `lxd client add --name
// <клиент> --invite-out <файл>` (копия, если CopyUsable, иначе ядро
// лаунчера) под runas → код 0: сопряжение по файлу, как у install.
func (ac *AppController) DaemonFreshInvite() DaemonRunResult {
	return daemonOpPending(DaemonOpFreshInvite)
}

// DaemonUninstallService — TODO(SPEC 141 фаза 2): `lxd --service=uninstall
// [--keep-copy] [--purge]` (копия, если CopyUsable, иначе ядро лаунчера)
// под runas; keepCopy=false, purge=true — «Remove all data…». Успех —
// снять свой системный прокси.
func (ac *AppController) DaemonUninstallService(keepCopy, purge bool) DaemonRunResult {
	_, _ = keepCopy, purge
	return daemonOpPending(DaemonOpUninstall)
}

// DaemonCopyOnly — TODO(SPEC 141 фаза 2): гейт версии → `lxd
// --service=copy` ядром лаунчера под runas (classic с правами, службы нет),
// warnings сайдкара.
func (ac *AppController) DaemonCopyOnly() DaemonRunResult {
	return daemonOpPending(DaemonOpCopy)
}

// daemonOpPending — итог заглушки: ошибка запуска без кода выхода.
func daemonOpPending(op DaemonServiceOp) DaemonRunResult {
	return DaemonRunResult{Op: op, Err: errDaemonWindowsPending}
}
