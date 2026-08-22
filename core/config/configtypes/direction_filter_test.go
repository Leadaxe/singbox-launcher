package configtypes

import "testing"

// Форма показывает тело регулярки, а хранится полный паттерн — проверяем,
// что круг замыкается для всех форм, которые пользователь реально увидит.
func TestDirectionFilterRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		invert  bool
		pattern string
	}{
		{"простое тело", "^DE-", false, "/^DE-/i"},
		{"инверсия", "🇷🇺", true, "!/🇷🇺/i"},
		{"эмодзи-альтернатива", "🇩🇪|🇳🇱", false, "/🇩🇪|🇳🇱/i"},
		{"кириллица", "Германия", false, "/Германия/i"},
		{"пусто", "", false, ""},
		{"пусто с инверсией всё равно пусто", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DirectionFilterPattern(c.body, c.invert)
			if got != c.pattern {
				t.Fatalf("Pattern(%q, %v) = %q, want %q", c.body, c.invert, got, c.pattern)
			}
			if c.pattern == "" {
				return
			}
			body, invert, ok := DirectionFilterBody(c.pattern)
			if !ok || body != c.body || invert != c.invert {
				t.Fatalf("Body(%q) = (%q, %v, %v), want (%q, %v, true)",
					c.pattern, body, invert, ok, c.body, c.invert)
			}
		})
	}
}

// Пробелы вокруг тела — частый результат копипасты; они не должны попадать
// в регулярку, где значат буквальный пробел.
func TestDirectionFilterTrimsBody(t *testing.T) {
	if got := DirectionFilterPattern("  ^DE-  ", false); got != "/^DE-/i" {
		t.Fatalf("got %q", got)
	}
	if got := DirectionFilterPattern("   ", false); got != "" {
		t.Fatalf("пробелы должны читаться как пустой фильтр, got %q", got)
	}
}

// Чужие формы паттерна (литерал, /re/ без флага) не должны теряться: форма
// показывает их текстом, а сохранение канонизирует.
func TestDirectionFilterBodyForeignForms(t *testing.T) {
	body, invert, ok := DirectionFilterBody("/^DE-/")
	if !ok || body != "^DE-" || invert {
		t.Fatalf("regex без флага i: (%q,%v,%v)", body, invert, ok)
	}
	body, invert, ok = DirectionFilterBody("proxy-out")
	if ok {
		t.Fatalf("литерал не должен считаться regex-формой")
	}
	if body != "proxy-out" || invert {
		t.Fatalf("литерал потерян: (%q,%v)", body, invert)
	}
	body, invert, ok = DirectionFilterBody("!literal")
	if ok || body != "literal" || !invert {
		t.Fatalf("!литерал: (%q,%v,%v)", body, invert, ok)
	}
}

// Ключ tag правится, остальные ключи фильтра переживают сохранение формы.
func TestSetDirectionFilterTagKeepsOtherKeys(t *testing.T) {
	in := map[string]interface{}{"tag": "/old/i", "host": "example.com"}

	out := SetDirectionFilterTag(in, "^DE-", false)
	if out["tag"] != "/^DE-/i" {
		t.Fatalf("tag не переписан: %v", out["tag"])
	}
	if out["host"] != "example.com" {
		t.Fatalf("чужой ключ потерян: %v", out)
	}

	// Пустое тело убирает только tag.
	out = SetDirectionFilterTag(in, "", false)
	if _, has := out["tag"]; has {
		t.Fatalf("пустое тело должно убирать ключ tag: %v", out)
	}
	if out["host"] != "example.com" {
		t.Fatalf("чужой ключ потерян при очистке: %v", out)
	}

	// Когда не осталось ничего — nil, а не пустой объект в state.json.
	if got := SetDirectionFilterTag(map[string]interface{}{"tag": "/x/i"}, "", false); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

// Чтение фильтра направления игнорирует чужие ключи и не падает на nil.
func TestDirectionFilterTag(t *testing.T) {
	body, invert := DirectionFilterTag(nil)
	if body != "" || invert {
		t.Fatalf("nil-фильтр: (%q,%v)", body, invert)
	}
	body, invert = DirectionFilterTag(map[string]interface{}{"host": "x"})
	if body != "" || invert {
		t.Fatalf("фильтр без tag: (%q,%v)", body, invert)
	}
	body, invert = DirectionFilterTag(map[string]interface{}{"tag": "!/🇷🇺/i"})
	if body != "🇷🇺" || !invert {
		t.Fatalf("(%q,%v)", body, invert)
	}
}

// Паттерн, собранный формой, обязан работать в общем матчере — иначе форма
// пишет то, что генератор не понимает.
func TestDirectionFilterPatternMatchesThroughMatcher(t *testing.T) {
	p := DirectionFilterPattern("🇩🇪|🇳🇱", false)
	if !MatchesPattern("AL:🇩🇪 Frankfurt", p) {
		t.Fatalf("эмодзи не совпал через MatchesPattern: %q", p)
	}
	if MatchesPattern("AL:🇷🇺 Moscow", p) {
		t.Fatalf("лишнее совпадение")
	}

	// Регистр не учитывается — это свойство направления, не выбор.
	if !MatchesPattern("al:de-frankfurt", DirectionFilterPattern("DE-FRANKFURT", false)) {
		t.Fatalf("регистр должен игнорироваться")
	}

	inv := DirectionFilterPattern("🇷🇺", true)
	if MatchesPattern("AL:🇷🇺 Moscow", inv) {
		t.Fatalf("инверсия не сработала")
	}
	if !MatchesPattern("AL:🇩🇪 Frankfurt", inv) {
		t.Fatalf("инверсия отбросила лишнее")
	}
}
