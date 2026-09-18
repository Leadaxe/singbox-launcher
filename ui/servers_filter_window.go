// File servers_filter_window.go — ОКНО фильтров списка узлов вкладки Servers.
// Модель и предикат, которыми оно управляет, живут в servers_filter.go.
//
//	┌─ Filters ────────────────── показано 24 / 312 ─┐
//	│ [!] Regex     [ NL|DE|Proton      ] [⧉] [✕]   │
//	│               ⚠ invalid regex: …               │
//	│     Emoji     [🇩🇪 24][🇳🇱 18][🇺🇸 17][🔥 4] …  │
//	│ [!] Protocol  [vless 8][ss 3] · [ws 5][tcp 6]  │
//	│ [!] Source    [Proton 40][BL: ЧЁРНЫЕ… 12]      │
//	│     Test      [any][ok][error][untested]       │
//	│               ping ≤ [   ] ms                  │
//	│ [ Invert all ]              [ Reset ] [ Close ]│
//	└────────────────────────────────────────────────┘
//
// # Как устроена форма
//
// Категория — ОДНА строка: слева узкая колонка «[!] подпись» (чип инверсии и
// название), справа поток чипов с переносом. Заголовок отдельной строкой,
// сетка в пять колонок и разделители между категориями, которые были здесь
// раньше, давали окно в 950pt шириной, не влезавшее по высоте; чипы при этом
// растягивались на пятую часть ширины каждый — «🇳🇱 1» шириной с абзац.
//
// Чипы — fynewidget.Chip: подпись капитальным кеглем в скруглённой плашке,
// минимальная ширина по тексту. Раскладка — fynewidget.FlowBox, она же
// считает свою высоту под фактическую ширину (см. flow.go).
//
// Колонка подписей фиксированной ширины у ВСЕХ категорий, включая те, у
// которых инверсии нет (Emoji, Test): на месте чипа «!» там пустая распорка
// той же ширины, иначе подписи разъехались бы по горизонтали.
//
// Окно НЕМОДАЛЬНОЕ и отдельное (Application.NewWindow): человек держит его
// открытым и правит отбор, глядя на список, а модальный попап и список, и
// правку заблокировал бы. Оно же и не dialog.Dialog — форма высокая, а Fyne
// раздувает такой попап на весь экран (память `fyne-label-minwidth-trap`).
//
// Экземпляр один на панель: повторное нажатие кнопки поднимает уже открытое
// окно, а не заводит второе, которое писало бы в то же состояние.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/emojitag"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/components"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	filterRegexCopyTooltipText  = "Copy the pattern body (no wrapper) — paste it into a Direction's filter field."
	filterRegexCopiedInvertText = "Pattern body copied. Its invert flag is NOT part of the body: tick Invert in the Direction's filter."
	filterRegexCopiedPlainText  = "Pattern body copied."
	filterInvertCategoryTooltip = "Invert this category: keep the nodes it does NOT select."
	filterInvertAllTooltipText  = "Invert the whole filter: show exactly the rows it hides now."
	filterEmojiChipTooltipText  = "Toggle this emoji in the Regex field (OR pattern)."
	filterNoEmojiText           = "No emoji in the names of the nodes in this list."
	filterKeepsSelectedRowsText = "Selected rows, direct-out and the active proxy are never hidden."
	filterPingTooltipText       = "Show nodes at or below this latency. Untested nodes pass the threshold — it answers «show me the fast ones», not «drop the unmeasured»; nodes whose test failed do not pass."
	// Плейсхолдер поля регулярки — подсказка формата, а не пример-обманка:
	// прежний «🇩🇪|🇳🇱|Proton» читался как уже введённый фильтр, и человек шёл
	// искать, почему список не отобран.
	filterWindowRegexPlaceholder = "regex, e.g. NL|DE|Proton"
)

// serversFilterDebounce — пауза перед компиляцией регулярки (LxBox §048 #4).
//
// Печать идёт посимвольно, и пересчёт среза на каждый символ гонял бы разбор
// 500 узлов и перерисовку списка десятки раз на одно слово.
const serversFilterDebounce = 300 * time.Millisecond

// Метрики компактной формы.
const (
	// filterLabelColWidth — ширина колонки подписей категорий. Под самое
	// длинное слово формы («Transport» ушло, осталось «Protocol»/«Untested»)
	// с запасом на перевод.
	filterLabelColWidth = 74
	// filterInvertSlotWidth — ширина ячейки чипа «!» (и распорки на её месте
	// у категорий без инверсии).
	filterInvertSlotWidth = 18
	// filterEmojiMaxRows — потолок высоты потока эмодзи в рядах чипов.
	// Эмодзи в подписке на 500 узлов бывает под сотню, и без потолка форма
	// уехала бы за край экрана, унося кнопки под док.
	filterEmojiMaxRows = 3
	// filterChipRowHeight — высота ряда чипов с вертикальным зазором; на ней
	// считается потолок потока эмодзи.
	filterChipRowHeight = 24
	// filterPingEntryWidth — ширина поля порога пинга. Это три цифры, а
	// растянутое поле читалось бы как поле ввода текста.
	filterPingEntryWidth = 60
)

// serversFilterWindow — открытое окно фильтров одной панели.
type serversFilterWindow struct {
	win fyne.Window
	// refreshTitle перерисовывает счётчик «показано / всего» по свежим данным.
	refreshTitle func()
	// rebuildChips пересобирает чипы под новый состав списка (смена группы).
	rebuildChips func()
}

// serversFilterHost — что окну нужно от панели списка.
//
// Интерфейсом-структурой, а не ссылкой на панель: панель — замыкание на
// полсотни переменных внутри BuildProxyListPanel, и вытаскивать её наружу
// значило бы вытаскивать их все.
type serversFilterHost struct {
	// State — текущее состояние фильтра (панель владеет им, окно правит).
	State func() *serversFilterState
	// Facets — сводка по текущему списку (кэш панели, не файловый разбор).
	Facets func() serversFilterFacets
	// Apply — пересчитать видимый срез и перерисовать список.
	Apply func()
	// Counts — показано / всего, для заголовка окна.
	Counts func() (shown int, total int)
	// Status — строка статуса панели (подсказки после копирования).
	Status func(string)
	// Closed — окно закрыли: панель обязана забыть экземпляр.
	Closed func()
}

// showServersFilterWindow открывает окно (или поднимает уже открытое).
func showServersFilterWindow(host serversFilterHost, existing *serversFilterWindow) *serversFilterWindow {
	if existing != nil && existing.win != nil {
		existing.refreshTitle()
		existing.rebuildChips()
		existing.win.Show()
		existing.win.RequestFocus()
		return existing
	}
	app := fyne.CurrentApp()
	if app == nil {
		return nil
	}

	fw := &serversFilterWindow{}
	fw.win = app.NewWindow(locale.T("Filters"))

	st := host.State
	// apply — общий хвост любой правки: пересчитать список и обновить счётчик.
	apply := func() {
		host.Apply()
		fw.refreshTitle()
	}

	// ── Regex ──────────────────────────────────────────────────────────────
	regexEntry := widget.NewEntry()
	regexEntry.SetPlaceHolder(locale.T(filterWindowRegexPlaceholder))
	regexEntry.SetText(st().RegexBody)

	// Строка ошибки — мелким кеглем и только когда есть ошибка. Canvas-текст,
	// а не Label: Label без Wrapping меряет свою единственную строку как
	// минимальную ширину и раздувает окно (память `fyne-label-minwidth-trap`),
	// а с Wrapping занимал бы место и пустым.
	regexError := canvas.NewText("", theme.Color(theme.ColorNameError))
	regexError.TextSize = theme.Size(theme.SizeNameCaptionText)
	regexError.Hide()

	var refreshEmojiChips func()
	syncRegexError := func() {
		body := strings.TrimSpace(st().RegexBody)
		if body == "" {
			regexError.Hide()
			return
		}
		if _, err := compileServersFilterRegex(body); err != nil {
			regexError.Text = locale.Tf("Invalid regex: %s", err.Error())
			regexError.Refresh()
			regexError.Show()
			return
		}
		regexError.Hide()
	}

	// Дебаунс живёт на UI-потоке: таймер только БУДИТ его через fyne.Do, а
	// само состояние правится там же, где его читает список.
	var debounce *time.Timer
	scheduleRegexApply := func() {
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(serversFilterDebounce, func() {
			fyne.Do(func() {
				syncRegexError()
				apply()
			})
		})
	}

	regexEntry.OnChanged = func(text string) {
		st().RegexBody = text
		// Подсветка чипов — СРАЗУ, без дебаунса: она отражает текст поля, а не
		// результат компиляции, и отставание на 300 мс читалось бы как «клик
		// не сработал».
		if refreshEmojiChips != nil {
			refreshEmojiChips()
		}
		scheduleRegexApply()
	}

	regexInvert := newFilterInvertChip(st().RegexInvert, func(on bool) {
		st().RegexInvert = on
		apply()
	})

	regexCopy := ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		body := strings.TrimSpace(st().RegexBody)
		if body == "" {
			return
		}
		if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
			app.Clipboard().SetContent(body)
		}
		// Инверсия в тело НЕ входит — ни здесь, ни в поле Направления
		// (configtypes.DirectionFilterPattern держит её префиксом «!», а форма
		// показывает галкой). Молча отдать тело и промолчать про галку значит
		// отдать фильтр, который в Направлении отберёт ровно наоборот.
		if st().RegexInvert {
			host.Status(locale.T(filterRegexCopiedInvertText))
		} else {
			host.Status(locale.T(filterRegexCopiedPlainText))
		}
	})
	regexCopy.Importance = widget.LowImportance
	regexCopy.SetToolTip(locale.T(filterRegexCopyTooltipText))

	regexClear := ttwidget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		regexEntry.SetText("") // OnChanged сам обновит состояние и чипы
	})
	regexClear.Importance = widget.LowImportance
	regexClear.SetToolTip(locale.T("Clear the pattern"))

	regexRow := filterCategoryRow(regexInvert, locale.T("Regex"),
		container.NewBorder(nil, nil, nil,
			container.NewHBox(regexCopy, regexClear),
			regexEntry,
		))

	// ── Потоки чипов ───────────────────────────────────────────────────────
	//
	// Подписи чипов собираются fmt.Sprintf, а не locale.Tf: переводить в них
	// нечего — это значение из данных («vless», «🇩🇪», имя подписки) и число
	// рядом. Прогонять их через словарь значило бы завести там сотни ключей,
	// приезжающих из чужих подписок.
	emojiFlow := fynewidget.NewFlowBox()
	emojiFlow.MaxHeight = filterEmojiMaxRows * filterChipRowHeight
	protocolFlow := fynewidget.NewFlowBox()
	sourceFlow := fynewidget.NewFlowBox()

	// Один чип инверсии на объединённую категорию «Протокол» (протоколы и
	// транспорт в одном потоке) — он переворачивает её итог целиком.
	protocolInvert := newFilterInvertChip(st().ProtocolsInvert, func(on bool) {
		st().ProtocolsInvert = on
		apply()
	})
	sourceInvert := newFilterInvertChip(st().SourcesInvert, func(on bool) {
		st().SourcesInvert = on
		apply()
	})

	refreshEmojiChips = func() {
		facets := host.Facets()
		if len(facets.Emojis) == 0 {
			emojiFlow.SetObjects([]fyne.CanvasObject{
				filterHintText(locale.T(filterNoEmojiText)),
			})
			return
		}
		// Выбранным считается то, что СТОИТ В ПОЛЕ: человек мог набрать терм
		// руками, и чип обязан это показать — иначе клик по нему добавил бы
		// дубль вместо снятия.
		selected := map[string]bool{}
		for _, term := range emojitag.SplitORPattern(st().RegexBody) {
			selected[term] = true
		}
		objs := make([]fyne.CanvasObject, 0, len(facets.Emojis))
		for _, e := range facets.Emojis {
			e := e
			chip := fynewidget.NewChip(fmt.Sprintf("%s %d", e.Emoji, e.Count), selected[e.Emoji], nil)
			chip.OnChanged = func(bool) {
				// Клик = toggle терма в OR-паттерне поля (LxBox :126). Поле
				// остаётся ЕДИНСТВЕННЫМ носителем отбора по имени: второй
				// набор «выбранных эмодзи» рядом с ним разошёлся бы с ним на
				// первой же ручной правке.
				next := emojitag.ToggleInORPattern(st().RegexBody, e.Emoji)
				regexEntry.SetText(next)
			}
			chip.SetToolTip(locale.T(filterEmojiChipTooltipText))
			objs = append(objs, chip)
		}
		emojiFlow.SetObjects(objs)
	}

	// refreshProtocolChips — ОДИН поток на объединённую категорию: сперва
	// протоколы, затем варианты транспорта/безопасности, между наборами —
	// точка-разделитель. Наборы остаются разными (внутри ИЛИ, между ними И),
	// и точка — единственное, что об этом говорит глазу.
	refreshProtocolChips := func() {
		facets := host.Facets()
		objs := make([]fyne.CanvasObject, 0, len(facets.Protocols)+len(facets.Variants)+1)
		for _, f := range facets.Protocols {
			f := f
			objs = append(objs, fynewidget.NewChip(
				fmt.Sprintf("%s %d", f.Key, f.Count),
				st().Protocols[f.Key],
				func(on bool) {
					setChipSelection(st().Protocols, f.Key, on)
					apply()
				}))
		}
		if len(facets.Protocols) > 0 && len(facets.Variants) > 0 {
			objs = append(objs, filterHintText("·"))
		}
		for _, f := range facets.Variants {
			f := f
			objs = append(objs, fynewidget.NewChip(
				fmt.Sprintf("%s %d", f.Key, f.Count),
				st().Variants[f.Key],
				func(on bool) {
					setChipSelection(st().Variants, f.Key, on)
					apply()
				}))
		}
		protocolFlow.SetObjects(objs)
	}

	refreshSourceChips := func() {
		facets := host.Facets()
		objs := make([]fyne.CanvasObject, 0, len(facets.Sources))
		for _, f := range facets.Sources {
			f := f
			chip := fynewidget.NewChip(
				fmt.Sprintf("%s %d", truncateSourceChipName(f.Name), f.Count),
				st().Sources[f.ID],
				func(on bool) {
					setChipSelection(st().Sources, f.ID, on)
					apply()
				})
			chip.SetToolTip(f.Name) // полное имя: в чипе оно обрезано
			objs = append(objs, chip)
		}
		sourceFlow.SetObjects(objs)
	}

	// ── Test ───────────────────────────────────────────────────────────────
	//
	// Радио-чипы вместо widget.RadioGroup: тот занимает высокий отдельный ряд
	// с кружками, а здесь строка обязана стоять в один рост с остальными.
	//
	// Порог пинга — ПЯТЫЙ чип того же взаимоисключающего ряда (решение
	// владельца), а поле числа при нём только уточняет выбранный вариант.
	testChips := newFilterRadioChips(
		[]string{
			locale.T("any"), locale.T("ok"), locale.T("error"),
			locale.T("untested"), locale.T("ping ≤"),
		},
		int(st().Test),
		func(i int) {
			// Через SetTest, а не присваиванием: выбор «error» обязан
			// погасить «глаз» панели, иначе режим показывал бы пустоту
			// (см. servers_filter.go).
			st().SetTest(serversTestMode(i))
			apply()
		})
	testChips.chips[serversTestPing].SetToolTip(locale.T(filterPingTooltipText))

	pingEntry := widget.NewEntry()
	pingEntry.SetText(st().PingText)
	// Не число — поле с ошибкой, а вариант просто не отбирает (PingMaxMs=0).
	// Прятать список из-за недопечатанной цифры нельзя, ровно как у регулярки.
	pingEntry.Validator = func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		if n, err := strconv.Atoi(text); err != nil || n <= 0 {
			return fmt.Errorf("%s", locale.T("milliseconds, e.g. 300"))
		}
		return nil
	}
	pingEntryWrap := container.NewGridWrap(
		fyne.NewSize(filterPingEntryWidth, pingEntry.MinSize().Height), pingEntry)

	// restoring — идёт программное восстановление полей (Reset, смена группы),
	// а не правка человеком.
	//
	// Без этого флага Reset вёл бы себя так: SetText кладёт в поле «300»,
	// OnChanged через 300 мс видит число и «удобно» выбирает вариант «ping ≤»
	// — то есть сброшенный фильтр сам собой включал бы порог.
	restoring := false

	var pingDebounce *time.Timer
	pingEntry.OnChanged = func(text string) {
		st().PingText = text
		if pingDebounce != nil {
			pingDebounce.Stop()
		}
		byHuman := !restoring
		pingDebounce = time.AfterFunc(serversFilterDebounce, func() {
			fyne.Do(func() {
				n, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil || n <= 0 {
					st().PingMaxMs = 0
				} else {
					st().PingMaxMs = n
					// Правка числа САМА выбирает вариант «ping ≤»: человек,
					// который печатает порог, уже сказал, чего хочет, и
					// требовать после этого ещё и клика по чипу значило бы
					// показывать ему неотфильтрованный список молча.
					if byHuman && st().Test != serversTestPing {
						st().SetTest(serversTestPing)
						testChips.set(int(serversTestPing))
					}
				}
				apply()
			})
		})
	}
	fynewidget.SetToolTipSafe(pingEntry, locale.T(filterPingTooltipText))

	// Поле и «ms» идут сразу за чипом «ping ≤» в том же потоке: разрыв между
	// вариантом и его числом читался бы как два независимых элемента.
	testRow := filterCategoryRow(nil, locale.T("Test"),
		fynewidget.NewFlowBox(append(testChips.objects,
			pingEntryWrap, filterHintText(locale.T("ms")))...))

	// ── Нижний ряд ─────────────────────────────────────────────────────────
	invertAll := ttwidget.NewButton(locale.T("Invert all"), nil)
	invertAll.Importance = widget.LowImportance
	syncInvertAll := func() {
		if st().InvertAll {
			invertAll.Importance = widget.HighImportance
		} else {
			invertAll.Importance = widget.LowImportance
		}
		invertAll.Refresh()
	}
	invertAll.OnTapped = func() {
		st().InvertAll = !st().InvertAll
		syncInvertAll()
		apply()
	}
	invertAll.SetToolTip(locale.T(filterInvertAllTooltipText))
	syncInvertAll()

	// Строка Source прячется целиком, когда контейнеров в списке нет: пустая
	// подпись над пустым местом читалась бы как «чипы не загрузились».
	sourceRow := filterCategoryRow(sourceInvert, locale.T("Source"), sourceFlow)

	// rebuildAll — перечитать ВСЁ состояние в виджеты. Нужен и Reset'у, и
	// смене группы: у новой группы свой снимок фильтра, и поля обязаны
	// показать именно его.
	rebuildAll := func() {
		f := st()
		// На время восстановления поля не считаются правкой человека: их
		// OnChanged не должен доводить состояние (см. restoring).
		restoring = true
		defer func() { restoring = false }()
		// SetText зовёт OnChanged, и тот перепишет RegexBody тем же текстом —
		// безобидно, но чипы обновятся до того, как мы дойдём до них ниже.
		regexEntry.SetText(f.RegexBody)
		regexInvert.SetSelected(f.RegexInvert)
		protocolInvert.SetSelected(f.ProtocolsInvert)
		sourceInvert.SetSelected(f.SourcesInvert)
		testChips.set(int(f.Test))
		pingEntry.SetText(f.PingText)
		syncInvertAll()
		syncRegexError()
		refreshEmojiChips()
		refreshProtocolChips()
		refreshSourceChips()
		if len(host.Facets().Sources) == 0 {
			sourceRow.Hide()
		} else {
			sourceRow.Show()
		}
	}

	resetBtn := widget.NewButton(locale.T("Reset"), func() {
		*st() = newServersFilterState()
		rebuildAll()
		apply()
	})
	resetBtn.Importance = widget.LowImportance
	closeBtn := widget.NewButton(locale.T("Close"), func() { fw.win.Close() })

	bottomRow := container.NewBorder(nil, nil, invertAll,
		container.NewHBox(resetBtn, closeBtn),
		layout.NewSpacer(),
	)

	// ── Сборка ─────────────────────────────────────────────────────────────
	//
	// Без разделителей-линий: у категории есть подпись в своей колонке, и
	// линия между строками добавляла бы только высоту.
	form := container.NewVBox(
		regexRow,
		filterErrorRow(regexError),
		filterCategoryRow(nil, locale.T("Emoji"), emojiFlow),
		filterCategoryRow(protocolInvert, locale.T("Protocol"), protocolFlow),
		sourceRow,
		testRow,
		filterKeepsHint(),
	)

	// Канонический gutter проекта (components.WrapInScrollWithGutter): без
	// него полоса прокрутки ложится на правые края чипов и кнопок ⧉/✕.
	content := container.NewBorder(nil, bottomRow, nil, nil,
		components.WrapInScrollWithGutter(container.NewPadded(form)))

	fw.refreshTitle = func() {
		shown, total := host.Counts()
		fw.win.SetTitle(locale.Tf("Filters — %d / %d shown", shown, total))
	}
	fw.rebuildChips = rebuildAll

	rebuildAll()
	fw.refreshTitle()

	// Закрытое окно обязано перестать быть «открытым» для панели: иначе
	// следующее нажатие кнопки поднимало бы мёртвый экземпляр, и фильтр
	// выглядел бы недоступным. Дебаунс-таймеры снимаются здесь же — их
	// отложенный fyne.Do пришёл бы к виджетам закрытого окна. Слой тултипов
	// разрушается тогда же: он держит ссылку на канву закрытого окна.
	fw.win.SetOnClosed(func() {
		if debounce != nil {
			debounce.Stop()
		}
		if pingDebounce != nil {
			pingDebounce.Stop()
		}
		fynetooltip.DestroyWindowToolTipLayer(fw.win.Canvas())
		if host.Closed != nil {
			host.Closed()
		}
	})

	// Слой тултипов обязателен: без него SetToolTip у чипов и кнопок молчит,
	// а полное имя обрезанного источника читается только из него.
	fw.win.SetContent(fynetooltip.AddWindowToolTipLayer(content, fw.win.Canvas()))
	fw.win.Resize(fyne.NewSize(460, 420))
	fynewidget.CenterOnScreen(fw.win)
	fw.win.Show()
	return fw
}

// filterCategoryRow — строка категории: «[!] Подпись │ содержимое».
//
// Чип инверсии стоит СЛЕВА от подписи (решение владельца). Когда инверсии у
// категории нет (invert == nil), его место занимает распорка той же ширины —
// иначе подписи Emoji и Test съехали бы влево относительно остальных.
func filterCategoryRow(invert *fynewidget.Chip, title string, content fyne.CanvasObject) *fyne.Container {
	var slot fyne.CanvasObject
	if invert != nil {
		slot = invert
	} else {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(filterInvertSlotWidth, 0))
		slot = spacer
	}
	label := canvas.NewText(title, theme.Color(theme.ColorNameForeground))
	label.TextSize = theme.Size(theme.SizeNameCaptionText)
	labelCol := container.NewGridWrap(
		fyne.NewSize(filterLabelColWidth, filterChipRowHeight),
		container.NewVBox(layout.NewSpacer(), label, layout.NewSpacer()),
	)
	lead := container.NewHBox(container.NewCenter(slot), labelCol)
	return container.NewBorder(nil, nil, lead, nil, content)
}

// filterErrorRow — строка ошибки регулярки, выровненная под колонку чипов.
func filterErrorRow(errText *canvas.Text) fyne.CanvasObject {
	pad := canvas.NewRectangle(color.Transparent)
	pad.SetMinSize(fyne.NewSize(filterInvertSlotWidth+filterLabelColWidth, 0))
	return container.NewBorder(nil, nil, pad, nil, errText)
}

// filterKeepsHint — единственная оставшаяся в форме подсказка (что фильтр не
// прячет никогда).
//
// Label с Wrapping, а не canvas.Text: строка длинная, и без переноса её
// «единственная строка» стала бы минимальной шириной окна (память
// `fyne-label-minwidth-trap`). Остальные пояснения ушли в тултипы — они не
// влезали и обрезались.
func filterKeepsHint() fyne.CanvasObject {
	l := widget.NewLabel(locale.T(filterKeepsSelectedRowsText))
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	l.TextStyle = fyne.TextStyle{Italic: true}
	return l
}

// filterHintText — мелкий приглушённый текст (подпись «ms», точка между
// наборами чипов).
//
// canvas.Text, а не widget.Label: у Label минимальная ширина считается по его
// единственной строке и раздувает окно (память `fyne-label-minwidth-trap`), да
// и кегль ему пришлось бы переопределять темой.
func filterHintText(text string) *canvas.Text {
	t := canvas.NewText(text, theme.Color(theme.ColorNameDisabled))
	t.TextSize = theme.Size(theme.SizeNameCaptionText)
	return t
}

// newFilterInvertChip — чип «!» категории.
func newFilterInvertChip(initial bool, onChange func(bool)) *fynewidget.Chip {
	c := fynewidget.NewChip("!", initial, onChange)
	c.SetToolTip(locale.T(filterInvertCategoryTooltip))
	return c
}

// filterRadioChips — взаимоисключающая группа чипов (категория «Тест»).
//
// Выбор снять нельзя: «ни один не выбран» у режима теста означало бы четвёртое
// состояние сверх any/ok/error/untested, а его в модели нет. Поэтому клик по
// уже выбранному чипу возвращает его в выбранное состояние.
type filterRadioChips struct {
	chips   []*fynewidget.Chip
	objects []fyne.CanvasObject
}

func newFilterRadioChips(titles []string, selected int, onSelect func(int)) *filterRadioChips {
	g := &filterRadioChips{
		chips:   make([]*fynewidget.Chip, 0, len(titles)),
		objects: make([]fyne.CanvasObject, 0, len(titles)),
	}
	for i, title := range titles {
		i := i
		chip := fynewidget.NewChip(title, i == selected, nil)
		chip.OnChanged = func(on bool) {
			if !on {
				chip.SetSelected(true) // снять выбор нельзя — см. шапку типа
				return
			}
			g.set(i)
			onSelect(i)
		}
		g.chips = append(g.chips, chip)
		g.objects = append(g.objects, chip)
	}
	return g
}

// set — подсветить i-й чип, погасив остальные; OnChanged не зовётся.
func (g *filterRadioChips) set(selected int) {
	for i, c := range g.chips {
		c.SetSelected(i == selected)
	}
}

// setChipSelection — выбор чипа в карте категории; снятый ключ удаляется, а не
// хранится как false, чтобы «категория выключена» читалось по длине карты.
func setChipSelection(m map[string]bool, key string, on bool) {
	if on {
		m[key] = true
		return
	}
	delete(m, key)
}
