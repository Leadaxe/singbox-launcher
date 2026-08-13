package ui

import (
	"image/color"
	"testing"
	"time"

	"singbox-launcher/api"
)

// Авто-обновление обязано сохранять порядок строк и локальные пинги: иначе
// список прыгает под курсором раз в 5 секунд, а измеренные задержки стираются
// нулями из ответа машины (она про них не знает).
func TestMergePreservingOrder(t *testing.T) {
	current := []api.ProxyInfo{
		{Name: "direct-out", Delay: 79},
		{Name: "DE-1", Delay: 123},
		{Name: "NL-1", Delay: 161},
	}
	// Машина отдала тот же состав, но в другом порядке, без пингов, и
	// сменила выбранный узел в группе.
	fresh := []api.ProxyInfo{
		{Name: "NL-1", Now: "x"},
		{Name: "direct-out"},
		{Name: "DE-1"},
	}

	got := mergePreservingOrder(current, fresh)

	wantOrder := []string{"direct-out", "DE-1", "NL-1"}
	if len(got) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("порядок сбит: [%d] = %q, want %q", i, got[i].Name, name)
		}
	}
	for _, p := range got {
		for _, old := range current {
			if old.Name == p.Name && p.Delay != old.Delay {
				t.Errorf("%s: пинг затёрт: %d, want %d", p.Name, p.Delay, old.Delay)
			}
		}
	}
	// Поля с машины применяются.
	if got[2].Now != "x" {
		t.Errorf("Now с машины не применился: %q", got[2].Now)
	}
}

// Состав всё-таки меняется: ушедшие узлы выпадают, новые дописываются в конец,
// чтобы не сдвигать уже видимые строки.
func TestMergePreservingOrderMembership(t *testing.T) {
	current := []api.ProxyInfo{
		{Name: "A", Delay: 10},
		{Name: "B", Delay: 20},
	}
	fresh := []api.ProxyInfo{
		{Name: "A"},
		{Name: "C"},
	}

	got := mergePreservingOrder(current, fresh)

	want := []string{"A", "C"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
	if got[0].Delay != 10 {
		t.Errorf("пинг выжившего узла затёрт: %d", got[0].Delay)
	}
}

// Первое заполнение: показывать нечего, берём ответ машины как есть.
func TestMergePreservingOrderEmptyCurrent(t *testing.T) {
	fresh := []api.ProxyInfo{{Name: "A"}, {Name: "B"}}
	got := mergePreservingOrder(nil, fresh)
	if len(got) != 2 || got[0].Name != "A" || got[1].Name != "B" {
		t.Errorf("got %v, want порядок ответа машины", got)
	}
}

// Градиент пульса: на полном отсчёте значок яркий, к нулю уходит в тусклый,
// и промежуточные шаги лежат между краями. Иначе «дыхание» либо не заметно,
// либо значок пропадает совсем.
func TestStepColor(t *testing.T) {
	dim := color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	bright := color.NRGBA{R: 200, G: 200, B: 200, A: 255}

	full := stepColor(autoRefreshSteps, dim, bright)
	if r, _, _, _ := full.RGBA(); r>>8 != 200 {
		t.Errorf("на полном отсчёте значок не яркий: %v", full)
	}
	zero := stepColor(0, dim, bright)
	if r, _, _, _ := zero.RGBA(); r>>8 != 100 {
		t.Errorf("на нуле значок не тусклый: %v", zero)
	}
	// Монотонность: каждый следующий шаг не ярче предыдущего.
	prev := uint32(0)
	for left := 0; left <= autoRefreshSteps; left++ {
		r, _, _, _ := stepColor(left, dim, bright).RGBA()
		if left > 0 && r < prev {
			t.Errorf("шаг %d ярче предыдущего: %d < %d", left, r, prev)
		}
		prev = r
	}
	// Выход за границы не должен уводить цвет за края.
	if got := stepColor(999, dim, bright); got != full {
		t.Errorf("значение выше отсчёта не прижато к яркому: %v", got)
	}
}

// Полный набор точек обязан совпадать с интервалом опроса: иначе индикатор
// досчитает до нуля раньше или позже самого запроса.
func TestAutoRefreshStepsMatchInterval(t *testing.T) {
	if got := time.Duration(autoRefreshSteps) * time.Second; got != autoRefreshInterval {
		t.Errorf("шагов на %v, а интервал %v", got, autoRefreshInterval)
	}
}
