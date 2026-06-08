package configtypes

import "testing"

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		pattern string
		want    bool
	}{
		// Literal (case-sensitive).
		{"literal match", "US-Premium", "US-Premium", true},
		{"literal mismatch", "US-Premium", "US-Basic", false},
		{"literal case-sensitive", "US-Premium", "us-premium", false},
		{"literal empty pattern empty value", "", "", true},
		{"literal empty pattern nonempty value", "x", "", false},

		// Negation literal.
		{"neg-literal differs", "US", "!JP", true},
		{"neg-literal equals", "JP", "!JP", false},
		{"neg-literal empty value differs", "", "!JP", true},

		// Regex /.../i — case-insensitive.
		{"regex match", "Tokyo-01", "/tokyo/i", true},
		{"regex case-insensitive", "TOKYO", "/tokyo/i", true},
		{"regex no match", "Osaka", "/tokyo/i", false},
		{"regex anchored", "premium", "/^prem/i", true},

		// Negation regex !/.../i — case-insensitive.
		{"neg-regex no match returns true", "Osaka", "!/tokyo/i", true},
		{"neg-regex match returns false", "Tokyo", "!/tokyo/i", false},
		{"neg-regex case-insensitive match returns false", "TOKYO", "!/tokyo/i", false},

		// Invalid regex → logged, treated as non-match.
		{"invalid regex returns false", "anything", "/[/i", false},
		{"invalid neg-regex returns false", "anything", "!/[/i", false},

		// A literal that merely starts with '/' but is not a regex form
		// (no trailing /i) is matched literally.
		{"slash literal no trailing i", "/path", "/path", true},
		{"slash literal mismatch", "/other", "/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesPattern(tt.value, tt.pattern); got != tt.want {
				t.Errorf("MatchesPattern(%q, %q) = %v, want %v", tt.value, tt.pattern, got, tt.want)
			}
		})
	}
}
