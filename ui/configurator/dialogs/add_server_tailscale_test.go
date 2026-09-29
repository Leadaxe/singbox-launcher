package dialogs

import "testing"

func TestSplitTailscaleList(t *testing.T) {
	got := splitTailscaleList(" tag:server, tag:home\ttag:lab\n")
	want := []string{"tag:server", "tag:home", "tag:lab"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %s, got %s", i, want[i], got[i])
		}
	}
}

// Пустой тег даёт знак по умолчанию, как в LxBox; свой тег не трогается.
func TestTailscaleTag_Default(t *testing.T) {
	cases := map[string]string{
		"":          "\U0001F578\uFE0F tailscale",
		"   ":       "\U0001F578\uFE0F tailscale",
		" home ":    "home",
		"🇩🇪 berlin": "🇩🇪 berlin",
	}
	for in, want := range cases {
		if got := tailscaleTag(in); got != want {
			t.Errorf("tailscaleTag(%q) = %q, want %q", in, got, want)
		}
	}
}
