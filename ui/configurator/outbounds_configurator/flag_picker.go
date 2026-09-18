// File flag_picker.go — эмодзи-пикер поля Filter формы Направления.
//
// Юзер кликает 🌐 справа от Filters → tag → открывается отдельное окно:
//
//	┌─ Emoji picker — 7 / 7 match ─────────────────────┐
//	│ Regex   ! [ 🇷🇺|🇳🇱                      ] ⧉ ✕    │
//	│ Emoji     🔥³ 🎭² ☂¹ 🇨🇦¹ 🇨🇭¹ 🇳🇱¹ 🇺🇸¹            │
//	│ ──────────────────────────────────────────────── │
//	│ ✓ 🔥🎭 WARP (MASQUE) auto                         │
//	│ ✓ Proton 🇨🇦 Канада #31                           │
//	│ ✗ Proton 🇺🇸 USA #137           (список в скролле)│
//	│                              [Cancel]  [Apply]   │
//	└──────────────────────────────────────────────────┘
//
// # Почему так
//
// Вид — тот же, что у окна фильтров вкладки Servers (ui/servers_filter_window.go):
// строка категории «подпись │ [!] │ содержимое», компактные чипы
// fynewidget.Chip со счётчиком, поток fynewidget.FlowBox с потолком высоты,
// высота окна по содержимому. Обе формы отбирают узлы по значку и регулярке
// одним и тем же механизмом, и разная вёрстка читалась бы как разные
// инструменты. Общие куски (CategoryRow, HintText, NewInvertChip, метрики)
// живут в internal/fynewidget — пакет ui пикеру недоступен, ui сам тянет
// конфигуратор.
//
// Отдельных подписей-заголовков («Emoji found in node names…», «Filter body…»)
// в форме нет: смысл ушёл в плейсхолдер поля и тултипы чипов, а строки
// съедали высоту, ради которой и затевалась компактность. Счётчик совпадений
// стоит в ЗАГОЛОВКЕ окна — там же, где у окна фильтров «показано / всего».
//
// Live: клик по чипу ИЛИ ручная правка поля — список перефильтровывается
// сразу. Используем тот же config.PreviewSelectorNodes, что Preview-вкладка:
// гарантия, что попадание ноды показывается так же, как в финальном emit'е.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package outbounds_configurator

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

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/emojitag"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/ui/components"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	pickerInvertTooltipText = "Invert: keep nodes that do NOT match"
	pickerEmojiChipTooltip  = "Toggle this emoji in the Regex field (OR pattern)."
	pickerNoEmojiText       = "No emoji in the names of these nodes."
	pickerRegexTooltipText  = "Filter body: a regex over node names, applied live. Emoji chips above toggle terms in it."
	pickerCopyTooltipText   = "Copy the pattern body."
	pickerRegexPlaceholder  = "regex, e.g. NL|DE|Proton"
)

// Метрики формы. Ширина колонки подписей, ячейка «!» и высота ряда чипов —
// общие с окном фильтров (fynewidget.CategoryLabelWidth и соседи).
const (
	// pickerEmojiMaxRows — потолок высоты потока эмодзи в рядах чипов: в
	// подписке на 500 узлов эмодзи бывает под сотню.
	pickerEmojiMaxRows = 3
	// pickerListRows — сколько строк превью видно без прокрутки.
	pickerListRows = 10
	// pickerRowHeight — высота строки превью (мелкий кегль, плотно).
	pickerRowHeight = 18
	// pickerWindowWidth / pickerWindowMaxHeight — как у окна фильтров.
	pickerWindowWidth     = 460
	pickerWindowMaxHeight = 640
)

// Цвета строк превью — те же, что на вкладке Preview: зелёный «узел вошёл»,
// приглушённо-красный «не вошёл».
var (
	pickerRowInColor  = color.RGBA{R: 0, G: 160, B: 0, A: 255}
	pickerRowOutColor = color.RGBA{R: 190, G: 90, B: 90, A: 255}
)

// extractFlags — эмодзи тегов всех нод с частотой, по убыванию частоты.
//
// SPEC 104: не только флаги. Провайдеры кладут в имена 🚀, ⭐, 🔒, 💎 и
// прочее — это такие же маркеры категории, как флаг страны, и отбирать по
// ним должно быть так же легко. Флаг (пара Regional Indicator) остаётся
// одним элементом, а не двумя буквами.
//
// Сам разбор живёт в internal/emojitag — общий с окном фильтров списка
// серверов: два места предлагают один и тот же отбор по значку, и второй
// копии алгоритма у них быть не должно.
func extractFlags(nodes []*config.ParsedNode) []emojitag.Entry {
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		names = append(names, n.Tag)
	}
	return emojitag.Entries(names)
}

// showFlagPickerPopup открывает окно пикера. На Apply зовёт onApply с телом
// регулярки и флагом инверсии; Cancel закрывает без изменений.
//
// `nodes` = model.NodePool (если nil/empty — чипы и список пусты, поле
// регулярки всё равно работает).
func showFlagPickerPopup(
	parent fyne.Window,
	nodes []*config.ParsedNode,
	currentBody string,
	currentInvert bool,
	onApply func(body string, invert bool),
) {
	if parent == nil || parent.Canvas() == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}

	flags := extractFlags(nodes)
	total := len(nodes)
	invert := currentInvert

	win := app.NewWindow(locale.T("Emoji picker"))

	// ── Regex ──────────────────────────────────────────────────────────────
	regexEntry := widget.NewEntry()
	regexEntry.SetPlaceHolder(locale.T(pickerRegexPlaceholder))
	regexEntry.SetText(currentBody)
	fynewidget.SetToolTipSafe(regexEntry, locale.T(pickerRegexTooltipText))

	// ── Список превью ──────────────────────────────────────────────────────
	//
	// canvas.Text мелким кеглем, а не widget.Label: у Label минимальная ширина
	// считается по единственной строке и раздувала бы окно (память
	// `fyne-label-minwidth-trap`), да и строки шли бы вдвое реже.
	type listRow struct {
		text  string
		color color.Color
	}
	var rows []listRow

	nodeList := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			t := canvas.NewText("", color.White)
			t.TextSize = theme.Size(theme.SizeNameCaptionText)
			return t
		},
		func(id int, o fyne.CanvasObject) {
			if id < 0 || id >= len(rows) {
				return
			}
			if t, ok := o.(*canvas.Text); ok {
				t.Text = rows[id].text
				t.Color = rows[id].color
				t.Refresh()
			}
		},
	)
	// Плотные строки: высоту ряда задаёт MinSize шаблона (canvas.Text
	// капитального кегля), а разделители между рядами убраны — в списке из
	// полусотни узлов они добавляли бы по пикселю на строку и ничего не
	// разделяли: строки и так разного цвета.
	nodeList.HideSeparators = true

	var chipRefresh func()

	// recomputeMatches — прогоняет текущее тело через ТУ ЖЕ функцию, что
	// Preview-вкладка (config.PreviewSelectorNodes), и пересобирает строки.
	recomputeMatches := func() {
		rows = rows[:0]

		// Синтетическое Направление с одним лишь фильтром: нас интересует
		// только то, какие узлы в него попадают.
		cfg := config.Direction{
			Tag:     "_flag_picker_",
			Type:    "selector",
			Filters: configtypes.SetDirectionFilterTag(nil, regexEntry.Text, invert),
		}

		filtered, _ := config.PreviewSelectorNodes(nodes, cfg)
		filteredSet := make(map[*config.ParsedNode]bool, len(filtered))
		for _, n := range filtered {
			filteredSet[n] = true
		}

		// Счётчик — в заголовке окна, как «показано / всего» у окна фильтров:
		// отдельная строка «matches N of M» занимала высоту ради числа,
		// которому есть готовое место.
		win.SetTitle(locale.Tf("Emoji picker — %d / %d match", len(filtered), total))

		// Сперва попавшие, затем отсеянные — как в Preview.
		var inRows, outRows []listRow
		for _, n := range nodes {
			if n == nil {
				continue
			}
			text := n.Tag
			if text == "" {
				if n.Label != "" {
					text = n.Label
				} else if n.Server != "" {
					text = fmt.Sprintf("%s:%d", n.Server, n.Port)
				} else {
					text = n.Scheme
				}
			}
			text = textnorm.NormalizeProxyDisplay(text)

			if filteredSet[n] {
				inRows = append(inRows, listRow{text: "✓ " + text, color: pickerRowInColor})
			} else {
				outRows = append(outRows, listRow{text: "✗ " + text, color: pickerRowOutColor})
			}
		}
		rows = append(rows, inRows...)
		rows = append(rows, outRows...)
		nodeList.Refresh()
	}

	// Подсветка чипов идёт СРАЗУ за текстом поля (без ожидания пересчёта): она
	// отражает то, что в поле, а не результат отбора.
	regexEntry.OnChanged = func(_ string) {
		if chipRefresh != nil {
			chipRefresh()
		}
		recomputeMatches()
	}

	invertChip := fynewidget.NewInvertChip(invert, func(on bool) {
		invert = on
		recomputeMatches()
	})
	// Подсказка пикера отличается от общей «инверсии категории»: здесь она
	// переворачивает весь отбор Направления, и текст остаётся прежним —
	// человек видит его и в форме Направления на кнопке «!».
	invertChip.SetToolTip(locale.T(pickerInvertTooltipText))

	regexCopy := ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		body := strings.TrimSpace(regexEntry.Text)
		if body == "" {
			return
		}
		if a := fyne.CurrentApp(); a != nil && a.Clipboard() != nil {
			a.Clipboard().SetContent(body)
		}
	})
	regexCopy.Importance = widget.LowImportance
	regexCopy.SetToolTip(locale.T(pickerCopyTooltipText))

	regexClear := ttwidget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		regexEntry.SetText("") // OnChanged сам обновит чипы и список
	})
	regexClear.Importance = widget.LowImportance
	regexClear.SetToolTip(locale.T("Clear the pattern"))

	regexRow := fynewidget.CategoryRow(invertChip, locale.T("Regex"),
		container.NewBorder(nil, nil, nil,
			container.NewHBox(regexCopy, regexClear),
			regexEntry,
		), regexEntry.MinSize().Height)

	// ── Поток чипов ────────────────────────────────────────────────────────
	//
	// Подписи чипов собираются strconv/данными, а не locale.Tf: переводить в
	// них нечего — это эмодзи из чужой подписки и число рядом.
	emojiFlow := fynewidget.NewFlowBox()
	emojiFlow.MaxHeight = pickerEmojiMaxRows * fynewidget.ChipRowHeight

	chipRefresh = func() {
		if len(flags) == 0 {
			emojiFlow.SetObjects([]fyne.CanvasObject{
				fynewidget.HintText(locale.T(pickerNoEmojiText)),
			})
			return
		}
		// Выбранным считается то, что СТОИТ В ПОЛЕ: человек мог набрать терм
		// руками, и чип обязан это показать — иначе клик по нему добавил бы
		// дубль вместо снятия.
		selected := map[string]bool{}
		for _, term := range emojitag.SplitORPattern(regexEntry.Text) {
			selected[term] = true
		}
		objs := make([]fyne.CanvasObject, 0, len(flags))
		for _, fe := range flags {
			fe := fe
			chip := fynewidget.NewChip(fe.Emoji, selected[fe.Emoji], nil).
				SetCount(strconv.Itoa(fe.Count))
			chip.OnChanged = func(bool) {
				// Клик = toggle терма в OR-паттерне поля. Поле остаётся
				// ЕДИНСТВЕННЫМ носителем отбора: второй набор «выбранных
				// эмодзи» рядом с ним разошёлся бы с ним на первой ручной правке.
				regexEntry.SetText(emojitag.ToggleInORPattern(regexEntry.Text, fe.Emoji))
			}
			chip.SetToolTip(locale.T(pickerEmojiChipTooltip))
			objs = append(objs, chip)
		}
		emojiFlow.SetObjects(objs)
	}
	chipRefresh()

	emojiRow := fynewidget.CategoryRow(nil, locale.T("Emoji"), emojiFlow)

	// ── Нижний ряд ─────────────────────────────────────────────────────────
	cancelBtn := widget.NewButton(locale.T("Cancel"), func() { win.Close() })
	applyBtn := widget.NewButton(locale.T("Apply"), func() {
		if onApply != nil {
			onApply(strings.TrimSpace(regexEntry.Text), invert)
		}
		win.Close()
	})
	applyBtn.Importance = widget.HighImportance
	bottomRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(cancelBtn, applyBtn), layout.NewSpacer())

	// ── Сборка ─────────────────────────────────────────────────────────────
	//
	// Форма (две строки категорий) — сверху, список превью занимает остаток.
	//
	// Gutter — отдельной колонкой СПРАВА ОТ списка, а не оборачиванием в
	// components.WrapInScrollWithGutter: widget.List прокручивает себя сам, и
	// вложить его во внешний Scroll значило бы убить виртуализацию — на пуле в
	// 500 узлов форма строила бы все строки разом.
	form := container.NewVBox(regexRow, emojiRow, widget.NewSeparator())

	// Минимальная высота списка — распоркой в стопке под ним: GridWrap задал бы
	// и ширину (нулевую), а SetMinSize есть только у Scroll, которого здесь нет.
	listMinH := canvas.NewRectangle(color.Transparent)
	listMinH.SetMinSize(fyne.NewSize(0, pickerListRows*pickerRowHeight))
	listBox := container.NewBorder(nil, nil, nil, components.NewScrollGutter(),
		container.NewStack(listMinH, nodeList))

	content := container.NewBorder(
		container.NewPadded(form), bottomRow, nil, nil,
		container.NewPadded(listBox),
	)

	// Слой тултипов обязателен: без него SetToolTip у чипов и кнопок молчит.
	// Разрушается на закрытии — он держит ссылку на канву закрытого окна.
	win.SetContent(fynetooltip.AddWindowToolTipLayer(content, win.Canvas()))
	closed := false
	win.SetOnClosed(func() {
		closed = true
		fynetooltip.DestroyWindowToolTipLayer(win.Canvas())
	})

	recomputeMatches() // и заголовок, и строки
	win.Resize(fyne.NewSize(pickerWindowWidth, 360))
	fynewidget.CenterOnScreen(win)
	win.Show()

	// Высота окна — ПО СОДЕРЖИМОМУ, с потолком: настоящая высота потока чипов
	// известна только после первой раскладки (она зависит от ширины), поэтому
	// подгонка идёт вдогонку показу — тот же приём, что в окне фильтров.
	fitHeight := func() {
		// Окно могли закрыть за эти 120 мс: тянуть размер у разрушенной канвы
		// нечего.
		if closed || win.Canvas() == nil {
			return
		}
		h := form.MinSize().Height + listBox.MinSize().Height +
			bottomRow.MinSize().Height + 6*theme.Padding()
		if h > pickerWindowMaxHeight {
			h = pickerWindowMaxHeight
		}
		win.Resize(fyne.NewSize(win.Canvas().Size().Width, h))
	}
	// Вдогонку показу, а не сразу: FlowBox знает свою высоту только после того,
	// как получил настоящую ширину в первой раскладке.
	time.AfterFunc(120*time.Millisecond, func() { fyne.Do(fitHeight) })
}
