package core

import (
	"runtime"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// CreateTrayMenu creates the system tray menu with proxy selection submenu.
func (ac *AppController) CreateTrayMenu() *fyne.Menu {
	menuItems := []*fyne.MenuItem{}

	// macOS: separator at top to fix menu positioning
	if runtime.GOOS == "darwin" {
		menuItems = append(menuItems, fyne.NewMenuItemSeparator())
	}

	// Open
	menuItems = append(menuItems,
		fyne.NewMenuItem(locale.T("Open"), func() {
			if ac.hasUI() {
				platform.RestoreDockIcon()
				ac.UIService.ShowMainWindowOrFocusWizard()
			}
		}),
		fyne.NewMenuItemSeparator(),
	)

	// VPN controls + proxy submenu (only when APIService is available)
	if ac.APIService != nil {
		menuItems = ac.addVPNAndProxyMenuItems(menuItems)
	}

	// macOS: "Hide app from Dock" toggle
	if runtime.GOOS == "darwin" {
		menuItems = ac.addHideDockMenuItem(menuItems)
	}

	// Quit
	menuItems = append(menuItems, fyne.NewMenuItem(locale.T("Quit"), ac.GracefulExit))

	return fyne.NewMenu("Singbox Launcher", menuItems...)
}

// addVPNAndProxyMenuItems adds Start/Stop VPN buttons and proxy submenu to the menu.
func (ac *AppController) addVPNAndProxyMenuItems(menuItems []*fyne.MenuItem) []*fyne.MenuItem {
	// Trigger auto-load if proxies list is empty and API is enabled
	ac.triggerProxyAutoLoadIfNeeded()

	// Start/Stop VPN buttons
	buttonState := ac.GetVPNButtonState()

	if buttonState.StartEnabled {
		menuItems = append(menuItems, fyne.NewMenuItem(locale.T("Start VPN"), func() { StartSingBoxProcess() }))
	} else {
		startItem := fyne.NewMenuItem(locale.T("Start VPN"), nil)
		startItem.Disabled = true
		menuItems = append(menuItems, startItem)
	}

	if buttonState.StopEnabled {
		menuItems = append(menuItems, fyne.NewMenuItem(locale.T("Stop VPN"), func() { StopSingBoxProcess() }))
	} else {
		stopItem := fyne.NewMenuItem(locale.T("Stop VPN"), nil)
		stopItem.Disabled = true
		menuItems = append(menuItems, stopItem)
	}

	menuItems = append(menuItems, fyne.NewMenuItemSeparator())

	// Proxy submenu (only if Clash API is enabled and group selected)
	_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
	selectedGroup := ac.APIService.GetSelectedClashGroup()

	if clashAPIEnabled && selectedGroup != "" {
		proxySubmenu := ac.buildProxySubmenu(selectedGroup)
		selectProxyItem := fyne.NewMenuItem(locale.T("Select Proxy"), nil)
		selectProxyItem.ChildMenu = proxySubmenu
		menuItems = append(menuItems, selectProxyItem, fyne.NewMenuItemSeparator())
	}

	return menuItems
}

// buildProxySubmenu creates the proxy selection submenu.
func (ac *AppController) buildProxySubmenu(selectedGroup string) *fyne.Menu {
	proxies := ac.APIService.GetProxiesList()
	activeProxy := ac.APIService.GetActiveProxyName()

	var items []*fyne.MenuItem
	if len(proxies) > 0 {
		for i := range proxies {
			proxy := proxies[i]
			pName := proxy.Name
			display := proxy.DisplayOrName()
			menuItem := fyne.NewMenuItem(display, func() {
				go func() {
					err := ac.APIService.SwitchProxy(selectedGroup, pName)
					fyne.Do(func() {
						if err != nil {
							debuglog.ErrorLog("CreateTrayMenu: Failed to switch proxy: %v", err)
							if ac.hasUI() {
								dialogs.ShowError(ac.UIService.MainWindow, err)
							}
						}
					})
				}()
			})
			if pName == activeProxy {
				menuItem.Label = "✓ " + display
			}
			items = append(items, menuItem)
		}
	} else {
		disabledItem := fyne.NewMenuItem(locale.T("No proxies available"), nil)
		disabledItem.Disabled = true
		items = append(items, disabledItem)
	}

	return fyne.NewMenu("Select Proxy", items...)
}

// triggerProxyAutoLoadIfNeeded starts background proxy loading if conditions are met.
func (ac *AppController) triggerProxyAutoLoadIfNeeded() {
	_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
	selectedGroup := ac.APIService.GetSelectedClashGroup()
	proxies := ac.APIService.GetProxiesList()

	if !clashAPIEnabled || selectedGroup == "" || len(proxies) > 0 {
		return
	}
	if !ac.RunningState.IsRunning() {
		return
	}

	ac.APIService.AutoLoadMutex.Lock()
	alreadyInProgress := ac.APIService.AutoLoadInProgress
	ac.APIService.AutoLoadMutex.Unlock()

	if !alreadyInProgress {
		go ac.AutoLoadProxies()
	}
}

// addHideDockMenuItem adds "Hide app from Dock" toggle menu item (macOS only).
func (ac *AppController) addHideDockMenuItem(menuItems []*fyne.MenuItem) []*fyne.MenuItem {
	if runtime.GOOS != "darwin" {
		return menuItems
	}

	hideDockEnabled := ac.UIService.HideAppFromDock
	hideDockLabel := locale.T("Hide app from Dock")
	if hideDockEnabled {
		hideDockLabel = "✓ " + hideDockLabel
	}

	menuItems = append(menuItems, fyne.NewMenuItem(hideDockLabel, func() {
		ac.UIService.HideAppFromDock = !ac.UIService.HideAppFromDock

		if runtime.GOOS == "darwin" {
			if ac.UIService.HideAppFromDock {
				platform.HideDockIcon()
				if ac.UIService.MainWindow != nil {
					ac.UIService.HideMainWindow()
				}
				debuglog.InfoLog("Tray: Hide app from Dock enabled — Dock hidden and window hidden")
			} else {
				platform.RestoreDockIcon()
				if ac.hasUI() {
					ac.UIService.ShowMainWindowOrFocusWizard()
				}
				debuglog.InfoLog("Tray: Hide app from Dock disabled — Dock restored and window shown")
			}
		}

		if ac.UIService.UpdateTrayMenuFunc != nil {
			ac.UIService.UpdateTrayMenuFunc()
		}
	}))
	menuItems = append(menuItems, fyne.NewMenuItemSeparator())

	return menuItems
}
