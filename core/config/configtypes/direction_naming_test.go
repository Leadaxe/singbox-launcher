package configtypes

import "testing"

// Тег занимает первую свободную позицию: после удаления среднего
// направления его номер должен переиспользоваться, а не пропадать.
func TestNextDirectionTagFillsGaps(t *testing.T) {
	if got := NextDirectionTag(nil); got != "vpn-1" {
		t.Fatalf("пустой список: %q", got)
	}
	if got := NextDirectionTag([]string{"vpn-1", "vpn-3"}); got != "vpn-2" {
		t.Fatalf("дырка не заполнена: %q", got)
	}
	if got := NextDirectionTag([]string{"vpn-1", "vpn-2"}); got != "vpn-3" {
		t.Fatalf("%q", got)
	}
	// Чужие теги не мешают: proxy-out не занимает номер.
	if got := NextDirectionTag([]string{"proxy-out", "ru VPN 🇷🇺"}); got != "vpn-1" {
		t.Fatalf("%q", got)
	}
}

// Потолка нет (D-4): десятое направление не должно упираться в лимит LxBox.
func TestNextDirectionTagHasNoCeiling(t *testing.T) {
	used := []string{"vpn-1", "vpn-2", "vpn-3", "vpn-4", "vpn-5",
		"vpn-6", "vpn-7", "vpn-8", "vpn-9", "vpn-10"}
	if got := NextDirectionTag(used); got != "vpn-11" {
		t.Fatalf("одиннадцатое направление должно выдаваться: %q", got)
	}
}

func TestDirectionNumber(t *testing.T) {
	if n, ok := DirectionNumber("vpn-7"); !ok || n != 7 {
		t.Fatalf("(%d,%v)", n, ok)
	}
	for _, tag := range []string{"proxy-out", "vpn-", "vpn-x", "vpn-0", "ru VPN 🇷🇺", ""} {
		if _, ok := DirectionNumber(tag); ok {
			t.Fatalf("%q не должен разбираться как номер", tag)
		}
	}
}

// Имя Направления — это его тег, и только он (контракт 0.9.0). Тест
// держит рубеж: вернись отдельное отображаемое имя — DisplayName начнёт
// расходиться с тем, что показано целью правил, и мы это увидим здесь.
func TestDirectionDisplayAndAutoTag(t *testing.T) {
	d := Direction{Tag: "vpn-1"}
	if d.DisplayName() != "vpn-1" {
		t.Fatalf("имя Направления — его тег, got %q", d.DisplayName())
	}
	if d.AutoTag() != "vpn-1-auto" {
		t.Fatalf("got %q", d.AutoTag())
	}
	if (Direction{}).AutoTag() != "" {
		t.Fatalf("двойник без тега не существует")
	}
	if !d.IsEnabled() {
		t.Fatalf("нулевое значение Disabled обязано означать «включено»")
	}
	d.Disabled = true
	if d.IsEnabled() {
		t.Fatalf("Disabled=true")
	}
}
