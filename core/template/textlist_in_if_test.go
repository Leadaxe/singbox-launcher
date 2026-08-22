package template

import (
	"encoding/json"
	"testing"
)

// TestTextListSplicesInsideIfWrapper — text_list, подставленный ВНУТРИ
// #if-обёртки в массиве, обязан сплайситься, а не вкладываться.
//
// Частность: `"address": ["@tun_address", {"#if": {"#value": "@tun_address6"}}]`
// в wizard_template. Без splice второй элемент приезжал вложенным массивом
// (["a", ["b"]]), и ядро отвергало конфиг — из-за чего tun_address6 был
// обязан объявляться как text, хотя парный ему tun_address — text_list.
func TestTextListSplicesInsideIfWrapper(t *testing.T) {
	vars := []TemplateVar{
		{Name: "ipv6_enabled", Type: "bool", DefaultValue: VarDefaultValue{Scalar: "true"}},
		{Name: "tun_address", Type: "text_list", DefaultValue: VarDefaultValue{Scalar: "172.16.0.1/30"}},
		{Name: "tun_address6", Type: "text_list", DefaultValue: VarDefaultValue{Scalar: "fd00::1/126\nfd00::2/126"}},
	}
	node := []byte(`{"address":["@tun_address",{"#if":{"#and":["@ipv6_enabled"],"#value":"@tun_address6"}}]}`)

	resolved := ResolveTemplateVarsFor(vars, map[string]string{}, nil, TargetSpec{})
	out, err := SubstituteVarsInJSON(node, vars, resolved, TargetSpec{})
	if err != nil {
		t.Fatalf("подстановка: %v", err)
	}

	var got struct {
		Address []interface{} `json:"address"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("разбор результата: %v", err)
	}
	want := []string{"172.16.0.1/30", "fd00::1/126", "fd00::2/126"}
	if len(got.Address) != len(want) {
		t.Fatalf("address = %s, ожидалось %d плоских элементов", out, len(want))
	}
	for i, w := range want {
		s, ok := got.Address[i].(string)
		if !ok {
			t.Fatalf("address[%d] = %T (%v), ожидалась строка — список вложился вместо splice", i, got.Address[i], got.Address[i])
		}
		if s != w {
			t.Errorf("address[%d] = %q, ожидалось %q", i, s, w)
		}
	}
}

// TestScalarInsideIfWrapperUnchanged — обычный text за #if-обёрткой
// по-прежнему приезжает одним элементом (splice его не трогает).
func TestScalarInsideIfWrapperUnchanged(t *testing.T) {
	vars := []TemplateVar{
		{Name: "on", Type: "bool", DefaultValue: VarDefaultValue{Scalar: "true"}},
		{Name: "one", Type: "text", DefaultValue: VarDefaultValue{Scalar: "value"}},
	}
	node := []byte(`{"a":[{"#if":{"#and":["@on"],"#value":"@one"}}]}`)
	resolved := ResolveTemplateVarsFor(vars, map[string]string{}, nil, TargetSpec{})
	out, err := SubstituteVarsInJSON(node, vars, resolved, TargetSpec{})
	if err != nil {
		t.Fatalf("подстановка: %v", err)
	}
	if string(out) != `{"a":["value"]}` {
		t.Errorf("a = %s, ожидалось {\"a\":[\"value\"]}", out)
	}
}
