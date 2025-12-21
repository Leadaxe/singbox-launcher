//go:build !wizard_standalone
// +build !wizard_standalone

package wizard

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/ui"
)

// CreateVLESSSourceTab создает первую вкладку с полями для VLESS URL и ParserConfig
func CreateVLESSSourceTab(state *ui.WizardState) fyne.CanvasObject {
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

	// Создаем прямоугольник-заглушку, равный по высоте прогресс-бару (примерно 4px + отступы)
	// ProgressBar обычно имеет минимальную высоту. Подберем экспериментально или используем layout.
	state.CheckURLPlaceholder = canvas.NewRectangle(nil)
	state.CheckURLPlaceholder.SetMinSize(fyne.NewSize(10, 4)) // Минимальная высота прогрессбара
	state.CheckURLPlaceholder.Hide()                          // Скрыт по умолчанию

	// Контейнер для кнопки и прогресс-бара
	// Используем Stack, чтобы прогресс-бар перекрывал кнопку или был под ней/вместо неё
	// Но лучше показывать прогресс ПОД кнопкой или ВМЕСТО заглушки.
	// Здесь реализуем "кнопка, а под ней прогресс (появляется)"
	// Чтобы интерфейс не прыгал, используем заглушку.

	// UPD: Лучший вариант - стек, где прогресс бар поверх кнопки (или вместо) - но тогда кнопка исчезает.
	// Вариант с заглушкой: VBox(Button, Stack(Placeholder, ProgressBar))
	state.CheckURLContainer = container.NewVBox(
		state.CheckURLButton,
		container.NewStack(state.CheckURLPlaceholder, state.CheckURLProgress),
	)

	state.VLESSURLEntry = widget.NewMultiLineEntry()
	state.VLESSURLEntry.SetPlaceHolder("Enter VLESS/VMess/Trojan subscription URL (http/https)\nOR list of direct links (vless://, vmess://...)")
	state.VLESSURLEntry.SetMinRowsVisible(5)
	state.VLESSURLEntry.OnChanged = func(s string) {
		// Сбрасываем статус при изменении
		state.URLStatusLabel.SetText("")
		state.GeneratedOutbounds = nil // Reset generated outbounds
		state.previewNeedsParse = true
	}

	state.URLStatusLabel = widget.NewLabel("")
	state.URLStatusLabel.Wrapping = fyne.TextWrapWord

	// Секция 2: Parser Config (JSON)
	state.ParserConfigEntry = widget.NewMultiLineEntry()
	state.ParserConfigEntry.SetPlaceHolder("Parser configuration (JSON).\nThis will be generated automatically if template is loaded.")
	state.ParserConfigEntry.SetMinRowsVisible(10)
	state.ParserConfigEntry.OnChanged = func(s string) {
		if !state.parserConfigUpdating {
			state.previewNeedsParse = true
		}
	}

	state.ParseButton = widget.NewButton("Parse & Generate Preview", func() {
		// Run parsing logic
		parseConfig(state)
	})

	state.OutboundsPreview = widget.NewMultiLineEntry()
	state.OutboundsPreview.SetPlaceHolder("Parsed outbounds preview will appear here...")
	state.OutboundsPreview.SetMinRowsVisible(8)
	state.OutboundsPreview.Disable() // Read-only

	return container.NewVBox(
		widget.NewLabelWithStyle("Source", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Enter subscription URL or direct links:"),
		state.VLESSURLEntry,
		state.CheckURLContainer,
		state.URLStatusLabel,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Parser Configuration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		state.ParserConfigEntry,
		widget.NewSeparator(),
		state.ParseButton,
		widget.NewLabel("Outbounds Preview:"),
		state.OutboundsPreview,
	)
}

// checkURL проверяет доступность URL и тип контента
func checkURL(state *ui.WizardState) {
	state.checkURLInProgress = true
	safeFyneDo(state.Window, func() {
		state.CheckURLButton.Disable()
		state.CheckURLPlaceholder.Hide()
		state.CheckURLProgress.Show()
		state.CheckURLProgress.SetValue(0) // Reset to 0 (indeterminate or start)
		state.URLStatusLabel.SetText("Checking...")
	})

	// Start indeterminate animation or just update progress
	// For now, simple progress updates
	updateProgress := func(p float64) {
		safeFyneDo(state.Window, func() {
			state.CheckURLProgress.SetValue(p)
		})
	}

	defer func() {
		state.checkURLInProgress = false
		safeFyneDo(state.Window, func() {
			state.CheckURLButton.Enable()
			state.CheckURLProgress.Hide()
			state.CheckURLPlaceholder.Show() // Show placeholder back to keep layout stable? Or Hide both.
			// Actually better to keep layout stable if possible.
			// If we hide placeholder, layout shrinks. Let's show placeholder.
			state.CheckURLPlaceholder.Refresh()
		})
	}()

	urlStr := strings.TrimSpace(state.VLESSURLEntry.Text)
	if urlStr == "" {
		safeFyneDo(state.Window, func() {
			state.URLStatusLabel.SetText("Error: URL is empty")
		})
		return
	}

	lines := strings.Split(urlStr, "\n")
	validLinks := 0
	isSubscription := false

	// Analyze input
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if subscription.IsSubscriptionURL(line) {
			isSubscription = true
			validLinks++
		} else if subscription.IsDirectLink(line) {
			validLinks++
		}
	}

	if validLinks == 0 {
		safeFyneDo(state.Window, func() {
			state.URLStatusLabel.SetText("Error: No valid subscription URL or direct links found")
		})
		return
	}

	updateProgress(0.3)

	if isSubscription {
		// Если есть подписка, пробуем скачать первую
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if subscription.IsSubscriptionURL(line) {
				updateProgress(0.5)
				// Use AppController to fetch (it has the logic)
				// Ideally logic should be in a service, but for now we call the method if available or reimplement?
				// Fetch subscription content using subscription.FetchSubscription

				// Fetch content
				_, err := subscription.FetchSubscription(line)
				if err != nil {
					safeFyneDo(state.Window, func() {
						state.URLStatusLabel.SetText(fmt.Sprintf("Error fetching subscription: %v", err))
					})
					return
				}
				break // Check only first subscription
			}
		}
	}

	updateProgress(1.0)
	safeFyneDo(state.Window, func() {
		state.URLStatusLabel.SetText(fmt.Sprintf("Success! Found %d valid link(s).", validLinks))
	})
}

// parseConfig запускает парсинг конфига
func parseConfig(state *ui.WizardState) {
	if state.Controller == nil || state.Controller.ConfigService == nil {
		dialogs.ShowError(state.Window, fmt.Errorf("Internal error: ConfigService not initialized"))
		return
	}

	// 1. Update ParserConfig from UI
	parserConfigJSON := state.ParserConfigEntry.Text
	if parserConfigJSON == "" {
		dialogs.ShowError(state.Window, fmt.Errorf("Parser configuration is empty"))
		return
	}

	// TODO: Parse ParserConfig JSON and use it
	// For now, we simulate parsing by using existing logic if possible
	// Actually, we need to parse the JSON to get ProxySource
	// This part needs ConfigService to expose a method to parse from string/struct
	// But ConfigService uses file path.
	// Refactoring needed: ConfigService.UpdateConfigFromSubscriptions reads from file.
	// We want to test "live" without saving to file yet?
	// Or we save to temporary file?

	// Let's implement a simpler check: just validate JSON
	// The actual "Preview" logic in current code seems to rely on ParserConfig struct
	// which is populated from file or template.

	state.OutboundsPreview.SetText("Parsing logic needs ConfigService integration...")
}
