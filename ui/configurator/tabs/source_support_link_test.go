package tabs

import "testing"

func TestIsTelegramURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://t.me/libertyvpn", true},
		{"http://t.me/foo", true},
		{"https://telegram.me/foo", true},
		{"https://telegram.org/faq", true},
		{"tg://resolve?domain=foo_bot", true},
		{"https://sub.t.me/foo", true},
		{"https://example.com/support", false},
		{"https://nott.me.evil.com/x", false},
		{"https://mybilling.io/t.me", false}, // path mentions t.me, host doesn't
		{"", false},
	}
	for _, c := range cases {
		if got := isTelegramURL(c.url); got != c.want {
			t.Errorf("isTelegramURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}
