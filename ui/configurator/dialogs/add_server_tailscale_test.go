package dialogs

import "testing"

// Разбор анонсируемых маршрутов — единственный рубеж перед []netip.Prefix в
// ядре: мусор отсюда свалил бы конфиг целиком.
func TestParseTailscalePrefixList(t *testing.T) {
	got, err := parseTailscalePrefixList("192.168.10.5/24, 10.0.0.0/8 172.16.0.0/12")
	if err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	want := []string{"192.168.10.0/24", "10.0.0.0/8", "172.16.0.0/12"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %s, got %s", i, want[i], got[i])
		}
	}

	for _, bad := range []string{"not-a-cidr", "192.168.1.1", "0.0.0.0/0", "::/0"} {
		if _, err := parseTailscalePrefixList(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}

	if got, err := parseTailscalePrefixList("  "); err != nil || len(got) != 0 {
		t.Errorf("empty input: %v %v", got, err)
	}
}

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
