package ui

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/configurator"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

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
	restartButton             *ttwidget.Button    // Restart (kill, watcher restarts) — tooltip carries shortcut
	wintunStatusLabel         *widget.Label       // wintun.dll status
	wintunHelpBtn             *widget.Button      // "?" help button, hidden when Download is hidden
	wintunDownloadButton      *widget.Button      // wintun.dll download button
	wintunDownloadProgress    *widget.ProgressBar // Progress bar for wintun.dll download
	wintunDownloadContainer   fyne.CanvasObject   // Container for wintun button/progress bar
	wintunDownloadPlaceholder *canvas.Rectangle   // keeps width when button hidden
	configStatusLabel         *widget.Button      // Используем Button для возможности клика
	templateDownloadButton    *widget.Button
	wizardButton              *widget.Button
	stateSelect               *widget.Select      // dropdown of saved named states (bottom row)
	updateConfigButton        *ttwidget.Button    // icon-only refresh-subs button (tooltip carries shortcut hint)
	parserProgressBar         *widget.ProgressBar // Progress bar for parser
	parserStatusLabel         *widget.Label       // Status label for parser

	// Subscription operation panel — single in-place toast under Exit
	// button. Updates as progress changes; final state (✓/✗ + ×) auto-hides
	// after subsToastTTL.
	subsToastBox   *fyne.Container
	subsToastTimer *time.Timer

	// Data
	downloadInProgress       bool // Flag for sing-box download process
	wintunDownloadInProgress bool // Flag for wintun.dll download process
}

// CreateCoreDashboardTab creates and returns the Core Dashboard tab
func CreateCoreDashboardTab(ac *core.AppController) fyne.CanvasObject {
	tab := &CoreDashboardTab{
		controller: ac,
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
		tab.createStateBlock(),
		widget.NewSeparator(),
	}

	// Горизонтальная линия и кнопка Exit в конце списка
	exitButton := widget.NewButton(locale.T("core.button_exit"), ac.GracefulExit)
	// Кнопка Exit в отдельной строке с отступом вниз
	contentItems = append(contentItems, widget.NewLabel("")) // Отступ
	contentItems = append(contentItems, container.NewCenter(exitButton))

	// SPEC 052 phase 8 polish: subscription status panel под Exit'ом —
	// log потока операции + finалный toast (×, ✓/✗, auto-hide 20s).
	// Сепаратор перед панелью убран: панель сама невидима пока нет
	// активной операции, и торчащая горизонтальная линия после Exit
	// в idle-состоянии выглядела как visual noise.
	contentItems = append(contentItems, tab.createSubsStatusBlock())

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

	// Регистрируем callback для обновления статуса конфига.
	// SPEC 047 phase 6: этот legacy-callback ещё дёргается из UI-путей
	// (configurator Save, dashboard Rebuild/Update/state-switch), поэтому
	// поле остаётся. Но core-пути (rebuild.go / config_service.go) больше
	// его не зовут — они публикуют events.ConfigBuilt, который ловит
	// подписчик ниже. См. SUB-TASK 3 (SPEC 070).
	tab.controller.UIService.UpdateConfigStatusFunc = func() {
		fyne.Do(func() {
			tab.updateConfigInfo()
		})
	}

	// SPEC 047 phase 6: config-status UI refresh теперь приходит через
	// EventBus. RebuildConfigIfDirty (rebuild.go) и UpdateConfigFromSubscriptions
	// (config_service.go) публикуют events.ConfigBuilt из core-goroutine'ы; здесь
	// мы делаем тот же refresh, что раньше делал прямой UpdateConfigStatusFunc
	// callback. Обёрнуто в fyne.Do — событие приходит не из UI-потока.
	if tab.controller.EventBus != nil {
		tab.controller.EventBus.Subscribe(events.ConfigBuilt, func(_ events.Event) {
			fyne.Do(func() {
				tab.updateConfigInfo()
			})
		})
	}

	// Регистрируем callback для обновления прогресса парсера. Поток:
	// единственный in-place toast в subs-status панели обновляется по
	// каждому progress callback'у — title "Refreshing subscriptions",
	// subtitle = текущий status, progress bar = текущий %.
	tab.controller.UIService.UpdateParserProgressFunc = func(progress float64, status string) {
		fyne.Do(func() {
			if tab.parserProgressBar == nil {
				return
			}
			if progress < 0 {
				// Error state — финальный toast будет показан через ShowSubsResultFunc.
				tab.parserProgressBar.Hide()
				tab.updateConfigInfo()
				return
			}
			tab.parserProgressBar.SetValue(progress / 100.0)
			tab.setSubsToastInProgress(locale.T("core.toast_refreshing_subs"), status)
		})
	}

	// Финальный тост от RunParserProcess (success/error). Заменяет
	// in-progress toast зелёной ✓ / красной ✗ карточкой (auto-hide 20s).
	tab.controller.UIService.ShowSubsResultFunc = func(success bool, message string) {
		fyne.Do(func() {
			tab.setSubsToastResult(message, success)
			tab.updateConfigInfo()
		})
	}

	// Первоначальное обновление
	tab.updateBinaryStatus() // Проверяет наличие бинарника и вызывает updateRunningStatus
	_ = tab.updateVersionInfo()
	if runtime.GOOS == "windows" {
		tab.updateWintunStatus() // Проверяет наличие wintun.dll
	}
	tab.updateConfigInfo()

	// Sing-box version is pinned via constants.RequiredCoreVersion (SPEC 046)
	// — no background latest-version polling here. Launcher self-update check
	// is independent (CheckLauncherVersionOnStartup, called from main.go).

	// Регистрируем callback для показа попапа обновления
	tab.controller.UIService.ShowUpdatePopupFunc = tab.showUpdatePopup

	return content
}

// createStatusRow creates a row with status and buttons
func (tab *CoreDashboardTab) createStatusRow() fyne.CanvasObject {
	// Объединяем все в один label: "Core Status" + иконка + текст статуса
	tab.statusLabel = widget.NewLabel(locale.T("core.status_checking"))
	tab.statusLabel.Wrapping = fyne.TextWrapOff       // Отключаем перенос текста
	tab.statusLabel.Alignment = fyne.TextAlignLeading // Выравнивание текста
	tab.statusLabel.Importance = widget.MediumImportance

	startButton := widget.NewButton(locale.T("core.button_start"), func() {
		core.StartSingBoxProcess()
		// Status will be updated automatically via UpdateCoreStatusFunc
	})

	stopButton := widget.NewButton(locale.T("core.button_stop"), func() {
		core.StopSingBoxProcess()
	})

	restartButton := ttwidget.NewButton("🔄", nil)
	restartButton.Importance = widget.MediumImportance
	restartButton.SetToolTip(fmt.Sprintf(locale.T("core.button_restart_tooltip"), platform.ShortcutModifierLabel()))
	tab.startButton = startButton
	tab.stopButton = stopButton
	tab.restartButton = restartButton
	// Restart-кнопка теперь — split-control: тап показывает popup-menu с
	// двумя опциями (SPEC 045 §6.4):
	//   1. «Только пересобрать config» — RebuildConfigIfDirty без kill+start.
	//      Полезно когда пользователь хочет проверить, что state.json даёт
	//      валидный config.json, не дёргая работающий sing-box. Скрипты
	//      аналогично ходят через POST /action/rebuild-config.
	//   2. «Пересобрать и перезапустить» — старое поведение Restart:
	//      kill → ProcessService.Start (внутри RebuildConfigIfDirty) → новый
	//      процесс с актуальным config.
	//
	// До 045 явная кнопка «rebuild only» отсутствовала — пользователю
	// приходилось нажимать Restart и ждать секундный fallout от kill+start.
	doRestartFull := func() {
		// Brief "Stopped" look: Start on, Stop off — then Restarting…; watcher
		// will bring process back and UpdateCoreStatusFunc will show "Running".
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
			tab.statusLabel.SetText(locale.T("core.status_restarting"))
			tab.statusLabel.Refresh()
		}
		tab.restartButton.Disable()
		tab.restartButton.Refresh()
		core.KillSingBoxForRestart()
	}

	doRebuildOnly := func() {
		ac := tab.controller
		if ac == nil {
			return
		}
		// forced=true: юзер явно нажал Rebuild — пересобираем даже если
		// dirty-markers чистые. Это гарантирует пробег sing-box check и
		// показ ошибок popup'ом (см. validateConfigViaSingBox), независимо
		// от того успел ли config.json обновиться от прошлых правок.
		if err := ac.RebuildConfigIfDirty(true); err != nil {
			debuglog.WarnLog("CoreDashboard: RebuildConfigIfDirty failed: %v", err)
			ShowError(ac.UIService.MainWindow, err)
			return
		}
		// Refresh dirty markers — RebuildConfigIfDirty снимает оба
		// (CacheStale + ConfigStale), кнопки должны посереть.
		if ac.UIService != nil {
			if ac.UIService.UpdateConfigStatusFunc != nil {
				ac.UIService.UpdateConfigStatusFunc()
			}
			if ac.UIService.UpdateCoreStatusFunc != nil {
				ac.UIService.UpdateCoreStatusFunc()
			}
		}
	}

	restartButton.OnTapped = func() {
		ac := tab.controller
		if ac == nil || ac.UIService == nil || ac.UIService.MainWindow == nil {
			return
		}

		// Кнопка enabled => state.json точно есть (см. updateRunningStatus).
		// Поэтому первый пункт всегда активен. Второй — два лейбла в
		// зависимости от running:
		//   sing-box работает   → «Rebuild & restart» (kill + rebuild + start)
		//   sing-box не запущен → «Rebuild & start»   (start: pre-rebuild
		//                          сработает внутри ProcessService.Start
		//                          по dirty-маркерам)
		rebuildItem := fyne.NewMenuItem(locale.T("core.restart_menu_rebuild"), doRebuildOnly)

		var fullItem *fyne.MenuItem
		if ac.RunningState.IsRunning() {
			fullItem = fyne.NewMenuItem(locale.T("core.restart_menu_full"), doRestartFull)
		} else {
			fullItem = fyne.NewMenuItem(locale.T("core.restart_menu_full_when_stopped"), func() {
				core.StartSingBoxProcess()
			})
		}
		menu := fyne.NewMenu("", rebuildItem, fullItem)
		pop := widget.NewPopUpMenu(menu, ac.UIService.MainWindow.Canvas())
		// Показываем popup сразу под кнопкой.
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(restartButton)
		pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+restartButton.Size().Height))
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
	title := widget.NewButton(locale.T("core.label_config"), func() {
		debuglog.DebugLog("CoreDashboard: Config title clicked, reading config...")
		tab.readConfigOnDemand()
	})
	// Делаем кнопку похожей на Label (без рамки)
	title.Importance = widget.LowImportance

	// Используем Button для configStatusLabel, чтобы сделать его кликабельным
	tab.configStatusLabel = widget.NewButton(locale.T("core.status_checking_config"), func() {
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

	// Update-кнопка — icon-only (ViewRefreshIcon).
	//   • Левый клик: ↻ refresh subscriptions only (cache → диск; config
	//     не трогается, ConfigStale взлетает → 🔄 синяя).
	//   • Правый клик: popup menu с двумя пунктами:
	//       — «Refresh subscriptions only» (то же что левый клик)
	//       — «Refresh & rebuild config» (chain Update + Rebuild)
	// SPEC 045 invariant: Update сам config.json НЕ пишет; rebuild делает
	// `RebuildConfigIfDirty` — единственный writer config'а.
	startProgress := func() {
		tab.parserProgressBar.Show()
		tab.parserProgressBar.SetValue(0)
		tab.parserStatusLabel.Show()
		tab.parserStatusLabel.SetText(locale.T("core.status_parser_starting"))
	}

	doRefreshOnly := func() {
		startProgress()
		go core.RunParserProcess()
	}

	tab.updateConfigButton = ttwidget.NewButtonWithIcon("", theme.ViewRefreshIcon(), doRefreshOnly)
	tab.updateConfigButton.Importance = widget.MediumImportance
	tab.updateConfigButton.SetToolTip(fmt.Sprintf(locale.T("core.button_update_tooltip"), platform.ShortcutModifierLabel()))

	tab.wizardButton = widget.NewButton(locale.T("core.button_wizard"), func() {
		// Get parent window from AppController
		ac := core.GetController()
		parentWindow := ac.GetMainWindow()
		configurator.ShowConfigWizard(parentWindow)
	})
	tab.wizardButton.Importance = widget.MediumImportance

	tab.templateDownloadButton = widget.NewButton(locale.T("core.button_download_template"), func() {
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

	// SPEC 045 фаза 9: убрана отдельная Refresh+Rebuild кнопка. Теперь
	// сама Update (refresh subscriptions) автоматически делает Rebuild
	// в конце (см. UpdateConfigFromSubscriptions). Дублирующая комбо-кнопка
	// больше ничего не добавляет.

	// Кнопки под статусом (по центру). Cmd/Ctrl+U shortcut → Update (с авто-rebuild).
	buttonsRow := container.NewCenter(
		container.NewHBox(
			tab.wizardButton,
			tab.updateConfigButton,
			tab.templateDownloadButton,
		),
	)

	// auto-update / auto-ping checkboxes moved to the dedicated Settings tab
	// (ui/settings_tab.go) so Core Dashboard stays focused on the sing-box lifecycle.
	//
	// SPEC 052 phase 8 polish: parserProgressBar / parserStatusLabel
	// перенесены ВНУТРЬ subscription status panel под Exit-кнопкой.
	// См. createSubsStatusBlock в core_dashboard_subs_status.go.

	return container.NewVBox(
		statusRow,
		buttonsRow,
	)
}

// createVersionBlock creates a block with version (similar to wintun)
func (tab *CoreDashboardTab) createVersionBlock() fyne.CanvasObject {
	title := widget.NewLabel(locale.T("core.label_singbox"))
	title.Importance = widget.MediumImportance

	singboxHelpBtn := widget.NewButton("?", func() {
		msg := locale.T("core.singbox_help_msg")
		if suffix := core.SingboxAssetSuffix(); suffix != "" {
			// Use the pinned RequiredCoreVersion in the filename hint — this
			// is exactly what the Download button installs.
			fileName := fmt.Sprintf("sing-box-%s-%s", constants.RequiredCoreVersion, suffix)
			msg += locale.Tf("core.singbox_help_look_for", fileName)
		}
		msg += locale.T("core.singbox_help_extract") +
			locale.T("core.singbox_help_manual")
		binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
		urlLink := widget.NewHyperlink(constants.SingboxReleasesURL, nil)
		_ = urlLink.SetURLFromString(constants.SingboxReleasesURL)
		urlLink.OnTapped = func() {
			if err := platform.OpenURL(constants.SingboxReleasesURL); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		}
		openBinBtn := widget.NewButtonWithIcon(locale.T("core.button_open_bin"), theme.FolderOpenIcon(), func() {
			if err := platform.OpenFolder(binDir); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		})
		content := container.NewVBox(widget.NewLabel(msg), urlLink, openBinBtn)
		dialogs.ShowCustom(tab.controller.GetMainWindow(), locale.T("core.dialog_singbox_title"), locale.T("core.dialog_singbox_close"), content)
	})
	tab.singboxHelpBtn = singboxHelpBtn

	tab.singboxStatusLabel = widget.NewLabel(locale.T("core.singbox_status_checking"))
	tab.singboxStatusLabel.Wrapping = fyne.TextWrapOff

	tab.downloadButton = widget.NewButton(locale.T("core.button_download"), func() {
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

// readConfigOnDemand triggers a UI status refresh and logs the canonical
// state.json snapshot when the user clicks the config label. Pure
// informational — does not mutate anything. Reads from state.json (SPEC 045
// canonical source); legacy `@ParserConfig` reading из config.json удалено.
func (tab *CoreDashboardTab) readConfigOnDemand() {
	if tab.controller.UIService != nil && tab.controller.UIService.UpdateConfigStatusFunc != nil {
		tab.controller.UIService.UpdateConfigStatusFunc()
	}

	if tab.controller.FileService == nil {
		return
	}
	statePath := platform.GetWizardStatePath(tab.controller.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		debuglog.WarnLog("CoreDashboard: state.json not loaded on demand: %v", err)
		return
	}
	debuglog.InfoLog("CoreDashboard: state.json snapshot (parser_config v%d, %d proxy sources, %d outbounds, %d custom rules)",
		s.ParserConfig.ParserConfig.Version,
		len(s.ParserConfig.ParserConfig.Proxies),
		len(s.ParserConfig.ParserConfig.Outbounds),
		len(s.CustomRules))
}

// createStateBlock — секция «Saved states» внизу дашборда: dropdown со
// списком именованных состояний (`bin/wizard_states/<id>.json`). Save-as /
// rename / delete делаются внутри Configurator (где есть полный workflow);
// здесь — только быстрое переключение между уже сохранёнными snapshot'ами.
func (tab *CoreDashboardTab) createStateBlock() fyne.CanvasObject {
	label := widget.NewLabelWithStyle(locale.T("core.state_section_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	tab.stateSelect = widget.NewSelect(nil, nil)
	tab.stateSelect.PlaceHolder = locale.T("core.state_select_placeholder")
	tab.stateSelect.OnChanged = func(selectedID string) {
		if selectedID == "" || tab.controller == nil || tab.controller.FileService == nil {
			return
		}
		// Preserve current state.json before overwriting. If there is no
		// current state (cold install / freshly cleared) — нечего терять,
		// просто переключаемся. Иначе показываем 3-кнопочный модал.
		statePath := platform.GetWizardStatePath(tab.controller.FileService.ExecDir)
		if _, err := os.Stat(statePath); os.IsNotExist(err) {
			tab.performStateSwitch(selectedID)
			return
		}
		tab.confirmStateSwitch(selectedID)
	}
	tab.refreshStateSelector()

	return container.NewHBox(label, layout.NewSpacer(), tab.stateSelect)
}

// refreshStateSelector перечитывает `bin/wizard_states/` и обновляет options
// dropdown'а. Текущая state.json не входит в список — она и есть «куда
// мы сейчас попали»; селектор показывает чем её заменить. Selected сбрасывается
// в plareholder, чтобы повторный выбор того же ID мог сработать.
func (tab *CoreDashboardTab) refreshStateSelector() {
	if tab.stateSelect == nil || tab.controller == nil || tab.controller.FileService == nil {
		return
	}
	dir := platform.GetWizardStatesDir(tab.controller.FileService.ExecDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		tab.stateSelect.Options = nil
		tab.stateSelect.Refresh()
		return
	}
	options := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// Пропускаем только активный state.json — нет смысла «переключаться
		// на себя». Все остальные .json — пользовательские «Save As»,
		// показываем в dropdown'е.
		if name == "state.json" {
			continue
		}
		options = append(options, strings.TrimSuffix(name, ".json"))
	}
	tab.stateSelect.Options = options
	if tab.stateSelect.Selected != "" {
		// SetSelected без триггера OnChanged: clearing.
		tab.stateSelect.ClearSelected()
	}
	tab.stateSelect.Refresh()
}

// performStateSwitch — рабочая лошадка: копирует <id>.json → state.json,
// помечает кэш+config stale, рефрешит селектор и статусы. Раньше эта
// логика жила inline в OnChanged — выделена чтобы переиспользовать после
// «save current and switch» / «discard and switch» flow'ов.
func (tab *CoreDashboardTab) performStateSwitch(selectedID string) {
	if err := tab.switchToNamedState(selectedID); err != nil {
		debuglog.WarnLog("CoreDashboard: switchToNamedState(%q): %v", selectedID, err)
		if tab.controller != nil && tab.controller.UIService != nil && tab.controller.UIService.MainWindow != nil {
			ShowError(tab.controller.UIService.MainWindow, err)
		}
		return
	}
	// state.json — копия выбранного → cache и config устарели.
	if tab.controller.StateService != nil {
		tab.controller.StateService.MarkCacheStale()
		tab.controller.StateService.MarkConfigStale()
	}
	tab.refreshStateSelector()
	if tab.controller.UIService != nil {
		if tab.controller.UIService.UpdateConfigStatusFunc != nil {
			tab.controller.UIService.UpdateConfigStatusFunc()
		}
		if tab.controller.UIService.UpdateCoreStatusFunc != nil {
			tab.controller.UIService.UpdateCoreStatusFunc()
		}
	}
}

// confirmStateSwitch — 3-кнопочный модал перед перезаписью state.json
// именованным state'ом:
//
//	[Save current] — открывает name-input dialog, копирует state.json в
//	                 <name>.json, потом switch.
//	[Discard]      — switch напрямую (текущее состояние теряется).
//	[Cancel]       — закрывает модал, сбрасывает выбор в dropdown'е.
//
// Закрытие через ESC / dismiss-text эквивалентно Cancel.
//
// SPEC: добавлено после инцидента когда юзер случайно загрузил `base.json`
// из главного экрана и потерял несохранённые preset-rules.
func (tab *CoreDashboardTab) confirmStateSwitch(targetID string) {
	if tab.controller == nil || tab.controller.UIService == nil || tab.controller.UIService.MainWindow == nil {
		// Нет окна — fallback на старое поведение (без модала).
		tab.performStateSwitch(targetID)
		return
	}
	win := tab.controller.UIService.MainWindow

	body := widget.NewLabel(locale.Tf("core.state_switch_confirm_body", targetID))
	body.Wrapping = fyne.TextWrapWord

	var dlg dialog.Dialog
	// «Не сохранять и переключить» — оригинальное поведение.
	discardBtn := widget.NewButton(locale.T("core.state_switch_btn_discard"), func() {
		if dlg != nil {
			dlg.Hide()
		}
		tab.performStateSwitch(targetID)
	})
	// «Сохранить текущее → переключить»: подцепляем secondary dialog.
	saveBtn := widget.NewButton(locale.T("core.state_switch_btn_save"), func() {
		if dlg != nil {
			dlg.Hide()
		}
		tab.promptSaveCurrentStateAs(func() {
			tab.performStateSwitch(targetID)
		})
	})
	saveBtn.Importance = widget.HighImportance

	buttons := container.NewHBox(layout.NewSpacer(), discardBtn, saveBtn)
	dlg = dialogs.NewCustom(
		locale.T("core.state_switch_title"),
		body,
		buttons,
		locale.T("core.state_switch_btn_cancel"),
		win,
	)
	dlg.SetOnClosed(func() {
		// Кнопки сами вызывают dlg.Hide() — на повторном SetOnClosed
		// (Cancel/ESC) сбросим dropdown, чтобы он не показывал «выбран X»
		// при не-сработавшем switch'е. ClearSelected безопасен и после
		// performStateSwitch (refreshStateSelector тоже его дёргает).
		if tab.stateSelect != nil {
			tab.stateSelect.ClearSelected()
		}
	})
	dlg.Show()
}

// promptSaveCurrentStateAs — диалог ввода имени для save-as копии
// текущего state.json в `bin/wizard_states/<id>.json`. На успех вызывает
// `then()` (обычно — переключение на новый state).
//
// Reuse semantic ShowSaveStateDialog'а из визарда (валидация ID,
// предупреждение о существующем имени), но не зависит от
// WizardPresenter — диалог рисуется поверх main window'а.
func (tab *CoreDashboardTab) promptSaveCurrentStateAs(then func()) {
	if tab.controller == nil || tab.controller.UIService == nil || tab.controller.UIService.MainWindow == nil || tab.controller.FileService == nil {
		return
	}
	win := tab.controller.UIService.MainWindow

	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder(locale.T("core.state_save_placeholder"))

	warning := widget.NewLabel("")
	warning.Hide()

	statesDir := platform.GetWizardStatesDir(tab.controller.FileService.ExecDir)
	exists := func(id string) bool {
		_, err := os.Stat(filepath.Join(statesDir, id+".json"))
		return err == nil
	}

	idEntry.OnChanged = func(text string) {
		t := strings.TrimSpace(text)
		if t == "" {
			warning.Hide()
			return
		}
		if err := wizardmodels.ValidateStateID(t); err != nil {
			warning.SetText(err.Error())
			warning.Show()
			return
		}
		if exists(t) {
			warning.SetText(locale.Tf("core.state_save_warning_exists", t+".json"))
			warning.Show()
			return
		}
		warning.Hide()
	}

	var dlg dialog.Dialog
	saveBtn := widget.NewButton(locale.T("core.state_save_btn_save"), func() {
		id := strings.TrimSpace(idEntry.Text)
		if id == "" {
			dialog.ShowError(fmt.Errorf("%s", locale.T("core.state_save_error_empty")), win)
			return
		}
		if err := wizardmodels.ValidateStateID(id); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if err := tab.copyCurrentStateAs(id); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if dlg != nil {
			dlg.Hide()
		}
		// Refresh dropdown сразу — новый файл должен появиться, даже если
		// then() не вызовет refresh'а (для defensive).
		tab.refreshStateSelector()
		then()
	})
	saveBtn.Importance = widget.HighImportance

	body := container.NewVBox(
		widget.NewLabel(locale.T("core.state_save_label")),
		idEntry,
		warning,
	)
	buttons := container.NewHBox(layout.NewSpacer(), saveBtn)
	dlg = dialogs.NewCustom(
		locale.T("core.state_save_title"),
		body,
		buttons,
		locale.T("core.state_switch_btn_cancel"),
		win,
	)
	dlg.SetOnClosed(func() {
		// Cancel в этом sub-dialog'е = передумали сохранять и передумали
		// переключаться — сбрасываем dropdown.
		if tab.stateSelect != nil {
			tab.stateSelect.ClearSelected()
		}
	})
	dlg.Resize(fyne.NewSize(380, 180))
	dlg.Show()
	idEntry.FocusGained()
}

// copyCurrentStateAs — атомарно копирует state.json → wizard_states/<id>.json.
// Если файл уже существует — перезапишет (валидация дубликата делается
// в UI диалоге через warning label; пользователь явно согласился).
func (tab *CoreDashboardTab) copyCurrentStateAs(id string) error {
	if tab.controller == nil || tab.controller.FileService == nil {
		return fmt.Errorf("file service not initialized")
	}
	src := platform.GetWizardStatePath(tab.controller.FileService.ExecDir)
	dst := filepath.Join(platform.GetWizardStatesDir(tab.controller.FileService.ExecDir), id+".json")
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup of partial write
		return fmt.Errorf("rename %s → %s: %w", tmp, dst, err)
	}
	debuglog.InfoLog("CoreDashboard: saved current state.json → %q.json", id)
	return nil
}

// switchToNamedState атомарно копирует `<id>.json` в state.json. Не
// трогает кэш / config — за это отвечают rebuild-flow + dirty-маркеры,
// которые caller выставит после успешного switch'а.
func (tab *CoreDashboardTab) switchToNamedState(id string) error {
	if tab.controller == nil || tab.controller.FileService == nil {
		return fmt.Errorf("file service not initialized")
	}
	statesDir := platform.GetWizardStatesDir(tab.controller.FileService.ExecDir)
	src := filepath.Join(statesDir, id+".json")
	dst := platform.GetWizardStatePath(tab.controller.FileService.ExecDir)
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup of partial write
		return fmt.Errorf("rename %s → %s: %w", tmp, dst, err)
	}
	debuglog.InfoLog("CoreDashboard: switched state.json → %q", id)
	return nil
}

func (tab *CoreDashboardTab) downloadConfigTemplate() {
	configTemplateURL := wizardtemplate.GetTemplateURL()
	if tab.templateDownloadButton != nil {
		tab.templateDownloadButton.Disable()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		data, status, err := tab.controller.GetURLBytes(ctx, configTemplateURL, 30*time.Second)
		if err != nil {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, GetURLBytes error)")
				dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		if status != http.StatusOK {
			fyne.Do(func() {
				if tab.templateDownloadButton != nil {
					tab.templateDownloadButton.Enable()
				}
				binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
				debuglog.DebugLog("core_dashboard: showing download failed manual (template, status not OK)")
				dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
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
				dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
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
				dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "Config template download failed", configTemplateURL, binDir)
			})
			return
		}
		// Pin install: record which launcher version installed this template so
		// the next launcher upgrade knows to invalidate it (SPEC 046). Best
		// effort — failure here doesn't undo the file write, just risks
		// re-invalidation on next upgrade.
		binDirForMark := filepath.Dir(target)
		if err := locale.MarkTemplateInstalled(binDirForMark, constants.AppVersion); err != nil {
			debuglog.WarnLog("template: failed to record install version: %v", err)
		}
		fyne.Do(func() {
			if tab.templateDownloadButton != nil {
				tab.templateDownloadButton.Hide()
			}
			dialog.ShowInformation(locale.T("core.dialog_template_title"), locale.Tf("core.dialog_template_saved", target), tab.controller.GetMainWindow())
			tab.updateConfigInfo()
		})
	}()
}

// handleDownload обрабатывает нажатие на кнопку Download/Reinstall.
// Версия не выбирается пользователем — DownloadCore сам подставит pinned
// `constants.RequiredCoreVersion` (форк-тег для всех платформ, вкл. windows/386), см. SPEC 046.
func (tab *CoreDashboardTab) handleDownload() {
	if tab.downloadInProgress {
		return
	}
	tab.downloadInProgress = true
	tab.downloadButton.Disable()
	tab.setSingboxState("", "", 0.0)

	progressChan := make(chan core.DownloadProgress, 10)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		// Empty version → DownloadCore uses constants.RequiredCoreVersion.
		tab.controller.DownloadCore(ctx, "", progressChan)
	}()

	go func() {
		for progress := range progressChan {
			fyne.Do(func() {
				progressValue := float64(progress.Progress) / 100.0
				tab.setSingboxState("", "", progressValue)

				switch progress.Status {
				case "done":
					tab.downloadInProgress = false
					// Refresh: clears binary-not-found state + button label.
					_ = tab.updateVersionInfo()
					tab.updateBinaryStatus()
					ShowInfo(tab.controller.GetMainWindow(), locale.T("core.dialog_download_complete_title"), progress.Message)
				case "error":
					tab.downloadInProgress = false
					tab.setSingboxState("", locale.Tf("core.button_download_version", constants.RequiredCoreVersion), -1)
					binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
					debuglog.DebugLog("core_dashboard: showing download failed manual (sing-box)")
					dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "sing-box download failed", constants.SingboxReleasesURL, binDir)
				}
			})
		}
	}()
}

// createWintunBlock creates a block for displaying wintun.dll status
func (tab *CoreDashboardTab) createWintunBlock() fyne.CanvasObject {
	title := widget.NewLabel(locale.T("core.label_wintun"))
	title.Importance = widget.MediumImportance

	wintunHelpBtn := widget.NewButton("?", func() {
		archDir := "amd64"
		if runtime.GOARCH == "arm64" {
			archDir = "arm64"
		}
		msg := locale.T("core.wintun_help_msg") +
			locale.Tf("core.wintun_help_in_archive", archDir) +
			locale.T("core.wintun_help_place") +
			locale.T("core.wintun_help_manual")
		binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
		urlLink := widget.NewHyperlink(constants.WintunHomeURL, nil)
		_ = urlLink.SetURLFromString(constants.WintunHomeURL)
		urlLink.OnTapped = func() {
			if err := platform.OpenURL(constants.WintunHomeURL); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		}
		openBinBtn := widget.NewButtonWithIcon(locale.T("core.button_open_bin"), theme.FolderOpenIcon(), func() {
			if err := platform.OpenFolder(binDir); err != nil {
				ShowError(tab.controller.GetMainWindow(), err)
			}
		})
		content := container.NewVBox(widget.NewLabel(msg), urlLink, openBinBtn)
		dialogs.ShowCustom(tab.controller.GetMainWindow(), locale.T("core.dialog_wintun_title"), locale.T("core.dialog_wintun_close"), content)
	})
	tab.wintunHelpBtn = wintunHelpBtn

	tab.wintunStatusLabel = widget.NewLabel(locale.T("core.wintun_status_checking"))
	tab.wintunStatusLabel.Wrapping = fyne.TextWrapOff

	tab.wintunDownloadButton = widget.NewButton(locale.T("core.button_download"), func() {
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
					ShowInfo(tab.controller.GetMainWindow(), locale.T("core.dialog_download_complete_title"), progress.Message)
				} else if progress.Status == "error" {
					tab.wintunDownloadInProgress = false
					tab.setWintunState("", locale.T("core.button_download"), -1)
					binDir := filepath.Join(tab.controller.FileService.ExecDir, constants.BinDirName)
					debuglog.DebugLog("core_dashboard: showing download failed manual (wintun)")
					dialogs.ShowDownloadFailedManual(tab.controller.GetMainWindow(), "wintun.dll download failed", constants.WintunHomeURL, binDir)
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
		downloadLink := widget.NewHyperlink(locale.T("core.button_download_from_github"), nil)
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
			widget.NewLabel(locale.T("core.dialog_update_msg")),
			widget.NewLabel(""),
			widget.NewLabel(locale.Tf("core.dialog_update_current", currentVersion)),
			widget.NewLabel(locale.Tf("core.dialog_update_new", latestVersion)),
			widget.NewLabel(""),
			downloadLink,
		)

		d := dialogs.NewCustom(locale.T("core.dialog_update_available_title"), mainContent, nil, locale.T("core.dialog_update_close"), tab.controller.UIService.MainWindow)

		// Показываем диалог
		d.Show()
	})
}
