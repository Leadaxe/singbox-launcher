package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	copyLinkNotSupportedText = "This outbound cannot be turned into a share link (e.g. selector, direct-out, or fields not supported by the URI encoder)."
)

// effectiveNodeConfigPath — config.json, описывающий ядро области scope.
// Для Remote с подключённой машиной это ЕЁ собранный конфиг
// (wizard_states/remote/<id>/config.json): локальный bin/config.json описывает
// другое ядро, и поиск удалённого узла в нём давал «not present in config.json»
// на живой ноде роутера. Без машины (и для Local) — локальный путь.
func effectiveNodeConfigPath(ac *core.AppController, scope services.ProxyScope) string {
	if ac == nil || ac.FileService == nil {
		return ""
	}
	if scope == services.ScopeRemote {
		if id, _, active := GetLxdRemoteOverride(); active && id != "" {
			return platform.GetRemoteConfigPathFor(ac.FileService.ExecDir, id)
		}
	}
	return ac.FileService.ConfigPath
}

// serversProxyContextMenu is the ПКМ menu for one proxy row: type line + copy link actions.
func serversProxyContextMenu(ac *core.AppController, status *widget.Label, win fyne.Window, proxy api.ProxyInfo, scope services.ProxyScope) *fyne.Menu {
	cfgPath := effectiveNodeConfigPath(ac, scope)
	hint := proxy.ContextMenuTypeLine(locale.T("Unknown type"))
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(hint, nil),
		// SPEC 095 — карточка узла: сервер, транспорт, TLS, состав группы и
		// полный JSON. Пунктом меню, а не кнопкой в строке: строка плотная,
		// а Info нужен изредка.
		fyne.NewMenuItem(locale.T("Node info…"), func() {
			showNodeInfoWindow(ac, proxy, cfgPath)
		}),
		fyne.NewMenuItem(locale.T("Copy server link"), func() {
			serversRunCopyShareURIToClipboard(ac, status, win, proxy.Name, cfgPath)
		}),
	}
	if ac != nil && ac.FileService != nil {
		if detourTag, err := config.GetDetourTagForOutboundTag(cfgPath, proxy.Name); err == nil && detourTag != "" {
			items = append(items, fyne.NewMenuItem(locale.T("Copy jump server link"), func() {
				serversRunCopyJumpShareURIToClipboard(ac, status, win, proxy.Name, cfgPath)
			}))
		}
	}
	return fyne.NewMenu("", items...)
}

func serversRunCopyShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string, cfgPath string) {
	go func() {
		fyne.Do(func() {
			status.SetText(locale.T("Building share URI from outbound…"))
		})
		line, err := config.ShareMainURIForOutboundTag(cfgPath, tag)
		fyne.Do(func() {
			if err != nil {
				if errors.Is(err, subscription.ErrShareURINotSupported) {
					ShowErrorText(win, locale.T("🖥️ Servers"), locale.T(copyLinkNotSupportedText))
				} else {
					ShowError(win, err)
				}
				return
			}
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(line)
			}
			status.SetText(locale.T("Share URI copied to clipboard"))
		})
	}()
}

func serversRunCopyJumpShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string, cfgPath string) {
	go func() {
		fyne.Do(func() {
			status.SetText(locale.T("Building share URI for jump outbound…"))
		})
		line, err := config.ShareJumpURIForOutboundTag(cfgPath, tag)
		fyne.Do(func() {
			if err != nil {
				if errors.Is(err, subscription.ErrShareURINotSupported) {
					ShowErrorText(win, locale.T("🖥️ Servers"), locale.T(copyLinkNotSupportedText))
				} else {
					ShowError(win, err)
				}
				return
			}
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(line)
			}
			status.SetText(locale.T("Jump share URI copied to clipboard"))
		})
	}()
}
