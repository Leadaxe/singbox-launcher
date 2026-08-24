package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"singbox-launcher/core"
	"singbox-launcher/core/events"
	"singbox-launcher/core/services"
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
	// localPanel / remotePanel — независимые списки прокси двух вкладок
	// (SPEC 098). Держим ссылки, чтобы отдавать разделяемые слоты UIService
	// активной панели при переключении вкладки.
	localPanel  *ProxyListPanel
	remotePanel *ProxyListPanel
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

	// SPEC 098: вкладки Local и Remote вместо Core и Servers. Обе
	// двухколоночные — слева список прокси, справа управление; см.
	// ui/local_remote_tabs.go.
	//
	// Local создаётся первой, чтобы её callback установился (внутри живёт
	// CreateCoreDashboardTab, который регистрирует UpdateCoreStatusFunc).
	// Emoji-in-label (💡 default emoji presentation) — colour rendering
	// via OS font fallback to Apple Color Emoji, matching sibling tabs
	// (⚙️ Settings / 🔍 Diagnostics).
	localContent, localPanel := CreateLocalTab(controller)
	remoteContent, remotePanel := CreateRemoteTab(controller)
	app.localPanel, app.remotePanel = localPanel, remotePanel
	// SPEC 100 §3.8: Debug API получает Connect/Disconnect вкладки Remote.
	// Строго после создания вкладок — подписчики OnOverrideChanged уже стоят.
	RegisterOverrideAPIHooks(controller)
	coreTabItem := container.NewTabItem(locale.T("Local"), localContent)
	app.clashAPITab = container.NewTabItem(locale.T("🌐 Remote"), remoteContent)
	// Settings — обычная вкладка со своим содержимым.
	//
	// Раньше она была кнопкой-подделкой: пустая вкладка, чей OnSelected
	// открывал отдельное окно и тут же откатывал выбор назад. Это стоило
	// защиты от бесконечного цикла в обработчике и делало Settings
	// единственным пунктом строки, ведущим себя не как вкладка. Теперь
	// содержимое рендерится на месте, отдельное окно удалено за
	// ненадобностью.
	settingsTabItem := container.NewTabItem(locale.T("⚙️ Settings"),
		components.WrapInScrollWithGutter(container.NewPadded(BuildSettingsContent(controller))))
	// Tab order: Core | Servers | 🔍 Diagnostics | ⚙️ Settings | ❓ Help.
	// Settings sits between Diagnostics and Help — close to other
	// "launcher behavior" controls and one click away from Help.
	app.tabs = container.NewAppTabs(
		coreTabItem,
		app.clashAPITab,
		container.NewTabItem(locale.T("🔍 Diagnostics"), CreateDiagnosticsTab(controller)),
		settingsTabItem,
		container.NewTabItem(locale.T("❓ Help"), CreateHelpTab(controller)),
	)

	// Set tab selection handler
	app.tabs.OnSelected = func(item *container.TabItem) {
		app.currentTab = item

		// SPEC 098: вкладка определяет, с КАКИМ ядром идёт разговор.
		//
		// Local — всегда своё ядро: транспорт удалённой машины снимается при
		// входе. Без этого список на Local показывал узлы роутера, выбранного
		// на Remote, — то есть чужие данные под именем локального ядра, и
		// вернуться к своему было нечем (дропдаун с пунктом Local убран
		// вместе с шапкой).
		//
		// Remote — выбранная машина либо ничего. Транспорт там ставит клик по
		// строке; сама вкладка ничего не восстанавливает, потому что выбор
		// эфемерный (SPEC 097 §4.3): после перезапуска активной машины нет, и
		// список пуст до первого клика.
		// Панели Local и Remote независимы, но слоты UIService рассчитаны на
		// одного владельца — отдаём их той, что сейчас на экране.
		switch item {
		case coreTabItem:
			// Порядок важен: сначала область, потом снятие транспорта. Оба
			// шага дёргают обновление списка, и оно должно писать уже в
			// local-состояние, а не в remote.
			if controller.APIService != nil {
				controller.APIService.SetProxyScope(services.ScopeLocal)
			}
			app.localPanel.Activate(controller)
			// Транспорт МАШИНЫ не снимаем: соединение — состояние самой
			// машины, а не вкладки. Рвать его при взгляде на своё ядро значит
			// заставлять жать Connect после каждого переключения. Связь
			// разрывает только явный Disconnect (или удаление машины).
			//
			// Чтобы Local при этом говорил со СВОИМ ядром, ставим его
			// транспорт: SetTransport(nil) означал бы «никакого», а в
			// lxd-режиме Clash HTTP нет — панель падала бы в
			// «connection refused» на 9190.
			controller.RestoreOwnTransport()
		case app.clashAPITab:
			if controller.APIService != nil {
				controller.APIService.SetProxyScope(services.ScopeRemote)
			}
			// Возвращаем транспорт выбранной машины: пока смотрели Local, его
			// место занимал транспорт своего движка. Само соединение никуда не
			// девалось — снимает его только явный Disconnect.
			ReapplyLxdRemoteTransport(controller)
			app.remotePanel.Activate(controller)
		}
		// Авто-обновление списка узлов идёт только на видимой вкладке Remote:
		// опрашивать машину, пока пользователь смотрит на Local, незачем.
		app.remotePanel.AutoRefresh().SetTabActive(item == app.clashAPITab)
		// Обновляем список только там, где есть с кем разговаривать.
		//
		// На Local это локальное ядро — оно есть всегда (RefreshAPIFunc сам
		// no-op, если ядро не запущено). На Remote собеседник появляется
		// только после выбора машины: без него запрос уходил с пустой группой
		// и возвращал «Daemon: group "" not found». Пустой список до выбора —
		// это честное состояние, а не сбой.
		needRefresh := item == coreTabItem
		if item == app.clashAPITab {
			_, _, hasMachine := GetLxdRemoteOverride()
			needRefresh = hasMachine
		}
		if needRefresh && controller.UIService != nil && controller.UIService.RefreshAPIFunc != nil {
			controller.UIService.RefreshAPIFunc()
		}
	}

	// Сохраняем оригинальный callback, который был установлен в CreateCoreDashboardTab
	originalUpdateCoreStatusFunc := controller.UIService.UpdateCoreStatusFunc

	// refreshCoreTabIcon — динамический emoji в табе Local по состоянию
	// sing-box. Перерисовывает label + дёргает AppTabs.Refresh чтобы
	// табстрип реально перечитал текст. Безопасно вызывать с UI-thread
	// (caller wrap'ит в fyne.Do).
	//
	// Status-indicator paradigm (как у media-плеера):
	//   ⏸️ Local  — stopped / idle (sing-box не запущен)
	//   ▶️ Local  — running (sing-box активен)
	//
	// Индикатор остался на Local, потому что показывает ЛОКАЛЬНОЕ ядро;
	// состояние удалённых машин видно в их строках на вкладке Remote.
	//
	// База берётся из локали (`Local` / `Локально` / etc), эмодзи приклеивается
	// тут чтобы не плодить per-state ключи в каждой локали.
	coreLabelBase := locale.T("Local")
	// Strip leading emoji + space from the locale base — text after the
	// first space character. Locale strings ship with a default ▶️ (or
	// previous attempt's icon) for the never-changed startup case; we
	// override per-state below so the leading emoji from locale gets
	// stripped to avoid double-icon.
	if i := indexEmojiSep(coreLabelBase); i > 0 {
		coreLabelBase = coreLabelBase[i:]
	}
	refreshCoreTabIcon := func() {
		var icon string
		switch {
		case controller.RunningState != nil && controller.RunningState.IsRunning():
			icon = "▶️"
		default:
			icon = "⏸️"
		}
		coreTabItem.Text = icon + " " + coreLabelBase
		app.tabs.Refresh()
	}

	// Регистрируем комбинированный callback для обновления состояния вкладки Servers
	// (legacy путь UpdateCoreStatusFunc — сохраняем пока на нём висят
	// другие потребители: core_dashboard_tab.updateRunningStatus, etc.)
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

	// Динамическая иконка Core подписывается на ТИПИЗИРОВАННЫЙ
	// EventBus (SPEC 047), а не на legacy UpdateCoreStatusFunc — это
	// канонический канал для cross-tab реакций на смену состояния
	// sing-box. Тот же канал слушает auto_update / proxy-active-changed
	// логика. Subscribe идемпотентен (одна handler-регистрация на NewApp).
	if controller.EventBus != nil {
		controller.EventBus.Subscribe(events.VpnStateChanged, func(_ events.Event) {
			fyne.Do(refreshCoreTabIcon)
		})
	}

	// SPEC 064: подписка на remote-override changes. Set/Clear из
	// gear-dialog'а в Servers tab → tab немедленно re-enable / re-disable.
	// Listener тонкий: только trigger UI refresh через fyne.Do.
	OnOverrideChanged(func() {
		fyne.Do(app.updateClashAPITabState)
	})

	// Local открыта на старте, но её слоты UIService перетёр конструктор
	// Remote (панели строятся обе, а слот один). Возвращаем владение той
	// панели, которая реально на экране, — иначе первый же авто-пинг или
	// ResetAPIState ушёл бы в невидимый список.
	app.localPanel.Activate(controller)

	// Авто-обновление Remote: на старте открыта Local, поэтому вкладка
	// неактивна, а окно — видимо (режим -tray скроет его сам, дёрнув
	// OnWindowHidden). Тикер поднимется при первом заходе на Remote.
	app.remotePanel.AutoRefresh().SetWindowVisible(true)
	// Пока окно в трее, обновлять нечего: данные никто не видит, а запросы
	// продолжали бы будить машину.
	if controller.UIService != nil {
		prevShown := controller.UIService.OnWindowShown
		controller.UIService.OnWindowShown = func() {
			if prevShown != nil {
				prevShown()
			}
			app.remotePanel.AutoRefresh().SetWindowVisible(true)
		}
		prevHidden := controller.UIService.OnWindowHidden
		controller.UIService.OnWindowHidden = func() {
			if prevHidden != nil {
				prevHidden()
			}
			app.remotePanel.AutoRefresh().SetWindowVisible(false)
		}
	}

	// Инициализируем состояние вкладки + первичный рендер иконки Core.
	// EventBus.Subscribe не fires backfill — рендерим вручную для startup'а.
	app.updateClashAPITabState()
	refreshCoreTabIcon()

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

// GetContent returns the root content for the main window (tabs alone when
// the overlay is disabled, tabs+overlay when enabled — see
// `wizardOverlayEnabled`).
func (a *App) GetContent() fyne.CanvasObject {
	if a.content != nil {
		return a.content
	}
	return a.tabs
}

// updateClashAPITabState — SPEC 064 update: tab **всегда** доступна.
//
// Раньше (до SPEC 064) tab disable'илась когда локальный sing-box не запущен.
// Это создало chicken-and-egg: gear-кнопка для настройки remote-endpoint
// живёт ВНУТРИ этой вкладки, юзер не мог до неё добраться из cold-start
// состояния (local не стартован, override ещё не задан → tab disabled →
// gear недоступен → override никогда не задать).
//
// Решение: вкладка постоянно enabled. Если ни local sing-box, ни remote
// override не активны — refresh-логика покажет «Clash API offline» в
// ApiStatusLabel, но badge + gear остаются нажимаемыми, и юзер может
// настроить remote или запустить local.
//
// Функция оставлена в качестве no-op-stub: вызывается из множества мест
// в кодовой базе (UpdateCoreStatusFunc, EventBus subscriber, OnOverrideChanged
// listener). Удалять hook не имеет смысла — нет cost'а, и позволяет в
// будущем вернуть гейтинг если потребуется.
func (a *App) updateClashAPITabState() {
	if a.clashAPITab == nil || a.tabs == nil {
		return
	}
	// SPEC 064: всегда enabled. Никаких DisableItem'ов больше нет.
	a.tabs.EnableItem(a.clashAPITab)
}

// indexEmojiSep — returns the byte index just AFTER the first ASCII
// space following an emoji prefix in s ("🚀 Core" → 5, "Core" → 0).
// Used to strip a baked-in emoji from the locale's app.tab.core string
// so we can substitute a state-driven one at runtime without each
// locale carrying separate `app.tab.core.running` keys.
func indexEmojiSep(s string) int {
	for i, r := range s {
		if r == ' ' {
			return i + 1 // byte index after the space
		}
	}
	return 0
}
