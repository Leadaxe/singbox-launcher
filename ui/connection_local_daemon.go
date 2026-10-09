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
	daemonHintText             = "Run the VPN core inside a long-lived system daemon (sing-box lxd). Config changes swap the core in-process — no password prompts, and quitting the launcher can keep the VPN up. Managed over gRPC like the Android app."
	daemonServiceOtherCoreText = "The service runs a different core (%s) than the launcher (%s). Install or update the service to switch it to the launcher core."
	daemonServiceNoBinaryText  = "The service has no core binary to run. Install or update the service to restore it."
	daemonServiceMismatchText  = "The service copy differs from the launcher core in %s. Install or update the service to refresh it."
)

// Платформенные подписи и тексты (Terminal и sudo на macOS, окно UAC на
// Windows) — в command_row_darwin.go / command_row_windows.go:
// daemonInstallRowLabel, daemonOpRowLabel, daemonFreshInviteRowLabel,
// daemonUninstallStepLabel, daemonPairHelpText, daemonServiceUnsafeText,
// daemonServiceManagerText.

// wrappedLabel — Label с переносом: длинная подпись не должна задавать
// min-width колонки (иначе вертикальный скролл распирает окно по ширине).
func wrappedLabel(key string) *widget.Label {
	label := widget.NewLabel(locale.T(key))
	label.Wrapping = fyne.TextWrapWord
	return label
}

// buildDaemonPanel — панель daemon-движка на вкладке LOCAL (macOS, Windows
// x64/arm64): короткая подсказка, «Stop VPN when quitting» и окно Service
// локальной службы (SPEC 161 §4.2) — шапка с диагнозом и вкладки Not
// running / Core / Pairing / Reference / Uninstall. Операции службы
// (install, bootstrap, restart, приглашение, Pair, адрес, секрет, удаление)
// живут в источнике (service_source_local.go).
//
// onPaired — колбэк успешного сопряжения (вкладка доводит переключение
// движка, если пользователь уже выбрал daemon). dispose останавливает опрос
// демона — звать при закрытии окна.
func buildDaemonPanel(ac *core.AppController, win fyne.Window, onPaired func()) (fyne.CanvasObject, func()) {
	binDir := ac.FileService.Layout.Data.Bin()

	// Длинный рассказ про daemon-движок читают один раз, а место он занимает
	// в каждом открытии окна: над окном Service остаётся одна строка, полный
	// текст — под «?».
	hintShort := widget.NewLabel(locale.T("The core runs inside a system daemon (sing-box lxd)."))
	hintShort.Wrapping = fyne.TextWrapWord
	hintHelp := widget.NewButton("?", func() {
		showTextHelpDialog(ac, win, locale.T("Daemon (lxd)"), locale.T(daemonHintText))
	})
	hintHelp.Importance = widget.LowImportance

	// Параметр движка, а не службы: в окно Service не входит (SPEC 161 §4.2).
	stopOnExitCheck := widget.NewCheck(locale.T("Stop VPN when quitting the launcher"), nil)
	stopOnExitCheck.SetChecked(locale.LoadSettings(binDir).DaemonStopVPNOnExit)
	stopOnExitCheck.OnChanged = func(checked bool) {
		st := locale.LoadSettings(binDir)
		st.DaemonStopVPNOnExit = checked
		if err := locale.SaveSettings(binDir, st); err != nil {
			debuglog.WarnLog("conn.daemon: save stop_on_exit: %v", err)
		}
	}

	view, dispose := buildServiceView(ac, win, newLocalServiceSource(ac, win, onPaired), serviceTabAuto)
	top := container.NewVBox(
		container.NewBorder(nil, nil, nil, hintHelp, hintShort),
		stopOnExitCheck,
	)
	return container.NewBorder(top, nil, nil, nil, view), dispose
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
