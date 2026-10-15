// File servers_openconnect_auth.go — диалоги второго фактора и SSO для
// endpoint'ов openconnect под демоном (SPEC 167 §2.5) и статус узла в
// строке списка Servers.
//
// Источник — стрим статуса openconnect (core/openconnect_status.go).
// Диалог всплывает сам один раз на новый challenge.id; повторно — по клику
// на «sign in» в строке узла. Классический режим своего ядра канала не
// имеет: core не отдаёт источник, и здесь ничего не происходит.
package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// openConnectNodeType — type узла openconnect в config.json.
const openConnectNodeType = "openconnect"

// openConnectWrongCodeWarnAfter — после стольких неверных кодов подряд
// диалог предупреждает о новых логинах на шлюзе.
const openConnectWrongCodeWarnAfter = 2

// openConnectRowErrorMaxRunes — сколько текста ошибки помещается в
// подзаголовок строки (полный текст — в Info узла).
const openConnectRowErrorMaxRunes = 28

// openConnectOpenDialog — открытый диалог челленджа.
type openConnectOpenDialog struct {
	target core.OpenConnectTarget
	tag    string
	id     string
	d      dialog.Dialog
}

// openConnectAuthUI — состояние диалогов. Трогается только в UI-потоке.
var openConnectAuthUI = struct {
	shown         map[string]bool
	open          map[string]*openConnectOpenDialog
	wrongStreak   map[string]int
	lastSubmitted map[string]string
}{
	shown:         map[string]bool{},
	open:          map[string]*openConnectOpenDialog{},
	wrongStreak:   map[string]int{},
	lastSubmitted: map[string]string{},
}

// openConnectKey — ключ «ядро + узел».
func openConnectKey(t core.OpenConnectTarget, tag string) string {
	return fmt.Sprintf("%d|%s|%s", t.Scope, t.MachineID, tag)
}

// installOpenConnectAuthHooks подписывает UI на стрим openconnect: новый
// челлендж → диалог, смена состояния → перерисовка списка узлов.
func installOpenConnectAuthHooks(ac *core.AppController) {
	if ac == nil {
		return
	}
	core.SetOnOpenConnectChange(func(t core.OpenConnectTarget) {
		fyne.Do(func() { openConnectOnChange(ac, t) })
	})
	ac.SetOnOpenConnectChallenge(func(t core.OpenConnectTarget, st core.OpenConnectStatus) {
		fyne.Do(func() { openConnectOnChallenge(ac, t, st) })
	})
}

// openConnectOnChange перерисовывает список и закрывает диалоги, чей
// челлендж ядро уже сняло (вход прошёл или сменился челлендж).
func openConnectOnChange(ac *core.AppController, t core.OpenConnectTarget) {
	if ac.UIService != nil && ac.UIService.ProxiesListWidget != nil {
		ac.UIService.ProxiesListWidget.Refresh()
	}
	for key, od := range openConnectAuthUI.open {
		if od.target != t {
			continue
		}
		st, _ := ac.OpenConnectStatus(t, od.tag)
		if st.PendingChallengeID() != od.id {
			delete(openConnectAuthUI.open, key)
			od.d.Hide()
		}
	}
}

// openConnectOnChallenge — новый challenge.id от стрима: показ один раз.
func openConnectOnChallenge(ac *core.AppController, t core.OpenConnectTarget, st core.OpenConnectStatus) {
	id := st.PendingChallengeID()
	if id == "" {
		return
	}
	key := openConnectKey(t, st.Tag)
	if openConnectAuthUI.shown[key+"|"+id] {
		return
	}
	openConnectAuthUI.shown[key+"|"+id] = true
	// Неверный код: после нашего Submit пришёл новый челлендж с ошибкой.
	if prev := openConnectAuthUI.lastSubmitted[key]; prev != "" && prev != id {
		if st.Challenge.IsForm && st.Challenge.Error != "" {
			openConnectAuthUI.wrongStreak[key]++
		} else {
			openConnectAuthUI.wrongStreak[key] = 0
		}
		delete(openConnectAuthUI.lastSubmitted, key)
	}
	showOpenConnectChallenge(ac, t, st)
}

// ShowOpenConnectChallengeFor — повторный показ по клику на статус узла.
func ShowOpenConnectChallengeFor(ac *core.AppController, scope services.ProxyScope, tag string) {
	t := ac.OpenConnectTargetOf(core.CoreIn(scope))
	st, ok := ac.OpenConnectStatus(t, tag)
	if !ok || st.PendingChallengeID() == "" {
		return
	}
	openConnectAuthUI.shown[openConnectKey(t, tag)+"|"+st.PendingChallengeID()] = true
	showOpenConnectChallenge(ac, t, st)
}

// showOpenConnectChallenge — диалог формы или браузера поверх главного
// окна. Прежний диалог того же узла закрывается: новый челлендж его заменил.
func showOpenConnectChallenge(ac *core.AppController, t core.OpenConnectTarget, st core.OpenConnectStatus) {
	if ac.UIService == nil || ac.UIService.MainWindow == nil || st.Challenge == nil {
		return
	}
	key := openConnectKey(t, st.Tag)
	if od, ok := openConnectAuthUI.open[key]; ok {
		delete(openConnectAuthUI.open, key)
		od.d.Hide()
	}
	debuglog.DebugLog("openconnect ui: showing challenge for %s (form=%v)", st.Tag, st.Challenge.IsForm)
	var d dialog.Dialog
	if st.Challenge.Browser != nil {
		d = showOpenConnectBrowser(ac, t, st)
	} else {
		d = showOpenConnectForm(ac, t, st)
	}
	if d == nil {
		return
	}
	od := &openConnectOpenDialog{target: t, tag: st.Tag, id: st.Challenge.ID, d: d}
	openConnectAuthUI.open[key] = od
	d.SetOnClosed(func() {
		if cur, ok := openConnectAuthUI.open[key]; ok && cur == od {
			delete(openConnectAuthUI.open, key)
		}
	})
}

// openConnectHeader — banner, message и ошибка ядра над содержимым.
func openConnectHeader(ch *services.OpenConnectChallenge) *fyne.Container {
	box := container.NewVBox()
	for _, s := range []string{ch.Banner, ch.Message} {
		if s = strings.TrimSpace(s); s != "" {
			l := widget.NewLabel(s)
			l.Wrapping = fyne.TextWrapWord
			box.Add(l)
		}
	}
	if s := strings.TrimSpace(ch.Error); s != "" {
		l := widget.NewLabel(s)
		l.Wrapping = fyne.TextWrapWord
		l.Importance = widget.DangerImportance
		box.Add(l)
	}
	return box
}

// openConnectStatusLabel — строка статуса диалога (ошибки вызова).
func openConnectStatusLabel() *widget.Label {
	l := widget.NewLabel("")
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.WarningImportance
	l.Hide()
	return l
}

// isOpenConnectPasswordField — поле пароля аккаунта (не код).
func isOpenConnectPasswordField(f services.OpenConnectFormField) bool {
	if !strings.EqualFold(f.Kind, "password") {
		return false
	}
	return strings.Contains(strings.ToLower(f.SubmissionKey+" "+f.Name), "password")
}

// showOpenConnectForm — форма второго фактора по Form.Fields как есть.
func showOpenConnectForm(ac *core.AppController, t core.OpenConnectTarget, st core.OpenConnectStatus) dialog.Dialog {
	win := ac.UIService.MainWindow
	ch := st.Challenge
	key := openConnectKey(t, st.Tag)

	content := container.NewVBox(openConnectHeader(ch))
	if openConnectAuthUI.wrongStreak[key] >= openConnectWrongCodeWarnAfter {
		warn := widget.NewLabel(locale.T("Each wrong code triggers a new sign-in on the gateway"))
		warn.Wrapping = fyne.TextWrapWord
		warn.Importance = widget.WarningImportance
		content.Add(warn)
	}

	type fieldValue struct {
		key      string
		password bool
		get      func() string
	}
	var values []fieldValue
	form := widget.NewForm()
	var firstEntry *widget.Entry
	entries := 0
	for _, f := range ch.Form {
		f := f
		label := f.Label
		if strings.TrimSpace(label) == "" {
			label = f.Name
		}
		kind := strings.ToLower(f.Kind)
		switch {
		case kind == "hidden":
			// Скрытое поле шлюза уходит как пришло.
			v := f.Value
			values = append(values, fieldValue{key: f.Key(), get: func() string { return v }})
		case len(f.Options) > 0 || kind == "select" || kind == "choice":
			labels := make([]string, 0, len(f.Options))
			byLabel := make(map[string]string, len(f.Options))
			selected := ""
			for _, o := range f.Options {
				l := o.Label
				if l == "" {
					l = o.Value
				}
				labels = append(labels, l)
				byLabel[l] = o.Value
				if o.Value == f.Value {
					selected = l
				}
			}
			sel := widget.NewSelect(labels, nil)
			if selected != "" {
				sel.SetSelected(selected)
			} else if len(labels) > 0 {
				sel.SetSelectedIndex(0)
			}
			form.Append(label, sel)
			values = append(values, fieldValue{key: f.Key(), get: func() string { return byLabel[sel.Selected] }})
		case kind == "password":
			e := widget.NewPasswordEntry()
			e.SetText(f.Value)
			form.Append(label, e)
			entries++
			if firstEntry == nil {
				firstEntry = e
			}
			values = append(values, fieldValue{key: f.Key(), password: isOpenConnectPasswordField(f), get: func() string { return e.Text }})
		default:
			e := widget.NewEntry()
			e.SetText(f.Value)
			form.Append(label, e)
			entries++
			if firstEntry == nil {
				firstEntry = e
			}
			values = append(values, fieldValue{key: f.Key(), get: func() string { return e.Text }})
		}
	}
	content.Add(form)
	status := openConnectStatusLabel()
	content.Add(status)

	var d dialog.Dialog
	var submitBtn, newCodeBtn *widget.Button
	setBusy := func(busy bool) {
		if busy {
			submitBtn.Disable()
			newCodeBtn.Disable()
		} else {
			submitBtn.Enable()
			newCodeBtn.Enable()
		}
	}
	showErr := func(err error) {
		status.SetText(err.Error())
		status.Show()
		setBusy(false)
	}
	submit := func() {
		out := make(map[string]string, len(values))
		passwordSent := false
		for _, v := range values {
			val := v.get()
			out[v.key] = val
			if v.password && val != "" {
				passwordSent = true
			}
		}
		setBusy(true)
		status.Hide()
		go func() {
			err := ac.OpenConnectSubmitForm(t, st.Tag, ch.ID, out)
			fyne.Do(func() {
				if err != nil {
					showErr(err)
					return
				}
				openConnectAuthUI.lastSubmitted[key] = ch.ID
				d.Hide()
				if passwordSent {
					// Только подсказка: запись пароля в узел — отдельная задача.
					dialogs.ShowInfo(win, locale.T("Password"),
						locale.Tf("If the password for %s has changed, update it in the node settings so the next sign-in does not ask again.", st.Tag))
				}
			})
		}()
	}
	submitBtn = widget.NewButton(locale.T("Submit"), submit)
	submitBtn.Importance = widget.HighImportance
	newCodeBtn = widget.NewButton(locale.T("Request a new code"), func() {
		dialog.ShowConfirm(locale.T("Request a new code"), locale.T("The gateway will send a new code"), func(ok bool) {
			if !ok {
				return
			}
			setBusy(true)
			status.Hide()
			go func() {
				err := ac.OpenConnectCancel(t, st.Tag, ch.ID)
				fyne.Do(func() {
					if err != nil {
						showErr(err)
						return
					}
					d.Hide()
				})
			}()
		}, win)
	})
	// Enter отправляет только форму из одного поля (код): в паре логин +
	// пароль он ушёл бы раньше, чем заполнено второе поле.
	if firstEntry != nil && entries == 1 {
		firstEntry.OnSubmitted = func(string) { submit() }
	}
	buttons := container.NewHBox(newCodeBtn, submitBtn)
	d = dialogs.NewCustom(locale.Tf("Sign in: %s", st.Tag), content, buttons, locale.T("Not now"), win)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
	if firstEntry != nil {
		win.Canvas().Focus(firstEntry)
	}
	return d
}

// openConnectWebURL — ссылка от ядра открывается только как http/https.
func openConnectWebURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

// openConnectNodeCallbackPort — поле lx_sso_callback_port тела узла
// (0 — не задано или не число).
func openConnectNodeCallbackPort(ac *core.AppController, scope services.ProxyScope, tag string) int {
	node := wizardbusiness.LoadConfigNodes(effectiveNodeConfigPath(ac, scope)).Lookup(tag)
	if node == nil || node.Raw == nil {
		return 0
	}
	if f, ok := node.Raw[core.SSOCallbackNodePortKey].(float64); ok && f > 0 && f <= 65535 && f == float64(int(f)) {
		return int(f)
	}
	return 0
}

// showOpenConnectBrowser — SSO через браузер. Callback-режим (префиксы на
// loopback): слушатель на этом компьютере, пока открыт диалог. Иначе
// (cookie/header) — не поддержано в этой версии, только Close.
func showOpenConnectBrowser(ac *core.AppController, t core.OpenConnectTarget, st core.OpenConnectStatus) dialog.Dialog {
	win := ac.UIService.MainWindow
	ch := st.Challenge
	br := ch.Browser
	title := locale.Tf("Sign in: %s", st.Tag)

	if !core.SSOCallbackLoopbackOnly(br.CallbackURLPrefixes) {
		debuglog.DebugLog("openconnect ui: browser challenge for %s without loopback callback — not supported", st.Tag)
		msg := widget.NewLabel(locale.T("SSO for this gateway type is not supported yet"))
		msg.Wrapping = fyne.TextWrapWord
		d := dialogs.NewCustom(title, container.NewVBox(openConnectHeader(ch), msg), nil, locale.T("Close"), win)
		d.Resize(fyne.NewSize(420, 0))
		d.Show()
		return d
	}

	port := core.SSOCallbackPort(br.CallbackURLPrefixes, openConnectNodeCallbackPort(ac, t.Scope, st.Tag))
	listener, err := core.StartSSOCallbackListener(port, br.CallbackURLPrefixes)
	if err != nil {
		msg := widget.NewLabel(locale.Tf("Cannot listen on 127.0.0.1:%d: the port is busy or blocked. Close the program that uses it, or set lx_sso_callback_port in the node.", port))
		msg.Wrapping = fyne.TextWrapWord
		d := dialogs.NewCustom(title, msg, nil, locale.T("Close"), win)
		d.Resize(fyne.NewSize(420, 0))
		d.Show()
		return d
	}

	content := container.NewVBox(openConnectHeader(ch))
	intro := widget.NewLabel(locale.T("Sign in in your browser"))
	intro.TextStyle = fyne.TextStyle{Bold: true}
	content.Add(intro)

	safe := openConnectWebURL(br.URL)
	link := widget.NewEntry()
	link.SetText(br.URL)
	link.OnChanged = func(s string) {
		if s != br.URL {
			link.SetText(br.URL)
		}
	}
	copyBtn := widget.NewButton(locale.T("Copy"), func() {
		if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
			app.Clipboard().SetContent(br.URL)
		}
	})
	content.Add(container.NewBorder(nil, nil, nil, copyBtn, link))
	if !safe {
		bad := widget.NewLabel(locale.T("The sign-in link is not a web address, so it is not opened automatically."))
		bad.Wrapping = fyne.TextWrapWord
		bad.Importance = widget.WarningImportance
		content.Add(bad)
	}
	waiting := widget.NewLabel(locale.T("Waiting for the browser to come back…"))
	waiting.Wrapping = fyne.TextWrapWord
	content.Add(waiting)
	status := openConnectStatusLabel()
	content.Add(status)

	openBtn := widget.NewButton(locale.T("Open"), func() {
		if err := platform.OpenURL(br.URL); err != nil {
			debuglog.WarnLog("openconnect ui: open browser: %v", err)
			status.SetText(err.Error())
			status.Show()
		}
	})
	openBtn.Importance = widget.HighImportance
	if !safe {
		openBtn.Disable()
	}

	ctx, cancel := context.WithCancel(context.Background())
	d := dialogs.NewCustom(title, content, container.NewHBox(openBtn), locale.T("Not now"), win)
	d.SetOnClosed(func() {
		cancel()
		listener.Close()
	})
	d.Resize(fyne.NewSize(480, 0))
	d.Show()

	go func() {
		select {
		case <-ctx.Done():
			return
		case finalURL := <-listener.URLs():
			fyne.Do(func() { waiting.SetText(locale.T("Signing in…")) })
			err := ac.OpenConnectSubmitBrowser(t, st.Tag, ch.ID, finalURL)
			fyne.Do(func() {
				if err != nil {
					status.SetText(err.Error())
					status.Show()
					return
				}
				d.Hide()
			})
		}
	}()
	return d
}

// openConnectRowState — для строки узла: суффикс подзаголовка и висит ли
// челлендж. Только узлы openconnect; без канала (классика) — пусто.
func openConnectRowState(ac *core.AppController, scope services.ProxyScope, tag string) (suffix string, pending bool) {
	node := wizardbusiness.LoadConfigNodes(effectiveNodeConfigPath(ac, scope)).Lookup(tag)
	if node == nil || node.Type != openConnectNodeType {
		return "", false
	}
	st, ok := ac.OpenConnectStatus(core.CoreIn(scope), tag)
	if !ok {
		return "", false
	}
	pending = st.PendingChallengeID() != ""
	switch {
	case st.State == services.OpenConnectStateAuthPending || pending:
		return locale.T("Waiting for sign-in"), pending
	case st.State == services.OpenConnectStateError:
		msg := strings.TrimSpace(st.Error)
		if msg == "" {
			msg = strings.TrimSpace(st.StateText)
		}
		if msg == "" {
			msg = locale.T("error")
		}
		return truncateRunes(msg, openConnectRowErrorMaxRunes), false
	}
	return "", false
}
