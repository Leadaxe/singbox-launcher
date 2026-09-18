// File section.go — секция «Предупреждения» в карточке/окне узла
// (SPEC 131 §6, волна W3).
//
// Секция одна на все окна узла: и на `ui/servers_node_info.go` (окно Info
// вкладки Servers и окна Core runtime), и на окна источника. Разъехаться им
// нельзя — это одни и те же коды об одном и том же узле.
//
// # Вёрстка
//
// Текст кода приезжает из реестра и длину имеет произвольную, поэтому каждый
// `Label` обязан нести `fyne.TextWrapWord`: без него Fyne берёт ширину одной
// строки за min-width виджета, та переопределяет `Resize`, и окно раздувается
// на весь экран (Л19, память `fyne-label-minwidth-trap`).
//
// «Подробнее» — кнопка-ссылка по образцу `supportLinkButton`: собственный
// `platform.OpenURL` за гейтом `urlsafe`, а не `fyne.CurrentApp().OpenURL`
// (его в проекте нет вовсе).
package nodewarn

import (
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

// SectionTitleText — заголовок секции.
const SectionTitleText = "Warnings" // l10n-key

// DetailsButtonText — подпись кнопки на якорь документации.
const DetailsButtonText = "Details" // l10n-key

// CauseLabelText — подпись строки с причиной.
const CauseLabelText = "Why it happens:" // l10n-key

// FixLabelText — подпись списка решений.
const FixLabelText = "What you can do:" // l10n-key

// InfoGroupTitleText — подзаголовок группы info-кодов.
const InfoGroupTitleText = "For your information" // l10n-key

// Section собирает секцию «Предупреждения» для окна узла.
//
// Порядок — error → warning → info: сверху то, из-за чего узел не работает,
// снизу то, о чём просто стоит знать.
//
// Группа info отделена подзаголовком и рисуется БЕЗ глифов: у её записей нет
// призыва к действию, и знак рядом с каждой читался бы как «тут тоже беда».
// Когда у узла кроме info ничего нет, секция состоит из одной этой группы, а
// заголовок «Предупреждения» не показывается вовсе — предупреждать не о чем.
//
// nil при пустом списке — вызывающий не добавляет ни заголовка, ни
// разделителя: секция, за которой ничего нет, обещала бы проблему там, где
// её нет.
func Section(in []state.NodeWarning) fyne.CanvasObject {
	errs, warns, infos := byLevel(Describe(in))
	if len(errs)+len(warns)+len(infos) == 0 {
		return nil
	}
	items := make([]fyne.CanvasObject, 0, len(errs)+len(warns)+len(infos)+2)

	if len(errs)+len(warns) > 0 {
		mark := WarnMark
		if len(errs) > 0 {
			mark = ErrorMark
		}
		head := widget.NewLabel(mark + " " + locale.T(SectionTitleText))
		head.TextStyle.Bold = true
		head.Wrapping = fyne.TextWrapWord
		head.Importance = widget.WarningImportance
		items = append(items, head)

		for _, t := range errs {
			items = append(items, row(t, MarkOf(t.Severity)))
		}
		for _, t := range warns {
			items = append(items, row(t, MarkOf(t.Severity)))
		}
	}

	if len(infos) > 0 {
		head := widget.NewLabel(locale.T(InfoGroupTitleText))
		head.TextStyle.Bold = true
		head.Wrapping = fyne.TextWrapWord
		head.Importance = widget.LowImportance
		items = append(items, head)
		for _, t := range infos {
			// Без глифа: см. комментарий к Section.
			items = append(items, row(t, ""))
		}
	}
	return container.NewVBox(items...)
}

// row — одно предупреждение: заголовок с путём поля, объяснение и «Подробнее».
//
// Путь приписан к ЗАГОЛОВКУ, а не отдельной строкой: он отвечает на «где», и
// в отрыве от «что случилось» читается как технический мусор. У кодов уровня
// узла его просто нет.
//
// mark — глиф уровня перед заголовком; пустой у info-группы, где знак не
// нужен. Маркер «•» остаётся у всех: он держит список списком.
func row(t Text, mark string) fyne.CanvasObject {
	title := t.Title
	if mark != "" {
		title = mark + " " + title
	}
	if t.Path != "" {
		title += "  ·  " + t.Path
	}
	titleLabel := widget.NewLabel("• " + title)
	titleLabel.TextStyle.Bold = true
	titleLabel.Wrapping = fyne.TextWrapWord

	bodyLabel := widget.NewLabel(t.Body)
	bodyLabel.Wrapping = fyne.TextWrapWord
	bodyLabel.Importance = widget.MediumImportance

	rows := []fyne.CanvasObject{titleLabel, bodyLabel}

	// Причина и решения — за объяснением, в том же порядке, что и в
	// документации: What happened (Body) → Why it happens → What you can do.
	if t.Cause != "" {
		rows = append(rows, caption(locale.T(CauseLabelText)), wrapped(t.Cause))
	}
	if len(t.Fixes) > 0 {
		rows = append(rows, caption(locale.T(FixLabelText)))
		for _, f := range t.Fixes {
			if f == "" {
				continue
			}
			rows = append(rows, wrapped("— "+f))
		}
	}

	if btn := detailsButton(t); btn != nil {
		// Кнопка прижата влево спейсером: растянутая на всю ширину, она
		// читалась бы как действие над всем окном, а не над этой строкой.
		rows = append(rows, container.NewHBox(btn, layout.NewSpacer()))
	}
	return container.NewVBox(rows...)
}

// caption — подпись блока внутри строки предупреждения.
//
// Отдельный виджет, а не приписка к тексту: подпись должна отличаться от
// самого текста, а рисовать её жирным наравне с заголовком кода значило бы
// спорить с ним за внимание.
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
