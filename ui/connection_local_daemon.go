//go:build darwin || (windows && !386)

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
	"singbox-launcher/ui/components"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	secretHelpText              = "The Bearer secret is only needed for a daemon running WITHOUT TLS (plain mode): there it is the whole authentication, paste it and press Enter. A paired mTLS daemon ignores it — the client certificate is the credential. The daemon owns the secret (daemon.json in its state dir); view it with the command below."
	daemonHintText              = "Run the VPN core inside a long-lived system daemon (sing-box lxd). Config changes swap the core in-process — no password prompts, and quitting the launcher can keep the VPN up. Managed over gRPC like the Android app."
	daemonUnpairConfirmBodyText = "Removes the launcher's client keys and daemon address. The daemon keeps its record of this client until removed there (sing-box lxd client remove)."
	daemonServiceOtherCoreText  = "The service runs a different core (%s) than the launcher (%s). Install or update the service to switch it to the launcher core."
	daemonServiceNoBinaryText   = "The service has no core binary to run. Install or update the service to restore it."
	daemonServiceMismatchText   = "The service copy differs from the launcher core in %s. Install or update the service to refresh it."
	daemonUninstallAskText      = "Keep the protected core copy? The classic engine starts from it when the launcher runs as administrator. Remove all also deletes the copy and all daemon data."
)

// Платформенные подписи и тексты (Terminal и sudo на macOS, окно UAC на
// Windows) — в command_row_darwin.go / command_row_windows.go:
// daemonInstallRowLabel, daemonStartRowLabel, daemonInstallStepLabel,
// daemonPairStepLabel, daemonUninstallStepLabel, daemonPairHelpText,
// daemonServiceUnsafeText, daemonServiceManagerText.

// wrappedLabel — Label с переносом: длинная подпись не должна задавать
// min-width колонки (иначе вертикальный скролл распирает окно по ширине).
func wrappedLabel(key string) *widget.Label {
	label := widget.NewLabel(locale.T(key))
	label.Wrapping = fyne.TextWrapWord
	return label
}

// buildDaemonPanel — панель daemon-движка на вкладке LOCAL (macOS, Windows
// x64/arm64).
//
// macOS: все привилегированные операции — консольные: панель показывает
// готовые sudo-команды (установка / удаление / полное удаление /
// пере-сопряжение / kickstart) с кнопками «копировать» и «открыть в
// терминале». Лаунчер сам ничего под root не запускает (AEWP выпилен).
// Сопряжение: команда печатает одноразовое приглашение — пользователь
// вставляет его в поле ниже. Windows (SPEC 141 §9): те же строки, но вместо
// терминала — «Run as administrator»: операцию исполняет core через окно
// UAC, install сопрягается сам (см. daemonOps).
//
// onPaired — колбэк успешного сопряжения (вкладка доводит переключение
// движка, если пользователь уже выбрал daemon).
func buildDaemonPanel(ac *core.AppController, win fyne.Window, onPaired func()) fyne.CanvasObject {
	binDir := ac.FileService.Layout.Data.Bin()

	// Длинный рассказ про daemon-движок читают один раз, а место он занимает
	// в каждом открытии окна: над вкладками остаётся одна строка, полный
	// текст — под «?».
	hintShort := widget.NewLabel(locale.T("The core runs inside a system daemon (sing-box lxd)."))
	hintShort.Wrapping = fyne.TextWrapWord
	hintHelp := widget.NewButton("?", func() {
		showTextHelpDialog(ac, win, locale.T("Daemon (lxd)"), locale.T(daemonHintText))
	})
	hintHelp.Importance = widget.LowImportance

	status := widget.NewLabel(locale.T("Checking daemon status…"))
	status.Wrapping = fyne.TextWrapWord

	// --- Статус ----------------------------------------------------------
	renderStatus := func(snap core.DaemonUIStatus) string {
		return renderDaemonStatusText(ac, snap, true)
	}

	// Плашка состояния службы (SPEC 136 §6): Unsafe — красная, Stale,
	// NotRunning и ProcessStale — жёлтая, под текстом команда: «Install or
	// update service», а для NotRunning — загрузка plist (bootstrap). На
	// вкладке Status, куда смотрят при проблеме; строки команд добавляются
	// ниже, когда есть ops. Скрыта при OK и без службы.
	// CoreTooOld — подсказка обновить ядро, без команды (красная, если
	// служба при этом Unsafe).
	serviceIcon := widget.NewIcon(theme.WarningIcon())
	serviceLabel := widget.NewLabel("")
	serviceLabel.Wrapping = fyne.TextWrapWord
	serviceBox := container.NewVBox(container.NewBorder(nil, nil, container.NewVBox(serviceIcon), nil, serviceLabel))
	serviceBox.Hide()
	var serviceInstallRow, serviceBootstrapRow, installStepRow fyne.CanvasObject
	// Шаг 1 вкладки Install при ядре без root-owned копии: вместо команды —
	// подсказка обновить ядро.
	installCoreHint := widget.NewLabel("")
	installCoreHint.Wrapping = fyne.TextWrapWord
	installCoreHint.Importance = widget.WarningImportance
	installCoreHint.Hide()
	applyServiceState := func(snap core.DaemonUIStatus) {
		if installStepRow != nil {
			if snap.Service.InstallSupported() {
				installCoreHint.Hide()
				installStepRow.Show()
			} else {
				installStepRow.Hide()
				installCoreHint.SetText(core.DaemonServiceCoreHint(snap.Service.LauncherVersion))
				installCoreHint.Show()
			}
		}
		text, danger := daemonServiceNoticeText(snap.Service)
		if text == "" {
			serviceBox.Hide()
			return
		}
		if serviceInstallRow != nil && serviceBootstrapRow != nil {
			serviceInstallRow.Hide()
			serviceBootstrapRow.Hide()
			switch {
			case snap.Service.NeedsBootstrap():
				serviceBootstrapRow.Show()
			case snap.Service.NeedsInstall():
				serviceInstallRow.Show()
			}
		}
		if danger {
			serviceIcon.SetResource(theme.ErrorIcon())
			serviceLabel.Importance = widget.DangerImportance
		} else {
			serviceIcon.SetResource(theme.WarningIcon())
			serviceLabel.Importance = widget.WarningImportance
		}
		serviceLabel.SetText(text)
		serviceBox.Show()
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
				applyServiceState(snap)
				if onSnapshot != nil {
					onSnapshot(snap)
				}
			})
		}()
	}

	// --- Команды и операции службы ----------------------------------------
	// Каждая строка: подпись, команда (копируемое поле), кнопки copy/terminal
	// (macOS) или copy/«Run as administrator» (Windows, daemonOps).
	// Команды перечитываются при каждом действии (адрес мог смениться).
	// Команды локальные — терминал открывается на этой же машине.
	ops := &daemonOps{ac: ac, win: win, after: func(r core.DaemonRunResult) {
		if r.Paired && onPaired != nil {
			onPaired()
		}
		refreshStatus()
	}}

	// Та же команда, что на вкладке Install (второй экземпляр строки: объект
	// Fyne не живёт в двух вкладках сразу). `--service=install` поверх
	// существующей службы обновляет root-owned копию и перезапускает службу.
	// Отдельной строки kickstart нет: после обновления ядра перезапуск поднял
	// бы ту же старую копию (SPEC 136 §5).
	serviceInstallRow = ops.row(daemonInstallRowLabel, daemonRunAsAdminKey, ac.DaemonInstallCommand, ac.DaemonInstallOrUpdate)
	// NotRunning: определение службы и копия в порядке, менеджер служб её не
	// держит — её запускают (launchd bootstrap / sc.exe start), а не
	// переустанавливают.
	serviceBootstrapRow = ops.row(daemonStartRowLabel, daemonStartServiceKey, ac.DaemonBootstrapCommand, ac.DaemonStartService)
	serviceBootstrapRow.Hide()
	serviceBox.Add(serviceInstallRow)
	serviceBox.Add(serviceBootstrapRow)

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
			locale.T(secretHelpText),
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
			locale.T(daemonPairHelpText),
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
			locale.T(daemonUnpairConfirmBodyText),
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
	uninstallButtons := container.NewHBox(uninstallCopyBtn)
	uninstallResult := ops.newResult()
	if core.DaemonOpsElevated {
		// Windows: перед запуском — вопрос про копию ядра: её исполняет
		// classic с правами администратора (SPEC 141 §8); «Remove all» —
		// полное удаление, как в «Remove all data…».
		uninstallRun := func(keepCopy, purge bool) func(*dialogs.ActionsDialog) {
			return func(d *dialogs.ActionsDialog) {
				d.Hide()
				refreshUninstallCommand()
				ops.run(func() core.DaemonRunResult { return ac.DaemonUninstallService(keepCopy, purge) }, uninstallEntry, uninstallResult)
			}
		}
		uninstallButtons.Add(ops.button(daemonRunAsAdminKey, func() {
			dialogs.ShowActions(win, locale.T("Remove the service"), locale.T(daemonUninstallAskText), []dialogs.Action{
				{Label: locale.T("Keep the core copy"), Important: true, Run: uninstallRun(true, purgeCheck.Checked)},
				{Label: locale.T("Remove all"), Run: uninstallRun(false, true)},
			}, locale.T("Cancel"))
		}))
	} else if openTerminal != nil {
		uninstallTermBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
			refreshUninstallCommand()
			if err := openTerminal(uninstallEntry.Text); err != nil {
				ShowError(win, err)
			}
		})
		uninstallTermBtn.SetToolTip(locale.T("Run in Terminal"))
		uninstallButtons.Add(uninstallTermBtn)
	}

	uninstallTab := container.NewVBox(
		wrappedLabel("1. Forget the pairing on the launcher side:"), // l10n-key
		unpairBtn,
		wrappedLabel(daemonUninstallStepLabel),
		purgeCheck,
		container.NewBorder(nil, nil, nil, uninstallButtons, uninstallEntry),
	)
	if core.DaemonOpsElevated {
		uninstallTab.Add(uninstallResult.object())
	}

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
	installStepRow = ops.row(daemonInstallStepLabel, daemonRunAsAdminKey, ac.DaemonInstallCommand, ac.DaemonInstallOrUpdate)
	installTab := container.NewVBox(
		installStepRow,
		installCoreHint,
		wrappedLabel(daemonPairStepLabel),
		container.NewBorder(nil, nil, nil, container.NewHBox(pairBtn, pairHelp), inviteEntry),
		widget.NewSeparator(),
		ops.row("Need a fresh invite (service already installed)?", daemonRunAsAdminKey, func() (string, error) { // l10n-key
			return ac.DaemonRepairCommand(), nil
		}, ac.DaemonFreshInvite),
	)

	// --- Вкладка Status: состояние + повседневные параметры ----------------
	// Порядок продиктован тем, как читают экран при проблеме: сначала статус
	// (что не так), сразу под ним плашка службы с командой ремонта, и только
	// затем параметры подключения.
	statusTab := container.NewVBox(
		container.NewBorder(nil, nil, nil, refreshBtn, status),
		serviceBox,
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
			b.WriteString(locale.T("❌ Installed core has no lxd support (a sing-box-lx build with the lxd command is required)"))
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

// daemonServiceNoticeText — текст плашки службы по вердикту классификатора
// (SPEC 136 §6); "" — плашки нет. danger — красная (Unsafe, в том числе
// CoreTooOld поверх Unsafe), иначе жёлтая.
func daemonServiceNoticeText(c core.DaemonServiceCheck) (text string, danger bool) {
	switch c.State {
	case core.DaemonServiceUnsafe:
		text = locale.T(daemonServiceUnsafeText)
		if c.ServicePath != "" {
			text += "\n" + locale.Tf("Service binary: %s", c.ServicePath)
		}
		return text, true
	case core.DaemonServiceStale:
		if c.CopyMissing {
			return locale.T(daemonServiceNoBinaryText), false
		}
		if c.MismatchFile != "" {
			// Windows: расходится libcronet.dll или в каталоге копии лишний
			// файл (SPEC 141 §6.2).
			return locale.Tf(daemonServiceMismatchText, c.MismatchFile), false
		}
		return locale.Tf(daemonServiceOtherCoreText,
			coreBuildLabel(c.CopyVersion, c.CopySHA256), coreBuildLabel(c.LauncherVersion, c.LauncherSHA256)), false
	case core.DaemonServiceCoreTooOld:
		// Команды нет: install ядра лаунчера переписал бы plist на его файл.
		text = core.DaemonServiceCoreHint(c.LauncherVersion)
		if c.BlockedState == core.DaemonServiceUnsafe {
			if c.ServicePath != "" {
				text += "\n" + locale.Tf("Service binary: %s", c.ServicePath)
			}
			return text, true
		}
		return text, false
	case core.DaemonServiceNotRunning:
		text = locale.T("The service is installed but not running.")
		if c.LaunchdState != "" {
			text += "\n" + locale.Tf(daemonServiceManagerText, c.LaunchdState)
		}
		return text, false
	case core.DaemonServiceProcessStale:
		current := c.CopyVersion
		if current == "" {
			current = c.LauncherVersion
		}
		// Файлы совпали (иначе был бы Stale): копия — ядро лаунчера.
		return locale.Tf(daemonServiceOtherCoreText,
			coreBuildLabel(c.RunningVersion, c.RunningSHA256), coreBuildLabel(current, c.CopySHA256)), false
	}
	return "", false
}

// coreBuildLabel — «версия · sha256[:12]» для плашки; пустые части
// опускаются, обе пустые — «?».
func coreBuildLabel(version, sha string) string {
	var parts []string
	if version != "" {
		parts = append(parts, version)
	}
	if len(sha) > 12 {
		sha = sha[:12]
	}
	if sha != "" {
		parts = append(parts, sha)
	}
	if len(parts) == 0 {
		return "?"
	}
	return strings.Join(parts, " · ")
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
	buttons := container.NewHBox(copyBtn)
	// Терминал — только там, где лаунчер умеет его открыть (macOS); на
	// Windows команду копируют в консоль администратора.
	if openTerminal != nil {
		termBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
			if err := openTerminal(cmdEntry.Text); err != nil {
				ShowError(win, err)
			}
		})
		termBtn.SetToolTip(locale.T("Run in Terminal"))
		buttons.Add(termBtn)
	}
	content := container.NewVBox(helpText)
	if command != "" {
		content.Add(container.NewBorder(nil, nil, nil, buttons, cmdEntry))
	}
	scrolled := container.NewVScroll(container.NewBorder(nil, nil, nil,
		components.NewScrollGutter(), content))
	dlg := dialogs.NewCustom(title, scrolled, nil, locale.T("OK"), win)
	dlg.Resize(fyne.NewSize(520, 360))
	dlg.Show()
}

// Кнопки операций службы на Windows (core.DaemonOpsElevated).
const (
	daemonRunAsAdminKey   = "Run as administrator" // l10n-key
	daemonStartServiceKey = "Start the service"    // l10n-key
)

// daemonOps — операции службы на панели (SPEC 141 §5.2, §9).
//
// macOS (core.DaemonOpsElevated = false): строка — CommandRow с Copy и «Run
// in Terminal», итог пользователь видит в терминале.
//
// Windows: поле команды с Copy и кнопкой «Run as administrator» («Start the
// service» для NotRunning) — операция core в горутине (окно UAC и ожидание
// процесса). На время операции гаснут все кнопки операций панели, под
// строкой — DaemonRunWaitingText; итог — StatusText, предупреждения
// сайдкара — оранжевой строкой; при неудаче в поле — команда операции для
// Copy; install без приглашения (NoInvite) — строка «свежего приглашения».
type daemonOps struct {
	ac      *core.AppController
	win     fyne.Window
	buttons []*widget.Button
	// after — в UI-потоке после операции: пересчёт статуса, onPaired.
	after func(core.DaemonRunResult)
}

// daemonOpResult — строки итога под строкой операции.
type daemonOpResult struct {
	status   *widget.Label
	warnings *widget.Label
	// fresh — строка «свежего приглашения» (NoInvite), создаётся по
	// первому такому итогу; freshCmd — её команда.
	fresh    *fyne.Container
	freshCmd core.DaemonCommand
}

func (o *daemonOps) newResult() *daemonOpResult {
	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Hide()
	warnings := widget.NewLabel("")
	warnings.Wrapping = fyne.TextWrapWord
	warnings.Importance = widget.WarningImportance
	warnings.Hide()
	fresh := container.NewVBox()
	fresh.Hide()
	return &daemonOpResult{status: status, warnings: warnings, fresh: fresh}
}

func (r *daemonOpResult) object() fyne.CanvasObject {
	return container.NewVBox(r.status, r.warnings, r.fresh)
}

// button — кнопка запуска операции; гаснет на время любой операции панели.
func (o *daemonOps) button(key string, tapped func()) *widget.Button {
	btn := widget.NewButton(locale.T(key), tapped)
	o.buttons = append(o.buttons, btn)
	return btn
}

func (o *daemonOps) setBusy(busy bool) {
	for _, b := range o.buttons {
		if busy {
			b.Disable()
		} else {
			b.Enable()
		}
	}
}

// row — строка операции службы: подпись (labelKey "" — без неё), поле
// команды с Copy и кнопка запуска buttonKey (см. daemonOps).
func (o *daemonOps) row(labelKey, buttonKey string, command func() (string, error), op func() core.DaemonRunResult) fyne.CanvasObject {
	if !core.DaemonOpsElevated {
		return CommandRow(o.win, labelKey, command, true)
	}
	entry := widget.NewEntry()
	entry.Wrapping = fyne.TextWrapOff
	if text, err := command(); err == nil {
		entry.SetText(text)
	}
	copyBtn := NewCopyButton("Copy the command", func() (string, bool) {
		return entry.Text, entry.Text != ""
	})
	res := o.newResult()
	runBtn := o.button(buttonKey, func() {
		if text, err := command(); err == nil {
			entry.SetText(text)
		}
		o.run(op, entry, res)
	})
	box := container.NewVBox()
	if labelKey != "" {
		box.Add(wrappedLabel(labelKey))
	}
	box.Add(container.NewBorder(nil, nil, nil, container.NewHBox(copyBtn, runBtn), entry))
	box.Add(res.object())
	return box
}

// run — операция в горутине: кнопки гаснут, строка ожидания, затем итог и
// after. entry — поле команды строки, res — строки итога.
func (o *daemonOps) run(op func() core.DaemonRunResult, entry *widget.Entry, res *daemonOpResult) {
	o.setBusy(true)
	res.status.Importance = widget.MediumImportance
	res.status.SetText(core.DaemonRunWaitingText())
	res.status.Show()
	res.warnings.Hide()
	go func() {
		r := op()
		fyne.Do(func() {
			o.setBusy(false)
			o.showResult(r, entry, res)
			if o.after != nil {
				o.after(r)
			}
		})
	}()
}

// showResult — итог операции (SPEC 141 §5.3): StatusText без предупреждений,
// они — отдельной оранжевой строкой; команда для Copy при неудаче; строка
// «свежего приглашения» при NoInvite.
func (o *daemonOps) showResult(r core.DaemonRunResult, entry *widget.Entry, res *daemonOpResult) {
	warnings := r.Warnings
	r.Warnings = nil
	settled := !r.Cancelled && !r.TimedOut
	// Команда не запустилась или вышла с кодом ≠ 0 — её показывают для Copy.
	cmdFailed := settled && (r.Exited && r.ExitCode != 0 || !r.Exited && r.Err != nil)
	res.status.Importance = widget.MediumImportance
	if settled && !r.NoInvite && (cmdFailed || r.Err != nil) {
		res.status.Importance = widget.DangerImportance
	}
	if text := r.StatusText(); text != "" {
		res.status.SetText(text)
		res.status.Show()
	} else {
		res.status.Hide()
	}
	if len(warnings) > 0 {
		lines := make([]string, 0, len(warnings))
		for _, w := range warnings {
			lines = append(lines, "⚠ "+w.DisplayText())
		}
		res.warnings.SetText(strings.Join(lines, "\n"))
		res.warnings.Show()
	}
	if cmdFailed && !r.Command.IsZero() {
		entry.SetText(r.Command.String())
	}
	if !r.NoInvite {
		res.fresh.Hide()
		return
	}
	res.freshCmd = r.FreshInvite
	if len(res.fresh.Objects) == 0 {
		res.fresh.Add(o.row("", daemonRunAsAdminKey, func() (string, error) {
			if res.freshCmd.IsZero() {
				return o.ac.DaemonRepairCommand(), nil
			}
			return res.freshCmd.String(), nil
		}, o.ac.DaemonFreshInvite))
	}
	res.fresh.Show()
}

// daemonPurgeRow — строка удаления службы в диалоге «Remove all data…»
// (SPEC 135 §4.3): macOS — команда для Terminal, Windows — полный uninstall
// кнопкой «Run as administrator» (SPEC 141 §9). survives — команда идёт
// через защищённую копию службы и переживает удаление данных.
func daemonPurgeRow(ac *core.AppController, win fyne.Window, survives bool, command func() (string, error)) fyne.CanvasObject {
	label := daemonPurgeFirstLabel
	if survives {
		label = daemonPurgeSurvivesLabel
	}
	ops := &daemonOps{ac: ac, win: win}
	return ops.row(label, daemonRunAsAdminKey, command, func() core.DaemonRunResult {
		return ac.DaemonUninstallService(false, true)
	})
}
