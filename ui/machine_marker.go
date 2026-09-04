package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/locale"
)

// Кликабельный маркер состояния машины.
//
// canvas.Circle событий не ловит — он объект рисования, а не виджет. Чтобы по
// кружку открывался журнал обмена, круг заворачивается в тонкий виджет,
// который умеет ровно две вещи: принять клик и показать курсор-указатель.
//
// Отдельным виджетом, а не кнопкой с иконкой: кнопка принесла бы рамку,
// подложку и свои отступы — в строке машины это выглядело бы четвёртой
// кнопкой ряда, тогда как маркер должен читаться как индикатор.

// machineMarker — кружок состояния, реагирующий на нажатие.
//
// ToolTipWidgetExtend даёт подсказку при наведении тем же механизмом, что у
// глифовых кнопок строки (ⓘ ✎ ✕ ↻), — и заодно приносит обработчики мыши,
// поэтому своих MouseIn/MouseOut писать не нужно.
type machineMarker struct {
	widget.BaseWidget
	ttwidget.ToolTipWidgetExtend
	fill  color.Color
	onTap func()
}

// markerTooltip — что означает цвет, словами.
//
// Обязательно: цвет сам по себе не объясняет, что делать, и не читается теми,
// кто различает оттенки хуже. Текст называет состояние и подсказывает, что по
// маркеру можно кликнуть.
func markerTooltip(state markerState) string {
	switch state {
	case markerLive:
		return locale.T("The machine answers. Click to see the exchange log.")
	case markerFlaky:
		return locale.T("No answer to the last poll — retrying. Click to see the exchange log.")
	case markerDown:
		return locale.T("The machine is not answering. Click to see the exchange log.")
	default:
		return locale.T("Not connected. Click to see the exchange log.")
	}
}

func newMachineMarker(state markerState, fill color.Color, onTap func()) fyne.CanvasObject {
	_, obj := newMarkerWidget(fill, markerTooltip(state), onTap)
	return obj
}

// newMarkerWidget — тот же маркер, но с доступом к самому виджету: там, где
// кружок живёт постоянно (строка Core Status), его состояние обновляют, а не
// пересоздают строку целиком, как это делает список машин.
func newMarkerWidget(fill color.Color, tooltip string, onTap func()) (*machineMarker, fyne.CanvasObject) {
	m := &machineMarker{fill: fill, onTap: onTap}
	m.ExtendBaseWidget(m)
	m.SetToolTip(tooltip)
	// dotLayout центрирует маркер по вертикали и оставляет поле справа,
	// чтобы кружок не липнул к имени; сам виджет держит размер через MinSize.
	return m, container.New(&dotLayout{size: 10}, m)
}

// setState перекрашивает существующий маркер и меняет подсказку.
func (m *machineMarker) setState(fill color.Color, tooltip string) {
	m.fill = fill
	m.SetToolTip(tooltip)
	m.Refresh()
}

// ExtendBaseWidget обязана поднять обе базы: иначе подсказка не получит
// объект, к которому она привязана (требование ToolTipWidgetExtend).
func (m *machineMarker) ExtendBaseWidget(w fyne.Widget) {
	m.BaseWidget.ExtendBaseWidget(w)
	m.ExtendToolTipWidget(w)
}

func (m *machineMarker) CreateRenderer() fyne.WidgetRenderer {
	circle := canvas.NewCircle(m.fill)
	return &markerRenderer{marker: m, circle: circle}
}

func (m *machineMarker) Tapped(*fyne.PointEvent) {
	if m.onTap != nil {
		m.onTap()
	}
}

// Cursor — указатель поверх маркера: без него ничто не подсказывает, что
// кружок нажимается.
func (m *machineMarker) Cursor() desktop.Cursor { return desktop.PointerCursor }

type markerRenderer struct {
	marker *machineMarker
	circle *canvas.Circle
}

func (r *markerRenderer) Layout(size fyne.Size) {
	r.circle.Resize(size)
	r.circle.Move(fyne.NewPos(0, 0))
}

func (r *markerRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 10) }

func (r *markerRenderer) Refresh() {
	r.circle.FillColor = r.marker.fill
	canvas.Refresh(r.circle)
}

func (r *markerRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.circle} }
func (r *markerRenderer) Destroy()                     {}
