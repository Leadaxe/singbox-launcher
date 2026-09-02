// File source_folder_drill_window.go — арифметика ОКНА строк для больших
// контейнеров (см. раздел «Окно строк на больших контейнерах» в шапке
// source_folder_drilldown.go).
//
// Здесь только чистые функции: ни одного виджета, ни одного обращения к
// Fyne. Причина ровно та, по которой отдельным файлом вынесена
// moveNodeWithinSlice: диапазоны и высоты распорок — единственное в этой
// затее, что можно перепутать молча (окно съедет на строку, распорка соврёт
// на один отступ, и список поедет при прокрутке), а проверяться это обязано
// таблицей без экрана.
package tabs

const (
	// folderDrillWindowThreshold — с какого размера состава включается окно.
	//
	// До порога рисуем ВСЁ, как раньше, до единой строки: у типовой папки на
	// десяток-другой узлов окно ничего не ускорит, зато добавит распорки,
	// перестройку по прокрутке и целый класс расхождений «а почему у меня
	// список дёргается». Порог берёт на себя всю несовместимость: ниже него
	// поведение обязано быть байт-в-байт прежним.
	folderDrillWindowThreshold = 250

	// folderDrillWindowMargin — запас строк над и под видимой областью.
	//
	// Строится больше, чем видно, чтобы обычная прокрутка колесом попадала в
	// уже построенное: перестройка окна на каждый тик колеса стоила бы
	// дороже, чем сотня лишних строк в памяти.
	folderDrillWindowMargin = 100

	// folderDrillWindowSlack — насколько близко к краю окна должна подойти
	// видимая область, чтобы окно пересдвинулось.
	//
	// Меньше margin намеренно: между «пора двигать» и «уже видно пустоту»
	// остаётся запас в (margin - slack) строк, за который перестройка успеет
	// произойти. Равенство порогов давало бы перестройку ровно в тот момент,
	// когда край окна уже въехал в экран.
	folderDrillWindowSlack = 30
)

// folderDrillWindowRange — какие строки строить при данном положении экрана.
//
// total — сколько строк в составе, firstVisible/lastVisible — номера строк,
// попадающих в видимую область (lastVisible включительно), margin — запас.
// Результат обрезан в [0, total) и полуоткрыт: [start, end).
//
// Отрицательный/перевёрнутый вход не считается ошибкой вызывающего: видимый
// диапазон приходит из Offset.Y и Size().Height, а те бывают нулевыми до
// первой раскладки и отрицательными на «оттяжке» прокрутки. Тогда окно
// строится от начала — это ровно то, что нужно показать на первом кадре.
func folderDrillWindowRange(total, firstVisible, lastVisible, margin int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if margin < 0 {
		margin = 0
	}
	if firstVisible > lastVisible {
		firstVisible, lastVisible = 0, 0
	}
	start := firstVisible - margin
	if start < 0 {
		start = 0
	}
	if start > total-1 {
		start = total - 1
	}
	end := lastVisible + margin + 1
	if end > total {
		end = total
	}
	if end <= start {
		end = start + 1
	}
	return start, end
}

// folderDrillSpacerHeights — высоты верхней и нижней распорок окна.
//
// Задача распорок одна: позиции ПОСТРОЕННЫХ строк и общая высота содержимого
// обязаны совпасть с полной отрисовкой, иначе смещение прокрутки после
// перестройки указывает не туда, а полоса прокрутки прыгает.
//
// Раскладка VBox (layout/boxlayout.go): каждый видимый элемент занимает свою
// MinSize().Height, между соседями — ровно один padding. Значит при полной
// отрисовке строка i стоит на y = i*pitch, где pitch = rowHeight + padding, а
// всё содержимое высотой total*rowHeight + (total-1)*padding.
//
// В окне первым элементом идёт верхняя распорка, и она СЪЕДАЕТ ОДИН padding
// между собой и первой строкой. Отсюда:
//
//	top = winStart*pitch - padding = winStart*rowHeight + (winStart-1)*padding
//
// (при winStart == 0 распорки сверху нет вовсе — top = 0). Симметрично снизу:
// hidden = total - winEnd скрытых строк дают hidden*rowHeight + hidden*padding
// минус один padding, который берёт на себя стык распорки с последней
// построенной строкой:
//
//	bottom = hidden*pitch - padding = hidden*rowHeight + (hidden-1)*padding
//
// Проверка суммой: top + padding + (win*rowHeight + (win-1)*padding) +
// padding + bottom = total*rowHeight + (total-1)*padding — та же высота, что
// у полной отрисовки, при любом положении окна.
//
// Ноль означает «распорка не нужна»: вызывающий её тогда не добавляет, иначе
// пустой элемент внёс бы в VBox лишний padding.
func folderDrillSpacerHeights(winStart, winEnd, total int, rowHeight, padding float32) (float32, float32) {
	if total <= 0 || winEnd <= winStart {
		return 0, 0
	}
	if winStart < 0 {
		winStart = 0
	}
	if winEnd > total {
		winEnd = total
	}
	pitch := rowHeight + padding
	top := float32(0)
	if winStart > 0 {
		top = float32(winStart)*pitch - padding
	}
	bottom := float32(0)
	if hidden := total - winEnd; hidden > 0 {
		bottom = float32(hidden)*pitch - padding
	}
	if top < 0 {
		top = 0
	}
	if bottom < 0 {
		bottom = 0
	}
	return top, bottom
}

// folderDrillWindowNeedsShift — подошла ли видимая область к краю окна.
//
// Двигаем только тогда, когда с ЭТОЙ стороны у окна ещё есть, что показать:
// у самого начала состава окно уже стоит на нуле, и перестраивать его от
// подошедшего к краю экрана бессмысленно — получилось бы то же самое окно, но
// с пересозданными виджетами и сброшенной группой перетаскивания.
//
// slack — в строках. Ноль и меньше = «двигать, только когда край уже виден».
func folderDrillWindowNeedsShift(winStart, winEnd, total, firstVisible, lastVisible int, slack int) bool {
	if total <= 0 || winEnd <= winStart {
		return true
	}
	if slack < 0 {
		slack = 0
	}
	if winStart > 0 && firstVisible-slack < winStart {
		return true
	}
	if winEnd < total && lastVisible+slack >= winEnd {
		return true
	}
	return false
}

// folderDrillVisibleRows — какие строки видны при данном смещении и высоте
// области прокрутки.
//
// pitch считается вызывающим по измеренной строке; нулевой или отрицательный
// шаг (строку ещё не мерили) даёт первый экран — до раскладки другого честного
// ответа нет.
//
// lastVisible включительный и не обрезается по total: обрезку делает
// folderDrillWindowRange, и держать её в одном месте дешевле, чем в двух.
func folderDrillVisibleRows(offsetY, viewHeight, pitch float32) (int, int) {
	if pitch <= 0 {
		return 0, 0
	}
	if offsetY < 0 {
		offsetY = 0
	}
	if viewHeight < 0 {
		viewHeight = 0
	}
	first := int(offsetY / pitch)
	last := int((offsetY + viewHeight) / pitch)
	if last < first {
		last = first
	}
	return first, last
}
