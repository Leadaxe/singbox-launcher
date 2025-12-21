package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"singbox-launcher/core"
)

// App manages the UI structure and tabs
type App struct {
	window      fyne.Window
	core        *core.AppController
	tabs        *container.AppTabs
	clashAPITab *container.TabItem
	currentTab  *container.TabItem
}

// NewApp creates a new App instance
func NewApp(window fyne.Window, controller *core.AppController) *App {
	app := &App{
		window: window,
		core:   controller,
	}

	// Create tabs - Core is first (opens on startup)
	// Создаем вкладку Core первой, чтобы её callback установился
	coreTabItem := container.NewTabItem("⚙️ Core", CreateCoreDashboardTab(controller))
	app.clashAPITab = container.NewTabItem("🖥️ Servers", CreateClashAPITab(controller))
	app.tabs = container.NewAppTabs(
		coreTabItem,
		app.clashAPITab,
		container.NewTabItem("🔍 Diagnostics", CreateDiagnosticsTab(controller)),
		container.NewTabItem("❓ Help", CreateHelpTab(controller)),
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

	return app
}

// GetTabs returns the tabs container
func (a *App) GetTabs() *container.AppTabs {
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
