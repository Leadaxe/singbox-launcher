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
