package ui

import (
	"fmt"
	"image/color"
	"log"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/api"
	"singbox-launcher/core"
)

// CreateClashAPITab creates and returns the content for the "Clash API" tab.
func CreateClashAPITab(ac *core.AppController) fyne.CanvasObject {
	ac.ApiStatusLabel = widget.NewLabel("Status: Not checked")
	status := widget.NewLabel("Click 'Load Proxies'")
	ac.ListStatusLabel = status

	selectorOptions, defaultSelector, err := core.GetSelectorGroupsFromConfig(ac.ConfigPath)
	if err != nil {
		log.Printf("clash_api_tab: failed to get selector groups: %v", err)
	}
	if len(selectorOptions) == 0 {
		selectorOptions = []string{"proxy-out"}
	}
	selectedGroup := defaultSelector
	if selectedGroup == "" {
		selectedGroup = selectorOptions[0]
	}
	// Only set SelectedClashGroup if it's not already set (to preserve value from initialization)
	if ac.SelectedClashGroup == "" {
		ac.SelectedClashGroup = selectedGroup
	} else {
		// Use existing value, but update selectedGroup variable for UI
		selectedGroup = ac.SelectedClashGroup
	}

	var (
		groupSelect            *widget.Select
		suppressSelectCallback bool
	)

	// --- Логика обновления и сброса ---

	onLoadAndRefreshProxies := func() {
		if !ac.ClashAPIEnabled {
			ShowErrorText(ac.MainWindow, "Clash API", "API is disabled: config error")
			if ac.ListStatusLabel != nil {
				ac.ListStatusLabel.SetText("Clash API disabled due to config error")
			}
			return
		}

		group := selectedGroup
		if group == "" {
			return
		}
		if ac.ListStatusLabel != nil {
			ac.ListStatusLabel.SetText(fmt.Sprintf("Loading proxies for '%s'...", group))
		}
		go func(group string) {
			proxies, now, err := api.GetProxiesInGroup(ac.ClashAPIBaseURL, ac.ClashAPIToken, group, ac.ApiLogFile)
			fyne.Do(func() {
				if err != nil {
					ShowError(ac.MainWindow, err)
					if ac.ListStatusLabel != nil {
						ac.ListStatusLabel.SetText("Error: " + err.Error())
					}
					return
				}

				ac.SetProxiesList(proxies)
				ac.SetActiveProxyName(now)

				if ac.ProxiesListWidget != nil {
					ac.ProxiesListWidget.Refresh()
				}

				if ac.ListStatusLabel != nil {
					ac.ListStatusLabel.SetText(fmt.Sprintf("Proxies loaded for '%s'. Active: %s", group, now))
				}

				// Update tray menu with new proxy list
				if ac.UpdateTrayMenuFunc != nil {
					ac.UpdateTrayMenuFunc()
				}
			})
		}(group)
	}

	// Функция для обновления списка селекторов из конфига (вызывается когда sing-box запущен и конфиг загружен)
	updateSelectorList := func() {
		updatedSelectorOptions, updatedDefaultSelector, err := core.GetSelectorGroupsFromConfig(ac.ConfigPath)
		if err == nil && len(updatedSelectorOptions) > 0 && groupSelect != nil {
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
				ac.SelectedClashGroup = selectedGroup
			}
		}
	}

	onTestAPIConnection := func() {
		if !ac.ClashAPIEnabled {
			ac.ApiStatusLabel.SetText("❌ ClashAPI Off (Config Error)")
			ShowErrorText(ac.MainWindow, "Clash API", "API is disabled: config error")
			return
		}
		go func() {
			err := api.TestAPIConnection(ac.ClashAPIBaseURL, ac.ClashAPIToken, ac.ApiLogFile)
			fyne.Do(func() {
				if err != nil {
					ac.ApiStatusLabel.SetText("❌ Clash API Off (Error)")
					ShowError(ac.MainWindow, err)
					return
				}
				ac.ApiStatusLabel.SetText("✅ Clash API On")
				// Обновить список селекторов после успешного подключения (sing-box запущен, конфиг загружен)
				updateSelectorList()
				onLoadAndRefreshProxies()
			})
		}()
	}

	onResetAPIState := func() {
		log.Println("clash_api_tab: Resetting API state.")
		ac.SetProxiesList([]api.ProxyInfo{})
		ac.SetActiveProxyName("")
		ac.SetSelectedIndex(-1)
		fyne.Do(func() {
			if ac.ApiStatusLabel != nil {
				ac.ApiStatusLabel.SetText("Status: Not running")
			}
			if ac.ListStatusLabel != nil {
				ac.ListStatusLabel.SetText("Sing-box is stopped.")
			}
			if ac.ProxiesListWidget != nil {
				ac.ProxiesListWidget.Refresh()
			}
			// Update tray menu when API state is reset
			if ac.UpdateTrayMenuFunc != nil {
				ac.UpdateTrayMenuFunc()
			}
		})
	}

	// --- Регистрация колбэков в контроллере ---
	ac.RefreshAPIFunc = onTestAPIConnection
	ac.ResetAPIStateFunc = onResetAPIState

	// --- Вспомогательная функция для пинга ---
	pingProxy := func(proxyName string, button *widget.Button) {
		go func() {
			fyne.Do(func() { button.SetText("...") })
			delay, err := api.GetDelay(ac.ClashAPIBaseURL, ac.ClashAPIToken, proxyName, ac.ApiLogFile)
			fyne.Do(func() {
				if err != nil {
					button.SetText("Error")
					status.SetText("Delay error: " + err.Error())
					ShowError(ac.MainWindow, err)
				} else {
					button.SetText(fmt.Sprintf("%d ms", delay))
					status.SetText(fmt.Sprintf("Delay: %d ms for %s", delay, proxyName))
				}
			})
		}()
	}

	// --- Создание виджета списка ---

	createItem := func() fyne.CanvasObject {
		background := canvas.NewRectangle(color.Transparent)
		background.CornerRadius = 5

		nameLabel := widget.NewLabel("Proxy Name")
		nameLabel.TextStyle.Bold = true

		pingButton := widget.NewButton("Ping", nil)
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
		pingButton := content.Objects[2].(*widget.Button)
		switchButton := content.Objects[3].(*widget.Button)

		nameLabel.SetText(proxyInfo.Name)

		if proxyInfo.Delay > 0 {
			pingButton.SetText(fmt.Sprintf("%d ms", proxyInfo.Delay))
		} else {
			pingButton.SetText("Ping")
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
			if !ac.ClashAPIEnabled {
				ShowErrorText(ac.MainWindow, "Clash API", "API is disabled: config error")
				return
			}
			go func(group string) {
				err := api.SwitchProxy(ac.ClashAPIBaseURL, ac.ClashAPIToken, group, proxyNameForCallback, ac.ApiLogFile)
				fyne.Do(func() {
					if err != nil {
						ShowError(ac.MainWindow, err)
						status.SetText("Switch error: " + err.Error())
					} else {
						ac.SetActiveProxyName(proxyNameForCallback)
						ac.ProxiesListWidget.Refresh()
						pingProxy(proxyNameForCallback, pingButton)
						if ac.ListStatusLabel != nil {
							ac.ListStatusLabel.SetText(fmt.Sprintf("Switched '%s' to %s", group, proxyNameForCallback))
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
			status.SetText("Selected: " + proxies[id].Name)
		}
		proxiesListWidget.Refresh()
	}

	ac.ProxiesListWidget = proxiesListWidget

	// Переменные для отслеживания направления сортировки
	sortNameAscending := true
	sortDelayAscending := true

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
				return sorted[i].Name < sorted[j].Name
			})
			status.SetText("Sorted by name (A-Z)")
		} else {
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].Name > sorted[j].Name
			})
			status.SetText("Sorted by name (Z-A)")
		}
		ac.SetProxiesList(sorted)
		if ac.ProxiesListWidget != nil {
			ac.ProxiesListWidget.Refresh()
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
			status.SetText("Sorted by delay (fastest first)")
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
			status.SetText("Sorted by delay (slowest first)")
		}

		ac.SetProxiesList(sorted)
		if ac.ProxiesListWidget != nil {
			ac.ProxiesListWidget.Refresh()
		}
	}

	// --- Функция массового пинга всех прокси ---
	pingAllProxies := func() {
		if !ac.ClashAPIEnabled {
			ShowErrorText(ac.MainWindow, "Clash API", "API is disabled: config error")
			return
		}
		proxies := ac.GetProxiesList()
		if len(proxies) == 0 {
			status.SetText("No proxies to ping")
			return
		}
		status.SetText(fmt.Sprintf("Pinging %d proxies...", len(proxies)))
		go func() {
			for i, proxy := range proxies {
				delay, err := api.GetDelay(ac.ClashAPIBaseURL, ac.ClashAPIToken, proxy.Name, ac.ApiLogFile)
				fyne.Do(func() {
					// Обновляем задержку в списке прокси
					updatedProxies := ac.GetProxiesList()
					for j := range updatedProxies {
						if updatedProxies[j].Name == proxy.Name {
							if err != nil {
								updatedProxies[j].Delay = -1 // Ошибка
							} else {
								updatedProxies[j].Delay = delay
							}
							break
						}
					}
					ac.SetProxiesList(updatedProxies)
					if ac.ProxiesListWidget != nil {
						ac.ProxiesListWidget.Refresh()
					}
					status.SetText(fmt.Sprintf("Pinging %d/%d...", i+1, len(proxies)))
				})
				// Небольшая задержка между запросами, чтобы не перегружать API
				time.Sleep(100 * time.Millisecond)
			}
			fyne.Do(func() {
				status.SetText(fmt.Sprintf("Ping test completed for %d proxies", len(proxies)))
			})
		}()
	}

	// --- Сборка всего контента ---
	scrollContainer := container.NewScroll(proxiesListWidget)
	scrollContainer.SetMinSize(fyne.NewSize(0, 300))

	// Кнопка сортировки по алфавиту (слева)
	var sortByNameButton *widget.Button
	sortByNameButton = widget.NewButton("↑", func() {
		// Выполняем сортировку с текущим направлением
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
	sortNameLabel := widget.NewLabel("A…Z")

	// Кнопки пинга и сортировки по задержке (справа)
	var sortByDelayButton *widget.Button
	sortByDelayButton = widget.NewButton("↑", func() {
		// Выполняем сортировку с текущим направлением
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
	pingAllButton := widget.NewButton("test", pingAllProxies)

	// Группа кнопок: слева сортировка, справа пинг и сортировка по задержке
	buttonsRow := container.NewHBox(
		sortByNameButton,
		sortNameLabel,
		layout.NewSpacer(),
		sortByDelayButton,
		pingAllButton,
	)

	groupSelect = widget.NewSelect(selectorOptions, func(value string) {
		if value == "" {
			return
		}
		selectedGroup = value
		ac.SelectedClashGroup = value
		if suppressSelectCallback {
			return
		}
		status.SetText(fmt.Sprintf("Selected group '%s'.", value))
		// Update tray menu when group changes
		if ac.UpdateTrayMenuFunc != nil {
			ac.UpdateTrayMenuFunc()
		}
		// Start auto-loading proxies for the new group only if sing-box is running
		if ac.RunningState.IsRunning() {
			ac.AutoLoadProxies()
		}
		onLoadAndRefreshProxies()
	})
	groupSelect.PlaceHolder = "Select selector group"
	if selectedGroup != "" {
		suppressSelectCallback = true
		groupSelect.SetSelected(selectedGroup)
		suppressSelectCallback = false
	}

	topControls := container.NewVBox(
		ac.ApiStatusLabel,
		container.NewHBox(widget.NewLabel("Selector group:"), groupSelect),
		widget.NewSeparator(),
		buttonsRow,
	)

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

	return contentContainer
}
