package ui

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/config/parser"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/wizard"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

const downloadPlaceholderWidth = 120

// CoreDashboardTab управляет вкладкой Core Dashboard
type CoreDashboardTab struct {
	controller *core.AppController

	// UI elements
	statusLabel               *widget.Label  // Full status: "Core Status" + icon + text
	singboxStatusLabel        *widget.Label  // sing-box status (version or "not found")
	singboxHelpBtn            *widget.Button // "?" help button, hidden when Download is hidden
	downloadButton            *widget.Button
	downloadProgress          *widget.ProgressBar // Progress bar for download
	downloadContainer         fyne.CanvasObject   // Container for button/progress bar
	downloadPlaceholder       *canvas.Rectangle   // keeps width when button hidden
	startButton               *widget.Button      // Start button
	stopButton                *widget.Button      // Stop button
	restartButton             *widget.Button      // Restart (kill, watcher restarts)
	wintunStatusLabel         *widget.Label       // wintun.dll status
	wintunHelpBtn             *widget.Button      // "?" help button, hidden when Download is hidden
	wintunDownloadButton      *widget.Button      // wintun.dll download button
	wintunDownloadProgress    *widget.ProgressBar // Progress bar for wintun.dll download
	wintunDownloadContainer   fyne.CanvasObject   // Container for wintun button/progress bar
	wintunDownloadPlaceholder *canvas.Rectangle   // keeps width when button hidden
	configStatusLabel         *widget.Button      // Используем Button для возможности клика
	templateDownloadButton    *widget.Button
	wizardButton              *widget.Button
	updateConfigButton        *widget.Button
	parserProgressBar         *widget.ProgressBar // Progress bar for parser
	parserStatusLabel         *widget.Label       // Status label for parser

	// Data
	stopAutoUpdate           chan bool
	lastUpdateSuccess        bool // Track success of last version update
	downloadInProgress       bool // Flag for sing-box download process
	wintunDownloadInProgress bool // Flag for wintun.dll download process
}

// CreateCoreDashboardTab creates and returns the Core Dashboard tab
func CreateCoreDashboardTab(ac *core.AppController) fyne.CanvasObject {
	tab := &CoreDashboardTab{
		controller:     ac,
		stopAutoUpdate: make(chan bool),
	}

	// Status block with buttons in one row
	statusRow := tab.createStatusRow()

	versionBlock := tab.createVersionBlock()
	configBlock := tab.createConfigBlock()

	var wintunBlock fyne.CanvasObject
	if runtime.GOOS == "windows" {
		wintunBlock = tab.createWintunBlock()
	}

	coreRows := []fyne.CanvasObject{versionBlock}
	if runtime.GOOS == "windows" && wintunBlock != nil {
		coreRows = append(coreRows, wintunBlock)
	}
	coreRows = append(coreRows, configBlock)
	coreInfo := container.NewVBox(coreRows...)

	contentItems := []fyne.CanvasObject{
		statusRow,
		widget.NewSeparator(),
		coreInfo,
		widget.NewSeparator(),
	}

	// Горизонтальная линия и кнопка Exit в конце списка
	exitButton := widget.NewButton("Exit", ac.GracefulExit)
	// Кнопка Exit в отдельной строке с отступом вниз
	contentItems = append(contentItems, widget.NewLabel("")) // Отступ
	contentItems = append(contentItems, container.NewCenter(exitButton))

	content := container.NewVBox(contentItems...)

	// Регистрируем callback для обновления статуса при изменении RunningState
	// Сохраняем оригинальный callback, если он есть
	originalUpdateCoreStatusFunc := tab.controller.UIService.UpdateCoreStatusFunc
	tab.controller.UIService.UpdateCoreStatusFunc = func() {
		// Вызываем оригинальный callback, если он есть
		if originalUpdateCoreStatusFunc != nil {
			originalUpdateCoreStatusFunc()
		}
		// Вызываем наш callback
		fyne.Do(func() {
			tab.updateRunningStatus()
		})
	}

	// Регистрируем callback для обновления статуса конфига
	tab.controller.UIService.UpdateConfigStatusFunc = func() {
		fyne.Do(func() {
			tab.updateConfigInfo()
		})
	}

	// Регистрируем callback для обновления прогресса парсера
	tab.controller.UIService.UpdateParserProgressFunc = func(progress float64, status string) {
		fyne.Do(func() {
			if tab.parserProgressBar != nil {
				if progress < 0 {
					// Error state - hide progress bar
					tab.parserProgressBar.Hide()
					tab.parserStatusLabel.Hide()
					// Проверяем, не запущен ли парсер
					tab.controller.ParserMutex.Lock()
					parserRunning := tab.controller.ParserRunning
					tab.controller.ParserMutex.Unlock()
					if !parserRunning {
						tab.updateConfigButton.Enable()
					}
				} else {
					// Show progress
					tab.parserProgressBar.Show()
					tab.parserStatusLabel.Show()
					tab.parserProgressBar.SetValue(progress / 100.0)
					tab.parserStatusLabel.SetText(status)
					if progress >= 100 {
						// Completed - hide after a short delay
						go func() {
							<-time.After(1 * time.Second)
							fyne.Do(func() {
								tab.parserProgressBar.Hide()
								tab.parserStatusLabel.Hide()
								// Проверяем, не запущен ли парсер
								tab.controller.ParserMutex.Lock()
								parserRunning := tab.controller.ParserRunning
								tab.controller.ParserMutex.Unlock()
								if !parserRunning {
									tab.updateConfigButton.Enable()
								}
							})
						}()
					}
				}
			}
		})
	}

	// Первоначальное обновление
	tab.updateBinaryStatus() // Проверяет наличие бинарника и вызывает updateRunningStatus
	_ = tab.updateVersionInfo()
	if runtime.GOOS == "windows" {
		tab.updateWintunStatus() // Проверяет наличие wintun.dll
	}
	tab.updateConfigInfo()

	// Запускаем автообновление версии
	tab.startAutoUpdate()

	// Регистрируем callback для показа попапа обновления
	tab.controller.UIService.ShowUpdatePopupFunc = tab.showUpdatePopup

	return content
}

// createStatusRow creates a row with status and buttons
func (tab *CoreDashboardTab) createStatusRow() fyne.CanvasObject {
	// Объединяем все в один label: "Core Status" + иконка + текст статуса
	tab.statusLabel = widget.NewLabel("Core Status Checking...")
	tab.statusLabel.Wrapping = fyne.TextWrapOff       // Отключаем перенос текста
	tab.statusLabel.Alignment = fyne.TextAlignLeading // Выравнивание текста
	tab.statusLabel.Importance = widget.MediumImportance

	startButton := widget.NewButton("Start", func() {
		core.StartSingBoxProcess()
		// Status will be updated automatically via UpdateCoreStatusFunc
	})

	stopButton := widget.NewButton("Stop", func() {
		core.StopSingBoxProcess()
	})

	restartButton := widget.NewButton("🔄", nil)
	restartButton.Importance = widget.MediumImportance
	tab.startButton = startButton
	tab.stopButton = stopButton
	tab.restartButton = restartButton
	restartButton.OnTapped = func() {
		// Brief "Stopped" look: Start on, Stop off — then Restarting...; watcher will bring process back and UpdateCoreStatusFunc will show "Running"
		if tab.startButton != nil {
			tab.startButton.Enable()
			tab.startButton.Importance = widget.HighImportance
			tab.startButton.Refresh()
		}
		if tab.stopButton != nil {
			tab.stopButton.Disable()
			tab.stopButton.Importance = widget.MediumImportance
			tab.stopButton.Refresh()
		}
		if tab.statusLabel != nil {
			tab.statusLabel.SetText("Core Status 🔄 Restarting...")
			tab.statusLabel.Refresh()
		}
		tab.restartButton.Disable()
		tab.restartButton.Refresh()
		core.KillSingBoxForRestart()
	}

	statusContainer := container.NewHBox(
		tab.statusLabel,
	)

	buttonsContainer := container.NewCenter(
		container.NewHBox(startButton, restartButton, stopButton),
	)

	// Return container with status and buttons, with empty lines before and after buttons
	return container.NewVBox(
		statusContainer,
		widget.NewLabel(""), // Empty line before buttons
		buttonsContainer,
		widget.NewLabel(""), // Empty line after buttons
	)
}

func (tab *CoreDashboardTab) createConfigBlock() fyne.CanvasObject {
	// Используем Button вместо Label для возможности клика
	title := widget.NewButton("Config", func() {
		debuglog.DebugLog("CoreDashboard: Config title clicked, reading config...")
		tab.readConfigOnDemand()
	})
	// Делаем кнопку похожей на Label (без рамки)
	title.Importance = widget.LowImportance

	// Используем Button для configStatusLabel, чтобы сделать его кликабельным
	tab.configStatusLabel = widget.NewButton("Checking config...", func() {
		debuglog.DebugLog("CoreDashboard: Config status label clicked, reading config...")
		tab.readConfigOnDemand()
	})
	tab.configStatusLabel.Importance = widget.LowImportance

	// Создаем прогрессбар и статус для парсера
	tab.parserProgressBar = widget.NewProgressBar()
	tab.parserProgressBar.Hide()
	tab.parserProgressBar.SetValue(0)

	tab.parserStatusLabel = widget.NewLabel("")
	tab.parserStatusLabel.Hide()
	tab.parserStatusLabel.Wrapping = fyne.TextWrapWord
	tab.parserStatusLabel.Alignment = fyne.TextAlignCenter

	// Кнопка Update
	tab.updateConfigButton = widget.NewButton("🔄 Update", func() {
		// Деактивируем кнопку и показываем прогрессбар
		tab.updateConfigButton.Disable()
		tab.parserProgressBar.Show()
		tab.parserProgressBar.SetValue(0)
		tab.parserStatusLabel.Show()
		tab.parserStatusLabel.SetText("Starting...")

		// Запускаем парсер в отдельной горутине
		go core.RunParserProcess()
	})
	tab.updateConfigButton.Importance = widget.MediumImportance

	tab.wizardButton = widget.NewButton("⚙️ Wizard", func() {
		// Get parent window from AppController
		ac := core.GetController()
		parentWindow := ac.GetMainWindow()
		wizard.ShowConfigWizard(parentWindow)
	})
	tab.wizardButton.Importance = widget.MediumImportance

	tab.templateDownloadButton = widget.NewButton("Download Config Template", func() {
		tab.downloadConfigTemplate()
	})
	tab.templateDownloadButton.Importance = widget.MediumImportance

	// Initially hide wizard/download buttons, updateConfigInfo will show the appropriate one
	tab.wizardButton.Hide()
	tab.templateDownloadButton.Hide()

	// Строка со статусом
	statusRow := container.NewHBox(
		title,
		layout.NewSpacer(),
		tab.configStatusLabel,
	)

	// Кнопки под статусом (по центру) - только кнопки, без прогрессбара
	buttonsRow := container.NewCenter(
		container.NewHBox(
			tab.updateConfigButton, // Кнопка Update
			tab.wizardButton,
			tab.templateDownloadButton,
		),
	)

	// Отдельная строка для прогрессбара и статуса парсера (под кнопками)
	parserProgressRow := container.NewVBox(
		tab.parserProgressBar,
		tab.parserStatusLabel,
	)

	return container.NewVBox(
		statusRow,
		buttonsRow,
		parserProgressRow, // Прогрессбар и статус парсера в отдельной строке
	)
}

// createVersionBlock creates a block with version (similar to wintun)
func (tab *CoreDashboardTab) createVersionBlock() fyne.CanvasObject {
	title := widget.NewLabel("Sing-box")
	title.Importance = widget.MediumImportance

	singboxHelpBtn := widget.NewButton("?", func() {
		msg := "sing-box is the core binary.\n\n"
		if suffix := core.SingboxAssetSuffix(); suffix != "" {
			fileName := "sing-box-*-" + suffix
			if v := tab.controller.GetCachedVersion(); v != "" {
				fileName = fmt.Sprintf("sing-box-%s-%s", v, suffix)
			}
			msg += fmt.Sprintf("Look for: %s\n", fileName)
		}
		msg += "Extract the binary into the bin folder.\n\n" +
			"You can download with the button above, or manually from:"
		binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
		urlLink := widget.NewHyperlink(constants.SingboxReleasesURL, nil)
		_ = urlLink.SetURLFromString(constants.SingboxReleasesURL)
		urlLink.OnTapped = func() {
			if err := platform.OpenURL(constants.SingboxReleasesURL); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		}
		openBinBtn := widget.NewButtonWithIcon("Open bin folder", theme.FolderOpenIcon(), func() {
			if err := platform.OpenFolder(binDir); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		})
		content := container.NewVBox(widget.NewLabel(msg), urlLink, openBinBtn)
		dialogs.ShowCustom(tab.controller.GetMainWindow(), "Sing-box", "Close", content)
	})
	tab.singboxHelpBtn = singboxHelpBtn

	tab.singboxStatusLabel = widget.NewLabel("Checking...")
	tab.singboxStatusLabel.Wrapping = fyne.TextWrapOff

	tab.downloadButton = widget.NewButton("Download", func() {
		tab.handleDownload()
	})
	tab.downloadButton.Importance = widget.MediumImportance
	tab.downloadButton.Disable()

	tab.downloadProgress = widget.NewProgressBar()
	tab.downloadProgress.Hide()
	tab.downloadProgress.SetValue(0)

	if tab.downloadPlaceholder == nil {
		tab.downloadPlaceholder = canvas.NewRectangle(color.Transparent)
	}
	placeholderSize := fyne.NewSize(downloadPlaceholderWidth, tab.downloadButton.MinSize().Height)
	tab.downloadPlaceholder.SetMinSize(placeholderSize)
	tab.downloadPlaceholder.Hide()

	tab.downloadContainer = container.NewStack(
		tab.downloadPlaceholder,
		tab.downloadButton,
		tab.downloadProgress,
	)

	return container.NewHBox(
		title,
		layout.NewSpacer(),
		tab.singboxStatusLabel,
		tab.downloadContainer,
		tab.singboxHelpBtn,
	)
}

// downloadComponentState represents UI components for download state management
type downloadComponentState struct {
	statusLabel *widget.Label
	button      *widget.Button
	progressBar *widget.ProgressBar
	placeholder *canvas.Rectangle
}

// setDownloadState - управляет состоянием компонента загрузки (лейбл, кнопка, прогресс)
// statusText: текст для статус-лейбла (если "", не менять)
// buttonText: текст кнопки (если "", скрыть кнопку; иначе показать с этим текстом и включить)
// progress: значение прогресса (если < 0, скрыть прогресс; иначе показать с этим значением 0.0-1.0)
func (tab *CoreDashboardTab) setDownloadState(component downloadComponentState, statusText string, buttonText string, progress float64) {
	// Управление статус-лейблом
	if statusText != "" && component.statusLabel != nil {
		component.statusLabel.SetText(statusText)
	}

	// Управление прогресс-баром
	progressVisible := false
	if progress < 0 {
		// Скрыть прогресс
		if component.progressBar != nil {
			component.progressBar.Hide()
			component.progressBar.SetValue(0)
		}
	} else {
		// Показать прогресс с значением
		if component.progressBar != nil {
			component.progressBar.SetValue(progress)
			component.progressBar.Show()
		}
		progressVisible = true
	}

	// Управление кнопкой (если прогресс виден, кнопка всегда скрыта)
	if progressVisible {
		// Если показываем прогресс, кнопка всегда скрыта
		if component.button != nil {
			component.button.Hide()
		}
	} else if buttonText == "" {
		// Скрыть кнопку
		if component.button != nil {
			component.button.Hide()
		}
	} else {
		// Показать кнопку с текстом
		if component.button != nil {
			component.button.SetText(buttonText)
			component.button.Show()
			component.button.Enable()
		}
	}

	// Управление placeholder: показывать если есть кнопка ИЛИ прогресс-бар
	if component.placeholder != nil {
		if progressVisible || buttonText != "" {
			component.placeholder.Show()
		} else {
			component.placeholder.Hide()
		}
	}
}

// setWintunState - управляет состоянием wintun (лейбл, кнопка, прогресс)
// statusText: текст для статус-лейбла (если "", не менять)
// buttonText: текст кнопки (если "", скрыть кнопку; иначе показать с этим текстом и включить)
// progress: значение прогресса (если < 0, скрыть прогресс; иначе показать с этим значением 0.0-1.0)
func (tab *CoreDashboardTab) setWintunState(statusText string, buttonText string, progress float64) {
	component := downloadComponentState{
		statusLabel: tab.wintunStatusLabel,
		button:      tab.wintunDownloadButton,
		progressBar: tab.wintunDownloadProgress,
		placeholder: tab.wintunDownloadPlaceholder,
	}
	tab.setDownloadState(component, statusText, buttonText, progress)
	if tab.wintunHelpBtn != nil {
		if buttonText != "" || progress >= 0 {
			tab.wintunHelpBtn.Show()
		} else {
			tab.wintunHelpBtn.Hide()
		}
	}
}

// setSingboxState - управляет состоянием sing-box (лейбл, кнопка, прогресс)
// statusText: текст для статус-лейбла (если "", не менять)
// buttonText: текст кнопки (если "", скрыть кнопку; иначе показать с этим текстом и включить)
// progress: значение прогресса (если < 0, скрыть прогресс; иначе показать с этим значением 0.0-1.0)
func (tab *CoreDashboardTab) setSingboxState(statusText string, buttonText string, progress float64) {
	component := downloadComponentState{
		statusLabel: tab.singboxStatusLabel,
		button:      tab.downloadButton,
		progressBar: tab.downloadProgress,
		placeholder: tab.downloadPlaceholder,
	}
	tab.setDownloadState(component, statusText, buttonText, progress)
	if tab.singboxHelpBtn != nil {
		if buttonText != "" || progress >= 0 {
			tab.singboxHelpBtn.Show()
		} else {
			tab.singboxHelpBtn.Hide()
		}
	}
}

// updateBinaryStatus проверяет наличие бинарника и обновляет статус
func (tab *CoreDashboardTab) updateBinaryStatus() {
	// Проверяем, существует ли бинарник
	if _, err := tab.controller.GetInstalledCoreVersion(); err != nil {
		tab.statusLabel.SetText("Core Status ❌ Error: sing-box not found")
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
		// UpdateUI will be called automatically by RunningState.Set() or other state changes
		// Don't call UpdateUI() here to avoid infinite loop
		return
	}
	// Если бинарник найден, обновляем статус запуска
	tab.updateRunningStatus()
	// UpdateUI will be called automatically by RunningState.Set() or other state changes
	// Don't call UpdateUI() here to avoid infinite loop
}

// updateRunningStatus обновляет статус Running/Stopped на основе RunningState
func (tab *CoreDashboardTab) updateRunningStatus() {
	// Get button state from centralized function (same logic as Tray Menu)
	buttonState := tab.controller.GetVPNButtonState()

	// Update status label based on state
	restartInfo := ""
	if tab.controller.ConsecutiveCrashAttempts > 0 {
		restartInfo = fmt.Sprintf(" [restart %d/%d]", tab.controller.ConsecutiveCrashAttempts, 3)
	}

	if !buttonState.BinaryExists {
		tab.statusLabel.SetText("Core Status ❌ Error: sing-box not found" + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	} else if buttonState.IsRunning {
		tab.statusLabel.SetText("Core Status ✅ Running" + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	} else {
		tab.statusLabel.SetText("Core Status ⏸️ Stopped" + restartInfo)
		tab.statusLabel.Importance = widget.MediumImportance // Текст всегда черный
	}

	// Update buttons based on centralized state
	if tab.startButton != nil {
		if buttonState.StartEnabled {
			tab.startButton.Enable()
			tab.startButton.Importance = widget.HighImportance // Синяя кнопка, когда доступна
			tab.startButton.Refresh()
		} else {
			tab.startButton.Disable()
			tab.startButton.Importance = widget.MediumImportance // Обычная, когда недоступна
			tab.startButton.Refresh()
		}
	}
	if tab.stopButton != nil {
		if buttonState.StopEnabled {
			tab.stopButton.Enable()
			tab.stopButton.Importance = widget.HighImportance
			tab.stopButton.Refresh()
		} else {
			tab.stopButton.Disable()
			tab.stopButton.Importance = widget.MediumImportance
			tab.stopButton.Refresh()
		}
	}
	if tab.restartButton != nil {
		if buttonState.StopEnabled {
			tab.restartButton.Enable()
			tab.restartButton.Refresh()
		} else {
			tab.restartButton.Disable()
			tab.restartButton.Refresh()
		}
	}
}

// readConfigOnDemand reads config when user clicks on config label/title
func (tab *CoreDashboardTab) readConfigOnDemand() {
	// Обновляем информацию о конфиге в UI
	if tab.controller.UIService != nil && tab.controller.UIService.UpdateConfigStatusFunc != nil {
		tab.controller.UIService.UpdateConfigStatusFunc()
	}

	// Читаем конфиг
	config, err := parser.ExtractParserConfig(tab.controller.FileService.ConfigPath)
	if err != nil {
		debuglog.ErrorLog("CoreDashboard: Failed to read config on demand: %v", err)
		// Можно показать сообщение пользователю через dialog
		return
	}

	debuglog.InfoLog("CoreDashboard: Config read successfully on demand (version %d, %d proxy sources, %d outbounds)",
		config.ParserConfig.Version,
		len(config.ParserConfig.Proxies),
		len(config.ParserConfig.Outbounds))
}

func (tab *CoreDashboardTab) updateConfigInfo() {
	// Обновляем статусы sing-box и wintun.dll
	_ = tab.updateVersionInfo()
	if runtime.GOOS == "windows" {
		tab.updateWintunStatus()
	}

	if tab.configStatusLabel == nil {
		return
	}
	configPath := tab.controller.FileService.ConfigPath
	configExists := false
	if info, err := os.Stat(configPath); err == nil {
		modTime := info.ModTime().Format("2006-01-02")
		tab.configStatusLabel.SetText(fmt.Sprintf("%s ✅ %s", filepath.Base(configPath), modTime))
		configExists = true
	} else if os.IsNotExist(err) {
		tab.configStatusLabel.SetText(fmt.Sprintf("%s ❌ not found", filepath.Base(configPath)))
		configExists = false
	} else {
		tab.configStatusLabel.SetText(fmt.Sprintf("Config error: %v", err))
		configExists = false
	}

	templateFileName := wizardtemplate.GetTemplateFileName()
	templatePath := filepath.Join(tab.controller.FileService.ExecDir, "bin", templateFileName)
	if _, err := os.Stat(templatePath); err != nil {
		// Template not found - show download button, hide wizard
		if tab.templateDownloadButton != nil {
			tab.templateDownloadButton.Show()
			tab.templateDownloadButton.Enable()
			// Если шаблона нет, делаем кнопку синей (HighImportance)
			tab.templateDownloadButton.Importance = widget.HighImportance
		}
		if tab.wizardButton != nil {
			tab.wizardButton.Hide()
		}
		if tab.updateConfigButton != nil {
			tab.updateConfigButton.Disable()
		}
	} else {
		// Template found - show wizard, hide download button
		if tab.templateDownloadButton != nil {
			tab.templateDownloadButton.Hide()
		}
		if tab.wizardButton != nil {
			tab.wizardButton.Show()
			// Если конфига нет, делаем кнопку Wizard синей (HighImportance)
			if !configExists {
				tab.wizardButton.Importance = widget.HighImportance
			} else {
				tab.wizardButton.Importance = widget.MediumImportance
			}
		}
		// Update кнопка активна только если конфиг существует и парсер не запущен
		if tab.updateConfigButton != nil {
			tab.controller.ParserMutex.Lock()
			parserRunning := tab.controller.ParserRunning
			tab.controller.ParserMutex.Unlock()
			if configExists && !parserRunning {
				tab.updateConfigButton.Enable()
			} else {
				tab.updateConfigButton.Disable()
			}
		}
	}

	// Обновляем статус кнопок Start/Stop, так как они зависят от наличия конфига
	tab.updateRunningStatus()
}

// updateVersionInfo обновляет информацию о версии (по аналогии с updateWintunStatus).
// Весь сценарий выполняется в одной горутине: GetInstalledCoreVersion() может долго выполняться (запуск sing-box version
// на медленной системе), поэтому вызов в UI-потоке приводил бы к краткому «зависанию» при открытии вкладки. Здесь
// версия запрашивается в фоне, UI обновляется через fyne.Do; затем в той же горутине вызывается updateVersionInfoAsync
// для подписи кнопки Download/Update по кешу. Итог: вкладка открывается без блокировки, статус и кнопка появляются
// с небольшой задержкой.
func (tab *CoreDashboardTab) updateVersionInfo() error {
	go func() {
		installedVersion, err := tab.controller.GetInstalledCoreVersion()
		fyne.Do(func() {
			if err != nil {
				tab.singboxStatusLabel.Importance = widget.MediumImportance
				tab.downloadButton.Importance = widget.HighImportance
				tab.setSingboxState("❌ not found", "Download", -1)
			} else {
				tab.singboxStatusLabel.Importance = widget.MediumImportance
				tab.setSingboxState(installedVersion, "", -1)
			}
		})
		tab.updateVersionInfoAsync(installedVersion, err != nil)
	}()
	return nil
}

// updateVersionInfoAsync обновляет кнопку Download/Update по закешированной последней версии (не блокирует UI).
func (tab *CoreDashboardTab) updateVersionInfoAsync(installedVersion string, binaryNotFound bool) {
	if binaryNotFound {
		cached := tab.controller.GetCachedVersion()
		if cached != "" {
			fyne.Do(func() {
				tab.setSingboxState("", fmt.Sprintf("Download v%s", cached), -1)
			})
		} else {
			tab.controller.CheckVersionInBackground()
			fyne.Do(func() {
				tab.setSingboxState("", "Download", -1)
			})
		}
		return
	}
	latest := tab.controller.GetCachedVersion()
	if tab.controller.ShouldCheckVersion() {
		tab.controller.CheckVersionInBackground()
	}
	fyne.Do(func() {
		if latest != "" && core.CompareVersions(installedVersion, latest) < 0 {
			tab.downloadButton.Importance = widget.HighImportance
			tab.setSingboxState("", fmt.Sprintf("Update v%s", latest), -1)
		} else {
			tab.setSingboxState("", "", -1)
		}
	})
}

func (tab *CoreDashboardTab) downloadConfigTemplate() {
	configTemplateURL := wizardtemplate.GetTemplateURL()
	if tab.templateDownloadButton != nil {
		tab.templateDownloadButton.Disable()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", configTemplateURL, nil)
		if err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, NewRequest error)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}

		resp, err := http.DefaultClient.Do(req)
		defer func() {
			if resp != nil {
				debuglog.RunAndLog("downloadConfigTemplate: close response body", resp.Body.Close)
			}
		}()
		if err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, Do error)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		if resp.StatusCode != http.StatusOK {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, status not OK)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, ReadAll error)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		templateFileName := wizardtemplate.GetTemplateFileName()
		target := filepath.Join(tab.controller.FileService.ExecDir, "bin", templateFileName)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, MkdirAll error)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, WriteFile error)")
				ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		fyne.Do(func() {
			if tab.templateDownloadButton != nil {
				tab.templateDownloadButton.Hide()
			}
			dialog.ShowInformation("Config Template", fmt.Sprintf("Template saved to %s", target), tab.controller.GetMainWindow())
			tab.updateConfigInfo()
		})
	}()
}

// handleDownload обрабатывает нажатие на кнопку Download
func (tab *CoreDashboardTab) handleDownload() {
	if tab.downloadInProgress {
		return // Уже идет скачивание
	}

	// Используем кешированную версию или получаем новую
	targetVersion := tab.controller.GetCachedVersion()
	if targetVersion == "" {
		// Если кеша нет, пытаемся получить версию синхронно (для скачивания нужна версия сразу)
		go func() {
			latest, err := tab.controller.GetLatestCoreVersion()
			fyne.Do(func() {
				if err != nil {
					ShowError(tab.controller.GetMainWindow(), fmt.Errorf("failed to get latest version: %w", err))
					tab.downloadInProgress = false
					tab.setSingboxState("", "Download", -1)
					return
				}
				// Сохраняем в кеш и запускаем скачивание
				if latest != "" && latest != core.FallbackVersion {
					tab.controller.SetCachedVersion(latest)
				}
				tab.startDownloadWithVersion(latest)
			})
		}()
		return
	}

	// Запускаем скачивание с известной версией
	tab.startDownloadWithVersion(targetVersion)
}

// startDownloadWithVersion запускает процесс скачивания с указанной версией
func (tab *CoreDashboardTab) startDownloadWithVersion(targetVersion string) {
	// Запускаем скачивание в отдельной горутине
	tab.downloadInProgress = true
	tab.downloadButton.Disable()
	tab.setSingboxState("", "", 0.0)

	// Создаем канал для прогресса
	progressChan := make(chan core.DownloadProgress, 10)

	// Start download in separate goroutine with context
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		tab.controller.DownloadCore(ctx, targetVersion, progressChan)
	}()

	// Обрабатываем прогресс в отдельной горутине
	go func() {
		for progress := range progressChan {
			fyne.Do(func() {
				// Обновляем прогресс-бар
				progressValue := float64(progress.Progress) / 100.0
				tab.setSingboxState("", "", progressValue)

				if progress.Status == "done" {
					tab.downloadInProgress = false
					// Обновляем статусы после успешного скачивания (это уберет ошибки и обновит статус)
					_ = tab.updateVersionInfo()
					tab.updateBinaryStatus() // Это вызовет updateRunningStatus() и обновит статус
					// UpdateUI will be called automatically by RunningState.Set() or other state changes
					// Don't call UpdateUI() here to avoid infinite loop
					ShowInfo(tab.controller.GetMainWindow(), "Download Complete", progress.Message)
				} else if progress.Status == "error" {
					tab.downloadInProgress = false
					tab.setSingboxState("", "Download", -1)
					binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
					debuglog.DebugLog("core_dashboard: showing download failed manual (sing-box)")
					ShowDownloadFailedManual(tab.controller.GetMainWindow(), "sing-box download failed", constants.SingboxReleasesURL, binDir)
				}
			})
		}
	}()
}

// startAutoUpdate запускает автообновление версии (статус управляется через RunningState)
func (tab *CoreDashboardTab) startAutoUpdate() {
	// Запускаем периодическое обновление с умной логикой
	go func() {
		rand.Seed(time.Now().UnixNano()) // Инициализация генератора случайных чисел

		for {
			select {
			case <-tab.stopAutoUpdate:
				return
			default:
				// Ждем перед следующим обновлением
				var delay time.Duration
				if tab.lastUpdateSuccess {
					// Если последнее обновление было успешным - не повторяем автоматически
					// Ждем очень долго (или можно вообще не повторять)
					delay = 10 * time.Minute
				} else {
					// Если была ошибка - повторяем через случайный интервал 20-35 секунд
					delay = time.Duration(20+rand.Intn(16)) * time.Second // 20-35 секунд
				}

				select {
				case <-time.After(delay):
					// Обновляем только версию асинхронно (не блокируем UI)
					// updateVersionInfo теперь полностью асинхронная
					_ = tab.updateVersionInfo()
					// Устанавливаем успех после небольшой задержки
					// (в реальности нужно отслеживать через канал, но для простоты используем задержку)
					go func() {
						<-time.After(2 * time.Second)
						tab.lastUpdateSuccess = true // Упрощенная логика
					}()
				case <-tab.stopAutoUpdate:
					return
				}
			}
		}
	}()
}

// createWintunBlock creates a block for displaying wintun.dll status
func (tab *CoreDashboardTab) createWintunBlock() fyne.CanvasObject {
	title := widget.NewLabel("Wintun")
	title.Importance = widget.MediumImportance

	wintunHelpBtn := widget.NewButton("?", func() {
		archDir := "amd64"
		if runtime.GOARCH == "arm64" {
			archDir = "arm64"
		}
		msg := "wintun.dll is required for TUN mode on Windows.\n\n" +
			fmt.Sprintf("In the download archive, take wintun.dll \n\n Your system is %s.\n\n ", archDir) +
			fmt.Sprintf("Place it in the bin folder: bin\n\n") +
			"You can download with the button above, or manually from:"
		binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
		urlLink := widget.NewHyperlink(constants.WintunHomeURL, nil)
		_ = urlLink.SetURLFromString(constants.WintunHomeURL)
		urlLink.OnTapped = func() {
			if err := platform.OpenURL(constants.WintunHomeURL); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		}
		openBinBtn := widget.NewButtonWithIcon("Open bin folder", theme.FolderOpenIcon(), func() {
			if err := platform.OpenFolder(binDir); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		})
		content := container.NewVBox(widget.NewLabel(msg), urlLink, openBinBtn)
		dialogs.ShowCustom(tab.controller.GetMainWindow(), "Wintun", "Close", content)
	})
	tab.wintunHelpBtn = wintunHelpBtn

	tab.wintunStatusLabel = widget.NewLabel("Checking...")
	tab.wintunStatusLabel.Wrapping = fyne.TextWrapOff

	tab.wintunDownloadButton = widget.NewButton("Download", func() {
		tab.handleWintunDownload()
	})
	tab.wintunDownloadButton.Importance = widget.MediumImportance
	tab.wintunDownloadButton.Disable()

	tab.wintunDownloadProgress = widget.NewProgressBar()
	tab.wintunDownloadProgress.Hide()
	tab.wintunDownloadProgress.SetValue(0)

	if tab.wintunDownloadPlaceholder == nil {
		tab.wintunDownloadPlaceholder = canvas.NewRectangle(color.Transparent)
	}
	wintunPlaceholderSize := fyne.NewSize(downloadPlaceholderWidth, tab.wintunDownloadButton.MinSize().Height)
	tab.wintunDownloadPlaceholder.SetMinSize(wintunPlaceholderSize)
	tab.wintunDownloadPlaceholder.Hide()

	tab.wintunDownloadContainer = container.NewStack(
		tab.wintunDownloadPlaceholder,
		tab.wintunDownloadButton,
		tab.wintunDownloadProgress,
	)

	return container.NewHBox(
		title,
		layout.NewSpacer(),
		tab.wintunStatusLabel,
		tab.wintunDownloadContainer,
		tab.wintunHelpBtn,
	)
}

// updateWintunStatus обновляет статус wintun.dll
func (tab *CoreDashboardTab) updateWintunStatus() {
	if runtime.GOOS != "windows" {
		return // wintun нужен только на Windows
	}

	exists, err := tab.controller.CheckWintunDLL()
	if err != nil {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.setWintunState("❌ Error checking wintun.dll", "", -1)
		return
	}

	if exists {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.setWintunState("ok", "", -1)
	} else {
		tab.wintunStatusLabel.Importance = widget.MediumImportance
		tab.wintunDownloadButton.Importance = widget.HighImportance
		tab.setWintunState("❌ not found", "Download", -1)
	}

	// Обновляем статус кнопок Start/Stop, так как они зависят от наличия wintun.dll
	tab.updateRunningStatus()
}

// handleWintunDownload обрабатывает нажатие на кнопку Download wintun.dll
func (tab *CoreDashboardTab) handleWintunDownload() {
	if tab.wintunDownloadInProgress {
		return // Уже идет скачивание
	}

	tab.wintunDownloadInProgress = true
	tab.wintunDownloadButton.Disable()
	tab.setWintunState("", "", 0.0)

	go func() {
		progressChan := make(chan core.DownloadProgress, 10)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			tab.controller.DownloadWintunDLL(ctx, progressChan)
		}()

		for progress := range progressChan {
			fyne.Do(func() {
				progressValue := float64(progress.Progress) / 100.0
				tab.setWintunState("", "", progressValue)

				if progress.Status == "done" {
					tab.wintunDownloadInProgress = false
					tab.updateWintunStatus() // Обновляет статус и управляет кнопкой
					ShowInfo(tab.controller.GetMainWindow(), "Download Complete", progress.Message)
				} else if progress.Status == "error" {
					tab.wintunDownloadInProgress = false
					tab.setWintunState("", "Download", -1)
					binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
					debuglog.DebugLog("core_dashboard: showing download failed manual (wintun)")
					ShowDownloadFailedManual(tab.controller.GetMainWindow(), "wintun.dll download failed", constants.WintunHomeURL, binDir)
				}
			})
		}
	}()
}

// showUpdatePopup показывает попап с информацией об обновлении
func (tab *CoreDashboardTab) showUpdatePopup(currentVersion, latestVersion string) {
	if tab.controller == nil || tab.controller.UIService == nil || tab.controller.UIService.MainWindow == nil {
		debuglog.WarnLog("showUpdatePopup: UIService or MainWindow not available")
		return
	}

	// Устанавливаем флаг, что попап был показан
	tab.controller.SetUpdatePopupShown(true)

	// Создаем содержимое попапа
	fyne.Do(func() {
		downloadURL := "https://github.com/Leadaxe/singbox-launcher/releases/latest"

		// Создаем ссылку на скачивание
		downloadLink := widget.NewHyperlink("Download from GitHub", nil)
		if err := downloadLink.SetURLFromString(downloadURL); err != nil {
			debuglog.ErrorLog("showUpdatePopup: Failed to set URL: %v", err)
		}
		downloadLink.OnTapped = func() {
			if err := platform.OpenURL(downloadURL); err != nil {
				debuglog.ErrorLog("showUpdatePopup: Failed to open download URL: %v", err)
				dialogs.ShowError(tab.controller.UIService.MainWindow, fmt.Errorf("Failed to open link: %w", err))
			}
		}

		// Создаем контейнер с информацией
		mainContent := container.NewVBox(
			widget.NewLabel("A new version of the application is available"),
			widget.NewLabel(""),
			widget.NewLabel(fmt.Sprintf("Current version: %s", currentVersion)),
			widget.NewLabel(fmt.Sprintf("New version: %s", latestVersion)),
			widget.NewLabel(""),
			downloadLink,
		)

		d := dialogs.NewCustom("Update Available", mainContent, nil, "Close", tab.controller.UIService.MainWindow)

		// Показываем диалог
		d.Show()
	})
}
