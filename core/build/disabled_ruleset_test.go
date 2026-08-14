package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
)

// Регрессия из эксплуатации (SPEC 097): выключенный пресет продолжал тащить
// свои rule_set в конфиг.
//
// Само по себе это мусор, но для remote-таргета — фатально: rule_set
// type=local несёт путь .srs НАШЕЙ машины, на роутере такого файла нет, и
// ядро падает «open …/geosite-category-ads-all-*.srs: no such file or
// directory». Пользователь выключил block-ads и russian, а конфиг всё равно
// требовал их файлы.
//
// Правило-владелец выключено → его routing-правило не эмитится → ссылаться
// на набор некому. Значит и rule_set эмитить нельзя.
func TestDisabledPresetDoesNotEmitRuleSets(t *testing.T) {
	presetsJSON := `[{
		"id": "block-ads",
		"label": "Block ads",
		"rule_set": [
			{"tag": "ads", "type": "inline",
			 "rules": [{"domain_suffix": ["ads.example"]}]}
		],
		"rules": [{"rule_set": ["ads"], "outbound": "block-out"}]
	}]`
	td := makeTestTD(t, presetsJSON)

	body, err := json.Marshal(map[string]interface{}{"vars": map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	newRule := func(enabled bool) state.Rule {
		return state.Rule{Kind: state.RuleKindPreset, Ref: "block-ads", Enabled: enabled, Body: body}
	}

	emit := func(enabled bool) (ruleSets, rules []interface{}) {
		t.Helper()
		ctx := PresetMergeContext{
			Presets: td.Presets,
			Rules:   []state.Rule{newRule(enabled)},
		}
		out, err := MergePresetsIntoRoute(json.RawMessage(`{"rules":[]}`), ctx)
		if err != nil {
			t.Fatalf("merge (enabled=%v): %v", enabled, err)
		}
		var route map[string]interface{}
		if err := json.Unmarshal(out, &route); err != nil {
			t.Fatal(err)
		}
		rs, _ := route["rule_set"].([]interface{})
		rl, _ := route["rules"].([]interface{})
		return rs, rl
	}

	// Выключено: ни правил, ни rule_set.
	offSets, offRules := emit(false)
	if len(offSets) != 0 {
		t.Errorf("disabled preset must emit no rule_set, got %v", offSets)
	}
	if len(offRules) != 0 {
		t.Errorf("disabled preset must emit no rules, got %v", offRules)
	}

	// Включено: и правило, и его rule_set на месте — фикс не должен резать
	// рабочий случай.
	onSets, onRules := emit(true)
	if len(onSets) != 1 {
		t.Errorf("enabled preset must emit its rule_set, got %v", onSets)
	}
	if len(onRules) != 1 {
		t.Errorf("enabled preset must emit its rule, got %v", onRules)
	}
}
