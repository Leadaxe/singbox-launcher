// SPEC 052 phase 8 polish — subscription operation status panel.
//
// Раньше Update показывал toast через `dialogs.ShowAutoHideInfo`
// (отдельный popup-window). Теперь весь поток статусов от Update'а идёт
// in-place под кнопкой Exit на Core dashboard:
//
//   - Promise log: streaming-строки от UpdateParserProgressFunc, latest
//     внизу. Скролл если перерос.
//   - Final toast: green ✓ / red ✗ карточка с close-X и текстом результата;
//     auto-hide через 20 секунд (см. subsToastTTL).
//
// Почему не popup: на каждый Update / Per-source Refresh / 1ч heartbeat
// открывать modal — раздражает; in-place panel discoverable и не воркует
// с user flow.
package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// createSubsStatusBlock — VBox под Exit кнопкой:
//
//   1. Progress bar + current-status label (active во время операции)
//   2. Лог стрима (по строке на каждый status callback)
//   3. Финальный тост (✓/✗ + ×, auto-hide 20s)
//
// На старте все три секции пустые (collapsed); появляются по мере операций.
func (tab *CoreDashboardTab) createSubsStatusBlock() fyne.CanvasObject {
	tab.subsLogBox = container.NewVBox()
	tab.subsLogScroll = container.NewVScroll(tab.subsLogBox)
	tab.subsLogScroll.SetMinSize(fyne.NewSize(0, 0))
	tab.subsLogScroll.Hide()

	tab.subsToastBox = container.NewMax()
	tab.subsToastBox.Hide()

	progressBlock := container.NewVBox(
		tab.parserProgressBar,
		tab.parserStatusLabel,
	)

	return container.NewVBox(
		progressBlock,
		tab.subsLogScroll,
		tab.subsToastBox,
	)
}

// appendSubsLogLine — добавить строку в лог; latest внизу; clamped до
// subsLogMaxLines (FIFO drop старых). Должна вызываться из main thread
// (через fyne.Do в callback'ах).
func (tab *CoreDashboardTab) appendSubsLogLine(message string) {
	if tab.subsLogBox == nil {
		return
	}
	if message == "" {
		return
	}
	line := widget.NewLabel(message)
	line.Importance = widget.LowImportance
	line.Wrapping = fyne.TextWrapWord
	tab.subsLogBox.Add(line)

	// Clamp old lines.
	if n := len(tab.subsLogBox.Objects); n > subsLogMaxLines {
		tab.subsLogBox.Objects = tab.subsLogBox.Objects[n-subsLogMaxLines:]
	}

	// Auto-show + scroll-to-bottom.
	if !tab.subsLogScroll.Visible() {
		tab.subsLogScroll.Show()
	}
	// Adapt min height до 6 строк (capped).
	height := float32(len(tab.subsLogBox.Objects)) * 22
	if height > 140 {
		height = 140
	}
	tab.subsLogScroll.SetMinSize(fyne.NewSize(0, height))
	tab.subsLogScroll.Refresh()
	tab.subsLogScroll.ScrollToBottom()
}

// clearSubsLog — скрыть и обнулить лог-секцию (вызывается перед стартом
// новой операции, чтобы не накапливать).
func (tab *CoreDashboardTab) clearSubsLog() {
	if tab.subsLogBox == nil {
		return
	}
	tab.subsLogBox.Objects = nil
	tab.subsLogBox.Refresh()
	tab.subsLogScroll.Hide()
	tab.subsLogScroll.SetMinSize(fyne.NewSize(0, 0))
}

// showSubsToast — показать финальный статус: success=true → green ✓,
// success=false → red ✗. Кнопка × закрывает немедленно. Auto-hide через
// subsToastTTL (20 сек).
//
// Если предыдущий тост ещё на экране — заменяем (предыдущий timer
// отменяется).
func (tab *CoreDashboardTab) showSubsToast(message string, success bool) {
	if tab.subsToastBox == nil {
		return
	}
	// Cancel previous timer.
	if tab.subsToastTimer != nil {
		tab.subsToastTimer.Stop()
		tab.subsToastTimer = nil
	}

	// Цветной только icon (зелёная ✓ / красный ✗); фон — нейтральный.
	var coloredIcon fyne.CanvasObject
	if success {
		t := canvas.NewText("✓", color.NRGBA{R: 60, G: 200, B: 80, A: 255})
		t.TextSize = 20
		t.TextStyle = fyne.TextStyle{Bold: true}
		coloredIcon = t
	} else {
		t := canvas.NewText("✗", color.NRGBA{R: 220, G: 70, B: 70, A: 255})
		t.TextSize = 20
		t.TextStyle = fyne.TextStyle{Bold: true}
		coloredIcon = t
	}

	msg := widget.NewLabel(message)
	msg.Wrapping = fyne.TextWrapWord

	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		tab.hideSubsToast()
	})
	closeBtn.Importance = widget.LowImportance

	body := container.NewBorder(nil, nil, coloredIcon, closeBtn, msg)
	padded := container.NewPadded(body)

	tab.subsToastBox.Objects = []fyne.CanvasObject{padded}
	tab.subsToastBox.Show()
	tab.subsToastBox.Refresh()

	// Auto-hide after TTL.
	tab.subsToastTimer = time.AfterFunc(subsToastTTL, func() {
		fyne.Do(func() {
			tab.hideSubsToast()
		})
	})
}

// hideSubsToast — закрывает финальный тост (×-клик или auto-hide TTL).
func (tab *CoreDashboardTab) hideSubsToast() {
	if tab.subsToastBox == nil {
		return
	}
	if tab.subsToastTimer != nil {
		tab.subsToastTimer.Stop()
		tab.subsToastTimer = nil
	}
	tab.subsToastBox.Objects = nil
	tab.subsToastBox.Hide()
	tab.subsToastBox.Refresh()
}
