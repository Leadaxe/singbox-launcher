// File category_row.go — строка категории компактной формы отбора:
//
//	│ Подпись │ [!] │ содержимое (поток чипов, поле ввода, …) │
//
// Раскладка родилась в окне фильтров вкладки Servers и повторяется теперь в
// эмодзи-пикере формы Направления. Общий вид у них не «похожий», а ОДИН: обе
// формы отбирают узлы по значку и регулярке, и разъехавшиеся колонки подписей
// читались бы как два разных инструмента.
//
// Почему здесь, а не в пакете ui. Пикер живёт в ui/configurator/
// outbounds_configurator, и импортировать пакет ui оттуда нельзя — ui сам
// тянет конфигуратор. internal/fynewidget — единственное место, видимое обоим.
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

	"singbox-launcher/internal/locale"
)

// Метрики компактной формы отбора.
const (
	// CategoryLabelWidth — ширина колонки подписей категорий. Под самое длинное
	// слово формы («Protocol»/«Untested») с запасом на перевод.
	CategoryLabelWidth = 74
	// CategoryInvertSlotWidth — ширина ячейки чипа «!» (и распорки на её месте
	// у категорий без инверсии).
	CategoryInvertSlotWidth = 18
	// ChipRowHeight — высота ряда чипов с вертикальным зазором; на ней считается
	// потолок высоты потоков.
	ChipRowHeight = 24
)

// CategoryInvertTooltipText — подсказка чипа «!». Длинный текст локализации:
// ключ = английская строка (SPEC 111).
const CategoryInvertTooltipText = "Invert this category: keep the nodes it does NOT select."

// CategoryRow — строка категории: «Подпись │ [!] │ содержимое».
//
// Чип инверсии стоит ПОСЛЕ подписи, перед содержимым (решение владельца). Когда
// инверсии у категории нет (invert == nil), его место занимает распорка той же
// ширины — иначе подписи категорий съехали бы влево относительно остальных.
//
// firstLineHeight — высота первой строки содержимого, если она выше ряда чипов
// (строка с полем ввода): подпись и «!» центрируются по ней, а не по всему
// блоку из нескольких рядов.
func CategoryRow(invert *Chip, title string, content fyne.CanvasObject, firstLineHeight ...float32) *fyne.Container {
	h := float32(ChipRowHeight)
	if len(firstLineHeight) > 0 && firstLineHeight[0] > h {
		h = firstLineHeight[0]
	}
	var slot fyne.CanvasObject
	if invert != nil {
		slot = invert
	} else {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(CategoryInvertSlotWidth, 0))
		slot = spacer
	}
	label := canvas.NewText(title, theme.Color(theme.ColorNameForeground))
	label.TextSize = theme.Size(theme.SizeNameCaptionText)
	labelCol := container.NewGridWrap(
		fyne.NewSize(CategoryLabelWidth, h),
		container.NewVBox(layout.NewSpacer(), label, layout.NewSpacer()),
	)
	slotCol := container.NewGridWrap(
		fyne.NewSize(CategoryInvertSlotWidth+4, h),
		container.NewCenter(slot),
	)
	lead := container.NewHBox(labelCol, slotCol)
	return container.NewBorder(nil, nil, container.NewVBox(lead), nil, content)
}

// CategoryIndentedRow — строка без подписи, выровненная под колонку содержимого
// (строка ошибки регулярки под полем ввода).
func CategoryIndentedRow(content fyne.CanvasObject) fyne.CanvasObject {
	pad := canvas.NewRectangle(color.Transparent)
	pad.SetMinSize(fyne.NewSize(CategoryInvertSlotWidth+4+CategoryLabelWidth+theme.Padding(), 0))
	return container.NewBorder(nil, nil, pad, nil, content)
}

// HintText — мелкий приглушённый текст (подпись «ms», точка между наборами
// чипов, «эмодзи не найдены»).
//
// canvas.Text, а не widget.Label: у Label минимальная ширина считается по его
// единственной строке и раздувает окно (память `fyne-label-minwidth-trap`), да
// и кегль ему пришлось бы переопределять темой.
func HintText(text string) *canvas.Text {
	t := canvas.NewText(text, theme.Color(theme.ColorNameDisabled))
	t.TextSize = theme.Size(theme.SizeNameCaptionText)
	return t
}

// NewInvertChip — чип «!» категории с канонической подсказкой.
func NewInvertChip(initial bool, onChange func(bool)) *Chip {
	c := NewChip("!", initial, onChange)
	c.SetToolTip(locale.T(CategoryInvertTooltipText))
	return c
}
