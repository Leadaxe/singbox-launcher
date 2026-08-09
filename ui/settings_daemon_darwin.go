//go:build darwin

package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// buildDaemonSection — секция «Движок ядра (демон lxd)» в настройках, macOS.
//
// Состав: переключатель режима Classic/Daemon, статус (ядро/служба/
// сопряжение/доступность), установка и удаление LaunchDaemon (включая полное
// удаление с данными), поле сопряжения по приглашению, правка адреса и
// поведение при выходе.
func buildDaemonSection(ac *core.AppController) fyne.CanvasObject {
	binDir := platform.GetBinDir(ac.FileService.ExecDir)

	title := widget.NewLabelWithStyle(locale.T("settings.section_daemon"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel(locale.T("settings.daemon_hint"))
	hint.Wrapping = fyne.TextWrapWord

	status := widget.NewLabel(locale.T("settings.daemon_status_checking"))
	status.Wrapping = fyne.TextWrapWord

	// --- Режим ---------------------------------------------------------
	// guard от рекурсии: SetChecked при откате изменил бы состояние и снова
	// дёрнул OnChanged.
	var updatingMode bool
	modeCheck := widget.NewCheck(locale.T("settings.daemon_mode_label"), nil)
	modeCheck.SetChecked(ac.BackendMode() == core.BackendDaemon)

	stopOnExitCheck := widget.NewCheck(locale.T("settings.daemon_stop_on_exit_label"), nil)
	{
		st := locale.LoadSettings(binDir)
		stopOnExitCheck.SetChecked(st.DaemonStopVPNOnExit)
	}
	stopOnExitCheck.OnChanged = func(checked bool) {
		st := locale.LoadSettings(binDir)
		st.DaemonStopVPNOnExit = checked
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("settings.daemon: save stop_on_exit: %v", err)
		}
	}

	// --- Статус --------------------------------------------------------
	renderStatus := func(snap core.DaemonUIStatus) string {
		var b strings.Builder
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
		if snap.Paired {
			b.WriteString(locale.Tf("settings.daemon_status_paired", snap.Address))
		} else {
			b.WriteString(locale.T("settings.daemon_status_not_paired"))
		}
		if snap.Reachable {
			b.WriteString("\n")
			b.WriteString(locale.Tf("settings.daemon_status_reachable", snap.CoreStatus))
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
		return b.String()
	}
	refreshStatus := func() {
		go func() {
			snap := ac.DaemonStatusSnapshot()
			fyne.Do(func() {
				status.SetText(renderStatus(snap))
				updatingMode = true
				modeCheck.SetChecked(ac.BackendMode() == core.BackendDaemon)
				updatingMode = false
			})
		}()
	}

	modeCheck.OnChanged = func(enabled bool) {
		if updatingMode {
			return
		}
		revert := func() {
			updatingMode = true
			modeCheck.SetChecked(!enabled)
			updatingMode = false
		}
		if ac.RunningState.IsRunning() {
			revert()
			ShowErrorText(ac.UIService.MainWindow, locale.T("settings.section_daemon"), locale.T("settings.daemon_stop_vpn_first"))
			return
		}
		target := core.BackendClassic
		if enabled {
			target = core.BackendDaemon
		}
		if err := ac.SwitchBackendMode(target); err != nil {
			revert()
			ShowError(ac.UIService.MainWindow, err)
			return
		}
		st := locale.LoadSettings(binDir)
		st.CoreBackendMode = string(target)
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("settings.daemon: save core_backend_mode: %v", err)
		}
		refreshStatus()
	}

	// --- Служба: установка / удаление ----------------------------------
	installBtn := widget.NewButtonWithIcon(locale.T("settings.daemon_install_btn"), theme.DownloadIcon(), nil)
	uninstallBtn := widget.NewButton(locale.T("settings.daemon_uninstall_btn"), nil)
	purgeBtn := widget.NewButton(locale.T("settings.daemon_purge_btn"), nil)
	purgeBtn.Importance = widget.DangerImportance

	serviceButtons := []*widget.Button{installBtn, uninstallBtn, purgeBtn}
	setServiceButtons := func(enabled bool) {
		for _, b := range serviceButtons {
			if enabled {
				b.Enable()
			} else {
				b.Disable()
			}
		}
	}
	// opInFlight — guard от повторного открытия confirm-диалога и параллельных
	// service-операций (двойной клик по «Install»/«Uninstall» плодил стопку
	// диалогов и параллельные привилегированные вызовы). Доступ только из
	// UI-потока (все обработчики кнопок), поэтому без мьютекса.
	opInFlight := false
	// runServiceOp — общий каркас: confirm → фон → диалог результата → refresh.
	runServiceOp := func(confirmTitle, confirmBody, doneMsg string, op func() error, autoEnableDaemon bool) {
		if opInFlight {
			return
		}
		opInFlight = true
		ShowConfirm(ac.UIService.MainWindow, confirmTitle, confirmBody, func(ok bool) {
			if !ok {
				opInFlight = false
				return
			}
			setServiceButtons(false)
			status.SetText(locale.T("settings.daemon_status_working"))
			go func() {
				err := op()
				fyne.Do(func() {
					setServiceButtons(true)
					opInFlight = false
					if err != nil {
						ShowError(ac.UIService.MainWindow, err)
						refreshStatus()
						return
					}
					if autoEnableDaemon {
						// Установка прошла + сопряжение готово: включаем режим,
						// не заставляя пользователя искать галочку.
						if err := ac.SwitchBackendMode(core.BackendDaemon); err == nil {
							st := locale.LoadSettings(binDir)
							st.CoreBackendMode = string(core.BackendDaemon)
							if saveErr := locale.SaveSettings(binDir, st); saveErr != nil {
								debuglog.WarnLog("settings.daemon: save mode after install: %v", saveErr)
							}
						} else {
							debuglog.WarnLog("settings.daemon: enable daemon mode after install: %v", err)
						}
					}
					dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
						locale.T("settings.section_daemon"), doneMsg)
					refreshStatus()
				})
			}()
		})
	}

	installBtn.OnTapped = func() {
		runServiceOp(
			locale.T("settings.daemon_install_confirm_title"),
			locale.T("settings.daemon_install_confirm_body"),
			locale.T("settings.daemon_install_done"),
			ac.InstallDaemonService,
			true,
		)
	}
	uninstallBtn.OnTapped = func() {
		runServiceOp(
			locale.T("settings.daemon_uninstall_confirm_title"),
			locale.T("settings.daemon_uninstall_confirm_body"),
			locale.T("settings.daemon_uninstall_done"),
			func() error { return ac.UninstallDaemonService(false) },
			false,
		)
	}
	purgeBtn.OnTapped = func() {
		runServiceOp(
			locale.T("settings.daemon_purge_confirm_title"),
			locale.T("settings.daemon_purge_confirm_body"),
			locale.T("settings.daemon_purge_done"),
			func() error { return ac.UninstallDaemonService(true) },
			false,
		)
	}

	// --- Второй ряд: терминальный путь (оператор управляет сам) ----------
	// Надёжнее автоматической установки (настоящий root через собственный
	// sudo, полный вывод launchctl). Лаунчер открывает Terminal.app с готовой
	// командой; после ручной установки — кнопка «Сопрячь» досопрягается сама.
	termInstallBtn := widget.NewButtonWithIcon(locale.T("settings.daemon_term_install_btn"), theme.ComputerIcon(), func() {
		cmd, err := ac.DaemonInstallCommand()
		if err != nil {
			ShowError(ac.UIService.MainWindow, err)
			return
		}
		if err := ac.OpenTerminalWithCommand(cmd); err != nil {
			// Терминал не открылся — покажем команду для ручного копирования.
			dialogs.ShowLinuxCapabilitiesRequired(ac.UIService.MainWindow,
				locale.T("settings.daemon_term_install_btn"),
				locale.T("settings.daemon_term_manual_hint"), cmd)
			return
		}
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
			locale.T("settings.section_daemon"), locale.T("settings.daemon_term_opened"))
	})
	termUninstallBtn := widget.NewButton(locale.T("settings.daemon_term_uninstall_btn"), func() {
		cmd := ac.DaemonUninstallCommand(false)
		if err := ac.OpenTerminalWithCommand(cmd); err != nil {
			dialogs.ShowLinuxCapabilitiesRequired(ac.UIService.MainWindow,
				locale.T("settings.daemon_term_uninstall_btn"),
				locale.T("settings.daemon_term_manual_hint"), cmd)
		}
	})
	pairLocalBtn := widget.NewButton(locale.T("settings.daemon_pair_local_btn"), func() {
		go func() {
			err := ac.PairWithLocalService()
			fyne.Do(func() {
				if err != nil {
					ShowError(ac.UIService.MainWindow, err)
					return
				}
				dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
					locale.T("settings.section_daemon"), locale.T("settings.daemon_pair_done"))
				refreshStatus()
			})
		}()
	})

	// --- Сопряжение по приглашению --------------------------------------
	inviteEntry := widget.NewEntry()
	inviteEntry.SetPlaceHolder(locale.T("settings.daemon_invite_placeholder"))
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("settings.daemon_secret_placeholder"))

	pairBtn := widget.NewButton(locale.T("settings.daemon_pair_btn"), func() {
		invite := strings.TrimSpace(inviteEntry.Text)
		if invite == "" {
			ShowErrorText(ac.UIService.MainWindow, locale.T("settings.section_daemon"), locale.T("settings.daemon_invite_empty"))
			return
		}
		secret := strings.TrimSpace(secretEntry.Text)
		go func() {
			err := ac.PairDaemonWithInvite(invite, secret)
			fyne.Do(func() {
				if err != nil {
					ShowError(ac.UIService.MainWindow, err)
					return
				}
				inviteEntry.SetText("")
				dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
					locale.T("settings.section_daemon"), locale.T("settings.daemon_pair_done"))
				refreshStatus()
			})
		}()
	})
	pairHelp := widget.NewButton("?", func() {
		ShowInfo(ac.UIService.MainWindow, locale.T("settings.daemon_pair_btn"), locale.T("settings.daemon_pair_help"))
	})
	pairHelp.Importance = widget.LowImportance

	unpairBtn := widget.NewButton(locale.T("settings.daemon_unpair_btn"), func() {
		ShowConfirm(ac.UIService.MainWindow,
			locale.T("settings.daemon_unpair_confirm_title"),
			locale.T("settings.daemon_unpair_confirm_body"),
			func(ok bool) {
				if !ok {
					return
				}
				if err := ac.UnpairDaemon(); err != nil {
					ShowError(ac.UIService.MainWindow, err)
					return
				}
				refreshStatus()
			})
	})

	// --- Адрес управляющего канала --------------------------------------
	addressEntry := widget.NewEntry()
	{
		st := locale.LoadSettings(binDir)
		addressEntry.SetText(st.DaemonAddress)
	}
	addressEntry.SetPlaceHolder("127.0.0.1:9091")
	addressEntry.OnSubmitted = func(text string) {
		if err := ac.SetDaemonAddress(text); err != nil {
			ShowError(ac.UIService.MainWindow, err)
			return
		}
		refreshStatus()
	}
	addressLabel := widget.NewLabel(locale.T("settings.daemon_address_label"))

	refreshBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), refreshStatus)
	refreshBtn.SetToolTip(locale.T("settings.daemon_refresh_tooltip"))

	refreshStatus()

	return container.NewVBox(
		title,
		hint,
		modeCheck,
		stopOnExitCheck,
		container.NewBorder(nil, nil, nil, refreshBtn, status),
		// Ряд 1 — автоматически (пароль macOS один раз через лаунчер).
		// Grid из 3 равных колонок ужимается в ширину окна вместо того чтобы
		// распирать его под минимальную ширину кнопок (как делал HBox).
		container.NewGridWithColumns(3, installBtn, uninstallBtn, purgeBtn),
		// Ряд 2 — в терминале (свой sudo, полный вывод; для тех, кто хочет
		// контролировать установку сам). Метка поясняет назначение ряда.
		widget.NewLabelWithStyle(locale.T("settings.daemon_term_row_label"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		container.NewGridWithColumns(3, termInstallBtn, termUninstallBtn, pairLocalBtn),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel(locale.T("settings.daemon_pair_label")), container.NewHBox(pairBtn, pairHelp), inviteEntry),
		container.NewBorder(nil, nil, nil, unpairBtn, secretEntry),
		container.NewBorder(nil, nil, addressLabel, nil, addressEntry),
	)
}
