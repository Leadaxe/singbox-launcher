// Package emojitag — разбор эмодзи в ИМЕНАХ УЗЛОВ и сборка из них тела
// регулярки.
//
// Провайдеры кладут в теги 🇩🇪, 🚀, ⭐, 🔒 — это такие же маркеры категории,
// как флаг страны, и отбирать по ним должно быть одинаково легко везде, где
// такой отбор предлагается: в пикере флагов формы Направления
// (ui/configurator/outbounds_configurator/flag_picker.go) и в окне фильтров
// списка серверов (ui/servers_filter_window.go).
//
// Пакет отделён от обоих потребителей намеренно: алгоритм «что считать одним
// эмодзи» нетривиален (флаг = ПАРА Regional Indicator, «⭐️» и «⭐»
// различаются невидимым селектором вариации), и вторая его копия разошлась бы
// с первой молча — чипы в одном окне отличались бы от чипов в другом на тех
// же именах.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package emojitag

import (
	"sort"
	"strings"
)

// Entry — один эмодзи и сколько имён его содержат.
type Entry struct {
	Emoji string
	Count int
}

// Find возвращает уникальные эмодзи строки в порядке появления.
//
// Уникальные В РАМКАХ СТРОКИ: имя «🇩🇪 🇩🇪 Berlin» даёт один 🇩🇪, и счётчик
// Count у Entries читается как «сколько УЗЛОВ несут этот значок», а не
// «сколько раз он встретился». Для чипа-фильтра важно первое: клик по нему
// показывает ровно столько строк, сколько написано на чипе.
func Find(s string) []string {
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

// Entries — эмодзи всех имён с частотой, по убыванию частоты; при равенстве —
// по самому эмодзи, чтобы порядок чипов не прыгал между открытиями окна.
func Entries(names []string) []Entry {
	counts := map[string]int{}
	for _, name := range names {
		for _, e := range Find(name) {
			counts[e]++
		}
	}
	out := make([]Entry, 0, len(counts))
	for e, c := range counts {
		out = append(out, Entry{Emoji: e, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Emoji < out[j].Emoji
	})
	return out
}

// BuildORPattern — ТЕЛО регулярки из термов: `a|b|c`.
//
// Тело без обёртки — тот же формат, что у поля фильтра Направления
// (configtypes.DirectionFilterPattern): обёртку и флаг регистра ставит
// потребитель, и выдумывать их здесь нельзя — иначе в поле тела оказался бы
// полный паттерн, и генератор искал бы в тегах символы обёртки.
func BuildORPattern(terms []string) string {
	return strings.Join(terms, "|")
}

// SplitORPattern — термы тела регулярки: разбор, обратный BuildORPattern.
//
// Нужен подсветке чипов: окно должно показать выбранным то, что УЖЕ стоит в
// поле, включая паттерн, набранный руками. Пустые куски и пробелы по краям
// отбрасываются, внешние скобки снимаются — наследие прежнего формата
// `/(🇷🇺)/i`, для сопоставления с чипами они значения не имеют.
func SplitORPattern(body string) []string {
	var out []string
	for _, part := range strings.Split(strings.Trim(body, "()"), "|") {
		part = strings.TrimSpace(strings.Trim(part, "()"))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ToggleInORPattern — клик по чипу: терма в теле нет → добавить в конец, есть
// → убрать. Возвращает новое тело.
func ToggleInORPattern(body, term string) string {
	parts := SplitORPattern(body)
	out := make([]string, 0, len(parts)+1)
	removed := false
	for _, p := range parts {
		if p == term {
			removed = true
			continue
		}
		out = append(out, p)
	}
	if !removed {
		out = append(out, term)
	}
	return BuildORPattern(out)
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
