package presentation

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 127 (state v8, одно пространство имён). Один интеграционный сценарий на
// весь UI-слой: модель → записи состояния → БАЙТЫ файла → обратно в модель.
//
// Через файл, а не через структуры в памяти: в v8 у DNS-записей больше нет
// кастомного Marshal, который выбрасывал `tag` из тела, — значит лишний ключ
// в `body` виден только в сериализованном виде, и проверять надо его.
//
// Покрывает три вещи, которые волна 1 переносит в UI:
//  1. правила эмитятся в форме v8: `name`/`refs`/`vars`/`num` — поля записи,
//     `body` — правило sing-box с целью внутри, у пресета тела нет;
//  2. пользовательский DNS-сервер кладёт в `body` ТОЛЬКО тело sing-box: тег
//     живёт полем `tag` (иначе он удвоится и разойдётся с миграцией);
//  3. обратный путь (state → UI) возвращает те же подписи, наборы, переменные
//     и позиции оси — то есть переоткрытие конфигуратора ничего не теряет.
func TestSpec127UIStateRoundTripV8(t *testing.T) {
	num := func(v int) *int { return &v }

	// ── UI-модель, какой её видит вкладка Rules ───────────────────────
	presetRefs := []*wizardmodels.PresetRefState{
		{Ref: "russian", Enabled: true, Vars: map[string]string{"out": "proxy-out"}, Num: num(1120)},
	}
	customRules := []*wizardmodels.RuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "4pda",
				Rule: map[string]interface{}{
					"domain_suffix": []interface{}{"4pda.to"},
					"outbound":      "proxy-out",
				},
			},
			Enabled:          true,
			SelectedOutbound: "proxy-out",
			Num:              num(1000),
		},
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "Ad lists",
				Rule:  map[string]interface{}{},
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"type":"remote","format":"binary","url":"https://example.com/a.srs"}`),
					json.RawMessage(`{"type":"remote","format":"binary","url":"https://example.com/b.srs"}`),
				},
			},
			Enabled:          true,
			SelectedOutbound: "reject",
			Num:              num(1010),
		},
	}
	order := []wizardmodels.RuleSlot{
		{Kind: wizardmodels.SlotKindCustom, Index: 0},
		{Kind: wizardmodels.SlotKindCustom, Index: 1},
		{Kind: wizardmodels.SlotKindPresetRef, Index: 0},
	}

	st := &state.State{Rules: wizardmodels.EmitStateRulesInAxisOrder(order, presetRefs, customRules)}

	// Пользовательский DNS-сервер — как его отдаёт вкладка DNS: плоский блоб
	// с тегом и тумблером на верхнем уровне.
	st.DNS = wizardmodels.SyncDNSByOrderToState(
		[]wizardmodels.DNSRuleSlot{{Kind: wizardmodels.DNSSlotKindUser, Index: 0}},
		presetRefs,
		[]wizardmodels.DNSUserRule{{Enabled: true, Body: map[string]interface{}{
			"domain_suffix": ".ts.net", "server": "my-doh",
		}}},
		[]json.RawMessage{json.RawMessage(`{"tag":"my-doh","enabled":true,"type":"https","server":"1.1.1.1","detour":"proxy-out"}`)},
		"",
		nil, nil,
	)

	// ── Байты файла ──────────────────────────────────────────────────
	blob, err := st.MarshalV8()
	if err != nil {
		t.Fatalf("MarshalV8: %v", err)
	}
	var disk struct {
		Rules []map[string]json.RawMessage `json:"rules"`
		DNS   struct {
			Servers []map[string]json.RawMessage `json:"servers"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(blob, &disk); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if len(disk.Rules) != 3 {
		t.Fatalf("правил в файле %d, ожидалось 3: %s", len(disk.Rules), blob)
	}
	// Порядок в файле — осевой (SortRulesByNum на выходе эмиссии).
	wantKinds := []string{`"inline"`, `"srs"`, `"preset"`}
	for i, want := range wantKinds {
		if got := string(disk.Rules[i]["kind"]); got != want {
			t.Errorf("rules[%d].kind = %s, ожидалось %s", i, got, want)
		}
	}
	// inline: имя и номер снаружи, цель внутри тела, `match` как обёртки нет.
	if got := string(disk.Rules[0]["name"]); got != `"4pda"` {
		t.Errorf(`rules[0].name = %s, ожидалось "4pda"`, got)
	}
	if got := string(disk.Rules[0]["num"]); got != "1000" {
		t.Errorf("rules[0].num = %s, ожидалось 1000", got)
	}
	var inlineBody map[string]interface{}
	if err := json.Unmarshal(disk.Rules[0]["body"], &inlineBody); err != nil {
		t.Fatalf("rules[0].body: %v", err)
	}
	if _, has := inlineBody["match"]; has {
		t.Errorf("rules[0].body несёт обёртку match — тело обязано быть правилом sing-box: %v", inlineBody)
	}
	if got, _ := inlineBody["outbound"].(string); got != "proxy-out" {
		t.Errorf("rules[0].body.outbound = %v", inlineBody["outbound"])
	}
	if _, has := inlineBody["domain_suffix"]; !has {
		t.Errorf("rules[0].body потерял матчер: %v", inlineBody)
	}
	// srs: ОБА набора полем записи, цель reject — в форме sing-box.
	var refs []string
	if err := json.Unmarshal(disk.Rules[1]["refs"], &refs); err != nil {
		t.Fatalf("rules[1].refs: %v", err)
	}
	if len(refs) != 2 || refs[0] != "https://example.com/a.srs" || refs[1] != "https://example.com/b.srs" {
		t.Errorf("rules[1].refs = %v — оба набора и их порядок обязаны пережить эмиссию", refs)
	}
	var srsBody map[string]interface{}
	if err := json.Unmarshal(disk.Rules[1]["body"], &srsBody); err != nil {
		t.Fatalf("rules[1].body: %v", err)
	}
	if got, _ := srsBody["action"].(string); got != "reject" {
		t.Errorf("rules[1].body = %v, ожидалась цель action=reject", srsBody)
	}
	if _, has := srsBody["rule_set"]; has {
		t.Errorf("rules[1].body несёт rule_set — его вписывает сборка по refs: %v", srsBody)
	}
	// preset: vars полем записи, тела нет.
	if _, has := disk.Rules[2]["body"]; has {
		t.Errorf("rules[2] (preset) несёт body — в v8 тела у пресета нет")
	}
	var presetVars map[string]string
	if err := json.Unmarshal(disk.Rules[2]["vars"], &presetVars); err != nil {
		t.Fatalf("rules[2].vars: %v", err)
	}
	if len(presetVars) != 1 || presetVars["out"] != "proxy-out" {
		t.Errorf("rules[2].vars = %v", presetVars)
	}

	// DNS-сервер: тег полем, в теле его нет.
	if len(disk.DNS.Servers) != 1 {
		t.Fatalf("DNS-серверов в файле %d, ожидался 1: %s", len(disk.DNS.Servers), blob)
	}
	if got := string(disk.DNS.Servers[0]["tag"]); got != `"my-doh"` {
		t.Errorf("dns.servers[0].tag = %s", got)
	}
	var dnsBody map[string]interface{}
	if err := json.Unmarshal(disk.DNS.Servers[0]["body"], &dnsBody); err != nil {
		t.Fatalf("dns.servers[0].body: %v", err)
	}
	if _, has := dnsBody["tag"]; has {
		t.Errorf("dns.servers[0].body несёт tag — он живёт полем записи: %v", dnsBody)
	}
	if _, has := dnsBody["enabled"]; has {
		t.Errorf("dns.servers[0].body несёт enabled: %v", dnsBody)
	}
	if got, _ := dnsBody["detour"].(string); got != "proxy-out" {
		t.Errorf("dns.servers[0].body потерял detour: %v", dnsBody)
	}

	// ── Обратный путь: файл → состояние → модель ─────────────────────
	back, err := state.Parse(blob)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	gotPresets := wizardmodels.SyncStateRulesToPresetRefs(back.Rules)
	if len(gotPresets) != 1 || gotPresets[0].Ref != "russian" ||
		gotPresets[0].Vars["out"] != "proxy-out" ||
		gotPresets[0].Num == nil || *gotPresets[0].Num != 1120 {
		t.Errorf("preset-ref после перезагрузки: %+v", gotPresets)
	}
	if len(back.CustomRules) != 2 {
		t.Fatalf("legacy-вид правил после перезагрузки: %d, ожидалось 2", len(back.CustomRules))
	}
	if back.CustomRules[0].Label != "4pda" || back.CustomRules[0].SelectedOutbound != "proxy-out" {
		t.Errorf("inline после перезагрузки: %+v", back.CustomRules[0])
	}
	if back.CustomRules[1].Label != "Ad lists" || back.CustomRules[1].SelectedOutbound != "reject" {
		t.Errorf("srs после перезагрузки: %+v", back.CustomRules[1])
	}
	if n := len(back.CustomRules[1].RuleSet); n != 2 {
		t.Errorf("наборов у srs после перезагрузки %d, ожидалось 2 — репорт 1.5.5", n)
	}
	// Ось: номера доехали до модели обратно (без этого drag «откатывается»).
	gotOrder := wizardmodels.RuleOrderFromAxis(back.Rules, gotPresets, customRules, nil)
	if len(gotOrder) != 3 {
		t.Fatalf("слотов после перезагрузки %d, ожидалось 3", len(gotOrder))
	}
	if gotOrder[2].Kind != wizardmodels.SlotKindPresetRef {
		t.Errorf("пресет с номером 1120 обязан быть последним: %+v", gotOrder)
	}

	// ── Провозимые метаданные второй стороны ─────────────────────────
	//
	// `id` правила и `id`/`name` DNS-правила лаунчер не заполняет и не
	// показывает, но обязан не терять (ONE_NAMESPACE §1). Сохранение визарда
	// пересобирает и state.Rules, и state.DNS ЦЕЛИКОМ из модели — значит
	// проверять надо именно круг, а не хранение.
	back.Rules[0].ID = "lx-rule-1"
	back.DNS.Rules = []state.DNSRule{{
		Kind:    state.DNSRuleKindUser,
		ID:      "lx-dns-1",
		Name:    "chat dns",
		Enabled: true,
		Body:    map[string]interface{}{"domain_suffix": ".ts.net", "server": "my-doh"},
	}}

	// Круг DNS: состояние → модель визарда → состояние.
	dnsOrder, dnsUserRules := wizardmodels.DNSRuleOrderFromStateRules(back.DNS.Rules, gotPresets)
	if len(dnsUserRules) != 1 || dnsUserRules[0].ID != "lx-dns-1" || dnsUserRules[0].Name != "chat dns" {
		t.Errorf("метаданные DNS-правила не доехали до модели: %+v", dnsUserRules)
	}
	reDNS := wizardmodels.SyncDNSByOrderToState(
		dnsOrder, gotPresets, dnsUserRules,
		[]json.RawMessage{json.RawMessage(`{"tag":"my-doh","enabled":true,"type":"https","server":"1.1.1.1"}`)},
		"", nil, nil,
	)
	if len(reDNS.Rules) != 1 || reDNS.Rules[0].ID != "lx-dns-1" || reDNS.Rules[0].Name != "chat dns" {
		t.Errorf("сохранение DNS из визарда стёрло метаданные записи: %+v", reDNS.Rules)
	}
	if reDNS.Rules[0].Body["server"] != "my-doh" {
		t.Errorf("тело DNS-правила потеряно: %v", reDNS.Rules[0].Body)
	}
	// В файле метаданные снаружи, а тело — объект sing-box и ничего кроме.
	dnsBlob, err := json.Marshal(reDNS.Rules[0])
	if err != nil {
		t.Fatal(err)
	}
	var dnsDisk map[string]json.RawMessage
	if err := json.Unmarshal(dnsBlob, &dnsDisk); err != nil {
		t.Fatal(err)
	}
	var dnsRuleBody map[string]interface{}
	if err := json.Unmarshal(dnsDisk["body"], &dnsRuleBody); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "name", "kind", "enabled"} {
		if _, dup := dnsRuleBody[k]; dup {
			t.Errorf("метаданные продублированы внутри body DNS-правила (%q): %v", k, dnsRuleBody)
		}
	}

	// Круг правил маршрута: UI-модели поля `id` не имеют, поэтому его
	// возвращает перенос метаданных по identity — тем же путём, что делает
	// сохранение визарда (presenter_state.go).
	reRules := state.CarryRuleMetadata(
		wizardmodels.EmitStateRulesInAxisOrder(order, presetRefs, customRules),
		back.Rules,
	)
	if len(reRules) != 3 || reRules[0].ID != "lx-rule-1" {
		t.Errorf("сохранение стёрло id правила маршрута: %+v", reRules)
	}
}
