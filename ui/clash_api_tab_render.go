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
	hint := proxy.ContextMenuTypeLine(locale.T("servers.menu_context_type_unknown"))
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(hint, nil),
		// SPEC 095 — карточка узла: сервер, транспорт, TLS, состав группы и
		// полный JSON. Пунктом меню, а не кнопкой в строке: строка плотная,
		// а Info нужен изредка.
		fyne.NewMenuItem(locale.T("servers.menu_node_info"), func() {
			showNodeInfoWindow(ac, proxy, cfgPath)
		}),
		fyne.NewMenuItem(locale.T("servers.menu_copy_server_link"), func() {
			serversRunCopyShareURIToClipboard(ac, status, win, proxy.Name, cfgPath)
		}),
	}
	if ac != nil && ac.FileService != nil {
		if detourTag, err := config.GetDetourTagForOutboundTag(cfgPath, proxy.Name); err == nil && detourTag != "" {
			items = append(items, fyne.NewMenuItem(locale.T("servers.menu_copy_jump_server_link"), func() {
				serversRunCopyJumpShareURIToClipboard(ac, status, win, proxy.Name, cfgPath)
			}))
		}
	}
	return fyne.NewMenu("", items...)
}

func serversRunCopyShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string, cfgPath string) {
	go func() {
		fyne.Do(func() {
			status.SetText(locale.T("servers.copy_link_resolving"))
		})
		line, err := config.ShareMainURIForOutboundTag(cfgPath, tag)
		fyne.Do(func() {
			if err != nil {
				if errors.Is(err, subscription.ErrShareURINotSupported) {
					ShowErrorText(win, locale.T("app.tab.servers"), locale.T("servers.copy_link_not_supported"))
				} else {
					ShowError(win, err)
				}
				return
			}
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(line)
			}
			status.SetText(locale.T("servers.copy_link_done"))
		})
	}()
}

func serversRunCopyJumpShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string, cfgPath string) {
	go func() {
		fyne.Do(func() {
			status.SetText(locale.T("servers.copy_jump_link_resolving"))
		})
		line, err := config.ShareJumpURIForOutboundTag(cfgPath, tag)
		fyne.Do(func() {
			if err != nil {
				if errors.Is(err, subscription.ErrShareURINotSupported) {
					ShowErrorText(win, locale.T("app.tab.servers"), locale.T("servers.copy_link_not_supported"))
				} else {
					ShowError(win, err)
				}
				return
			}
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(line)
			}
			status.SetText(locale.T("servers.copy_jump_link_done"))
		})
	}()
}
