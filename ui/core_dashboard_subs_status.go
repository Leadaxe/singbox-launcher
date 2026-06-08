// SPEC 052 phase 8 polish — subscription operation status panel.
//
// Один in-place toast под Exit-кнопкой, updating in place на каждом
// progress callback'е. Никакого scrollable-лога — пользователь видит
// текущее состояние и финальный результат в одной карточке.
//
// Layout: [icon]  Title         [×]
//
//	Subtitle (опционально)
//	[progress bar при активной операции]
//
// Состояния:
//   - inProgress: нейтральная иконка ⋯, без × кнопки, прогрессбар активен
//   - success:    зелёная ✓, × кнопка, без прогрессбара, auto-hide 20s
//   - error:      красная ✗, × кнопка, без прогрессбара, auto-hide 20s
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

// subsToastTTL — сколько финальный (success/error) тост висит до
// автоскрытия (или до клика на ×). Промежуточный (in-progress) toast
// не имеет TTL — он живёт до перехода в final state.
const subsToastTTL = 20 * time.Second

// createSubsStatusBlock — пустой контейнер под Exit для будущих тостов.
// На старте скрыт; появляется при первом вызове setSubsToastInProgress.
func (tab *CoreDashboardTab) createSubsStatusBlock() fyne.CanvasObject {
	tab.subsToastBox = container.NewStack()
	tab.subsToastBox.Hide()
	return tab.subsToastBox
}

// setSubsToastInProgress — карточка статуса для активной операции.
// title — основная строка (например, "Refreshing subscriptions").
// subtitle — детали (например, "Fetching 3/5: https://..."). Может быть пустым.
func (tab *CoreDashboardTab) setSubsToastInProgress(title, subtitle string) {
	if tab.subsToastBox == nil {
		return
	}
	if tab.subsToastTimer != nil {
		tab.subsToastTimer.Stop()
		tab.subsToastTimer = nil
	}

	// Нейтральная иконка ⋯ (waiting).
	icon := canvas.NewText("⋯", color.NRGBA{R: 100, G: 140, B: 220, A: 255})
	icon.TextSize = 22
	icon.TextStyle = fyne.TextStyle{Bold: true}

	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Wrapping = fyne.TextWrapWord

	textCol := container.NewVBox(titleLabel)
	if subtitle != "" {
		subLabel := widget.NewLabel(subtitle)
		subLabel.Importance = widget.LowImportance
		subLabel.Wrapping = fyne.TextWrapWord
		textCol.Add(subLabel)
	}
	if tab.parserProgressBar != nil {
		// Re-attach: progress bar live в этом toast'е.
		tab.parserProgressBar.Show()
		textCol.Add(tab.parserProgressBar)
	}

	body := container.NewBorder(nil, nil, container.NewPadded(icon), nil, textCol)
	padded := container.NewPadded(body)

	tab.subsToastBox.Objects = []fyne.CanvasObject{padded}
	tab.subsToastBox.Show()
	tab.subsToastBox.Refresh()
}

// setSubsToastResult — финальный статус: success → зелёная ✓, иначе
// красная ✗. Кнопка × закрывает немедленно. Auto-hide через subsToastTTL.
func (tab *CoreDashboardTab) setSubsToastResult(message string, success bool) {
	if tab.subsToastBox == nil {
		return
	}
	if tab.subsToastTimer != nil {
		tab.subsToastTimer.Stop()
		tab.subsToastTimer = nil
	}

	var icon fyne.CanvasObject
	if success {
		t := canvas.NewText("✓", color.NRGBA{R: 60, G: 200, B: 80, A: 255})
		t.TextSize = 22
		t.TextStyle = fyne.TextStyle{Bold: true}
		icon = t
	} else {
		t := canvas.NewText("✗", color.NRGBA{R: 220, G: 70, B: 70, A: 255})
		t.TextSize = 22
		t.TextStyle = fyne.TextStyle{Bold: true}
		icon = t
	}

	msg := widget.NewLabel(message)
	msg.Wrapping = fyne.TextWrapWord

	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		tab.hideSubsToast()
	})
	closeBtn.Importance = widget.LowImportance

	// Прячем progress bar — финальное состояние не нуждается в нём.
	if tab.parserProgressBar != nil {
		tab.parserProgressBar.Hide()
	}

	body := container.NewBorder(nil, nil, container.NewPadded(icon), closeBtn, msg)
	padded := container.NewPadded(body)

	tab.subsToastBox.Objects = []fyne.CanvasObject{padded}
	tab.subsToastBox.Show()
	tab.subsToastBox.Refresh()

	tab.subsToastTimer = time.AfterFunc(subsToastTTL, func() {
		fyne.Do(func() { tab.hideSubsToast() })
	})
}

// hideSubsToast — закрывает toast (×-клик или auto-hide TTL).
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
