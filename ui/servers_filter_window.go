// File servers_filter_window.go — ОКНО фильтров списка узлов вкладки Servers.
// Модель и предикат, которыми оно управляет, живут в servers_filter.go.
//
//	┌─ Filters ────────────────── показано 24 / 312 ─┐
//	│ Regex   [ 🇩🇪|🇳🇱|Proton        ] [⧉] [✕]  [!] │
//	│         ⚠ invalid regex: …                     │
//	│ Emoji   [🇩🇪 24] [🇳🇱 18] [🇺🇸 17] [🔥 4] …     │
//	│ Protocol   [vless] [wireguard] …           [!] │
//	│ Transport  [tcp] [ws] [Reality+Vision] …   [!] │
//	│ Source     [Proton] [BL: ЧЁРНЫЕ СП…] …     [!] │
//	│ Test    (•) any ( ) ok ( ) error ( ) untested  │
//	│         [ ] ping ≤ [200] ms                    │
//	│ [ Invert all ]              [ Reset ] [ Close ]│
//	└────────────────────────────────────────────────┘
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
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/emojitag"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	filterRegexCopyTooltipText   = "Copy the pattern body (no wrapper) — paste it into a Direction's filter field."
	filterRegexCopiedInvertText  = "Pattern body copied. Its invert flag is NOT part of the body: tick Invert in the Direction's filter."
	filterRegexCopiedPlainText   = "Pattern body copied."
	filterInvertCategoryTooltip  = "Invert this category: keep the nodes it does NOT select."
	filterInvertAllTooltipText   = "Invert the whole filter: show exactly the rows it hides now."
	filterEmojiChipTooltipText   = "Toggle this emoji in the Regex field (OR pattern)."
	filterNoEmojiText            = "No emoji in the names of the nodes in this list."
	filterKeepsSelectedRowsText  = "Selected rows, direct-out and the active proxy are never hidden."
	filterPingKeepsUntestedText  = "Untested nodes pass the threshold — it answers «show me the fast ones», not «drop the unmeasured»."
	filterWindowRegexPlaceholder = "🇩🇪|🇳🇱|Proton"
)

// serversFilterDebounce — пауза перед компиляцией регулярки (LxBox §048 #4).
//
// Печать идёт посимвольно, и пересчёт среза на каждый символ гонял бы разбор
// 500 узлов и перерисовку списка десятки раз на одно слово.
const serversFilterDebounce = 300 * time.Millisecond

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
	regexEntry.SetPlaceHolder(filterWindowRegexPlaceholder)
	regexEntry.SetText(st().RegexBody)

	// Строка ошибки — canvas-текста ей не нужно, но Wrapping обязателен:
	// сообщение regexp бывает длинным, а Label без переноса меряет себя ОДНОЙ
	// строкой и раздувает окно по ширине (память `fyne-label-minwidth-trap`).
	regexError := widget.NewLabel("")
	regexError.Wrapping = fyne.TextWrapWord
	regexError.Importance = widget.DangerImportance
	regexError.Hide()

	var refreshEmojiChips func()
	syncRegexError := func() {
		body := strings.TrimSpace(st().RegexBody)
		if body == "" {
			regexError.Hide()
			return
		}
		if _, err := compileServersFilterRegex(body); err != nil {
			regexError.SetText(locale.Tf("Invalid regex: %s", err.Error()))
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

	regexInvert := newFilterInvertButton(st().RegexInvert, func(on bool) {
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
	regexCopy.SetToolTip(locale.T(filterRegexCopyTooltipText))

	regexClear := ttwidget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		regexEntry.SetText("") // OnChanged сам обновит состояние и чипы
	})
	regexClear.SetToolTip(locale.T("Clear the pattern"))

	regexRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(regexCopy, regexClear, regexInvert.button),
		regexEntry,
	)

	// ── Чипы категорий ─────────────────────────────────────────────────────
	//
	// Подписи чипов собираются fmt.Sprintf, а не locale.Tf: переводить в них
	// нечего — это значение из данных («vless», «🇩🇪», имя подписки) и число
	// рядом. Прогонять их через словарь значило бы завести там сотни ключей,
	// приезжающих из чужих подписок.
	emojiBox := container.NewVBox()
	protocolBox := container.NewVBox()
	variantBox := container.NewVBox()
	sourceBox := container.NewVBox()

	protocolInvert := newFilterInvertButton(st().ProtocolsInvert, func(on bool) {
		st().ProtocolsInvert = on
		apply()
	})
	variantInvert := newFilterInvertButton(st().VariantsInvert, func(on bool) {
		st().VariantsInvert = on
		apply()
	})
	sourceInvert := newFilterInvertButton(st().SourcesInvert, func(on bool) {
		st().SourcesInvert = on
		apply()
	})

	refreshEmojiChips = func() {
		facets := host.Facets()
		emojiBox.Objects = nil
		if len(facets.Emojis) == 0 {
			emojiBox.Add(widget.NewLabel(locale.T(filterNoEmojiText)))
			emojiBox.Refresh()
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
			btn := ttwidget.NewButton(fmt.Sprintf("%s %d", e.Emoji, e.Count), nil)
			btn.OnTapped = func() {
				// Клик = toggle терма в OR-паттерне поля (LxBox :126). Поле
				// остаётся ЕДИНСТВЕННЫМ носителем отбора по имени: второй
				// набор «выбранных эмодзи» рядом с ним разошёлся бы с ним на
				// первой же ручной правке.
				next := emojitag.ToggleInORPattern(st().RegexBody, e.Emoji)
				regexEntry.SetText(next)
			}
			if selected[e.Emoji] {
				btn.Importance = widget.HighImportance
			}
			btn.SetToolTip(locale.T(filterEmojiChipTooltipText))
			objs = append(objs, btn)
		}
		emojiBox.Add(newFilterChipGrid(objs))
		emojiBox.Refresh()
	}

	refreshProtocolChips := func() {
		facets := host.Facets()
		protocolBox.Objects = nil
		objs := make([]fyne.CanvasObject, 0, len(facets.Protocols))
		for _, f := range facets.Protocols {
			f := f
			objs = append(objs, newFilterToggleChip(
				fmt.Sprintf("%s %d", f.Key, f.Count), "",
				st().Protocols[f.Key],
				func(on bool) {
					setChipSelection(st().Protocols, f.Key, on)
					apply()
				}))
		}
		protocolBox.Add(newFilterChipGrid(objs))
		protocolBox.Refresh()
	}

	refreshVariantChips := func() {
		facets := host.Facets()
		variantBox.Objects = nil
		objs := make([]fyne.CanvasObject, 0, len(facets.Variants))
		for _, f := range facets.Variants {
			f := f
			objs = append(objs, newFilterToggleChip(
				fmt.Sprintf("%s %d", f.Key, f.Count), "",
				st().Variants[f.Key],
				func(on bool) {
					setChipSelection(st().Variants, f.Key, on)
					apply()
				}))
		}
		variantBox.Add(newFilterChipGrid(objs))
		variantBox.Refresh()
	}

	refreshSourceChips := func() {
		facets := host.Facets()
		sourceBox.Objects = nil
		objs := make([]fyne.CanvasObject, 0, len(facets.Sources))
		for _, f := range facets.Sources {
			f := f
			objs = append(objs, newFilterToggleChip(
				fmt.Sprintf("%s %d", truncateSourceChipName(f.Name), f.Count),
				f.Name, // полное имя — в тултип: в чипе оно обрезано
				st().Sources[f.ID],
				func(on bool) {
					setChipSelection(st().Sources, f.ID, on)
					apply()
				}))
		}
		sourceBox.Add(newFilterChipGrid(objs))
		sourceBox.Refresh()
	}

	// ── Test ───────────────────────────────────────────────────────────────
	testOptions := []string{
		locale.T("any"), locale.T("ok"), locale.T("error"), locale.T("untested"),
	}
	testRadio := widget.NewRadioGroup(testOptions, nil)
	testRadio.Horizontal = true
	testRadio.SetSelected(testOptions[int(st().Test)])
	testRadio.OnChanged = func(sel string) {
		for i, opt := range testOptions {
			if opt == sel {
				// Через SetTest, а не присваиванием: выбор «error» обязан
				// погасить «глаз» панели, иначе режим показывал бы пустоту
				// (см. servers_filter.go).
				st().SetTest(serversTestMode(i))
				break
			}
		}
		apply()
	}

	pingEntry := widget.NewEntry()
	pingEntry.SetText(st().PingText)
	// Поле узкое: это число миллисекунд, а растянутое на всю строку оно
	// читалось бы как поле ввода текста.
	pingEntryWrap := container.NewGridWrap(fyne.NewSize(72, pingEntry.MinSize().Height), pingEntry)

	pingCheck := widget.NewCheck(locale.T("ping ≤"), nil)
	pingCheck.SetChecked(st().PingEnabled)
	pingCheck.OnChanged = func(on bool) {
		st().PingEnabled = on
		apply()
	}

	var pingDebounce *time.Timer
	pingEntry.OnChanged = func(text string) {
		st().PingText = text
		if pingDebounce != nil {
			pingDebounce.Stop()
		}
		pingDebounce = time.AfterFunc(serversFilterDebounce, func() {
			fyne.Do(func() {
				n, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil || n <= 0 {
					st().PingMaxMs = 0
				} else {
					st().PingMaxMs = n
				}
				apply()
			})
		})
	}

	pingRow := container.NewHBox(
		pingCheck, pingEntryWrap, widget.NewLabel(locale.T("ms")),
	)
	// Обе подсказки — с Wrapping: Label без переноса меряет свою ЕДИНСТВЕННУЮ
	// строку как минимальную ширину и раздувает окно на весь экран (память
	// `fyne-label-minwidth-trap`).
	pingHint := widget.NewLabel(locale.T(filterPingKeepsUntestedText))
	pingHint.Wrapping = fyne.TextWrapWord
	pingHint.Importance = widget.LowImportance

	keepsHint := widget.NewLabel(locale.T(filterKeepsSelectedRowsText))
	keepsHint.Wrapping = fyne.TextWrapWord
	keepsHint.Importance = widget.LowImportance

	// ── Нижний ряд ─────────────────────────────────────────────────────────
	invertAll := ttwidget.NewButton(locale.T("Invert all"), nil)
	syncInvertAll := func() {
		if st().InvertAll {
			invertAll.Importance = widget.HighImportance
		} else {
			invertAll.Importance = widget.MediumImportance
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

	// rebuildAll — перечитать ВСЁ состояние в виджеты. Нужен и Reset'у, и
	// смене группы: у новой группы свой снимок фильтра, и поля обязаны
	// показать именно его.
	rebuildAll := func() {
		f := st()
		// SetText зовёт OnChanged, и тот перепишет RegexBody тем же текстом —
		// безобидно, но чипы обновятся до того, как мы дойдём до них ниже.
		regexEntry.SetText(f.RegexBody)
		regexInvert.set(f.RegexInvert)
		protocolInvert.set(f.ProtocolsInvert)
		variantInvert.set(f.VariantsInvert)
		sourceInvert.set(f.SourcesInvert)
		testRadio.SetSelected(testOptions[int(f.Test)])
		pingCheck.SetChecked(f.PingEnabled)
		pingEntry.SetText(f.PingText)
		syncInvertAll()
		syncRegexError()
		refreshEmojiChips()
		refreshProtocolChips()
		refreshVariantChips()
		refreshSourceChips()
	}

	resetBtn := widget.NewButton(locale.T("Reset"), func() {
		*st() = newServersFilterState()
		rebuildAll()
		apply()
	})
	closeBtn := widget.NewButton(locale.T("Close"), func() { fw.win.Close() })

	bottomRow := container.NewBorder(nil, nil, invertAll,
		container.NewHBox(resetBtn, closeBtn),
		layout.NewSpacer(),
	)

	// ── Сборка ─────────────────────────────────────────────────────────────
	form := container.NewVBox(
		filterSectionLabel(locale.T("Regex")),
		regexRow,
		regexError,
		widget.NewSeparator(),

		filterSectionLabel(locale.T("Emoji")),
		emojiBox,
		widget.NewSeparator(),

		filterSectionHeader(locale.T("Protocol"), protocolInvert.button),
		protocolBox,
		widget.NewSeparator(),

		filterSectionHeader(locale.T("Transport"), variantInvert.button),
		variantBox,
		widget.NewSeparator(),

		filterSectionHeader(locale.T("Source"), sourceInvert.button),
		sourceBox,
		widget.NewSeparator(),

		filterSectionLabel(locale.T("Test")),
		testRadio,
		pingRow,
		pingHint,
		widget.NewSeparator(),

		keepsHint,
	)

	content := container.NewBorder(nil, bottomRow, nil, nil,
		container.NewVScroll(form))

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
	// отложенный fyne.Do пришёл бы к виджетам закрытого окна.
	fw.win.SetOnClosed(func() {
		if debounce != nil {
			debounce.Stop()
		}
		if pingDebounce != nil {
			pingDebounce.Stop()
		}
		if host.Closed != nil {
			host.Closed()
		}
	})

	fw.win.SetContent(content)
	fw.win.Resize(fyne.NewSize(520, 560))
	fynewidget.CenterOnScreen(fw.win)
	fw.win.Show()
	return fw
}

// filterSectionLabel — подпись категории.
func filterSectionLabel(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return l
}

// filterSectionHeader — подпись категории с кнопкой инверсии справа.
func filterSectionHeader(text string, invert fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, filterSectionLabel(text), invert, layout.NewSpacer())
}

// filterInvertButton — кнопка «!» категории: подсвечена, когда включена.
type filterInvertButton struct {
	button *ttwidget.Button
	on     bool
}

func newFilterInvertButton(initial bool, onChange func(bool)) *filterInvertButton {
	b := &filterInvertButton{on: initial}
	b.button = ttwidget.NewButton("!", nil)
	b.button.OnTapped = func() {
		b.on = !b.on
		b.sync()
		onChange(b.on)
	}
	b.button.SetToolTip(locale.T(filterInvertCategoryTooltip))
	b.sync()
	return b
}

// set — поставить состояние БЕЗ вызова onChange (восстановление снимка).
func (b *filterInvertButton) set(on bool) {
	b.on = on
	b.sync()
}

func (b *filterInvertButton) sync() {
	if b.on {
		b.button.Importance = widget.HighImportance
	} else {
		b.button.Importance = widget.MediumImportance
	}
	b.button.Refresh()
}

// newFilterToggleChip — чип-тоггл категории: подсвечен, когда выбран.
//
// Кнопка, а не widget.Check: чипов бывает под сотню, и ряд галок читается как
// форма настроек, а не как быстрый отбор.
func newFilterToggleChip(label, tooltip string, selected bool, onChange func(bool)) fyne.CanvasObject {
	on := selected
	btn := ttwidget.NewButton(label, nil)
	sync := func() {
		if on {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.MediumImportance
		}
		btn.Refresh()
	}
	btn.OnTapped = func() {
		on = !on
		sync()
		onChange(on)
	}
	if tooltip != "" {
		btn.SetToolTip(tooltip)
	}
	sync()
	return btn
}

// newFilterChipGrid — сетка чипов 5 в ряд с потолком высоты.
//
// Потолок обязателен: эмодзи и источников в подписке на 500 узлов бывает
// много, и без него окно выросло бы за пределы экрана, унося кнопки под док
// (ровно это чинили в пикере флагов).
func newFilterChipGrid(objs []fyne.CanvasObject) fyne.CanvasObject {
	if len(objs) == 0 {
		return widget.NewLabel("—")
	}
	const perRow = 5
	grid := container.NewGridWithColumns(perRow, objs...)
	rows := (len(objs) + perRow - 1) / perRow
	height := float32(rows) * 38
	const maxHeight = 152 // ≈4 ряда, дальше прокрутка
	if height > maxHeight {
		height = maxHeight
	}
	scroll := container.NewVScroll(grid)
	scroll.SetMinSize(fyne.NewSize(0, height))
	return scroll
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
