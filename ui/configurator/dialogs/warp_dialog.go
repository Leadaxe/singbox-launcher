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

	"singbox-launcher/core/warp"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// ShowAddWarpDialog открывает WARP-конфигуратор. onURI получает готовый URI
// (wireguard:// или masque://) в главном потоке Fyne — обычно applyAddedSources.
func ShowAddWarpDialog(presenter *wizardpresentation.WizardPresenter, onURI func(string)) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onURI == nil {
		return
	}
	win := guiState.Window

	wg := newWarpWGSection()
	mq := newWarpMasqueSection()

	// Transport switch: показываем ровно одну секцию.
	transport := widget.NewRadioGroup([]string{
		locale.T("wizard.warp.mode_wireguard"),
		locale.T("wizard.warp.mode_masque"),
	}, nil)
	transport.Horizontal = true
	transport.SetSelected(locale.T("wizard.warp.mode_wireguard"))
	transport.OnChanged = func(sel string) {
		if sel == locale.T("wizard.warp.mode_masque") {
			wg.container.Hide()
			mq.container.Show()
		} else {
			mq.container.Hide()
			wg.container.Show()
		}
	}
	mq.container.Hide()

	intro := widget.NewLabel(locale.T("wizard.warp.intro"))
	intro.Wrapping = fyne.TextWrapWord // иначе 120-симв строка задаёт огромный min-width окна
	content := container.NewVBox(
		intro,
		container.NewHBox(widget.NewLabel(locale.T("wizard.warp.transport_label")), transport),
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
	warpWindow := controller.UIService.Application.NewWindow(locale.T("wizard.warp.title"))

	// Точь-в-точь как preset_ref_edit (Edit Rule), который работает: контент в
	// VScroll + gutter внутри, ширину держит само окно (Resize ниже). Никаких
	// GridWrap/HBox-капов — они и ломали раскладку в прошлых итерациях.
	scrollInner := container.NewBorder(nil, nil, nil, components.NewScrollGutter(), content)
	scroll := container.NewVScroll(scrollInner)

	cancelButton := widget.NewButton(locale.T("wizard.warp.button_cancel"), func() {
		warpWindow.Close()
	})
	createButton := widget.NewButton(locale.T("wizard.warp.button_create"), func() {
		warpWindow.Close()
		if transport.Selected == locale.T("wizard.warp.mode_masque") {
			runMasqueRegistration(win, onURI, mq.collect())
		} else {
			runWarpRegistration(win, onURI, wg.collect())
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
	obfuscate := widget.NewCheck(locale.T("wizard.warp.obfuscate"), nil)
	obfuscate.SetChecked(true)

	license := widget.NewEntry()
	license.SetPlaceHolder(locale.T("wizard.warp.license_placeholder"))

	endpoint := widget.NewEntry()
	endpoint.SetPlaceHolder("engage.cloudflareclient.com:2408")
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
	idEntry.SetText("www.google.com")
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

	reserved := widget.NewCheck(locale.T("wizard.warp.reserved"), nil)

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
	ibRow := labeledRow(locale.T("wizard.warp.masq_browser"), ibSel)
	ipSel.OnChanged = func(v string) {
		if v == "quic" {
			ibRow.Show()
		} else {
			ibRow.Hide()
		}
	}

	junkNote := widget.NewLabelWithStyle(locale.T("wizard.warp.junk_note"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	junkNote.Wrapping = fyne.TextWrapWord // 224-симв подсказка — без wrap задаёт огромный min-width

	advanced := container.NewVBox(
		labeledRow(locale.T("wizard.warp.license_label"), license),
		labeledRow(locale.T("wizard.warp.endpoint_label"), container.NewBorder(nil, nil, nil, randEndpointBtn, endpoint)),
		reserved,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(locale.T("wizard.warp.masq_header"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		labeledRow(locale.T("wizard.warp.masq_protocol"), ipSel),
		labeledRow(locale.T("wizard.warp.masq_domain"), container.NewBorder(nil, nil, nil, randIDBtn, idEntry)),
		ibRow,
		container.NewGridWithColumns(3,
			labeledRow("jc", jc), labeledRow("jmin", jmin), labeledRow("jmax", jmax)),
		junkNote,
	)
	acc := widget.NewAccordion(widget.NewAccordionItem(locale.T("wizard.warp.advanced"), advanced))

	// obfuscate=false → прячем пресет+advanced masquerade (plain WARP).
	presetRow := labeledRow(locale.T("wizard.warp.preset_label"), preset)
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
	network := widget.NewSelect([]string{"h3", "h2"}, nil)
	network.SetSelected("h3")

	sni := widget.NewSelectEntry(warp.MasqueSNIPool)
	sni.SetPlaceHolder(locale.T("wizard.warp.masque_sni_placeholder"))
	randSNIBtn := widget.NewButton("🎲", func() { sni.SetText(warp.RandomMasqueSNI(nil)) })

	idle := numEntry("")
	idle.SetPlaceHolder("5")
	keep := numEntry("")
	keep.SetPlaceHolder("30")
	keepRow := labeledRow(locale.T("wizard.warp.masque_keepalive"), keep)
	network.OnChanged = func(v string) {
		if v == "h3" {
			keepRow.Show()
		} else {
			keepRow.Hide()
		}
	}

	masqueNote := widget.NewLabel(locale.T("wizard.warp.masque_note"))
	masqueNote.Wrapping = fyne.TextWrapWord // 108-симв подсказка — без wrap раздувает окно

	box := container.NewVBox(
		masqueNote,
		labeledRow(locale.T("wizard.warp.masque_transport"), network),
		labeledRow(locale.T("wizard.warp.masque_sni"), container.NewBorder(nil, nil, nil, randSNIBtn, sni)),
		labeledRow(locale.T("wizard.warp.masque_idle"), idle),
		keepRow,
	)
	collect := func() masqueRegParams {
		return masqueRegParams{network: network.Selected, sni: sni.Text}
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
	network string
	sni     string
}

func runWarpRegistration(win fyne.Window, onURI func(string), p warpRegParams) {
	loading := showWarpProgress(win)
	go func() {
		client := warp.NewClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		acc, err := client.Register(ctx, warp.RegisterOptions{
			LicenseKey:     p.license,
			Endpoint:       p.endpoint,
			Obfuscate:      p.obfuscate,
			Quic:           p.quic,
			RandomEndpoint: p.randomEndpoint,
		})
		var uri string
		if err == nil {
			uri, err = acc.ToWireguardURI(p.reserved)
		}
		fyne.Do(func() { finishWarp(win, loading, onURI, uri, err) })
	}()
}

func runMasqueRegistration(win fyne.Window, onURI func(string), p masqueRegParams) {
	loading := showWarpProgress(win)
	go func() {
		client := warp.NewClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		acc, err := client.RegisterMasque(ctx, time.Now().UTC(), p.network, p.sni)
		var uri string
		if err == nil {
			uri, err = acc.ToMasqueURI()
		}
		fyne.Do(func() { finishWarp(win, loading, onURI, uri, err) })
	}()
}

func showWarpProgress(win fyne.Window) *dialog.CustomDialog {
	d := dialog.NewCustomWithoutButtons(
		locale.T("wizard.warp.registering_title"),
		widget.NewLabel(locale.T("wizard.warp.registering_msg")),
		win,
	)
	d.Show()
	return d
}

func finishWarp(win fyne.Window, loading *dialog.CustomDialog, onURI func(string), uri string, err error) {
	loading.Hide()
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.warp.error_register"), err), win)
		return
	}
	onURI(uri)
	dialog.ShowInformation(locale.T("wizard.warp.done_title"), locale.T("wizard.warp.done_msg"), win)
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
