package ui

import (
	"errors"
	"image/color"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/components"
)

// ProxyListPanel — построенная панель списка прокси вместе с её СОБСТВЕННЫМИ
// виджетами и колбэками (SPEC 098).
//
// Local и Remote держат по независимому экземпляру: у каждого свой список
// узлов, своя выбранная группа и свои строки статуса. Раньше оба писали в
// общие слоты `UIService`, и второй конструктор затирал первый — обновления с
// Remote прилетали в виджеты Local, а переключение вкладки перетирало
// состояние соседней.
//
// Слоты `UIService` (ProxiesListWidget, ApiStatusLabel, ListStatusLabel,
// RefreshAPIFunc, ResetAPIStateFunc, AutoPingAfterConnectFunc) рассчитаны на
// одного владельца: их дёргают снаружи — main.go, core, горячие клавиши, —
// и адресовать они должны АКТИВНУЮ панель. Поэтому привязка делается не в
// конструкторе, а в Activate() при переключении вкладки.
type ProxyListPanel struct {
	// reloadGroups перечитывает список selector-групп у активного ядра.
	reloadGroups func()
	// Content — корневой контейнер панели (левая колонка вкладки).
	Content fyne.CanvasObject

	// scope — чьё состояние ведёт эта панель. Обе строятся на старте, когда
	// активна ScopeLocal, поэтому «активную» область спрашивать нельзя:
	// панель обязана знать свою.
	scope services.ProxyScope

	// Собственные виджеты и колбэки панели; переезжают в UIService на Activate.
	apiStatusLabel       *widget.Label
	listStatusLabel      *widget.Label
	proxiesList          *widget.List
	refreshAPI           func()
	resetAPIState        func()
	autoPingAfterConnect func()
	setEnabled           func(bool)
	clear                func()
}

// SetEnabled показывает или прячет содержимое панели.
//
// Нужен вкладке Remote: до Connect у неё нет собеседника, и вся панель —
// дропдаун групп, сортировка, ping — вела бы в пустоту. Показывать
// неработающие органы управления хуже, чем не показывать их вовсе.
func (p *ProxyListPanel) SetEnabled(on bool) {
	if p != nil && p.setEnabled != nil {
		p.setEnabled(on)
	}
}

// Activate делает панель владельцем разделяемых слотов UIService.
//
// Вызывается при выборе её вкладки: внешние потребители (авто-пинг после
// resume, ResetAPIState из core, Cmd+P) обязаны попадать в тот список,
// который пользователь сейчас видит.
func (p *ProxyListPanel) Activate(ac *core.AppController) {
	if p == nil || ac == nil || ac.UIService == nil {
		return
	}
	ac.UIService.ApiStatusLabel = p.apiStatusLabel
	ac.UIService.ListStatusLabel = p.listStatusLabel
	ac.UIService.ProxiesListWidget = p.proxiesList
	ac.UIService.RefreshAPIFunc = p.refreshAPI
	ac.UIService.ResetAPIStateFunc = p.resetAPIState
	ac.UIService.AutoPingAfterConnectFunc = p.autoPingAfterConnect
}

// Refresh перезагружает список панели, если она активна.
func (p *ProxyListPanel) Refresh() {
	if p != nil && p.refreshAPI != nil {
		p.refreshAPI()
	}
}

// Clear очищает список панели, НЕ ходя в сеть.
//
// Нужен при отключении от машины: собеседника уже нет, и Refresh ушёл бы
// за узлами с пустой группой, отвечая ошибкой на штатное действие.
func (p *ProxyListPanel) Clear() {
	if p != nil && p.clear != nil {
		p.clear()
	}
}

// CreateProxyListPanel строит панель списка прокси — левую колонку обеих
// вкладок (SPEC 098 §2.1).
//
// Один и тот же КОД на Local и Remote (поведение, сортировка, ping и
// переключение узла обязаны совпадать), но РАЗНЫЕ экземпляры: у каждой
// вкладки своё состояние. Чьи прокси показывать, решает активный транспорт
// (см. lxd_remote_override).
//
// Шапки управления машиной здесь нет: питанием локального ядра управляет
// правая колонка Local, удалённым — строка машины на Remote.
func CreateProxyListPanel(ac *core.AppController, scope services.ProxyScope) *ProxyListPanel {
	panel := &ProxyListPanel{scope: scope}
	apiStatusLabel := widget.NewLabel(locale.T("servers.status_not_checked"))
	panel.apiStatusLabel = apiStatusLabel
	status := widget.NewLabel(locale.T("servers.status_click_load"))
	panel.listStatusLabel = status

	var (
		selectorOptions []string
		defaultSelector string
	)
	// Локальный config.json описывает ТОЛЬКО локальное ядро, поэтому группы из
	// него читает лишь local-панель.
	//
	// Для remote-панели он не источник ни при каких условиях: до Connect у неё
	// вообще нет собеседника, а группы у машины свои. Без этого различия
	// дропдаун на Remote при старте заполнялся группами своего ядра
	// (`vpn ①`, `ru VPN`…) — чужой список под именем удалённой машины.
	if scope == services.ScopeLocal {
		var err error
		selectorOptions, defaultSelector, err = config.GetSelectorGroupsFromConfig(ac.FileService.ConfigPath)
		if err != nil {
			// Cold-start: config.json ещё не существует (пользователь не нажал
			// Save). Сваливаемся на "proxy-out" дефолт ниже — не повод писать
			// ERROR. На любую другую ошибку (битый JSON, нет experimental.clash_api)
			// логируем громко.
			if os.IsNotExist(err) {
				debuglog.DebugLog("clash_api_tab: config.json not present yet (cold start): %v", err)
			} else {
				debuglog.ErrorLog("clash_api_tab: failed to get selector groups: %v", err)
			}
		}
	}
	// SPEC 097: при подключении к удалённому демону локальный config.json не
	// описывает ЕГО ядро — группы спрашиваем у самого демона по gRPC.
	if remoteGroups, isRemote, groupsErr := RemoteDaemonGroups(); isRemote {
		// Выбрана удалённая машина: её группы — единственный корректный
		// источник. При ошибке НЕ откатываемся на локальный config.json —
		// он описывает другое ядро, и подстановка выдавала бы чужой список
		// за список выбранной машины.
		selectorOptions = remoteGroups
		defaultSelector = ""
		if len(remoteGroups) > 0 {
			defaultSelector = remoteGroups[0]
		} else if groupsErr != nil {
			debuglog.WarnLog("clash_api_tab: remote groups unavailable: %v", groupsErr)
		}
	}
	// Заглушка "proxy-out" — только для local: у него ядро есть всегда, и
	// пустой дропдаун был бы тупиком. На Remote до Connect список честно пуст.
	if len(selectorOptions) == 0 && scope == services.ScopeLocal {
		selectorOptions = []string{"proxy-out"}
	}
	selectedGroup := defaultSelector
	if selectedGroup == "" && len(selectorOptions) > 0 {
		selectedGroup = selectorOptions[0]
	}
	// Only set SelectedClashGroup if it's not already set (to preserve value from initialization)
	//
	// SPEC 098: пишем в СВОЮ область, а не в активную. Обе панели строятся на
	// старте, когда активна ScopeLocal, поэтому remote-панель записывала свою
	// группу в local-состояние, а её собственное оставалось пустым — и первый
	// же запрос к машине уходил с group="" («Daemon: group "" not found»),
	// хотя дропдаун показывал proxy-out.
	if ac.APIService != nil {
		currentGroup := ac.APIService.SelectedClashGroupIn(panel.scope)
		if currentGroup == "" {
			ac.APIService.SetSelectedClashGroupIn(panel.scope, selectedGroup)
		} else {
			// Use existing value, but update selectedGroup variable for UI
			selectedGroup = currentGroup
		}
	}

	var (
		groupSelect                      *widget.Select
		suppressSelectCallback           bool
		applySavedSort                   func()                      // Объявляем переменную заранее, значение будет присвоено позже
		pingAllGeneration                uint64                      // инкремент при новом «ping all» — устаревшие воркеры не трогают UI
		selectedProxyNames               = make(map[string]struct{}) // выделение по тегу (устойчиво к фильтру/сортировке)
		selectionAnchorVis               = -1                        // якорь для Shift+клик (индекс в текущем отображаемом списке)
		hidePingErrors                   bool                        // скрывать в списке прокси с Delay == -1 (ошибка пинга)
		reconcileListSelection           func()
		applyServersPointerSelection     func(rowID int, proxyName string, tapMods fyne.KeyModifier)
		refreshServersProxySelectionUI   func()
		exportShareURIsButton            *ttwidget.Button
		syncExportShareURIsButtonTooltip func()
	)

	// --- Логика обновления и сброса ---

	onLoadAndRefreshProxies := func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		_, _, clashAPIEnabled, _ := EffectiveClashAPIConfigIn(ac, scope)
		if !clashAPIEnabled {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
			if ac.UIService.ListStatusLabel != nil {
				ac.UIService.ListStatusLabel.SetText(locale.T("servers.status_clash_api_disabled"))
			}
			return
		}

		group := selectedGroup
		if group == "" {
			return
		}
		if ac.UIService.ListStatusLabel != nil {
			ac.UIService.ListStatusLabel.SetText(locale.Tf("servers.status_loading", group))
		}
		go func(group string) {
			if platform.IsSleeping() {
				return
			}
			// SPEC 064: capture generation для drop-stale если override
			// сменился пока запрос летит.
			gen := CurrentGeneration()
			transport := EffectiveProxyTransportIn(ac, scope)
			proxies, now, err := transport.GroupProxies(group)
			fyne.Do(func() {
				if gen != CurrentGeneration() {
					// Endpoint override сменился — результат stale, drop без write в UI.
					return
				}
				if err != nil {
					ShowError(ac.UIService.MainWindow, err)
					if ac.UIService.ListStatusLabel != nil {
						ac.UIService.ListStatusLabel.SetText("Error: " + err.Error())
					}
					return
				}

				// Preserve local ping state (Delay / Error) when refreshing from API so switching tabs does not reset button text.
				oldProxies := ac.GetProxiesList()
				for i := range proxies {
					for _, old := range oldProxies {
						if old.Name == proxies[i].Name {
							proxies[i].Delay = old.Delay
							break
						}
					}
				}
				// Keep "direct-out" and active proxy at the top regardless of sort.
				ac.SetProxiesList(reorderWithPinned(ac, proxies))
				ac.SetActiveProxyName(now)

				// Применяем сохраненную сортировку после загрузки
				if applySavedSort != nil {
					applySavedSort()
				}

				// Примечание: автоматическое переключение на сохраненный прокси выполняется
				// только в AutoLoadProxies при старте sing-box, здесь только обновляем список

				if ac.UIService.ProxiesListWidget != nil {
					ac.UIService.ProxiesListWidget.Refresh()
					ac.UIService.ProxiesListWidget.ScrollToTop()
				}
				if reconcileListSelection != nil {
					reconcileListSelection()
				}

				if ac.UIService.ListStatusLabel != nil {
					ac.UIService.ListStatusLabel.SetText(locale.Tf("servers.status_loaded", group, textnorm.NormalizeProxyDisplay(now)))
				}

				// Update tray menu with new proxy list
				if ac.UIService != nil && ac.UIService.UpdateTrayMenuFunc != nil {
					ac.UIService.UpdateTrayMenuFunc()
				}
			})
		}(group)
	}

	// Функция для обновления списка селекторов (вызывается когда sing-box
	// запущен и конфиг загружен, а также при смене источника).
	//
	// SPEC 097: источник групп зависит от ВЫБРАННОЙ машины. Для удалённой
	// локальный config.json описывает не её ядро — спрашиваем группы у
	// самого демона по gRPC. Без этого список групп оставался от локального
	// ядра, хотя прокси внутри уже приезжали с роутера.
	updateSelectorList := func() {
		var updatedSelectorOptions []string
		var updatedDefaultSelector string
		var err error
		if remoteGroups, isRemote, groupsErr := RemoteDaemonGroups(); isRemote {
			updatedSelectorOptions = remoteGroups
			if len(remoteGroups) > 0 {
				updatedDefaultSelector = remoteGroups[0]
			} else if groupsErr != nil {
				// Машина недоступна или ядро не запущено — список пуст, но
				// локальные группы подставлять нельзя: это чужое ядро.
				debuglog.WarnLog("clash_api_tab: remote groups unavailable: %v", groupsErr)
			}
		} else if scope == services.ScopeLocal {
			updatedSelectorOptions, updatedDefaultSelector, err = config.GetSelectorGroupsFromConfig(ac.FileService.ConfigPath)
		} else if groupSelect != nil {
			// Remote без выбранной машины: собеседника нет, значит нет и групп.
			// Оставить прежние значило бы показывать группы отключённой машины
			// (а на старте — локальные) как её собственные.
			selectorOptions = nil
			selectedGroup = ""
			groupSelect.SetOptions(nil)
			suppressSelectCallback = true
			groupSelect.SetSelected("")
			suppressSelectCallback = false
			if ac.APIService != nil {
				ac.APIService.SetSelectedClashGroupIn(panel.scope, "")
			}
			return
		}
		if err == nil && len(updatedSelectorOptions) > 0 && groupSelect != nil {
			// Обновляем и переменную selectorOptions, и виджет groupSelect
			selectorOptions = updatedSelectorOptions
			groupSelect.SetOptions(updatedSelectorOptions)

			// Обновить selectedGroup если текущий выбор больше не доступен
			currentSelected := selectedGroup
			found := false
			for _, opt := range updatedSelectorOptions {
				if opt == currentSelected {
					found = true
					break
				}
			}
			if !found {
				if updatedDefaultSelector != "" {
					selectedGroup = updatedDefaultSelector
				} else if len(updatedSelectorOptions) > 0 {
					selectedGroup = updatedSelectorOptions[0]
				}
				suppressSelectCallback = true
				groupSelect.SetSelected(selectedGroup)
				suppressSelectCallback = false
				if ac.APIService != nil {
					ac.APIService.SetSelectedClashGroupIn(panel.scope, selectedGroup)
				}
			}
		}
	}

	onTestAPIConnection := func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		// Daemon-режим: Clash API нет, управление по gRPC. «Тест» = проверка
		// gRPC-транспорта через загрузку групп; статус показываем как gRPC,
		// без Clash-ошибок.
		// Подключённая машина lxd — тот же gRPC-путь, хотя локальный бэкенд
		// остаётся classic: у remote-конфига Clash API нет, и HTTP-ветка ушла
		// бы на пустой baseURL (`unsupported protocol scheme ""`).
		_, _, lxdConnected := GetLxdRemoteOverride()
		if ac.BackendMode() == core.BackendDaemon || (scope == services.ScopeRemote && lxdConnected) {
			go func() {
				_, _, err := EffectiveProxyTransportIn(ac, scope).GroupProxies(ac.APIService.GetSelectedClashGroup())
				fyne.Do(func() {
					if err != nil {
						ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_grpc_off"))
						// FailedPrecondition = канал и сопряжение живы, но
						// ядро внутри демона не запущено — сырой RPC-текст
						// пугает, а лекарство одно: нажать Start.
						if strings.Contains(err.Error(), "service is not started") {
							ShowErrorText(ac.UIService.MainWindow, "Daemon",
								locale.T("servers.error_daemon_core_idle"))
							return
						}
						// Машина недоступна по сети (роутер перезагружается,
						// сменился Wi-Fi, кабель): показывать сырой
						// «rpc error: code = Unavailable desc = transport:
						// Error while dialing…» — значит пугать текстом, из
						// которого пользователю нечего извлечь.
						if isUnreachableErr(err) {
							ShowErrorText(ac.UIService.MainWindow, "Daemon",
								locale.T("servers.error_daemon_unreachable"))
							return
						}
						// Группа машины ещё не прочитана — это не сбой, а
						// момент до первого чтения списка. Диалог тут пугает
						// на ровном месте; хватает строки состояния.
						if services.IsRemoteGroupUnknown(err) {
							if panel.listStatusLabel != nil {
								panel.listStatusLabel.SetText(locale.T("remote.proxies.groups_unknown"))
							}
							return
						}
						ShowError(ac.UIService.MainWindow, err)
						return
					}
					ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_grpc_on"))
					updateSelectorList()
					onLoadAndRefreshProxies()
				})
			}()
			return
		}
		_, _, clashAPIEnabled, _ := EffectiveClashAPIConfigIn(ac, scope)
		if !clashAPIEnabled {
			ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_clash_api_off_config"))
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
			return
		}
		go func() {
			if platform.IsSleeping() {
				return
			}
			baseURL, token, _, _ := EffectiveClashAPIConfigIn(ac, scope)
			var err error
			for attempt := 0; attempt < clashAPITestMaxAttempts; attempt++ {
				if platform.IsSleeping() {
					return
				}
				err = api.TestAPIConnection(baseURL, token)
				if err == nil {
					break
				}
				if errors.Is(err, api.ErrPlatformInterrupt) {
					return
				}
				if attempt < clashAPITestMaxAttempts-1 {
					time.Sleep(clashAPITestRetryInterval)
				}
			}
			fyne.Do(func() {
				if err != nil {
					ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_clash_api_off_error"))
					ShowError(ac.UIService.MainWindow, err)
					return
				}
				ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_clash_api_on"))
				// Обновить список селекторов после успешного подключения (sing-box запущен, конфиг загружен)
				updateSelectorList()
				onLoadAndRefreshProxies()
			})
		}()
	}

	onResetAPIState := func() {
		debuglog.InfoLog("clash_api_tab: Resetting API state.")
		// SPEC 097: сменился источник — список групп берём у новой машины.
		// Сам сброс идёт до перечитывания прокси, поэтому группы успевают
		// обновиться раньше, чем поедет запрос за списком узлов.
		updateSelectorList()
		ac.SetProxiesList([]api.ProxyInfo{})
		ac.SetActiveProxyName("")
		ac.SetSelectedIndex(-1)
		selectedProxyNames = make(map[string]struct{})
		selectionAnchorVis = -1
		fyne.Do(func() {
			// Пишем в ЛЕЙБЛЫ СВОЕЙ панели, а не в глобальные слоты UIService:
			// «Sing-box is stopped» — про локальное ядро, и на вкладке Remote
			// эта надпись противоречила зелёной машине со статусом started.
			if panel.apiStatusLabel != nil {
				panel.apiStatusLabel.SetText(locale.T("servers.status_not_running"))
			}
			if panel.listStatusLabel != nil {
				if panel.scope == services.ScopeRemote {
					panel.listStatusLabel.SetText(locale.T("remote.proxies.core_stopped"))
				} else {
					panel.listStatusLabel.SetText(locale.T("servers.status_singbox_stopped"))
				}
			}
			if panel.proxiesList != nil {
				panel.proxiesList.Refresh()
			}
			if syncExportShareURIsButtonTooltip != nil {
				syncExportShareURIsButtonTooltip()
			}
			// Update tray menu when API state is reset
			if ac.UIService != nil && ac.UIService.UpdateTrayMenuFunc != nil {
				ac.UIService.UpdateTrayMenuFunc()
			}
		})
	}

	// --- Регистрация колбэков ---
	// Панель держит их у себя (Activate переносит в UIService при выборе её
	// вкладки), а в UIService пишет и сразу: первая построенная панель должна
	// быть рабочей ещё до первого переключения вкладок.
	panel.refreshAPI = onTestAPIConnection
	panel.resetAPIState = onResetAPIState
	// Очистка при отключении: тот же сброс состояния, но статус-строка
	// объясняет причину пустого списка — машина отключена, а не ядро упало.
	panel.clear = func() {
		onResetAPIState()
		fyne.Do(func() {
			if panel.listStatusLabel != nil {
				panel.listStatusLabel.SetText(locale.T("remote.proxies.no_machine"))
			}
		})
	}
	// Колбэки панели складываем в неё саму, а в UIService их ставит только
	// Activate — то есть панель ТОЙ вкладки, которую пользователь видит.
	//
	// Раньше их захватывал конструктор, и владельцем оставалась панель,
	// построенная последней (Remote). Из-за этого падение ЛОКАЛЬНОГО ядра
	// дёргало remote-панель: UpdateUI зовёт ResetAPIStateFunc на состоянии
	// «Down», та шла в сеть к машине и перерисовывала список. На отвалившемся
	// роутере (no route to host) это кончалось падением всего приложения.
	// --- Вспомогательная функция для пинга ---
	// Delay in ProxyInfo: >0 = ms, 0 = not pinged, -1 = error (so updateItem shows correct text after list refresh).
	pingProxy := func(proxyName string, button interface{ SetText(string) }) {
		go func() {
			if platform.IsSleeping() {
				return
			}
			fyne.Do(func() { button.SetText("...") })
			transport := EffectiveProxyTransportIn(ac, scope)
			delay, err := transport.Delay(proxyName)
			fyne.Do(func() {
				proxies := ac.GetProxiesList()
				for i := range proxies {
					if proxies[i].Name == proxyName {
						if err != nil {
							proxies[i].Delay = -1
							if ac.APIService != nil {
								ac.APIService.SetLastPingError(proxyName, err.Error())
							}
							button.SetText(locale.T("servers.ping_button_error"))
							// Set tooltip immediately so hover shows error without needing a list refresh.
							if tb, ok := button.(interface{ SetToolTip(string) }); ok && ac.APIService != nil {
								tb.SetToolTip(ac.APIService.GetLastPingError(proxyName))
							}
							status.SetText(locale.Tf("servers.status_delay_error", err.Error()))
						} else {
							proxies[i].Delay = delay
							if ac.APIService != nil {
								ac.APIService.SetLastPingError(proxyName, "")
							}
							button.SetText(locale.Tf("servers.ping_format_ms", delay))
							if tb, ok := button.(interface{ SetToolTip(string) }); ok {
								tb.SetToolTip("")
							}
							status.SetText(locale.Tf("servers.status_delay_format", delay, textnorm.NormalizeProxyDisplay(proxyName)))
						}
						ac.SetProxiesList(proxies)
						break
					}
				}
				if reconcileListSelection != nil {
					reconcileListSelection()
				}
			})
		}()
	}

	// Срез для отображения в списке (полный или без прокси с ошибкой пинга).
	// Выбранная строка не скрывается, чтобы не терять контекст при фильтре.
	proxiesForListView := func() []api.ProxyInfo {
		all := ac.GetProxiesList()
		if !hidePingErrors {
			return all
		}
		out := make([]api.ProxyInfo, 0, len(all))
		for i := range all {
			_, sel := selectedProxyNames[all[i].Name]
			if all[i].Delay != -1 || sel {
				out = append(out, all[i])
			}
		}
		return out
	}

	// --- Создание виджета списка ---

	createItem := func() fyne.CanvasObject {
		background := canvas.NewRectangle(color.Transparent)
		background.CornerRadius = 5

		// SPEC 095 — имя и подзаголовок обязаны уместиться в ПРЕЖНЮЮ высоту
		// строки: с ней список был плотным и читался хорошо.
		//
		// Оба текста — canvas.Text, а не widget.Label: Label добавляет
		// вертикальные отступы под размер шрифта темы, и пара таких виджетов
		// удваивает высоту строки. canvas.Text занимает ровно свою строку.
		nameText := canvas.NewText("", theme.Color(theme.ColorNameForeground))
		nameText.TextSize = serversNameTextSize
		nameText.TextStyle.Bold = true

		// Подзаголовок резервирует место ВСЕГДА, даже пустой: widget.List
		// берёт высоту строки из первого созданного элемента, и скрытый
		// подзаголовок сделал бы строки разной высоты.
		// БЕЗ Italic: у эмодзи курсивного глифа нет, Fyne подставляет его из
		// emoji-шрифта с другой базовой линией — значки режима (🎯/⚖️/🔀) и
		// флаги повисали выше наклонного текста. Подзаголовок и так отличим
		// от имени размером и цветом.
		subtitleText := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
		subtitleText.TextSize = serversSubtitleTextSize

		titleBox := container.New(
			tightVBoxLayout{gap: serversTitleSubtitleGap},
			nameText, subtitleText,
		)

		switchButton := widget.NewButton("▶️", nil)

		rowGutter := canvas.NewRectangle(color.Transparent)
		rowGutter.SetMinSize(fyne.NewSize(components.ScrollbarGutterWidth, 0))

		// Замер — КЛИКАБЕЛЬНЫЙ ЦВЕТНОЙ ТЕКСТ вместо кнопки.
		//
		// widget.Button не позволяет покрасить свой текст: Fyne даёт только
		// Importance, а тот заливает весь фон — число тонет в заливке. Здесь
		// цвет несёт само значение, а клик обрабатывает TapWrap (он же ставит
		// курсор-указатель, чтобы зона читалась как кликабельная).
		delayText := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
		delayText.TextSize = serversDelayTextSize
		delayText.TextStyle.Bold = true
		delayText.Alignment = fyne.TextAlignCenter

		// Фон-подложка: даёт зоне клика видимые границы и постоянную ширину,
		// иначе кнопка ▶ прыгала бы по горизонтали от строки к строке
		// («31 ms» против «1194 ms»).
		delayBackground := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
		delayBackground.CornerRadius = 4
		delayBackground.SetMinSize(fyne.NewSize(serversDelayColumnWidth, serversDelayCellHeight))

		delayTappable := fynewidget.NewTapWrap(
			container.NewStack(delayBackground, container.NewCenter(delayText)), nil)

		buttons := container.NewHBox(delayTappable, switchButton, rowGutter)

		// titleBox без Center: тот сжал бы его по ширине, и длинное имя
		// перестало бы использовать всю строку.
		content := container.NewBorder(
			nil, nil, nil,
			container.NewCenter(buttons),
			titleBox,
		)

		// Padding БЕЗ правой стороны: справа зазор уже даёт rowGutter, и
		// обычный NewPadded складывался с ним в ~18pt вместо 14.
		paddedContent := components.NewListRowPadded(content)
		stack := container.NewStack(background, paddedContent)
		return fynewidget.NewSecondaryTapWrap(stack)
	}

	updateItem := func(id int, o fyne.CanvasObject) {
		proxies := proxiesForListView()
		if id < 0 || id >= len(proxies) {
			return
		}
		proxyInfo := proxies[id]

		wrap := o.(*fynewidget.SecondaryTapWrap)
		stack := wrap.Content.(*fyne.Container)
		background := stack.Objects[0].(*canvas.Rectangle)
		paddedContent := stack.Objects[1].(*fyne.Container)
		content := paddedContent.Objects[0].(*fyne.Container)

		// Border кладёт объекты в порядке [center, right]: titleBox, кнопки.
		titleBox := content.Objects[0].(*fyne.Container)
		nameText := titleBox.Objects[0].(*canvas.Text)
		subtitleText := titleBox.Objects[1].(*canvas.Text)

		// content.Objects: [titleBox, центрированные кнопки].
		// buttonsBox: [кликабельный замер, ▶, распорка].
		buttonsCenter := content.Objects[1].(*fyne.Container)
		buttonsBox := buttonsCenter.Objects[0].(*fyne.Container)

		delayTappable := buttonsBox.Objects[0].(*fynewidget.TapWrap)
		delayStack := delayTappable.Content.(*fyne.Container)
		delayBackground := delayStack.Objects[0].(*canvas.Rectangle)
		delayText := delayStack.Objects[1].(*fyne.Container).Objects[0].(*canvas.Text)

		switchButton := buttonsBox.Objects[1].(*widget.Button)

		// canvas.Text не умеет ellipsis сам — режем по длине, иначе длинное
		// имя растянет строку и вытолкнет кнопки за край.
		nameText.Text = truncateRunes(proxyInfo.DisplayOrName(), serversNameMaxRunes)
		nameText.Color = theme.Color(theme.ColorNameForeground)
		nameText.Refresh()

		// SPEC 095 — подзаголовок из config.json. Узел, которого там нет
		// (гонка перегенерации), просто остаётся без подзаголовка.
		subtitleText.Text = truncateSubtitle(serversNodeSubtitle(ac, proxyInfo))
		subtitleText.Color = theme.Color(theme.ColorNamePlaceHolder)
		subtitleText.Refresh()

		// Замер — цветное число на нейтральной подложке; клик по нему
		// запускает новый замер (обработчик ниже).
		delayText.Text = serversDelayText(proxyInfo.Delay)
		delayText.Color = serversDelayColor(proxyInfo.Delay)
		delayText.Refresh()

		delayBackground.FillColor = theme.Color(theme.ColorNameInputBackground)
		delayBackground.Refresh()

		// Обновляем фон
		if proxyInfo.Name == ac.GetActiveProxyName() {
			background.FillColor = color.NRGBA{R: 144, G: 238, B: 144, A: 128} // Зеленый для активного
		} else if _, sel := selectedProxyNames[proxyInfo.Name]; sel {
			background.FillColor = color.NRGBA{R: 135, G: 206, B: 250, A: 128} // Синий для выделенных (один или несколько)
		} else {
			background.FillColor = color.Transparent
		}
		background.Refresh()

		// Обновляем колбэки кнопок
		proxyNameForCallback := proxyInfo.Name
		rowID := id

		wrap.OnPrimary = func(tapMods fyne.KeyModifier) {
			if applyServersPointerSelection != nil {
				applyServersPointerSelection(rowID, proxyNameForCallback, tapMods)
			}
		}
		wrap.OnSecondary = func(pe *fyne.PointEvent) {
			if ac.UIService == nil || ac.UIService.MainWindow == nil {
				return
			}
			_, inSet := selectedProxyNames[proxyInfo.Name]
			if len(selectedProxyNames) <= 1 || !inSet {
				selectedProxyNames = map[string]struct{}{proxyInfo.Name: {}}
				selectionAnchorVis = rowID
			}
			if refreshServersProxySelectionUI != nil {
				refreshServersProxySelectionUI()
			}

			win := ac.UIService.MainWindow
			menu := serversProxyContextMenu(ac, status, win, proxyInfo)
			pop := widget.NewPopUpMenu(menu, win.Canvas())
			pop.ShowAtPosition(pe.AbsolutePosition)
		}

		// Замер запускается кликом по самому числу — кнопки больше нет.
		// canvas.Text не реализует SetText, поэтому pingProxy получает
		// тонкий адаптер, который заодно перерисовывает текст.
		delaySetter := &canvasTextSetter{text: delayText}
		delayTappable.OnTapped = func() {
			pingProxy(proxyNameForCallback, delaySetter)
		}

		switchButton.OnTapped = func() {
			if ac.APIService == nil {
				ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
				return
			}
			_, _, clashAPIEnabled, _ := EffectiveClashAPIConfigIn(ac, scope)
			if !clashAPIEnabled {
				ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
				return
			}
			go func(group string) {
				// Через EffectiveProxyTransport: учитывает remote-override
				// (SPEC 064) и gRPC-транспорт daemon-режима — прямой
				// APIService.SwitchProxy их не видит.
				err := ac.APIService.SwitchProxyVia(EffectiveProxyTransportIn(ac, scope), group, proxyNameForCallback)
				fyne.Do(func() {
					if err != nil {
						ShowError(ac.UIService.MainWindow, err)
						status.SetText(locale.Tf("servers.status_switch_error", err.Error()))
					} else {
						// Active name already set in APIService.SwitchProxy; pin active row to top like after API load.
						ac.SetProxiesList(reorderWithPinned(ac, ac.GetProxiesList()))
						if ac.UIService.ProxiesListWidget != nil {
							ac.UIService.ProxiesListWidget.Refresh()
							ac.UIService.ProxiesListWidget.ScrollToTop()
						}
						if reconcileListSelection != nil {
							reconcileListSelection()
						}
						pingProxy(proxyNameForCallback, delaySetter)
						if ac.UIService.ListStatusLabel != nil {
							ac.UIService.ListStatusLabel.SetText(locale.Tf("servers.status_switched", group, textnorm.NormalizeProxyDisplay(proxyNameForCallback)))
						}
					}
				})
			}(selectedGroup)
		}
	}

	proxiesListWidget := widget.NewList(
		func() int { return len(proxiesForListView()) },
		createItem,
		updateItem,
	)

	syncServersListWidgetFromSelection := func() {
		if proxiesListWidget == nil {
			return
		}
		vis := proxiesForListView()
		n := len(selectedProxyNames)
		if n == 0 {
			ac.SetSelectedIndex(-1)
			proxiesListWidget.UnselectAll()
			return
		}
		if n == 1 {
			var onlyName string
			for k := range selectedProxyNames {
				onlyName = k
				break
			}
			for i := range vis {
				if vis[i].Name == onlyName {
					proxiesListWidget.Select(i)
					ac.SetSelectedIndex(i)
					return
				}
			}
			ac.SetSelectedIndex(-1)
			proxiesListWidget.UnselectAll()
			return
		}
		ac.SetSelectedIndex(-1)
		proxiesListWidget.UnselectAll()
	}

	refreshServersSelectionStatus := func() {
		n := len(selectedProxyNames)
		if n == 0 {
			status.SetText(locale.T("servers.status_selected_none"))
			return
		}
		if n == 1 {
			var name string
			for k := range selectedProxyNames {
				name = k
				break
			}
			for _, p := range ac.GetProxiesList() {
				if p.Name == name {
					status.SetText(locale.Tf("servers.status_selected", p.DisplayOrName()))
					return
				}
			}
			status.SetText(locale.Tf("servers.status_selected", textnorm.NormalizeProxyDisplay(name)))
			return
		}
		status.SetText(locale.Tf("servers.status_selected_multi", n))
	}

	refreshServersProxySelectionUI = func() {
		syncServersListWidgetFromSelection()
		refreshServersSelectionStatus()
		proxiesListWidget.Refresh()
		if syncExportShareURIsButtonTooltip != nil {
			syncExportShareURIsButtonTooltip()
		}
	}

	applyServersPointerSelection = func(rowID int, proxyName string, tapMods fyne.KeyModifier) {
		if ac.UIService == nil || proxiesListWidget == nil {
			return
		}
		vis := proxiesForListView()
		if rowID < 0 || rowID >= len(vis) {
			return
		}
		mods := tapMods
		if mods == 0 {
			mods = keyModifiers()
		}
		shift := mods&fyne.KeyModifierShift != 0
		toggle := (mods&fyne.KeyModifierControl != 0) || (mods&fyne.KeyModifierSuper != 0)

		if shift && selectionAnchorVis >= 0 && selectionAnchorVis < len(vis) {
			lo, hi := selectionAnchorVis, rowID
			if lo > hi {
				lo, hi = hi, lo
			}
			selectedProxyNames = make(map[string]struct{})
			for i := lo; i <= hi && i < len(vis); i++ {
				selectedProxyNames[vis[i].Name] = struct{}{}
			}
		} else if toggle {
			if _, ok := selectedProxyNames[proxyName]; ok {
				delete(selectedProxyNames, proxyName)
			} else {
				selectedProxyNames[proxyName] = struct{}{}
			}
			selectionAnchorVis = rowID
		} else {
			selectedProxyNames = map[string]struct{}{proxyName: {}}
			selectionAnchorVis = rowID
		}

		refreshServersProxySelectionUI()
	}

	proxiesListWidget.OnSelected = func(id int) {
		vis := proxiesForListView()
		if id >= 0 && id < len(vis) {
			selectedProxyNames = map[string]struct{}{vis[id].Name: {}}
			selectionAnchorVis = id
			ac.SetSelectedIndex(id)
		} else {
			selectedProxyNames = make(map[string]struct{})
			selectionAnchorVis = -1
			ac.SetSelectedIndex(-1)
		}
		refreshServersProxySelectionUI()
	}

	reconcileListSelection = func() {
		if proxiesListWidget == nil {
			return
		}
		all := ac.GetProxiesList()
		for name := range selectedProxyNames {
			found := false
			for i := range all {
				if all[i].Name == name {
					found = true
					break
				}
			}
			if !found {
				delete(selectedProxyNames, name)
			}
		}
		disp := proxiesForListView()
		if selectionAnchorVis < 0 || selectionAnchorVis >= len(disp) {
			selectionAnchorVis = -1
			for i := range disp {
				if _, ok := selectedProxyNames[disp[i].Name]; ok {
					selectionAnchorVis = i
					break
				}
			}
		}
		refreshServersProxySelectionUI()
	}

	panel.proxiesList = proxiesListWidget

	// Переменные для отслеживания направления сортировки
	sortNameAscending := true
	sortDelayAscending := true
	// Переменная для отслеживания текущего типа сортировки ("" - нет сортировки, "name" - по имени, "delay" - по задержке)
	currentSortType := ""
	// Сохраненное направление сортировки (используется при восстановлении сортировки)
	savedSortNameAscending := true
	savedSortDelayAscending := true

	// Функция сортировки по имени с указанным направлением
	sortByName := func(ascending bool) {
		proxies := ac.GetProxiesList()
		if len(proxies) == 0 {
			return
		}
		sorted := make([]api.ProxyInfo, len(proxies))
		copy(sorted, proxies)
		// Сортировка по имени
		if ascending {
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].DisplayOrName() < sorted[j].DisplayOrName()
			})
			status.SetText(locale.T("servers.status_sorted_name_az"))
		} else {
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].DisplayOrName() > sorted[j].DisplayOrName()
			})
			status.SetText(locale.T("servers.status_sorted_name_za"))
		}
		currentSortType = "name"
		savedSortNameAscending = ascending // Сохраняем направление для восстановления
		ac.SetProxiesList(reorderWithPinned(ac, sorted))
		if ac.UIService.ProxiesListWidget != nil {
			ac.UIService.ProxiesListWidget.Refresh()
		}
		reconcileListSelection()
	}

	// Функция сортировки по задержке с указанным направлением
	sortByDelay := func(ascending bool) {
		proxies := ac.GetProxiesList()
		if len(proxies) == 0 {
			return
		}
		sorted := make([]api.ProxyInfo, len(proxies))
		copy(sorted, proxies)

		if ascending {
			// Сортировка по задержке (меньше - лучше), прокси без задержки в конец
			sort.Slice(sorted, func(i, j int) bool {
				delayI := sorted[i].Delay
				delayJ := sorted[j].Delay
				// Прокси без задержки (0 или отрицательная) идут в конец
				if delayI <= 0 {
					delayI = 999999
				}
				if delayJ <= 0 {
					delayJ = 999999
				}
				return delayI < delayJ
			})
			status.SetText(locale.T("servers.status_sorted_delay_fast"))
		} else {
			// Сортировка по задержке (больше - выше), прокси без задержки в начало
			sort.Slice(sorted, func(i, j int) bool {
				delayI := sorted[i].Delay
				delayJ := sorted[j].Delay
				// Прокси без задержки (0 или отрицательная) идут в начало
				if delayI <= 0 {
					delayI = -1
				}
				if delayJ <= 0 {
					delayJ = -1
				}
				return delayI > delayJ
			})
			status.SetText(locale.T("servers.status_sorted_delay_slow"))
		}

		currentSortType = "delay"
		savedSortDelayAscending = ascending // Сохраняем направление для восстановления
		ac.SetProxiesList(reorderWithPinned(ac, sorted))
		if ac.UIService.ProxiesListWidget != nil {
			ac.UIService.ProxiesListWidget.Refresh()
		}
		reconcileListSelection()
	}

	// Функция для применения сохраненной сортировки (присваиваем значение переменной, объявленной ранее)
	applySavedSort = func() {
		if currentSortType == "" {
			return // Сортировка не применялась, оставляем список как есть
		}
		if currentSortType == "name" {
			sortByName(savedSortNameAscending) // Используем сохраненное направление
		} else if currentSortType == "delay" {
			sortByDelay(savedSortDelayAscending) // Используем сохраненное направление
		}
	}

	// --- Функция массового пинга всех прокси ---
	pingAllProxies := func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		_, _, clashAPIEnabled, _ := EffectiveClashAPIConfigIn(ac, scope)
		if !clashAPIEnabled {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
			return
		}
		proxies := ac.GetProxiesList()
		if len(proxies) == 0 {
			status.SetText(locale.T("servers.status_no_proxies"))
			return
		}
		status.SetText(locale.Tf("servers.status_pinging", len(proxies)))

		go func() {
			gen := atomic.AddUint64(&pingAllGeneration, 1)
			transport := EffectiveProxyTransportIn(ac, scope)

			type pingJob struct {
				Name string
			}

			jobs := make(chan pingJob)
			done := make(chan struct{})
			total := len(proxies)
			completed := 0
			concurrency := api.GetPingTestAllConcurrency()
			if concurrency <= 0 {
				concurrency = 1
			}
			if concurrency > total {
				concurrency = total
			}

			worker := func() {
				for job := range jobs {
					delay, err := transport.Delay(job.Name)
					fyne.Do(func() {
						if atomic.LoadUint64(&pingAllGeneration) != gen {
							return
						}
						updatedProxies := ac.GetProxiesList()
						for j := range updatedProxies {
							if updatedProxies[j].Name == job.Name {
								if err != nil {
									updatedProxies[j].Delay = -1
									if ac.APIService != nil {
										ac.APIService.SetLastPingError(job.Name, err.Error())
									}
								} else {
									updatedProxies[j].Delay = delay
									if ac.APIService != nil {
										ac.APIService.SetLastPingError(job.Name, "")
									}
								}
								break
							}
						}
						ac.SetProxiesList(updatedProxies)
						if ac.UIService.ProxiesListWidget != nil {
							ac.UIService.ProxiesListWidget.Refresh()
						}
						reconcileListSelection()
						completed++
						status.SetText(locale.Tf("servers.status_pinging_progress", completed, total))
					})
				}
				done <- struct{}{}
			}

			for i := 0; i < concurrency; i++ {
				go worker()
			}

			for _, proxy := range proxies {
				jobs <- pingJob{Name: proxy.Name}
			}
			close(jobs)

			for i := 0; i < concurrency; i++ {
				<-done
			}

			fyne.Do(func() {
				if atomic.LoadUint64(&pingAllGeneration) != gen {
					return
				}
				status.SetText(locale.Tf("servers.status_ping_completed", len(proxies)))
			})
		}()
	}

	// --- Сборка всего контента ---
	scrollContainer := container.NewScroll(proxiesListWidget)
	scrollContainer.SetMinSize(fyne.NewSize(0, 300))

	// Кнопка сортировки по алфавиту (слева)
	var sortByNameButton *ttwidget.Button
	sortByNameButton = ttwidget.NewButton("↑", func() {
		// Применяем сортировку с текущим направлением (сохранит его в savedSortNameAscending)
		sortByName(sortNameAscending)
		// Переключаем направление для следующего раза
		sortNameAscending = !sortNameAscending
		// Обновляем иконку для следующего нажатия
		if sortNameAscending {
			sortByNameButton.SetText("↑")
		} else {
			sortByNameButton.SetText("↓")
		}
	})
	sortByNameButton.SetToolTip(locale.T("servers.tooltip_sort_by_name"))
	sortNameLabel := widget.NewLabel(locale.T("servers.label_sort_by_name"))

	exportShareURIsButton = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		if ac.UIService == nil || ac.UIService.MainWindow == nil {
			return
		}
		win := ac.UIService.MainWindow
		if ac.FileService == nil || strings.TrimSpace(ac.FileService.ConfigPath) == "" {
			ShowErrorText(win, locale.T("app.tab.servers"), locale.T("servers.error_export_no_config"))
			return
		}
		allProxies := ac.GetProxiesList()
		if len(allProxies) == 0 {
			status.SetText(locale.T("servers.status_no_proxies"))
			return
		}
		visible := proxiesForListView()
		if len(visible) == 0 {
			status.SetText(locale.T("servers.status_export_nothing_visible"))
			return
		}
		var rowsForExport []api.ProxyInfo
		if len(selectedProxyNames) > 1 {
			for _, p := range visible {
				if _, ok := selectedProxyNames[p.Name]; ok {
					rowsForExport = append(rowsForExport, p)
				}
			}
			if len(rowsForExport) == 0 {
				status.SetText(locale.T("servers.status_export_nothing_selected"))
				return
			}
		} else {
			rowsForExport = visible
		}
		tags := make([]string, 0, len(rowsForExport))
		for _, p := range rowsForExport {
			if proxyClashTypeSkippedForShareExport(p) {
				continue
			}
			tags = append(tags, p.Name)
		}
		cfgPath := ac.FileService.ConfigPath
		go func() {
			fyne.Do(func() {
				status.SetText(locale.T("servers.status_export_uris_building"))
			})
			lines, err := config.BuildShareURILinesForOutboundTags(cfgPath, tags)
			fyne.Do(func() {
				if err != nil {
					ShowError(win, err)
					return
				}
				if len(lines) == 0 {
					ShowErrorText(win, locale.T("app.tab.servers"), locale.T("servers.status_export_uris_none"))
					return
				}
				// One line per server URI; full block to clipboard.
				clipboardText := strings.Join(lines, "\n")
				if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
					app.Clipboard().SetContent(clipboardText)
				}
				status.SetText(locale.Tf("servers.status_export_uris_done", len(lines)))
			})
		}()
	})
	syncExportShareURIsButtonTooltip = func() {
		if exportShareURIsButton == nil {
			return
		}
		if len(selectedProxyNames) > 1 {
			exportShareURIsButton.SetToolTip(locale.T("servers.tooltip_export_uris_selected"))
		} else {
			exportShareURIsButton.SetToolTip(locale.T("servers.tooltip_export_uris"))
		}
	}
	syncExportShareURIsButtonTooltip()

	// Кнопки пинга и сортировки по задержке (справа)
	var sortByDelayButton *ttwidget.Button
	sortByDelayButton = ttwidget.NewButton("↑", func() {
		// Применяем сортировку с текущим направлением (сохранит его в savedSortDelayAscending)
		sortByDelay(sortDelayAscending)
		// Переключаем направление для следующего раза
		sortDelayAscending = !sortDelayAscending
		// Обновляем иконку для следующего нажатия
		if sortDelayAscending {
			sortByDelayButton.SetText("↑")
		} else {
			sortByDelayButton.SetText("↓")
		}
	})
	sortByDelayButton.SetToolTip(locale.T("servers.tooltip_sort_by_delay"))

	filterPingErrorsButton := ttwidget.NewButtonWithIcon("", theme.VisibilityOffIcon(), nil)
	// Default (medium) importance — same gray style as sort arrows and Test in this row.
	updatePingErrorsFilterButton := func() {
		if hidePingErrors {
			filterPingErrorsButton.SetIcon(theme.VisibilityIcon())
			filterPingErrorsButton.SetText("")
			filterPingErrorsButton.SetToolTip(locale.T("servers.tooltip_show_ping_errors"))
		} else {
			filterPingErrorsButton.SetIcon(theme.VisibilityOffIcon())
			filterPingErrorsButton.SetText("")
			filterPingErrorsButton.SetToolTip(locale.T("servers.tooltip_hide_ping_errors"))
		}
	}
	updatePingErrorsFilterButton()
	setListFilterStatus := func() {
		all := ac.GetProxiesList()
		total := len(all)
		avail := 0
		for i := range all {
			if all[i].Delay != -1 {
				avail++
			}
		}
		status.SetText(locale.Tf("servers.status_list_counts", total, avail))
	}
	filterPingErrorsButton.OnTapped = func() {
		hidePingErrors = !hidePingErrors
		updatePingErrorsFilterButton()
		reconcileListSelection()
		proxiesListWidget.Refresh()
		setListFilterStatus()
	}

	pingAllButton := ttwidget.NewButton(locale.T("servers.button_test"), pingAllProxies)
	pingAllButton.SetToolTip(locale.T("servers.tooltip_ping_all"))

	// Let the controller trigger ping-all ~5s after VPN connects, so latency
	// in the list is fresh when the user looks. Runs on the UI thread via
	// fyne.Do because AutoPingAfterConnectFunc is called from a time.AfterFunc
	// goroutine deep inside RunningState.Set.
	//
	// This hook is intentionally uncapped — it's also bound to Cmd/Ctrl+P and
	// the /action/ping-all debug-API endpoint, both of which are explicit user
	// requests. The soft cap for the *automatic* (timer-driven) path lives at
	// the timer call sites in controller.go and main.go (resume). See SPEC 039
	// §1.3 / §2.7.
	panel.autoPingAfterConnect = func() {
		fyne.Do(pingAllProxies)
	}

	// Настройки Ping test (endpoint для delay).
	pingSettingsButton := ttwidget.NewButton("⚙", func() {
		currentURL := api.GetPingTestURL()

		// Predefined endpoints with titles from api package.
		endpoints := []api.PingTestEndpoint{
			api.PingTestEndpointGStatic,
			api.PingTestEndpointGoogle,
			api.PingTestEndpointGosuslugi,
			api.PingTestEndpointYaStaticICO,
		}

		customMode := locale.T("servers.ping_option_custom")

		options := make([]string, 0, len(endpoints)+1)
		selected := customMode
		for _, ep := range endpoints {
			options = append(options, ep.Title)
			if currentURL == ep.URL {
				selected = ep.Title
			}
		}
		options = append(options, customMode)

		radio := widget.NewRadioGroup(options, nil)
		radio.Selected = selected

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("https://example.com/generate_204")
		urlEntry.SetText(currentURL)
		if selected != customMode {
			urlEntry.Disable()
		}

		parallelChosen := strconv.Itoa(api.GetPingTestAllConcurrency())
		parallelSelect := widget.NewSelect(pingAllConcurrencyOptions, func(v string) {
			parallelChosen = v
		})
		parallelSelect.SetSelected(parallelChosen)

		parallelRow := container.NewHBox(
			widget.NewLabel(locale.T("servers.ping_label_parallel")),
			parallelSelect,
		)

		content := container.NewVBox(
			widget.NewLabel(locale.T("servers.ping_label_url")),
			radio,
			widget.NewLabel(locale.T("servers.ping_label_custom_url")),
			urlEntry,
			parallelRow,
			widget.NewLabel(" "),
		)

		d := dialog.NewCustomConfirm(locale.T("servers.dialog_ping_settings_title"), locale.T("servers.ping_button_save"), locale.T("servers.ping_button_cancel"), content, func(ok bool) {
			if !ok {
				return
			}
			selectedMode := radio.Selected
			newURL := currentURL

			if selectedMode == customMode {
				if strings.TrimSpace(urlEntry.Text) != "" {
					newURL = strings.TrimSpace(urlEntry.Text)
				}
			} else {
				for _, ep := range endpoints {
					if ep.Title == selectedMode {
						newURL = ep.URL
						break
					}
				}
			}

			api.SetPingTestURL(newURL)
			n, _ := strconv.Atoi(parallelChosen)
			if n == 0 && parallelSelect.Selected != "" {
				n, _ = strconv.Atoi(parallelSelect.Selected)
			}
			api.SetPingTestAllConcurrency(n)

			binDir := platform.GetBinDir(ac.FileService.ExecDir)
			st := locale.LoadSettings(binDir)
			st.PingTestURL = api.GetPingTestURL()
			st.PingTestAllConcurrency = api.GetPingTestAllConcurrency()
			if err := locale.SaveSettings(binDir, st); err != nil {
				debuglog.WarnLog("ping settings: failed to save settings.json: %v", err)
			}

			status.SetText(locale.Tf("servers.status_ping_url_updated", newURL))
		}, ac.UIService.MainWindow)

		radio.OnChanged = func(val string) {
			if val == customMode {
				urlEntry.Enable()
			} else {
				urlEntry.Disable()
			}
		}

		d.Show()
	})
	pingSettingsButton.SetToolTip(locale.T("servers.tooltip_ping_settings"))

	// Группа кнопок: слева сортировка, справа пинг, настройки и сортировка по задержке
	buttonsRow := container.NewHBox(
		sortByNameButton,
		sortNameLabel,
		exportShareURIsButton,
		layout.NewSpacer(),
		filterPingErrorsButton,
		sortByDelayButton,
		pingAllButton,
		pingSettingsButton,
	)

	// Mapping button for showing selector -> currently active outbound (queried from Clash API)
	mapButton := widget.NewButton("⇄", func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		// Гейт «clash_api включён» осмыслен только для classic: в daemon-режиме
		// запросы идут по gRPC-транспорту и от clash_api-секции конфига не
		// зависят (её может вообще не быть).
		if ac.BackendMode() != core.BackendDaemon {
			if _, _, enabled, _ := EffectiveClashAPIConfigIn(ac, scope); !enabled {
				ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
				return
			}
		}
		transport := EffectiveProxyTransportIn(ac, scope)

		// Run queries in background to avoid blocking UI
		go func() {
			// Используем актуальный список селекторов из groupSelect или перечитываем из конфига
			var currentSelectorOptions []string
			if groupSelect != nil && len(groupSelect.Options) > 0 {
				// Используем актуальный список из виджета (обновляется через updateSelectorList)
				currentSelectorOptions = groupSelect.Options
			} else {
				// Fallback: перечитываем из конфига, если groupSelect еще не инициализирован
				updatedOptions, _, err := config.GetSelectorGroupsFromConfig(ac.FileService.ConfigPath)
				if err != nil {
					if os.IsNotExist(err) {
						debuglog.DebugLog("clash_api_tab: config.json not present yet (popup): %v", err)
					} else {
						debuglog.ErrorLog("clash_api_tab: failed to get selector groups for popup: %v", err)
					}
					currentSelectorOptions = selectorOptions // Используем старый список как fallback
				} else if len(updatedOptions) > 0 {
					currentSelectorOptions = updatedOptions
				} else {
					currentSelectorOptions = selectorOptions // Fallback на старый список
				}
			}

			results := make([]string, 0, len(currentSelectorOptions))
			for _, sel := range currentSelectorOptions {
				_, now, err := transport.GroupProxies(sel)
				if err != nil {
					results = append(results, locale.Tf("servers.selector_error", sel, err))
					continue
				}
				if now == "" {
					results = append(results, locale.Tf("servers.selector_no_active", sel))
				} else {
					results = append(results, locale.Tf("servers.selector_active", sel, textnorm.NormalizeProxyDisplay(now)))
				}
			}

			// Show dialog on UI thread
			fyne.Do(func() {
				content := container.NewVBox()
				for _, line := range results {
					lbl := widget.NewLabel(line)
					content.Add(lbl)
				}
				scroll := container.NewVScroll(content)
				scroll.SetMinSize(fyne.NewSize(480, 260))
				dlg := dialogs.NewCustom(locale.T("servers.dialog_selector_active_title"), scroll, nil, locale.T("servers.dialog_selector_close"), ac.UIService.MainWindow)
				dlg.Show()
			})
		}()
	})
	// subtle importance to avoid visual noise
	mapButton.Importance = widget.LowImportance

	groupSelect = widget.NewSelect(selectorOptions, func(value string) {
		if value == "" {
			return
		}
		selectedGroup = value
		if ac.APIService != nil {
			ac.APIService.SetSelectedClashGroupIn(panel.scope, value)
		}
		if suppressSelectCallback {
			return
		}
		// Update status to show selected group and last used proxy for the group (if any)
		lastUsed := ac.GetLastSelectedProxyForGroup(value)
		if lastUsed != "" {
			status.SetText(locale.Tf("servers.status_selected_group", value, textnorm.NormalizeProxyDisplay(lastUsed)))
		} else {
			status.SetText(locale.Tf("servers.status_selected_group_only", value))
		}
		// Update tray menu when group changes
		if ac.UIService != nil && ac.UIService.UpdateTrayMenuFunc != nil {
			ac.UIService.UpdateTrayMenuFunc()
		}
		// Перезагружаем список под новую группу, если Clash API включён.
		//
		// Критерий — именно ВКЛЮЧЁННОСТЬ API, а не «кто запустил ядро».
		// Раньше здесь стояло ac.RunningState.IsRunning(), и список молча не
		// обновлялся в двух реальных сценариях:
		//   - ядро запущено НЕ лаунчером (tracked PID = -1, isOurProcess=false);
		//   - подключение к чужому sing-box'у через remote endpoint (SPEC 064).
		// В обоих случаях API прекрасно отвечает — пинги идут, список живой, —
		// но флаг ложен, и узлы оставались от ПРЕДЫДУЩЕЙ группы.
		//
		// Защита от «API disabled»-popup'а при холодном старте сохраняется:
		// onLoadAndRefreshProxies показывает диалог, если API выключен, — и
		// именно поэтому проверка стоит ЗДЕСЬ, до вызова.
		if _, _, clashAPIEnabled, _ := EffectiveClashAPIConfigIn(ac, scope); clashAPIEnabled {
			ac.AutoLoadProxies()
			onLoadAndRefreshProxies()
		}
	})
	groupSelect.PlaceHolder = locale.T("servers.placeholder_select_group")
	if selectedGroup != "" {
		suppressSelectCallback = true
		groupSelect.SetSelected(selectedGroup)
		suppressSelectCallback = false
	}

	// SPEC 098: панель начинается сразу с выбора группы. Питание и выбор
	// машины уехали в правую колонку вкладок — там видно, о какой машине
	// речь, тогда как в шапке списка это было неочевидно: дропдаун говорил,
	// чьи прокси показывать, а кнопки управляли «текущей» машиной.
	//
	// Гейр настроек подключения уехал туда же — в блок Core на вкладке Local:
	// он про ЛОКАЛЬНОЕ ядро (движок и сопряжение со своим демоном), и в шапке
	// списка, который может показывать узлы роутера, читался как настройка
	// удалённой машины.
	// Принудительная перезагрузка списка групп. После Deploy ядро на машине
	// перезапускается с новым конфигом, и набор selector-групп меняется — а
	// дропдаун держит тот, что прочитали при соединении. Сам лаунчер за этим
	// не следит: на удалённой машине конфиг могли поменять и мимо него.
	// Отдаём перечитывание групп наружу: после Connect панель машин обязана
	// сначала узнать группы машины и только потом грузить узлы — иначе запрос
	// уходит с пустой группой.
	panel.reloadGroups = updateSelectorList

	reloadGroupsBtn := ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		updateSelectorList()
		onLoadAndRefreshProxies()
	})
	reloadGroupsBtn.SetToolTip(locale.T("servers.reload_groups_tooltip"))
	reloadGroupsBtn.Importance = widget.LowImportance

	// ↻ прижата к правому краю: это действие над всей панелью (перечитать
	// группы у ядра), а не часть выбора группы. Border разводит их по краям
	// строки, HBox сложил бы всё встык слева.
	groupRow := container.NewBorder(nil, nil, nil, reloadGroupsBtn,
		container.NewHBox(
			widget.NewLabel(locale.T("servers.label_selector_group")), groupSelect, mapButton,
		),
	)
	topControls := container.NewVBox(groupRow, widget.NewSeparator(), buttonsRow)

	// Обертываем status label в контейнер с горизонтальной прокруткой
	// Scroll контейнер ограничит ширину label и добавит прокрутку при необходимости
	statusScroll := container.NewScroll(status)
	statusScroll.Direction = container.ScrollBoth
	// Ограничиваем только высоту, ширина будет ограничена родительским Border контейнером
	statusScroll.SetMinSize(fyne.NewSize(0, status.MinSize().Height))

	contentContainer := container.NewBorder(
		topControls,
		statusScroll,
		nil,
		nil,
		scrollContainer,
	)

	// Remote без выбранной машины: собеседника нет, и нажимать в панели
	// нечего — любое действие ушло бы в пустоту.
	//
	// Гасим ОРГАНЫ УПРАВЛЕНИЯ, а не саму панель: спрятать её целиком значит
	// отдать всё окно правой колонке, и разметка вкладки скачет туда-сюда при
	// каждом Connect. Каркас стоит на месте, кнопки серые — это и читается как
	// «здесь пока нечего делать». Панель включается по Connect
	// (SetEnabled из machine_list_panel).
	disableable := []fyne.Disableable{groupSelect, mapButton}
	for _, o := range buttonsRow.Objects {
		if d, ok := o.(fyne.Disableable); ok {
			disableable = append(disableable, d)
		}
	}
	panel.setEnabled = func(on bool) {
		for _, d := range disableable {
			if on {
				d.Enable()
			} else {
				d.Disable()
			}
		}
	}
	if scope == services.ScopeRemote {
		if _, _, connected := GetLxdRemoteOverride(); !connected {
			panel.setEnabled(false)
			// Пустой список без объяснения читается как поломка. Говорим, чего
			// не хватает: машина не выбрана.
			status.SetText(locale.T("remote.proxies.no_machine"))
		}
	}

	panel.Content = contentContainer
	return panel
}

// isUnreachableErr — ошибка означает «до машины не достучались», а не отказ
// на её стороне.
//
// gRPC заворачивает такие сбои в code=Unavailable с сырым transport-текстом
// внутри. Для пользователя разница принципиальна: машина недоступна — чинить
// сеть, а не конфиг, и никакой полезной информации в «Error while dialing»
// для него нет.
func isUnreachableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{
		"no route to host",
		"connection refused",
		"i/o timeout",
		"network is unreachable",
		"context deadline exceeded",
		"Error while dialing",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// ReloadGroups перечитывает selector-группы у активного ядра.
//
// Нужен панели машин: сразу после Connect группы машины ещё не прочитаны, и
// Refresh ушёл бы с пустой группой — ядро отвечало «group "" not found» на
// штатное подключение.
func (p *ProxyListPanel) ReloadGroups() {
	if p != nil && p.reloadGroups != nil {
		p.reloadGroups()
	}
}
