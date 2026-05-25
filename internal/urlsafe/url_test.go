package urlsafe

import "testing"

func TestIsSafeAnnounceURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/billing":      true,
		"http://example.com/":              true,
		"tg://resolve?domain=nash_vpn_bot": true,
		"tg://join?invite=abc":             true,

		// hostile / unsupported
		"javascript:alert(1)":     false,
		"file:///etc/passwd":      false,
		"data:text/html,<script>": false,
		"mailto:foo@example.com":  false,
		"ftp://example.com/x":     false,
		"tg://":                   false,
		"https://":                false,
		"":                        false,
		"   ":                     false,
		"not a url at all %%":     false,
	}
	for in, want := range cases {
		got := IsSafeAnnounceURL(in)
		if got != want {
			t.Errorf("IsSafeAnnounceURL(%q) = %v, want %v", in, got, want)
		}
	}
}
