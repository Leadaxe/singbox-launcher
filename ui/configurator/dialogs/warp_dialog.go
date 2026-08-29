// Package dialogs — диалоги визарда конфигурации.
//
// Файл warp_dialog.go: WARP-конфигуратор (SPEC 084.1/084.2) — генератор Cloudflare
// WARP с полным набором полей AmneziaWG-обфускации, пресетами и режимом MASQUE.
// Регистрирует аккаунт через Cloudflare API (ключ генерится на устройстве),
// собирает узел и отдаёт готовый URI в onURI-колбэк, который прогоняет его через
// тот же путь Add, что и ручная вставка ссылки. Структура повторяет мобильный
// warp_wizard_screen (LxBox): выбор транспорта WireGuard/MASQUE, obfuscate +
// Advanced со всеми полями, кубик 🎲 для random endpoint/SNI/domain.
package dialogs

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/warp"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	warpJunkNoteText    = "jc/jmin/jmax add standalone junk packets (safe with WARP). Packet padding (s1–s4) and magic headers (h1–h4) are fixed to WARP-compatible values — a plain-WireGuard WARP server drops any other padding, breaking the handshake."
	warpNewKeysNoteText = "By default the node reuses the registration you already have, so H2 and H3 share one key. Tick this if you need a new account — the old one is replaced."
)

// ShowAddWarpDialog открывает WARP-конфигуратор. onURI получает готовый URI
// (wireguard:// или masque://) в главном потоке Fyne — обычно applyAddedSources.
//
// owner — окно, которому принадлежит диалог; nil означает главное окно
// визарда. Параметр появился в SPEC 116 W6: наполнение папки зовёт эту же
// форму из ОТДЕЛЬНОГО окна источника (app.NewWindow), и диалог, прибитый к
// главному окну, всплыл бы за спиной у пользователя — он нажал бы «Add WARP»
// и не увидел ничего.
func ShowAddWarpDialog(presenter *wizardpresentation.WizardPresenter, owner fyne.Window, onURI func(string)) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onURI == nil {
		return
	}
	win := guiState.Window
	if owner != nil {
		win = owner
	}

	wg := newWarpWGSection()
	mq := newWarpMasqueSection()

	// Transport switch: показываем ровно одну секцию.
	transport := widget.NewRadioGroup([]string{
		locale.T("WireGuard"),
		locale.T("MASQUE"),
	}, nil)
	transport.Horizontal = true
	transport.SetSelected(locale.T("WireGuard"))
	transport.OnChanged = func(sel string) {
		if sel == locale.T("MASQUE") {
			wg.container.Hide()
			mq.container.Show()
		} else {
			mq.container.Hide()
			wg.container.Show()
		}
	}
	mq.container.Hide()

	intro := widget.NewLabel(locale.T("Registers a new anonymous WARP account. The key is generated on this device — only the public key is sent to Cloudflare."))
	intro.Wrapping = fyne.TextWrapWord // иначе 120-симв строка задаёт огромный min-width окна

	// Регистрация переиспользуется из кеша (state.warp_accounts): H2 и H3,
	// добавленные подряд, должны сидеть на одном ключе — как в LxBox. Галочка
	// снята по умолчанию; включённая заставляет пойти в Cloudflare за свежей
	// регистрацией и перезаписать кеш. Показываем её только когда кеш реально
	// есть — иначе она обещает выбор, которого нет.
	newKeys := widget.NewCheck(locale.T("Create new keys (fresh Cloudflare registration)"), nil)
	newKeysNote := widget.NewLabel(locale.T(warpNewKeysNoteText))
	newKeysNote.Wrapping = fyne.TextWrapWord
	newKeysNote.TextStyle = fyne.TextStyle{Italic: true}
	newKeysRow := container.NewVBox(newKeys, newKeysNote)
	if presenter.Model() == nil || presenter.Model().WarpAccounts == nil {
		newKeysRow.Hide()
	}

	content := container.NewVBox(
		intro,
		container.NewHBox(widget.NewLabel(locale.T("Transport")), transport),
		newKeysRow,
		widget.NewSeparator(),
		wg.container,
		mq.container,
	)

	// Отдельное окно (Application.NewWindow), а НЕ модальный попап. Попап Fyne
	// подтягивает свой размер до Content.MinSize() и игнорирует .Resize() как
	// потолок → форма либо раздувалась на всё окно, либо вылезала без скролла.
	// Окно фиксированного размера с обычным VScroll внутри решает это — тот же
	// паттерн, что add_rule_dialog / preset_ref_edit (Edit Rule).
	controller := presenter.Controller()
	if controller == nil || controller.UIService == nil {
		return
	}
	warpWindow := controller.UIService.Application.NewWindow(locale.T("Generate Cloudflare WARP"))

	// Точь-в-точь как preset_ref_edit (Edit Rule), который работает: контент в
	// VScroll + gutter внутри, ширину держит само окно (Resize ниже). Никаких
	// GridWrap/HBox-капов — они и ломали раскладку в прошлых итерациях.
	scrollInner := container.NewBorder(nil, nil, nil, components.NewScrollGutter(), content)
	scroll := container.NewVScroll(scrollInner)

	cancelButton := widget.NewButton(locale.T("Cancel"), func() {
		warpWindow.Close()
	})
	createButton := widget.NewButton(locale.T("Create"), func() {
		warpWindow.Close()
		if transport.Selected == locale.T("MASQUE") {
			runMasqueRegistration(win, presenter, onURI, mq.collect(), newKeys.Checked)
		} else {
			runWarpRegistration(win, presenter, onURI, wg.collect(), newKeys.Checked)
		}
	})
	createButton.Importance = widget.HighImportance
	buttons := container.NewHBox(layout.NewSpacer(), cancelButton, createButton)

	dialogContent := container.NewBorder(nil, buttons, nil, nil, scroll)
	warpWindow.Resize(fyne.NewSize(600, 640))
	warpWindow.CenterOnScreen()
	warpWindow.SetContent(dialogContent)
	warpWindow.SetCloseIntercept(func() { warpWindow.Close() })
	warpWindow.Show()
}

// ---- WireGuard / AmneziaWG section ----

type warpWGSection struct {
	container *fyne.Container
	collect   func() warpRegParams
}

func newWarpWGSection() *warpWGSection {
	obfuscate := widget.NewCheck(locale.T("AmneziaWG obfuscation (anti-DPI)"), nil)
	obfuscate.SetChecked(true)

	license := widget.NewEntry()
	license.SetPlaceHolder(locale.T("optional — leave empty for free WARP"))

	endpoint := widget.NewEntry()
	endpoint.SetPlaceHolder("engage.cloudflareclient.com:2408") // l10n-exempt: wire endpoint
	randEndpointBtn := widget.NewButton("🎲", func() { endpoint.SetText(warp.RandomEndpoint(nil)) })

	// Obfuscation preset — заполняет поля ниже.
	presetNames := make([]string, 0)
	for _, p := range warp.ObfuscationPresets() {
		presetNames = append(presetNames, p.Name)
	}
	preset := widget.NewSelect(presetNames, nil)
	preset.SetSelectedIndex(0)

	// Masquerade + junk fields (все поля обфускации).
	ipSel := widget.NewSelect([]string{"quic", "dns", "stun", "sip"}, nil)
	ipSel.SetSelected("quic")
	idEntry := widget.NewSelectEntry(warp.SNIPool)
	idEntry.SetText("www.google.com") // l10n-exempt: sample host
	randIDBtn := widget.NewButton("🎲", func() { idEntry.SetText(warp.RandomSNI(nil)) })
	ibSel := widget.NewSelect([]string{"chrome", "firefox", "curl"}, nil)
	ibSel.SetSelected("chrome")

	jc := numEntry("4")
	jmin := numEntry("40")
	jmax := numEntry("70")
	// s1-s4 (init/response/cookie/transport padding) и h1-h4 (magic headers)
	// НЕ выставляются в UI: Cloudflare WARP — плейн-WireGuard сервер (padding=0,
	// не AmneziaWG). Любой ненулевой s1-s4 сдвигает тип/размер РЕАЛЬНОГО пакета →
	// WARP-сервер не распознаёт handshake и молча дропает (проверено по коду ядра
	// amneziawg-go send.go/receive.go). h1-h4 WARP требует строго 1/2/3/4. Оба
	// набора форсятся в collect() ниже. Против DPI с WARP работают только jc/jmin/
	// jmax (отдельные мусорные датаграммы, сервер их игнорит) + masquerade id/ip/ib.

	reserved := widget.NewCheck(locale.T("Bind to this device (reserved)"), nil)

	applyPreset := func(name string) {
		p := warp.PresetByName(name)
		ipSel.SetSelected(p.IP)
		if p.SNI != "" {
			idEntry.SetText(p.SNI)
		}
		ibSel.SetSelected(p.IB)
		jc.SetText(strconv.Itoa(p.JC))
		jmin.SetText(strconv.Itoa(p.JMin))
		jmax.SetText(strconv.Itoa(p.JMax))
	}
	preset.OnChanged = applyPreset

	// ib только при ip=quic; masquerade-блок только при obfuscate.
	ibRow := labeledRow(locale.T("Browser (ib)"), ibSel)
	ipSel.OnChanged = func(v string) {
		if v == "quic" {
			ibRow.Show()
		} else {
			ibRow.Hide()
		}
	}

	junkNote := widget.NewLabelWithStyle(locale.T(warpJunkNoteText), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	junkNote.Wrapping = fyne.TextWrapWord // 224-симв подсказка — без wrap задаёт огромный min-width

	advanced := container.NewVBox(
		labeledRow(locale.T("WARP+ license"), license),
		labeledRow(locale.T("Endpoint"), container.NewBorder(nil, nil, nil, randEndpointBtn, endpoint)),
		reserved,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(locale.T("Masquerade"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		labeledRow(locale.T("Protocol (ip)"), ipSel),
		labeledRow(locale.T("Domain (id)"), container.NewBorder(nil, nil, nil, randIDBtn, idEntry)),
		ibRow,
		container.NewGridWithColumns(3,
			labeledRow("jc", jc), labeledRow("jmin", jmin), labeledRow("jmax", jmax)),
		junkNote,
	)
	acc := widget.NewAccordion(widget.NewAccordionItem(locale.T("Advanced (all fields)"), advanced))

	// obfuscate=false → прячем пресет+advanced masquerade (plain WARP).
	presetRow := labeledRow(locale.T("Obfuscation preset"), preset)
	obfuscate.OnChanged = func(on bool) {
		if on {
			presetRow.Show()
			acc.Show()
			reserved.SetChecked(false)
		} else {
			presetRow.Hide()
			acc.Close(0)
			reserved.SetChecked(true)
		}
	}

	box := container.NewVBox(obfuscate, presetRow, acc)

	collect := func() warpRegParams {
		// s1-s4 форсятся в 0, h1-h4 — в 1/2/3/4: WARP-сервер плейн-WG, ненулевой
		// padding ломает handshake, а magic headers должны быть каноничны (см.
		// коммент выше). Юзеру эти поля не даём — только jc/jmin/jmax + masquerade.
		p := warp.QuicParams{
			JC: atoiDef(jc.Text, 4), JMin: atoiDef(jmin.Text, 40), JMax: atoiDef(jmax.Text, 70),
			S1: 0, S2: 0, S3: 0, S4: 0,
			H1: 1, H2: 2, H3: 3, H4: 4,
			IP: ipSel.Selected, SNI: idEntry.Text, IB: ibSel.Selected,
		}
		return warpRegParams{
			obfuscate:      obfuscate.Checked,
			randomEndpoint: obfuscate.Checked && endpoint.Text == "",
			endpoint:       endpoint.Text,
			license:        license.Text,
			reserved:       reserved.Checked || !obfuscate.Checked,
			quic:           p,
		}
	}
	return &warpWGSection{container: box, collect: collect}
}

// ---- MASQUE section ----

type warpMasqueSection struct {
	container *fyne.Container
	collect   func() masqueRegParams
}

func newWarpMasqueSection() *warpMasqueSection {
	vhttp := widget.NewSelect([]string{"h3", "h2"}, nil)
	vhttp.SetSelected("h3")

	// Пустой sni → ядро подставляет consumer-masque.cloudflareclient.com, туннель
	// встаёт, но данные не идут (DPI глушит фирменный SNI). Дефолт обязателен —
	// как у masquerade-домена в WG-секции выше.
	sni := widget.NewSelectEntry(warp.MasqueSNIPool)
	sni.SetText(warp.RandomMasqueSNI(nil))
	randSNIBtn := widget.NewButton("🎲", func() { sni.SetText(warp.RandomMasqueSNI(nil)) })

	idle := numEntry("")
	idle.SetPlaceHolder("5")
	keep := numEntry("")
	keep.SetPlaceHolder("30")
	keepRow := labeledRow(locale.T("Keep-alive (sec)"), keep)
	vhttp.OnChanged = func(v string) {
		if v == "h3" {
			keepRow.Show()
		} else {
			keepRow.Hide()
		}
	}

	masqueNote := widget.NewLabel(locale.T("MASQUE tunnels over HTTPS/QUIC and masks itself — AmneziaWG obfuscation does not apply. Requires core lx.3+."))
	masqueNote.Wrapping = fyne.TextWrapWord // 108-симв подсказка — без wrap раздувает окно

	box := container.NewVBox(
		masqueNote,
		labeledRow(locale.T("Transport"), vhttp),
		labeledRow(locale.T("SNI"), container.NewBorder(nil, nil, nil, randSNIBtn, sni)),
		labeledRow(locale.T("Idle timeout (min)"), idle),
		keepRow,
	)
	collect := func() masqueRegParams {
		return masqueRegParams{vhttp: vhttp.Selected, sni: sni.Text}
	}
	return &warpMasqueSection{container: box, collect: collect}
}

// ---- registration runners ----

type warpRegParams struct {
	obfuscate      bool
	randomEndpoint bool
	endpoint       string
	license        string
	reserved       bool
	quic           warp.QuicParams
}

type masqueRegParams struct {
	vhttp string
	sni   string
}

func runWarpRegistration(win fyne.Window, presenter *wizardpresentation.WizardPresenter, onURI func(string), p warpRegParams, forceNew bool) {
	loading := showWarpProgress(win)
	go func() {
		var acc *warp.Account
		var err error

		client := warp.NewClient(nil)
		opts := warp.RegisterOptions{
			LicenseKey:     p.license,
			Endpoint:       p.endpoint,
			Obfuscate:      p.obfuscate,
			Quic:           p.quic,
			RandomEndpoint: p.randomEndpoint,
		}

		if acc = cachedWG(presenter, forceNew); acc != nil {
			// Регистрация из кеша: endpoint и поля обфускации — параметры узла,
			// их UI задаёт заново на каждую сборку (пресет, кубик 🎲).
			client.ApplyNodeOptions(acc, opts)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			acc, err = client.Register(ctx, opts)
			if err == nil {
				storeWG(presenter, acc)
			}
		}

		var uri string
		if err == nil {
			uri, err = acc.ToWireguardURI(p.reserved)
		}
		fyne.Do(func() { finishWarp(win, loading, onURI, uri, err) })
	}()
}

func runMasqueRegistration(win fyne.Window, presenter *wizardpresentation.WizardPresenter, onURI func(string), p masqueRegParams, forceNew bool) {
	loading := showWarpProgress(win)
	go func() {
		var acc *warp.MasqueAccount
		var err error

		if acc = cachedMasque(presenter, forceNew); acc == nil {
			client := warp.NewClient(nil)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			acc, err = client.RegisterMasque(ctx, time.Now().UTC(), p.vhttp, p.sni)
			if err == nil {
				storeMasque(presenter, acc)
			}
		}
		if err == nil {
			// Транспорт/SNI/таймауты — параметры узла, не регистрации: именно
			// поэтому H2 и H3 собираются из одной кешированной записи.
			acc.ApplyNodeOptions(p.vhttp, p.sni)
		}

		var uri string
		if err == nil {
			uri, err = acc.ToMasqueURI()
		}
		fyne.Do(func() { finishWarp(win, loading, onURI, uri, err) })
	}()
}

// cachedWG возвращает кешированную WG-регистрацию или nil (промах кеша либо
// пользователь явно попросил новые ключи).
func cachedWG(presenter *wizardpresentation.WizardPresenter, forceNew bool) *warp.Account {
	if forceNew {
		return nil
	}
	m := presenter.Model()
	if m == nil || m.WarpAccounts == nil {
		return nil
	}
	return warp.WGFromCache(m.WarpAccounts.WG)
}

// cachedMasque — то же для MASQUE.
func cachedMasque(presenter *wizardpresentation.WizardPresenter, forceNew bool) *warp.MasqueAccount {
	if forceNew {
		return nil
	}
	m := presenter.Model()
	if m == nil || m.WarpAccounts == nil {
		return nil
	}
	return warp.MasqueFromCache(m.WarpAccounts.Masque)
}

// storeWG кладёт свежую регистрацию в кеш модели; на диск она уедет вместе с
// остальным state при сохранении визарда.
func storeWG(presenter *wizardpresentation.WizardPresenter, acc *warp.Account) {
	m := presenter.Model()
	if m == nil {
		return
	}
	fyne.Do(func() {
		if m.WarpAccounts == nil {
			m.WarpAccounts = &corestate.WarpAccountsSection{}
		}
		m.WarpAccounts.WG = warp.WGToCache(acc)
		presenter.MarkAsChanged()
	})
}

// storeMasque — то же для MASQUE.
func storeMasque(presenter *wizardpresentation.WizardPresenter, acc *warp.MasqueAccount) {
	m := presenter.Model()
	if m == nil {
		return
	}
	fyne.Do(func() {
		if m.WarpAccounts == nil {
			m.WarpAccounts = &corestate.WarpAccountsSection{}
		}
		m.WarpAccounts.Masque = warp.MasqueToCache(acc)
		presenter.MarkAsChanged()
	})
}

func showWarpProgress(win fyne.Window) *dialog.CustomDialog {
	d := dialog.NewCustomWithoutButtons(
		locale.T("Registering WARP…"),
		widget.NewLabel(locale.T("Contacting Cloudflare and generating keys.")),
		win,
	)
	d.Show()
	return d
}

func finishWarp(win fyne.Window, loading *dialog.CustomDialog, onURI func(string), uri string, err error) {
	loading.Hide()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("WARP registration failed"), err), win)
		return
	}
	onURI(uri)
	dialog.ShowInformation(locale.T("WARP added"), locale.T("The WARP node was added to Sources. Click Add-list / rebuild to use it."), win)
}

// ---- small helpers ----

func numEntry(def string) *widget.Entry {
	e := widget.NewEntry()
	if def != "" {
		e.SetText(def)
	}
	return e
}

func labeledRow(label string, control fyne.CanvasObject) *fyne.Container {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, control)
}

func atoiDef(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
