//go:build darwin || (windows && !386)

package ui

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
)

// Источник окна Service «локальный демон» (SPEC 161, PLAN §6.5): служба
// sing-box lxd на этом компьютере. Опрашивает ac.DaemonStatusSnapshot
// тикером и переводит тегированные типы core (DaemonServiceCheck,
// daemonOps) в плоский снапшот и готовые строки localServiceRows — окно и
// вкладки (без тега) их не видят.
//
// Строки операций службы (install, bootstrap, restart, приглашение, Pair,
// адрес, секрет, Uninstall) перенесены сюда из прежней панели Local
// (buildDaemonPanel) без изменения поведения: macOS — команда для Terminal
// под sudo пользователя, Windows — «Run as administrator» (daemonOps).

// serviceLocalRefresh — период опроса локального демона. Опрос — один
// /admin/status (+ /admin/info) по loopback и проверка файлов службы;
// пять секунд дают шапке позеленеть вскоре после restart без кликов.
const serviceLocalRefresh = 5 * time.Second

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	secretHelpText              = "The Bearer secret is only needed for a daemon running WITHOUT TLS (plain mode): there it is the whole authentication, paste it and press Enter. A paired mTLS daemon ignores it — the client certificate is the credential. The daemon owns the secret (daemon.json in its state dir); view it with the command below."
	daemonUnpairConfirmBodyText = "Removes the launcher's client keys and daemon address. The daemon keeps its record of this client until removed there (sing-box lxd client remove)."
	daemonUninstallAskText      = "Keep the protected core copy? The classic engine starts from it when the launcher runs as administrator. Remove all also deletes the copy and all daemon data."
	daemonNoLxdText             = "❌ Installed core has no lxd support (a sing-box-lx build with the lxd command is required)"
	daemonNotInstalledText      = "The system service is not installed yet — install it on the Core tab, then pair on the Pairing tab."
	daemonEngineInactiveText    = "The daemon engine is not active yet: it switches on by itself once the service answers and the launcher is paired (stop the VPN first if it is running)."
)

type localServiceSource struct {
	ac       *core.AppController
	win      fyne.Window
	onPaired func()

	// Состояние — только в UI-потоке (refresh/apply зовутся через fyne.Do).
	st        core.DaemonUIStatus
	loaded    bool
	downSince time.Time
	attempts  int
	onUpdate  func()
	// inFlight — опрос идёт; pending — пока он шёл, попросили ещё один
	// (операция службы завершилась): он стартует сразу после текущего.
	inFlight bool
	pending  bool
	stopped  bool

	stopOnce sync.Once
	stop     chan struct{}
}

// newLocalServiceSource — источник локальной службы. onPaired — колбэк
// успешного сопряжения (панель Local доводит переключение движка).
func newLocalServiceSource(ac *core.AppController, win fyne.Window, onPaired func()) *localServiceSource {
	return &localServiceSource{ac: ac, win: win, onPaired: onPaired, stop: make(chan struct{})}
}

func (s *localServiceSource) Key() string { return "local" }

// localPlatform — платформа этого компьютера.
func localPlatform() core.ServicePlatform {
	return core.ServicePlatform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Init: core.DefaultServiceInit(runtime.GOOS, runtime.GOARCH)}
}

func (s *localServiceSource) Snapshot() serviceSnapshot {
	st := s.st
	snap := serviceSnapshot{
		Loaded:   s.loaded,
		Title:    locale.T("This computer"),
		Platform: localPlatform(),
		LiveLog:  true,
		Local:    &localServiceExtras{},
	}
	var passport core.ServicePassport
	if !st.PassportSeenAt.IsZero() {
		p := st.Passport
		tls := p.TLS
		passport = core.ServicePassport{Version: p.Version, StateDir: p.StateDir, Executable: p.Executable,
			LogPath: p.LogPath, Listen: p.Listen, TLS: &tls}
		snap.PassportAt = st.PassportSeenAt
	}
	snap.Paths = core.MergeServicePaths(passport, s.ac.DaemonServicePaths())
	if !s.loaded {
		return snap
	}

	// Plain-демон (tls:false) не «сопряжён» (нет пина), но канал работает по
	// Bearer-секрету — для окна это та же рабочая пара.
	snap.Paired = st.Paired || st.Reachable
	snap.Connected = snap.Paired
	snap.Reachable = st.Reachable
	if !st.Reachable {
		snap.Err = st.ReachErr
		snap.Reach = core.ClassifyDaemonReachError(st.ReachErr)
		snap.DownSince = s.downSince
		snap.Attempts = s.attempts
	}
	snap.CoreStatus = st.CoreStatus
	snap.InterruptedApply = st.InterruptedApply
	snap.Running = passport.Version
	if snap.Running == "" {
		snap.Running = st.DaemonVersion
	}
	snap.PassportLive = st.Reachable && !st.PassportCached && !st.PassportSeenAt.IsZero()
	if snap.PassportLive {
		snap.Uptime = time.Duration(st.Passport.UptimeSeconds) * time.Second
	}

	c := st.Service
	l := snap.Local
	l.ServiceInstalled = st.ServiceInstalled
	// Службы нет вовсе — тоже «Install or update service»: окно открывается
	// на Core (раньше — вкладка Install), а не на Pairing.
	l.NeedsInstall = c.NeedsInstall() || c.State == core.DaemonServiceNotInstalled
	l.NeedsBootstrap = c.NeedsBootstrap()
	l.InstallSupported = c.InstallSupported()
	l.CoreTooOld = c.State == core.DaemonServiceCoreTooOld
	l.LauncherVersion = c.LauncherVersion
	if !l.InstallSupported && !l.CoreTooOld {
		// CoreTooOld несёт ту же подсказку в плашке шапки.
		l.CoreHint = core.DaemonServiceCoreHint(c.LauncherVersion)
	}

	// Строка о службе в шапке: плашка SPEC 136 §6, ядро без lxd, движок ещё
	// не переключился (выбор daemon — намерение, а не состояние).
	var notes []string
	note, danger := daemonServiceNoticeText(c)
	if note != "" {
		notes = append(notes, note)
	}
	if !st.CoreSupportsLxd {
		notes = append(notes, locale.T(daemonNoLxdText))
		danger = true
	}
	switch {
	case c.State == core.DaemonServiceNotInstalled:
		notes = append(notes, locale.T(daemonNotInstalledText))
	case s.ac.BackendMode() != core.BackendDaemon:
		notes = append(notes, locale.T(daemonEngineInactiveText))
	}
	snap.ServiceNote = strings.Join(notes, "\n")
	snap.ServiceNoteDanger = danger
	return snap
}

func (s *localServiceSource) Poll(onUpdate func()) {
	s.onUpdate = onUpdate
	s.refresh()
	go func() {
		t := time.NewTicker(serviceLocalRefresh)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				fyne.Do(s.refresh)
			}
		}
	}()
}

func (s *localServiceSource) StopPoll() {
	s.stopOnce.Do(func() {
		s.stopped = true
		close(s.stop)
	})
}

// refresh — внеочередной опрос (UI-поток). DaemonStatusSnapshot ходит по
// сети — в горутине; второй опрос поверх идущего не стартует.
func (s *localServiceSource) refresh() {
	if s.stopped {
		return
	}
	if s.inFlight {
		s.pending = true
		return
	}
	s.inFlight = true
	go func() {
		st := s.ac.DaemonStatusSnapshot()
		fyne.Do(func() {
			s.inFlight = false
			s.apply(st)
			if s.pending {
				s.pending = false
				s.refresh()
			}
		})
	}()
}

// apply кладёт снимок в кэш и считает серию отказов (UI-поток).
func (s *localServiceSource) apply(st core.DaemonUIStatus) {
	if s.stopped {
		return
	}
	s.st, s.loaded = st, true
	if (st.Paired || st.Reachable) && !st.Reachable {
		if s.attempts == 0 {
			s.downSince = time.Now()
		}
		s.attempts++
	} else {
		s.attempts, s.downSince = 0, time.Time{}
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
}

func (s *localServiceSource) SSH() (services.SSHTarget, bool) { return services.SSHTarget{}, false }

func (s *localServiceSource) SetInit(core.ServiceInit) error { return nil }

// Pair — сопряжение по приглашению. Адрес несёт само приглашение
// (PairDaemonWithInvite сохраняет его), поэтому addr не нужен. Локально
// вкладка Pairing берёт строку LocalRows().Pair, этот вызов — для полноты
// интерфейса.
func (s *localServiceSource) Pair(invite, _, secret string, done func(error)) {
	go func() {
		err := s.ac.PairDaemonWithInvite(strings.TrimSpace(invite), strings.TrimSpace(secret))
		fyne.Do(func() {
			if err == nil && s.onPaired != nil {
				s.onPaired()
			}
			s.refresh()
			done(err)
		})
	}()
}

func (s *localServiceSource) SetSecret(secret string) error {
	if err := s.ac.SetDaemonSecret(secret); err != nil {
		return err
	}
	s.refresh()
	return nil
}

// OpenLiveLog — окно Logs: его вкладка Core в daemon-режиме читает
// кольцевой буфер SubscribeLog демона.
func (s *localServiceSource) OpenLiveLog(fyne.Window) { OpenLogViewerWindow(s.ac) }

// LocalRows — строки операций локальной службы (бывшая панель Local).
func (s *localServiceSource) LocalRows(win fyne.Window) localServiceRows {
	ac := s.ac
	binDir := ac.FileService.Layout.Data.Bin()
	ops := &daemonOps{ac: ac, win: win, after: func(r core.DaemonRunResult) {
		if r.Paired && s.onPaired != nil {
			s.onPaired()
		}
		s.refresh()
	}}
	var rows localServiceRows

	// `--service=install` поверх существующей службы обновляет root-owned
	// копию и перезапускает службу; первая установка печатает приглашение
	// (Windows — сопрягается сама).
	rows.Install = ops.row(daemonInstallRowLabel, daemonRunAsAdminKey, ac.DaemonInstallCommand, ac.DaemonInstallOrUpdate)
	// NotRunning: определение службы и копия в порядке, менеджер служб её не
	// держит — её загружают (launchd bootstrap / sc.exe start), а не
	// переустанавливают.
	rows.Bootstrap = ops.row(daemonOpRowLabel, daemonStartServiceKey, ac.DaemonBootstrapCommand, ac.DaemonStartService)
	rows.Restart = ops.row(daemonOpRowLabel, daemonRunAsAdminKey, func() (string, error) {
		return ac.DaemonRestartCommand(), nil
	}, ac.DaemonRestartService)
	rows.FreshInvite = ops.row(daemonFreshInviteRowLabel, daemonRunAsAdminKey, func() (string, error) {
		return ac.DaemonRepairCommand(), nil
	}, ac.DaemonFreshInvite)

	// --- Секрет plain-режима ----------------------------------------------
	// Для plain-h2c демона (без TLS) сопряжения не существует — Bearer-секрет
	// и есть весь канал аутентификации. Для mTLS-демона поле не нужно.
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("Bearer secret (only for a daemon without TLS)"))
	secretEntry.SetText(locale.LoadSettings(binDir).DaemonSecret)
	saveSecret := func(text string) {
		if err := s.SetSecret(text); err != nil {
			ShowError(win, err)
		}
	}
	secretEntry.OnSubmitted = saveSecret
	secretSave := widget.NewButton(locale.T("Save"), func() { saveSecret(secretEntry.Text) })
	secretHelp := widget.NewButton("?", func() {
		showCommandHelpDialog(ac, win,
			locale.T("Bearer secret (only for a daemon without TLS)"),
			locale.T(secretHelpText),
			ac.DaemonShowSecretCommand())
	})
	secretHelp.Importance = widget.LowImportance
	rows.Secret = container.NewBorder(nil, nil, nil, container.NewHBox(secretSave, secretHelp), secretEntry)

	// --- Приглашение и Pair -----------------------------------------------
	inviteEntry := widget.NewEntry()
	inviteEntry.SetPlaceHolder(locale.T("address#fingerprint#code"))
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
				if s.onPaired != nil {
					s.onPaired()
				}
				s.refresh()
			})
		}()
	})
	pairBtn.Importance = widget.HighImportance
	pairHelp := widget.NewButton("?", func() {
		showCommandHelpDialog(ac, win,
			locale.T("Pair"),
			locale.T(daemonPairHelpText),
			ac.DaemonRepairCommand())
	})
	pairHelp.Importance = widget.LowImportance
	rows.Pair = container.NewBorder(nil, nil, nil, container.NewHBox(pairBtn, pairHelp), inviteEntry)

	// --- Адрес демона -------------------------------------------------------
	addressEntry := widget.NewEntry()
	addressEntry.SetText(locale.LoadSettings(binDir).DaemonAddress)
	addressEntry.SetPlaceHolder("127.0.0.1:19091")
	saveAddress := func(text string) {
		if err := ac.SetDaemonAddress(text); err != nil {
			ShowError(win, err)
			return
		}
		s.refresh()
	}
	addressEntry.OnSubmitted = saveAddress
	addressSave := widget.NewButton(locale.T("Save"), func() { saveAddress(addressEntry.Text) })
	rows.Address = container.NewBorder(nil, nil, nil, addressSave, addressEntry)

	rows.Uninstall = s.uninstallPane(win, ops)
	return rows
}

// uninstallPane — вкладка Uninstall: два последовательных шага.
//  1. Unpair — локальная сторона (пара лаунчера, пин, адрес);
//  2. удаление службы командой, с галкой --purge (по умолчанию ВКЛ: «снести —
//     так снести»; выключают её осознанно, чтобы сохранить state демона для
//     будущей переустановки без пере-сопряжения).
func (s *localServiceSource) uninstallPane(win fyne.Window, ops *daemonOps) fyne.CanvasObject {
	ac := s.ac
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
				s.refresh()
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

	pane := container.NewVBox(
		wrappedLabel("1. Forget the pairing on the launcher side:"), // l10n-key
		unpairBtn,
		wrappedLabel(daemonUninstallStepLabel),
		purgeCheck,
		container.NewBorder(nil, nil, nil, uninstallButtons, uninstallEntry),
	)
	if core.DaemonOpsElevated {
		pane.Add(uninstallResult.object())
	}
	return pane
}
