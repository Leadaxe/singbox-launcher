package ui

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// Окно Service (SPEC 161 §5): раннбук обслуживания демона. Шапка — диагноз
// (платформа, версия ядра, отвечает ли демон, где выполняются команды),
// вкладки — по случаю: Not running, Core, Pairing, Reference (+ Uninstall у
// локальной службы). Тело окна (buildServiceView) встраивает и панель Local
// окна подключения.
//
// Окно обновляется само: источник зовёт onUpdate, окно сравнивает снапшот с
// прежним и перестраивает шапку и содержимое вкладок только при изменении.
// Виджеты с вводом и состоянием (приглашение, секрет, скачивание ядра,
// раскрытые секции, строки локальной службы) создаются один раз — иначе тик
// стирал бы введённое.

// serviceTabScrollMinHeight — минимум прокрутки вкладки. AppTabs берёт
// минимум по всем вкладкам, и высокая Core без константы держала бы высоту
// окна (см. ui/configurator/tabs/scroll_height.go).
const serviceTabScrollMinHeight = 160

const (
	serviceWindowWidth  = 640
	serviceWindowHeight = 560
)

var (
	serviceWindowsMu sync.Mutex
	serviceWindows   = map[string]*serviceWindowEntry{}
)

type serviceWindowEntry struct {
	win  fyne.Window
	view *serviceView
}

// OpenServiceWindow открывает окно Service источника src на вкладке tab
// (serviceTabAuto — по диагнозу). Одно окно на src.Key(): повторный вызов
// поднимает открытое и переключает вкладку, если она задана явно.
func OpenServiceWindow(ac *core.AppController, src serviceSource, tab serviceTab) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	key := src.Key()
	serviceWindowsMu.Lock()
	if e, ok := serviceWindows[key]; ok {
		serviceWindowsMu.Unlock()
		if tab != serviceTabAuto {
			e.view.selectTab(tab)
		}
		e.win.Show()
		e.win.RequestFocus()
		return
	}
	serviceWindowsMu.Unlock()

	win := ac.UIService.Application.NewWindow(locale.Tf("Service — %s", src.Snapshot().Title))
	view := newServiceView(ac, win, src, tab)
	win.SetContent(fynetooltip.AddWindowToolTipLayer(container.NewPadded(view.root), win.Canvas()))
	win.SetOnClosed(func() {
		view.dispose()
		fynetooltip.DestroyWindowToolTipLayer(win.Canvas())
		serviceWindowsMu.Lock()
		delete(serviceWindows, key)
		serviceWindowsMu.Unlock()
	})
	win.Resize(fyne.NewSize(serviceWindowWidth, serviceWindowHeight))
	fynewidget.CenterOnScreen(win)

	serviceWindowsMu.Lock()
	serviceWindows[key] = &serviceWindowEntry{win: win, view: view}
	serviceWindowsMu.Unlock()
	win.Show()
}

// CloseServiceWindow закрывает окно Service по ключу (машину удалили).
// UI-поток.
func CloseServiceWindow(key string) {
	serviceWindowsMu.Lock()
	e, ok := serviceWindows[key]
	serviceWindowsMu.Unlock()
	if ok {
		e.win.Close()
	}
}

// buildServiceView — тело окна Service для встраивания (панель Local окна
// подключения). dispose останавливает опрос источника; звать при закрытии
// окна-хозяина.
func buildServiceView(ac *core.AppController, win fyne.Window, src serviceSource, tab serviceTab) (fyne.CanvasObject, func()) {
	v := newServiceView(ac, win, src, tab)
	return v.root, v.dispose
}

// serviceView — состояние одного окна (или встроенного вида) Service.
type serviceView struct {
	ac  *core.AppController
	win fyne.Window
	src serviceSource

	snap     serviceSnapshot
	rendered bool
	disposed bool
	// wantTab — вкладка из вызова; autoSelected — стартовая вкладка уже
	// выбрана (дальше вкладку выбирает пользователь, тик её не угоняет).
	wantTab      serviceTab
	autoSelected bool

	root    *fyne.Container
	header  *fyne.Container
	tabs    *container.AppTabs
	order   []serviceTab
	items   map[serviceTab]*container.TabItem
	scrolls map[serviceTab]*container.Scroll

	// Создаются один раз и переживают перестройку вкладок.
	rows       localServiceRows
	initSelect *widget.Select
	sections   map[string]*widget.Accordion
	pairForm   *servicePairForm
	secret     *widget.Entry
	download   *targetCoreDownload
}

func newServiceView(ac *core.AppController, win fyne.Window, src serviceSource, tab serviceTab) *serviceView {
	v := &serviceView{
		ac: ac, win: win, src: src, wantTab: tab,
		items:    make(map[serviceTab]*container.TabItem),
		scrolls:  make(map[serviceTab]*container.Scroll),
		sections: make(map[string]*widget.Accordion),
	}
	v.rows = src.LocalRows(win)
	v.order = []serviceTab{serviceTabNotRunning, serviceTabCore, serviceTabPairing, serviceTabReference}
	if v.rows.Uninstall != nil {
		v.order = append(v.order, serviceTabUninstall)
	}
	v.tabs = container.NewAppTabs()
	for _, t := range v.order {
		scroll := container.NewVScroll(widget.NewLabel(""))
		scroll.SetMinSize(fyne.NewSize(0, serviceTabScrollMinHeight))
		if t == serviceTabUninstall {
			// Uninstall — готовая панель локального источника, не зависит от
			// снапшота и не перестраивается.
			scroll.Content = container.NewPadded(v.rows.Uninstall)
		}
		item := container.NewTabItem(serviceTabName(t), scroll)
		v.items[t], v.scrolls[t] = item, scroll
		v.tabs.Append(item)
	}
	v.initSelect = widget.NewSelect(serviceInitOptions(), nil)
	v.header = container.NewVBox()
	v.root = container.NewBorder(v.header, nil, nil, nil, v.tabs)

	v.update(true)
	src.Poll(func() { v.update(false) })
	return v
}

func (v *serviceView) dispose() {
	if v.disposed {
		return
	}
	v.disposed = true
	v.src.StopPoll()
}

// update перечитывает снапшот и перестраивает окно, если он изменился.
func (v *serviceView) update(force bool) {
	if v.disposed {
		return
	}
	s := v.src.Snapshot()
	if !force && v.rendered && serviceSnapshotEqual(v.snap, s) {
		return
	}
	v.snap = s
	v.render()
}

// render перестраивает шапку и вкладки по v.snap, сохраняя выбранную вкладку
// и позицию прокрутки.
func (v *serviceView) render() {
	if v.disposed {
		return
	}
	v.rendered = true
	s := v.snap
	v.renderHeader()

	ssh, remote := v.src.SSH()
	in := core.ServiceRecipeInput{Platform: s.Platform, Paths: s.Paths, Running: s.Running}
	if remote {
		in.SSH = ssh
	}
	if v.download != nil && v.download.matches(s.Platform.GOOS, s.Platform.GOARCH) {
		in.Uploaded = v.download.path()
	}
	c := &serviceTabCtx{
		v: v, s: s, rec: core.BuildServiceRecipes(in),
		run: &serviceRunner{win: v.win, remote: remote, ssh: ssh, init: s.Platform.Init, title: s.Title},
	}
	builders := map[serviceTab]func(*serviceTabCtx) fyne.CanvasObject{
		serviceTabNotRunning: buildNotRunningTab,
		serviceTabCore:       buildCoreTab,
		serviceTabPairing:    buildPairingTab,
		serviceTabReference:  buildReferenceTab,
	}
	glyphs, first := serviceDiagnosis(s)
	for _, t := range v.order {
		if build, ok := builders[t]; ok {
			scroll := v.scrolls[t]
			scroll.Content = container.NewPadded(build(c))
			scroll.Refresh()
		}
		label := serviceTabName(t)
		if g := glyphs[t]; g != "" {
			label = g + " " + label
		}
		v.items[t].Text = label
	}
	v.tabs.Refresh()

	if !v.autoSelected && s.Loaded {
		v.autoSelected = true
		t := v.wantTab
		if t == serviceTabAuto {
			t = first
		}
		v.selectTab(t)
	}
}

// selectTab переключает вкладку (serviceTabAuto — по диагнозу).
func (v *serviceView) selectTab(t serviceTab) {
	if t == serviceTabAuto {
		_, t = serviceDiagnosis(v.snap)
	}
	if item, ok := v.items[t]; ok {
		v.tabs.Select(item)
	}
}

// renderHeader — шапка (SPEC 161 §5.1).
func (v *serviceView) renderHeader() {
	s := v.snap
	objs := make([]fyne.CanvasObject, 0, 8)

	platformLine := serviceNoteLabel(locale.Tf("%s · %s/%s · service %s (%s)", serviceOSLabel(s.Platform),
		s.Platform.GOOS, s.Platform.GOARCH, core.ServiceName, string(s.Platform.Init)), widget.MediumImportance)
	left := container.NewVBox(platformLine)
	if s.InitChoice {
		v.initSelect.OnChanged = nil
		v.initSelect.SetSelected(serviceInitOption(s.Platform.Init))
		v.initSelect.OnChanged = v.onInitChanged
		left.Add(container.NewHBox(widget.NewLabel(locale.T("Init system:")), v.initSelect))
	}
	guides := serviceGuidesRow(v.win, locale.T("Guides:"), serviceHeaderGuides(s.Platform)...)
	objs = append(objs, container.NewBorder(nil, nil, nil, guides, left))

	objs = append(objs, serviceCoreHeaderLine(s))
	if line, importance := serviceStateLine(s); line != "" {
		objs = append(objs, serviceNoteLabel(line, importance))
	}
	if s.InterruptedApply {
		objs = append(objs, serviceNoteLabel(locale.T("The last deploy did not start: the core rolled back to the last-good config."), widget.WarningImportance))
	}
	if s.ServiceNote != "" {
		importance := widget.WarningImportance
		if s.ServiceNoteDanger {
			importance = widget.DangerImportance
		}
		objs = append(objs, serviceNoteLabel(s.ServiceNote, importance))
	}
	if ssh, remote := v.src.SSH(); remote {
		objs = append(objs, serviceNoteLabel(locale.Tf("Commands run via  ssh %s", ssh.String()), widget.MediumImportance))
	} else {
		objs = append(objs, serviceNoteLabel(locale.T("Commands run on this computer"), widget.MediumImportance))
	}
	v.header.Objects = objs
	v.header.Refresh()
}

func (v *serviceView) onInitChanged(option string) {
	init := serviceInitFromOption(option)
	if init == v.snap.Platform.Init {
		return
	}
	if err := v.src.SetInit(init); err != nil {
		debuglog.WarnLog("service window: set init %q: %v", init, err)
		ShowError(v.win, err)
	}
	v.update(true)
}

// serviceCoreHeaderLine — «Core <running> (required <req> ⚠)».
func serviceCoreHeaderLine(s serviceSnapshot) fyne.CanvasObject {
	req := constants.RequiredCoreVersion
	switch {
	case s.Running == "":
		return serviceNoteLabel(locale.Tf("Core version unknown (required %s)", req), widget.MediumImportance)
	case serviceCoreVerdict(s) == core.CoreVersionOlder:
		return serviceNoteLabel(locale.Tf("Core %s (required %s ⚠)", s.Running, req), widget.WarningImportance)
	case serviceCoreVerdict(s) == core.CoreVersionUnknown:
		return serviceNoteLabel(locale.Tf("Core %s (required %s — cannot compare)", s.Running, req), widget.MediumImportance)
	}
	return serviceNoteLabel(locale.Tf("Core %s (required %s)", s.Running, req), widget.MediumImportance)
}

// serviceStateLine — отвечает ли демон: зелёная, красная или нейтральная
// строка шапки.
func serviceStateLine(s serviceSnapshot) (string, widget.Importance) {
	switch {
	case !s.Loaded:
		return locale.T("Checking the daemon…"), widget.MediumImportance
	case !s.Connected && s.Local == nil:
		return locale.T("Not connected — press Connect on the machine's row to check the daemon. The commands below use the last known paths."), widget.MediumImportance
	case !s.Connected:
		return "", widget.MediumImportance
	case s.Reachable:
		line := locale.T("✅ Answers")
		if up := serviceAgeLabel(s.Uptime); up != "" {
			line += " · " + locale.Tf("uptime %s", up)
		}
		if s.CoreStatus != "" {
			line += " · " + locale.Tf("core %s", s.CoreStatus)
		}
		return line, widget.SuccessImportance
	}
	if servicePairingBroken(s.Reach) {
		return locale.Tf("✖ The daemon refuses this launcher: %s", s.Err), widget.DangerImportance
	}
	line := locale.Tf("✖ Not answering: %s", s.Err)
	if since := serviceSinceLabel(s.DownSince); since != "" {
		line = locale.Tf("✖ Not answering for %s: %s", since, s.Err)
	}
	if s.Attempts > 0 {
		line += " " + locale.Tf("(%d failed checks in a row)", s.Attempts)
	}
	return line, widget.DangerImportance
}

func serviceTabName(t serviceTab) string {
	switch t {
	case serviceTabNotRunning:
		return locale.T("Not running")
	case serviceTabCore:
		return locale.T("Core")
	case serviceTabPairing:
		return locale.T("Pairing")
	case serviceTabReference:
		return locale.T("Reference")
	case serviceTabUninstall:
		return locale.T("Uninstall")
	}
	return ""
}

// Варианты выбора init-системы Linux-машины.
const (
	serviceInitOptionProcd   = "OpenWrt (procd)" // l10n-exempt: product name
	serviceInitOptionSystemd = "systemd"         // l10n-exempt: product name
)

func serviceInitOptions() []string {
	return []string{serviceInitOptionProcd, serviceInitOptionSystemd}
}

func serviceInitOption(init core.ServiceInit) string {
	if init == core.ServiceInitProcd {
		return serviceInitOptionProcd
	}
	return serviceInitOptionSystemd
}

func serviceInitFromOption(option string) core.ServiceInit {
	if option == serviceInitOptionProcd {
		return core.ServiceInitProcd
	}
	return core.ServiceInitSystemd
}

// section — свёрнутая секция (опасное и редкое). Accordion создаётся один
// раз на key: раскрытие переживает перестройку вкладки, меняется только
// содержимое.
func (v *serviceView) section(key, title string, content fyne.CanvasObject) fyne.CanvasObject {
	if acc, ok := v.sections[key]; ok {
		acc.Items[0].Title = title
		acc.Items[0].Detail = content
		acc.Refresh()
		return acc
	}
	acc := widget.NewAccordion(widget.NewAccordionItem(title, content))
	v.sections[key] = acc
	return acc
}

// coreDownload — шаг скачивания ядра под платформу машины (один на окно;
// пересоздаётся, если платформу машины поменяли).
func (v *serviceView) coreDownload(goos, goarch string) *targetCoreDownload {
	if v.download == nil || !v.download.matches(goos, goarch) {
		v.download = newTargetCoreDownload(v.ac, v.win, goos, goarch, v.render)
	}
	return v.download
}

// servicePairForm — поля сопряжения машины (вкладка Pairing): приглашение,
// необязательные адрес и секрет, кнопка и статус. Живёт в окне.
type servicePairForm struct {
	invite, addr, secret *widget.Entry
	btn                  *widget.Button
	status               *widget.Label
	advanced             *widget.Accordion
}

func (v *serviceView) pairFormFor() *servicePairForm {
	if v.pairForm != nil {
		return v.pairForm
	}
	f := &servicePairForm{}
	f.invite = widget.NewEntry()
	f.invite.SetPlaceHolder(locale.T("address#fingerprint#code"))
	f.addr = widget.NewEntry()
	f.addr.SetPlaceHolder(locale.T("host:port — leave empty to keep the current address"))
	f.secret = widget.NewPasswordEntry()
	f.secret.SetPlaceHolder(locale.T("only for a plain-h2c daemon; leave empty for mTLS"))
	f.status = widget.NewLabel("")
	f.status.Wrapping = fyne.TextWrapWord
	f.status.Hide()
	f.advanced = widget.NewAccordion(widget.NewAccordionItem(locale.T("Advanced (address, secret)"),
		widget.NewForm(
			widget.NewFormItem(locale.T("Address"), f.addr),
			widget.NewFormItem(locale.T("Secret"), f.secret),
		)))
	f.btn = widget.NewButton(locale.T("Pair"), v.onPair)
	f.btn.Importance = widget.HighImportance
	v.pairForm = f
	return f
}

func (v *serviceView) onPair() {
	f := v.pairForm
	invite := strings.TrimSpace(f.invite.Text)
	if invite == "" {
		f.status.SetText(locale.T("Paste the invite printed by the daemon."))
		f.status.Show()
		return
	}
	// Re-pair перевыпускает клиентский ключ — прежний мандат становится
	// мусором на машине; отменить нечем, поэтому спрашиваем.
	ShowConfirm(v.win, locale.T("Re-pair machine"), locale.Tf(repairConfirmBodyText, v.snap.Title), func(ok bool) {
		if !ok {
			return
		}
		f.btn.Disable()
		f.status.SetText(locale.T("Re-pairing…"))
		f.status.Show()
		v.src.Pair(invite, f.addr.Text, f.secret.Text, func(err error) {
			f.btn.Enable()
			if err != nil {
				f.status.SetText(locale.Tf("Pairing failed: %v", err))
				return
			}
			f.invite.SetText("")
			f.status.SetText(locale.T("Paired. A new client key was issued; connect again."))
			v.update(true)
		})
	})
}

// secretEntryFor — поле Bearer-секрета plain-режима машины.
func (v *serviceView) secretEntryFor() *widget.Entry {
	if v.secret == nil {
		v.secret = widget.NewPasswordEntry()
		v.secret.SetPlaceHolder(locale.T("Bearer secret (only for a daemon without TLS)"))
	}
	return v.secret
}
