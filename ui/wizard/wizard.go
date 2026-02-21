// Package wizard содержит точку входа и координацию компонентов визарда конфигурации.
//
// Файл wizard.go содержит функцию ShowConfigWizard - главную точку входа визарда конфигурации.
// Она координирует создание всех компонентов визарда:
//   - Создание модели (WizardModel) и GUI-состояния (GUIState)
//   - Загрузку данных шаблона (TemplateData)
//   - Создание презентера (WizardPresenter), связывающего модель, GUI и бизнес-логику
//   - Создание табов (Source, Rules, Preview) и их содержимого
//   - Настройку обработчиков событий и навигации
//   - Инициализацию данных (загрузка конфигурации из файла, установка начальных значений)
//
// Визард следует архитектуре MVP (Model-View-Presenter):
//   - Model (models.WizardModel) - чистые бизнес-данные без GUI зависимостей
//   - View (GUIState + tabs/dialogs) - только GUI виджеты и их компоновка
//   - Presenter (WizardPresenter) - связывает модель и представление, координирует бизнес-логику
//
// Файл содержит высокоуровневую координацию всех компонентов визарда.
// Определяет жизненный цикл визарда (создание, инициализация, закрытие).
// Является единственным местом, где создаются все основные компоненты вместе.
//
// Используется в:
//   - core/ui/ui.go - вызывается при открытии визарда из главного окна приложения
//
// Координирует:
//   - models - создает WizardModel
//   - presentation - создает GUIState и WizardPresenter
//   - tabs - создает все три таба визарда
//   - dialogs - настраивает вызовы диалогов
//   - business - инициализирует загрузку конфигурации через presenter
package wizard

import (
	"fmt"
	"image/color"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/ui/components"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizarddialogs "singbox-launcher/ui/wizard/dialogs"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardtabs "singbox-launcher/ui/wizard/tabs"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowConfigWizard opens the configuration wizard window.
//
// Implemented as a singleton window: we keep a reference to the created
// window in `controller.UIService.WizardWindow` so that subsequent calls
// only focus the existing window instead of creating a second instance.
// This prevents multiple parallel instances of the wizard from being
// opened and simplifies lifecycle management.
func ShowConfigWizard(parent fyne.Window) {
	ac := core.GetController()
	if ac == nil {
		return
	}
	// If wizard is already open - just focus it and return.
	// Using RequestFocus() ensures the already-open window is brought
	// to the foreground without creating a duplicate.
	if ac.UIService != nil && ac.UIService.WizardWindow != nil {
		ac.UIService.WizardWindow.RequestFocus()
		return
	}

	// Create model and GUI state
	model := wizardmodels.NewWizardModel()
	guiState := &wizardpresentation.GUIState{}

	// Load template data
	templateLoader := &wizardbusiness.DefaultTemplateLoader{}
	templateData, err := templateLoader.LoadTemplateData(ac.FileService.ExecDir)
	if err != nil {
		templateFileName := wizardtemplate.GetTemplateFileName()
		debuglog.ErrorLog("ConfigWizard: failed to load %s from %s: %v", templateFileName, filepath.Join(ac.FileService.ExecDir, "bin", templateFileName), err)
		debuglog.DebugLog("wizard: showing download failed manual (template load on open)")
		binDir := filepath.Join(ac.FileService.ExecDir, constants.BinDirName)
		dialogs.ShowDownloadFailedManual(parent, "Config template failed to load", wizardtemplate.GetTemplateURL(), binDir)
		if ac.UIService != nil && ac.UIService.UpdateConfigStatusFunc != nil {
			ac.UIService.UpdateConfigStatusFunc()
		}
		return
	}
	model.TemplateData = templateData
	model.ExecDir = ac.FileService.ExecDir

	// Create new window for wizard
	wizardWindow := ac.UIService.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(620, 660))
	wizardWindow.CenterOnScreen()
	guiState.Window = wizardWindow

	// Store wizard window in UIService
	if ac.UIService != nil {
		ac.UIService.WizardWindow = wizardWindow
		// Notify UIService consumers that wizard state changed
		if ac.UIService.OnStateChange != nil {
			ac.UIService.OnStateChange()
		}
	}

	// Create presenter
	presenter := wizardpresentation.NewWizardPresenter(model, guiState, templateLoader)
	if ac.UIService != nil {
		ac.UIService.FocusOpenRuleDialogs = func() {
			openDialogs := presenter.OpenRuleDialogs()
			for _, dlg := range openDialogs {
				if dlg != nil {
					dlg.Show()
					dlg.RequestFocus()
				}
			}
		}
		wizardWindow.SetOnClosed(func() {
			ac.UIService.WizardWindow = nil
			ac.UIService.FocusOpenRuleDialogs = nil
			if ac.UIService.OnStateChange != nil {
				ac.UIService.OnStateChange()
			}
		})
	}

	// Check if state.json exists and load it directly
	fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}
	stateStore := wizardbusiness.NewStateStore(fileServiceAdapter)

	// If state.json exists, load it directly without dialog
	if stateStore.StateExists("") {
		stateFile, err := stateStore.LoadCurrentState()
		if err != nil {
			debuglog.WarnLog("ShowConfigWizard: failed to load state.json: %v, falling back to config.json", err)
			// Fallback to config.json/template
			loadConfigFromFile(presenter, fileServiceAdapter, templateData, model, wizardWindow)
		} else {
			// Load state into model
			if err := presenter.LoadState(stateFile); err != nil {
				debuglog.WarnLog("ShowConfigWizard: failed to restore state: %v, falling back to config.json", err)
				// Fallback to config.json/template
				loadConfigFromFile(presenter, fileServiceAdapter, templateData, model, wizardWindow)
			} else {
				debuglog.InfoLog("ShowConfigWizard: loaded state from state.json")
			}
		}
	} else {
		// No state.json - load from config.json/template (current behavior)
		loadConfigFromFile(presenter, fileServiceAdapter, templateData, model, wizardWindow)
	}

	// Continue with wizard initialization
	// InitializeTemplateState вызывается внутри initializeWizardContent
	initializeWizardContent(presenter, guiState, wizardWindow, model, templateData)
}

// loadConfigFromFile загружает конфигурацию из config.json или шаблона (текущее поведение).
func loadConfigFromFile(presenter *wizardpresentation.WizardPresenter, fileService wizardbusiness.FileServiceInterface, templateData *wizardtemplate.TemplateData, model *wizardmodels.WizardModel, wizardWindow fyne.Window) {
	loadedConfig, parserConfigJSON, sourceURLs, err := wizardbusiness.LoadConfigFromFile(fileService, templateData)
	if err != nil {
		debuglog.ErrorLog("loadConfigFromFile: Failed to load config: %v", err)
		dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to load existing config: %w", err))
	}
	if loadedConfig {
		model.ParserConfigJSON = parserConfigJSON
		model.SourceURLs = sourceURLs
	} else {
		// If we didn't load from template or config.json - show manual download dialog
		if model.TemplateData == nil || model.TemplateData.ParserConfig == "" {
			ac := core.GetController()
			binDir := filepath.Join(ac.FileService.ExecDir, constants.BinDirName)
			debuglog.DebugLog("wizard: showing download failed manual (template missing)")
			dialogs.ShowDownloadFailedManual(wizardWindow, "Config template missing", wizardtemplate.GetTemplateURL(), binDir)
			wizardWindow.Close()
			return
		}
	}
}

// loadStateFromFile загружает состояние из файла.
func loadStateFromFile(presenter *wizardpresentation.WizardPresenter, stateStore *wizardbusiness.StateStore, stateID string, templateData *wizardtemplate.TemplateData, model *wizardmodels.WizardModel, wizardWindow fyne.Window) {
	var stateFile *wizardmodels.WizardStateFile
	var err error

	if stateID == "" {
		// Load state.json
		stateFile, err = stateStore.LoadCurrentState()
	} else {
		// Load named state
		stateFile, err = stateStore.LoadWizardState(stateID)
		if err == nil {
			// Copy to state.json
			if err := stateStore.SaveCurrentState(stateFile); err != nil {
				debuglog.WarnLog("loadStateFromFile: failed to copy state to state.json: %v", err)
			}
		}
	}

	if err != nil {
		debuglog.ErrorLog("loadStateFromFile: failed to load state: %v", err)
		dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to load state: %w", err))
		// Fallback to config.json/template
		fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: presenter.Controller().FileService}
		loadConfigFromFile(presenter, fileServiceAdapter, templateData, model, wizardWindow)
		return
	}

	// Load state into model
	if err := presenter.LoadState(stateFile); err != nil {
		debuglog.ErrorLog("loadStateFromFile: failed to load state into model: %v", err)
		dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to restore state: %w", err))
		// Fallback to config.json/template
		fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: presenter.Controller().FileService}
		loadConfigFromFile(presenter, fileServiceAdapter, templateData, model, wizardWindow)
		return
	}
}

// initializeWizardContent инициализирует содержимое визарда (табы, кнопки и т.д.).
func initializeWizardContent(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, wizardWindow fyne.Window, model *wizardmodels.WizardModel, templateData *wizardtemplate.TemplateData) {
	// Initialize template state
	presenter.InitializeTemplateState()

	// Create tabs
	tabs, rulesTabItem, previewTabItem := createWizardTabs(presenter, guiState)

	// Create buttons
	var currentTabIndex int = 0
	createWizardButtons(presenter, guiState, wizardWindow, tabs, &currentTabIndex)

	// Setup tab change handler
	setupTabChangeHandler(presenter, guiState, wizardWindow, tabs, rulesTabItem, previewTabItem, model, &currentTabIndex)

	// Sync model to GUI after initial setup
	presenter.SyncModelToGUI()

	// Set initial window content
	setWindowContent(guiState, wizardWindow, tabs)

	// Close window via X
	wizardWindow.SetCloseIntercept(func() {
		handleCloseButton(presenter, guiState, wizardWindow)
	})

	wizardWindow.Show()
}

// createWizardTabs создает табы визарда.
// Возвращает контейнер табов и ссылки на Rules и Preview табы.
func createWizardTabs(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) (*container.AppTabs, *container.TabItem, *container.TabItem) {
	// Create first tab
	tab1 := wizardtabs.CreateSourceTab(presenter)
	tab1Item := container.NewTabItem("Sources & ParserConfig", tab1)
	tabs := container.NewAppTabs(tab1Item)
	guiState.Tabs = tabs

	// Overlay that redirects clicks to open rule dialog when present
	ac := core.GetController()
	guiState.RuleDialogOverlay = components.NewClickRedirect(ac)
	guiState.RuleDialogOverlay.Hide()

	var rulesTabItem *container.TabItem
	var previewTabItem *container.TabItem

	// Use ShowAddRuleDialog from wizard/dialogs directly
	// We need to create a wrapper that includes createRulesTab to avoid circular import
	var showAddRuleDialogWrapper func(*wizardpresentation.WizardPresenter, *wizardmodels.RuleState, int)
	var createRulesTabWrapper func(*wizardpresentation.WizardPresenter) fyne.CanvasObject
	
	// Define createRulesTabWrapper first (it depends on showAddRuleDialogWrapper)
	createRulesTabWrapper = func(p *wizardpresentation.WizardPresenter) fyne.CanvasObject {
		return wizardtabs.CreateRulesTab(p, showAddRuleDialogWrapper)
	}
	
	// Define showAddRuleDialogWrapper (it depends on createRulesTabWrapper)
	showAddRuleDialogWrapper = func(p *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int) {
		wizarddialogs.ShowAddRuleDialog(p, editRule, ruleIndex, createRulesTabWrapper)
	}

	// Устанавливаем функцию создания вкладки Rules в презентер для возможности пересоздания после LoadState
	presenter.SetCreateRulesTabFunc(createRulesTabWrapper)

	if templateTab := wizardtabs.CreateRulesTab(presenter, showAddRuleDialogWrapper); templateTab != nil {
		rulesTabItem = container.NewTabItem("Rules", templateTab)
		previewTabItem = container.NewTabItem("Preview", wizardtabs.CreatePreviewTab(presenter))
		tabs.Append(rulesTabItem)
		tabs.Append(previewTabItem)
	}

	return tabs, rulesTabItem, previewTabItem
}

// createWizardButtons создает все кнопки визарда.
func createWizardButtons(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, wizardWindow fyne.Window, tabs *container.AppTabs, currentTabIndex *int) {
	// Create state management buttons
	createStateManagementButtons(presenter, guiState, wizardWindow)

	// Create navigation buttons
	createNavigationButtons(presenter, guiState, tabs, currentTabIndex)

	// Create Save button with progress bar
	createSaveButtonWithProgress(presenter, guiState)
}

// createStateManagementButtons создает кнопки управления состояниями.
func createStateManagementButtons(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, wizardWindow fyne.Window) {
	guiState.ReadButton = widget.NewButton("Read", func() {
		handleReadButton(presenter, wizardWindow)
	})
	guiState.ReadButton.Importance = widget.MediumImportance

	guiState.SaveAsButton = widget.NewButton("Save As", func() {
		handleSaveAsButton(presenter, wizardWindow)
	})
	guiState.SaveAsButton.Importance = widget.MediumImportance
}

// createNavigationButtons создает кнопки навигации (Prev, Next, Close).
// currentTabIndex передается по ссылке для обновления в обработчиках.
func createNavigationButtons(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, tabs *container.AppTabs, currentTabIndex *int) {
	guiState.CloseButton = widget.NewButton("Close", func() {
		handleCloseButton(presenter, guiState, guiState.Window)
	})
	guiState.CloseButton.Importance = widget.HighImportance

	guiState.PrevButton = widget.NewButton("Prev", func() {
		if *currentTabIndex > 0 {
			*currentTabIndex--
			tabs.SelectTab(tabs.Items[*currentTabIndex])
		}
	})
	guiState.PrevButton.Importance = widget.HighImportance

	guiState.NextButton = widget.NewButton("Next", func() {
		if *currentTabIndex < len(tabs.Items)-1 {
			*currentTabIndex++
			tabs.SelectTab(tabs.Items[*currentTabIndex])
		}
	})
	guiState.NextButton.Importance = widget.HighImportance
}

// createSaveButtonWithProgress создает кнопку Save с прогресс-баром.
func createSaveButtonWithProgress(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState) {
	guiState.SaveButton = widget.NewButton("Save", func() {
		debuglog.InfoLog("wizard: Save button clicked")
		presenter.SaveConfig()
	})
	guiState.SaveButton.Importance = widget.HighImportance

	// Create ProgressBar for Save button
	guiState.SaveProgress = widget.NewProgressBar()
	guiState.SaveProgress.Hide()
	guiState.SaveProgress.SetValue(0)

	// Set fixed size via placeholder (same as button)
	saveButtonWidth := guiState.SaveButton.MinSize().Width
	saveButtonHeight := guiState.SaveButton.MinSize().Height

	// Create placeholder to preserve size
	guiState.SavePlaceholder = canvas.NewRectangle(color.Transparent)
	guiState.SavePlaceholder.SetMinSize(fyne.NewSize(saveButtonWidth, saveButtonHeight))
	guiState.SavePlaceholder.Show()
}

// updateNavigationButtons обновляет контейнер кнопок в зависимости от текущего таба.
func updateNavigationButtons(guiState *wizardpresentation.GUIState, tabs *container.AppTabs, currentTabIndex int) {
	totalTabs := len(tabs.Items)

	// Create save button stack
	saveButtonStack := container.NewStack(
		guiState.SavePlaceholder,
		guiState.SaveButton,
		guiState.SaveProgress,
	)

	var buttonsContent fyne.CanvasObject
	if currentTabIndex == totalTabs-1 {
		// Last tab (Preview): Close, Save As on left, Prev, Save on right
		buttonsContent = container.NewHBox(
			guiState.CloseButton,
			guiState.SaveAsButton,
			layout.NewSpacer(),
			guiState.PrevButton,
			saveButtonStack,
		)
	} else if currentTabIndex == 0 {
		// First tab: Close, Read on left, Next on right (Prev hidden)
		buttonsContent = container.NewHBox(
			guiState.CloseButton,
			guiState.ReadButton,
			layout.NewSpacer(),
			guiState.NextButton,
		)
	} else {
		// Middle tabs: Close on left, Prev and Next on right
		buttonsContent = container.NewHBox(
			guiState.CloseButton,
			layout.NewSpacer(),
			guiState.PrevButton,
			guiState.NextButton,
		)
	}
	guiState.ButtonsContainer = buttonsContent
}

// setupTabChangeHandler настраивает обработчик изменения табов.
func setupTabChangeHandler(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, wizardWindow fyne.Window, tabs *container.AppTabs, rulesTabItem *container.TabItem, previewTabItem *container.TabItem, model *wizardmodels.WizardModel, currentTabIndex *int) {
	// Initialize button container
	updateNavigationButtons(guiState, tabs, *currentTabIndex)

	// Update buttons when switching tabs
	tabs.OnChanged = func(item *container.TabItem) {
		// Sync GUI to model before switching
		presenter.SyncGUIToModel()

		// Update current tab index
		for i, tabItem := range tabs.Items {
			if tabItem == item {
				*currentTabIndex = i
				break
			}
		}

		// Handle tab-specific actions
		if item == rulesTabItem {
			// Trigger async parsing to ensure outbounds are up-to-date
			presenter.TriggerParseForPreview()
			// Refresh outbound options when switching to Rules tab
			presenter.RefreshOutboundOptions()
		}
		if item == previewTabItem {
			// Trigger async parsing (if needed)
			presenter.TriggerParseForPreview()
			// Check if preview needs recalculation due to changes on Rules tab
			if model.TemplatePreviewNeedsUpdate {
				presenter.UpdateTemplatePreviewAsync()
			}
		}

		// Update navigation buttons
		updateNavigationButtons(guiState, tabs, *currentTabIndex)

		// Update window content
		setWindowContent(guiState, wizardWindow, tabs)
	}
}

// setWindowContent устанавливает содержимое окна визарда.
func setWindowContent(guiState *wizardpresentation.GUIState, wizardWindow fyne.Window, tabs *container.AppTabs) {
	content := container.NewBorder(
		nil,                       // top
		guiState.ButtonsContainer, // bottom
		nil,                       // left
		nil,                       // right
		tabs,                      // center
	)
	if guiState.RuleDialogOverlay != nil {
		content = container.NewMax(content, guiState.RuleDialogOverlay)
	}
	wizardWindow.SetContent(content)
}

// handleReadButton обрабатывает нажатие кнопки "Read".
func handleReadButton(presenter *wizardpresentation.WizardPresenter, wizardWindow fyne.Window) {
	// Проверяем наличие несохранённых изменений
	if presenter.HasUnsavedChanges() {
		// Показываем диалог подтверждения
		dialog.ShowConfirm("Confirmation", "Current changes will be lost. Save current state?",
			func(save bool) {
				if save {
					// Show "Save As" dialog
					wizarddialogs.ShowSaveStateDialog(presenter, func(result wizarddialogs.SaveStateResult) {
						if result.Action == "save" {
							if err := presenter.SaveStateAs(result.Comment, result.ID); err != nil {
								dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to save state: %w", err))
								return
							}
							// Continue loading after saving
							loadStateFromRead(presenter, wizardWindow)
						}
					})
				} else {
					// Continue loading without saving
					loadStateFromRead(presenter, wizardWindow)
				}
			}, wizardWindow)
	} else {
		// Нет изменений - сразу загружаем
		loadStateFromRead(presenter, wizardWindow)
	}
}

// loadStateFromRead загружает состояние через кнопку "Read".
// Использует ShowLoadStateDialog для выбора состояния.
func loadStateFromRead(presenter *wizardpresentation.WizardPresenter, wizardWindow fyne.Window) {
	wizarddialogs.ShowLoadStateDialog(presenter, func(result wizarddialogs.LoadStateResult) {
		if result.Action == "cancel" {
			return
		}

		if result.Action == "new" {
			// "New" - инициализировать новое состояние из шаблона/config.json
			// Игнорируем все сохранённые состояния
			ac := core.GetController()
			fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}
			model := presenter.Model()
			
			// Если TemplateData ещё не загружен, загружаем его
			if model.TemplateData == nil {
				templateLoader := &wizardbusiness.DefaultTemplateLoader{}
				templateData, err := templateLoader.LoadTemplateData(ac.FileService.ExecDir)
				if err != nil {
					binDir := filepath.Join(ac.FileService.ExecDir, constants.BinDirName)
					debuglog.DebugLog("wizard: showing download failed manual (template load on New)")
					dialogs.ShowDownloadFailedManual(wizardWindow, "Config template failed to load", wizardtemplate.GetTemplateURL(), binDir)
					return
				}
				model.TemplateData = templateData
			}
			
			// Загружаем конфигурацию из config.json или шаблона
			loadConfigFromFile(presenter, fileServiceAdapter, model.TemplateData, model, wizardWindow)
			
			// Сбрасываем флаг изменений, так как это новая конфигурация
			presenter.MarkAsSaved()
			
			// Синхронизируем GUI
			presenter.SyncModelToGUI()
			return
		}

		// Загружаем выбранное состояние
		stateStore := presenter.GetStateStore()
		var stateFile *wizardmodels.WizardStateFile
		var loadErr error

		if result.SelectedID == "" {
			// Загружаем state.json
			stateFile, loadErr = stateStore.LoadCurrentState()
		} else {
			// Загружаем именованное состояние
			stateFile, loadErr = stateStore.LoadWizardState(result.SelectedID)
			if loadErr == nil {
				// Копируем в state.json
				if err := stateStore.SaveCurrentState(stateFile); err != nil {
					debuglog.WarnLog("loadStateFromRead: failed to copy state to state.json: %v", err)
				}
			}
		}

		if loadErr != nil {
			dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to load state: %w", loadErr))
			return
		}

		// Загружаем состояние в модель
		if err := presenter.LoadState(stateFile); err != nil {
			dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to restore state: %w", err))
			return
		}

		// Синхронизируем GUI
		presenter.SyncModelToGUI()
	})
}

// handleSaveAsButton обрабатывает нажатие кнопки "Save As".
func handleSaveAsButton(presenter *wizardpresentation.WizardPresenter, wizardWindow fyne.Window) {
	wizarddialogs.ShowSaveStateDialog(presenter, func(result wizarddialogs.SaveStateResult) {
		if result.Action == "save" {
			if err := presenter.SaveStateAs(result.Comment, result.ID); err != nil {
				dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to save state: %w", err))
				return
			}
			// Закрываем визард после успешного сохранения
			wizardWindow.Close()
		}
	})
}

// handleCloseButton обрабатывает закрытие визарда с проверкой изменений.
func handleCloseButton(presenter *wizardpresentation.WizardPresenter, guiState *wizardpresentation.GUIState, wizardWindow fyne.Window) {
	debuglog.InfoLog("handleCloseButton: called")

	// Cancel save operation if in progress
	if guiState.SaveInProgress {
		debuglog.InfoLog("handleCloseButton: Save operation in progress, cancelling and closing")
		presenter.CancelSaveOperation()
		wizardWindow.Close()
		return
	}

	// Проверяем наличие несохранённых изменений
	hasChanges := presenter.HasUnsavedChanges()

	if hasChanges {
		// Создаем кастомный диалог с тремя кнопками: Save, Discard, Cancel
		messageLabel := widget.NewLabel("Save changes before closing?")

		var d dialog.Dialog

		saveButton := widget.NewButton("Save", func() {
			if d != nil {
				d.Hide()
			}
			// Save to state.json
			if err := presenter.SaveCurrentState(); err != nil {
				dialogs.ShowError(wizardWindow, fmt.Errorf("Failed to save state: %w", err))
				return
			}
			wizardWindow.Close()
		})
		saveButton.Importance = widget.HighImportance

		discardButton := widget.NewButton("Discard", func() {
			if d != nil {
				d.Hide()
			}
			wizardWindow.Close()
		})
		discardButton.Importance = widget.MediumImportance

		// Buttons container (без cancelButton - он будет через dismissText)
		buttonsRow := container.NewHBox(
			layout.NewSpacer(),
			saveButton,
			discardButton,
		)

		d = dialogs.NewCustom("Confirmation", messageLabel, buttonsRow, "Cancel", wizardWindow)
		d.Show()
	} else {
		// Нет изменений - закрываем без диалога
		wizardWindow.Close()
	}
}
