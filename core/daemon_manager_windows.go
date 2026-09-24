//go:build windows && !386

package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Служба демона на Windows (SPEC 141 §5), платформенная часть менеджера:
// имя службы SCM, рендер команды для показа и Copy (PowerShell), исполнение
// команды через runas (platform.RunElevated, окно UAC) с ожиданием кода
// выхода, операции Install / Start / Fresh invite / Uninstall / Copy с
// автосопряжением по файлу --invite-out. Общее — daemon_manager.go.

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
	// daemonClientNameMax — норма ядра: до 64 символов.
	daemonClientNameMax = 64
	// daemonStatusTimeout — `lxd --service=status` без прав (§5.3).
	daemonStatusTimeout = 30 * time.Second
	// daemonStartSettle — сколько ждать RUNNING после `sc.exe start`.
	daemonStartSettle = 10 * time.Second
	// scErrorServiceAlreadyRunning — код выхода sc.exe start для уже
	// работающей службы (ERROR_SERVICE_ALREADY_RUNNING).
	scErrorServiceAlreadyRunning = 1056
)

// DaemonOpsElevated — на Windows операции службы исполняет лаунчер через
// runas (одно окно UAC), ждёт кода выхода и возвращает итог в
// DaemonRunResult. Кнопка — «Run as administrator» (NotRunning — «Start
// the service»).
const DaemonOpsElevated = true

// daemonFallbackStateDir — <ProgramData>\sing-box-lxd\state (DefaultServiceStateDir
// ядра), если /admin/info недоступен.
func daemonFallbackStateDir() string {
	return filepath.Join(platform.PrivilegedDataDir(), "state")
}

// daemonEngineAvailable — daemon-движок на Windows x64/arm64 доступен:
// умеет ли его ядро лаунчера, решают CoreSupportsLxd и гейт версии.
func daemonEngineAvailable() error { return nil }

// daemonServiceCommand — рендер команды для показа и Copy (SPEC 141 §5.1):
// PowerShell `& '<путь>' <args>`; одинарная кавычка внутри удваивается. Аргументы
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

// daemonSystemDir — %SystemRoot%\System32: sc.exe и нейтральный рабочий
// каталог команд под runas (uninstall удаляет каталог копии — cwd в нём
// помешал бы).
func daemonSystemDir() string {
	dir, err := windows.GetSystemDirectory()
	if err != nil || dir == "" {
		dir = `C:\Windows\System32`
	}
	return dir
}

// daemonScExe — %SystemRoot%\System32\sc.exe.
func daemonScExe() string { return filepath.Join(daemonSystemDir(), "sc.exe") }

// daemonBootstrapCommand — NotRunning: `sc.exe start sing-box-lxd` под runas
// (START у Authenticated Users нет, SPEC 141 §3 п. 1a, §13 п. 4).
func daemonBootstrapCommand() string {
	return daemonServiceCommand(daemonScExe(), "start", daemonServiceName)
}

// DaemonKickstartCommand — на Windows команды нет (SPEC 141 §5.1): install и
// copy сами перезапускают службу.
func (ac *AppController) DaemonKickstartCommand() string { return "" }

// DaemonShowSecretCommand — чтение "secret" из daemon.json службы
// (<ProgramData>\sing-box-lxd\state, SYSTEM + Administrators): команда для
// PowerShell от имени администратора, только Copy.
func (ac *AppController) DaemonShowSecretCommand() string {
	return "Select-String -Path " + powerShellQuote(filepath.Join(daemonFallbackStateDir(), "daemon.json")) + ` -Pattern '"secret"'`
}

// daemonClientName — singbox-launcher-<user>: <user> — SAM account name в
// нижнем регистре, символы вне [a-z0-9_-] → _, всё имя не длиннее 64
// (§5.3). Уходит в --invite-name install и --name client add.
func daemonClientName() string {
	name := ""
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	if i := strings.LastIndex(name, `\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	norm := b.String()
	if norm == "" {
		norm = "user"
	}
	full := daemonClientNamePrefix + norm
	if len(full) > daemonClientNameMax {
		full = full[:daemonClientNameMax]
	}
	return full
}

// daemonInstallArgs — «Install or update service» для показа и Copy: имя
// клиента на пользователя (§5.3). Лаунчер сам добавляет --invite-out.
func daemonInstallArgs() []string {
	return []string{"lxd", "--service=install", "--invite-name", daemonClientName()}
}

// runDaemonCommandElevated — cmd через platform.RunElevated (runas, окно
// UAC, SW_HIDE, рабочий каталог — System32), ожидание выхода до
// daemonRunWaitTimeout. Ошибки: platform.ErrElevationCancelled — отказ
// UAC; errDaemonCommandStillRunning — таймаут (процесс не трогается);
// прочие — ShellExecuteExW или ожидание.
func runDaemonCommandElevated(cmd DaemonCommand) (exitCode int, err error) {
	debuglog.InfoLog("daemon service: running as administrator: %s", cmd.String())
	p, err := platform.RunElevated(cmd.Binary, cmd.Args, daemonSystemDir(), platform.ElevatedShowHidden)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := p.Close(); cerr != nil {
			debuglog.DebugLog("daemon service: close process handle: %v", cerr)
		}
	}()
	code, exited, err := p.Wait(daemonRunWaitTimeout)
	if err != nil {
		return 0, err
	}
	if !exited {
		return 0, errDaemonCommandStillRunning
	}
	debuglog.InfoLog("daemon service: %s exited with code %d", filepath.Base(cmd.Binary), code)
	return int(code), nil
}

// applyElevatedOutcome раскладывает итог runDaemonCommandElevated по полям
// результата; true — процесс завершился (Exited).
func applyElevatedOutcome(r *DaemonRunResult, code int, err error) bool {
	switch {
	case errors.Is(err, platform.ErrElevationCancelled):
		debuglog.InfoLog("daemon service: %s: the UAC prompt was cancelled", r.Op)
		r.Cancelled = true
	case errors.Is(err, errDaemonCommandStillRunning):
		debuglog.WarnLog("daemon service: %s: the administrator command is still running after %s", r.Op, daemonRunWaitTimeout)
		r.TimedOut = true
	case err != nil:
		debuglog.WarnLog("daemon service: %s: %v", r.Op, err)
		r.Err = err
	default:
		r.Exited = true
		r.ExitCode = code
	}
	return r.Exited
}

// daemonInvitePath — новый файл приглашения <Data>\bin\daemon\invite-<hex>.txt
// (ядро создаёт его сам, O_EXCL, без следования ссылкам).
func (ac *AppController) daemonInvitePath() (string, error) {
	dir := DaemonIdentityDir(ac.FileService.Layout.Data)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	return filepath.Join(dir, "invite-"+hex.EncodeToString(rnd[:])+".txt"), nil
}

// pairFromInviteFile — сопряжение по файлу --invite-out; приглашение в лог
// не пишется.
func (ac *AppController) pairFromInviteFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read the invite file: %w", err)
	}
	invite := strings.TrimSpace(string(data))
	if invite == "" {
		return errors.New("the invite file is empty")
	}
	secret := locale.LoadSettings(ac.FileService.Layout.Data.Bin()).DaemonSecret
	return ac.PairDaemonWithInvite(invite, secret)
}

// removeInviteFile — файл приглашения удаляется в любом исходе.
func removeInviteFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		debuglog.WarnLog("daemon service: remove the invite file: %v", err)
	}
}

// addSidecarWarnings — warnings сайдкара после install/copy: в результат и
// в лог (SPEC 141 §5.3).
func addSidecarWarnings(r *DaemonRunResult) {
	r.Warnings = readDaemonServiceSidecarWarnings(daemonServiceCorePath())
	for _, w := range r.Warnings {
		debuglog.WarnLog("daemon service: %s: %s", w.Code, w.Text)
	}
}

// daemonServiceStatusCode — `lxd --service=status` ядром binary без прав
// (SPEC 141 §3 п. 6): 0 OK, 2 MISMATCH/UNSAFE, 3 NOT INSTALLED, 4 COPY
// ONLY, 5 NOT RUNNING, 1 ошибка.
func daemonServiceStatusCode(binary string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), daemonStatusTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "lxd", "--service=status")
	platform.PrepareCommand(cmd)
	out, err := cmd.CombinedOutput()
	debuglog.DebugLog("daemon service: --service=status:\n%s", strings.TrimSpace(string(out)))
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// DaemonInstallOrUpdate — «Install or update the service» (SPEC 141 §5):
// гейт версии → `lxd --service=install --invite-out <файл> --invite-name
// <клиент>` ядром лаунчера под runas (одно окно UAC) → код 0: сопряжение
// по файлу, warnings сайдкара; код 1: `--service=status` без прав → ≠ 0 —
// вердикт; 0 и файла нет — NoInvite с командой FreshInvite. Затем пересчёт
// классификатора.
func (ac *AppController) DaemonInstallOrUpdate() DaemonRunResult {
	r := DaemonRunResult{Op: DaemonOpInstall}
	version := ac.launcherCoreVersion()
	if err := serviceCoreGate(version); err != nil {
		r.CoreHint = DaemonServiceCoreHint(version)
		debuglog.WarnLog("daemon service: install: %v", err)
		return r
	}
	core := ac.FileService.SingboxPath
	r.Command = DaemonCommand{Binary: core, Args: daemonInstallArgs()}
	invite, err := ac.daemonInvitePath()
	if err != nil {
		r.Err = err
		return r
	}
	defer removeInviteFile(invite)
	run := DaemonCommand{Binary: core, Args: []string{"lxd", "--service=install", "--invite-out", invite, "--invite-name", daemonClientName()}}
	code, err := runDaemonCommandElevated(run)
	if applyElevatedOutcome(&r, code, err) {
		addSidecarWarnings(&r)
		switch r.ExitCode {
		case 0:
			r.Err = ac.pairFromInviteFile(invite)
			r.Paired = r.Err == nil
		case 1:
			ac.checkInstallWithoutInvite(&r, core, invite)
		}
	}
	r.Service = ac.daemonServiceCheck(nil, "")
	return r
}

// checkInstallWithoutInvite — install вышел с кодом 1 (SPEC 141 §5.3):
// status ядром лаунчера без прав; 0, служба есть у SCM и файла нет —
// приглашение не получено, следующий шаг — «fresh invite».
func (ac *AppController) checkInstallWithoutInvite(r *DaemonRunResult, core, invite string) {
	status, err := daemonServiceStatusCode(core)
	if err != nil {
		debuglog.WarnLog("daemon service: install exited 1; --service=status: %v", err)
		return
	}
	r.StatusChecked, r.StatusCode = true, status
	if status != 0 || !daemonServiceDefined(systemDaemonServiceLayout()) {
		debuglog.WarnLog("daemon service: install exited 1; --service=status exited %d", status)
		return
	}
	if _, err := os.Stat(invite); err == nil {
		return
	}
	debuglog.WarnLog("daemon service: installed, but no invite was received; pair with a fresh invite")
	r.NoInvite = true
	r.FreshInvite = ac.freshInviteCommand()
}

// freshInviteCommand — `lxd client add --name <клиент>` для показа и Copy
// (без --invite-out: в консоли администратора приглашение печатается).
func (ac *AppController) freshInviteCommand() DaemonCommand {
	binary := daemonServiceBinaryFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath)
	return DaemonCommand{Binary: binary, Args: []string{"lxd", "client", "add", "--name", daemonClientName()}}
}

// DaemonStartService — `sc.exe start sing-box-lxd` под runas (NotRunning);
// «уже работает» (1056) — успех. Затем короткое ожидание RUNNING и
// пересчёт классификатора.
func (ac *AppController) DaemonStartService() DaemonRunResult {
	r := DaemonRunResult{Op: DaemonOpStart, Command: DaemonCommand{Binary: daemonScExe(), Args: []string{"start", daemonServiceName}}}
	code, err := runDaemonCommandElevated(r.Command)
	if applyElevatedOutcome(&r, code, err) {
		if r.ExitCode == scErrorServiceAlreadyRunning {
			r.ExitCode = 0
		}
		if r.ExitCode == 0 {
			waitDaemonServiceRunning(daemonStartSettle)
		}
	}
	r.Service = ac.daemonServiceCheck(nil, "")
	return r
}

// waitDaemonServiceRunning ждёт SERVICE_RUNNING не дольше timeout.
func waitDaemonServiceRunning(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := platform.QueryService(daemonServiceName); err == nil && info.StatusErr == nil && platform.ServiceRunning(info.State) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// DaemonFreshInvite — `lxd client add --name <клиент> --invite-out <файл>`
// (копия, если CopyUsable, иначе ядро лаунчера — тогда гейт версии) под
// runas → код 0: сопряжение по файлу, как у install.
func (ac *AppController) DaemonFreshInvite() DaemonRunResult {
	r := DaemonRunResult{Op: DaemonOpFreshInvite, Command: ac.freshInviteCommand()}
	if r.Command.Binary == ac.FileService.SingboxPath {
		version := ac.launcherCoreVersion()
		if err := serviceCoreGate(version); err != nil {
			debuglog.WarnLog("daemon service: fresh invite: %v", err)
			return DaemonRunResult{Op: DaemonOpFreshInvite, CoreHint: DaemonServiceCoreHint(version)}
		}
	}
	invite, err := ac.daemonInvitePath()
	if err != nil {
		r.Err = err
		return r
	}
	defer removeInviteFile(invite)
	run := DaemonCommand{Binary: r.Command.Binary, Args: append(append([]string{}, r.Command.Args...), "--invite-out", invite)}
	code, err := runDaemonCommandElevated(run)
	if applyElevatedOutcome(&r, code, err) && r.ExitCode == 0 {
		r.Err = ac.pairFromInviteFile(invite)
		r.Paired = r.Err == nil
	}
	r.Service = ac.daemonServiceCheck(nil, "")
	return r
}

// DaemonUninstallService — `lxd --service=uninstall [--keep-copy]
// [--purge]` (копия, если CopyUsable, иначе ядро лаунчера) под runas;
// keepCopy=false, purge=true — «Remove all data…». Успех — снять свой
// системный прокси.
func (ac *AppController) DaemonUninstallService(keepCopy, purge bool) DaemonRunResult {
	binary := daemonServiceBinaryFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath)
	args := []string{"lxd", "--service=uninstall"}
	if keepCopy {
		args = append(args, "--keep-copy")
	}
	if purge {
		args = append(args, "--purge")
	}
	r := DaemonRunResult{Op: DaemonOpUninstall, Command: DaemonCommand{Binary: binary, Args: args}}
	code, err := runDaemonCommandElevated(r.Command)
	if applyElevatedOutcome(&r, code, err) && r.ExitCode == 0 {
		ac.clearDaemonSystemProxy("the service was uninstalled")
	}
	r.Service = ac.daemonServiceCheck(nil, "")
	return r
}

// DaemonCopyOnly — гейт версии → `lxd --service=copy` ядром лаунчера под
// runas (classic с правами, службы нет, SPEC 141 §8), warnings сайдкара.
func (ac *AppController) DaemonCopyOnly() DaemonRunResult {
	r := DaemonRunResult{Op: DaemonOpCopy}
	version := ac.launcherCoreVersion()
	if err := serviceCoreGate(version); err != nil {
		r.CoreHint = DaemonServiceCoreHint(version)
		debuglog.WarnLog("daemon service: copy: %v", err)
		return r
	}
	r.Command = DaemonCommand{Binary: ac.FileService.SingboxPath, Args: []string{"lxd", "--service=copy"}}
	code, err := runDaemonCommandElevated(r.Command)
	if applyElevatedOutcome(&r, code, err) {
		addSidecarWarnings(&r)
	}
	r.Service = ac.daemonServiceCheck(nil, "")
	return r
}
