package components

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
)

// clickRedirect — невидимый tappable-оverlay, который перехватывает клики
// по основному содержимому и переносит фокус на окно визарда, если он открыт.
//
// Вынесен в `ui/components` для лучшей организации кода — компонент может быть
// переиспользован или протестирован отдельно от композиции `App`.
//
// Особенности реализации:
//   - Это отдельный Widget (расширяет widget.BaseWidget), поэтому он корректно
//     интегрируется в Fyne layout и не ломает систему событий.
//   - CreateRenderer возвращает прозрачный прямоугольник — оверлей визуально
//     не видим, но принимает события кликов.
//   - Tapped вызывается Fyne в UI-потоке, поэтому Show/RequestFocus безопасны напрямую.
//
// Место использования: создаётся в InitWizardOverlay() и помещается поверх вкладок.
// ClickRedirect — невидимый tappable-оverlay, который перехватывает клики
// по основному содержимому и переносит фокус на окно визарда, если он открыт.
//
// Экспортированное имя позволяет другим пакетам (например, `ui`) явно
// ссылаться на тип при необходимости более строгой типизации.
// При этом конструктор `NewClickRedirect` остаётся предпочтительным способом
// создания экземпляра.
type ClickRedirect struct {
	widget.BaseWidget
	controller *core.AppController
}

// NewClickRedirect creates a new ClickRedirect overlay instance.
func NewClickRedirect(controller *core.AppController) *ClickRedirect {
	w := &ClickRedirect{controller: controller}
	w.ExtendBaseWidget(w)
	return w
}

func (w *ClickRedirect) Tapped(e *fyne.PointEvent) {
	if w == nil || w.controller == nil || w.controller.UIService == nil {
		return
	}
	if w.controller.UIService.WizardWindow != nil {
		w.controller.UIService.WizardWindow.Show()
		w.controller.UIService.WizardWindow.RequestFocus()
		if w.controller.UIService.FocusOpenChildWindows != nil {
			w.controller.UIService.FocusOpenChildWindows()
		}
	}
}

func (w *ClickRedirect) TappedSecondary(e *fyne.PointEvent) { w.Tapped(e) }

func (w *ClickRedirect) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(rect)
}
