package textnorm

import "strings"

// NormalizeProxyDisplay repairs invalid UTF-8 and replaces Unicode angle quotation marks
// common in subscription/outbound tags (U+276F ❯, U+00BB », U+203A ›) with ASCII " > ".
// Also replaces Fyne-unrenderable geometric shape emojis (U+1F7E0..U+1F7EB —
// 🟠🟡🟢🔴🟣🟤🟥🟧🟨🟩🟦🟪🟫) with ASCII "*". Fyne v2 doesn't support COLRv1
// color font features for these single-codepoint emojis, so they otherwise
// display as U+FFFD `�` replacement character.
// Collapses runs of spaces and trims ends. Safe for tags sent to sing-box and for UI labels.
func NormalizeProxyDisplay(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToValidUTF8(s, "")
	s = strings.ReplaceAll(s, "❯", " > ") // ❯ HEAVY RIGHT-POINTING ANGLE QUOTATION MARK
	s = strings.ReplaceAll(s, "»", " > ") // » RIGHT-POINTING DOUBLE ANGLE QUOTATION MARK
	s = strings.ReplaceAll(s, "›", " > ") // › SINGLE RIGHT-POINTING ANGLE QUOTATION MARK
	// Geometric shape emojis (colored circles + squares, U+1F7E0..U+1F7EB).
	// Fyne v2 doesn't render them — show ASCII "*" instead of `�`.
	s = strings.Map(func(r rune) rune {
		if r >= 0x1F7E0 && r <= 0x1F7EB {
			return '*'
		}
		return r
	}, s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
