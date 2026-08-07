package ui

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/go-text/typesetting/font"

	"singbox-launcher/internal/fynewidget"
)

// Каталог глифов — отладочное окно для подбора значков UI.
//
// Зачем: подобрать символ «на глаз» по таблице Unicode нельзя. Шрифты темы
// Fyne покрывают разные наборы, и символ, который прекрасно выглядит в
// редакторе, в приложении может:
//   - не отрисоваться вовсе (глиф в шрифте пустой — так вышло с ⚡ U+26A1);
//   - превратиться в тофу-квадрат (нет в NotoSans, куда тема уходит на
//     кириллице — так вышло с → U+2192);
//   - прийти готовой ЦВЕТНОЙ картинкой, которую не покрасить (эмодзи);
//   - схлопнуться в нечитаемое пятно на мелком кегле (🔀 на 10pt).
//
// Окно показывает КАЖДЫЙ символ шрифтов темы отрисованным настоящим
// рендерером Fyne, на том же кегле, что и подзаголовок в списке серверов, —
// то есть ровно так, как он будет выглядеть в бою.
//
// Открывается из Диагностики; в обычной работе не мешает.

// glyphExplorerPreviewSize — кегль превью.
//
// Совпадает с serversSubtitleTextSize: значки подбираются именно для
// подзаголовка, и смотреть их надо в его размере, а не крупными.
const glyphExplorerPreviewSize = serversSubtitleTextSize

// glyphExplorerColumns — сколько символов в ряду сетки.
const glyphExplorerColumns = 12

// glyphExplorerCellSize — сторона ячейки сетки.
//
// Заметно больше кегля: символ рисуется в своём размере, а пустое поле вокруг
// нужно, чтобы попасть по нему мышью и увидеть выделение.
const glyphExplorerCellSize = 34

// glyphSource описывает, из какого шрифта берётся символ.
type glyphSource struct {
	// Name — имя шрифта для колонки «Источник».
	Name string
	// Face — разобранный шрифт; nil, если не загрузился.
	Face *font.Face
}

// glyphEntry — одна строка каталога.
type glyphEntry struct {
	// R — сам символ.
	R rune
	// Sources — шрифты темы, в которых он есть.
	Sources []string
	// Emoji — символ приходит из эмодзи-шрифта: цветной, не красится.
	Emoji bool
	// TextSafe — есть во ВСЕХ текстовых шрифтах, значит рисуется одинаково
	// и в латинице, и в кириллице, и наследует цвет.
	TextSafe bool
}

// ShowGlyphExplorer открывает каталог глифов шрифтов темы.
func ShowGlyphExplorer(app fyne.App) {
	win := app.NewWindow("Каталог глифов — подбор значков UI")
	win.Resize(fyne.NewSize(720, 620))

	entries := collectGlyphs()

	// Фильтры: по умолчанию показываем всё.
	var (
		filterText  string
		onlySafe    bool
		onlyEmoji   bool
		shown       []glyphEntry
		grid        *fyne.Container
		statusLabel *widget.Label
		detail      *widget.Label
		preview     *canvas.Text
		// selected — символ под курсором; кнопки копирования берут его.
		selected glyphEntry
	)

	// Крупное превью выбранного символа рядом с текстом подзаголовка —
	// чтобы сразу видеть, как он смотрится в реальной строке.
	preview = canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	preview.TextSize = glyphExplorerPreviewSize

	detail = widget.NewLabel("Выберите символ")
	detail.Wrapping = fyne.TextWrapWord

	statusLabel = widget.NewLabel("")

	selectGlyph := func(e glyphEntry) {
		selected = e
		kind := "текстовый (красится темой)"
		if e.Emoji {
			kind = "ЭМОДЗИ — цветной, покрасить нельзя"
		}
		safety := "⚠ есть НЕ во всех текстовых шрифтах: на кириллице возможен тофу"
		if e.TextSafe {
			safety = "✓ есть во всех текстовых шрифтах — тофу не будет"
		}
		if e.Emoji {
			safety = "✓ эмодзи-шрифт покрывает любой язык"
		}

		detail.SetText(fmt.Sprintf(
			"%c   U+%04X\n\nGo-литерал: \"\\U%08X\"\nТип: %s\n%s\nШрифты: %s",
			e.R, e.R, e.R, kind, safety, strings.Join(e.Sources, ", ")))

		// Показываем символ в реальной строке подзаголовка.
		preview.Text = fmt.Sprintf("%c [37] balanced ‣ AL:🇩🇪-Германия", e.R)
		preview.Refresh()
	}

	rebuild := func() {
		shown = shown[:0]
		q := strings.ToLower(strings.TrimSpace(filterText))
		for _, e := range entries {
			if onlySafe && !e.TextSafe {
				continue
			}
			if onlyEmoji && !e.Emoji {
				continue
			}
			if q != "" {
				// Ищем по коду («26a1», «u+26a1») и по самому символу.
				hex := fmt.Sprintf("%04x", e.R)
				if !strings.Contains(hex, strings.TrimPrefix(q, "u+")) &&
					!strings.ContainsRune(q, e.R) {
					continue
				}
			}
			shown = append(shown, e)
		}

		grid.Objects = grid.Objects[:0]
		for i := range shown {
			e := shown[i]
			cell := newGlyphCell(e, selectGlyph)
			grid.Objects = append(grid.Objects, cell)
		}
		grid.Refresh()

		statusLabel.SetText(fmt.Sprintf("Показано %d из %d символов", len(shown), len(entries)))
	}

	grid = container.NewGridWithColumns(glyphExplorerColumns)

	search := widget.NewEntry()
	search.SetPlaceHolder("Код (26a1) или сам символ")
	search.OnChanged = func(s string) {
		filterText = s
		rebuild()
	}

	safeCheck := widget.NewCheck("Только безопасные (без тофу)", func(v bool) {
		onlySafe = v
		rebuild()
	})
	emojiCheck := widget.NewCheck("Только эмодзи", func(v bool) {
		onlyEmoji = v
		rebuild()
	})

	controls := container.NewVBox(
		search,
		container.NewHBox(safeCheck, emojiCheck),
		statusLabel,
	)

	// Кнопки копирования. Символ подбирается ДЛЯ КОДА, поэтому копировать
	// нужно две разные вещи: сам глиф (вставить в строку) и Go-литерал
	// «\U0001F504» (безопасен в исходнике — не зависит от кодировки файла
	// и виден в diff'е).
	copyStatus := widget.NewLabel("")

	copyToClipboard := func(s, what string) {
		if s == "" {
			return
		}
		app := fyne.CurrentApp()
		if app == nil || app.Clipboard() == nil {
			copyStatus.SetText("Буфер обмена недоступен")
			return
		}
		app.Clipboard().SetContent(s)
		copyStatus.SetText("Скопировано: " + what)
	}

	copyGlyphBtn := widget.NewButton("Копировать символ", func() {
		if selected.R == 0 {
			return
		}
		copyToClipboard(string(selected.R), string(selected.R))
	})
	copyLiteralBtn := widget.NewButton("Копировать Go-литерал", func() {
		if selected.R == 0 {
			return
		}
		lit := fmt.Sprintf("\\U%08X", selected.R)
		copyToClipboard(lit, lit)
	})

	// Нижняя панель: реальная строка подзаголовка + описание символа.
	previewCard := container.NewVBox(
		widget.NewLabelWithStyle("Как это выглядит в строке списка:",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		preview,
		widget.NewSeparator(),
		detail,
		container.NewHBox(copyGlyphBtn, copyLiteralBtn),
		copyStatus,
	)

	win.SetContent(container.NewBorder(
		controls, previewCard, nil, nil,
		container.NewVScroll(grid),
	))

	rebuild()
	win.Show()
}

// newGlyphCell строит ячейку сетки — кликабельный символ.
func newGlyphCell(e glyphEntry, onTap func(glyphEntry)) fyne.CanvasObject {
	txt := canvas.NewText(string(e.R), theme.Color(theme.ColorNameForeground))
	txt.TextSize = glyphExplorerPreviewSize
	txt.Alignment = fyne.TextAlignCenter

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	bg.CornerRadius = 4
	bg.SetMinSize(fyne.NewSize(glyphExplorerCellSize, glyphExplorerCellSize))

	stack := container.NewStack(bg, container.NewCenter(txt))
	return fynewidget.NewTapWrap(stack, func() { onTap(e) })
}

// collectGlyphs собирает символы всех шрифтов темы.
//
// Берём именно ресурсы темы, а не файлы с диска: приложение рисует ими же,
// поэтому каталог показывает ровно то, что будет на экране.
func collectGlyphs() []glyphEntry {
	textFonts := []glyphSource{
		{Name: "Text", Face: parseThemeFont(theme.DefaultTextFont())},
		{Name: "Bold", Face: parseThemeFont(theme.DefaultTextBoldFont())},
		{Name: "Italic", Face: parseThemeFont(theme.DefaultTextItalicFont())},
	}
	emojiFace := parseThemeFont(theme.DefaultEmojiFont())
	symbolFace := parseThemeFont(theme.DefaultSymbolFont())

	// map: символ → запись. Один проход по всему диапазону, который вообще
	// может встретиться в UI (до конца эмодзи-плоскости).
	seen := make(map[rune]*glyphEntry, 2048)

	add := func(r rune, source string, emoji bool) {
		e, ok := seen[r]
		if !ok {
			e = &glyphEntry{R: r, Emoji: emoji}
			seen[r] = e
		}
		if emoji {
			e.Emoji = true
		}
		e.Sources = append(e.Sources, source)
	}

	for cp := rune(0x21); cp <= 0x1FAFF; cp++ {
		if cp >= 0xD800 && cp <= 0xDFFF {
			continue // суррогаты — не символы
		}
		inAllText := len(textFonts) > 0
		for _, f := range textFonts {
			if f.Face == nil {
				continue
			}
			if _, ok := f.Face.NominalGlyph(cp); ok {
				add(cp, f.Name, false)
			} else {
				inAllText = false
			}
		}
		if emojiFace != nil {
			if _, ok := emojiFace.NominalGlyph(cp); ok {
				add(cp, "Emoji", true)
			}
		}
		if symbolFace != nil {
			if _, ok := symbolFace.NominalGlyph(cp); ok {
				add(cp, "Symbols", false)
			}
		}
		if e, ok := seen[cp]; ok {
			e.TextSafe = inAllText
		}
	}

	out := make([]glyphEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].R < out[j].R })
	return out
}

// parseThemeFont разбирает ресурс шрифта темы.
//
// nil означает «шрифт недоступен» — например, сборка с -tags no_emoji вообще
// не содержит эмодзи-шрифта. Вызывающий обязан проверять.
func parseThemeFont(res fyne.Resource) *font.Face {
	if res == nil {
		return nil
	}
	f, err := font.ParseTTF(bytes.NewReader(res.Content()))
	if err != nil {
		return nil
	}
	return f
}
