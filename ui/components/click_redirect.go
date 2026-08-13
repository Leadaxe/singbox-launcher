package components

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/uiservice"
)

// clickRedirect — невидимый tappable-оverlay, который перехватывает клики
// по основному содержимому и переносит фокус на окно визарда, если он открыт.
//
// Зависит от *uiservice.UIService, а не от *core.AppController: фокус окон —
// единственное, что виджету нужно. Раньше он тянул сюда весь `core`, и через
// общий пакет виджетов эта зависимость доставалась всем, кто импортирует
// отсюда gutter-хелперы (20+ файлов, включая ui/traffic, заявленный
// изолированным от AppController). uiservice — листовой пакет.
//
// Особенности реализации:
//   - Это отдельный Widget (расширяет widget.BaseWidget), поэтому он корректно
//     интегрируется в Fyne layout и не ломает систему событий.
//   - CreateRenderer возвращает прозрачный прямоугольник — оверлей визуально
//     не видим, но принимает события кликов.
//   - Tapped вызывается Fyne в UI-потоке, поэтому Show/RequestFocus безопасны напрямую.
//
// Использование:
//   - В главном окне — гейтится через `ui.wizardOverlayEnabled` (off by default
//     с v0.9.8; главное окно остаётся юзабельным параллельно с визардом).
//   - Внутри визарда — используется `presenter.UpdateChildOverlay` поверх
//     wizard-табов, чтобы открытое child-окно (Edit Outbound, View Source,
//     rule dialog) не пряталось за wizard'ом.
//
// ClickRedirect — невидимый tappable-оverlay, который перехватывает клики
// по основному содержимому и переносит фокус на окно визарда, если он открыт.
//
// Экспортированное имя позволяет другим пакетам (например, `ui`) явно
// ссылаться на тип при необходимости более строгой типизации.
// При этом конструктор `NewClickRedirect` остаётся предпочтительным способом
// создания экземпляра.
type ClickRedirect struct {
	widget.BaseWidget
	ui *uiservice.UIService
}

// NewClickRedirect creates a new ClickRedirect overlay instance.
func NewClickRedirect(ui *uiservice.UIService) *ClickRedirect {
	w := &ClickRedirect{ui: ui}
	w.ExtendBaseWidget(w)
	return w
}

func (w *ClickRedirect) Tapped(e *fyne.PointEvent) {
	if w == nil || w.ui == nil || w.ui.WizardWindow == nil {
		return
	}
	w.ui.WizardWindow.Show()
	w.ui.WizardWindow.RequestFocus()
	if w.ui.FocusOpenChildWindows != nil {
		w.ui.FocusOpenChildWindows()
	}
}

func (w *ClickRedirect) TappedSecondary(e *fyne.PointEvent) { w.Tapped(e) }

func (w *ClickRedirect) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(rect)
}
