// File clash_remote_ui.go — SPEC 064: badge remote-endpoint flow.
//
// От UI SPEC 064 остался только `newRemoteEndpointBadge` — текстовый badge
// "🏠 Local" / "🌐 host:port" в шапке Servers tab (авто-обновление через
// OnOverrideChanged). Диалог ручного Clash-override удалён: окно подключения
// (Servers → ⚙) теперь ведёт на локальный/удалённый ДЕМОН; транспортный
// механизм override (clash_remote.go, EffectiveClashAPIConfig) сохранён.
package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
)

// newRemoteEndpointBadge — text widget показывающий текущий endpoint-mode.
//
// Renders:
//   - "🏠 Local"     — default, override не active
//   - "🌐 host:port" — override active
//
// Регистрирует listener через OnOverrideChanged для авто-update'а; caller
// должен убедиться что viz-callback'и thread-safe (fyne.Do внутри listener'а).
func newRemoteEndpointBadge() *widget.Label {
	badge := widget.NewLabel("")
	refresh := func() {
		if ov, ok := GetRemoteOverride(); ok {
			badge.SetText(locale.Tf("servers.endpoint.badge_remote_format", ov.Host, ov.Port))
		} else {
			badge.SetText(locale.T("servers.endpoint.badge_local"))
		}
	}
	refresh()
	OnOverrideChanged(func() {
		fyne.Do(refresh)
	})
	return badge
}
