package template

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Параметры предупреждений канона и сплайс ветки условного элемента (SPEC 143
// волна 1). Корпус сравнивает только коды; набор параметров по реестру
// (contract/registry/warnings.json) и дедуп по паре (код, параметры)
// проверяются здесь.
func TestCanonTemplateWarningParamsAndSplice(t *testing.T) {
	run := func(t *testing.T, cfg string, vars []TemplateVar, resolved map[string]ResolvedVar) (interface{}, []TemplateWarning) {
		t.Helper()
		out, warnings, err := SubstituteVarsInJSONCanonWarnings([]byte(cfg), vars, resolved, LocalTarget())
		if err != nil {
			t.Fatalf("подстановка: %v", err)
		}
		var got interface{}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("разбор вывода: %v", err)
		}
		return got, warnings
	}
	wantOnly := func(t *testing.T, got []TemplateWarning, want TemplateWarning) {
		t.Helper()
		if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Fatalf("warnings = %#v, ожидалось ровно %#v", got, want)
		}
	}
	decode := func(t *testing.T, s string) interface{} {
		t.Helper()
		var v interface{}
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	t.Run("int_invalid", func(t *testing.T) {
		vars := []TemplateVar{{Name: "mtu", Type: "int"}}
		// Две ссылки на одну переменную — одна запись.
		_, w := run(t, `{"a":"@mtu","b":["@mtu"]}`, vars, map[string]ResolvedVar{"mtu": {Scalar: " abc "}})
		wantOnly(t, w, TemplateWarning{Code: warnIntInvalid, Params: map[string]string{"name": "mtu", "value": "abc"}})
	})

	t.Run("int_clamped", func(t *testing.T) {
		vars := []TemplateVar{{Name: "port", Type: "int"}}
		got, w := run(t, `{"p":"@port"}`, vars, map[string]ResolvedVar{"port": {Scalar: "70000"}})
		wantOnly(t, w, TemplateWarning{Code: warnIntClamped, Params: map[string]string{"name": "port", "value": "70000"}})
		if !reflect.DeepEqual(got, decode(t, `{"p":65535}`)) {
			t.Fatalf("кламп: %v", got)
		}
	})

	t.Run("undeclared", func(t *testing.T) {
		// Опечатка в значении и в предикате — одно имя, одна запись; второе
		// имя с тем же кодом — отдельная запись. @runtime.<неизвестное> —
		// тот же код с полным именем, плейсхолдер остаётся.
		cfg := `{"a":"@typo","b":{"#if":{"and":["@typo"],"value":{"x":1}}},"c":"@other","r":"@runtime.nope"}`
		got, w := run(t, cfg, nil, nil)
		want := map[string]bool{"typo": true, "other": true, "runtime.nope": true}
		if len(w) != len(want) {
			t.Fatalf("warnings = %#v", w)
		}
		for _, x := range w {
			if x.Code != warnVarUndeclared || len(x.Params) != 1 || !want[x.Params["name"]] {
				t.Fatalf("лишний или кривой warning %#v", x)
			}
			delete(want, x.Params["name"])
		}
		m := got.(map[string]interface{})
		if m["a"] != "@typo" || m["r"] != "@runtime.nope" {
			t.Fatalf("плейсхолдер не сохранён: %v", got)
		}
	})

	t.Run("unknown_directive", func(t *testing.T) {
		_, w := run(t, `{"x":{"#future":1,"y":2},"z":{"#future":3}}`, nil, nil)
		wantOnly(t, w, TemplateWarning{Code: warnUnknownDirective, Params: map[string]string{"key": "#future"}})
	})

	vars := []TemplateVar{{Name: "on", Type: "bool"}}
	on := map[string]ResolvedVar{"on": {Scalar: "true"}}

	t.Run("literal_array_branch_splices", func(t *testing.T) {
		got, w := run(t, `{"s":["1.1.1.1",{"#if":{"and":["@on"],"value":["a","b"]}}]}`, vars, on)
		if len(w) != 0 {
			t.Fatalf("warnings = %#v", w)
		}
		if !reflect.DeepEqual(got, decode(t, `{"s":["1.1.1.1","a","b"]}`)) {
			t.Fatalf("сплайс: %v", got)
		}
	})

	t.Run("double_brackets_nest", func(t *testing.T) {
		got, _ := run(t, `{"s":["1.1.1.1",{"#if":{"and":["@on"],"value":[["a","b"]]}}]}`, vars, on)
		if !reflect.DeepEqual(got, decode(t, `{"s":["1.1.1.1",["a","b"]]}`)) {
			t.Fatalf("вложение: %v", got)
		}
	})
}
