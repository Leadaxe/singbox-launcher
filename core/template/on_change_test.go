package template

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

// mustVar декодирует один TemplateVar из JSON-литерала (удобно для on_change,
// который сам по себе json.RawMessage).
func mustVar(t *testing.T, raw string) TemplateVar {
	t.Helper()
	var v TemplateVar
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("mustVar: %v\nraw=%s", err, raw)
	}
	return v
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestApplyOnChange_simpleChainTwoTargets(t *testing.T) {
	// ipv6_enabled → dns_strategy + resolve_strategy (пример из задачи).
	ipv6 := mustVar(t, `{
		"name": "ipv6_enabled", "type": "bool",
		"on_change": {"set": {
			"@dns_strategy": {"#if": {"and": ["@ipv6_enabled"], "value": "prefer_ipv4", "else": "ipv4_only"}},
			"@resolve_strategy": {"#if": {"and": ["@ipv6_enabled"], "value": "prefer_ipv4", "else": "ipv4_only"}}
		}}
	}`)
	dns := mustVar(t, `{"name": "dns_strategy", "type": "enum"}`)
	resolve := mustVar(t, `{"name": "resolve_strategy", "type": "enum"}`)
	vars := []TemplateVar{ipv6, dns, resolve}

	state := map[string]string{"ipv6_enabled": "true"}
	touched := ApplyOnChange("ipv6_enabled", vars, state, LocalTarget())

	if got := sortedCopy(touched); !reflect.DeepEqual(got, []string{"dns_strategy", "resolve_strategy"}) {
		t.Fatalf("touched = %v", touched)
	}
	if state["dns_strategy"] != "prefer_ipv4" {
		t.Fatalf("dns_strategy = %q", state["dns_strategy"])
	}
	if state["resolve_strategy"] != "prefer_ipv4" {
		t.Fatalf("resolve_strategy = %q", state["resolve_strategy"])
	}

	// Ветка false → else.
	state2 := map[string]string{"ipv6_enabled": "false"}
	ApplyOnChange("ipv6_enabled", vars, state2, LocalTarget())
	if state2["dns_strategy"] != "ipv4_only" || state2["resolve_strategy"] != "ipv4_only" {
		t.Fatalf("false branch: %+v", state2)
	}
}

func TestApplyOnChange_recursiveChain(t *testing.T) {
	// A меняет B; у B свой on_change, меняющий C. Проверяем, что цепочка
	// доходит до C.
	a := mustVar(t, `{
		"name": "a", "type": "bool",
		"on_change": {"set": {"@b": {"#if": {"and": ["@a"], "value": "b-on", "else": "b-off"}}}}
	}`)
	b := mustVar(t, `{
		"name": "b", "type": "text",
		"on_change": {"set": {"@c": {"#if": {"and": ["@a"], "value": "c-on", "else": "c-off"}}}}
	}`)
	c := mustVar(t, `{"name": "c", "type": "text"}`)
	vars := []TemplateVar{a, b, c}

	state := map[string]string{"a": "true"}
	touched := ApplyOnChange("a", vars, state, LocalTarget())

	if got := sortedCopy(touched); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("touched = %v", touched)
	}
	if state["b"] != "b-on" {
		t.Fatalf("b = %q", state["b"])
	}
	if state["c"] != "c-on" {
		t.Fatalf("c = %q", state["c"])
	}
}

func TestApplyOnChange_fixpointStopsRecursion(t *testing.T) {
	// B уже равна значению, которое пере-вычислит on_change — запись не
	// происходит и рекурсия в on_change B не идёт (следим по C, за которую
	// отвечает on_change B — она не должна тронуться).
	a := mustVar(t, `{
		"name": "a", "type": "bool",
		"on_change": {"set": {"@b": {"#if": {"and": ["@a"], "value": "same", "else": "same"}}}}
	}`)
	b := mustVar(t, `{
		"name": "b", "type": "text",
		"on_change": {"set": {"@c": {"#if": {"and": ["@a"], "value": "c-touched", "else": "c-touched"}}}}
	}`)
	c := mustVar(t, `{"name": "c", "type": "text"}`)
	vars := []TemplateVar{a, b, c}

	state := map[string]string{"a": "true", "b": "same"} // b уже "same" — совпадёт с пере-вычисленным
	touched := ApplyOnChange("a", vars, state, LocalTarget())

	if len(touched) != 0 {
		t.Fatalf("expected no writes (fixpoint), got touched=%v state=%+v", touched, state)
	}
	if _, ok := state["c"]; ok {
		t.Fatalf("on_change of b must not have run: c=%q", state["c"])
	}
}

func TestApplyOnChange_cycleGuard(t *testing.T) {
	// A меняет B, B меняет A — оба on_change всегда пишут РАЗНОЕ значение
	// (без этого guard'а рекурсия ушла бы в бесконечность). Хотим: функция
	// завершается (не виснет/не паникует) и останавливается по guard'у, а не
	// по глубине в тысячи кадров.
	a := mustVar(t, `{
		"name": "a", "type": "text",
		"on_change": {"set": {"@b": {"#if": {"and": ["@toggle"], "value": "b-from-a", "else": "b-from-a"}}}}
	}`)
	b := mustVar(t, `{
		"name": "b", "type": "text",
		"on_change": {"set": {"@a": {"#if": {"and": ["@toggle"], "value": "a-from-b", "else": "a-from-b"}}}}
	}`)
	toggle := mustVar(t, `{"name": "toggle", "type": "bool"}`)
	vars := []TemplateVar{a, b, toggle}

	state := map[string]string{"a": "start", "toggle": "true"}

	done := make(chan []string, 1)
	go func() {
		done <- ApplyOnChange("a", vars, state, LocalTarget())
	}()
	select {
	case touched := <-done:
		if len(touched) == 0 {
			t.Fatalf("expected the cycle to alternate writes before the guard trips")
		}
		// Обе var должны быть в согласованном терминальном состоянии (не
		// важно, в каком именно — важно что цикл оборвался).
		if state["a"] != "start" && state["a"] != "a-from-b" {
			t.Fatalf("a in unexpected state: %q", state["a"])
		}
		if state["b"] != "b-from-a" {
			t.Fatalf("b in unexpected state: %q", state["b"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ApplyOnChange did not terminate — cycle guard failed")
	}
}

func TestApplyOnChange_noOnChangeIsNoop(t *testing.T) {
	plain := mustVar(t, `{"name": "plain", "type": "bool"}`)
	other := mustVar(t, `{"name": "other", "type": "text"}`)
	vars := []TemplateVar{plain, other}

	state := map[string]string{"plain": "true", "other": "unchanged"}
	touched := ApplyOnChange("plain", vars, state, LocalTarget())

	if len(touched) != 0 {
		t.Fatalf("touched = %v, want none", touched)
	}
	if state["other"] != "unchanged" {
		t.Fatalf("other mutated: %q", state["other"])
	}
}

func TestApplyOnChange_unevaluableIfLeavesTargetUntouched(t *testing.T) {
	// #if без and/or-совпадения с false и без else → ветка не выбрана (see
	// selectIfBranch): EvalIfScalar возвращает ok=false, цель не трогаем.
	v := mustVar(t, `{
		"name": "v", "type": "bool",
		"on_change": {"set": {"@target": {"#if": {"and": ["@v"], "value": "on"}}}}
	}`)
	target := mustVar(t, `{"name": "target", "type": "text"}`)
	vars := []TemplateVar{v, target}

	state := map[string]string{"v": "false"} // false без else → branch не выбрана
	touched := ApplyOnChange("v", vars, state, LocalTarget())

	if len(touched) != 0 {
		t.Fatalf("touched = %v, want none", touched)
	}
	if _, ok := state["target"]; ok {
		t.Fatalf("target must stay unset, got %q", state["target"])
	}
}
