package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/locale"
)

// Вкладки окна Service (SPEC 161 §5.2–§5.5): общие для локальной службы и
// машины. Команды — из core.BuildServiceRecipes; у локальной службы строки
// операций (install, bootstrap, restart, приглашение, Pair, секрет, адрес)
// приходят готовыми из localServiceRows и встают на место рецепта.

const (
	serviceAnswersText        = "✅ The daemon answers — nothing to fix here."
	serviceAnswersUptimeText  = "✅ The daemon answers, uptime %s — nothing to fix here."
	serviceUnknownStateText   = "The daemon has not been checked yet: connect to the machine to see whether it answers. The steps below use the last known paths."
	serviceLikelyDownText     = "Likely: the service is stopped or crash-looping."
	serviceLikelyPairingText  = "Likely: a pairing problem, not the service — the daemon answers but refuses this launcher."
	serviceRestartHintText    = "If the header turns green, you are done."
	serviceLogHintText        = "\"refusing to run\" → Core tab · \"bind: address in use\" → Reference tab"
	serviceNothingHelpsText   = "Nothing helps? Reinstall the core on the Core tab."
	serviceUploadOpenWrtText  = "Upload it (streamed over ssh — scp does not work on OpenWrt)"
	serviceCheckHintText      = "Expect with_lxd among the build tags, the same sha256 as in step 1 and no error from check."
	serviceSwapHintText       = "The current core is kept as %s."
	serviceHeaderAfterText    = "The header should now say %s · started."
	serviceRollbackDangerText = "Puts the backed-up core back and restarts the service."
	serviceScratchHintText    = "Save this as %s on the machine, then enable and start it:"
	serviceSysupgradeText     = "Add %s to /etc/sysupgrade.conf, or a firmware upgrade will drop it."
	serviceRemoveDangerText   = "Stops the daemon and removes its service definition. The state (keys, clients, last-good) stays."
	serviceRemoveStateText    = "Also wipe the state — keys, clients and last-good are lost, and every client has to pair again:"
	serviceDesktopCoreText    = "On %s the service is installed on the machine itself: run the core's `lxd --service=install` there as administrator — it installs or updates the service in place."
	serviceLocalCoreText      = "Launcher core %s (required %s). The service runs its own copy of it; download a newer core on the Core tab of the dashboard, then install or update the service."
	serviceRepairWhenText     = "Re-pair when the error says \"certificate changed\" or \"not paired\", the service was reinstalled or its state dir was wiped."
	serviceRevokeDangerText   = "The client loses access at once. Replace <name> with a name from the list above."
	serviceSecretHintText     = "Only for a daemon with tls:false in daemon.json: the Bearer secret is the whole authentication."
	serviceDaemonJSONText     = "daemon.json — edit it, then restart the service (never reinstall)"
	servicePathsLiveText      = "Paths — reported by the daemon just now"
	servicePathsAgoText       = "Paths — reported by the daemon %s ago"
	servicePathsDefaultText   = "Paths — defaults: the daemon has not reported its paths yet"
)

// serviceTabCtx — всё, из чего строится вкладка на один рендер.
type serviceTabCtx struct {
	v   *serviceView
	s   serviceSnapshot
	rec core.ServiceRecipes
	run *serviceRunner
}

// step — строка шага рецепта id; n > 0 — номер перед заголовком. nil —
// шага нет у этой init-системы.
func (c *serviceTabCtx) step(n int, title, hint, id string, danger bool) fyne.CanvasObject {
	st, ok := c.rec.Steps[id]
	if !ok {
		return nil
	}
	return serviceStepRow(c.run, serviceNumbered(n, title), hint, st, danger)
}

// serviceNumbered — «1  Заголовок».
func serviceNumbered(n int, title string) string {
	if n <= 0 {
		return title
	}
	return fmt.Sprintf("%d  %s", n, title)
}

// serviceVBox — VBox без nil-объектов (шага может не быть у init-системы).
func serviceVBox(objs ...fyne.CanvasObject) *fyne.Container {
	box := container.NewVBox()
	serviceAdd(box, objs...)
	return box
}

// serviceAdd — добавить в box объекты, пропуская nil.
func serviceAdd(box *fyne.Container, objs ...fyne.CanvasObject) {
	for _, o := range objs {
		if o != nil {
			box.Add(o)
		}
	}
}

// serviceTitled — заголовок над готовой строкой (строки localServiceRows).
func serviceTitled(title string, obj fyne.CanvasObject) fyne.CanvasObject {
	if obj == nil {
		return nil
	}
	return container.NewVBox(serviceStepTitle(title), obj)
}

// serviceGoTo — кнопка перехода на другую вкладку окна.
func (c *serviceTabCtx) goTo(label string, t serviceTab) fyne.CanvasObject {
	btn := widget.NewButton(label, func() { c.v.selectTab(t) })
	btn.Importance = widget.LowImportance
	return container.NewHBox(btn)
}

// ---------------------------------------------------------------- Not running

func buildNotRunningTab(c *serviceTabCtx) fyne.CanvasObject {
	s := c.s
	rows := c.v.rows
	needsBootstrap := s.Local != nil && s.Local.NeedsBootstrap
	restart := func(n int, title, hint string) fyne.CanvasObject {
		if rows.Restart != nil {
			return serviceTitled(serviceNumbered(n, title), rows.Restart)
		}
		return c.step(n, title, hint, core.ServiceStepRestart, true)
	}
	guides := serviceGuidesRow(c.v.win, locale.T("Guide:"), guideTroubleshooting, guideLxdDiagnose)

	if s.Loaded && s.Connected && s.Reachable && !needsBootstrap {
		text := locale.T(serviceAnswersText)
		if up := serviceAgeLabel(s.Uptime); up != "" {
			text = locale.Tf(serviceAnswersUptimeText, up)
		}
		return serviceVBox(
			serviceNoteLabel(text, widget.SuccessImportance),
			restart(0, locale.T("Restart it"), ""),
			c.step(0, locale.T("Read the log"), "", core.ServiceStepLogTail, false),
			guides,
		)
	}

	var lead fyne.CanvasObject
	switch {
	case !s.Connected && s.Local == nil:
		lead = serviceNoteLabel(locale.T(serviceUnknownStateText), widget.MediumImportance)
	case s.Connected && !s.Reachable && servicePairingBroken(s.Reach):
		lead = serviceVBox(serviceNoteLabel(locale.T(serviceLikelyPairingText), widget.WarningImportance),
			c.goTo(locale.T("Open the Pairing tab"), serviceTabPairing))
	case s.Connected && !s.Reachable:
		lead = serviceNoteLabel(locale.T(serviceLikelyDownText), widget.MediumImportance)
	}

	n := 1
	next := func() int { n++; return n - 1 }
	box := serviceVBox(lead, c.step(next(), locale.T("Is it running?"), "", core.ServiceStepStatus, false))
	if needsBootstrap && rows.Bootstrap != nil {
		serviceAdd(box, serviceTitled(serviceNumbered(next(), locale.T("Load the service")), rows.Bootstrap))
	}
	serviceAdd(box, restart(next(), locale.T("Restart it"), locale.T(serviceRestartHintText)))
	serviceAdd(box, c.step(next(), locale.T("Still down? Read why"), locale.T(serviceLogHintText), core.ServiceStepLogTail, false))
	serviceAdd(box, c.step(next(), locale.T("The last config broke it? Boot the last working one once"), "", core.ServiceStepLastGood, true))
	serviceAdd(box, serviceStepTitle(serviceNumbered(next(), locale.T(serviceNothingHelpsText))))
	serviceAdd(box, c.goTo(locale.T("Open the Core tab"), serviceTabCore))
	serviceAdd(box, guides)
	return box
}

// ----------------------------------------------------------------------- Core

func buildCoreTab(c *serviceTabCtx) fyne.CanvasObject {
	s := c.s
	req := constants.RequiredCoreVersion
	verdict := serviceCoreVerdict(s)
	var lead *widget.Label
	switch {
	case verdict == core.CoreVersionOlder:
		lead = serviceNoteLabel(locale.Tf("Running %s → required %s", s.Running, req), widget.WarningImportance)
	case verdict == core.CoreVersionCurrent:
		lead = serviceNoteLabel(locale.Tf("✅ Core %s is current", s.Running), widget.SuccessImportance)
	case verdict == core.CoreVersionNewer:
		lead = serviceNoteLabel(locale.Tf("✅ Core %s is newer than required (%s)", s.Running, req), widget.SuccessImportance)
	case s.Running == "":
		lead = serviceNoteLabel(locale.Tf("The daemon has not reported its core version yet (required %s).", req), widget.MediumImportance)
	default:
		lead = serviceNoteLabel(locale.Tf("Core %s cannot be compared with the required %s.", s.Running, req), widget.MediumImportance)
	}
	box := serviceVBox(lead)
	switch {
	case s.Local != nil:
		buildLocalCore(c, box)
	case s.Platform.GOOS == "linux":
		buildLinuxCore(c, box, verdict == core.CoreVersionCurrent || verdict == core.CoreVersionNewer)
	default:
		serviceAdd(box, serviceNoteLabel(locale.Tf(serviceDesktopCoreText, serviceOSLabel(s.Platform)), widget.MediumImportance))
	}
	serviceAdd(box, serviceGuidesRow(c.v.win, locale.T("Guide:"), serviceCoreGuides(s.Platform)...))
	return box
}

// buildLocalCore — локальная служба: ядро лаунчера и «Install or update
// service» (копия ставится тем же install, отката нет).
func buildLocalCore(c *serviceTabCtx, box *fyne.Container) {
	l := c.s.Local
	serviceAdd(box, serviceNoteLabel(locale.Tf(serviceLocalCoreText, l.LauncherVersion, constants.RequiredCoreVersion), widget.MediumImportance))
	if l.CoreHint != "" {
		serviceAdd(box, serviceNoteLabel(l.CoreHint, widget.WarningImportance))
	}
	if c.v.rows.Install != nil {
		serviceAdd(box, serviceTitled(locale.T("Install or update the service"), c.v.rows.Install))
	}
	if c.v.rows.Uninstall != nil {
		serviceAdd(box, c.goTo(locale.T("Remove the service → Uninstall tab"), serviceTabUninstall))
	}
}

// buildLinuxCore — замена ядра на Linux-машине: скачать здесь → залить по
// ssh → проверить новым бинарём → бэкап, замена, рестарт; откат, установка с
// нуля и удаление — свёрнуты. current — версия в порядке: рецепт целиком
// уходит в свёрнутую «Reinstall the same version».
func buildLinuxCore(c *serviceTabCtx, box *fyne.Container, current bool) {
	s := c.s
	procd := s.Platform.Init == core.ServiceInitProcd
	upload := locale.T("Upload it (streamed over ssh)")
	if procd {
		upload = locale.T(serviceUploadOpenWrtText)
	}
	recipe := serviceVBox(
		serviceStepTitle(serviceNumbered(1, locale.Tf("Get the core for %s/%s", s.Platform.GOOS, s.Platform.GOARCH))),
		c.v.coreDownload(s.Platform.GOOS, s.Platform.GOARCH).object(),
		c.step(2, upload, "", core.ServiceStepCoreUpload, false),
		c.step(3, locale.T("Check before swapping: build tags and the current config"), locale.T(serviceCheckHintText), core.ServiceStepCoreCheck, false),
		c.step(4, locale.T("Back up, swap, restart"), locale.Tf(serviceSwapHintText, c.rec.BackupPath), core.ServiceStepCoreSwap, true),
		serviceStepTitle(serviceNumbered(5, locale.Tf(serviceHeaderAfterText, constants.RequiredCoreVersion))),
	)
	if current {
		serviceAdd(box, c.v.section("core_reinstall", locale.T("Reinstall the same version"), recipe))
	} else {
		serviceAdd(box, recipe)
	}

	serviceAdd(box, c.v.section("core_rollback", locale.T("Roll back if the new core misbehaves"), serviceVBox(
		serviceNoteLabel(locale.T(serviceRollbackDangerText), widget.DangerImportance),
		c.step(0, "", "", core.ServiceStepCoreRollback, true),
	)))

	script := widget.NewMultiLineEntry()
	script.Wrapping = fyne.TextWrapOff
	script.SetText(c.rec.ScratchScript)
	script.SetMinRowsVisible(strings.Count(c.rec.ScratchScript, "\n") + 1)
	scriptCopy := NewCopyButton("Copy the command", func() (string, bool) { return script.Text, script.Text != "" })
	scratch := serviceVBox(
		serviceNoteLabel(locale.Tf(serviceScratchHintText, c.rec.ScratchPath), widget.MediumImportance),
		container.NewBorder(nil, nil, nil, container.NewVBox(scriptCopy), script),
		c.step(0, locale.T("Enable and start it"), "", core.ServiceStepScratchEnable, false),
	)
	if procd {
		scratch.Add(serviceNoteLabel(locale.Tf(serviceSysupgradeText, c.rec.ScratchPath), widget.WarningImportance))
	}
	serviceAdd(box, c.v.section("core_scratch", locale.T("Install from scratch (no service yet)"), scratch))

	serviceAdd(box, c.v.section("core_remove", locale.T("Remove the service"), serviceVBox(
		serviceNoteLabel(locale.T(serviceRemoveDangerText), widget.DangerImportance),
		c.step(0, "", "", core.ServiceStepRemoveService, true),
		serviceNoteLabel(locale.T(serviceRemoveStateText), widget.DangerImportance),
		c.step(0, "", "", core.ServiceStepRemoveState, true),
	)))
}

// -------------------------------------------------------------------- Pairing

func buildPairingTab(c *serviceTabCtx) fyne.CanvasObject {
	s := c.s
	rows := c.v.rows
	box := serviceVBox(servicePairingStatus(s), serviceNoteLabel(locale.T(serviceRepairWhenText), widget.MediumImportance))

	if rows.FreshInvite != nil {
		serviceAdd(box, serviceTitled(serviceNumbered(1, locale.T("Mint an invite")), rows.FreshInvite))
	} else if st := c.step(1, locale.T("Mint an invite on the machine"), "", core.ServiceStepClientAdd, false); st != nil {
		serviceAdd(box, st)
	}
	if rows.Pair != nil {
		serviceAdd(box, serviceTitled(serviceNumbered(2, locale.T("Paste what it printed")), rows.Pair))
	} else {
		f := c.v.pairFormFor()
		serviceAdd(box, serviceVBox(
			serviceStepTitle(serviceNumbered(2, locale.T("Paste what it printed"))),
			container.NewBorder(nil, nil, nil, f.btn, f.invite),
			f.advanced,
			f.status,
		))
	}
	if rows.Address != nil {
		serviceAdd(box, serviceTitled(locale.T("Daemon address"), rows.Address))
	}

	serviceAdd(box, c.v.section("pair_list", locale.T("Who is trusted"), serviceVBox(
		c.step(0, "", "", core.ServiceStepClientList, false),
	)))
	serviceAdd(box, c.v.section("pair_revoke", locale.T("Revoke a client"), serviceVBox(
		serviceNoteLabel(locale.T(serviceRevokeDangerText), widget.DangerImportance),
		c.step(0, "", "", core.ServiceStepClientRemove, true),
	)))

	var secret fyne.CanvasObject = rows.Secret
	if secret == nil {
		entry := c.v.secretEntryFor()
		save := widget.NewButton(locale.T("Save"), func() {
			if err := c.v.src.SetSecret(entry.Text); err != nil {
				ShowError(c.v.win, err)
				return
			}
			entry.SetText("")
			ShowInfo(c.v.win, locale.T("Bearer secret"), locale.T("Saved. It is used on the next connect."))
		})
		secret = container.NewBorder(nil, nil, nil, save, entry)
	}
	serviceAdd(box, c.v.section("pair_plain", locale.T("Plain mode (tls:false): Bearer secret"), serviceVBox(
		serviceNoteLabel(locale.T(serviceSecretHintText), widget.MediumImportance),
		secret,
	)))
	serviceAdd(box, serviceGuidesRow(c.v.win, locale.T("Guide:"), guideLxdPairing))
	return box
}

// servicePairingStatus — состояние сопряжения по классу ошибки связи.
func servicePairingStatus(s serviceSnapshot) fyne.CanvasObject {
	switch {
	case !s.Loaded:
		return nil
	case !s.Paired:
		return serviceNoteLabel(locale.T("✖ Not paired yet"), widget.DangerImportance)
	case s.Connected && s.Reachable:
		return serviceNoteLabel(locale.T("✅ Paired · certificate ok"), widget.SuccessImportance)
	case !s.Connected:
		return serviceNoteLabel(locale.T("Paired — connect to check the certificate."), widget.MediumImportance)
	}
	switch s.Reach {
	case core.ReachCertChanged:
		return serviceNoteLabel(locale.Tf("✖ The daemon's certificate changed: %s", s.Err), widget.DangerImportance)
	case core.ReachNotTrusted:
		return serviceNoteLabel(locale.Tf("✖ The daemon does not trust this launcher: %s", s.Err), widget.DangerImportance)
	case core.ReachChannelMismatch:
		return serviceNoteLabel(locale.Tf("✖ The TLS mode does not match the daemon's: %s", s.Err), widget.DangerImportance)
	}
	return serviceNoteLabel(locale.T("Paired — the daemon does not answer, so the certificate cannot be checked now."), widget.MediumImportance)
}

// ------------------------------------------------------------------ Reference

func buildReferenceTab(c *serviceTabCtx) fyne.CanvasObject {
	s := c.s
	p := s.Paths
	sep := "/"
	if s.Platform.Init == core.ServiceInitSCM {
		sep = `\`
	}
	stateFile := func(name string) core.ServicePath {
		return core.ServicePath{Value: strings.TrimRight(p.StateDir.Value, sep) + sep + name, Default: p.StateDir.Default}
	}

	pathsTitle := locale.T(servicePathsDefaultText)
	switch {
	case s.PassportLive:
		pathsTitle = locale.T(servicePathsLiveText)
	case !s.PassportAt.IsZero():
		pathsTitle = locale.Tf(servicePathsAgoText, serviceSinceLabel(s.PassportAt))
	}
	paths := widget.NewForm(
		serviceRefPath(locale.T("Core binary"), p.Executable),
		serviceRefPath(locale.T("Service"), p.ServiceFile),
		serviceRefPath(locale.T("State dir"), p.StateDir),
		serviceRefPath(locale.T("Settings"), stateFile("daemon.json")),
		serviceRefPath(locale.T("Last-good"), stateFile("last_good.json")),
		serviceRefPath(locale.T("Log"), p.LogPath),
	)
	if p.ServiceBinary.Value != "" {
		paths.Append(locale.T("Service binary"), serviceRefPathValue(p.ServiceBinary))
	}
	if p.InstallRecord.Value != "" {
		paths.Append(locale.T("Install record"), serviceRefPathValue(p.InstallRecord))
	}

	tls := locale.T("not reported")
	if p.TLS != nil {
		tls = fmt.Sprintf("%t", *p.TLS)
	}
	daemonJSON := widget.NewForm(
		serviceRefValue("listen", locale.T("where it answers"), p.Listen.Value),                            // l10n-exempt: daemon.json key
		serviceRefValue("tls", locale.T("mTLS on/off"), tls),                                               // l10n-exempt: daemon.json key
		serviceRefValue("secret", locale.T("Bearer for tls:false"), locale.T("shown on the machine only")), // l10n-exempt: daemon.json key
		serviceRefValue("log_file", locale.T("log path"), p.LogPath.Value),                                 // l10n-exempt: daemon.json key
		serviceRefValue("log_max_*", locale.T("rotation: size MB / backups / age h"), "1 / 1 / 24"),        // l10n-exempt: daemon.json key
	)

	liveLog := ttwidget.NewButton(locale.T("Live log window"), func() { c.v.src.OpenLiveLog(c.v.win) })
	if !s.LiveLog {
		liveLog.Disable()
		liveLog.SetToolTip(locale.T("Connect first: the log streams over the machine's channel."))
	} else {
		liveLog.SetToolTip(locale.T("The core's log, live, while the daemon answers"))
	}

	return serviceVBox(
		serviceStepTitle(pathsTitle),
		paths,
		widget.NewSeparator(),
		serviceStepTitle(locale.T(serviceDaemonJSONText)),
		daemonJSON,
		c.step(0, locale.T("Show it"), "", core.ServiceStepShowConfig, false),
		widget.NewSeparator(),
		serviceStepTitle(locale.T("Logs")),
		container.NewHBox(liveLog),
		c.step(0, locale.T("Follow the log"), "", core.ServiceStepLogFollow, false),
		c.step(0, locale.T("Who holds the port"), "", core.ServiceStepPortOwner, false),
		serviceGuidesRow(c.v.win, locale.T("Guide:"), guideDaemonRemote, guideLxdConfig),
	)
}

// serviceRefPath — строка пути: значение, пометка default и ⧉.
func serviceRefPath(name string, p core.ServicePath) *widget.FormItem {
	return widget.NewFormItem(name, serviceRefPathValue(p))
}

func serviceRefPathValue(p core.ServicePath) fyne.CanvasObject {
	value := widget.NewLabel(p.Value)
	value.Wrapping = fyne.TextWrapBreak
	right := container.NewHBox()
	if p.Default {
		mark := widget.NewLabel(locale.T("default"))
		mark.Importance = widget.LowImportance
		right.Add(mark)
	}
	right.Add(NewCopyButton("Copy the path", func() (string, bool) { return p.Value, p.Value != "" }))
	return container.NewBorder(nil, nil, nil, right, value)
}

// serviceRefValue — ключ daemon.json: что он значит и значение у машины.
func serviceRefValue(key, meaning, value string) *widget.FormItem {
	text := meaning
	if value != "" {
		text += ":  " + value
	}
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return widget.NewFormItem(key, l)
}
