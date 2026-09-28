//go:build windows && !386

package core

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Classic под правами администратора и диалоги службы на Windows (SPEC 141
// §8, §9, §10): повышенный лаунчер исполняет только защищённую копию
// <ProgramFiles>\sing-box-lxd\sing-box-lxd.exe (гейт по токену при любом
// конфиге, §13 п. 1), вывод — в classic.log; отказ гейта, «Core updated»,
// модальное Unsafe и «Install service» в диалоге «TUN без прав» — одна
// кнопка «Run as administrator» над операциями менеджера (runas, UAC).

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	privilegedCopyMissingWinText     = "With administrator rights the launcher starts the sing-box core from a protected copy in Program Files that your user account cannot modify, and there is no such copy yet."
	privilegedCopyOutdatedWinText    = "With administrator rights the launcher starts the sing-box core from a protected copy in Program Files, and the copy does not match the launcher's current core (%s) — for example after a core update."
	privilegedCopyUnsafeWinText      = "With administrator rights the launcher starts the sing-box core from a protected copy, and the copy's location failed the protection check:\n%s"
	privilegedCopyServiceNoteWinText = "The daemon service is installed on this computer, so the command is its Install or update command: it refreshes the same copy and restarts the service."
	privilegedCopyInstructionWinText = "Click Run as administrator (or run this command in PowerShell as administrator), then click Retry:"
	privilegedSameProblemWinText     = "If the command below reports the same problem, fix the permissions of that path."
	privilegedFixInstructionWinText  = "Run this command in PowerShell as administrator, then click Retry:"
	privilegedCopyRiskWinText        = "Run anyway starts the launcher's own core with administrator rights: any program running under your account could replace that file and get administrator rights on the next start. The choice lasts until the launcher is closed."
	privilegedLogMissingWinText      = "With administrator rights the core writes its log to a protected folder, and the folder is missing:\n%s"
	privilegedLogUnsafeWinText       = "With administrator rights the core writes its log to a protected folder, and the folder failed the protection check:\n%s"
	privilegedLogRiskWinText         = "Any program running under your account could replace the log folder and get administrator rights on the next start. Run anyway writes the core output to the launcher's own log instead; the choice lasts until the launcher is closed."
	daemonCoreUpdatedWinText         = "The daemon service still runs the previous core. Install the new core into the service and restart it (Run as administrator):"
	daemonUnsafeNoticeWinText        = "The daemon service runs with SYSTEM rights from a file that is not protected:\n%s\n\nAny program running as you could replace it and take over this computer. Install or update the service to move it to a protected copy of the core (Run as administrator). The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
	daemonUnsafeNoticeCoreWinText    = "The daemon service runs with SYSTEM rights from a file that is not protected:\n%s\n\nAny program running as you could replace it and take over this computer. %s The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
)

// classicElevatedUsesCopy — повышенный classic исполняет только защищённую
// копию (SPEC 141 §8): гейт по токену, а не по TUN.
func classicElevatedUsesCopy() bool { return platform.IsElevated() }

// privilegedCopyRunAnyway — «Run anyway» по копии уже выбран в этой
// сессии (SPEC 150): гейт стартует ядро лаунчера с WARN вместо диалога.
func (ac *AppController) privilegedCopyRunAnyway() bool {
	return ac.ProcessService != nil && ac.ProcessService.runAnywayCopy.Load()
}

// elevatedClassicStart — гейт копии и classic.log перед повышенным стартом
// classic. Отказ гейта уже показан диалогом (errPrivilegedCopyNotReady).
// classic.log не открылся (нет logs\, нарушена защита) — свой диалог с
// «Run anyway» (SPEC 150); после него — nil-файл: Start пишет вывод ядра в
// лог лаунчера. Проверки копии и лога деградируют независимо.
func (ac *AppController) elevatedClassicStart() (string, *os.File, error) {
	corePath, err := ac.privilegedCoreCopyGate()
	if err != nil {
		return "", nil, err
	}
	logFile, err := platform.OpenPrivilegedCoreLog()
	if err != nil {
		if ac.ProcessService != nil && ac.ProcessService.runAnywayLog.Load() {
			debuglog.WarnLog("startSingBox: core log %s: %v; Run anyway was chosen in this session, the core writes to the launcher log",
				platform.PrivilegedCoreLogPath(), err)
			debuglog.InfoLog("startSingBox: elevated start, core %s, launcher log", corePath)
			return corePath, nil, nil
		}
		debuglog.WarnLog("startSingBox: privileged start refused, core log %s: %v", platform.PrivilegedCoreLogPath(), err)
		if errors.Is(err, platform.ErrPrivilegedLogDirMissing) || ac.hasUI() {
			ac.showPrivilegedLogDialog(err)
			return "", nil, errPrivilegedCopyNotReady
		}
		return "", nil, fmt.Errorf("core log %s: %w", platform.PrivilegedCoreLogPath(), err)
	}
	debuglog.InfoLog("startSingBox: elevated start, core %s, log %s", corePath, logFile.Name())
	return corePath, logFile, nil
}

// elevatedRefusal — содержимое диалога отказа повышенного старта.
type elevatedRefusal struct {
	title, reason, risk string
	// command — copy/install (SPEC 141 §8); "" — ядро лаунчера копию не
	// умеет, вместо команды coreHint.
	command    string
	viaService bool
	coreHint   string
	// protection — нарушение защиты звена (SPEC 150): у предка с командой
	// icacls диалог показывает её вместо copy/install.
	protection *platform.ProtectionError
	// runAnyway — флаг сессии, который ставит «Run anyway».
	runAnyway func() *atomic.Bool
}

// showPrivilegedCopyDialog — отказ гейта копии (SPEC 141 §8, SPEC 150).
func (ac *AppController) showPrivilegedCopyDialog(c privilegedCopyCheck, command string, viaService bool, coreHint string) {
	if !ac.hasUI() {
		return
	}
	r := elevatedRefusal{risk: locale.T(privilegedCopyRiskWinText), command: command, viaService: viaService, coreHint: coreHint,
		runAnyway: func() *atomic.Bool { return &ac.ProcessService.runAnywayCopy }}
	switch c.State {
	case privilegedCopyMissing:
		r.title = locale.T("Core copy for privileged start is missing")
		r.reason = locale.T(privilegedCopyMissingWinText)
	case privilegedCopyOutdated:
		r.title = locale.T("Core copy for privileged start is outdated")
		what := c.Detail
		if c.CopySHA256 != c.LauncherSHA256 {
			what = shortSHA(c.CopySHA256) + " ≠ " + shortSHA(c.LauncherSHA256)
		}
		r.reason = locale.Tf(privilegedCopyOutdatedWinText, what)
	default:
		r.title = locale.T("Core copy for privileged start is not protected")
		r.reason = locale.Tf(privilegedCopyUnsafeWinText, c.Detail)
		errors.As(c.Err, &r.protection)
	}
	ac.showElevatedRefusalDialog(r)
}

// showPrivilegedLogDialog — classic.log не открылся (SPEC 150): нет
// каталога logs\ (его создают install и copy) или нарушена защита.
func (ac *AppController) showPrivilegedLogDialog(logErr error) {
	if !ac.hasUI() {
		return
	}
	version := ac.launcherCoreVersion()
	command, viaService, cmdErr := privilegedCopyCommandFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath, version)
	r := elevatedRefusal{risk: locale.T(privilegedLogRiskWinText), command: command, viaService: viaService,
		runAnyway: func() *atomic.Bool { return &ac.ProcessService.runAnywayLog }}
	if cmdErr != nil {
		r.coreHint = DaemonServiceCoreHint(version)
	}
	if errors.Is(logErr, platform.ErrPrivilegedLogDirMissing) {
		r.title = locale.T("Core log folder is missing")
		r.reason = locale.Tf(privilegedLogMissingWinText, logErr.Error())
	} else {
		r.title = locale.T("Core log folder is not protected")
		r.reason = locale.Tf(privilegedLogUnsafeWinText, logErr.Error())
		errors.As(logErr, &r.protection)
	}
	ac.showElevatedRefusalDialog(r)
}

// showElevatedRefusalDialog — причина, риск, команда исправления; кнопки
// Run as administrator (copy/install) / Copy the command / Retry / Run
// anyway, слева Close. Нарушение на предке с командой icacls — она вместо
// copy/install, без Run as administrator. «Run anyway» ставит флаг сессии
// и повторяет старт тем же путём, что Retry.
func (ac *AppController) showElevatedRefusalDialog(r elevatedRefusal) {
	parts := []string{r.reason, r.risk}
	retry := dialogs.Action{Label: locale.T("Retry"), Run: func(d *dialogs.ActionsDialog) {
		d.Hide()
		go StartSingBoxProcess()
	}}
	var actions []dialogs.Action
	switch {
	case r.protection != nil && r.protection.Ancestor && r.protection.Fix != "":
		parts = append(parts, locale.T(privilegedFixInstructionWinText), r.protection.Fix)
		actions = append(actions, copyCommandAction(r.protection.Fix), retry)
	case r.command != "":
		if r.viaService {
			parts = append(parts, locale.T(privilegedCopyServiceNoteWinText))
		}
		if r.protection != nil {
			parts = append(parts, locale.T(privilegedSameProblemWinText))
		}
		parts = append(parts, locale.T(privilegedCopyInstructionWinText), r.command)
		op := ac.DaemonCopyOnly
		if r.viaService {
			op = ac.DaemonInstallOrUpdate
		}
		actions = append(actions, ac.daemonOpAction(locale.T("Run as administrator"), op, nil), copyCommandAction(r.command), retry)
	default:
		parts = append(parts, r.coreHint)
	}
	if ac.ProcessService != nil {
		actions = append(actions, dialogs.Action{Label: locale.T("Run anyway"), Run: func(d *dialogs.ActionsDialog) {
			r.runAnyway().Store(true)
			debuglog.WarnLog("startSingBox: Run anyway chosen (%s), kept until the launcher is closed", r.title)
			d.Hide()
			go StartSingBoxProcess()
		}})
	}
	dialogs.ShowActions(ac.UIService.MainWindow, r.title, strings.Join(parts, "\n\n"), actions, locale.T("Close"))
}

// daemonOpAction — кнопка операции службы под runas: строка ожидания при
// выключенных кнопках, операция в горутине (UAC не блокирует UI), затем
// строка итога (StatusText) с командой для консоли администратора при
// ошибке. Install без приглашения (NoInvite) — следующее нажатие делает
// «fresh invite» (второе окно UAC, SPEC 141 §5.3). onSuccess — вместо
// строки итога при успехе (из горутины).
func (ac *AppController) daemonOpAction(label string, op func() DaemonRunResult, onSuccess func(d *dialogs.ActionsDialog, r DaemonRunResult)) dialogs.Action {
	next := op
	return dialogs.Action{
		Label:     label,
		Important: true,
		Run: func(d *dialogs.ActionsDialog) {
			d.SetBusyStatus(DaemonRunWaitingText())
			run := next
			go func() {
				r := run()
				switch {
				case r.NoInvite:
					next = ac.DaemonFreshInvite
				case r.Succeeded():
					next = op
				}
				if r.Succeeded() && onSuccess != nil {
					onSuccess(d, r)
					return
				}
				d.SetStatus(daemonRunStatusLine(r))
			}()
		},
	}
}

// daemonRunStatusLine — StatusText и, где нужна консоль администратора,
// команда (без --invite-out) отдельной строкой.
func daemonRunStatusLine(r DaemonRunResult) string {
	line := r.StatusText()
	switch {
	case r.NoInvite && !r.FreshInvite.IsZero():
		line += "\n" + r.FreshInvite.String()
	case !r.Cancelled && !r.TimedOut && ((r.Exited && r.ExitCode != 0) || (!r.Exited && r.Err != nil)) && !r.Command.IsZero():
		line += "\n" + r.Command.String()
	}
	return line
}

// copyCommandAction — «Copy the command»: команда в буфер обмена.
func copyCommandAction(command string) dialogs.Action {
	return dialogs.Action{Label: locale.T("Copy the command"), Run: func(_ *dialogs.ActionsDialog) {
		if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
			app.Clipboard().SetContent(command)
		}
	}}
}

// tunInstallServiceAction — «Install service» в диалоге «TUN без прав»
// (SPEC 139 §4, SPEC 141 §9): install (одно окно UAC) → сопряжение по
// приглашению → движок daemon (сохраняется в settings.json) → Start.
func (ac *AppController) tunInstallServiceAction() (dialogs.Action, bool) {
	return ac.daemonOpAction(locale.T("Install service"), ac.DaemonInstallOrUpdate, func(d *dialogs.ActionsDialog, _ DaemonRunResult) {
		if err := ac.switchToDaemonEngine(); err != nil {
			debuglog.WarnLog("install service: switch to daemon mode: %v", err)
			d.SetStatus(err.Error())
			return
		}
		d.Hide()
		debuglog.InfoLog("install service: daemon mode active, starting the VPN")
		StartSingBoxProcess()
	}), true
}

// switchToDaemonEngine — движок daemon и выбор в settings.json (как радио
// панели Local и Debug API SwitchEngine).
func (ac *AppController) switchToDaemonEngine() error {
	if err := ac.SwitchBackendMode(BackendDaemon); err != nil {
		return err
	}
	binDir := ac.FileService.Layout.Data.Bin()
	st := locale.LoadSettings(binDir)
	st.CoreBackendMode = string(BackendDaemon)
	if err := locale.SaveSettings(binDir, st); err != nil {
		return fmt.Errorf("daemon mode is on, but saving the choice failed: %w", err)
	}
	return nil
}

// showDaemonCoreUpdatedDialog — «Core updated — update the daemon service»
// (SPEC 141 §10): install под runas или Copy.
func (ac *AppController) showDaemonCoreUpdatedDialog(command string) {
	message := locale.T(daemonCoreUpdatedWinText) + "\n\n" + command
	actions := []dialogs.Action{
		ac.daemonOpAction(locale.T("Run as administrator"), ac.DaemonInstallOrUpdate, nil),
		copyCommandAction(command),
	}
	dialogs.ShowActions(ac.UIService.MainWindow, locale.T("Core updated — update the daemon service"), message, actions, locale.T("Close"))
}

// ShowDaemonUnsafeNoticeElevated — модальное предупреждение SPEC 136 §6 на
// Windows (раз на версию лаунчера): служба на незащищённом файле, кнопка
// Run as administrator (install). true — показано здесь.
func (ac *AppController) ShowDaemonUnsafeNoticeElevated(win fyne.Window, servicePath, command, coreHint string) bool {
	if command == "" {
		dialogs.ShowActions(win, locale.T("The daemon service is not protected"),
			locale.Tf(daemonUnsafeNoticeCoreWinText, servicePath, coreHint), nil, locale.T("Close"))
		return true
	}
	actions := []dialogs.Action{
		ac.daemonOpAction(locale.T("Run as administrator"), ac.DaemonInstallOrUpdate, nil),
		copyCommandAction(command),
	}
	dialogs.ShowActions(win, locale.T("The daemon service is not protected"),
		locale.Tf(daemonUnsafeNoticeWinText, servicePath)+"\n\n"+command, actions, locale.T("Close"))
	return true
}
