// File source_node_row.go — ОДИН шаблон строки узла для всех списков
// (обкатка SPEC 116, заход 3: «шаблон строки внутри папки и на root-уровне
// должен быть один и правиться из одного места»).
//
// # Что было
//
// Строку узла рисовали ДВА места своей вёрсткой: список Sources в корне
// (source_tab.go) и drill-down контейнера (source_folder_drilldown.go). Они
// разъехались ровно так, как и должны были разъехаться две копии: в корне
// заголовок — `ttwidget.Label` с обрезкой и тултипом, у узла нет карандаша
// (клик по строке открывает то же окно), справа только корзина; в папке —
// пара `canvas.Text`, карандаш И корзина, а клик по строке не делал ничего.
// Эталоном принят КОРЕНЬ (решение обкатки), и живёт он теперь здесь.
//
// # Расклад строки
//
//		[захват] [галка] Имя…                                   [кнопки] [gutter]
//		                 подстрока (протокол·транспорт·security | ⚠ причина)
//
//	  - захват — только там, где порядок принадлежит пользователю (папка); у
//	    подписки его нет: порядок задаёт тело провайдера, и перестановка
//	    потерялась бы на первом же fetch. На его месте распорка той же ширины,
//	    чтобы колонка галок не съезжала между видами;
//	  - галка — включённость узла; у неразобранной записи выключена (собирать
//	    из неё нечего);
//	  - клик по строке открывает окно узла — как в корне;
//	  - кнопки: у узла ПАПКИ только корзина (карандаш дублировал бы клик), у
//	    узла ПОДПИСКИ кнопок нет вовсе — состав принадлежит провайдеру, и
//	    всякая правка отменится следующим обновлением (обкатка: «в подписках
//	    надо все кнопки отключать, оставлять только галки»);
//	  - gutter — ширина полосы прокрутки: она рисуется ПОВЕРХ строк, и без
//	    резерва ложилась бы на кнопки.
package tabs

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
)

// sourceNodeRowSpec — всё, что отличает одну строку узла от другой.
//
// Намеренно данные, а не поведение: два списка отличаются адресом узла и
// правами над ним, а не вёрсткой. Всё, что здесь не названо, у обоих списков
// одинаково — и остаётся одинаковым при любой правке этого файла.
type sourceNodeRowSpec struct {
	// Title / Subtitle — что показывать; Subtitle пустой = строки нет.
	Title    string
	Subtitle string
	// SubtitleWarn — подстрока это ПРИЧИНА отбраковки, а не описание узла:
	// красится цветом предупреждения.
	SubtitleWarn bool
	// Dimmed — узел выключен либо неразобран: заголовок гаснет.
	Dimmed bool
	// ToolTip — полный текст под курсором; пусто = тултипа нет.
	ToolTip string

	// Checked / CheckDisabled — состояние галки включённости.
	Checked        bool
	CheckDisabled  bool
	OnCheckChanged func(bool)

	// Drag — захват перетаскивания; nil = на его месте распорка.
	Drag fyne.CanvasObject

	// OnOpen — клик по строке (окно узла); nil = строка не кликается.
	OnOpen func()
	// OnMenu — правый клик; nil = меню нет.
	OnMenu func(*fyne.PointEvent)
	// OnDelete — корзина; nil = кнопки нет (узел подписки).
	OnDelete func()
}

// newSourceNodeRow собирает строку узла по спецификации.
//
// Возвращает готовый виджет и сам HoverRow: вызывающему он нужен для
// регистрации в группе перетаскивания и для проброса hover с кнопок.
func newSourceNodeRow(spec sourceNodeRowSpec) (fyne.CanvasObject, *fynewidget.HoverRow) {
	var row *fynewidget.HoverRow
	rowGetter := func() *fynewidget.HoverRow { return row }

	// Заголовок — `ttwidget.Label` с обрезкой, как в корне: длинный тег не
	// имеет права раздувать окно (ловушка fyne-label-minwidth-trap), а полный
	// текст доступен тултипом.
	title := ttwidget.NewLabel(spec.Title)
	title.Wrapping = fyne.TextWrapOff
	title.Truncation = fyne.TextTruncateEllipsis
	if spec.Dimmed {
		title.Importance = widget.LowImportance
	}
	if spec.ToolTip != "" {
		title.SetToolTip(spec.ToolTip)
	}

	check := widget.NewCheck("", nil)
	check.SetChecked(spec.Checked)
	if spec.CheckDisabled {
		check.Disable()
	} else if spec.OnCheckChanged != nil {
		check.OnChanged = spec.OnCheckChanged
	}

	// Ведущий кластер: захват ЛЕВЕЕ галки (guideline вкладки Rules). Без
	// захвата на его месте распорка — колонка галок не съезжает между видами
	// контейнеров, а сама галка не липнет к краю списка.
	drag := spec.Drag
	if drag == nil {
		drag = fynewidget.NewDragHandleSpacer()
	}
	leftLead := container.NewHBox(drag, fynewidget.CheckLeadingWrap(check))

	rightItems := make([]fyne.CanvasObject, 0, 2)
	if spec.OnDelete != nil {
		delBtn := fynewidget.NewHoverForwardButtonWithIcon("", theme.DeleteIcon(),
			spec.OnDelete, rowGetter)
		delBtn.Importance = widget.LowImportance
		fynewidget.SetToolTipSafe(delBtn, locale.T("Del"))
		rightItems = append(rightItems, delBtn)
	}
	rightControls := container.NewHBox(
		container.New(tightHBox{spacing: rowIconGap}, rightItems...),
		components.NewScrollGutter(),
	)

	titleRow := container.NewBorder(nil, nil, leftLead, rightControls, title)

	lines := []fyne.CanvasObject{titleRow}
	if spec.Subtitle != "" {
		// Отступ подстроки равен ширине ведущего кластера — тогда она
		// начинается ровно под заголовком. Хардкод здесь уже ломался при
		// смене состава кластера (см. корневой список).
		pad := canvas.NewRectangle(color.Transparent)
		pad.SetMinSize(fyne.NewSize(leftLead.MinSize().Width, 0))

		subColor := theme.Color(theme.ColorNamePlaceHolder)
		if spec.SubtitleWarn {
			subColor = theme.Color(theme.ColorNameWarning)
		}
		sub := canvas.NewText(spec.Subtitle, subColor)
		sub.TextSize = previewSubtitleTextSize
		lines = append(lines, container.NewBorder(nil, nil, pad, nil, sub))
	}

	var inner fyne.CanvasObject = titleRow
	if len(lines) > 1 {
		inner = container.New(tightVBox{}, lines...)
	}

	row = fynewidget.NewHoverRow(inner,
		fynewidget.HoverRowConfig{IsSelected: func() bool { return false }})
	row.WireTooltipLabelHover(title)

	// Обёртка СНАРУЖИ HoverRow — ловушка Fyne: событие достаётся самому
	// глубокому подходящему объекту, и внутри обёртка перехватила бы hover,
	// погасив подсветку строки. Кнопки лежат глубже и свой tap получают сами.
	wrap := fynewidget.NewSecondaryTapWrap(row)
	if spec.OnOpen != nil {
		wrap.OnPrimary = func(fyne.KeyModifier) { spec.OnOpen() }
	}
	if spec.OnMenu != nil {
		wrap.OnSecondary = spec.OnMenu
	}
	return wrap, row
}
