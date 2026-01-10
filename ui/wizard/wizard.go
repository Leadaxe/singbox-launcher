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
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizarddialogs "singbox-launcher/ui/wizard/dialogs"
	wizardpresentation "singbox-launcher/ui/wizard/presentation"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtabs "singbox-launcher/ui/wizard/tabs"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// ShowConfigWizard opens the configuration wizard window.
func ShowConfigWizard(parent fyne.Window, controller *core.AppController) {
	// Create model and GUI state
	model := wizardmodels.NewWizardModel()
	guiState := &wizardpresentation.GUIState{}

	// Load template data
	templateLoader := &wizardbusiness.DefaultTemplateLoader{}
	templateData, err := templateLoader.LoadTemplateData(controller.FileService.ExecDir)
	if err != nil {
		templateFileName := wizardtemplate.GetTemplateFileName()
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: failed to load %s from %s: %v", templateFileName, filepath.Join(controller.FileService.ExecDir, "bin", templateFileName), err)
		// Update config status in Core Dashboard
		if controller.UIService != nil && controller.UIService.UpdateConfigStatusFunc != nil {
			controller.UIService.UpdateConfigStatusFunc()
		}
		return
	}
	model.TemplateData = templateData

	// Create new window for wizard
	wizardWindow := controller.UIService.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(620, 660))
	wizardWindow.CenterOnScreen()
	guiState.Window = wizardWindow

	// Create presenter
	presenter := wizardpresentation.NewWizardPresenter(model, guiState, controller, templateLoader)

	// Load config from file
	fileService := &wizardbusiness.FileServiceAdapter{FileService: controller.FileService}
	loadedConfig, parserConfigJSON, sourceURLs, err := wizardbusiness.LoadConfigFromFile(fileService, templateData)
	if err != nil {
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "ConfigWizard: Failed to load config: %v", err)
		dialog.ShowError(fmt.Errorf("Failed to load existing config: %w", err), wizardWindow)
	}
	if loadedConfig {
		model.ParserConfigJSON = parserConfigJSON
		model.SourceURLs = sourceURLs
	} else {
		// If we didn't load from template or config.json - show error
		if model.TemplateData == nil || model.TemplateData.ParserConfig == "" {
			templateFileName := wizardtemplate.GetTemplateFileName()
			dialog.ShowError(fmt.Errorf("No config found and template file (bin/%s) is missing or invalid.\nPlease create %s or ensure config.json exists.", templateFileName, templateFileName), wizardWindow)
			wizardWindow.Close()
			return
		}
	}

	// Initialize template state
	presenter.InitializeTemplateState()

	// Create first tab
	tab1 := wizardtabs.CreateSourceTab(presenter)

	// Create container with tabs (only one for now)
	tab1Item := container.NewTabItem("Sources & ParserConfig", tab1)
	tabs := container.NewAppTabs(tab1Item)
	guiState.Tabs = tabs
	var rulesTabItem *container.TabItem
	var previewTabItem *container.TabItem
	var currentTabIndex int = 0
	// Use ShowAddRuleDialog from wizard/dialogs directly
	showAddRuleDialogWrapper := func(p *wizardpresentation.WizardPresenter, editRule *wizardmodels.RuleState, ruleIndex int) {
		wizarddialogs.ShowAddRuleDialog(p, editRule, ruleIndex)
	}
	if templateTab := wizardtabs.CreateRulesTab(presenter, showAddRuleDialogWrapper); templateTab != nil {
		rulesTabItem = container.NewTabItem("Rules", templateTab)
		previewTabItem = container.NewTabItem("Preview", wizardtabs.CreatePreviewTab(presenter))
		tabs.Append(rulesTabItem)
		tabs.Append(previewTabItem)
	}

	// Create navigation buttons
	guiState.CloseButton = widget.NewButton("Close", func() {
		wizardWindow.Close()
	})

	// Close window via X
	wizardWindow.SetCloseIntercept(func() {
		wizardWindow.Close()
	})
	guiState.CloseButton.Importance = widget.HighImportance

	guiState.PrevButton = widget.NewButton("Prev", func() {
		if currentTabIndex > 0 {
			currentTabIndex--
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	guiState.PrevButton.Importance = widget.HighImportance

	guiState.NextButton = widget.NewButton("Next", func() {
		if currentTabIndex < len(tabs.Items)-1 {
			currentTabIndex++
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	guiState.NextButton.Importance = widget.HighImportance

	guiState.SaveButton = widget.NewButton("Save", func() {
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

	// Create container with stack for Save button (placeholder, button, progress)
	// Create container with stack for Save button (placeholder, button, progress)
	saveButtonStack := container.NewStack(
		guiState.SavePlaceholder,
		guiState.SaveButton,
		guiState.SaveProgress,
	)

	// Function to update buttons based on tab
	updateNavigationButtons := func() {
		totalTabs := len(tabs.Items)

		var buttonsContent fyne.CanvasObject
		if currentTabIndex == totalTabs-1 {
			// Last tab (Preview): Close on left, Prev and Save on right
			buttonsContent = container.NewHBox(
				guiState.CloseButton,
				layout.NewSpacer(),
				guiState.PrevButton,
				saveButtonStack, // Use stack with ProgressBar
			)
		} else if currentTabIndex == 0 {
			// First tab: Close on left, Next on right (Prev hidden)
			buttonsContent = container.NewHBox(
				guiState.CloseButton,
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

	// Initialize button container
	updateNavigationButtons()

	// Sync model to GUI after initial setup
	presenter.SyncModelToGUI()

	// Update buttons when switching tabs
	tabs.OnChanged = func(item *container.TabItem) {
		// Sync GUI to model before switching
		presenter.SyncGUIToModel()

		// Update current tab index
		for i, tabItem := range tabs.Items {
			if tabItem == item {
				currentTabIndex = i
				break
			}
		}
		if item == previewTabItem {
			// Trigger async parsing (if needed)
			presenter.TriggerParseForPreview()
			// Check if preview needs recalculation due to changes on Rules tab
			if model.TemplatePreviewNeedsUpdate {
				presenter.UpdateTemplatePreviewAsync()
			}
		}
		updateNavigationButtons()
		// Update Border container with new buttons
		content := container.NewBorder(
			nil,                      // top
			guiState.ButtonsContainer, // bottom
			nil,                      // left
			nil,                      // right
			tabs,                     // center
		)
		wizardWindow.SetContent(content)
	}

	// Preview is generated only via "Show Preview" button

	content := container.NewBorder(
		nil,                      // top
		guiState.ButtonsContainer, // bottom
		nil,                      // left
		nil,                      // right
		tabs,                     // center
	)

	wizardWindow.SetContent(content)
	wizardWindow.Show()
}
