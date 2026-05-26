package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"singbox-launcher/core"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
)

// App manages the UI structure and tabs.
//
// `overlay` and `content` exist for the optional main-window click-redirect
// overlay (see `ui/wizard_overlay.go::wizardOverlayEnabled`). When the
// feature flag is off, `content == tabs` (bare passthrough) and `overlay`
// stays nil — clicks on the main window flow normally even while the
// configurator is open.
type App struct {
	window      fyne.Window
	core        *core.AppController
	tabs        *container.AppTabs
	clashAPITab *container.TabItem
	currentTab  *container.TabItem
	content     fyne.CanvasObject
	// overlay is a concrete ClickRedirect component from `ui/components`.
	// nil when `wizardOverlayEnabled` is false (current default).
	overlay *components.ClickRedirect
}

// NewApp creates a new App instance
func NewApp(window fyne.Window, controller *core.AppController) *App {
	app := &App{
		window: window,
		core:   controller,
	}

	// Create tabs - Core is first (opens on startup)
	// Создаем вкладку Core первой, чтобы её callback установился.
	// Emoji-in-label (💡 default emoji presentation) — colour rendering
	// via OS font fallback to Apple Color Emoji, matching sibling tabs
	// (🖥️ Servers / ⚙️ Settings / 🔍 Diagnostics).
	coreTabItem := container.NewTabItem(locale.T("app.tab.core"), CreateCoreDashboardTab(controller))
	app.clashAPITab = container.NewTabItem(locale.T("app.tab.servers"), CreateClashAPITab(controller))
	app.tabs = container.NewAppTabs(
		coreTabItem,
		app.clashAPITab,
		container.NewTabItem(locale.T("app.tab.settings"), CreateSettingsTab(controller)),
		container.NewTabItem(locale.T("app.tab.diagnostics"), CreateDiagnosticsTab(controller)),
		container.NewTabItem(locale.T("app.tab.help"), CreateHelpTab(controller)),
	)

	// Set tab selection handler
	app.tabs.OnSelected = func(item *container.TabItem) {
		app.currentTab = item
		if item == app.clashAPITab {
			// Проверяем, запущен ли sing-box
			if !controller.RunningState.IsRunning() {
				// Если не запущен, переключаем обратно на Core
				app.tabs.Select(coreTabItem)
				// Можно показать сообщение пользователю
				return
			}
			if controller.UIService != nil && controller.UIService.RefreshAPIFunc != nil {
				controller.UIService.RefreshAPIFunc()
			}
		}
	}

	// Сохраняем оригинальный callback, который был установлен в CreateCoreDashboardTab
	originalUpdateCoreStatusFunc := controller.UIService.UpdateCoreStatusFunc

	// Регистрируем комбинированный callback для обновления состояния вкладки Servers
	controller.UIService.UpdateCoreStatusFunc = func() {
		// Вызываем оригинальный callback, если он есть
		if originalUpdateCoreStatusFunc != nil {
			originalUpdateCoreStatusFunc()
		}
		// Обновляем состояние вкладки Servers
		fyne.Do(func() {
			app.updateClashAPITabState()
		})
	}

	// Инициализируем состояние вкладки
	app.updateClashAPITabState()

	// Инициализируем overlay для перенаправления кликов на визард.
	// Поведение зависит от `wizardOverlayEnabled` (см. ui/wizard_overlay.go) —
	// по дефолту выключено, главное окно работает параллельно с визардом.
	InitWizardOverlay(app, controller)

	// Main-window keyboard shortcuts for power users — matches the
	// right-click menu on the Update button (core_dashboard_tab.go).
	// Modifier is ShortcutDefault which maps to Super on macOS, Control on
	// Linux/Windows. Registered on the Canvas so they fire regardless of
	// which tab has focus, unless a text field is actively consuming input.
	app.registerShortcuts()

	return app
}

// registerShortcuts wires keyboard accelerators for the most common daily
// power-user actions: reconnect sing-box, update subscriptions.
func (a *App) registerShortcuts() {
	if a.window == nil || a.window.Canvas() == nil {
		return
	}
	reconnect := &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierShortcutDefault}
	a.window.Canvas().AddShortcut(reconnect, func(fyne.Shortcut) {
		core.KillSingBoxForRestart()
	})
	updateSubs := &desktop.CustomShortcut{KeyName: fyne.KeyU, Modifier: fyne.KeyModifierShortcutDefault}
	a.window.Canvas().AddShortcut(updateSubs, func(fyne.Shortcut) {
		core.RunParserProcess()
	})
	// Cmd/Ctrl+P → ping-all. Bound to the same hook the power-resume path
	// uses (AutoPingAfterConnectFunc), so it works even when the Servers tab
	// isn't focused.
	pingAll := &desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault}
	a.window.Canvas().AddShortcut(pingAll, func(fyne.Shortcut) {
		if a.core != nil && a.core.UIService != nil && a.core.UIService.AutoPingAfterConnectFunc != nil {
			a.core.UIService.AutoPingAfterConnectFunc()
		}
	})
}

// GetTabs returns the tabs container
func (a *App) GetTabs() *container.AppTabs {
	return a.tabs
}

// GetContent returns the root content for the main window (tabs alone when
// the overlay is disabled, tabs+overlay when enabled — see
// `wizardOverlayEnabled`).
func (a *App) GetContent() fyne.CanvasObject {
	if a.content != nil {
		return a.content
	}
	return a.tabs
}

// GetWindow returns the main window
func (a *App) GetWindow() fyne.Window {
	return a.window
}

// GetController returns the core controller
func (a *App) GetController() *core.AppController {
	return a.core
}

// updateClashAPITabState обновляет состояние вкладки Servers в зависимости от статуса запуска
func (a *App) updateClashAPITabState() {
	if a.clashAPITab == nil || a.tabs == nil {
		return
	}

	isRunning := a.core.RunningState.IsRunning()

	// Используем DisableItem/EnableItem из AppTabs для визуальной индикации неактивности
	if !isRunning {
		// Вкладка неактивна - отключаем её (будет показана серым цветом)
		a.tabs.DisableItem(a.clashAPITab)
	} else {
		// Вкладка активна - включаем её
		a.tabs.EnableItem(a.clashAPITab)
	}

	// Если sing-box не запущен и вкладка Servers выбрана, переключаем на Core
	if !isRunning && a.currentTab == a.clashAPITab {
		if len(a.tabs.Items) > 0 {
			coreTab := a.tabs.Items[0]
			a.tabs.Select(coreTab)
		}
	}
}
