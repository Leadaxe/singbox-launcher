// Package dialogs — диалоги визарда конфигурации.
//
// Файл warp_dialog.go: «Add WARP» (SPEC 084.1) — генератор Cloudflare WARP.
// Регистрирует аккаунт через Cloudflare API (ключ генерится на устройстве, в
// облако уходит только публичный — как wgcf), собирает wireguard://-узел и
// отдаёт его в onURI-колбэк, который прогоняет URI через тот же путь Add, что и
// ручная вставка ссылки. Ничего не сохраняет напрямую — состояние меняет только
// общий Add-путь.
package dialogs

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/warp"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// ShowAddWarpDialog открывает визард генерации WARP. onURI получает готовый
// wireguard://-URI (вызывается в главном потоке Fyne) — обычно это
// applyAddedSources из source_tab, т.е. тот же путь, что и ручная вставка.
func ShowAddWarpDialog(presenter *wizardpresentation.WizardPresenter, onURI func(string)) {
	guiState := presenter.GUIState()
	if guiState == nil || guiState.Window == nil || onURI == nil {
		return
	}
	win := guiState.Window

	// Transport: обфускация по умолчанию (лучше для РФ — AmneziaWG обходит DPI).
	transport := widget.NewRadioGroup([]string{
		locale.T("wizard.warp.transport_awg"),
		locale.T("wizard.warp.transport_plain"),
	}, nil)
	transport.SetSelected(locale.T("wizard.warp.transport_awg"))

	randomEndpoint := widget.NewCheck(locale.T("wizard.warp.random_endpoint"), nil)
	randomEndpoint.SetChecked(true)

	license := widget.NewEntry()
	license.SetPlaceHolder(locale.T("wizard.warp.license_placeholder"))

	form := widget.NewForm(
		widget.NewFormItem(locale.T("wizard.warp.transport_label"), transport),
		widget.NewFormItem("", randomEndpoint),
		widget.NewFormItem(locale.T("wizard.warp.license_label"), license),
	)
	content := container.NewVBox(
		widget.NewLabel(locale.T("wizard.warp.intro")),
		form,
	)

	dlg := dialog.NewCustomConfirm(
		locale.T("wizard.warp.title"),
		locale.T("wizard.warp.button_create"),
		locale.T("wizard.warp.button_cancel"),
		content,
		func(ok bool) {
			if !ok {
				return
			}
			obfuscate := transport.Selected == locale.T("wizard.warp.transport_awg")
			runWarpRegistration(win, onURI, warpRegParams{
				obfuscate:      obfuscate,
				randomEndpoint: randomEndpoint.Checked,
				license:        license.Text,
			})
		},
		win,
	)
	dlg.Resize(fyne.NewSize(460, 260))
	dlg.Show()
}

type warpRegParams struct {
	obfuscate      bool
	randomEndpoint bool
	license        string
}

// runWarpRegistration регистрирует WARP на горутине (сеть) и через fyne.Do
// возвращает результат в главный поток: URI → onURI, ошибка → dialog.ShowError.
func runWarpRegistration(win fyne.Window, onURI func(string), p warpRegParams) {
	loading := dialog.NewCustomWithoutButtons(
		locale.T("wizard.warp.registering_title"),
		widget.NewLabel(locale.T("wizard.warp.registering_msg")),
		win,
	)
	loading.Show()

	go func() {
		// warp.NewClient(nil) использует http.DefaultTransport → уважает
		// HTTP_PROXY/HTTPS_PROXY окружения (полезно, если регистрация идёт через
		// активный туннель, когда api.cloudflareclient.com недоступен напрямую).
		client := warp.NewClient(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		acc, err := client.Register(ctx, warp.RegisterOptions{
			LicenseKey:     p.license,
			Obfuscate:      p.obfuscate,
			RandomEndpoint: p.randomEndpoint,
		})
		var uri string
		if err == nil {
			uri, err = acc.ToWireguardURI(true)
		}

		fyne.Do(func() {
			loading.Hide()
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.warp.error_register"), err), win)
				return
			}
			onURI(uri)
			dialog.ShowInformation(
				locale.T("wizard.warp.done_title"),
				locale.T("wizard.warp.done_msg"),
				win,
			)
		})
	}()
}
