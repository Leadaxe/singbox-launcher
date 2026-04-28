package utils

import "testing"

func TestTruncateStringEllipsis(t *testing.T) {
	tests := []struct {
		s, want string
		max     int
	}{
		{"ascii short", "ascii short", 20},
		{"hello", "hel...", 3},
		{"", "", 5},
		{"Привет", "Пр...", 2},
		{"🇱🇹abc", "🇱🇹ab...", 4},
		{"a»b", "a»...", 2},
	}
	for _, tt := range tests {
		got := TruncateStringEllipsis(tt.s, tt.max, "...")
		if got != tt.want {
			t.Errorf("TruncateStringEllipsis(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}
