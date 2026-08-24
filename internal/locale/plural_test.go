package locale

import "testing"

func TestRuPluralResolver(t *testing.T) {
	r := RuPluralResolver{}
	cases := map[int]string{
		1: "one", 21: "one", 101: "one", 1001: "one",
		2: "few", 3: "few", 4: "few", 22: "few", 24: "few", 102: "few",
		0: "many", 5: "many", 11: "many", 12: "many", 14: "many",
		19: "many", 25: "many", 100: "many", 111: "many", 112: "many",
	}
	for n, want := range cases {
		if got := r.Resolve(n); got != want {
			t.Errorf("Ru(%d) = %q, want %q", n, got, want)
		}
	}
	if got := r.Resolve(-2); got != "few" {
		t.Errorf("Ru(-2) = %q, want few", got)
	}
	if len(r.Forms()) != 4 {
		t.Errorf("Ru forms = %v", r.Forms())
	}
}

func TestEnPluralResolver(t *testing.T) {
	r := EnPluralResolver{}
	cases := map[int]string{1: "one", -1: "one", 0: "other", 2: "other", 21: "other"}
	for n, want := range cases {
		if got := r.Resolve(n); got != want {
			t.Errorf("En(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestResolverFor(t *testing.T) {
	if _, ok := resolverFor("ru").(RuPluralResolver); !ok {
		t.Error("ru must get RuPluralResolver")
	}
	if _, ok := resolverFor("en").(EnPluralResolver); !ok {
		t.Error("en must get EnPluralResolver")
	}
	if _, ok := resolverFor("de").(EnPluralResolver); !ok {
		t.Error("unknown language falls back to EnPluralResolver")
	}
}
