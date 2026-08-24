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

	// Длинный рассказ про daemon-движок читают один раз, а место он занимает
	// в каждом открытии окна: над вкладками остаётся одна строка, полный
	// текст — под «?».
	hintShort := widget.NewLabel(locale.T("The core runs inside a system daemon (sing-box lxd)."))
	hintShort.Wrapping = fyne.TextWrapWord
	hintHelp := widget.NewButton("?", func() {
		showTextHelpDialog(ac, win, locale.T("Daemon (lxd)"), locale.T("Run the VPN core inside a long-lived system daemon (sing-box lxd). Config changes swap the core in-process — no password prompts, and quitting the launcher can keep the VPN up. Managed over gRPC like the Android app."))
	})
	hintHelp.Importance = widget.LowImportance

	status := widget.NewLabel(locale.T("Checking daemon status…"))
	status.Wrapping = fyne.TextWrapWord

	// --- Статус ----------------------------------------------------------
	renderStatus := func(snap core.DaemonUIStatus) string {
		return renderDaemonStatusText(ac, snap, true)
	}
	// onSnapshot — дополнительный потребитель того же снапшота (выбор
	// стартовой вкладки). Отдельной горутины он не заводит: DaemonStatusSnapshot
	// ходит по сети, и два независимых опроса на каждое открытие окна — это
	// удвоенная задержка ради одного и того же ответа.
	var onSnapshot func(core.DaemonUIStatus)
	refreshStatus := func() {
		go func() {
			snap := ac.DaemonStatusSnapshot()
			fyne.Do(func() {
				status.SetText(renderStatus(snap))
				if onSnapshot != nil {
					onSnapshot(snap)
				}
			})
		}()
	}

	// --- Консольные команды ----------------------------------------------
	// Каждая строка: подпись, команда (копируемое поле), кнопки copy/terminal.
	// Команды перечитываются при каждом действии (адрес мог смениться).
	// Команды локальные — терминал открывается на этой же машине.
	commandRowLocal := func(labelKey string, command func() (string, error)) fyne.CanvasObject {
		return CommandRow(win, labelKey, command, true)
	}

	kickstartRow := commandRowLocal("Restart the service (after a core update):", func() (string, error) { // l10n-key
		return ac.DaemonKickstartCommand(), nil
	})

	// --- Сопряжение по приглашению ---------------------------------------
	inviteEntry := widget.NewEntry()
	inviteEntry.SetPlaceHolder(locale.T("address#fingerprint#code"))
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("Bearer secret (only for a daemon without TLS)"))
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
			locale.T("Bearer secret (only for a daemon without TLS)"),
			locale.T("The Bearer secret is only needed for a daemon running WITHOUT TLS (plain mode): there it is the whole authentication, paste it and press Enter. A paired mTLS daemon ignores it — the client certificate is the credential. The daemon owns the secret (daemon.json in its state dir); view it with the command below."),
			ac.DaemonShowSecretCommand())
	})
	secretHelp.Importance = widget.LowImportance

	pairBtn := widget.NewButton(locale.T("Pair"), func() {
		invite := strings.TrimSpace(inviteEntry.Text)
		if invite == "" {
			ShowErrorText(win, locale.T("Connection settings"), locale.T("Paste an invite first (address#fingerprint#code)."))
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
					locale.T("Connection settings"), locale.T("Paired with the daemon."))
				if onPaired != nil {
					onPaired()
				}
				refreshStatus()
			})
		}()
	})
	pairHelp := widget.NewButton("?", func() {
		showCommandHelpDialog(ac, win,
			locale.T("Pair"),
			locale.T("Paste the invite printed by the daemon and click Pair. Where to get one:\n\n- Installing the service prints an invite at the end of its Terminal output (Install section, step 1).\n- For a fresh invite run the command below (copy or open in Terminal), then paste the printed invite into the pairing field.\n\nThe code is one-time: it burns after a successful pairing. The secret field is only for daemons running without TLS."),
			ac.DaemonRepairCommand())
	})
	pairHelp.Importance = widget.LowImportance

	// --- Секция Uninstall: два последовательных шага ----------------------
	// 1) Unpair — локальная сторона (пара лаунчера, пин, адрес);
	// 2) удаление службы sudo-командой, с галкой --purge (по умолчанию ВКЛ:
	//    «снести — так снести»; выключают её осознанно, чтобы сохранить
	//    state демона для будущей переустановки без пере-сопряжения).
	unpairBtn := widget.NewButton(locale.T("Unpair"), func() {
		ShowConfirm(win,
			locale.T("Forget pairing?"),
			locale.T("Removes the launcher's client keys and daemon address. The daemon keeps its record of this client until removed there (sing-box lxd client remove)."),
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

	purgeCheck := widget.NewCheck(locale.T("Also wipe all daemon data — keys, clients, last-good (--purge)"), nil)
	purgeCheck.SetChecked(true)
	uninstallEntry := widget.NewEntry()
	uninstallEntry.Wrapping = fyne.TextWrapOff
	refreshUninstallCommand := func() {
		uninstallEntry.SetText(ac.DaemonUninstallCommand(purgeCheck.Checked))
	}
	purgeCheck.OnChanged = func(bool) { refreshUninstallCommand() }
	refreshUninstallCommand()
	uninstallCopyBtn := NewCopyButton("Copy the command", func() (string, bool) {
		refreshUninstallCommand()
		return uninstallEntry.Text, true
	})
	uninstallTermBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
		refreshUninstallCommand()
		if err := ac.OpenTerminalWithCommand(uninstallEntry.Text); err != nil {
			ShowError(win, err)
		}
	})
	uninstallTermBtn.SetToolTip(locale.T("Run in Terminal"))

	uninstallTab := container.NewVBox(
		wrappedLabel("1. Forget the pairing on the launcher side:"), // l10n-key
		unpairBtn,
		wrappedLabel("2. Remove the service (run in Terminal, your sudo):"), // l10n-key
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

	stopOnExitCheck := widget.NewCheck(locale.T("Stop VPN when quitting the launcher"), nil)
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
	refreshBtn.SetToolTip(locale.T("Refresh daemon status"))

	// --- Вкладка Install: два последовательных шага ------------------------
	// 1) sudo-команда установки (сама печатает приглашение в конце);
	// 2) поле приглашения + Pair. Свежее приглашение для пере-сопряжения —
	//    отдельной строкой ниже (lxd client add): это тот же шаг 2, только
	//    для случая «служба уже стоит, приглашение из установки протухло».
	installTab := container.NewVBox(
		commandRowLocal("1. Install the service (run in Terminal, your sudo — prints a pairing invite at the end):", ac.DaemonInstallCommand), // l10n-key
		wrappedLabel("2. Paste the invite (address#fingerprint#code) and pair:"),                                                              // l10n-key
		container.NewBorder(nil, nil, nil, container.NewHBox(pairBtn, pairHelp), inviteEntry),
		widget.NewSeparator(),
		commandRowLocal("Need a fresh invite (service already installed)?", func() (string, error) { // l10n-key
			return ac.DaemonRepairCommand(), nil
		}),
	)

	// --- Вкладка Status: состояние + повседневные параметры ----------------
	// Порядок продиктован тем, как читают экран при проблеме: сначала статус
	// (что не так), сразу под ним перезапуск службы (типовое лечение после
	// обновления ядра), и только затем параметры подключения.
	statusTab := container.NewVBox(
		container.NewBorder(nil, nil, nil, refreshBtn, status),
		widget.NewSeparator(),
		kickstartRow,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(locale.T("Connection"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, widget.NewLabel(locale.T("Daemon address:")), nil, addressEntry),
		container.NewBorder(nil, nil, nil, secretHelp, secretEntry),
		stopOnExitCheck,
	)

	// Три вкладки вместо одной простыни: повседневное (Status), разовая
	// установка и опасное удаление разведены — по частоте использования и по
	// цене ошибки. Заголовки секций стали именами вкладок, поэтому внутри их
	// больше нет.
	tabs := container.NewAppTabs(
		container.NewTabItem(locale.T("Status"), container.NewPadded(statusTab)),
		container.NewTabItem(locale.T("Install"), container.NewPadded(installTab)),
		container.NewTabItem(locale.T("Uninstall"), container.NewPadded(uninstallTab)),
	)
	// Стартовая вкладка — по состоянию: пока служба не установлена или не
	// сопряжена, единственное осмысленное действие живёт в Install; открывать
	// на Status значило бы показывать список того, чего ещё нет. Срабатывает
	// один раз: дальше вкладку выбирает пользователь, и перевод её под ним на
	// плановом рефреше статуса был бы угоном фокуса.
	var autoSelectDone bool
	onSnapshot = func(snap core.DaemonUIStatus) {
		if autoSelectDone {
			return
		}
		autoSelectDone = true
		if !snap.ServiceInstalled || !snap.Paired {
			tabs.SelectIndex(1)
		}
	}

	// Первый опрос — только теперь: onSnapshot должен быть на месте до него,
	// иначе быстрый ответ демона проскочит мимо автовыбора вкладки.
	refreshStatus()

	return container.NewVBox(
		container.NewBorder(nil, nil, nil, hintHelp, hintShort),
		tabs,
	)
}

// renderDaemonStatusText — общий рендер статуса демона. includeLocalService
// добавляет строки про ЛОКАЛЬНОЕ окружение (поддержка lxd установленным
// ядром, наличие launchd-службы) — для удалённого демона они бессмысленны.
func renderDaemonStatusText(ac *core.AppController, snap core.DaemonUIStatus, includeLocalService bool) string {
	var b strings.Builder
	if includeLocalService {
		if snap.CoreSupportsLxd {
			b.WriteString(locale.T("✅ Installed core supports the daemon"))
		} else {
			b.WriteString(locale.T("❌ Installed core has no lxd support (need sing-box-lx 1.14.0-lx.23+)"))
		}
		b.WriteString("\n")
		if snap.ServiceInstalled {
			b.WriteString(locale.T("✅ System service installed"))
		} else {
			b.WriteString(locale.T("— System service not installed"))
		}
		b.WriteString("\n")
	}
	if snap.Paired {
		b.WriteString(locale.Tf("✅ Paired (%s)", snap.Address))
	} else {
		b.WriteString(locale.T("— Not paired"))
	}
	if snap.Reachable {
		b.WriteString("\n")
		b.WriteString(locale.Tf("✅ Daemon reachable, core: %s", snap.CoreStatus))
		if snap.DaemonVersion != "" {
			// Локальная сборка ядра репортит version=unknown — «vunknown»
			// выглядит как опечатка; показываем честное «dev build».
			displayVersion := "v" + snap.DaemonVersion
			if snap.DaemonVersion == "unknown" {
				displayVersion = "dev build"
			}
			b.WriteString("\n")
			b.WriteString(locale.Tf("Daemon %s · home: %s", displayVersion, snap.StateDir))
		}
		if snap.InterruptedApply {
			b.WriteString("\n")
			b.WriteString(locale.T("⚠️ A previous config apply was interrupted; the daemon booted the last working config."))
		}
		if snap.LastError != "" {
			b.WriteString("\n")
			b.WriteString(locale.Tf("Last error: %s", snap.LastError))
		}
	} else if snap.Paired {
		b.WriteString("\n")
		b.WriteString(locale.T("❌ Daemon not reachable"))
	}
	// Движок фактически не daemon (не сопряжён / переключение не прошло):
	// подсказываем, что выбор выше — намерение, а не состояние.
	if ac.BackendMode() != core.BackendDaemon {
		b.WriteString("\n")
		b.WriteString(locale.T("— Daemon engine is not active yet: install the service and pair on the Install tab, then it switches on automatically."))
	}
	return b.String()
}

// showCommandHelpDialog — единый вид справок «текст + готовая команда»:
// пояснение с переносом, командная строка и кнопки copy/terminal (тихий
// фидбек галочкой). Вертикальный скролл с каноническим gutter'ом.
// showTextHelpDialog — справка без команды: тот же вид, что и у справок
// «текст + команда», чтобы «?» в окне вели себя одинаково.
func showTextHelpDialog(ac *core.AppController, win fyne.Window, title, text string) {
	showCommandHelpDialog(ac, win, title, text, "")
}

// command=="" — справка без командной строки (см. showTextHelpDialog).
func showCommandHelpDialog(ac *core.AppController, win fyne.Window, title, text, command string) {
	helpText := widget.NewLabel(text)
	helpText.Wrapping = fyne.TextWrapWord
	cmdEntry := widget.NewEntry()
	cmdEntry.Wrapping = fyne.TextWrapOff
	cmdEntry.SetText(command)
	copyBtn := NewCopyButton("Copy the command", func() (string, bool) { return cmdEntry.Text, true })
	termBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
		if err := ac.OpenTerminalWithCommand(cmdEntry.Text); err != nil {
			ShowError(win, err)
		}
	})
	termBtn.SetToolTip(locale.T("Run in Terminal"))
	content := container.NewVBox(helpText)
	if command != "" {
		content.Add(container.NewBorder(nil, nil, nil, container.NewHBox(copyBtn, termBtn), cmdEntry))
	}
	scrolled := container.NewVScroll(container.NewBorder(nil, nil, nil,
		components.NewScrollGutter(), content))
	dlg := dialogs.NewCustom(title, scrolled, nil, locale.T("OK"), win)
	dlg.Resize(fyne.NewSize(520, 360))
	dlg.Show()
}
