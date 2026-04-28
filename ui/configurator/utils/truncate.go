package utils

// TruncateStringEllipsis returns s unchanged if it has at most maxRunes Unicode code points.
// Otherwise returns a prefix of maxRunes runes followed by ellipsis (typically "...").
func TruncateStringEllipsis(s string, maxRunes int, ellipsis string) string {
	if maxRunes <= 0 {
		return ellipsis
	}
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i] + ellipsis
		}
		n++
	}
	return s
}
