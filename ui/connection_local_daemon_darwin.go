//go:build darwin

package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/components"
)

// wrappedLabel — Label с переносом: длинная подпись не должна задавать
// min-width колонки (иначе вертикальный скролл распирает окно по ширине).
func wrappedLabel(key string) *widget.Label {
	label := widget.NewLabel(locale.T(key))
	label.Wrapping = fyne.TextWrapWord
	return label
}

// buildDaemonPanel — панель daemon-движка на вкладке LOCAL (macOS).
//
// Все привилегированные операции — консольные: панель показывает готовые
// sudo-команды (установка / удаление / полное удаление / пере-сопряжение /
// kickstart) с кнопками «копировать» и «открыть в терминале». Лаунчер сам
// ничего под root не запускает (AEWP выпилен). Сопряжение: команда печатает
// одноразовое приглашение — пользователь вставляет его в поле ниже.
//
// onPaired — колбэк успешного сопряжения (вкладка доводит переключение
// движка, если пользователь уже выбрал daemon).
func buildDaemonPanel(ac *core.AppController, win fyne.Window, onPaired func()) fyne.CanvasObject {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)

	hint := widget.NewLabel(locale.T("settings.daemon_hint"))
	hint.Wrapping = fyne.TextWrapWord

	status := widget.NewLabel(locale.T("settings.daemon_status_checking"))
	status.Wrapping = fyne.TextWrapWord

	// --- Статус ----------------------------------------------------------
	renderStatus := func(snap core.DaemonUIStatus) string {
		return renderDaemonStatusText(ac, snap, true)
	}
	refreshStatus := func() {
		go func() {
			snap := ac.DaemonStatusSnapshot()
			fyne.Do(func() {
				status.SetText(renderStatus(snap))
			})
		}()
	}

	// --- Консольные команды ----------------------------------------------
	// Каждая строка: подпись, команда (копируемое поле), кнопки copy/terminal.
	// Команды перечитываются при каждом действии (адрес мог смениться).
	commandRow := func(labelKey string, command func() (string, error)) fyne.CanvasObject {
		entry := widget.NewEntry()
		entry.Wrapping = fyne.TextWrapOff
		if text, err := command(); err == nil {
			entry.SetText(text)
		}
		var copyBtn *ttwidget.Button
		copyBtn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
			text, err := command()
			if err != nil {
				ShowError(win, err)
				return
			}
			entry.SetText(text)
			win.Clipboard().SetContent(text)
			// Тихий фидбек: иконка на секунду становится галочкой. Никаких
			// системных нотификаций и модалок ради копирования строки.
			copyBtn.SetIcon(theme.ConfirmIcon())
			go func() {
				time.Sleep(1200 * time.Millisecond)
				fyne.Do(func() { copyBtn.SetIcon(theme.ContentCopyIcon()) })
			}()
		})
		copyBtn.SetToolTip(locale.T("conn.cmd_copy_tooltip"))
		termBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
			text, err := command()
			if err != nil {
				ShowError(win, err)
				return
			}
			entry.SetText(text)
			if err := ac.OpenTerminalWithCommand(text); err != nil {
				ShowError(win, err)
			}
		})
		termBtn.SetToolTip(locale.T("conn.cmd_terminal_tooltip"))
		rowLabel := widget.NewLabel(locale.T(labelKey))
		rowLabel.Wrapping = fyne.TextWrapWord
		return container.NewVBox(
			rowLabel,
			container.NewBorder(nil, nil, nil, container.NewHBox(copyBtn, termBtn), entry),
		)
	}

	maintenanceCommands := container.NewVBox(
		widget.NewLabelWithStyle(locale.T("conn.cmd_section"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		commandRow("conn.cmd_kickstart", func() (string, error) { return ac.DaemonKickstartCommand(), nil }),
	)

	// --- Сопряжение по приглашению ---------------------------------------
	inviteEntry := widget.NewEntry()
	inviteEntry.SetPlaceHolder(locale.T("settings.daemon_invite_placeholder"))
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("settings.daemon_secret_placeholder"))
	{
		st := locale.LoadSettings(binDir)
		secretEntry.SetText(st.DaemonSecret)
	}
	// Enter сохраняет секрет сам по себе: для plain-h2c демона (без TLS)
	// сопряжения не существует — Pair недоступен, а Bearer-секрет — это весь
	// канал аутентификации. Для mTLS-демона поле не нужно вовсе.
	secretEntry.OnSubmitted = func(text string) {
		if err := ac.SetDaemonSecret(text); err != nil {
			ShowError(win, err)
			return
		}
		refreshStatus()
	}

	secretHelp := widget.NewButton("?", func() {
		showCommandHelpDialog(ac, win,
			locale.T("settings.daemon_secret_placeholder"),
			locale.T("conn.secret_help"),
			ac.DaemonShowSecretCommand())
	})
	secretHelp.Importance = widget.LowImportance

	pairBtn := widget.NewButton(locale.T("settings.daemon_pair_btn"), func() {
		invite := strings.TrimSpace(inviteEntry.Text)
		if invite == "" {
			ShowErrorText(win, locale.T("conn.window_title"), locale.T("settings.daemon_invite_empty"))
			return
		}
		secret := strings.TrimSpace(secretEntry.Text)
		go func() {
			err := ac.PairDaemonWithInvite(invite, secret)
			fyne.Do(func() {
				if err != nil {
					ShowError(win, err)
					return
				}
				inviteEntry.SetText("")
				dialogs.ShowAutoHideInfo(ac.UIService.Application, win,
					locale.T("conn.window_title"), locale.T("settings.daemon_pair_done"))
				if onPaired != nil {
					onPaired()
				}
				refreshStatus()
			})
		}()
	})
	pairHelp := widget.NewButton("?", func() {
		showCommandHelpDialog(ac, win,
			locale.T("settings.daemon_pair_btn"),
			locale.T("settings.daemon_pair_help"),
			ac.DaemonRepairCommand())
	})
	pairHelp.Importance = widget.LowImportance

	// --- Секция Uninstall: два последовательных шага ----------------------
	// 1) Unpair — локальная сторона (пара лаунчера, пин, адрес);
	// 2) удаление службы sudo-командой, с галкой --purge (по умолчанию ВКЛ:
	//    «снести — так снести»; выключают её осознанно, чтобы сохранить
	//    state демона для будущей переустановки без пере-сопряжения).
	unpairBtn := widget.NewButton(locale.T("settings.daemon_unpair_btn"), func() {
		ShowConfirm(win,
			locale.T("settings.daemon_unpair_confirm_title"),
			locale.T("settings.daemon_unpair_confirm_body"),
			func(ok bool) {
				if !ok {
					return
				}
				if err := ac.UnpairDaemon(); err != nil {
					ShowError(win, err)
					return
				}
				refreshStatus()
			})
	})

	purgeCheck := widget.NewCheck(locale.T("conn.uninstall_purge_check"), nil)
	purgeCheck.SetChecked(true)
	uninstallEntry := widget.NewEntry()
	uninstallEntry.Wrapping = fyne.TextWrapOff
	refreshUninstallCommand := func() {
		uninstallEntry.SetText(ac.DaemonUninstallCommand(purgeCheck.Checked))
	}
	purgeCheck.OnChanged = func(bool) { refreshUninstallCommand() }
	refreshUninstallCommand()
	var uninstallCopyBtn *ttwidget.Button
	uninstallCopyBtn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		refreshUninstallCommand()
		win.Clipboard().SetContent(uninstallEntry.Text)
		// Тихий фидбек, как у commandRow: галочка на секунду.
		uninstallCopyBtn.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(1200 * time.Millisecond)
			fyne.Do(func() { uninstallCopyBtn.SetIcon(theme.ContentCopyIcon()) })
		}()
	})
	uninstallCopyBtn.SetToolTip(locale.T("conn.cmd_copy_tooltip"))
	uninstallTermBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
		refreshUninstallCommand()
		if err := ac.OpenTerminalWithCommand(uninstallEntry.Text); err != nil {
			ShowError(win, err)
		}
	})
	uninstallTermBtn.SetToolTip(locale.T("conn.cmd_terminal_tooltip"))

	uninstallSection := container.NewVBox(
		widget.NewLabelWithStyle(locale.T("conn.uninstall_section"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		wrappedLabel("conn.uninstall_step_unpair"),
		unpairBtn,
		wrappedLabel("conn.uninstall_step_service"),
		purgeCheck,
		container.NewBorder(nil, nil, nil, container.NewHBox(uninstallCopyBtn, uninstallTermBtn), uninstallEntry),
	)

	// --- Прочее: адрес, stop-on-exit -------------------------------------
	addressEntry := widget.NewEntry()
	{
		st := locale.LoadSettings(binDir)
		addressEntry.SetText(st.DaemonAddress)
	}
	addressEntry.SetPlaceHolder("127.0.0.1:19091")
	addressEntry.OnSubmitted = func(text string) {
		if err := ac.SetDaemonAddress(text); err != nil {
			ShowError(win, err)
			return
		}
		refreshStatus()
	}

	stopOnExitCheck := widget.NewCheck(locale.T("settings.daemon_stop_on_exit_label"), nil)
	{
		st := locale.LoadSettings(binDir)
		stopOnExitCheck.SetChecked(st.DaemonStopVPNOnExit)
	}
	stopOnExitCheck.OnChanged = func(checked bool) {
		st := locale.LoadSettings(binDir)
		st.DaemonStopVPNOnExit = checked
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("conn.daemon: save stop_on_exit: %v", err)
		}
	}

	refreshBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), refreshStatus)
	refreshBtn.SetToolTip(locale.T("settings.daemon_refresh_tooltip"))

	refreshStatus()

	// --- Секция Install: два последовательных шага ------------------------
	// 1) sudo-команда установки (сама печатает приглашение в конце);
	// 2) поле приглашения + Pair. Свежее приглашение для пере-сопряжения —
	//    в Maintenance (lxd client add).
	installSection := container.NewVBox(
		widget.NewLabelWithStyle(locale.T("conn.install_section"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		commandRow("conn.install_step_cmd", ac.DaemonInstallCommand),
		wrappedLabel("conn.install_step_pair"),
		container.NewBorder(nil, nil, nil, container.NewHBox(pairBtn, pairHelp), inviteEntry),
	)

	return container.NewVBox(
		hint,
		container.NewBorder(nil, nil, nil, refreshBtn, status),
		widget.NewSeparator(),
		installSection,
		widget.NewSeparator(),
		maintenanceCommands,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, nil, secretHelp, secretEntry),
		container.NewBorder(nil, nil, widget.NewLabel(locale.T("settings.daemon_address_label")), nil, addressEntry),
		stopOnExitCheck,
		widget.NewSeparator(),
		uninstallSection,
	)
}

// renderDaemonStatusText — общий рендер статуса демона. includeLocalService
// добавляет строки про ЛОКАЛЬНОЕ окружение (поддержка lxd установленным
// ядром, наличие launchd-службы) — для удалённого демона они бессмысленны.
func renderDaemonStatusText(ac *core.AppController, snap core.DaemonUIStatus, includeLocalService bool) string {
	var b strings.Builder
	if includeLocalService {
		if snap.CoreSupportsLxd {
			b.WriteString(locale.T("settings.daemon_status_core_ok"))
		} else {
			b.WriteString(locale.T("settings.daemon_status_core_unsupported"))
		}
		b.WriteString("\n")
		if snap.ServiceInstalled {
			b.WriteString(locale.T("settings.daemon_status_service_installed"))
		} else {
			b.WriteString(locale.T("settings.daemon_status_service_missing"))
		}
		b.WriteString("\n")
	}
	if snap.Paired {
		b.WriteString(locale.Tf("settings.daemon_status_paired", snap.Address))
	} else {
		b.WriteString(locale.T("settings.daemon_status_not_paired"))
	}
	if snap.Reachable {
		b.WriteString("\n")
		b.WriteString(locale.Tf("settings.daemon_status_reachable", snap.CoreStatus))
		if snap.DaemonVersion != "" {
			// Локальная сборка ядра репортит version=unknown — «vunknown»
			// выглядит как опечатка; показываем честное «dev build».
			displayVersion := "v" + snap.DaemonVersion
			if snap.DaemonVersion == "unknown" {
				displayVersion = "dev build"
			}
			b.WriteString("\n")
			b.WriteString(locale.Tf("settings.daemon_status_daemon_line", displayVersion, snap.StateDir))
		}
		if snap.InterruptedApply {
			b.WriteString("\n")
			b.WriteString(locale.T("settings.daemon_status_interrupted_apply"))
		}
		if snap.LastError != "" {
			b.WriteString("\n")
			b.WriteString(locale.Tf("settings.daemon_status_last_error", snap.LastError))
		}
	} else if snap.Paired {
		b.WriteString("\n")
		b.WriteString(locale.T("settings.daemon_status_unreachable"))
	}
	// Движок фактически не daemon (не сопряжён / переключение не прошло):
	// подсказываем, что выбор выше — намерение, а не состояние.
	if ac.BackendMode() != core.BackendDaemon {
		b.WriteString("\n")
		b.WriteString(locale.T("conn.daemon_engine_inactive"))
	}
	return b.String()
}

// activateDaemonEngineIfPossible включает daemon-движок после успешного
// сопряжения (общий хвост Pair для local- и remote-панелей). Молча
// пропускает, если VPN работает или сопряжение неполно — статус объяснит.
func activateDaemonEngineIfPossible(ac *core.AppController) {
	if ac.BackendMode() == core.BackendDaemon || ac.RunningState.IsRunning() {
		return
	}
	if err := ac.SwitchBackendMode(core.BackendDaemon); err != nil {
		debuglog.InfoLog("conn: daemon engine not active yet: %v", err)
		return
	}
	binDir := platform.GetBinDir(ac.FileService.ExecDir)
	st := locale.LoadSettings(binDir)
	st.CoreBackendMode = string(core.BackendDaemon)
	if err := locale.SaveSettings(binDir, st); err != nil {
		debuglog.WarnLog("conn: save core_backend_mode: %v", err)
	}
}

// connectionScopeIsRemote — текущее подключение указывает на удалённый демон
// (адрес задан и не loopback): окно подключения открывается в REMOTE-виде.
func connectionScopeIsRemote(ac *core.AppController) bool {
	st := locale.LoadSettings(platform.GetBinDir(ac.FileService.ExecDir))
	return st.DaemonAddress != "" && !lxdclient.IsLoopbackAddr(st.DaemonAddress)
}

// showCommandHelpDialog — единый вид справок «текст + готовая команда»:
// пояснение с переносом, командная строка и кнопки copy/terminal (тихий
// фидбек галочкой). Вертикальный скролл с каноническим gutter'ом.
func showCommandHelpDialog(ac *core.AppController, win fyne.Window, title, text, command string) {
	helpText := widget.NewLabel(text)
	helpText.Wrapping = fyne.TextWrapWord
	cmdEntry := widget.NewEntry()
	cmdEntry.Wrapping = fyne.TextWrapOff
	cmdEntry.SetText(command)
	var copyBtn *ttwidget.Button
	copyBtn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		win.Clipboard().SetContent(cmdEntry.Text)
		copyBtn.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(1200 * time.Millisecond)
			fyne.Do(func() { copyBtn.SetIcon(theme.ContentCopyIcon()) })
		}()
	})
	copyBtn.SetToolTip(locale.T("conn.cmd_copy_tooltip"))
	termBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
		if err := ac.OpenTerminalWithCommand(cmdEntry.Text); err != nil {
			ShowError(win, err)
		}
	})
	termBtn.SetToolTip(locale.T("conn.cmd_terminal_tooltip"))
	content := container.NewVBox(
		helpText,
		container.NewBorder(nil, nil, nil, container.NewHBox(copyBtn, termBtn), cmdEntry),
	)
	scrolled := container.NewVScroll(container.NewBorder(nil, nil, nil,
		components.NewScrollGutter(), content))
	dlg := dialogs.NewCustom(title, scrolled, nil, locale.T("dialog.ok"), win)
	dlg.Resize(fyne.NewSize(520, 360))
	dlg.Show()
}
