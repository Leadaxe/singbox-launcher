package ui

import (
	"time"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
)

const downloadPlaceholderWidth = 120

// formatRelativeAge renders a short "subs updated Xm ago" hint.
// Sub-minute resolution is noisy here; clamp to minutes / hours / days.
func formatRelativeAge(d time.Duration) string {
	if d < time.Minute {
		return locale.T("core.subs_updated_just_now")
	}
	if d < time.Hour {
		return locale.Tf("core.subs_updated_min_ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return locale.Tf("core.subs_updated_hr_ago", int(d.Hours()))
	}
	return locale.Tf("core.subs_updated_day_ago", int(d.Hours()/24))
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
