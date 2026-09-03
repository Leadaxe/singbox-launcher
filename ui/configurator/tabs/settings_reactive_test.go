package tabs

import (
	"testing"

	wizardtemplate "singbox-launcher/core/template"
)

// SPEC 107 §8: индекс подписывает строку только на переменные её гейта и
// обновляет виджет только тогда, когда результат гейта изменился.

func gateVar(name, enable string) wizardtemplate.TemplateVar {
	v := wizardtemplate.TemplateVar{Name: name, Type: "text"}
	if enable != "" {
		v.Enable = []byte(enable)
	}
	return v
}

func TestGateIndexSubscribesOnlyToItsDeps(t *testing.T) {
	idx := newGateIndex()
	idx.subscribe(gateVar("lan_ifaces", `["@gateway_mode"]`), false, func(bool) {})
	idx.subscribe(gateVar("mtu", `["@tun"]`), true, func(bool) {})

	if got := len(idx.affected([]string{"gateway_mode"})); got != 1 {
		t.Errorf("на gateway_mode подписано %d строк, ожидалась 1", got)
	}
	if got := len(idx.affected([]string{"unrelated_var"})); got != 0 {
		t.Errorf("на постороннюю переменную подписано %d строк — индекс не фильтрует", got)
	}
}

func TestGateIndexSkipsVarsWithoutGate(t *testing.T) {
	idx := newGateIndex()
	idx.subscribe(gateVar("plain", ""), true, func(bool) {})
	if len(idx.all) != 0 {
		t.Error("строка без гейта не должна подписываться: её состояние ни от чего не зависит")
	}
}

func TestGateIndexBatchUpdatesRowOnce(t *testing.T) {
	// Каскад on_change меняет две переменные за клик; строка, зависящая от
	// обеих, обязана пересчитаться ОДИН раз (§8.3).
	idx := newGateIndex()
	calls := 0
	idx.subscribe(gateVar("row", `{"or":["@a","@b"]}`), false, func(bool) { calls++ })

	vars := map[string]wizardtemplate.TemplateVar{
		"row": gateVar("row", `{"or":["@a","@b"]}`),
		"a":   {Name: "a", Type: "bool"},
		"b":   {Name: "b", Type: "bool"},
	}
	res := map[string]wizardtemplate.ResolvedVar{
		"a": {Scalar: "true"}, "b": {Scalar: "true"},
	}
	idx.recompute([]string{"a", "b"}, vars, res, wizardtemplate.LocalTarget())
	if calls != 1 {
		t.Errorf("виджет обновлён %d раз(а), ожидался 1 — батч не схлопнут", calls)
	}
}

func TestGateIndexSkipsUnchangedResult(t *testing.T) {
	// Пересчёт есть, но значение то же — виджет не трогаем.
	idx := newGateIndex()
	calls := 0
	idx.subscribe(gateVar("row", `["@a"]`), true, func(bool) { calls++ })

	vars := map[string]wizardtemplate.TemplateVar{
		"row": gateVar("row", `["@a"]`),
		"a":   {Name: "a", Type: "bool"},
	}
	res := map[string]wizardtemplate.ResolvedVar{"a": {Scalar: "true"}}
	if n := idx.recompute([]string{"a"}, vars, res, wizardtemplate.LocalTarget()); n != 0 {
		t.Errorf("обновлено %d строк при неизменившемся гейте", n)
	}
	if calls != 0 {
		t.Error("виджет тронут вхолостую")
	}
}

func TestGateIndexAppliesBothDirections(t *testing.T) {
	// Гейт должен и гасить, и ВОЗВРАЩАТЬ строку — прежний код умел только
	// Disable при первичной сборке.
	idx := newGateIndex()
	var states []bool
	idx.subscribe(gateVar("row", `["@a"]`), false, func(on bool) { states = append(states, on) })

	vars := map[string]wizardtemplate.TemplateVar{
		"row": gateVar("row", `["@a"]`),
		"a":   {Name: "a", Type: "bool"},
	}
	on := map[string]wizardtemplate.ResolvedVar{"a": {Scalar: "true"}}
	off := map[string]wizardtemplate.ResolvedVar{"a": {Scalar: "false"}}
	tgt := wizardtemplate.LocalTarget()

	idx.recompute([]string{"a"}, vars, on, tgt)
	idx.recompute([]string{"a"}, vars, off, tgt)
	if len(states) != 2 || !states[0] || states[1] {
		t.Errorf("последовательность состояний %v, ожидалась [true false]", states)
	}
}

// subscribeOn: строка без гейта в шаблоне всё равно обязана пересчитываться по
// внешней зависимости. Так подписан bind_interface на auto_detect_interface —
// приоритет диктует ядро, а не шаблон, и через GateDeps эта строка не
// подписалась бы ни на что.
func TestGateIndexSubscribeOnExternalDep(t *testing.T) {
	idx := newGateIndex()
	var states []bool
	recalc := func(res map[string]wizardtemplate.ResolvedVar) bool {
		return interfaceRowEnabled(true, res)
	}
	idx.subscribeOn([]string{"auto_detect_interface"}, false, recalc,
		func(on bool) { states = append(states, on) })

	vars := map[string]wizardtemplate.TemplateVar{
		"auto_detect_interface": {Name: "auto_detect_interface", Type: "bool"},
	}
	off := map[string]wizardtemplate.ResolvedVar{"auto_detect_interface": {Scalar: "false"}}
	on := map[string]wizardtemplate.ResolvedVar{"auto_detect_interface": {Scalar: "true"}}
	tgt := wizardtemplate.LocalTarget()

	// Галку сняли — поле включается; поставили обратно — гаснет.
	idx.recompute([]string{"auto_detect_interface"}, vars, off, tgt)
	idx.recompute([]string{"auto_detect_interface"}, vars, on, tgt)
	if len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("последовательность состояний %v, ожидалась [true false]", states)
	}
}

// Регрессия: subscribeOn-подписчик не имеет varName, и общий пересчёт по
// varByName выбросил бы его молча (`continue` на отсутствующей переменной).
func TestGateIndexSubscribeOnIgnoresVarByName(t *testing.T) {
	idx := newGateIndex()
	calls := 0
	idx.subscribeOn([]string{"a"}, false,
		func(map[string]wizardtemplate.ResolvedVar) bool { return true },
		func(bool) { calls++ })

	// varByName ПУСТ: у подписки собственной переменной нет вовсе.
	idx.recompute([]string{"a"}, map[string]wizardtemplate.TemplateVar{},
		map[string]wizardtemplate.ResolvedVar{}, wizardtemplate.LocalTarget())
	if calls != 1 {
		t.Fatalf("подписчик обновлён %d раз(а), ожидался 1 — recalc не вызвана", calls)
	}
}

// Живой шаблон: gateway_include_interface зависит ровно от tun и gateway_mode.
func TestBundledTemplateGateDeps(t *testing.T) {
	td := loadBundledTemplate(t)
	v, ok := wizardtemplate.VarByName(td.Vars, "gateway_include_interface")
	if !ok {
		t.Skip("переменной нет в шаблоне")
	}
	deps := v.GateDeps()
	if len(deps) != 2 || deps[0] != "gateway_mode" || deps[1] != "tun" {
		t.Errorf("deps=%v, ожидалось [gateway_mode tun]", deps)
	}
}

// Регрессия: батч обязан включать САМУ изменённую переменную, а не только
// цели каскада on_change. Иначе поле, гейт которого зависит от переключателя,
// не пересчитывается — «Enable IPv6» включён, а «TUN IPv6 address» приглушён.
func TestGateBatchIncludesChangedVarItself(t *testing.T) {
	idx := newGateIndex()
	var states []bool
	// tun_address6 зависит от tun и ipv6_enabled — как в живом шаблоне.
	idx.subscribe(gateVar("tun_address6", `["@tun","@ipv6_enabled"]`), false,
		func(on bool) { states = append(states, on) })

	vars := map[string]wizardtemplate.TemplateVar{
		"tun_address6": gateVar("tun_address6", `["@tun","@ipv6_enabled"]`),
		"tun":          {Name: "tun", Type: "bool"},
		"ipv6_enabled": {Name: "ipv6_enabled", Type: "bool"},
	}
	res := map[string]wizardtemplate.ResolvedVar{
		"tun": {Scalar: "true"}, "ipv6_enabled": {Scalar: "true"},
	}
	// Батч содержит только саму переключённую переменную (её on_change мог
	// не тронуть ничего постороннего).
	idx.recompute([]string{"ipv6_enabled"}, vars, res, wizardtemplate.LocalTarget())
	if len(states) != 1 || !states[0] {
		t.Errorf("поле не включилось: states=%v", states)
	}
}
