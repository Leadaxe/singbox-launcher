package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"image/color"

	"github.com/muhammadmuzzammil1998/jsonc"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/parsers"
	"singbox-launcher/internal/platform"
)

// maxNodesForFullPreview is the maximum number of nodes to show full preview.
// If nodes count exceeds this value, statistics comment will be shown instead.
const maxNodesForFullPreview = 20

// generateTagPrefix generates a tag prefix for a subscription based on its index.
// Format: "1:", "2:", "3:", etc.
// This function can be easily modified to change the prefix format.
func generateTagPrefix(index int) string {
	return fmt.Sprintf("%d:", index)
}

// ShowConfigWizard открывает окно мастера конфигурации
func ShowConfigWizard(parent fyne.Window, controller *core.AppController) {
	state := &WizardState{
		Controller:        controller,
		previewNeedsParse: true,
	}

	if templateData, err := loadTemplateData(controller.ExecDir); err != nil {
		templateFileName := GetTemplateFileName()
		errorLog("ConfigWizard: failed to load %s from %s: %v", templateFileName, filepath.Join(controller.ExecDir, "bin", templateFileName), err)
		// Update config status in Core Dashboard (similar to UpdateConfigStatusFunc)
		if controller.UpdateConfigStatusFunc != nil {
			controller.UpdateConfigStatusFunc()
		}
		// Show error to user
		//	dialog.ShowError(fmt.Errorf("Failed to load template file:\n%v\n\nPlease ensure bin/config_template.json exists and is valid.", err), wizardWindow)
		return
	} else {
		state.TemplateData = templateData
	}

	// Создаем новое окно для мастера
	wizardWindow := controller.Application.NewWindow("Config Wizard")
	wizardWindow.Resize(fyne.NewSize(620, 660))
	wizardWindow.CenterOnScreen()
	state.Window = wizardWindow

	// Создаем первую вкладку
	tab1 := createVLESSSourceTab(state)

	loadedConfig, err := loadConfigFromFile(state)
	if err != nil {
		errorLog("ConfigWizard: Failed to load config: %v", err)
		dialog.ShowError(fmt.Errorf("Failed to load existing config: %w", err), wizardWindow)
	}
	if !loadedConfig {
		// Если не загрузили ни из шаблона, ни из config.json - показываем ошибку
		if state.TemplateData == nil || state.TemplateData.ParserConfig == "" {
			templateFileName := GetTemplateFileName()
			dialog.ShowError(fmt.Errorf("No config found and template file (bin/%s) is missing or invalid.\nPlease create %s or ensure config.json exists.", templateFileName, templateFileName), wizardWindow)
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

	// Закрытие окна через X
	wizardWindow.SetCloseIntercept(func() {
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
		if state.saveInProgress {
			dialog.ShowInformation("Saving", "Save operation already in progress... Please wait.", state.Window)
			return
		}
		if state.autoParseInProgress {
			dialog.ShowInformation("Parsing", "Parsing in progress... Please wait.", state.Window)
			return
		}

		// Начинаем асинхронное сохранение с индикацией прогресса
		state.setSaveState("", 0.0) // Показываем прогресс-бар
		go func() {
			defer safeFyneDo(state.Window, func() {
				state.setSaveState("Save", -1) // Скрываем прогресс, показываем кнопку
			})

			// Шаг 0: Проверяем и ждем парсинг, если нужно (0-40%)
			if state.previewNeedsParse || state.autoParseInProgress {
				safeFyneDo(state.Window, func() {
					state.SaveProgress.SetValue(0.05)
				})

				// Если парсинг еще не запущен, запускаем его
				if !state.autoParseInProgress {
					state.autoParseInProgress = true
					go parseAndPreview(state)
				}

				// Ждем завершения парсинга (проверяем каждые 100мс)
				maxWaitTime := 60 * time.Second // Максимальное время ожидания
				startTime := time.Now()
				iterations := 0
				for state.autoParseInProgress {
					if time.Since(startTime) > maxWaitTime {
						safeFyneDo(state.Window, func() {
							dialog.ShowError(fmt.Errorf("Parsing timeout: operation took too long"), state.Window)
						})
						return
					}
					time.Sleep(100 * time.Millisecond)
					iterations++
					// Обновляем прогресс плавно (0.05 - 0.40)
					// Показываем, что процесс идет
					progressRange := 0.35
					baseProgress := 0.05
					// Плавное движение вперед с циклическим эффектом
					cycleProgress := float64(iterations%40) / 40.0
					currentProgress := baseProgress + cycleProgress*progressRange
					safeFyneDo(state.Window, func() {
						state.SaveProgress.SetValue(currentProgress)
					})
				}
				safeFyneDo(state.Window, func() {
					state.SaveProgress.SetValue(0.4)
				})
			}

			// Шаг 1: Строим конфиг (40-80%)
			safeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.4)
			})
			text, err := buildTemplateConfig(state, false)
			if err != nil {
				safeFyneDo(state.Window, func() {
					dialog.ShowError(err, state.Window)
				})
				return
			}
			safeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.8)
			})

			// Шаг 2: Сохраняем файл (80-95%)
			path, err := state.saveConfigWithBackup(text)
			if err != nil {
				safeFyneDo(state.Window, func() {
					dialog.ShowError(err, state.Window)
				})
				return
			}
			safeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(0.95)
			})

			// Шаг 3: Завершение (95-100%)
			time.Sleep(100 * time.Millisecond)
			safeFyneDo(state.Window, func() {
				state.SaveProgress.SetValue(1.0)
			})
			// Небольшая задержка, чтобы пользователь увидел прогресс
			time.Sleep(200 * time.Millisecond)

			// Успешно сохранено
			safeFyneDo(state.Window, func() {
				dialog.ShowInformation("Config Saved", fmt.Sprintf("Config written to %s", path), state.Window)
				state.Window.Close()
			})
		}()
	})
	state.SaveButton.Importance = widget.HighImportance

	// Создаем ProgressBar для кнопки Save
	state.SaveProgress = widget.NewProgressBar()
	state.SaveProgress.Hide()
	state.SaveProgress.SetValue(0)

	// Устанавливаем фиксированный размер через placeholder (такой же как кнопка)
	saveButtonWidth := state.SaveButton.MinSize().Width
	saveButtonHeight := state.SaveButton.MinSize().Height

	// Создаем placeholder для сохранения размера
	state.SavePlaceholder = canvas.NewRectangle(color.Transparent)
	state.SavePlaceholder.SetMinSize(fyne.NewSize(saveButtonWidth, saveButtonHeight))
	state.SavePlaceholder.Show()

	// Сохраняем ссылку на tabs в state
	state.tabs = tabs

	// Создаем контейнер со стеком для кнопки Save (placeholder, button, progress)
	saveButtonStack := container.NewStack(
		state.SavePlaceholder,
		state.SaveButton,
		state.SaveProgress,
	)

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
				saveButtonStack, // Используем стек с ProgressBar
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
			// Запускаем парсинг асинхронно (если нужно)
			go func() {
				state.triggerParseForPreview()
			}()
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

	// Превью генерируется только по кнопке "Show Preview"

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
	// Секция 1: VLESS Subscription URL or Direct Links
	state.CheckURLButton = widget.NewButton("Check", func() {
		if state.checkURLInProgress {
			return
		}
		go checkURL(state)
	})

	// Создаем прогресс-бар для кнопки Check
	state.CheckURLProgress = widget.NewProgressBar()
	state.CheckURLProgress.Hide()
	state.CheckURLProgress.SetValue(0)

	// Устанавливаем фиксированный размер через placeholder
	checkButtonWidth := float32(180)
	checkButtonHeight := state.CheckURLButton.MinSize().Height + 4 // Немного выше

	// Создаем placeholder для сохранения размера (всегда показываем, чтобы сохранить размер)
	state.CheckURLPlaceholder = canvas.NewRectangle(color.Transparent)
	state.CheckURLPlaceholder.SetMinSize(fyne.NewSize(checkButtonWidth, checkButtonHeight))
	state.CheckURLPlaceholder.Show() // Всегда показываем для сохранения размера

	// Создаем контейнер со стеком (placeholder, button, progress)
	checkURLStack := container.NewStack(
		state.CheckURLPlaceholder,
		state.CheckURLButton,
		state.CheckURLProgress,
	)

	// Добавляем отступ от края справа (10 единиц в Fyne)
	// Используем пустой Rectangle для создания отступа
	paddingRect := canvas.NewRectangle(color.Transparent)
	paddingRect.SetMinSize(fyne.NewSize(10, 0)) // Отступ 10px справа
	state.CheckURLContainer = container.NewHBox(
		checkURLStack, // Кнопка/прогресс
		paddingRect,   // Отступ справа
	)

	urlLabel := widget.NewLabel("VLESS Subscription URL or Direct Links:")
	urlLabel.Importance = widget.MediumImportance

	state.VLESSURLEntry = widget.NewMultiLineEntry()
	state.VLESSURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...\nhysteria2://...")
	state.VLESSURLEntry.Wrapping = fyne.TextWrapOff
	state.VLESSURLEntry.OnChanged = func(value string) {
		state.previewNeedsParse = true
		state.applyURLToParserConfig(strings.TrimSpace(value))
	}

	// Подсказка под полем ввода с кнопкой Check справа
	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://, hysteria2://).\nFor multiple links, use a new line for each.")
	hintLabel.Wrapping = fyne.TextWrapWord

	hintRow := container.NewBorder(
		nil,                     // top
		nil,                     // bottom
		nil,                     // left
		state.CheckURLContainer, // right - кнопка/прогресс
		hintLabel,               // center - подсказка займет всё доступное пространство
	)

	state.URLStatusLabel = widget.NewLabel("")
	state.URLStatusLabel.Wrapping = fyne.TextWrapWord

	// Ограничиваем ширину и высоту поля ввода URL (3 строки)
	// Обертываем MultiLineEntry в Scroll контейнер для показа скроллбаров
	urlEntryScroll := container.NewScroll(state.VLESSURLEntry)
	urlEntryScroll.Direction = container.ScrollBoth
	// Создаем фиктивный Rectangle для установки размера (высота 3 строки, ширина ограничена)
	urlEntrySizeRect := canvas.NewRectangle(color.Transparent)
	urlEntrySizeRect.SetMinSize(fyne.NewSize(0, 60)) // Ширина 900px, высота ~3 строки (примерно 20px на строку)
	// Обертываем в Max контейнер с Rectangle для фиксации размера
	// Scroll контейнер будет ограничен этим размером и покажет скроллбары, когда содержимое не помещается
	urlEntryWithSize := container.NewMax(
		urlEntrySizeRect,
		urlEntryScroll,
	)

	urlContainer := container.NewVBox(
		urlLabel,             // Заголовок
		urlEntryWithSize,     // Поле ввода с ограничением размера (3 строки)
		hintRow,              // Подсказка с кнопкой справа
		state.URLStatusLabel, // Статус
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
		state.refreshOutboundOptions()

		// Статус превью будет обновлен при переключении на вкладку Preview
	}

	// Ограничиваем ширину и высоту поля ParserConfig
	parserConfigScroll := container.NewScroll(state.ParserConfigEntry)
	parserConfigScroll.Direction = container.ScrollBoth
	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	parserHeightRect := canvas.NewRectangle(color.Transparent)
	parserHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк
	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	parserConfigWithHeight := container.NewMax(
		parserHeightRect,
		parserConfigScroll,
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

	// Ограничиваем ширину и высоту поля Preview
	previewScroll := container.NewScroll(state.OutboundsPreview)
	previewScroll.Direction = container.ScrollBoth
	// Создаем фиктивный Rectangle для установки высоты через container.NewMax
	previewHeightRect := canvas.NewRectangle(color.Transparent)
	previewHeightRect.SetMinSize(fyne.NewSize(0, 90)) // ~8-9 строк (уменьшено на ~30px)
	// Обертываем в Max контейнер с Rectangle для фиксации высоты
	previewWithHeight := container.NewMax(
		previewHeightRect,
		previewScroll,
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
	scrollContainer.SetMinSize(fyne.NewSize(0, 620))

	return scrollContainer
}

func createTemplateTab(state *WizardState) fyne.CanvasObject {
	if state.TemplateData == nil {
		templateFileName := GetTemplateFileName()
		return container.NewVBox(
			widget.NewLabel(fmt.Sprintf("Template file bin/%s not found.", templateFileName)),
			widget.NewLabel("Create the template file to enable this tab."),
		)
	}

	state.initializeTemplateState()

	availableOutbounds := state.getAvailableOutbounds()
	if len(availableOutbounds) == 0 {
		availableOutbounds = []string{defaultOutboundTag, rejectActionName}
	}

	// Устанавливаем флаг для блокировки callbacks при инициализации
	state.updatingOutboundOptions = true

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
					// Игнорируем callback при программном обновлении
					if state.updatingOutboundOptions {
						return
					}
					state.SelectableRuleStates[idx].SelectedOutbound = value
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
		// Игнорируем callback при программном обновлении
		if state.updatingOutboundOptions {
			return
		}
		state.SelectedFinalOutbound = value

		// Статус превью будет обновлен при переключении на вкладку Preview
	})
	finalSelect.SetSelected(state.SelectedFinalOutbound)
	state.FinalOutboundSelect = finalSelect

	rulesScroll := createRulesScroll(state, rulesBox)

	// Сбрасываем флаг перед refreshOutboundOptions, так как он сам установит его при необходимости
	state.updatingOutboundOptions = false
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
		// Read-only поле, ничего не делаем при изменении
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

	// Создаем статус-лейбл и кнопку для генерации превью
	state.TemplatePreviewStatusLabel = widget.NewLabel("Click 'Show Preview' to generate preview (this may take a long time for large configurations)")
	state.TemplatePreviewStatusLabel.Wrapping = fyne.TextWrapWord

	state.ShowPreviewButton = widget.NewButton("Show Preview", func() {
		if state.ShowPreviewButton != nil {
			state.ShowPreviewButton.Disable()
		}
		state.updateTemplatePreviewAsync()
	})

	// Контейнер со статусом (занимает всё доступное место) и кнопкой справа
	statusRow := container.NewBorder(
		nil, nil,
		nil,                              // left
		state.ShowPreviewButton,          // right - фиксированная ширина по содержимому
		state.TemplatePreviewStatusLabel, // center - занимает всё доступное пространство
	)

	return container.NewVBox(
		widget.NewLabel("Preview"),
		previewScroll,
		statusRow,
	)
}

func createRulesScroll(state *WizardState, content fyne.CanvasObject) fyne.CanvasObject {
	maxHeight := state.Window.Canvas().Size().Height * 0.65
	if maxHeight <= 0 {
		maxHeight = 430
	}
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(0, maxHeight))
	return scroll
}

// generateRandomSecret генерирует случайную строку для secret
func generateRandomSecret(length int) string {
	if length <= 0 {
		length = 24 // По умолчанию 24 символа
	}
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback на простую генерацию, если crypto/rand не работает
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	// Используем base64 URL-safe encoding, но убираем padding и ограничиваем длину
	secret := base64.URLEncoding.EncodeToString(bytes)
	// Убираем padding и ограничиваем длину
	secret = strings.TrimRight(secret, "=")
	if len(secret) > length {
		secret = secret[:length]
	}
	return secret
}

func (state *WizardState) saveConfigWithBackup(text string) (string, error) {
	// Validate JSON before saving (support JSONC with comments)
	jsonBytes := jsonc.ToJSON([]byte(text))
	var configJSON map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &configJSON); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	// Генерируем случайный secret
	randomSecret := generateRandomSecret(24)

	// Пытаемся заменить secret в оригинальном тексте, сохраняя комментарии
	// Ищем secret внутри clash_api блока
	finalText := text
	secretReplaced := false

	// Пробуем найти и заменить secret с помощью регулярного выражения
	simpleSecretPattern := regexp.MustCompile(`("secret"\s*:\s*)"[^"]*"`)
	if simpleSecretPattern.MatchString(text) && strings.Contains(text, "clash_api") {
		// Заменяем существующий secret (предполагаем, что он в clash_api)
		finalText = simpleSecretPattern.ReplaceAllString(text, fmt.Sprintf(`$1"%s"`, randomSecret))
		secretReplaced = true
	}

	if !secretReplaced {
		// Secret не найден, нужно добавить его через JSON парсинг
		if experimental, ok := configJSON["experimental"].(map[string]interface{}); ok {
			if clashAPI, ok := experimental["clash_api"].(map[string]interface{}); ok {
				clashAPI["secret"] = randomSecret
			} else {
				// Если clash_api не существует, создаем его
				experimental["clash_api"] = map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				}
			}
		} else {
			// Если experimental не существует, создаем его
			configJSON["experimental"] = map[string]interface{}{
				"clash_api": map[string]interface{}{
					"external_controller": "127.0.0.1:9090",
					"secret":              randomSecret,
				},
			}
		}

		// Сериализуем обратно в JSON с форматированием
		finalJSONBytes, err := json.MarshalIndent(configJSON, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal config: %w", err)
		}
		finalText = string(finalJSONBytes)
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
	if err := os.WriteFile(configPath, []byte(finalText), 0o644); err != nil {
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
	// Если есть шаблон с ParserConfig, используем его outbounds, но proxies берем из config.json
	if state.TemplateData != nil && state.TemplateData.ParserConfig != "" {
		infoLog("ConfigWizard: Using ParserConfig from template for outbounds")
		// Парсим ParserConfig из шаблона
		var templateParserConfig core.ParserConfig
		if err := json.Unmarshal([]byte(state.TemplateData.ParserConfig), &templateParserConfig); err != nil {
			errorLog("ConfigWizard: Failed to parse ParserConfig from template: %v", err)
			// Fallback to config.json if template parsing fails
		} else {
			// Проверяем наличие config.json для получения proxies
			var configParserConfig *core.ParserConfig
			if _, err := os.Stat(state.Controller.ConfigPath); err == nil {
				// config.json существует - берем proxies оттуда
				configParserConfig, err = core.ExtractParserConfig(state.Controller.ConfigPath)
				if err != nil {
					infoLog("ConfigWizard: Failed to extract ParserConfig from config.json, using template proxies: %v", err)
				}
			}

			// Объединяем: outbounds из шаблона, proxies из config.json (если есть)
			proxiesFromConfig := false
			if configParserConfig != nil && len(configParserConfig.ParserConfig.Proxies) > 0 {
				// Используем proxies из config.json
				templateParserConfig.ParserConfig.Proxies = configParserConfig.ParserConfig.Proxies
				proxiesFromConfig = true
				infoLog("ConfigWizard: Using proxies from config.json, outbounds from template")
			} else {
				infoLog("ConfigWizard: Using proxies from template (config.json not found or empty)")
			}

			state.ParserConfig = &templateParserConfig

			// Заполняем поле URL только если proxies из реального config.json, а не из шаблона
			if proxiesFromConfig && len(templateParserConfig.ParserConfig.Proxies) > 0 {
				lines := make([]string, 0)
				for _, proxySource := range templateParserConfig.ParserConfig.Proxies {
					if proxySource.Source != "" {
						lines = append(lines, proxySource.Source)
					}
					lines = append(lines, proxySource.Connections...)
				}
				state.VLESSURLEntry.SetText(strings.Join(lines, "\n"))
			}

			parserConfigJSON, err := serializeParserConfig(&templateParserConfig)
			if err != nil {
				errorLog("ConfigWizard: Failed to serialize ParserConfig: %v", err)
				return false, err
			}

			state.parserConfigUpdating = true
			state.ParserConfigEntry.SetText(string(parserConfigJSON))
			state.parserConfigUpdating = false
			return true, nil
		}
	}

	// Проверяем наличие config.json
	if _, err := os.Stat(state.Controller.ConfigPath); os.IsNotExist(err) {
		// Конфиг не существует - оставляем значения по умолчанию
		infoLog("ConfigWizard: config.json not found, using default values")
		return false, nil
	}

	// Извлекаем ParserConfig из config.json (fallback)
	parserConfig, err := core.ExtractParserConfig(state.Controller.ConfigPath)
	if err != nil {
		// Если не удалось извлечь - оставляем значения по умолчанию
		errorLog("ConfigWizard: Failed to extract ParserConfig: %v", err)
		return false, nil // Не критическая ошибка
	}

	state.ParserConfig = parserConfig

	// Заполняем поле ParserConfig ПЕРВЫМ, чтобы applyURLToParserConfig мог его прочитать
	parserConfigJSON, err := serializeParserConfig(parserConfig)
	if err != nil {
		errorLog("ConfigWizard: Failed to serialize ParserConfig: %v", err)
		return false, err
	}

	state.parserConfigUpdating = true
	state.ParserConfigEntry.SetText(string(parserConfigJSON))
	state.parserConfigUpdating = false

	// Теперь заполняем поле URL - это вызовет applyURLToParserConfig, который прочитает уже заполненный ParserConfigEntry
	// Вставляем все URL из реального config.json (не из шаблона)
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		lines := make([]string, 0)
		for _, proxySource := range parserConfig.ParserConfig.Proxies {
			if proxySource.Source != "" {
				lines = append(lines, proxySource.Source)
			}
			lines = append(lines, proxySource.Connections...)
		}
		state.VLESSURLEntry.SetText(strings.Join(lines, "\n"))
	}

	state.previewNeedsParse = true

	infoLog("ConfigWizard: Successfully loaded config from file")
	return true, nil
}

// setCheckURLState управляет состоянием кнопки Check и прогресс-бара
func (state *WizardState) setCheckURLState(statusText string, buttonText string, progress float64) {
	if statusText != "" && state.URLStatusLabel != nil {
		state.URLStatusLabel.SetText(statusText)
	}

	progressVisible := false
	if progress < 0 {
		// Скрыть прогресс
		if state.CheckURLProgress != nil {
			state.CheckURLProgress.Hide()
			state.CheckURLProgress.SetValue(0)
		}
	} else {
		// Показать прогресс
		if state.CheckURLProgress != nil {
			state.CheckURLProgress.SetValue(progress)
			state.CheckURLProgress.Show()
		}
		progressVisible = true
	}

	buttonVisible := false
	if progressVisible {
		// Если показываем прогресс, кнопка скрыта
		if state.CheckURLButton != nil {
			state.CheckURLButton.Hide()
		}
	} else if buttonText == "" {
		// Скрыть кнопку
		if state.CheckURLButton != nil {
			state.CheckURLButton.Hide()
		}
	} else {
		// Показать кнопку
		if state.CheckURLButton != nil {
			state.CheckURLButton.SetText(buttonText)
			state.CheckURLButton.Show()
			state.CheckURLButton.Enable()
		}
		buttonVisible = true
	}

	// Управление placeholder
	if state.CheckURLPlaceholder != nil {
		if buttonVisible || progressVisible {
			state.CheckURLPlaceholder.Show()
		} else {
			state.CheckURLPlaceholder.Hide()
		}
	}
}

// setSaveState управляет состоянием кнопки Save и прогресс-бара
func (state *WizardState) setSaveState(buttonText string, progress float64) {
	progressVisible := false
	if progress < 0 {
		// Скрыть прогресс
		if state.SaveProgress != nil {
			state.SaveProgress.Hide()
			state.SaveProgress.SetValue(0)
		}
		state.saveInProgress = false
	} else {
		// Показать прогресс
		if state.SaveProgress != nil {
			state.SaveProgress.SetValue(progress)
			state.SaveProgress.Show()
		}
		progressVisible = true
		state.saveInProgress = true
	}

	buttonVisible := false
	if progressVisible {
		// Если показываем прогресс, кнопка скрыта
		if state.SaveButton != nil {
			state.SaveButton.Hide()
			state.SaveButton.Disable()
		}
	} else if buttonText == "" {
		// Скрыть кнопку
		if state.SaveButton != nil {
			state.SaveButton.Hide()
			state.SaveButton.Disable()
		}
	} else {
		// Показать кнопку
		if state.SaveButton != nil {
			state.SaveButton.SetText(buttonText)
			state.SaveButton.Show()
			state.SaveButton.Enable()
		}
		buttonVisible = true
	}

	// Управление placeholder
	if state.SavePlaceholder != nil {
		if buttonVisible || progressVisible {
			state.SavePlaceholder.Show()
		} else {
			state.SavePlaceholder.Hide()
		}
	}
}

// checkURL проверяет доступность URL подписки или валидность прямых ссылок
func checkURL(state *WizardState) {
	startTime := time.Now()
	debugLog("checkURL: START at %s", startTime.Format("15:04:05.000"))

	input := strings.TrimSpace(state.VLESSURLEntry.Text)
	if input == "" {
		debugLog("checkURL: Empty input, returning early")
		safeFyneDo(state.Window, func() {
			state.URLStatusLabel.SetText("❌ Please enter a URL or direct link")
			state.setCheckURLState("", "Check", -1)
		})
		return
	}

	state.checkURLInProgress = true
	safeFyneDo(state.Window, func() {
		state.URLStatusLabel.SetText("⏳ Checking...")
		state.setCheckURLState("", "", 0.0)
	})

	// Разбиваем на строки для обработки
	inputLines := strings.Split(input, "\n")
	debugLog("checkURL: Processing %d input lines", len(inputLines))
	totalValid := 0
	previewLines := make([]string, 0)
	errors := make([]string, 0)

	for i, line := range inputLines {
		lineStartTime := time.Now()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		debugLog("checkURL: Processing line %d/%d: %s (elapsed: %v)", i+1, len(inputLines),
			func() string {
				if len(line) > 50 {
					return line[:50] + "..."
				}
				return line
			}(), time.Since(startTime))

		safeFyneDo(state.Window, func() {
			progress := float64(i+1) / float64(len(inputLines))
			state.setCheckURLState(fmt.Sprintf("⏳ Checking... (%d/%d)", i+1, len(inputLines)), "", progress)
		})

		if core.IsSubscriptionURL(line) {
			// Это URL подписки - проверяем доступность
			fetchStartTime := time.Now()
			debugLog("checkURL: Fetching subscription %d/%d: %s", i+1, len(inputLines), line)
			content, err := core.FetchSubscription(line)
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				debugLog("checkURL: Failed to fetch subscription %d/%d (took %v): %v", i+1, len(inputLines), fetchDuration, err)
				errors = append(errors, fmt.Sprintf("Failed to fetch %s: %v", line, err))
				continue
			}
			debugLog("checkURL: Fetched subscription %d/%d: %d bytes in %v", i+1, len(inputLines), len(content), fetchDuration)

			// Проверяем содержимое подписки
			parseStartTime := time.Now()
			subLines := strings.Split(string(content), "\n")
			debugLog("checkURL: Parsing subscription %d/%d: %d lines", i+1, len(inputLines), len(subLines))
			validInSub := 0
			for _, subLine := range subLines {
				subLine = strings.TrimSpace(subLine)
				if subLine != "" && parsers.IsDirectLink(subLine) {
					validInSub++
					totalValid++
					if len(previewLines) < 10 { // Ограничиваем превью
						previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, subLine))
					}
				}
			}
			parseDuration := time.Since(parseStartTime)
			debugLog("checkURL: Parsed subscription %d/%d: %d valid links in %v (line processing took %v total)",
				i+1, len(inputLines), validInSub, parseDuration, time.Since(lineStartTime))
			if validInSub == 0 {
				errors = append(errors, fmt.Sprintf("Subscription %s contains no valid proxy links", line))
			}
		} else if parsers.IsDirectLink(line) {
			// Это прямая ссылка - проверяем парсинг
			parseStartTime := time.Now()
			debugLog("checkURL: Parsing direct link %d/%d", i+1, len(inputLines))
			_, err := parsers.ParseNode(line, nil)
			parseDuration := time.Since(parseStartTime)
			if err != nil {
				debugLog("checkURL: Invalid direct link %d/%d (took %v): %v", i+1, len(inputLines), parseDuration, err)
				errors = append(errors, fmt.Sprintf("Invalid direct link: %v", err))
			} else {
				totalValid++
				debugLog("checkURL: Valid direct link %d/%d (took %v)", i+1, len(inputLines), parseDuration)
				if len(previewLines) < 10 {
					previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, line))
				}
			}
		} else {
			debugLog("checkURL: Unknown format for line %d/%d: %s", i+1, len(inputLines), line)
			errors = append(errors, fmt.Sprintf("Unknown format: %s", line))
		}
	}

	state.checkURLInProgress = false
	totalDuration := time.Since(startTime)
	debugLog("checkURL: Processed all lines in %v (total valid: %d, errors: %d)",
		totalDuration, totalValid, len(errors))

	safeFyneDo(state.Window, func() {
		if totalValid == 0 {
			errorMsg := "❌ No valid proxy links found"
			if len(errors) > 0 {
				errorMsg += "\n" + strings.Join(errors[:min(3, len(errors))], "\n")
			}
			state.URLStatusLabel.SetText(errorMsg)
		} else {
			statusMsg := fmt.Sprintf("✅ Working! Found %d valid proxy link(s)", totalValid)
			if len(errors) > 0 {
				statusMsg += fmt.Sprintf("\n⚠️ %d error(s)", len(errors))
			}
			state.URLStatusLabel.SetText(statusMsg)
			if len(previewLines) > 0 {
				previewText := strings.Join(previewLines, "\n")
				if totalValid > len(previewLines) {
					previewText += fmt.Sprintf("\n... and %d more", totalValid-len(previewLines))
				}
				setPreviewText(state, previewText)
			}
		}
		state.setCheckURLState("", "Check", -1)
	})
	debugLog("checkURL: END (total duration: %v)", totalDuration)
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseAndPreview парсит ParserConfig и генерирует предпросмотр outbounds
func parseAndPreview(state *WizardState) {
	startTime := time.Now()
	debugLog("parseAndPreview: START at %s", startTime.Format("15:04:05.000"))

	defer func() {
		totalDuration := time.Since(startTime)
		debugLog("parseAndPreview: END (total duration: %v)", totalDuration)
		safeFyneDo(state.Window, func() {
			state.autoParseInProgress = false
		})
	}()
	safeFyneDo(state.Window, func() {
		state.ParseButton.Disable()
		state.ParseButton.SetText("Parsing...")
		setPreviewText(state, "Parsing configuration...")
	})

	// Парсим ParserConfig из поля
	parseStartTime := time.Now()
	parserConfigJSON := strings.TrimSpace(state.ParserConfigEntry.Text)
	debugLog("parseAndPreview: ParserConfig text length: %d bytes", len(parserConfigJSON))
	if parserConfigJSON == "" {
		debugLog("parseAndPreview: ParserConfig is empty, returning early")
		safeFyneDo(state.Window, func() {
			setPreviewText(state, "Error: ParserConfig is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		debugLog("parseAndPreview: Failed to parse ParserConfig JSON (took %v): %v", time.Since(parseStartTime), err)
		safeFyneDo(state.Window, func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to parse ParserConfig JSON: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}
	debugLog("parseAndPreview: Parsed ParserConfig in %v (sources: %d, outbounds: %d)",
		time.Since(parseStartTime), len(parserConfig.ParserConfig.Proxies), len(parserConfig.ParserConfig.Outbounds))

	// Проверяем наличие URL или прямых ссылок
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	debugLog("parseAndPreview: URL text length: %d bytes", len(url))
	if url == "" {
		debugLog("parseAndPreview: URL is empty, returning early")
		safeFyneDo(state.Window, func() {
			setPreviewText(state, "Error: VLESS URL or direct links are empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Обновляем конфиг через applyURLToParserConfig, который правильно разделяет подписки и connections
	applyStartTime := time.Now()
	debugLog("parseAndPreview: Applying URL to ParserConfig")
	state.applyURLToParserConfig(url)
	debugLog("parseAndPreview: Applied URL to ParserConfig in %v", time.Since(applyStartTime))

	// Перезагружаем parserConfig после обновления
	reloadStartTime := time.Now()
	parserConfigJSON = strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON != "" {
		if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
			debugLog("parseAndPreview: Failed to parse updated ParserConfig JSON (took %v): %v", time.Since(reloadStartTime), err)
			safeFyneDo(state.Window, func() {
				setPreviewText(state, fmt.Sprintf("Error: Failed to parse updated ParserConfig JSON: %v", err))
				state.ParseButton.Enable()
				state.ParseButton.SetText("Parse")
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
			})
			return
		}
		debugLog("parseAndPreview: Reloaded ParserConfig in %v (sources: %d)",
			time.Since(reloadStartTime), len(parserConfig.ParserConfig.Proxies))
	}

	// Генерируем все outbounds используя unified функцию
	// Это устраняет дублирование кода и добавляет поддержку локальных outbounds
	generateStartTime := time.Now()
	debugLog("parseAndPreview: Starting outbound generation using unified function")

	// Map to track unique tags and their counts
	tagCounts := make(map[string]int)
	debugLog("parseAndPreview: Initializing tag deduplication tracker")

	// Progress callback для обновления UI с throttling (не чаще чем раз в 200ms)
	var lastProgressUpdate time.Time
	progressCallback := func(p float64, s string) {
		now := time.Now()
		if now.Sub(lastProgressUpdate) < 200*time.Millisecond {
			return // Skip update if less than 200ms passed
		}
		lastProgressUpdate = now
		safeFyneDo(state.Window, func() {
			setPreviewText(state, s)
		})
	}

	// Используем unified функцию для генерации всех outbounds
	result, err := state.Controller.ConfigService.GenerateOutboundsFromParserConfig(
		&parserConfig, tagCounts, progressCallback)
	if err != nil {
		debugLog("parseAndPreview: Failed to generate outbounds (took %v): %v", time.Since(generateStartTime), err)
		safeFyneDo(state.Window, func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to generate outbounds: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Log statistics about duplicates
	core.LogDuplicateTagStatistics(tagCounts, "ConfigWizard")

	// Сохраняем статистику для использования в buildParserOutboundsBlock
	state.OutboundStats.NodesCount = result.NodesCount
	state.OutboundStats.LocalSelectorsCount = result.LocalSelectorsCount
	state.OutboundStats.GlobalSelectorsCount = result.GlobalSelectorsCount
	state.GeneratedOutbounds = result.OutboundsJSON // Store full JSON for later use (e.g., saving)

	// Формируем итоговый текст для предпросмотра
	// Если узлов больше maxNodesForFullPreview, показываем статистику в виде комментария
	var previewText string
	if result.NodesCount > maxNodesForFullPreview {
		// Формируем статистику в виде комментария
		joinStartTime := time.Now()
		statsComment := fmt.Sprintf(`/** @ParserSTART */
	// Generated: %d nodes, %d local selectors, %d global selectors
	// Total outbounds: %d
/** @ParserEND */`,
			result.NodesCount,
			result.LocalSelectorsCount,
			result.GlobalSelectorsCount,
			len(result.OutboundsJSON))
		previewText = statsComment
		debugLog("parseAndPreview: Generated statistics comment in %v (nodes: %d > %d)", time.Since(joinStartTime), result.NodesCount, maxNodesForFullPreview)
	} else {
		// Показываем полный текст для малого количества узлов
		joinStartTime := time.Now()
		previewText = strings.Join(result.OutboundsJSON, "\n")
		debugLog("parseAndPreview: Joined %d JSON strings in %v (total preview text length: %d bytes)",
			len(result.OutboundsJSON), time.Since(joinStartTime), len(previewText))
	}
	debugLog("parseAndPreview: Total outbound generation took %v", time.Since(generateStartTime))

	safeFyneDo(state.Window, func() {
		uiUpdateStartTime := time.Now()
		setPreviewText(state, previewText)
		state.ParseButton.Enable()
		state.ParseButton.SetText("Parse")
		state.ParserConfig = &parserConfig
		state.previewNeedsParse = false
		state.refreshOutboundOptions()
		debugLog("parseAndPreview: UI update took %v", time.Since(uiUpdateStartTime))
		// Включаем кнопку Save после успешного парсинга (независимо от превью)
		if state.SaveButton != nil {
			state.SaveButton.Enable()
		}
		// Автоматически генерируем превью на 3-й вкладке после успешного парсинга
		if state.TemplateData != nil && len(state.GeneratedOutbounds) > 0 {
			go state.updateTemplatePreviewAsync()
		}
	})
}

func setPreviewText(state *WizardState, text string) {
	state.OutboundsPreviewText = text
	if state.OutboundsPreview != nil {
		// Безопасный вызов SetText - функция уже вызывается из safeFyneDo в большинстве случаев,
		// но для надежности оборачиваем в safeFyneDo
		safeFyneDo(state.Window, func() {
			state.OutboundsPreview.SetText(text)
		})
	}
}

func (state *WizardState) applyURLToParserConfig(input string) {
	startTime := time.Now()
	debugLog("applyURLToParserConfig: START at %s (input length: %d bytes)",
		startTime.Format("15:04:05.000"), len(input))

	if state.ParserConfigEntry == nil || input == "" {
		debugLog("applyURLToParserConfig: ParserConfigEntry is nil or input is empty, returning early")
		return
	}
	text := strings.TrimSpace(state.ParserConfigEntry.Text)
	if text == "" {
		debugLog("applyURLToParserConfig: ParserConfigEntry text is empty, returning early")
		return
	}

	parseStartTime := time.Now()
	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(text), &parserConfig); err != nil {
		debugLog("applyURLToParserConfig: Failed to parse ParserConfig (took %v): %v",
			time.Since(parseStartTime), err)
		return
	}
	debugLog("applyURLToParserConfig: Parsed ParserConfig in %v (outbounds: %d)",
		time.Since(parseStartTime), len(parserConfig.ParserConfig.Outbounds))

	// Разделяем подписки и прямые ссылки
	splitStartTime := time.Now()
	lines := strings.Split(input, "\n")
	debugLog("applyURLToParserConfig: Split input into %d lines", len(lines))
	subscriptions := make([]string, 0)
	connections := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if core.IsSubscriptionURL(line) {
			subscriptions = append(subscriptions, line)
		} else if parsers.IsDirectLink(line) {
			connections = append(connections, line)
		}
	}
	debugLog("applyURLToParserConfig: Classified lines: %d subscriptions, %d connections (took %v)",
		len(subscriptions), len(connections), time.Since(splitStartTime))

	// Сохраняем существующие локальные outbounds, tag_prefix и tag_postfix для каждого источника
	// Используем source URL как ключ для сопоставления
	existingOutboundsMap := make(map[string][]core.OutboundConfig)
	existingTagPrefixMap := make(map[string]string)
	existingTagPostfixMap := make(map[string]string)
	for i, existingProxy := range parserConfig.ParserConfig.Proxies {
		if existingProxy.Source != "" {
			existingOutboundsMap[existingProxy.Source] = existingProxy.Outbounds
			if existingProxy.TagPrefix != "" {
				existingTagPrefixMap[existingProxy.Source] = existingProxy.TagPrefix
			}
			if existingProxy.TagPostfix != "" {
				existingTagPostfixMap[existingProxy.Source] = existingProxy.TagPostfix
			}
		} else if i == 0 && len(existingProxy.Outbounds) > 0 {
			// Если первый proxy не имел source, но имел outbounds, сохраняем их
			// Это может быть случай, когда был только connections без source
			existingOutboundsMap[""] = existingProxy.Outbounds
			if existingProxy.TagPrefix != "" {
				existingTagPrefixMap[""] = existingProxy.TagPrefix
			}
			if existingProxy.TagPostfix != "" {
				existingTagPostfixMap[""] = existingProxy.TagPostfix
			}
		}
	}

	// Создаем новый массив ProxySource
	newProxies := make([]core.ProxySource, 0)

	// Автоматически добавляем tag_prefix с порядковым номером только если подписок несколько
	autoAddPrefix := len(subscriptions) > 1

	// Создаем отдельный ProxySource для каждой подписки
	for idx, sub := range subscriptions {
		proxySource := core.ProxySource{
			Source: sub,
		}
		// Восстанавливаем локальные outbounds, если они были для этого источника
		if existingOutbounds, ok := existingOutboundsMap[sub]; ok {
			proxySource.Outbounds = existingOutbounds
			debugLog("applyURLToParserConfig: Restored %d local outbounds for subscription: %s", len(existingOutbounds), sub)
		}
		// Восстанавливаем tag_prefix, если он был установлен для этого источника
		if existingTagPrefix, ok := existingTagPrefixMap[sub]; ok {
			proxySource.TagPrefix = existingTagPrefix
			debugLog("applyURLToParserConfig: Restored tag_prefix '%s' for subscription: %s", existingTagPrefix, sub)
		} else if autoAddPrefix {
			// Автоматически добавляем tag_prefix с порядковым номером для новых подписок (только если подписок несколько)
			proxySource.TagPrefix = generateTagPrefix(idx + 1)
			debugLog("applyURLToParserConfig: Added automatic tag_prefix '%s' for subscription: %s", proxySource.TagPrefix, sub)
		}
		// Восстанавливаем tag_postfix, если он был установлен для этого источника
		if existingTagPostfix, ok := existingTagPostfixMap[sub]; ok {
			proxySource.TagPostfix = existingTagPostfix
			debugLog("applyURLToParserConfig: Restored tag_postfix '%s' for subscription: %s", existingTagPostfix, sub)
		}
		newProxies = append(newProxies, proxySource)
	}

	// Если есть прямые ссылки, создаем отдельный ProxySource для них
	if len(connections) > 0 {
		proxySource := core.ProxySource{
			Connections: connections,
		}
		// Если был первый proxy без source, но с outbounds, восстанавливаем их
		if existingOutbounds, ok := existingOutboundsMap[""]; ok && len(newProxies) == 0 {
			// Если не было подписок, но были connections с outbounds
			proxySource.Outbounds = existingOutbounds
			debugLog("applyURLToParserConfig: Restored %d local outbounds for connections", len(existingOutbounds))
		}
		newProxies = append(newProxies, proxySource)
	}

	// Если нет ни подписок, ни connections, создаем пустой массив
	if len(newProxies) == 0 {
		newProxies = []core.ProxySource{{}}
	}

	// Обновляем массив proxies
	parserConfig.ParserConfig.Proxies = newProxies
	debugLog("applyURLToParserConfig: Created %d proxy sources (%d subscriptions, %d with connections)",
		len(newProxies), len(subscriptions), len(connections))

	serializeStartTime := time.Now()
	serialized, err := serializeParserConfig(&parserConfig)
	if err != nil {
		debugLog("applyURLToParserConfig: Failed to serialize ParserConfig (took %v): %v",
			time.Since(serializeStartTime), err)
		return
	}
	debugLog("applyURLToParserConfig: Serialized ParserConfig in %v (result length: %d bytes, outbounds before: %d)",
		time.Since(serializeStartTime), len(serialized), len(parserConfig.ParserConfig.Outbounds))

	// Обновляем UI безопасно из любого потока
	safeFyneDo(state.Window, func() {
		state.parserConfigUpdating = true
		state.ParserConfigEntry.SetText(serialized)
		state.parserConfigUpdating = false
	})
	state.ParserConfig = &parserConfig
	state.previewNeedsParse = true
	debugLog("applyURLToParserConfig: END (total duration: %v)", time.Since(startTime))
}

func (state *WizardState) setTemplatePreviewText(text string) {
	// Оптимизация: не обновляем, если текст не изменился
	if state.TemplatePreviewText == text {
		return
	}

	state.TemplatePreviewText = text
	if state.TemplatePreviewEntry == nil {
		return
	}

	// Проверяем, изменился ли текст в Entry
	if state.TemplatePreviewEntry.Text == text {
		return
	}

	debugLog("setTemplatePreviewText: Setting preview text (length: %d bytes)", len(text))

	// Для больших текстов (>50KB) показываем сообщение о загрузке перед вставкой
	if len(text) > 50000 {
		safeFyneDo(state.Window, func() {
			state.TemplatePreviewEntry.SetText("Loading large preview...")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("⏳ Loading large preview...")
			}
		})

		// Вставляем большой текст асинхронно
		go func() {
			safeFyneDo(state.Window, func() {
				insertStartTime := time.Now()
				state.TemplatePreviewEntry.SetText(text)
				debugLog("setTemplatePreviewText: Large text inserted in %v", time.Since(insertStartTime))

			})
		}()
	} else {
		// Для обычных текстов используем синхронную вставку
		safeFyneDo(state.Window, func() {
			state.TemplatePreviewEntry.SetText(text)
		})
	}
}

func (state *WizardState) refreshOutboundOptions() {
	startTime := time.Now()
	debugLog("refreshOutboundOptions: START at %s", startTime.Format("15:04:05.000"))

	if len(state.SelectableRuleStates) == 0 && state.FinalOutboundSelect == nil {
		debugLog("refreshOutboundOptions: No rule states and no final select, returning early")
		return
	}

	getOptionsStartTime := time.Now()
	options := state.getAvailableOutbounds()
	debugLog("refreshOutboundOptions: getAvailableOutbounds took %v (found %d options)",
		time.Since(getOptionsStartTime), len(options))
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

	// Устанавливаем флаг, чтобы блокировать callbacks при программном обновлении
	state.updatingOutboundOptions = true
	defer func() {
		state.updatingOutboundOptions = false
	}()

	uiUpdateStartTime := time.Now()
	safeFyneDo(state.Window, func() {
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
	debugLog("refreshOutboundOptions: UI update took %v", time.Since(uiUpdateStartTime))
	debugLog("refreshOutboundOptions: END (total duration: %v)", time.Since(startTime))
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
	// Обновляем статус и отключаем кнопку Save при начале парсинга
	safeFyneDo(state.Window, func() {
		if state.SaveButton != nil {
			state.SaveButton.Disable()
		}
		// Показываем статус парсинга на 3-й вкладке
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Parsing subscriptions and generating outbounds...")
		}
		if state.TemplatePreviewEntry != nil {
			state.setTemplatePreviewText("Parsing configuration... Please wait.")
		}
	})
	go parseAndPreview(state)
}

func (state *WizardState) updateTemplatePreviewAsync() {
	startTime := time.Now()
	debugLog("updateTemplatePreviewAsync: START at %s", startTime.Format("15:04:05.000"))

	if state.TemplateData == nil || state.TemplatePreviewEntry == nil {
		debugLog("updateTemplatePreviewAsync: TemplateData or TemplatePreviewEntry is nil, returning early")
		return
	}

	// Устанавливаем флаг генерации и отключаем кнопку Save
	state.previewGenerationInProgress = true
	safeFyneDo(state.Window, func() {
		if state.TemplatePreviewEntry != nil {
			state.setTemplatePreviewText("Building preview...")
		}
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Building preview configuration...")
		}
		// Отключаем кнопку Save во время генерации
		if state.SaveButton != nil {
			state.SaveButton.Disable()
		}
	})

	// Строим конфиг асинхронно
	go func() {
		goroutineStartTime := time.Now()
		debugLog("updateTemplatePreviewAsync: Goroutine START at %s", goroutineStartTime.Format("15:04:05.000"))

		defer func() {
			totalDuration := time.Since(goroutineStartTime)
			debugLog("updateTemplatePreviewAsync: Goroutine END (duration: %v)", totalDuration)
			state.previewGenerationInProgress = false
			safeFyneDo(state.Window, func() {
				// Включаем кнопку Save после завершения
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
				// Включаем кнопку Show Preview
				if state.ShowPreviewButton != nil {
					state.ShowPreviewButton.Enable()
				}
			})
		}()

		// Обновляем статус: парсинг ParserConfig
		safeFyneDo(state.Window, func() {
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("⏳ Parsing ParserConfig...")
			}
		})

		buildStartTime := time.Now()
		debugLog("updateTemplatePreviewAsync: Calling buildTemplateConfig")
		text, err := buildTemplateConfig(state, true)
		buildDuration := time.Since(buildStartTime)
		if err != nil {
			debugLog("updateTemplatePreviewAsync: buildTemplateConfig failed (took %v): %v", buildDuration, err)
			safeFyneDo(state.Window, func() {
				state.setTemplatePreviewText(fmt.Sprintf("Preview error: %v", err))
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: %v", err))
				}
			})
			return
		}
		debugLog("updateTemplatePreviewAsync: buildTemplateConfig completed in %v (result size: %d bytes)",
			buildDuration, len(text))

		// Обновляем текст превью
		// Для больших текстов setTemplatePreviewText сам обновит статус после завершения
		isLargeText := len(text) > 50000
		safeFyneDo(state.Window, func() {
			state.setTemplatePreviewText(text)

			// Обновляем статус только для небольших текстов
			// Для больших текстов статус обновится после завершения асинхронной вставки
			if !isLargeText {
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText("✅ Preview ready")
				}
				if state.ShowPreviewButton != nil {
					state.ShowPreviewButton.Enable()
				}
			}
		})
		if !isLargeText {
			debugLog("updateTemplatePreviewAsync: Preview text inserted")
		} else {
			debugLog("updateTemplatePreviewAsync: Large text insertion started (status will update when complete)")
		}
	}()
}

func buildTemplateConfig(state *WizardState, forPreview bool) (string, error) {
	startTime := time.Now()
	debugLog("buildTemplateConfig: START at %s", startTime.Format("15:04:05.000"))

	if state.TemplateData == nil {
		debugLog("buildTemplateConfig: TemplateData is nil, returning error")
		return "", fmt.Errorf("template data not available")
	}
	parserConfigText := strings.TrimSpace(state.ParserConfigEntry.Text)
	debugLog("buildTemplateConfig: ParserConfig text length: %d bytes", len(parserConfigText))
	if parserConfigText == "" {
		debugLog("buildTemplateConfig: ParserConfig is empty, returning error")
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	// Parse ParserConfig JSON to ensure it has version 2 and parser object
	parseStartTime := time.Now()
	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigText), &parserConfig); err != nil {
		// If parsing fails, use text as-is (might be invalid JSON, but let user fix it)
		debugLog("buildTemplateConfig: Failed to parse ParserConfig JSON (took %v): %v", time.Since(parseStartTime), err)
	} else {
		// Normalize ParserConfig (migrate version, set defaults, update last_updated)
		normalizeStartTime := time.Now()
		core.NormalizeParserConfig(&parserConfig, true)
		debugLog("buildTemplateConfig: Normalized ParserConfig in %v", time.Since(normalizeStartTime))

		// Serialize back to JSON with proper formatting (always version 2 format)
		serializeStartTime := time.Now()
		configToSerialize := map[string]interface{}{
			"ParserConfig": parserConfig.ParserConfig,
		}
		serialized, err := json.MarshalIndent(configToSerialize, "", "  ")
		if err == nil {
			parserConfigText = string(serialized)
			debugLog("buildTemplateConfig: Serialized ParserConfig in %v (new length: %d bytes)",
				time.Since(serializeStartTime), len(parserConfigText))
		} else {
			debugLog("buildTemplateConfig: Failed to serialize ParserConfig (took %v): %v",
				time.Since(serializeStartTime), err)
		}
	}
	debugLog("buildTemplateConfig: ParserConfig processing took %v total", time.Since(parseStartTime))

	sectionsStartTime := time.Now()
	sections := make([]string, 0)
	sectionCount := 0
	debugLog("buildTemplateConfig: Processing %d sections", len(state.TemplateData.SectionOrder))
	for _, key := range state.TemplateData.SectionOrder {
		sectionStartTime := time.Now()
		if selected, ok := state.TemplateSectionSelections[key]; !ok || !selected {
			debugLog("buildTemplateConfig: Section '%s' not selected, skipping", key)
			continue
		}
		raw := state.TemplateData.Sections[key]
		var formatted string
		var err error
		if key == "outbounds" && state.TemplateData.HasParserOutboundsBlock {
			// If template had @PARSER_OUTBOUNDS_BLOCK marker, replace entire outbounds array
			// with generated content
			outboundsStartTime := time.Now()
			debugLog("buildTemplateConfig: Building outbounds block (generated outbounds: %d)",
				len(state.GeneratedOutbounds))
			content := state.buildParserOutboundsBlock(forPreview)
			debugLog("buildTemplateConfig: Built outbounds block in %v (content length: %d bytes)",
				time.Since(outboundsStartTime), len(content))

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
			routeStartTime := time.Now()
			debugLog("buildTemplateConfig: Merging route section (rules: %d)",
				len(state.SelectableRuleStates))
			merged, err := mergeRouteSection(raw, state.SelectableRuleStates, state.SelectedFinalOutbound)
			if err != nil {
				debugLog("buildTemplateConfig: Route merge failed (took %v): %v",
					time.Since(routeStartTime), err)
				return "", fmt.Errorf("route merge failed: %w", err)
			}
			raw = merged
			formatStartTime := time.Now()
			formatted, err = formatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			debugLog("buildTemplateConfig: Formatted route section in %v (total route processing: %v)",
				time.Since(formatStartTime), time.Since(routeStartTime))
		} else {
			formatStartTime := time.Now()
			formatted, err = formatSectionJSON(raw, 2)
			if err != nil {
				formatted = string(raw)
			}
			debugLog("buildTemplateConfig: Formatted section '%s' in %v", key, time.Since(formatStartTime))
		}
		sections = append(sections, fmt.Sprintf(`  "%s": %s`, key, formatted))
		sectionCount++
		debugLog("buildTemplateConfig: Processed section '%s' in %v (total sections processed: %d)",
			key, time.Since(sectionStartTime), sectionCount)
	}
	debugLog("buildTemplateConfig: Processed all sections in %v (total: %d)",
		time.Since(sectionsStartTime), sectionCount)

	if len(sections) == 0 {
		debugLog("buildTemplateConfig: No sections selected, returning error")
		return "", fmt.Errorf("no sections selected")
	}

	buildStartTime := time.Now()
	var builder strings.Builder
	builder.WriteString("{\n")
	builder.WriteString("/** @ParserConfig\n")
	builder.WriteString(parserConfigText)
	builder.WriteString("\n*/\n")
	builder.WriteString(strings.Join(sections, ",\n"))
	builder.WriteString("\n}\n")
	result := builder.String()
	debugLog("buildTemplateConfig: Built final config in %v (result length: %d bytes)",
		time.Since(buildStartTime), len(result))
	debugLog("buildTemplateConfig: END (total duration: %v)", time.Since(startTime))
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

		// Handle reject and drop selections
		if outbound == rejectActionName {
			// User selected reject - set action: reject without method, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = rejectActionName
			delete(cloned, "method")
		} else if outbound == "drop" {
			// User selected drop - set action: reject with method: drop, remove outbound
			delete(cloned, "outbound")
			cloned["action"] = rejectActionName
			cloned["method"] = rejectActionMethod
		} else if outbound != "" {
			// User selected regular outbound - set outbound, remove action and method
			cloned["outbound"] = outbound
			delete(cloned, "action")
			delete(cloned, "method")
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

func (state *WizardState) buildParserOutboundsBlock(forPreview bool) string {
	const indent = "    "
	var builder strings.Builder
	builder.WriteString(indent + "/** @ParserSTART */\n")

	// Если это для окна (forPreview == true) И узлов больше maxNodesForFullPreview, показываем статистику
	if forPreview && state.OutboundStats.NodesCount > maxNodesForFullPreview {
		statsComment := fmt.Sprintf(`%s// Generated: %d nodes, %d local selectors, %d global selectors
%s// Total outbounds: %d
`,
			indent,
			state.OutboundStats.NodesCount,
			state.OutboundStats.LocalSelectorsCount,
			state.OutboundStats.GlobalSelectorsCount,
			len(state.GeneratedOutbounds),
			indent)
		builder.WriteString(statsComment)
	} else {
		// Для файла (forPreview == false) или для малого количества узлов - показываем полный список
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
		"drop":             {}, // Always include "drop" in available options
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
		// Добавляем глобальные outbounds
		for _, outbound := range parserCfg.ParserConfig.Outbounds {
			// Пропускаем outbounds с "wizard":"hide"
			if outbound.Wizard == "hide" {
				continue
			}
			if outbound.Tag != "" {
				tags[outbound.Tag] = struct{}{}
			}
			for _, extra := range outbound.AddOutbounds {
				tags[extra] = struct{}{}
			}
		}
		// Добавляем локальные outbounds из всех ProxySource
		for _, proxySource := range parserCfg.ParserConfig.Proxies {
			for _, outbound := range proxySource.Outbounds {
				// Пропускаем outbounds с "wizard":"hide"
				if outbound.Wizard == "hide" {
					continue
				}
				if outbound.Tag != "" {
					tags[outbound.Tag] = struct{}{}
				}
				for _, extra := range outbound.AddOutbounds {
					tags[extra] = struct{}{}
				}
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

// parseNodeFromString парсит узел из строки (обертка над parsers.ParseNode)
func parseNodeFromString(uri string, skipFilters []map[string]string) (*parsers.ParsedNode, error) {
	return parsers.ParseNode(uri, skipFilters)
}

// generateNodeJSONForPreview генерирует JSON для узла через ConfigService
func generateNodeJSONForPreview(state *WizardState, node *parsers.ParsedNode) (string, error) {
	if state.Controller == nil || state.Controller.ConfigService == nil {
		return "", fmt.Errorf("ConfigService not available")
	}
	return state.Controller.ConfigService.GenerateNodeJSON(node)
}

// generateSelectorForPreview генерирует JSON для селектора через ConfigService
func generateSelectorForPreview(state *WizardState, allNodes []*parsers.ParsedNode, outboundConfig core.OutboundConfig) (string, error) {
	if state.Controller == nil || state.Controller.ConfigService == nil {
		return "", fmt.Errorf("ConfigService not available")
	}
	return state.Controller.ConfigService.GenerateSelector(allNodes, outboundConfig)
}

func serializeParserConfig(parserConfig *core.ParserConfig) (string, error) {
	if parserConfig == nil {
		return "", fmt.Errorf("parserConfig is nil")
	}

	// Normalize ParserConfig (migrate version, set defaults, but don't update last_updated)
	core.NormalizeParserConfig(parserConfig, false)

	// Serialize in version 2 format (version inside ParserConfig, not at top level)
	configToSerialize := map[string]interface{}{
		"ParserConfig": parserConfig.ParserConfig,
	}
	data, err := json.MarshalIndent(configToSerialize, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
