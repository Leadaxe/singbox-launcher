// Package tabs содержит UI компоненты для табов визарда конфигурации.
//
// Файл preview_tab.go содержит функцию CreatePreviewTab, которая создает UI третьего таба визарда:
//   - Отображение preview финальной конфигурации (TemplatePreviewEntry)
//   - Статус генерации preview (TemplatePreviewStatusLabel)
//   - Кнопка показа preview в отдельном окне (ShowPreviewButton)
//
// Preview таб показывает финальную конфигурацию, которая будет сохранена, включая:
//   - @ParserConfig блок с нормализованными данными
//   - Все выбранные секции из шаблона
//   - Сгенерированные outbounds
//   - Объединенные правила маршрутизации
//
// Каждый таб визарда имеет свою отдельную ответственность и логику UI.
// Preview таб имеет простую структуру (только текстовое поле и кнопка).
//
// Используется в:
//   - wizard.go - при создании окна визарда, вызывается CreatePreviewTab(presenter)
//
// Взаимодействует с:
//   - presenter - preview текст устанавливается через SetTemplatePreviewText
//   - presenter_async.go - UpdateTemplatePreviewAsync обновляет preview асинхронно
package tabs

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	wizardpresentation "singbox-launcher/ui/wizard/presentation"
)

// CreatePreviewTab creates the Preview tab UI.
func CreatePreviewTab(presenter *wizardpresentation.WizardPresenter) fyne.CanvasObject {
	guiState := presenter.GUIState()
	
	guiState.TemplatePreviewEntry = widget.NewMultiLineEntry()
	guiState.TemplatePreviewEntry.SetPlaceHolder("Preview will appear here")
	guiState.TemplatePreviewEntry.Wrapping = fyne.TextWrapOff
	guiState.TemplatePreviewEntry.OnChanged = func(text string) {
		// Read-only field, do nothing on change
	}
	previewWithHeight := container.NewMax(
		canvas.NewRectangle(color.Transparent),
		guiState.TemplatePreviewEntry,
	)
	presenter.SetTemplatePreviewText("Preview will appear here")

	previewScroll := container.NewVScroll(previewWithHeight)
	maxHeight := guiState.Window.Canvas().Size().Height * 0.7
	if maxHeight <= 0 {
		maxHeight = 480
	}
	previewScroll.SetMinSize(fyne.NewSize(0, maxHeight))

	// Create status label and button for generating preview
	guiState.TemplatePreviewStatusLabel = widget.NewLabel("Click 'Show Preview' to generate preview (this may take a long time for large configurations)")
	guiState.TemplatePreviewStatusLabel.Wrapping = fyne.TextWrapWord

	guiState.ShowPreviewButton = widget.NewButton("Show Preview", func() {
		if guiState.ShowPreviewButton != nil {
			guiState.ShowPreviewButton.Disable()
		}
		presenter.UpdateTemplatePreviewAsync()
	})

	// Container with status (takes all available space) and button on right
	statusRow := container.NewBorder(
		nil, nil,
		nil,                              // left
		guiState.ShowPreviewButton,       // right - fixed width by content
		guiState.TemplatePreviewStatusLabel, // center - takes all available space
	)

	return container.NewVBox(
		widget.NewLabel("Preview"),
		previewScroll,
		statusRow,
	)
}
