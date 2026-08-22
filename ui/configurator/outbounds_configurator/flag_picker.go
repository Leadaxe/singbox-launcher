// File flag_picker.go — emoji-flag picker popup for the Filter input.
//
// Юзер кликает 🌐 справа от Filters → tag → вылезает popup поверх Edit-окна:
//
//	┌──────────────────────────────────────────────────────────────────────┐
//	│ Flag picker                                                          │
//	├──────────────────────────────────────────────────────────────────────┤
//	│ Available flags (click to toggle):                                   │
//	│  ☐ 🇳🇱 (8)  ☐ 🇺🇸 (12)  ☐ 🇩🇪 (4) ...                                │
//	│  ☐ Exclude these flags instead (negation)                           │
//	│                                                                      │
//	│ Filter regex (editable, live-applied):                               │
//	│  [/(🇳🇱|🇺🇸)/i____________________________________________]          │
//	│                                                                      │
//	│ ▶ matches 20 of 84 total nodes                                       │
//	├──────────────────────────────────────────────────────────────────────┤
//	│ ✓ 🇳🇱 amsterdam-1 — sub-1                       (green: matches)     │
//	│ ✓ 🇳🇱 amsterdam-2 — sub-1                       (green)              │
//	│ ✗ 🇷🇺 moscow      — sub-2                       (red:   excluded)    │
//	│ ✓ 🇺🇸 nyc         — sub-2                       (green)              │
//	│ ...                                                                  │
//	├──────────────────────────────────────────────────────────────────────┤
//	│                                            [Cancel] [Apply]          │
//	└──────────────────────────────────────────────────────────────────────┘
//
// Live: при клике на чип ИЛИ ручной правке regex'а — node-list re-filter'ится,
// зелёные/красные строки обновляются мгновенно.
//
// Используем тот же `config.PreviewSelectorNodes` что Preview-tab — гарантия
// что попадание/непопадание ноды показывается так же, как в финальном emit'е.
package outbounds_configurator

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)

// flagEntry — один флаг + сколько нод его содержит.
type flagEntry struct {
	Flag  string
	Count int
}

// extractFlags — пробегает по тэгам всех нод, собирает уникальные эмодзи с
// count'ами, sorted by count desc.
//
// SPEC 104: не только флаги. Провайдеры кладут в имена 🚀, ⭐, 🔒, 💎 и
// прочее — это такие же маркеры категории, как флаг страны, и отбирать по
// ним должно быть так же легко. Флаг (пара Regional Indicator) остаётся
// одним элементом, а не двумя буквами.
func extractFlags(nodes []*config.ParsedNode) []flagEntry {
	counts := map[string]int{}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for _, f := range findFlagsInString(n.Tag) {
			counts[f]++
		}
	}
	out := make([]flagEntry, 0, len(counts))
	for f, c := range counts {
		out = append(out, flagEntry{Flag: f, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Flag < out[j].Flag
	})
	return out
}

func findFlagsInString(s string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(e string) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// Флаг — пара Regional Indicator, один элемент.
		if i+1 < len(runes) && isRegionalIndicator(r) && isRegionalIndicator(runes[i+1]) {
			add(string(runes[i : i+2]))
			i++
			continue
		}
		if isEmojiRune(r) {
			// Захватываем следующий за эмодзи селектор вариации / модификатор
			// тона кожи, чтобы «⭐️» и «⭐» не считались разными.
			end := i + 1
			for end < len(runes) && isEmojiModifier(runes[end]) {
				end++
			}
			add(string(runes[i:end]))
			i = end - 1
		}
	}
	return out
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// isEmojiRune — основные блоки эмодзи Unicode. Буквы, цифры и пунктуацию
// не трогаем: пикер нужен для символов, которые неудобно набирать.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF: // Misc Symbols & Pictographs … Symbols Extended-A
		return true
	case r >= 0x2600 && r <= 0x27BF: // Misc Symbols, Dingbats (☀ ⭐ ✅ ✈)
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols & Pictographs
		return true
	case r == 0x2B50 || r == 0x2B55 || r == 0x231A || r == 0x231B || r == 0x23F0 || r == 0x23F3:
		return true
	}
	return false
}

// isEmojiModifier — селектор вариации (U+FE0F) и модификаторы тона кожи.
func isEmojiModifier(r rune) bool {
	return r == 0xFE0F || (r >= 0x1F3FB && r <= 0x1F3FF)
}

// buildFlagRegex — ТЕЛО регулярки из выбранных чипов: `flag1|flag2`.
//
// SPEC 104: пикер работает в тех же терминах, что и форма Направления —
// тело без обёртки, инверсия отдельной галкой. Обёртку `/…/i` ставит
// форма при сохранении, и пикеру выдумывать её нельзя: иначе в поле тела
// оказывался бы полный паттерн, и генератор искал бы символы «/» в тегах.
func buildFlagRegex(selected []string) string {
	return strings.Join(selected, "|")
}

// showFlagPickerPopup — modal popup поверх parent-canvas'а. На Apply вызывает
// onApply с финальным regex. Cancel / клик вне → закрыть.
//
// `nodes` = model.PreviewNodes (если nil/empty — chips и list пустые,
// regex-поле всё равно работает).
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

	flags := extractFlags(nodes)
	total := len(nodes)

	// ── State ──────────────────────────────────────────────────────────────
	selected := map[string]bool{}
	excludeCheck := widget.NewCheck(locale.T("wizard.outbound.flag_picker.exclude"), nil)
	excludeCheck.SetChecked(currentInvert)

	regexEntry := widget.NewEntry()
	regexEntry.SetText(currentBody)
	regexEntry.SetPlaceHolder("🇳🇱|🇺🇸")

	countLabel := widget.NewLabel("")
	countLabel.TextStyle = fyne.TextStyle{Bold: true}

	// ── Node list (mirror Preview tab look) ────────────────────────────────
	type listRow struct {
		text  string
		color color.Color
	}
	var rows []listRow

	nodeList := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject { return canvas.NewText("", color.White) },
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
	nodeListScroll := container.NewScroll(nodeList)
	nodeListScroll.SetMinSize(fyne.NewSize(520, 280))

	// recomputeMatches — applies current filter regex via the SAME function
	// as Preview tab (config.PreviewSelectorNodes) and rebuilds the row list.
	recomputeMatches := func() {
		rows = rows[:0]

		// Build synthetic Direction with only the filter — we only care
		// about which nodes match.
		cfg := config.Direction{
			Tag:     "_flag_picker_",
			Type:    "selector",
			Filters: configtypes.SetDirectionFilterTag(nil, regexEntry.Text, excludeCheck.Checked),
		}

		filtered, _ := config.PreviewSelectorNodes(nodes, cfg)
		filteredSet := make(map[*config.ParsedNode]bool, len(filtered))
		for _, n := range filtered {
			filteredSet[n] = true
		}

		matched := len(filtered)
		countLabel.SetText(locale.Tf("wizard.outbound.flag_picker.matches", matched, total))

		// Build rows: matching nodes first, then non-matching. Same color
		// scheme as Preview (green=in, red=out).
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

			var c color.Color
			var prefix string
			if filteredSet[n] {
				c = color.RGBA{R: 0, G: 160, B: 0, A: 255}
				prefix = "✓ "
			} else {
				c = color.RGBA{R: 200, G: 0, B: 0, A: 255}
				prefix = "✗ "
			}
			row := listRow{text: prefix + text, color: c}
			if filteredSet[n] {
				inRows = append(inRows, row)
			} else {
				outRows = append(outRows, row)
			}
		}
		rows = append(rows, inRows...)
		rows = append(rows, outRows...)
		nodeList.Refresh()
	}

	// Initial render.
	recomputeMatches()

	// ── Chip → regex rebuild ───────────────────────────────────────────────
	rebuildFromChips := func() {
		picked := make([]string, 0, len(selected))
		for _, fe := range flags {
			if selected[fe.Flag] {
				picked = append(picked, fe.Flag)
			}
		}
		regexEntry.SetText(buildFlagRegex(picked))
		// SetText triggers OnChanged → recomputeMatches called transitively.
	}
	excludeCheck.OnChanged = func(_ bool) { recomputeMatches() }

	// ── Live-apply on regex edit (manual or chip-driven) ───────────────────
	regexEntry.OnChanged = func(_ string) { recomputeMatches() }

	// ── Chips grid ─────────────────────────────────────────────────────────
	var chipsContent fyne.CanvasObject
	if len(flags) == 0 {
		chipsContent = widget.NewLabel(locale.T("wizard.outbound.flag_picker.no_flags"))
	} else {
		chipObjs := make([]fyne.CanvasObject, 0, len(flags))
		for _, fe := range flags {
			fe := fe
			label := fmt.Sprintf("%s (%d)", fe.Flag, fe.Count)
			chk := widget.NewCheck(label, func(checked bool) {
				selected[fe.Flag] = checked
				rebuildFromChips()
			})
			chipObjs = append(chipObjs, chk)
		}
		// 5 чипов в ряд — компактно.
		chipsContent = container.NewGridWithColumns(5, chipObjs...)
	}

	// ── Layout ─────────────────────────────────────────────────────────────
	header := widget.NewLabelWithStyle(
		locale.T("wizard.outbound.flag_picker.title"),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	cancelBtn := widget.NewButton(locale.T("wizard.outbound.flag_picker.cancel"), nil)
	applyBtn := widget.NewButton(locale.T("wizard.outbound.flag_picker.apply"), nil)
	applyBtn.Importance = widget.HighImportance

	buttonRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(layout.NewSpacer(), cancelBtn, applyBtn),
	)

	// Top stack: header + chips + exclude + regex + count.
	topStack := container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabel(locale.T("wizard.outbound.flag_picker.flags_header")),
		chipsContent,
		excludeCheck,
		widget.NewSeparator(),
		widget.NewLabel(locale.T("wizard.outbound.flag_picker.regex_header")),
		regexEntry,
		countLabel,
		widget.NewSeparator(),
	)

	// Main layout: topStack at top, node-list filling middle, buttons at bottom.
	content := container.NewBorder(
		topStack,
		buttonRow,
		nil,
		nil,
		nodeListScroll,
	)

	// Separate OS-level window (not a popup overlaying parent canvas).
	// User wants to see/move it independently of the Edit-Outbound window.
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	win := app.NewWindow(locale.T("wizard.outbound.flag_picker.title"))
	win.SetContent(content)
	win.Resize(fyne.NewSize(580, 640))
	win.CenterOnScreen()

	cancelBtn.OnTapped = func() { win.Close() }
	applyBtn.OnTapped = func() {
		if onApply != nil {
			onApply(strings.TrimSpace(regexEntry.Text), excludeCheck.Checked)
		}
		win.Close()
	}

	win.Show()
}
