package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"image/color"

	"github.com/muhammadmuzzammil1998/jsonc"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/platform"
)

// WizardState хранит состояние мастера конфигурации
type WizardState struct {
	Controller *core.AppController
	Window     fyne.Window

	// Tab 1: VLESS Sources
	VLESSURLEntry        *widget.Entry
	URLStatusLabel       *widget.Label
	ParserConfigEntry    *widget.Entry
	OutboundsPreview     *widget.Entry
	OutboundsPreviewText string // Храним текст для read-only режима
	CheckURLButton       *widget.Button
	ParseButton          *widget.Button
	parserConfigUpdating bool

	// Parsed data
	ParserConfig       *core.ParserConfig
	GeneratedOutbounds []string

	// Template data for second tab
	TemplateData              *TemplateData
	TemplateSectionSelections map[string]bool
	SelectableRuleStates      []*SelectableRuleState
	TemplatePreviewEntry      *widget.Entry
	TemplatePreviewText       string
	templatePreviewUpdating   bool
	FinalOutboundSelect       *widget.Select
	SelectedFinalOutbound     string
	previewNeedsParse         bool
	autoParseInProgress       bool

	// Navigation buttons
	CloseButton      *widget.Button
	PrevButton       *widget.Button
	NextButton       *widget.Button
	SaveButton       *widget.Button
	ButtonsContainer fyne.CanvasObject
	tabs             *container.AppTabs
}

type SelectableRuleState struct {
	Rule             TemplateSelectableRule
	Enabled          bool
	SelectedOutbound string
	OutboundSelect   *widget.Select
}

const (
	defaultOutboundTag = "direct-out"
	rejectActionName   = "reject"
	rejectActionMethod = "drop"
)

// ShowConfigWizard открывает окно мастера конфигурации
func ShowConfigWizard(parent fyne.Window, controller *core.AppController) {
	state := &WizardState{
		Controller:        controller,
		previewNeedsParse: true,
	}

	// Создаем новое окно для мастера
	wizardWindow := controller.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(920, 720))
	wizardWindow.CenterOnScreen()
	state.Window = wizardWindow

	if templateData, err := loadTemplateData(controller.ExecDir); err != nil {
		log.Printf("ConfigWizard: failed to load config_template.json from %s: %v", filepath.Join(controller.ExecDir, "bin", "config_template.json"), err)
		// Show error to user
		dialog.ShowError(fmt.Errorf("Failed to load template file:\n%v\n\nPlease ensure bin/config_template.json exists and is valid.", err), wizardWindow)
	} else {
		state.TemplateData = templateData
	}

	// Создаем первую вкладку
	tab1 := createVLESSSourceTab(state)

	loadedConfig, err := loadConfigFromFile(state)
	if err != nil {
		log.Printf("ConfigWizard: Failed to load config: %v", err)
		// Показываем ошибку, но продолжаем работу с дефолтными значениями
		dialog.ShowError(fmt.Errorf("Failed to load existing config: %w", err), wizardWindow)
	}
	if !loadedConfig {
		if state.TemplateData != nil && state.TemplateData.ParserConfig != "" {
			if state.ParserConfigEntry != nil {
				state.parserConfigUpdating = true
				state.ParserConfigEntry.SetText(state.TemplateData.ParserConfig)
				state.parserConfigUpdating = false
				state.previewNeedsParse = true
			}
		} else {
			// Нет конфига и нет шаблона - показываем ошибку и закрываем визард
			dialog.ShowError(fmt.Errorf("No config found and template file (bin/config_template.json) is missing or invalid.\nPlease create config_template.json or ensure config.json exists."), wizardWindow)
			wizardWindow.Close()
			return
		}
	}

	// Инициализируем состояние шаблона
	state.initializeTemplateState()

	// Создаем контейнер с вкладками (пока только одна)
	tab1Item := container.NewTabItem("VLESS Sources & ParserConfig", tab1)
	tabs := container.NewAppTabs(tab1Item)
	var rulesTabItem *container.TabItem
	var previewTabItem *container.TabItem
	var currentTabIndex int = 0
	if templateTab := createTemplateTab(state); templateTab != nil {
		rulesTabItem = container.NewTabItem("Rules", templateTab)
		previewTabItem = container.NewTabItem("Preview", createPreviewTab(state))
		tabs.Append(rulesTabItem)
		tabs.Append(previewTabItem)
	}

	// Создаем кнопки навигации
	state.CloseButton = widget.NewButton("Close", func() {
		wizardWindow.Close()
	})
	state.CloseButton.Importance = widget.HighImportance

	state.PrevButton = widget.NewButton("Prev", func() {
		if currentTabIndex > 0 {
			currentTabIndex--
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	state.PrevButton.Importance = widget.HighImportance

	state.NextButton = widget.NewButton("Next", func() {
		if currentTabIndex < len(tabs.Items)-1 {
			currentTabIndex++
			tabs.SelectTab(tabs.Items[currentTabIndex])
		}
	})
	state.NextButton.Importance = widget.HighImportance

	state.SaveButton = widget.NewButton("Save", func() {
		if strings.TrimSpace(state.ParserConfigEntry.Text) == "" {
			dialog.ShowError(fmt.Errorf("ParserConfig is empty"), state.Window)
			return
		}
		if strings.TrimSpace(state.VLESSURLEntry.Text) == "" {
			dialog.ShowError(fmt.Errorf("VLESS URL is empty"), state.Window)
			return
		}
		if state.previewNeedsParse {
			state.triggerParseForPreview()
			dialog.ShowInformation("Parsing", "Parsing subscription... Please save once it completes.", state.Window)
			return
		}
		if state.autoParseInProgress {
			dialog.ShowInformation("Parsing", "Parsing in progress... Please wait.", state.Window)
			return
		}
		text, err := buildTemplateConfig(state)
		if err != nil {
			dialog.ShowError(err, state.Window)
			return
		}
		if path, err := state.saveConfigWithBackup(text); err != nil {
			dialog.ShowError(err, state.Window)
		} else {
			dialog.ShowInformation("Config Saved", fmt.Sprintf("Config written to %s", path), state.Window)
			state.Window.Close()
		}
	})
	state.SaveButton.Importance = widget.HighImportance

	// Сохраняем ссылку на tabs в state
	state.tabs = tabs

	// Функция обновления кнопок в зависимости от вкладки
	updateNavigationButtons := func() {
		totalTabs := len(tabs.Items)

		var buttonsContent fyne.CanvasObject
		if currentTabIndex == totalTabs-1 {
			// Последняя вкладка (Preview): Close слева, Prev и Save справа
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.PrevButton,
				state.SaveButton,
			)
		} else if currentTabIndex == 0 {
			// Первая вкладка: Close слева, Next справа (Prev скрыта)
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.NextButton,
			)
		} else {
			// Средние вкладки: Close слева, Prev и Next справа
			buttonsContent = container.NewHBox(
				state.CloseButton,
				layout.NewSpacer(),
				state.PrevButton,
				state.NextButton,
			)
		}
		state.ButtonsContainer = buttonsContent
	}

	// Инициализируем контейнер кнопок
	updateNavigationButtons()

	// Обновляем кнопки при переключении вкладок
	tabs.OnChanged = func(item *container.TabItem) {
		// Обновляем индекс текущей вкладки
		for i, tabItem := range tabs.Items {
			if tabItem == item {
				currentTabIndex = i
				break
			}
		}
		if item == previewTabItem {
			state.triggerParseForPreview()
		}
		updateNavigationButtons()
		// Обновляем Border контейнер с новыми кнопками
		content := container.NewBorder(
			nil,                    // top
			state.ButtonsContainer, // bottom
			nil,                    // left
			nil,                    // right
			tabs,                   // center
		)
		wizardWindow.SetContent(content)
	}

	// Обновляем предпросмотр после создания всех вкладок
	state.updateTemplatePreview()

	content := container.NewBorder(
		nil,                    // top
		state.ButtonsContainer, // bottom
		nil,                    // left
		nil,                    // right
		tabs,                   // center
	)

	wizardWindow.SetContent(content)
	wizardWindow.Show()
}

// createVLESSSourceTab создает первую вкладку с полями для VLESS URL и ParserConfig
func createVLESSSourceTab(state *WizardState) fyne.CanvasObject {
	// Секция 1: VLESS Subscription URL
	urlLabel := widget.NewLabel("VLESS Subscription URL:")
	urlLabel.Importance = widget.MediumImportance

	state.VLESSURLEntry = widget.NewEntry()
	state.VLESSURLEntry.SetPlaceHolder("https://example.com/subscription")
	state.VLESSURLEntry.Wrapping = fyne.TextWrapOff
	state.VLESSURLEntry.OnChanged = func(value string) {
		state.previewNeedsParse = true
		state.applyURLToParserConfig(strings.TrimSpace(value))
	}

	state.CheckURLButton = widget.NewButton("Check URL", func() {
		go checkURL(state)
	})

	state.URLStatusLabel = widget.NewLabel("")
	state.URLStatusLabel.Wrapping = fyne.TextWrapWord

	urlContainer := container.NewVBox(
		urlLabel,
		container.NewBorder(
			nil,                  // top
			nil,                  // bottom
			nil,                  // left
			state.CheckURLButton, // right - кнопка справа
			state.VLESSURLEntry,  // center - поле ввода занимает всё доступное пространство
		),
		state.URLStatusLabel,
	)

	// Секция 2: ParserConfig
	state.ParserConfigEntry = widget.NewMultiLineEntry()
	state.ParserConfigEntry.SetPlaceHolder("Enter ParserConfig JSON here...")
	state.ParserConfigEntry.Wrapping = fyne.TextWrapOff
	state.ParserConfigEntry.OnChanged = func(string) {
		if state.parserConfigUpdating {
			return
		}
		state.previewNeedsParse = true
		state.updateTemplatePreview()
		state.refreshOutboundOptions()
	}

	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	parserHeightRect := canvas.NewRectangle(color.Transparent)
	parserHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк

	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	parserConfigWithHeight := container.NewMax(
		parserHeightRect,
		state.ParserConfigEntry,
	)

	// Кнопка документации
	docButton := widget.NewButton("📖 Documentation", func() {
		docURL := "https://github.com/Leadaxe/singbox-launcher/blob/main/README.md#configuring-configjson"
		if err := platform.OpenURL(docURL); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open documentation: %w", err), state.Window)
		}
	})

	parserLabel := widget.NewLabel("ParserConfig:")
	parserLabel.Importance = widget.MediumImportance

	// Кнопка Parse (располагается слева от ParserConfig)
	state.ParseButton = widget.NewButton("Parse", func() {
		if state.autoParseInProgress {
			return
		}
		state.autoParseInProgress = true
		state.previewNeedsParse = true
		go parseAndPreview(state)
	})
	state.ParseButton.Importance = widget.MediumImportance

	headerRow := container.NewHBox(
		parserLabel,
		widget.NewLabel("  "), // небольшой отступ между текстом и кнопкой
		state.ParseButton,
		layout.NewSpacer(),
		docButton,
	)

	parserContainer := container.NewVBox(
		headerRow,
		parserConfigWithHeight,
	)

	// Секция 3: Preview Generated Outbounds
	previewLabel := widget.NewLabel("Preview")
	previewLabel.Importance = widget.MediumImportance

	// Используем Entry без Disable для черного текста, но делаем его read-only через OnChanged
	state.OutboundsPreview = widget.NewMultiLineEntry()
	state.OutboundsPreview.SetPlaceHolder("Generated outbounds will appear here after clicking Parse...")
	state.OutboundsPreview.Wrapping = fyne.TextWrapOff
	state.OutboundsPreviewText = "Generated outbounds will appear here after clicking Parse..."
	state.OutboundsPreview.SetText(state.OutboundsPreviewText)
	// Делаем поле read-only, но текст остается черным (не disabled)
	state.OutboundsPreview.OnChanged = func(text string) {
		// Восстанавливаем сохраненный текст при попытке редактирования
		if text != state.OutboundsPreviewText {
			state.OutboundsPreview.SetText(state.OutboundsPreviewText)
		}
	}

	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	previewHeightRect := canvas.NewRectangle(color.Transparent)
	previewHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк

	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	previewWithHeight := container.NewMax(
		previewHeightRect,
		state.OutboundsPreview,
	)

	previewContainer := container.NewVBox(
		previewLabel,
		previewWithHeight,
	)

	// Объединяем все секции
	content := container.NewVBox(
		widget.NewSeparator(),
		urlContainer,
		widget.NewSeparator(),
		parserContainer,
		widget.NewSeparator(),
		previewContainer,
		widget.NewSeparator(),
	)

	// Добавляем скролл для длинного контента
	scrollContainer := container.NewScroll(content)
	scrollContainer.SetMinSize(fyne.NewSize(900, 680))

	return scrollContainer
}

func createTemplateTab(state *WizardState) fyne.CanvasObject {
	if state.TemplateData == nil {
		return container.NewVBox(
			widget.NewLabel("Template file bin/config_template.json not found."),
			widget.NewLabel("Create the template file to enable this tab."),
		)
	}

	state.initializeTemplateState()

	availableOutbounds := state.getAvailableOutbounds()
	if len(availableOutbounds) == 0 {
		availableOutbounds = []string{defaultOutboundTag, rejectActionName}
	}

	rulesBox := container.NewVBox()
	if len(state.SelectableRuleStates) == 0 {
		rulesBox.Add(widget.NewLabel("No selectable rules defined in template."))
	} else {
		for i := range state.SelectableRuleStates {
			ruleState := state.SelectableRuleStates[i]
			idx := i

			// Only show outbound selector if rule has "outbound" field
			var outboundSelect *widget.Select
			var outboundRow fyne.CanvasObject
			if ruleState.Rule.HasOutbound {
				if ruleState.SelectedOutbound == "" {
					if ruleState.Rule.DefaultOutbound != "" {
						ruleState.SelectedOutbound = ruleState.Rule.DefaultOutbound
					} else {
						ruleState.SelectedOutbound = availableOutbounds[0]
					}
				}
				outboundSelect = widget.NewSelect(availableOutbounds, func(value string) {
					state.SelectableRuleStates[idx].SelectedOutbound = value
					state.updateTemplatePreview()
				})
				outboundSelect.SetSelected(ruleState.SelectedOutbound)
				if !ruleState.Enabled {
					outboundSelect.Disable()
				}
				outboundRow = container.NewHBox(
					widget.NewLabel("Outbound:"),
					outboundSelect,
				)
			}
			state.SelectableRuleStates[idx].OutboundSelect = outboundSelect

			checkbox := widget.NewCheck(ruleState.Rule.Label, func(val bool) {
				state.SelectableRuleStates[idx].Enabled = val
				if outboundSelect != nil {
					if val {
						outboundSelect.Enable()
					} else {
						outboundSelect.Disable()
					}
				}
				state.updateTemplatePreview()
			})
			checkbox.SetChecked(ruleState.Enabled)

			// Create checkbox container with optional info button for description
			checkboxContainer := container.NewHBox(checkbox)
			if ruleState.Rule.Description != "" {
				infoButton := widget.NewButton("?", func() {
					dialog.ShowInformation(ruleState.Rule.Label, ruleState.Rule.Description, state.Window)
				})
				infoButton.Importance = widget.LowImportance
				checkboxContainer.Add(infoButton)
			}

			rowContent := []fyne.CanvasObject{checkboxContainer, layout.NewSpacer()}
			if outboundRow != nil {
				rowContent = append(rowContent, outboundRow)
			}
			rulesBox.Add(container.NewHBox(rowContent...))
		}
	}

	state.ensureFinalSelected(availableOutbounds)
	finalSelect := widget.NewSelect(availableOutbounds, func(value string) {
		state.SelectedFinalOutbound = value
		state.updateTemplatePreview()
	})
	finalSelect.SetSelected(state.SelectedFinalOutbound)
	state.FinalOutboundSelect = finalSelect

	rulesScroll := createRulesScroll(state, rulesBox)

	state.refreshOutboundOptions()

	return container.NewVBox(
		widget.NewLabel("Selectable rules"),
		rulesScroll,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Final outbound:"),
			finalSelect,
			layout.NewSpacer(),
		),
	)
}

func createPreviewTab(state *WizardState) fyne.CanvasObject {
	state.TemplatePreviewEntry = widget.NewMultiLineEntry()
	state.TemplatePreviewEntry.SetPlaceHolder("Preview will appear here")
	state.TemplatePreviewEntry.Wrapping = fyne.TextWrapOff
	state.TemplatePreviewEntry.OnChanged = func(text string) {
		if state.templatePreviewUpdating {
			return
		}
		state.setTemplatePreviewText(state.TemplatePreviewText)
	}
	previewWithHeight := container.NewMax(
		canvas.NewRectangle(color.Transparent),
		state.TemplatePreviewEntry,
	)
	state.setTemplatePreviewText("Preview will appear here")

	previewScroll := container.NewVScroll(previewWithHeight)
	maxHeight := state.Window.Canvas().Size().Height * 0.7
	if maxHeight <= 0 {
		maxHeight = 480
	}
	previewScroll.SetMinSize(fyne.NewSize(0, maxHeight))

	return container.NewVBox(
		widget.NewLabel("Preview"),
		previewScroll,
	)
}

func createRulesScroll(state *WizardState, content fyne.CanvasObject) fyne.CanvasObject {
	maxHeight := state.Window.Canvas().Size().Height * 0.7
	if maxHeight <= 0 {
		maxHeight = 480
	}
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(0, maxHeight))
	return scroll
}

func (state *WizardState) saveConfigWithBackup(text string) (string, error) {
	// Validate JSON before saving (support JSONC with comments)
	jsonBytes := jsonc.ToJSON([]byte(text))
	var testJSON interface{}
	if err := json.Unmarshal(jsonBytes, &testJSON); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	configPath := state.Controller.ConfigPath
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
		backup := state.nextBackupPath(configPath)
		if err := os.Rename(configPath, backup); err != nil {
			return "", err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(configPath, []byte(text), 0o644); err != nil {
		return "", err
	}
	// Update config status in Core Dashboard if callback is set
	if state.Controller != nil && state.Controller.UpdateConfigStatusFunc != nil {
		state.Controller.UpdateConfigStatusFunc()
	}
	return configPath, nil
}

func (state *WizardState) nextBackupPath(path string) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	candidate := filepath.Join(dir, fmt.Sprintf("%s-old%s", base, ext))
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-old-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// loadConfigFromFile загружает данные из существующего config.json
func loadConfigFromFile(state *WizardState) (bool, error) {
	// Проверяем наличие config.json
	if _, err := os.Stat(state.Controller.ConfigPath); os.IsNotExist(err) {
		// Конфиг не существует - оставляем значения по умолчанию
		log.Println("ConfigWizard: config.json not found, using default values")
		return false, nil
	}

	// Извлекаем ParserConfig
	parserConfig, err := core.ExtractParcerConfig(state.Controller.ConfigPath)
	if err != nil {
		// Если не удалось извлечь - оставляем значения по умолчанию
		log.Printf("ConfigWizard: Failed to extract ParserConfig: %v", err)
		return false, nil // Не критическая ошибка
	}

	state.ParserConfig = parserConfig

	// Заполняем поле URL
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		state.VLESSURLEntry.SetText(parserConfig.ParserConfig.Proxies[0].Source)
	}

	parserConfigJSON, err := serializeParserConfig(parserConfig)
	if err != nil {
		log.Printf("ConfigWizard: Failed to serialize ParserConfig: %v", err)
		return false, err
	}

	state.parserConfigUpdating = true
	state.ParserConfigEntry.SetText(string(parserConfigJSON))
	state.parserConfigUpdating = false
	state.previewNeedsParse = true

	log.Println("ConfigWizard: Successfully loaded config from file")
	return true, nil
}

// checkURL проверяет доступность URL подписки
func checkURL(state *WizardState) {
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	if url == "" {
		fyne.Do(func() {
			state.URLStatusLabel.SetText("❌ Please enter a URL")
		})
		return
	}

	// Обновляем UI
	fyne.Do(func() {
		state.URLStatusLabel.SetText("⏳ Checking...")
		state.CheckURLButton.Disable()
	})

	// Проверяем URL в горутине
	content, err := core.FetchSubscription(url)
	if err != nil {
		fyne.Do(func() {
			state.URLStatusLabel.SetText(fmt.Sprintf("❌ Failed: %v", err))
			state.CheckURLButton.Enable()
		})
		return
	}

	// Проверяем, что контент не пустой и содержит хотя бы одну строку
	lines := strings.Split(string(content), "\n")
	validLines := 0
	previewLines := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "vmess://") || strings.HasPrefix(line, "trojan://") || strings.HasPrefix(line, "ss://")) {
			validLines++
			previewLines = append(previewLines, fmt.Sprintf("%d. %s", validLines, line))
		}
	}

	if validLines == 0 {
		fyne.Do(func() {
			state.URLStatusLabel.SetText("❌ URL is accessible but contains no valid proxy links")
			state.CheckURLButton.Enable()
		})
		return
	}

	fyne.Do(func() {
		state.URLStatusLabel.SetText(fmt.Sprintf("✅ Working! Found %d valid proxy link(s)", validLines))
		state.CheckURLButton.Enable()
		if len(previewLines) > 0 {
			setPreviewText(state, strings.Join(previewLines, "\n"))
		} else {
			setPreviewText(state, "No valid proxy links found to preview.")
		}
	})
}

// parseAndPreview парсит ParserConfig и генерирует предпросмотр outbounds
func parseAndPreview(state *WizardState) {
	defer func() {
		fyne.Do(func() {
			state.autoParseInProgress = false
		})
	}()
	fyne.Do(func() {
		state.ParseButton.Disable()
		state.ParseButton.SetText("Parsing...")
		setPreviewText(state, "Parsing configuration...")
	})

	// Парсим ParserConfig из поля
	parserConfigJSON := strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON == "" {
		fyne.Do(func() {
			setPreviewText(state, "Error: ParserConfig is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		fyne.Do(func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to parse ParserConfig JSON: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Проверяем наличие URL
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	if url == "" {
		fyne.Do(func() {
			setPreviewText(state, "Error: VLESS URL is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Обновляем URL в конфиге, если он отличается
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		parserConfig.ParserConfig.Proxies[0].Source = url
	} else {
		// Добавляем новый источник, если его нет
		parserConfig.ParserConfig.Proxies = []core.ProxySource{
			{Source: url},
		}
	}

	// Загружаем подписку
	fyne.Do(func() {
		setPreviewText(state, "Downloading subscription...")
	})

	content, err := core.FetchSubscription(url)
	if err != nil {
		fyne.Do(func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to fetch subscription: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Парсим узлы из подписки
	fyne.Do(func() {
		setPreviewText(state, "Parsing nodes from subscription...")
	})

	allNodes := make([]*core.ParsedNode, 0)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var skipFilters []map[string]string
		if len(parserConfig.ParserConfig.Proxies) > 0 {
			skipFilters = parserConfig.ParserConfig.Proxies[0].Skip
		}

		node, err := parseNodeFromString(line, skipFilters)
		if err != nil {
			log.Printf("ConfigWizard: Failed to parse node: %v", err)
			continue
		}

		if node != nil {
			allNodes = append(allNodes, node)
		}
	}

	if len(allNodes) == 0 {
		fyne.Do(func() {
			setPreviewText(state, "Error: No valid nodes found in subscription")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
		})
		return
	}

	// Генерируем JSON для узлов
	fyne.Do(func() {
		setPreviewText(state, "Generating outbounds...")
	})

	selectorsJSON := make([]string, 0)

	// Генерируем JSON для всех узлов
	for _, node := range allNodes {
		nodeJSON, err := generateNodeJSONForPreview(node)
		if err != nil {
			log.Printf("ConfigWizard: Failed to generate JSON for node: %v", err)
			continue
		}
		selectorsJSON = append(selectorsJSON, nodeJSON)
	}

	// Генерируем селекторы
	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		selectorJSON, err := generateSelectorForPreview(allNodes, outboundConfig)
		if err != nil {
			log.Printf("ConfigWizard: Failed to generate selector: %v", err)
			continue
		}
		if selectorJSON != "" {
			selectorsJSON = append(selectorsJSON, selectorJSON)
		}
	}

	// Формируем итоговый текст для предпросмотра
	previewText := strings.Join(selectorsJSON, "\n")

	fyne.Do(func() {
		setPreviewText(state, previewText)
		state.ParseButton.Enable()
		state.ParseButton.SetText("Parse")
		state.GeneratedOutbounds = selectorsJSON
		state.ParserConfig = &parserConfig
		state.previewNeedsParse = false
		state.refreshOutboundOptions()
		state.updateTemplatePreview()
	})
}

func setPreviewText(state *WizardState, text string) {
	state.OutboundsPreviewText = text
	if state.OutboundsPreview != nil {
		state.OutboundsPreview.SetText(text)
	}
}

func (state *WizardState) applyURLToParserConfig(url string) {
	if state.ParserConfigEntry == nil || url == "" {
		return
	}
	text := strings.TrimSpace(state.ParserConfigEntry.Text)
	if text == "" {
		return
	}
	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(text), &parserConfig); err != nil {
		return
	}
	if len(parserConfig.ParserConfig.Proxies) == 0 {
		parserConfig.ParserConfig.Proxies = []core.ProxySource{
			{Source: url},
		}
	} else {
		parserConfig.ParserConfig.Proxies[0].Source = url
	}
	serialized, err := serializeParserConfig(&parserConfig)
	if err != nil {
		return
	}
	state.parserConfigUpdating = true
	state.ParserConfigEntry.SetText(serialized)
	state.parserConfigUpdating = false
	state.ParserConfig = &parserConfig
	state.previewNeedsParse = true
}

func (state *WizardState) setTemplatePreviewText(text string) {
	state.TemplatePreviewText = text
	if state.TemplatePreviewEntry == nil {
		return
	}
	state.templatePreviewUpdating = true
	state.TemplatePreviewEntry.SetText(text)
	state.templatePreviewUpdating = false
}

func (state *WizardState) refreshOutboundOptions() {
	if len(state.SelectableRuleStates) == 0 && state.FinalOutboundSelect == nil {
		return
	}
	options := state.getAvailableOutbounds()
	if len(options) == 0 {
		options = []string{defaultOutboundTag, rejectActionName}
	}

	ensureSelected := func(ruleState *SelectableRuleState) {
		if !ruleState.Rule.HasOutbound {
			return
		}
		if ruleState.SelectedOutbound != "" && containsString(options, ruleState.SelectedOutbound) {
			return
		}
		candidate := ruleState.Rule.DefaultOutbound
		if candidate == "" || !containsString(options, candidate) {
			candidate = options[0]
		}
		ruleState.SelectedOutbound = candidate
	}

	state.ensureFinalSelected(options)

	fyne.Do(func() {
		for _, ruleState := range state.SelectableRuleStates {
			if !ruleState.Rule.HasOutbound || ruleState.OutboundSelect == nil {
				continue
			}
			ensureSelected(ruleState)
			ruleState.OutboundSelect.Options = options
			ruleState.OutboundSelect.SetSelected(ruleState.SelectedOutbound)
			ruleState.OutboundSelect.Refresh()
		}
		if state.FinalOutboundSelect != nil {
			state.FinalOutboundSelect.Options = options
			state.FinalOutboundSelect.SetSelected(state.SelectedFinalOutbound)
			state.FinalOutboundSelect.Refresh()
		}
	})
}

func (state *WizardState) triggerParseForPreview() {
	if state.autoParseInProgress {
		return
	}
	if !state.previewNeedsParse && len(state.GeneratedOutbounds) > 0 {
		return
	}
	if state.VLESSURLEntry == nil || state.ParserConfigEntry == nil {
		return
	}
	if strings.TrimSpace(state.VLESSURLEntry.Text) == "" || strings.TrimSpace(state.ParserConfigEntry.Text) == "" {
		return
	}
	state.autoParseInProgress = true
	go parseAndPreview(state)
}

func (state *WizardState) updateTemplatePreview() {
	if state.TemplateData == nil || state.TemplatePreviewEntry == nil {
		return
	}
	text, err := buildTemplateConfig(state)
	if err != nil {
		state.setTemplatePreviewText(fmt.Sprintf("Preview error: %v", err))
		return
	}
	state.setTemplatePreviewText(text)
}

func buildTemplateConfig(state *WizardState) (string, error) {
	if state.TemplateData == nil {
		return "", fmt.Errorf("template data not available")
	}
	parserConfig := strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfig == "" {
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}
	sections := make([]string, 0)
	for _, key := range state.TemplateData.SectionOrder {
		if selected, ok := state.TemplateSectionSelections[key]; !ok || !selected {
			continue
		}
		raw := state.TemplateData.Sections[key]
		var formatted string
		var err error
		if key == "outbounds" && state.TemplateData.HasParserOutboundsBlock {
			// If template had @PARSER_OUTBOUNDS_BLOCK marker, replace entire outbounds array
			// with generated content
			content := state.buildParserOutboundsBlock()

			// Add elements after marker if they exist (any elements, not just direct-out)
			if state.TemplateData.OutboundsAfterMarker != "" {
				// Убираем лишние пробелы и запятые
				cleaned := strings.TrimSpace(state.TemplateData.OutboundsAfterMarker)
				cleaned = strings.TrimRight(cleaned, ",")
				if cleaned != "" {
					indented := indentMultiline(cleaned, "    ")
					// НЕ добавляем запятую перед элементами - она уже есть после последнего элемента перед @ParserEND
					content += "\n" + indented
				}
			}
			// Всегда добавляем \n в конце content перед закрывающей скобкой
			content += "\n"

			// Wrap content in array brackets
			formatted = "[\n" + content + "\n  ]"
		} else if key == "route" {
			merged, err := mergeRouteSection(raw, state.SelectableRuleStates, state.SelectedFinalOutbound)
			if err != nil {
				return "", fmt.Errorf("route merge failed: %w", err)
			}
			raw = merged
			formatted, err = formatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
		} else {
			formatted, err = formatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
		}
		sections = append(sections, fmt.Sprintf(`  "%s": %s`, key, formatted))
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("no sections selected")
	}
	var builder strings.Builder
	builder.WriteString("{\n")
	builder.WriteString("/** @ParcerConfig\n")
	builder.WriteString(parserConfig)
	builder.WriteString("\n*/\n")
	builder.WriteString(strings.Join(sections, ",\n"))
	builder.WriteString("\n}\n")
	result := builder.String()
	return result, nil
}

func mergeRouteSection(raw json.RawMessage, states []*SelectableRuleState, finalOutbound string) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}
	var rules []interface{}
	if existing, ok := route["rules"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			rules = arr
		} else {
			rules = []interface{}{existing}
		}
	}
	for _, state := range states {
		if !state.Enabled {
			continue
		}
		cloned := cloneRule(state.Rule)
		outbound := state.SelectedOutbound
		if outbound == "" {
			outbound = state.Rule.DefaultOutbound
		}
		if outbound != "" {
			if outbound == rejectActionName {
				delete(cloned, "outbound")
				cloned["action"] = rejectActionName
				cloned["method"] = rejectActionMethod
			} else {
				cloned["outbound"] = outbound
				delete(cloned, "action")
				delete(cloned, "method")
			}
		}
		rules = append(rules, cloned)
	}
	if len(rules) > 0 {
		route["rules"] = rules
	}
	if finalOutbound != "" {
		route["final"] = finalOutbound
	}
	return json.Marshal(route)
}

func cloneRule(rule TemplateSelectableRule) map[string]interface{} {
	cloned := make(map[string]interface{}, len(rule.Raw))
	for key, value := range rule.Raw {
		cloned[key] = value
	}
	return cloned
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (state *WizardState) buildParserOutboundsBlock() string {
	const indent = "    "
	var builder strings.Builder
	builder.WriteString(indent + "/** @ParserSTART */\n")
	count := len(state.GeneratedOutbounds)
	// Проверяем, есть ли элементы после маркера (любые, не только direct-out)
	hasAfterMarker := state.TemplateData != nil &&
		strings.TrimSpace(state.TemplateData.OutboundsAfterMarker) != ""

	for idx, entry := range state.GeneratedOutbounds {
		// Убираем запятые и пробелы в конце строки, если они есть
		cleaned := strings.TrimRight(entry, ",\n\r\t ")
		indented := indentMultiline(cleaned, indent)
		builder.WriteString(indented)
		// Добавляем запятую:
		// - если не последний элемент (всегда)
		// - или если последний элемент И есть элементы после маркера
		if idx < count-1 || hasAfterMarker {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
	}
	endLine := indent + "/** @ParserEND */"
	builder.WriteString(endLine) // Без запятой и без \n
	return builder.String()
}

func indentMultiline(text, indent string) string {
	if text == "" {
		return indent
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func (state *WizardState) ensureFinalSelected(options []string) {
	if len(options) == 0 {
		options = []string{defaultOutboundTag, rejectActionName}
	}
	preferred := state.SelectedFinalOutbound
	if preferred == "" && state.TemplateData != nil && state.TemplateData.DefaultFinal != "" {
		preferred = state.TemplateData.DefaultFinal
	}
	if preferred == "" {
		preferred = defaultOutboundTag
	}
	if !containsString(options, preferred) {
		if state.TemplateData != nil && state.TemplateData.DefaultFinal != "" && containsString(options, state.TemplateData.DefaultFinal) {
			preferred = state.TemplateData.DefaultFinal
		} else if containsString(options, defaultOutboundTag) {
			preferred = defaultOutboundTag
		} else {
			preferred = options[0]
		}
	}
	state.SelectedFinalOutbound = preferred
}

func formatSectionJSON(raw json.RawMessage, indentLevel int) (string, error) {
	var buf bytes.Buffer
	prefix := strings.Repeat(" ", indentLevel)
	if err := json.Indent(&buf, raw, prefix, "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (state *WizardState) initializeTemplateState() {
	if state.TemplateData == nil {
		return
	}
	if state.TemplateSectionSelections == nil {
		state.TemplateSectionSelections = make(map[string]bool)
	}
	for _, key := range state.TemplateData.SectionOrder {
		if _, ok := state.TemplateSectionSelections[key]; !ok {
			state.TemplateSectionSelections[key] = true
		}
	}
	options := state.getAvailableOutbounds()
	if len(options) == 0 {
		options = []string{defaultOutboundTag, rejectActionName}
	}

	if len(state.SelectableRuleStates) == 0 {
		for _, rule := range state.TemplateData.SelectableRules {
			outbound := rule.DefaultOutbound
			if outbound == "" {
				outbound = options[0]
			}
			state.SelectableRuleStates = append(state.SelectableRuleStates, &SelectableRuleState{
				Rule:             rule,
				SelectedOutbound: outbound,
				Enabled:          rule.IsDefault, // Enable rule if @default directive is present
			})
		}
	} else {
		for _, ruleState := range state.SelectableRuleStates {
			if ruleState.SelectedOutbound == "" {
				if ruleState.Rule.DefaultOutbound != "" {
					ruleState.SelectedOutbound = ruleState.Rule.DefaultOutbound
				} else {
					ruleState.SelectedOutbound = options[0]
				}
			}
		}
	}

	state.ensureFinalSelected(options)
	// Не вызываем updateTemplatePreview здесь - он будет вызван после создания всех вкладок
}

func (state *WizardState) getAvailableOutbounds() []string {
	tags := map[string]struct{}{
		defaultOutboundTag: {},
		rejectActionName:   {},
	}
	var parserCfg *core.ParserConfig
	if state.ParserConfig != nil {
		parserCfg = state.ParserConfig
	} else if state.ParserConfigEntry != nil && state.ParserConfigEntry.Text != "" {
		var parsed core.ParserConfig
		if err := json.Unmarshal([]byte(state.ParserConfigEntry.Text), &parsed); err == nil {
			parserCfg = &parsed
		}
	}
	if parserCfg != nil {
		for _, outbound := range parserCfg.ParserConfig.Outbounds {
			if outbound.Tag != "" {
				tags[outbound.Tag] = struct{}{}
			}
			for _, extra := range outbound.Outbounds.AddOutbounds {
				tags[extra] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

// parseNodeFromString парсит узел из строки (обертка над core.ParseNode)
func parseNodeFromString(uri string, skipFilters []map[string]string) (*core.ParsedNode, error) {
	return core.ParseNode(uri, skipFilters)
}

// generateNodeJSONForPreview генерирует JSON для узла (обертка над core.GenerateNodeJSON)
func generateNodeJSONForPreview(node *core.ParsedNode) (string, error) {
	return core.GenerateNodeJSON(node)
}

// generateSelectorForPreview генерирует JSON для селектора (обертка над core.GenerateSelector)
func generateSelectorForPreview(allNodes []*core.ParsedNode, outboundConfig core.OutboundConfig) (string, error) {
	return core.GenerateSelector(allNodes, outboundConfig)
}

func serializeParserConfig(parserConfig *core.ParserConfig) (string, error) {
	if parserConfig == nil {
		return "", fmt.Errorf("parserConfig is nil")
	}
	configToSerialize := map[string]interface{}{
		"version": parserConfig.Version,
		"ParserConfig": map[string]interface{}{
			"proxies":   parserConfig.ParserConfig.Proxies,
			"outbounds": parserConfig.ParserConfig.Outbounds,
		},
	}
	data, err := json.MarshalIndent(configToSerialize, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
