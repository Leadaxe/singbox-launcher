// File section.go — раздел «Уведомления» в окне узла (SPEC 131 §6;
// переделка по утверждённому дизайну владельца 18.09.2026).
//
// Раздел один на все окна узла: и на `ui/servers_node_info.go` (окно Info
// вкладки Servers и окна Core runtime), и на окно источника конфигуратора
// (вкладка Settings). Разъехаться им нельзя — это одни и те же коды об одном
// и том же узле.
//
// # Что поменялось и почему
//
// Прежняя вёрстка была ПЛОСКОЙ: заголовок «Предупреждения», под ним подряд
// все коды, у каждого — заголовок, объяснение, «Почему так бывает», «Что
// можно сделать» и кнопка. Три кода разворачивали окно в простыню, по которой
// приходилось скроллить, чтобы узнать хотя бы СКОЛЬКО их и какого уровня. И
// стоял раздел ПЕРВЫМ блоком — то есть настройки узла, ради которых окно
// открывают чаще, оказывались под ним.
//
// Теперь:
//
//   - раздел уехал ВНИЗ, под все настройки, за разделитель;
//   - шапка отвечает на «сколько и чего» одной строкой: ✖ N · ⚠ N · ⓘ N,
//     нулевые уровни молчат;
//   - коды разложены по трём подразделам (Errors → Warnings → Info), каждый
//     со своим знаком и цветом; пустой подраздел не рисуется;
//   - каждое уведомление — СВЁРНУТАЯ строка аккордеона: сперва человек видит
//     список того, что случилось, и разворачивает только нужное.
//
// # Вёрстка: две ловушки ширины
//
//  1. Текст кода приезжает из реестра и длину имеет произвольную, поэтому
//     каждый `Label` обязан нести `fyne.TextWrapWord`: без него Fyne берёт
//     ширину одной строки за min-width виджета, та переопределяет `Resize`, и
//     окно раздувается на весь экран (Л19, память `fyne-label-minwidth-trap`).
//
//  2. Заголовок `AccordionItem` — это `widget.Button` (см. accordionRenderer),
//     а у кнопки нет ни `Wrapping`, ни `Truncation`: её min-width равна полной
//     ширине текста, и `accordionRenderer.MinSize` берёт МАКСИМУМ по всем
//     заголовкам. Один длинный заголовок раздул бы окно ровно так же, как
//     Label без Wrapping. Поэтому заголовок обрезается по рунам
//     (`accordionTitleMaxRunes`), а `path` в него не идёт вовсе — он уезжает
//     первой строкой внутрь, где Wrapping работает.
//
// «Подробнее» — кнопка-ссылка по образцу `supportLinkButton`: собственный
// `platform.OpenURL` за гейтом `urlsafe`, а не `fyne.CurrentApp().OpenURL`
// (его в проекте нет вовсе).
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package nodewarn

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/urlsafe"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).

// SectionTitleText — заголовок раздела.
//
// «Уведомления», а не «Предупреждения»: раздел собирает ТРИ уровня, и два из
// них ни о чём не предупреждают. Прежнее слово обещало проблему там, где у
// узла одни лишь сведения, — ровно то, от чего уходило разведение уровней.
const SectionTitleText = "Notifications" // l10n-key

// ErrorsGroupTitleText — подзаголовок группы error-кодов.
const ErrorsGroupTitleText = "Errors" // l10n-key

// WarningsGroupTitleText — подзаголовок группы warning-кодов.
const WarningsGroupTitleText = "Warnings" // l10n-key

// InfoGroupTitleText — подзаголовок группы info-кодов.
const InfoGroupTitleText = "For your information" // l10n-key

// DetailsButtonText — подпись кнопки на якорь документации.
const DetailsButtonText = "Details" // l10n-key

// WhatHappenedLabelText — подпись блока с объяснением кода.
const WhatHappenedLabelText = "What happened:" // l10n-key

// CauseLabelText — подпись строки с причиной.
const CauseLabelText = "Why it happens:" // l10n-key

// FixLabelText — подпись списка решений.
const FixLabelText = "What you can do:" // l10n-key

// accordionTitleMaxRunes — предел длины заголовка строки аккордеона.
//
// Ловушка №2 из шапки файла: заголовок `AccordionItem` — кнопка, её min-width
// равна ширине текста целиком, и accordionRenderer берёт максимум по всем
// строкам. Окно узла открывается на 620pt, заголовок стоит с отступами и
// стрелкой — 64 руны укладываются в эту ширину с запасом на кириллицу, а всё
// длиннее получает «…» и читается целиком внутри, где Wrapping работает.
const accordionTitleMaxRunes = 64

// Section собирает раздел «Уведомления» для окна узла.
//
// Порядок — error → warning → info: сверху то, из-за чего узел не работает,
// снизу то, о чём просто стоит знать.
//
// nil при пустом списке — вызывающий не добавляет ни заголовка, ни
// разделителя: раздел, за которым ничего нет, обещал бы проблему там, где её
// нет.
func Section(in []state.NodeWarning) fyne.CanvasObject {
	errs, warns, infos := byLevel(Describe(in))
	if len(errs)+len(warns)+len(infos) == 0 {
		return nil
	}

	items := make([]fyne.CanvasObject, 0, 7)
	items = append(items, sectionHeader(len(errs), len(warns), len(infos)))

	// Порядок групп — ТОТ ЖЕ, что в тултипе и в счётчиках шапки: человек,
	// пришедший сюда по знаку из списка, читает уровни в одном порядке везде.
	if group := levelGroup(errs, ErrorsGroupTitleText, ErrorMark, widget.DangerImportance); group != nil {
		items = append(items, group)
	}
	if group := levelGroup(warns, WarningsGroupTitleText, WarnMark, widget.WarningImportance); group != nil {
		items = append(items, group)
	}
	if group := levelGroup(infos, InfoGroupTitleText, "", widget.LowImportance); group != nil {
		items = append(items, group)
	}
	return container.NewVBox(items...)
}

// sectionHeader — шапка раздела: название слева, счётчики по уровням справа.
//
// Счётчики отвечают на вопрос «сколько и чего», не разворачивая ни одной
// строки: до переделки узнать это можно было только досчитав глазами весь
// список. Нулевой уровень МОЛЧИТ — «✖ 0» сообщал бы об ошибках там, где их
// нет, а место занимал бы наравне с настоящими.
func sectionHeader(errs, warns, infos int) fyne.CanvasObject {
	// БЕЗ Wrapping и с Truncation: заголовок раздела — два слова, переносить
	// их нечего, а `TextWrapWord` даёт min-width в ОДНО слово, и Border,
	// поставив такой Label слева, ужимал его до этой ширины — «Уведомления»
	// обрезалось до «Ув…». Truncation держит ловушку min-width закрытой на
	// случай длинного перевода.
	head := widget.NewLabel(locale.T(SectionTitleText))
	head.TextStyle.Bold = true
	head.Wrapping = fyne.TextWrapOff
	head.Truncation = fyne.TextTruncateEllipsis

	counters := make([]fyne.CanvasObject, 0, 3)
	if errs > 0 {
		counters = append(counters, levelCounter(ErrorMark, errs, widget.DangerImportance))
	}
	if warns > 0 {
		counters = append(counters, levelCounter(WarnMark, warns, widget.WarningImportance))
	}
	if infos > 0 {
		// У info знака-глифа нет (ⓘ U+24D8 нет во встроенных шрифтах Fyne,
		// см. info_icon.go) — счётчик получает ту же ИКОНКУ, что строка
		// списка: человек обязан узнать знак, по которому сюда пришёл.
		counters = append(counters,
			container.NewCenter(InfoIconCell()),
			levelCounter("", infos, widget.LowImportance))
	}
	if len(counters) == 0 {
		return head
	}
	// Заголовок — ЦЕНТР Border'а, счётчики — правый край: центр забирает всё
	// оставшееся место, а `left` получил бы только свой MinSize (см. выше).
	return container.NewBorder(nil, nil, nil, container.NewHBox(counters...), head)
}

// levelCounter — «✖ 3» одним Label'ом своего цвета.
//
// mark пустой у info: его знак рисуется иконкой рядом, и дублировать его
// текстом значило бы поставить два знака на один уровень.
func levelCounter(mark string, n int, imp widget.Importance) *widget.Label {
	// strconv, а не locale.Tf("%d"): голое число переводить нечем, а ключ «%d»
	// в каталоге был бы записью без смысла, за которой пришлось бы следить.
	text := strconv.Itoa(n)
	if mark != "" {
		text = mark + " " + text
	}
	l := widget.NewLabel(text)
	l.Importance = imp
	return l
}

// levelGroup — один подраздел: заголовок со знаком и аккордеон из кодов.
//
// nil на пустом списке: пустой подзаголовок обещал бы строки, которых нет.
//
// mark пустой у info — его заголовок получает иконку слева, ту же, что в
// подстроке строки узла и в счётчике шапки.
func levelGroup(texts []Text, titleKey, mark string, imp widget.Importance) fyne.CanvasObject {
	if len(texts) == 0 {
		return nil
	}
	title := locale.T(titleKey)
	if mark != "" {
		title = mark + " " + title
	}
	head := widget.NewLabel(title)
	head.TextStyle.Bold = true
	head.Wrapping = fyne.TextWrapWord
	head.Importance = imp

	var headCell fyne.CanvasObject = head
	if mark == "" {
		headCell = container.NewBorder(nil, nil,
			container.NewCenter(InfoIconCell()), nil, head)
	}

	// Строки собираются в срез и отдаются КОНСТРУКТОРУ, а не добавляются
	// Append'ом: Append зовёт Refresh, тот — Layout, а Layout меряет заголовок
	// (widget.Button) через тему приложения. На сборке раздела приложение уже
	// есть, а вот в тестах пакета — нет, и Append ронял их nil-разыменованием
	// в Button.CreateRenderer. Конструктор ничего не меряет: первый замер
	// случается, когда раздел уже в окне.
	items := make([]*widget.AccordionItem, 0, len(texts))
	for _, t := range texts {
		// Всё СВЁРНУТО (`Open` по умолчанию false): раздел сперва отвечает
		// «что случилось», списком, и только по клику — «почему и что делать».
		items = append(items, widget.NewAccordionItem(accordionTitle(t), detail(t)))
	}
	acc := widget.NewAccordion(items...)
	// MultiOpen: уведомления независимы, и раскрытие второго не имеет права
	// закрывать первое — человек сравнивает их между собой.
	acc.MultiOpen = true
	return container.NewVBox(headCell, acc)
}

// accordionTitle — заголовок строки аккордеона: ТОЛЬКО заголовок кода,
// обрезанный по длине.
//
// `path` сюда не идёт (он уезжает первой строкой внутрь): заголовок — кнопка
// без переноса, и «заголовок · путь» вдвое приближал бы окно к раздуванию по
// ширине (ловушка №2 в шапке файла).
func accordionTitle(t Text) string {
	return truncateRunes(t.Title, accordionTitleMaxRunes)
}

// truncateRunes обрезает строку по РУНАМ, добавляя многоточие.
//
// По рунам, а не байтам: в подстановках реестра живут теги с эмодзи-флагами, и
// обрезка по байтам разрубила бы их посередине.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max < 1 {
		return ""
	}
	return string(runes[:max-1]) + "…"
}

// detail — содержимое развёрнутой строки: что случилось, почему так бывает,
// что можно сделать, ссылка на документацию.
//
// Порядок — тот же, что в сгенерированной документации контракта: человек,
// нажавший «Подробности», не должен переучиваться, попав на страницу кода.
//
// Между блоками ПУСТЫХ строк нет: `container.NewVBox` и так кладёт
// theme.Padding, а абзацный отступ сверх него растягивал бы три строки текста
// на пол-экрана — ровно то, от чего уходит свёрнутый по умолчанию аккордеон.
func detail(t Text) fyne.CanvasObject {
	rows := make([]fyne.CanvasObject, 0, 8)

	// Путь поля — ПЕРВОЙ строкой, приглушённо: он отвечает на «где», и в
	// заголовке-кнопке ему места нет (см. accordionTitle). У кодов уровня
	// узла его просто нет.
	if t.Path != "" {
		rows = append(rows, caption(t.Path))
	}

	rows = append(rows, caption(locale.T(WhatHappenedLabelText)), wrapped(t.Body))
	if t.Cause != "" {
		rows = append(rows, caption(locale.T(CauseLabelText)), wrapped(t.Cause))
	}
	if len(t.Fixes) > 0 {
		rows = append(rows, caption(locale.T(FixLabelText)))
		for _, f := range t.Fixes {
			if f == "" {
				continue
			}
			rows = append(rows, wrapped("• "+f))
		}
	}
	if btn := detailsButton(t); btn != nil {
		// Кнопка прижата влево спейсером: растянутая на всю ширину, она
		// читалась бы как действие над всем окном, а не над этим кодом.
		rows = append(rows, container.NewHBox(btn, layout.NewSpacer()))
	}
	return container.NewVBox(rows...)
}

// caption — приглушённая подпись блока внутри уведомления.
//
// Отдельный виджет, а не приписка к тексту: подпись должна отличаться от
// самого текста, а рисовать её наравне с ним значило бы спорить с ним за
// внимание.
func caption(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

// wrapped — обычная строка произвольной длины. Wrapping обязателен: без него
// одна длинная строка реестра раздувает окно на весь экран (Л19).
func wrapped(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.MediumImportance
	return l
}

// detailsButton — «Подробнее» на якорь кода в документации контракта.
//
// nil, когда открывать нечего (кода нет) либо схема адреса не прошла гейт:
// кнопка, которая не открывает, хуже её отсутствия.
func detailsButton(t Text) *widget.Button {
	url := t.DocURL
	if url == "" || !urlsafe.IsSafeAnnounceURL(url) {
		return nil
	}
	btn := widget.NewButton(locale.T(DetailsButtonText), func() {
		if err := platform.OpenURL(url); err != nil {
			debuglog.WarnLog("node warning: open %q failed: %v", url, err)
		}
	})
	btn.Importance = widget.LowImportance
	return btn
}
