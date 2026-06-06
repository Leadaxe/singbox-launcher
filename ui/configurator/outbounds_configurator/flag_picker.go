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
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)

// flagEntry — один флаг + сколько нод его содержит.
type flagEntry struct {
	Flag  string
	Count int
}

// extractFlags — пробегает по тэгам всех нод, собирает уникальные emoji-флаги
// (Regional Indicator pairs U+1F1E6..U+1F1FF) с count'ами. Sorted by count desc.
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
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		if isRegionalIndicator(runes[i]) && isRegionalIndicator(runes[i+1]) {
			flag := string(runes[i : i+2])
			if !seen[flag] {
				seen[flag] = true
				out = append(out, flag)
			}
			i++ // skip paired rune
		}
	}
	return out
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// buildFlagRegex — строка фильтра в формате `/(flag1|flag2)/i` (или с `!`-префиксом).
func buildFlagRegex(selected []string, exclude bool) string {
	if len(selected) == 0 {
		return ""
	}
	body := strings.Join(selected, "|")
	if exclude {
		return "!/(" + body + ")/i"
	}
	return "/(" + body + ")/i"
}

// showFlagPickerPopup — modal popup поверх parent-canvas'а. На Apply вызывает
// onApply с финальным regex. Cancel / клик вне → закрыть.
//
// `nodes` = model.PreviewNodes (если nil/empty — chips и list пустые,
// regex-поле всё равно работает).
func showFlagPickerPopup(
	parent fyne.Window,
	nodes []*config.ParsedNode,
	current string,
	onApply func(filter string),
) {
	if parent == nil || parent.Canvas() == nil {
		return
	}

	flags := extractFlags(nodes)
	total := len(nodes)

	// ── State ──────────────────────────────────────────────────────────────
	selected := map[string]bool{}
	excludeCheck := widget.NewCheck(locale.T("wizard.outbound.flag_picker.exclude"), nil)

	regexEntry := widget.NewEntry()
	regexEntry.SetText(current)
	regexEntry.SetPlaceHolder("/(🇳🇱|🇺🇸)/i")

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

		// Build synthetic OutboundConfig with only the filter — we only care
		// about which nodes match.
		cfg := config.OutboundConfig{
			Tag:  "_flag_picker_",
			Type: "selector",
			Filters: map[string]interface{}{
				"tag": regexEntry.Text,
			},
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
		regexEntry.SetText(buildFlagRegex(picked, excludeCheck.Checked))
		// SetText triggers OnChanged → recomputeMatches called transitively.
	}
	excludeCheck.OnChanged = func(_ bool) { rebuildFromChips() }

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
			onApply(strings.TrimSpace(regexEntry.Text))
		}
		win.Close()
	}

	win.Show()
}
