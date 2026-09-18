// File info_icon.go — как info выглядит в СТРОКЕ узла (правка владельца к
// разведению уровней, SPEC 131 §6).
//
// # Что было и почему поменялось
//
// Первый заход ставил текстовый «(i)» ПОСЛЕ ИМЕНИ узла: глифа «ⓘ» U+24D8 нет
// ни в одном встроенном шрифте Fyne, и рисовать его было нечем. Место выбрано
// неудачно дважды. Во-первых, у имени знак спорит с самим именем — а имя узла
// служит тегом, и всякая приписка к нему читается как часть тега. Во-вторых,
// «(i)» тремя символами весит больше, чем значит: info говорит «всё работает,
// просто знай», и такой знак у имени выглядел пометкой на узле.
//
// Решение владельца: знак переезжает во ВТОРУЮ строку — туда, где узел
// описывает себя («vless·tcp·Reality+Vision»), — и рисуется не шрифтом, а
// иконкой темы. `theme.InfoIcon()` — это и есть «i в кружке», SVG из ресурсов
// Fyne: он одинаков на всех платформах, включая Win7-сборку, где на подбор
// системного шрифта по руне рассчитывать нельзя.
//
// # Где в строке
//
//   - у узла БЕЗ проблем (все коды info) подстрока держит обычный состав, и
//     иконка встаёт ПЕРЕД ним: она комментирует эту строку целиком;
//   - у узла с error/warning подстрока уже занята («✖/⚠ заголовок +N»), и
//     иконка уходит в КОНЕЦ: спорить с главным знаком за начало строки она не
//     вправе — сперва «что не так», потом «и ещё есть что знать».
//
// Текстовый `InfoMark` остался ровно в одном месте — тултипе строки
// (`ToolTip`): тултип Fyne это строка, виджет в неё не вставить.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package nodewarn

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/state"
)

// InfoIconSize — сторона квадрата под иконкой info в строке узла.
//
// Подстрока во всех трёх списках набрана 10pt (serversSubtitleTextSize,
// previewSubtitleTextSize — они совпадают по построению), и иконка обязана
// читаться как часть ЭТОЙ строки: взятая в «нормальный» размер темы (~20pt),
// она стала бы выше подстроки и подняла бы высоту строки списка, а высота
// строки в widget.List берётся с ПЕРВОГО созданного элемента и отзывается на
// весь список.
//
// 12 = кегль подстроки плюс воздух на скругление кружка. Число, а не
// theme.CaptionTextSize(): размеры подстроки в проекте заданы константами, и
// привязка иконки к другому источнику дала бы разъезд при первой же правке
// одного из двух.
const InfoIconSize float32 = 12

// infoIconGap — зазор между иконкой и текстом подстроки: ширина пробела
// кегля подстроки. tightSubtitleLayout своих отступов не добавляет (в отличие
// от HBox), поэтому зазор положительный — иначе кружок слипается с первой
// буквой («ⓘvless»).
const infoIconGap float32 = 4

// InfoIcon — «i в кружке» приглушённым цветом подстроки.
//
// Ресурс темы, а не глиф шрифта: «ⓘ» U+24D8 нет ни в одном встроенном шрифте
// Fyne (см. шапку nodewarn.go), а SVG из theme рисуется везде.
//
// Цвет — ColorNamePlaceHolder, тот же, которым тема красит подстроку строки
// узла: иконка цвета переднего плана спорила бы с именем узла за внимание, а
// info не вправе быть заметнее того, что она комментирует.
func InfoIcon() fyne.Resource {
	return theme.NewColoredResource(theme.InfoIcon(), theme.ColorNamePlaceHolder)
}

// InfoIconCell — иконка info в ячейке фиксированного размера.
//
// Размер задаёт ЯЧЕЙКА, а не виджет: widget.Icon отдаёт свой MinSize из темы
// (иконка «нормального» размера), и вложенный в строку он раздул бы её.
// GridWrap на InfoIconSize×InfoIconSize зажимает знак ровно под кегль
// подстроки.
//
// Одна функция на все поверхности: три списка рисуют один и тот же знак, и
// три своих размера читались бы как три разных знака.
func InfoIconCell() fyne.CanvasObject {
	return container.NewGridWrap(
		fyne.NewSize(InfoIconSize, InfoIconSize),
		widget.NewIcon(InfoIcon()),
	)
}

// InfoSubtitleLine — подстрока узла с двумя местами под иконку info.
//
// Строится ОДИН раз в шаблоне строки списка (createItem), а на каждом
// обновлении вызывается только Update: widget.List переиспользует строки, и
// пересборка контейнера на каждый update стоила бы полной перевёрстки списка
// на каждой прокрутке — плюс порвала бы разбор дерева, которым updateItem
// достаёт свои виджеты.
//
// Обе иконки живут в дереве ВСЕГДА и лишь прячутся: Show/Hide не меняет состав
// контейнера, а tightSubtitleLayout пропускает скрытые объекты и в MinSize, и
// в Layout — места невидимый знак не занимает.
type InfoSubtitleLine struct {
	// Content — готовый контейнер: вставляется туда, где раньше стоял один
	// текст подстроки.
	Content fyne.CanvasObject

	lead  fyne.CanvasObject
	trail fyne.CanvasObject
}

// NewInfoSubtitleLine оборачивает объект подстроки (canvas.Text у списков) в
// строку с местами под иконку слева и справа.
func NewInfoSubtitleLine(text fyne.CanvasObject) *InfoSubtitleLine {
	l := &InfoSubtitleLine{lead: InfoIconCell(), trail: InfoIconCell()}
	l.lead.Hide()
	l.trail.Hide()
	l.Content = container.New(tightSubtitleLayout{gap: infoIconGap}, l.lead, text, l.trail)
	return l
}

// BindInfoSubtitleLine восстанавливает управление строкой по её контейнеру.
//
// Нужен widget.List: шаблон строки (createItem) и её обновление (updateItem) —
// разные замыкания, и ссылку на объект из первого во второе передать нельзя —
// строк-то много, а шаблон один. updateItem разбирает дерево виджетов и здесь
// говорит «этот контейнер — та самая подстрока».
//
// nil, когда контейнер не наш: дерево разбирается по индексам, и молча
// принять чужой объект значило бы прятать иконку у случайного виджета.
func BindInfoSubtitleLine(o fyne.CanvasObject) *InfoSubtitleLine {
	c, ok := o.(*fyne.Container)
	if !ok || len(c.Objects) != 3 {
		return nil
	}
	if _, ok := c.Layout.(tightSubtitleLayout); !ok {
		return nil
	}
	return &InfoSubtitleLine{Content: c, lead: c.Objects[0], trail: c.Objects[2]}
}

// infoIconSides — где стоять иконке info: перед текстом подстроки, после него
// или нигде.
//
// Отдельная ЧИСТАЯ функция, а не `if` внутри Update: развилка повторяет
// правило Subtitle («начало подстроки принадлежит старшему уровню»), и
// разъехаться им нельзя — значит она обязана быть проверяемой без сборки
// виджетов.
func infoIconSides(in []state.NodeWarning) (lead, trail bool) {
	if !HasInfo(in) {
		return false, false
	}
	// Подстрока занята «✖/⚠ заголовок +N» — знак уходит в конец: спорить с
	// главным знаком за начало строки info не вправе.
	if HasProblems(in) {
		return false, true
	}
	return true, false
}

// Update ставит иконку на своё место по кодам узла.
func (l *InfoSubtitleLine) Update(in []state.NodeWarning) {
	if l == nil {
		return
	}
	lead, trail := infoIconSides(in)
	if lead {
		l.lead.Show()
	} else {
		l.lead.Hide()
	}
	if trail {
		l.trail.Show()
	} else {
		l.trail.Hide()
	}
}

// tightSubtitleLayout — иконка и текст в одну строку, без театрального
// воздуха HBox и с вертикальным центрированием по самому высокому элементу.
//
// Свой layout, а не container.NewHBox: HBox кладёт theme.Padding между
// элементами и тянет каждый на полную высоту строки, из-за чего 12pt-иконка
// растягивалась бы и съезжала относительно базовой линии текста.
type tightSubtitleLayout struct {
	gap float32
}

// MinSize — сумма ширин видимых элементов с зазорами, высота по самому
// высокому.
func (l tightSubtitleLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	visible := 0
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		m := o.MinSize()
		w += m.Width
		if m.Height > h {
			h = m.Height
		}
		visible++
	}
	if visible > 1 {
		w += l.gap * float32(visible-1)
	}
	if w < 0 {
		w = 0
	}
	return fyne.NewSize(w, h)
}

// Layout расставляет слева направо; каждый элемент на своей минимальной
// высоте, по центру строки.
func (l tightSubtitleLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var x float32
	first := true
	for _, o := range objects {
		if o == nil || !o.Visible() {
			continue
		}
		m := o.MinSize()
		if !first {
			x += l.gap
		}
		o.Resize(m)
		y := (size.Height - m.Height) / 2
		if y < 0 {
			y = 0
		}
		o.Move(fyne.NewPos(x, y))
		x += m.Width
		first = false
	}
}
