package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/locale"
)

// serversProxyContextMenu is the ПКМ menu for one proxy row: type line + copy link actions.
func serversProxyContextMenu(ac *core.AppController, status *widget.Label, win fyne.Window, proxy api.ProxyInfo) *fyne.Menu {
	hint := proxy.ContextMenuTypeLine(locale.T("servers.menu_context_type_unknown"))
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(hint, nil),
		fyne.NewMenuItem(locale.T("servers.menu_copy_server_link"), func() {
			serversRunCopyShareURIToClipboard(ac, status, win, proxy.Name)
		}),
	}
	if ac != nil && ac.FileService != nil {
		if detourTag, err := config.GetDetourTagForOutboundTag(ac.FileService.ConfigPath, proxy.Name); err == nil && detourTag != "" {
			items = append(items, fyne.NewMenuItem(locale.T("servers.menu_copy_jump_server_link"), func() {
				serversRunCopyJumpShareURIToClipboard(ac, status, win, proxy.Name)
			}))
		}
	}
	return fyne.NewMenu("", items...)
}

func serversRunCopyShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string) {
	cfgPath := ac.FileService.ConfigPath
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

func serversRunCopyJumpShareURIToClipboard(ac *core.AppController, status *widget.Label, win fyne.Window, tag string) {
	cfgPath := ac.FileService.ConfigPath
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
