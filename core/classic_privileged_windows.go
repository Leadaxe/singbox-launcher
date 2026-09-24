//go:build windows && !386

package core

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
	privilegedCopyMissingWinText     = "With administrator rights the launcher starts the sing-box core only from a protected copy in Program Files that your user account cannot modify, and there is no such copy yet."
	privilegedCopyOutdatedWinText    = "With administrator rights the launcher starts the sing-box core from a protected copy in Program Files, and the copy does not match the launcher's current core (%s) — for example after a core update."
	privilegedCopyUnsafeWinText      = "With administrator rights the launcher starts the sing-box core only from a protected copy, and the copy's location failed the protection check:\n%s\nIf the command below reports the same problem, fix the permissions of that path."
	privilegedCopyServiceNoteWinText = "The daemon service is installed on this computer, so the command is its Install or update command: it refreshes the same copy and restarts the service."
	privilegedCopyInstructionWinText = "Click Run as administrator (or run this command in PowerShell as administrator), then click Retry:"
	daemonCoreUpdatedWinText         = "The daemon service still runs the previous core. Install the new core into the service and restart it (Run as administrator):"
	daemonUnsafeNoticeWinText        = "The daemon service runs with SYSTEM rights from a file that is not protected:\n%s\n\nAny program running as you could replace it and take over this computer. Install or update the service to move it to a protected copy of the core (Run as administrator). The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
	daemonUnsafeNoticeCoreWinText    = "The daemon service runs with SYSTEM rights from a file that is not protected:\n%s\n\nAny program running as you could replace it and take over this computer. %s The VPN keeps working meanwhile; the LOCAL tab of the connection settings shows this until it is fixed."
)

// classicElevatedUsesCopy — повышенный classic исполняет только защищённую
// копию (SPEC 141 §8): гейт по токену, а не по TUN.
func classicElevatedUsesCopy() bool { return platform.IsElevated() }

// elevatedClassicStart — гейт копии и classic.log перед повышенным стартом
// classic. Отказ гейта уже показан диалогом (errPrivilegedCopyNotReady);
// каталога logs\ нет — тот же диалог «missing»: его создают install и copy.
func (ac *AppController) elevatedClassicStart() (string, *os.File, error) {
	corePath, err := ac.privilegedCoreCopyGate()
	if err != nil {
		return "", nil, err
	}
	logFile, err := platform.OpenPrivilegedCoreLog()
	if errors.Is(err, platform.ErrPrivilegedLogDirMissing) {
		version := ac.launcherCoreVersion()
		command, viaService, cmdErr := privilegedCopyCommandFor(systemDaemonServiceLayout(), ac.FileService.SingboxPath, version)
		coreHint := ""
		if cmdErr != nil {
			coreHint = DaemonServiceCoreHint(version)
		}
		debuglog.WarnLog("startSingBox: privileged start refused: %v", err)
		ac.showPrivilegedCopyDialog(privilegedCopyCheck{State: privilegedCopyMissing, CorePath: corePath, Detail: err.Error()},
			command, viaService, coreHint)
		return "", nil, errPrivilegedCopyNotReady
	}
	if err != nil {
		return "", nil, fmt.Errorf("core log %s: %w", platform.PrivilegedCoreLogPath(), err)
	}
	debuglog.InfoLog("startSingBox: elevated start from the protected copy %s, log %s", corePath, logFile.Name())
	return corePath, logFile, nil
}

// showPrivilegedCopyDialog — отказ гейта (SPEC 141 §8): причина, команда,
// Run as administrator / Copy the command / Retry / Close. Нет команды
// (ядро лаунчера копию не умеет) — вместо неё coreHint, кнопка Close.
func (ac *AppController) showPrivilegedCopyDialog(c privilegedCopyCheck, command string, viaService bool, coreHint string) {
	if !ac.hasUI() {
		return
	}
	var title, reason string
	switch c.State {
	case privilegedCopyMissing:
		title = locale.T("Core copy for privileged start is missing")
		reason = locale.T(privilegedCopyMissingWinText)
	case privilegedCopyOutdated:
		title = locale.T("Core copy for privileged start is outdated")
		what := c.Detail
		if c.CopySHA256 != c.LauncherSHA256 {
			what = shortSHA(c.CopySHA256) + " ≠ " + shortSHA(c.LauncherSHA256)
		}
		reason = locale.Tf(privilegedCopyOutdatedWinText, what)
	default:
		title = locale.T("Core copy for privileged start is not protected")
		reason = locale.Tf(privilegedCopyUnsafeWinText, c.Detail)
	}
	parts := []string{reason}
	if command == "" {
		parts = append(parts, coreHint)
		dialogs.ShowActions(ac.UIService.MainWindow, title, strings.Join(parts, "\n\n"), nil, locale.T("Close"))
		return
	}
	if viaService {
		parts = append(parts, locale.T(privilegedCopyServiceNoteWinText))
	}
	parts = append(parts, locale.T(privilegedCopyInstructionWinText), command)
	op := ac.DaemonCopyOnly
	if viaService {
		op = ac.DaemonInstallOrUpdate
	}
	actions := []dialogs.Action{
		ac.daemonOpAction(locale.T("Run as administrator"), op, nil),
		copyCommandAction(command),
		{Label: locale.T("Retry"), Run: func(d *dialogs.ActionsDialog) {
			d.Hide()
			go StartSingBoxProcess()
		}},
	}
	dialogs.ShowActions(ac.UIService.MainWindow, title, strings.Join(parts, "\n\n"), actions, locale.T("Close"))
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
