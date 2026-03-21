package ui

import (
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
)

// Maximum number of concurrent ping requests for "test" button.
const pingAllConcurrency = 20

// serversTabScrollbarGutterWidth reserves space to the right of scrollable content so the
// native scrollbar strip does not overlap list rows or status text (same idea as wizard Sources/Rules).
const serversTabScrollbarGutterWidth = 10

// serversScrollGutterWidth reserves space to the right of scrollable content so the
// native scrollbar strip does not overlap list rows or status text (same idea as wizard tabs).
const serversScrollGutterWidth = 10

// reorderWithPinned moves special proxies to the top of the list while
// preserving relative order of the rest:
//   - "direct-out" (if present)
//   - currently active proxy (if set and different from direct-out)
func reorderWithPinned(ac *core.AppController, list []api.ProxyInfo) []api.ProxyInfo {
	if len(list) == 0 {
		return list
	}
	const directName = "direct-out"
	activeName := ac.GetActiveProxyName()

	hasDirect := false
	hasActive := false
	for i := range list {
		if list[i].Name == directName {
			hasDirect = true
		}
		if activeName != "" && list[i].Name == activeName {
			hasActive = true
		}
	}
	if !hasDirect && (!hasActive || activeName == "") {
		return list
	}

	result := make([]api.ProxyInfo, 0, len(list))
	used := make(map[string]struct{}, 2)

	if hasDirect {
		for i := range list {
			if list[i].Name == directName {
				result = append(result, list[i])
				used[directName] = struct{}{}
				break
			}
		}
	}
	if hasActive && activeName != directName {
		for i := range list {
			if list[i].Name == activeName {
				result = append(result, list[i])
				used[activeName] = struct{}{}
				break
			}
		}
	}
	for i := range list {
		if _, ok := used[list[i].Name]; ok {
			continue
		}
		result = append(result, list[i])
	}
	return result
}

// CreateClashAPITab creates and returns the content for the "Clash API" tab.
func CreateClashAPITab(ac *core.AppController) fyne.CanvasObject {
	ac.UIService.ApiStatusLabel = widget.NewLabel(locale.T("servers.status_not_checked"))
	status := widget.NewLabel(locale.T("servers.status_click_load"))
	ac.UIService.ListStatusLabel = status

	selectorOptions, defaultSelector, err := config.GetSelectorGroupsFromConfig(ac.FileService.ConfigPath)
	if err != nil {
		debuglog.ErrorLog("clash_api_tab: failed to get selector groups: %v", err)
	}
	if len(selectorOptions) == 0 {
		selectorOptions = []string{"proxy-out"}
	}
	selectedGroup := defaultSelector
	if selectedGroup == "" {
		selectedGroup = selectorOptions[0]
	}
	// Only set SelectedClashGroup if it's not already set (to preserve value from initialization)
	if ac.APIService != nil {
		currentGroup := ac.APIService.GetSelectedClashGroup()
		if currentGroup == "" {
			ac.APIService.SetSelectedClashGroup(selectedGroup)
		} else {
			// Use existing value, but update selectedGroup variable for UI
			selectedGroup = currentGroup
		}
	}

	var (
		groupSelect            *widget.Select
		suppressSelectCallback bool
		applySavedSort         func() // Объявляем переменную заранее, значение будет присвоено позже
	)

	// --- Логика обновления и сброса ---

	onLoadAndRefreshProxies := func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
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
			baseURL, token, _ := ac.APIService.GetClashAPIConfig()
			proxies, now, err := api.GetProxiesInGroup(baseURL, token, group)
			fyne.Do(func() {
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

	// Функция для обновления списка селекторов из конфига (вызывается когда sing-box запущен и конфиг загружен)
	updateSelectorList := func() {
		updatedSelectorOptions, updatedDefaultSelector, err := config.GetSelectorGroupsFromConfig(ac.FileService.ConfigPath)
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
					ac.APIService.SetSelectedClashGroup(selectedGroup)
				}
			}
		}
	}

	onTestAPIConnection := func() {
		if ac.APIService == nil {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
			return
		}
		_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
		if !clashAPIEnabled {
			ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_clash_api_off_config"))
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
			return
		}
		go func() {
			if platform.IsSleeping() {
				return
			}
			baseURL, token, _ := ac.APIService.GetClashAPIConfig()
			err := api.TestAPIConnection(baseURL, token)
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
		ac.SetProxiesList([]api.ProxyInfo{})
		ac.SetActiveProxyName("")
		ac.SetSelectedIndex(-1)
		fyne.Do(func() {
			if ac.UIService.ApiStatusLabel != nil {
				ac.UIService.ApiStatusLabel.SetText(locale.T("servers.status_not_running"))
			}
			if ac.UIService.ListStatusLabel != nil {
				ac.UIService.ListStatusLabel.SetText(locale.T("servers.status_singbox_stopped"))
			}
			if ac.UIService.ProxiesListWidget != nil {
				ac.UIService.ProxiesListWidget.Refresh()
			}
			// Update tray menu when API state is reset
			if ac.UIService != nil && ac.UIService.UpdateTrayMenuFunc != nil {
				ac.UIService.UpdateTrayMenuFunc()
			}
		})
	}

	// --- Регистрация колбэков в контроллере ---
	if ac.UIService != nil {
		ac.UIService.RefreshAPIFunc = onTestAPIConnection
		ac.UIService.ResetAPIStateFunc = onResetAPIState
	}

	// --- Вспомогательная функция для пинга ---
	// Delay in ProxyInfo: >0 = ms, 0 = not pinged, -1 = error (so updateItem shows correct text after list refresh).
	pingProxy := func(proxyName string, button interface{ SetText(string) }) {
		go func() {
			if platform.IsSleeping() {
				return
			}
			fyne.Do(func() { button.SetText("...") })
			baseURL, token, _ := ac.APIService.GetClashAPIConfig()
			delay, err := api.GetDelay(baseURL, token, proxyName)
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
			})
		}()
	}

	// --- Создание виджета списка ---

	createItem := func() fyne.CanvasObject {
		background := canvas.NewRectangle(color.Transparent)
		background.CornerRadius = 5

		nameLabel := widget.NewLabel(locale.T("servers.label_proxy_name"))
		nameLabel.TextStyle.Bold = true

		pingButton := ttwidget.NewButton(locale.T("servers.button_ping"), nil)
		switchButton := widget.NewButton("▶️", nil)

		content := container.NewHBox(
			nameLabel,
			layout.NewSpacer(),
			pingButton,
			switchButton,
		)

		paddedContent := container.NewPadded(content)
		return container.NewStack(background, paddedContent)
	}

	updateItem := func(id int, o fyne.CanvasObject) {
		proxies := ac.GetProxiesList()
		if id < 0 || id >= len(proxies) {
			return
		}
		proxyInfo := proxies[id]

		stack := o.(*fyne.Container)
		background := stack.Objects[0].(*canvas.Rectangle)
		paddedContent := stack.Objects[1].(*fyne.Container)
		content := paddedContent.Objects[0].(*fyne.Container)

		nameLabel := content.Objects[0].(*widget.Label)
		pingButton := content.Objects[2].(*ttwidget.Button)
		if ac.APIService != nil {
			pingButton.SetToolTip(ac.APIService.GetLastPingError(proxyInfo.Name))
		} else {
			pingButton.SetToolTip("")
		}
		switchButton := content.Objects[3].(*widget.Button)

		nameLabel.SetText(proxyInfo.DisplayOrName())

		if proxyInfo.Delay > 0 {
			pingButton.SetText(locale.Tf("servers.ping_format_ms", proxyInfo.Delay))
		} else if proxyInfo.Delay == -1 {
			pingButton.SetText(locale.T("servers.ping_button_error"))
		} else {
			pingButton.SetText(locale.T("servers.button_ping"))
		}

		// Обновляем фон
		if proxyInfo.Name == ac.GetActiveProxyName() {
			background.FillColor = color.NRGBA{R: 144, G: 238, B: 144, A: 128} // Зеленый для активного
		} else if id == ac.GetSelectedIndex() {
			background.FillColor = color.NRGBA{R: 135, G: 206, B: 250, A: 128} // Синий оттенок для выделенного
		} else {
			background.FillColor = color.Transparent
		}
		background.Refresh()

		// Обновляем колбэки кнопок
		proxyNameForCallback := proxyInfo.Name

		pingButton.OnTapped = func() {
			pingProxy(proxyNameForCallback, pingButton)
		}

		switchButton.OnTapped = func() {
			if ac.APIService == nil {
				ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_not_initialized"))
				return
			}
			_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
			if !clashAPIEnabled {
				ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
				return
			}
			go func(group string) {
				err := ac.APIService.SwitchProxy(group, proxyNameForCallback)
				fyne.Do(func() {
					if err != nil {
						ShowError(ac.UIService.MainWindow, err)
						status.SetText(locale.Tf("servers.status_switch_error", err.Error()))
					} else {
						ac.SetActiveProxyName(proxyNameForCallback)
						ac.UIService.ProxiesListWidget.Refresh()
						pingProxy(proxyNameForCallback, pingButton)
						if ac.UIService.ListStatusLabel != nil {
							ac.UIService.ListStatusLabel.SetText(locale.Tf("servers.status_switched", group, textnorm.NormalizeProxyDisplay(proxyNameForCallback)))
						}
					}
				})
			}(selectedGroup)
		}
	}

	proxiesListWidget := widget.NewList(
		func() int { return len(ac.GetProxiesList()) },
		createItem,
		updateItem,
	)

	proxiesListWidget.OnSelected = func(id int) {
		ac.SetSelectedIndex(id)
		proxies := ac.GetProxiesList()
		if id >= 0 && id < len(proxies) {
			status.SetText(locale.Tf("servers.status_selected", proxies[id].DisplayOrName()))
		}
		proxiesListWidget.Refresh()
	}

	ac.UIService.ProxiesListWidget = proxiesListWidget

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
		_, _, clashAPIEnabled := ac.APIService.GetClashAPIConfig()
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
			baseURL, token, _ := ac.APIService.GetClashAPIConfig()

			type pingJob struct {
				Name string
			}

			jobs := make(chan pingJob)
			done := make(chan struct{})
			total := len(proxies)
			completed := 0
			concurrency := pingAllConcurrency
			if concurrency <= 0 {
				concurrency = 1
			}
			if concurrency > total {
				concurrency = total
			}

			worker := func() {
				for job := range jobs {
					delay, err := api.GetDelay(baseURL, token, job.Name)
					fyne.Do(func() {
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
				status.SetText(locale.Tf("servers.status_ping_completed", len(proxies)))
			})
		}()
	}

	// --- Сборка всего контента ---
	listScrollGutter := canvas.NewRectangle(color.Transparent)
	listScrollGutter.SetMinSize(fyne.NewSize(serversTabScrollbarGutterWidth, 0))
	proxiesListWithGutter := container.NewBorder(nil, nil, nil, listScrollGutter, proxiesListWidget)
	scrollContainer := container.NewScroll(proxiesListWithGutter)
	scrollContainer.SetMinSize(fyne.NewSize(0, 300))

	// Кнопка сортировки по алфавиту (слева)
	var sortByNameButton *widget.Button
	sortByNameButton = widget.NewButton("↑", func() {
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
	sortNameLabel := widget.NewLabel(locale.T("servers.label_sort_by_name"))

	// Кнопки пинга и сортировки по задержке (справа)
	var sortByDelayButton *widget.Button
	sortByDelayButton = widget.NewButton("↑", func() {
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
	pingAllButton := widget.NewButton(locale.T("servers.button_test"), pingAllProxies)

	// Настройки Ping test (endpoint для delay).
	pingSettingsButton := widget.NewButton("⚙", func() {
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

		content := container.NewVBox(
			widget.NewLabel(locale.T("servers.ping_label_url")),
			radio,
			widget.NewLabel(locale.T("servers.ping_label_custom_url")),
			urlEntry,
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

	// Группа кнопок: слева сортировка, справа пинг, настройки и сортировка по задержке
	buttonsRow := container.NewHBox(
		sortByNameButton,
		sortNameLabel,
		layout.NewSpacer(),
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
		baseURL, token, enabled := ac.APIService.GetClashAPIConfig()
		if !enabled {
			ShowErrorText(ac.UIService.MainWindow, "Clash API", locale.T("servers.error_api_disabled"))
			return
		}

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
					debuglog.ErrorLog("clash_api_tab: failed to get selector groups for popup: %v", err)
					currentSelectorOptions = selectorOptions // Используем старый список как fallback
				} else if len(updatedOptions) > 0 {
					currentSelectorOptions = updatedOptions
				} else {
					currentSelectorOptions = selectorOptions // Fallback на старый список
				}
			}

			results := make([]string, 0, len(currentSelectorOptions))
			for _, sel := range currentSelectorOptions {
				_, now, err := api.GetProxiesInGroup(baseURL, token, sel)
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
			ac.APIService.SetSelectedClashGroup(value)
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
		// Start auto-loading proxies for the new group only if sing-box is running
		if ac.RunningState.IsRunning() {
			ac.AutoLoadProxies()
		}
		onLoadAndRefreshProxies()
	})
	groupSelect.PlaceHolder = locale.T("servers.placeholder_select_group")
	if selectedGroup != "" {
		suppressSelectCallback = true
		groupSelect.SetSelected(selectedGroup)
		suppressSelectCallback = false
	}

	topControls := container.NewVBox(
		ac.UIService.ApiStatusLabel,
		container.NewHBox(widget.NewLabel(locale.T("servers.label_selector_group")), groupSelect, mapButton),
		widget.NewSeparator(),
		buttonsRow,
	)

	// Обертываем status label в контейнер с горизонтальной прокруткой
	// Scroll контейнер ограничит ширину label и добавит прокрутку при необходимости
	statusScrollGutter := canvas.NewRectangle(color.Transparent)
	statusScrollGutter.SetMinSize(fyne.NewSize(serversTabScrollbarGutterWidth, 0))
	statusWithGutter := container.NewBorder(nil, nil, nil, statusScrollGutter, status)
	statusScroll := container.NewScroll(statusWithGutter)
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

	return contentContainer
}
