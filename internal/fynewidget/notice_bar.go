// File notice_bar.go — ПЛАШКА-уведомление: скруглённая полоса в окне, а не
// всплывашка (SPEC 132 §6.1, решение владельца №5).
//
// # Зачем свой виджет
//
// Готовой плашки в проекте не было: сообщения показывались либо модальным
// диалогом (dialogs.ShowInfo), либо строкой статуса. Ни то, ни другое не
// годится для «ядро выключило N серверов»: диалог требует ответа на действие,
// которого человек не совершал и которое уже случилось, а строка статуса
// живёт до следующего обновления и уносит с собой кнопку «Показать».
//
// Плашка занимает место в раскладке, пока её не закрыли крестиком, и держит
// при себе два действия: посмотреть подробности и убрать с глаз.
//
// # Вёрстка: ловушка ширины
//
// Текст плашки приходит с подстановкой имён узлов и длину имеет произвольную,
// поэтому `Label` обязан нести `fyne.TextWrapWord`: без него Fyne берёт
// ширину ОДНОЙ строки за min-width виджета, та переопределяет Resize, и окно
// раздувается на весь экран (Л19, память `fyne-label-minwidth-trap`).
// Заголовок — короткий («Отключено серверов: 3»), ему перенос не нужен, но
// Truncation всё равно стоит: длинный перевод не имеет права раздуть окно.
//
// # Цвет
//
// Фон — `theme.ColorNameWarning` с альфой: сплошной оранжевый под текстом
// темы нечитаем в обеих темах, а приглушённый читается и там, и там, не
// заводя собственных констант цвета. Значок ⚠ — тот же глиф, которым весь
// проект метит проблему узла (nodewarn.WarnMark); новых глифов не заводим
// (память `ui-visuals-approve-first`).
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package fynewidget

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// noticeBarBgAlpha — насколько приглушён фон плашки (доля от 255).
//
// 38/255 ≈ 15%: пятно читается как выделенная область в обеих темах, а текст
// поверх остаётся текстом темы, не требуя своего цвета.
const noticeBarBgAlpha = 38

// noticeBarMark — знак плашки. Тот же ⚠, что у деградаций узла.
const noticeBarMark = "⚠"

// NoticeBar — полоса-уведомление со значком, заголовком, текстом и двумя
// действиями справа.
//
// Виджет ничего не решает сам: показывать его или нет, с каким текстом и что
// делать по кнопкам — дело вызывающего. Крестик лишь зовёт onClose, а прятать
// плашку вызывающий обязан сам (Hide на возвращённом объекте): у разных
// плашек разная память о закрытии, и виджету о ней знать неоткуда.
type NoticeBar struct {
	widget.BaseWidget

	title  *widget.Label
	body   *widget.Label
	action *widget.Button
	close  *widget.Button
	bg     *canvas.Rectangle
}

// NewNoticeBar собирает плашку.
//
// actionText пустой — кнопки действия нет (плашка только сообщает).
// onClose nil — крестика нет (плашку убирает не человек, а событие).
func NewNoticeBar(title, body, actionText string, onAction, onClose func()) *NoticeBar {
	b := &NoticeBar{}
	b.ExtendBaseWidget(b)

	b.title = widget.NewLabel(noticeBarMark + " " + title)
	b.title.TextStyle.Bold = true
	// БЕЗ Wrapping и с Truncation: заголовок — одна короткая строка, переносить
	// в ней нечего, а TextWrapWord дал бы min-width в ОДНО слово, и Border,
	// поставив такой Label сверху, ужал бы его до этой ширины.
	b.title.Wrapping = fyne.TextWrapOff
	b.title.Truncation = fyne.TextTruncateEllipsis

	// Текст — произвольной длины (до трёх имён узлов плюс хвост), Wrapping
	// обязателен (ловушка в шапке файла).
	b.body = widget.NewLabel(body)
	b.body.Wrapping = fyne.TextWrapWord
	b.body.Importance = widget.MediumImportance

	if actionText != "" {
		b.action = widget.NewButton(actionText, onAction)
		b.action.Importance = widget.LowImportance
	}
	if onClose != nil {
		b.close = widget.NewButtonWithIcon("", theme.CancelIcon(), onClose)
		b.close.Importance = widget.LowImportance
	}
	return b
}

// SetText меняет заголовок и текст плашки без пересборки виджета: повторное
// событие переписывает ту же полосу, а не громоздит вторую.
func (b *NoticeBar) SetText(title, body string) {
	if b == nil {
		return
	}
	b.title.SetText(noticeBarMark + " " + title)
	b.body.SetText(body)
}

// CreateRenderer implements fyne.Widget.
func (b *NoticeBar) CreateRenderer() fyne.WidgetRenderer {
	th := b.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	b.bg = canvas.NewRectangle(fadeColor(th.Color(theme.ColorNameWarning, v), noticeBarBgAlpha))
	b.bg.CornerRadius = th.Size(theme.SizeNameSelectionRadius)

	// Кнопки — СПРАВА и по вертикали по центру: они действия над всей
	// плашкой. VBox со спейсерами вместо NewCenter — у Center min-height
	// равна высоте кнопок, и на однострочной плашке она задирала бы полосу.
	buttons := make([]fyne.CanvasObject, 0, 2)
	if b.action != nil {
		buttons = append(buttons, b.action)
	}
	if b.close != nil {
		buttons = append(buttons, b.close)
	}
	var right fyne.CanvasObject
	if len(buttons) > 0 {
		right = container.NewVBox(
			layout.NewSpacer(),
			container.NewHBox(buttons...),
			layout.NewSpacer(),
		)
	}

	// Заголовок сверху, текст под ним; кнопки — правый край всей полосы.
	text := container.NewVBox(b.title, b.body)
	body := container.NewBorder(nil, nil, nil, right, text)
	content := container.NewStack(b.bg, container.NewPadded(body))
	return widget.NewSimpleRenderer(content)
}

// Refresh перечитывает тему (смена варианта светлая/тёмная).
func (b *NoticeBar) Refresh() {
	if b.bg != nil {
		th := b.Theme()
		v := fyne.CurrentApp().Settings().ThemeVariant()
		b.bg.FillColor = fadeColor(th.Color(theme.ColorNameWarning, v), noticeBarBgAlpha)
		b.bg.CornerRadius = th.Size(theme.SizeNameSelectionRadius)
		b.bg.Refresh()
	}
	b.BaseWidget.Refresh()
}

// fadeColor приглушает цвет темы до заданной альфы.
//
// NRGBA, а не подмешивание к фону: фон у окна свой в каждой теме, и складывать
// цвета вручную значило бы угадывать его.
func fadeColor(c color.Color, alpha uint8) color.NRGBA {
	r, g, bl, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: alpha}
}
