package ui

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
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
	"singbox-launcher/internal/platform"
)

// safeFyneDo safely calls fyne.Do only if window is still valid
func safeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}

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
	CheckURLProgress     *widget.ProgressBar
	CheckURLPlaceholder  *canvas.Rectangle
	CheckURLContainer    fyne.CanvasObject
	checkURLInProgress   bool
	ParseButton          *widget.Button
	parserConfigUpdating bool

	// Parsed data
	ParserConfig       *core.ParserConfig
	GeneratedOutbounds []string

	// Template data for second tab
	TemplateData                *TemplateData
	TemplateSectionSelections   map[string]bool
	SelectableRuleStates        []*SelectableRuleState
	TemplatePreviewEntry        *widget.Entry
	TemplatePreviewText         string
	templatePreviewUpdating     bool
	TemplatePreviewStatusLabel  *widget.Label
	TemplatePreviewCache        string // Кэшированный текст превью
	TemplatePreviewCacheHash    string // Хеш данных для проверки изменений
	FinalOutboundSelect         *widget.Select
	SelectedFinalOutbound       string
	previewNeedsParse           bool
	autoParseInProgress         bool
	previewGenerationInProgress bool

	// Debounce timer for template preview updates
	previewUpdateTimer *time.Timer
	previewUpdateMutex sync.Mutex

	// Navigation buttons
	CloseButton      *widget.Button
	PrevButton       *widget.Button
	NextButton       *widget.Button
	SaveButton       *widget.Button
	SaveProgress     *widget.ProgressBar
	SavePlaceholder  *canvas.Rectangle
	saveInProgress   bool
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
		// Очищаем таймер обновления превью перед закрытием
		state.previewUpdateMutex.Lock()
		if state.previewUpdateTimer != nil {
			state.previewUpdateTimer.Stop()
			state.previewUpdateTimer = nil
		}
		state.previewUpdateMutex.Unlock()
		wizardWindow.Close()
	})

	// Очищаем таймер при закрытии окна через X
	wizardWindow.SetCloseIntercept(func() {
		state.previewUpdateMutex.Lock()
		if state.previewUpdateTimer != nil {
			state.previewUpdateTimer.Stop()
			state.previewUpdateTimer = nil
		}
		state.previewUpdateMutex.Unlock()
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
		if state.previewGenerationInProgress {
			dialog.ShowInformation("Generating", "Preview generation in progress... Please wait.", state.Window)
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
			text, err := buildTemplateConfig(state)
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
			// Проверяем, есть ли кэшированное превью
			currentHash := state.calculatePreviewHash()
			if currentHash == state.TemplatePreviewCacheHash && state.TemplatePreviewCache != "" {
				// Показываем кэшированное превью сразу
				if state.TemplatePreviewEntry != nil {
					state.setTemplatePreviewText(state.TemplatePreviewCache)
				}
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText("✅ Preview ready (cached)")
				}
				// Кнопка Save остается включенной
			} else {
				// Показываем индикацию загрузки только если нет кэша
				if state.TemplatePreviewEntry != nil {
					if state.TemplatePreviewCache != "" {
						// Показываем старое превью пока генерируется новое
						state.setTemplatePreviewText(state.TemplatePreviewCache)
					} else {
						state.setTemplatePreviewText("Loading preview...")
					}
				}
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText("⏳ Loading...")
				}
				// Отключаем кнопку Save только если идет генерация
				if state.previewGenerationInProgress {
					if state.SaveButton != nil {
						state.SaveButton.Disable()
					}
				}
			}

			// Запускаем операции с задержкой 500мс после переключения вкладки для полной отрисовки
			go func() {
				time.Sleep(500 * time.Millisecond)

				// Запускаем парсинг асинхронно (если нужно)
				state.triggerParseForPreview()

				// Обновляем превью только если данные изменились или кэша нет
				// updateTemplatePreviewAsync сама проверит хеш и использует кэш если возможно
				if currentHash != state.TemplatePreviewCacheHash || state.TemplatePreviewCache == "" {
					state.updateTemplatePreviewAsync()
				}
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
	state.VLESSURLEntry.SetPlaceHolder("https://example.com/subscription\nor\nvless://...\nvmess://...")
	state.VLESSURLEntry.Wrapping = fyne.TextWrapOff
	state.VLESSURLEntry.OnChanged = func(value string) {
		state.previewNeedsParse = true
		state.applyURLToParserConfig(strings.TrimSpace(value))
	}

	// Подсказка под полем ввода с кнопкой Check справа
	hintLabel := widget.NewLabel("Supports subscription URLs (http/https) or direct links (vless://, vmess://, trojan://, ss://).\nFor multiple links, use a new line for each.")
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
	urlEntrySizeRect.SetMinSize(fyne.NewSize(900, 60)) // Ширина 900px, высота ~3 строки (примерно 20px на строку)
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

		// Debounce updateTemplatePreview - обновляем через 500ms после последнего изменения
		// Это предотвращает 100% загрузку CPU при быстром вводе текста
		state.previewUpdateMutex.Lock()
		if state.previewUpdateTimer != nil {
			state.previewUpdateTimer.Stop()
		}
		state.previewUpdateTimer = time.AfterFunc(500*time.Millisecond, func() {
			safeFyneDo(state.Window, func() {
				state.updateTemplatePreview()
			})
		})
		state.previewUpdateMutex.Unlock()
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
	previewHeightRect.SetMinSize(fyne.NewSize(0, 200)) // ~10 строк
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

	// Создаем статус-лейбл под полем превью
	state.TemplatePreviewStatusLabel = widget.NewLabel("Ready")
	state.TemplatePreviewStatusLabel.Wrapping = fyne.TextWrapWord

	return container.NewVBox(
		widget.NewLabel("Preview"),
		previewScroll,
		state.TemplatePreviewStatusLabel,
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

	// Заполняем поле URL - объединяем Source и Connections
	if len(parserConfig.ParserConfig.Proxies) > 0 {
		proxySource := parserConfig.ParserConfig.Proxies[0]
		lines := make([]string, 0)
		if proxySource.Source != "" {
			lines = append(lines, proxySource.Source)
		}
		lines = append(lines, proxySource.Connections...)
		state.VLESSURLEntry.SetText(strings.Join(lines, "\n"))
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
	input := strings.TrimSpace(state.VLESSURLEntry.Text)
	if input == "" {
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
	totalValid := 0
	previewLines := make([]string, 0)
	errors := make([]string, 0)

	for i, line := range inputLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		safeFyneDo(state.Window, func() {
			progress := float64(i+1) / float64(len(inputLines))
			state.setCheckURLState(fmt.Sprintf("⏳ Checking... (%d/%d)", i+1, len(inputLines)), "", progress)
		})

		if core.IsSubscriptionURL(line) {
			// Это URL подписки - проверяем доступность
			content, err := core.FetchSubscription(line)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to fetch %s: %v", line, err))
				continue
			}

			// Проверяем содержимое подписки
			subLines := strings.Split(string(content), "\n")
			validInSub := 0
			for _, subLine := range subLines {
				subLine = strings.TrimSpace(subLine)
				if subLine != "" && (strings.HasPrefix(subLine, "vless://") || strings.HasPrefix(subLine, "vmess://") ||
					strings.HasPrefix(subLine, "trojan://") || strings.HasPrefix(subLine, "ss://")) {
					validInSub++
					totalValid++
					if len(previewLines) < 10 { // Ограничиваем превью
						previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, subLine))
					}
				}
			}
			if validInSub == 0 {
				errors = append(errors, fmt.Sprintf("Subscription %s contains no valid proxy links", line))
			}
		} else if core.IsDirectLink(line) {
			// Это прямая ссылка - проверяем парсинг
			_, err := core.ParseNode(line, nil)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Invalid direct link: %v", err))
			} else {
				totalValid++
				if len(previewLines) < 10 {
					previewLines = append(previewLines, fmt.Sprintf("%d. %s", totalValid, line))
				}
			}
		} else {
			errors = append(errors, fmt.Sprintf("Unknown format: %s", line))
		}
	}

	state.checkURLInProgress = false
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
	defer func() {
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
	parserConfigJSON := strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON == "" {
		safeFyneDo(state.Window, func() {
			setPreviewText(state, "Error: ParserConfig is empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("❌ Error: ParserConfig is empty")
			}
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
		safeFyneDo(state.Window, func() {
			setPreviewText(state, fmt.Sprintf("Error: Failed to parse ParserConfig JSON: %v", err))
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: Failed to parse ParserConfig JSON: %v", err))
			}
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Проверяем наличие URL или прямых ссылок
	url := strings.TrimSpace(state.VLESSURLEntry.Text)
	if url == "" {
		safeFyneDo(state.Window, func() {
			setPreviewText(state, "Error: VLESS URL or direct links are empty")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("❌ Error: VLESS URL or direct links are empty")
			}
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Обновляем конфиг через applyURLToParserConfig, который правильно разделяет подписки и connections
	state.applyURLToParserConfig(url)

	// Перезагружаем parserConfig после обновления
	parserConfigJSON = strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigJSON != "" {
		if err := json.Unmarshal([]byte(parserConfigJSON), &parserConfig); err != nil {
			safeFyneDo(state.Window, func() {
				setPreviewText(state, fmt.Sprintf("Error: Failed to parse updated ParserConfig JSON: %v", err))
				state.ParseButton.Enable()
				state.ParseButton.SetText("Parse")
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: Failed to parse updated ParserConfig JSON: %v", err))
				}
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
			})
			return
		}
	}

	// Парсим узлы используя новую логику (поддерживает и подписки и прямые ссылки)
	safeFyneDo(state.Window, func() {
		setPreviewText(state, "Processing sources...")
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Processing subscription sources...")
		}
	})

	// Map to track unique tags and their counts (same logic as UpdateConfigFromSubscriptions)
	tagCounts := make(map[string]int)
	log.Printf("ConfigWizard: Initializing tag deduplication tracker")

	allNodes := make([]*core.ParsedNode, 0)
	totalSources := len(parserConfig.ParserConfig.Proxies)

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		sourceNum := i + 1
		safeFyneDo(state.Window, func() {
			setPreviewText(state, fmt.Sprintf("Processing source %d/%d...", sourceNum, totalSources))
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("⏳ Processing source %d/%d...", sourceNum, totalSources))
			}
		})

		// Используем processProxySource для обработки (поддерживает подписки и прямые ссылки)
		progressCallback := func(p float64, s string) {
			// Можно обновлять прогресс, но не обязательно для превью
		}

		nodesFromSource, err := core.ProcessProxySource(proxySource, tagCounts, progressCallback, i, totalSources)
		if err != nil {
			log.Printf("ConfigWizard: Error processing source %d/%d: %v", i+1, totalSources, err)
			safeFyneDo(state.Window, func() {
				setPreviewText(state, fmt.Sprintf("Error: Failed to process source: %v", err))
				state.ParseButton.Enable()
				state.ParseButton.SetText("Parse")
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: Failed to process source: %v", err))
				}
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
			})
			return
		}

		allNodes = append(allNodes, nodesFromSource...)
		log.Printf("ConfigWizard: Successfully parsed %d nodes from source %d/%d", len(nodesFromSource), i+1, totalSources)
	}

	// Log statistics about duplicates
	core.LogDuplicateTagStatistics(tagCounts, "ConfigWizard")

	if len(allNodes) == 0 {
		safeFyneDo(state.Window, func() {
			setPreviewText(state, "Error: No valid nodes found in subscription")
			state.ParseButton.Enable()
			state.ParseButton.SetText("Parse")
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("❌ Error: No valid nodes found in subscription")
			}
			if state.SaveButton != nil {
				state.SaveButton.Enable()
			}
		})
		return
	}

	// Генерируем JSON для узлов
	safeFyneDo(state.Window, func() {
		setPreviewText(state, "Generating outbounds...")
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Generating outbounds...")
		}
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

	safeFyneDo(state.Window, func() {
		setPreviewText(state, previewText)
		state.ParseButton.Enable()
		state.ParseButton.SetText("Parse")
		state.GeneratedOutbounds = selectorsJSON
		state.ParserConfig = &parserConfig
		state.previewNeedsParse = false
		state.refreshOutboundOptions()
		// Обновляем статус после завершения парсинга
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳✅ Parsing complete, generating preview...")
		}
		// Кнопка Save будет включена после завершения генерации превью
		state.updateTemplatePreview()
	})
}

func setPreviewText(state *WizardState, text string) {
	state.OutboundsPreviewText = text
	if state.OutboundsPreview != nil {
		state.OutboundsPreview.SetText(text)
	}
}

func (state *WizardState) applyURLToParserConfig(input string) {
	if state.ParserConfigEntry == nil || input == "" {
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

	// Разделяем подписки и прямые ссылки
	lines := strings.Split(input, "\n")
	subscriptions := make([]string, 0)
	connections := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if core.IsSubscriptionURL(line) {
			subscriptions = append(subscriptions, line)
		} else if core.IsDirectLink(line) {
			connections = append(connections, line)
		}
	}

	// Обновляем ProxySource
	if len(parserConfig.ParserConfig.Proxies) == 0 {
		parserConfig.ParserConfig.Proxies = []core.ProxySource{
			{},
		}
	}

	proxySource := &parserConfig.ParserConfig.Proxies[0]

	// Сохраняем подписки (если несколько, берем первую или объединяем)
	if len(subscriptions) > 0 {
		proxySource.Source = subscriptions[0] // Пока берем первую, можно расширить логику
	} else {
		proxySource.Source = ""
	}

	// Сохраняем прямые ссылки в connections
	proxySource.Connections = connections

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

	state.templatePreviewUpdating = true
	state.TemplatePreviewEntry.SetText(text)
	state.templatePreviewUpdating = false
}

// calculatePreviewHash вычисляет хеш всех данных, от которых зависит превью
func (state *WizardState) calculatePreviewHash() string {
	var data strings.Builder

	// ПarserConfig
	if state.ParserConfigEntry != nil {
		data.WriteString(state.ParserConfigEntry.Text)
	}

	// GeneratedOutbounds
	data.WriteString(fmt.Sprintf("%d", len(state.GeneratedOutbounds)))
	for _, outbound := range state.GeneratedOutbounds {
		data.WriteString(outbound)
	}

	// TemplateSectionSelections
	if state.TemplateSectionSelections != nil {
		keys := make([]string, 0, len(state.TemplateSectionSelections))
		for k, v := range state.TemplateSectionSelections {
			if v {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			data.WriteString(k)
		}
	}

	// SelectedFinalOutbound
	data.WriteString(state.SelectedFinalOutbound)

	// SelectableRuleStates
	if state.SelectableRuleStates != nil {
		for _, ruleState := range state.SelectableRuleStates {
			if ruleState.Enabled {
				data.WriteString(ruleState.Rule.Label)
				data.WriteString(ruleState.SelectedOutbound)
			}
		}
	}

	// Вычисляем SHA256 хеш
	hash := sha256.Sum256([]byte(data.String()))
	return hex.EncodeToString(hash[:])
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
		if state.TemplatePreviewStatusLabel != nil {
			state.TemplatePreviewStatusLabel.SetText("⏳ Parsing subscription links...")
		}
		if state.SaveButton != nil {
			state.SaveButton.Disable()
		}
	})
	go parseAndPreview(state)
}

// invalidatePreviewCache инвалидирует кэш превью (вызывается при изменениях данных)
func (state *WizardState) invalidatePreviewCache() {
	state.TemplatePreviewCache = ""
	state.TemplatePreviewCacheHash = ""
}

func (state *WizardState) updateTemplatePreview() {
	// Синхронная версия для вызова из других мест (не блокирует UI)
	state.updateTemplatePreviewAsync()
}

func (state *WizardState) updateTemplatePreviewAsync() {
	if state.TemplateData == nil || state.TemplatePreviewEntry == nil {
		return
	}

	// Вычисляем хеш текущих данных
	currentHash := state.calculatePreviewHash()

	// Если хеш не изменился и есть кэш, используем кэш
	if currentHash == state.TemplatePreviewCacheHash && state.TemplatePreviewCache != "" {
		safeFyneDo(state.Window, func() {
			state.setTemplatePreviewText(state.TemplatePreviewCache)
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("✅ Preview ready (cached)")
			}
		})
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
		defer func() {
			state.previewGenerationInProgress = false
			safeFyneDo(state.Window, func() {
				// Включаем кнопку Save после завершения
				if state.SaveButton != nil {
					state.SaveButton.Enable()
				}
			})
		}()

		// Обновляем статус: парсинг ParserConfig
		safeFyneDo(state.Window, func() {
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("⏳ Parsing ParserConfig...")
			}
		})

		text, err := buildTemplateConfig(state)
		if err != nil {
			safeFyneDo(state.Window, func() {
				state.setTemplatePreviewText(fmt.Sprintf("Preview error: %v", err))
				if state.TemplatePreviewStatusLabel != nil {
					state.TemplatePreviewStatusLabel.SetText(fmt.Sprintf("❌ Error: %v", err))
				}
			})
			return
		}

		// Сохраняем в кэш
		state.TemplatePreviewCache = text
		state.TemplatePreviewCacheHash = currentHash

		// Обновляем статус: готово
		safeFyneDo(state.Window, func() {
			state.setTemplatePreviewText(text)
			if state.TemplatePreviewStatusLabel != nil {
				state.TemplatePreviewStatusLabel.SetText("✅ Preview ready")
			}
		})
	}()
}

func buildTemplateConfig(state *WizardState) (string, error) {
	if state.TemplateData == nil {
		return "", fmt.Errorf("template data not available")
	}
	parserConfigText := strings.TrimSpace(state.ParserConfigEntry.Text)
	if parserConfigText == "" {
		return "", fmt.Errorf("ParserConfig is empty and no template available")
	}

	// Parse ParserConfig JSON to ensure it has version 2 and parser object
	var parserConfig core.ParserConfig
	if err := json.Unmarshal([]byte(parserConfigText), &parserConfig); err != nil {
		// If parsing fails, use text as-is (might be invalid JSON, but let user fix it)
		log.Printf("buildTemplateConfig: Warning: Failed to parse ParserConfig JSON: %v", err)
	} else {
		// Normalize ParserConfig (migrate version, set defaults, update last_updated)
		core.NormalizeParserConfig(&parserConfig, true)

		// Serialize back to JSON with proper formatting (always version 2 format)
		configToSerialize := map[string]interface{}{
			"ParserConfig": parserConfig.ParserConfig,
		}
		serialized, err := json.MarshalIndent(configToSerialize, "", "  ")
		if err == nil {
			parserConfigText = string(serialized)
		} else {
			log.Printf("buildTemplateConfig: Warning: Failed to serialize ParserConfig: %v", err)
		}
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
	builder.WriteString(parserConfigText)
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
