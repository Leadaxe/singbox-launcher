package configtypes

import "testing"

// Ссылка на переменную обязана нести «@»: без неё подстановка видит обычную
// строку, оставляет её в конфиге как есть, и ядро бракует
// «urltest_tolerance» на месте числа.
//
// Вызывающие передают имя в обоих видах: форма автогруппы срезает «@» перед
// вызовом, шаблон отдаёт его целиком.
func TestNewTemplateVarKeepsAtSign(t *testing.T) {
	for _, in := range []string{"urltest_tolerance", "@urltest_tolerance", "  @urltest_tolerance  "} {
		got := NewTemplateVar(in)
		v, _ := got.Value().(string)
		if v != "@urltest_tolerance" {
			t.Errorf("NewTemplateVar(%q) → %q, ожидалось %q", in, v, "@urltest_tolerance")
		}
		if n, ok := got.Int(); ok {
			t.Errorf("NewTemplateVar(%q) прочиталось числом %d — это ссылка, а не число", in, n)
		}
	}
}

func TestNewTemplateVarEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "@"} {
		if got := NewTemplateVar(in); !got.IsZero() {
			t.Errorf("NewTemplateVar(%q) → %v, ожидалось пустое значение", in, got.Value())
		}
	}
}
