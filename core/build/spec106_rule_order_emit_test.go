package build

// SPEC 106-B: эмит config.json обязан следовать ОСИ порядка, а не позиции
// правила в state.Rules[]. Потребитель оси (этот файл) жил и раньше —
// проверяем, что он действительно читает номера, которые теперь проставляет
// UI при перетаскивании.

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

func numPtr(v int) *int { return &v }
func falsePtr() *bool   { b := false; return &b }

// emitOrderPresets — минимальный шаблон: несортируемая голова на 0, два
// сортируемых якоря и широкий перехватчик в конце.
func emitOrderPresets() []template.Preset {
	return []template.Preset{
		{
			ID: "traffic-processing", Label: "Traffic", Num: numPtr(0), Sortable: falsePtr(),
			Rules: []map[string]interface{}{{"protocol": "dns", "action": "hijack-dns"}},
		},
		{
			ID: "private-ips", Label: "Private", Num: numPtr(950),
			Rules: []map[string]interface{}{{"ip_is_private": true, "outbound": "direct-out"}},
		},
		{
			ID: "block-ads", Label: "Ads", Num: numPtr(960),
			Rules: []map[string]interface{}{{"domain_suffix": "ads.example", "action": "reject"}},
		},
		{
			ID: "russian", Label: "RU", Num: numPtr(1120),
			Rules: []map[string]interface{}{{"domain_suffix": ".ru", "outbound": "direct-out"}},
		},
	}
}

func inlineStateRule(name string, num int, outbound string) corestate.Rule {
	body, _ := json.Marshal(corestate.InlineBody{
		Name:     name,
		Match:    map[string]interface{}{"domain_suffix": name + ".example"},
		Outbound: outbound,
	})
	return corestate.Rule{Kind: corestate.RuleKindInline, Enabled: true, OrderNum: &num, Body: body}
}

func presetStateRule(ref string, num int) corestate.Rule {
	n := num
	return corestate.Rule{
		Kind: corestate.RuleKindPreset, Ref: ref, Enabled: true,
		OrderNum: &n, Body: json.RawMessage(`{"vars":{}}`),
	}
}

// emittedRuleMarkers — по одному опознавательному признаку на правило в том
// порядке, в котором они легли в route.rules[].
func emittedRuleMarkers(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var route map[string]interface{}
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatalf("разбор route: %v", err)
	}
	rules, _ := route["rules"].([]interface{})
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		m, _ := r.(map[string]interface{})
		switch {
		case m["protocol"] == "dns":
			out = append(out, "traffic-processing")
		case m["ip_is_private"] == true:
			out = append(out, "private-ips")
		case m["domain_suffix"] == "ads.example":
			out = append(out, "block-ads")
		case m["domain_suffix"] == ".ru":
			out = append(out, "russian")
		default:
			ds, _ := m["domain_suffix"].(string)
			out = append(out, "inline:"+ds)
		}
	}
	return out
}

// Тест 3 (обязательный): порядок route.rules следует оси; системное правило
// первым; несортируемый пресет на якоре из шаблона.
//
// state.Rules намеренно перемешан относительно оси — если бы эмит доверял
// позиции в слайсе, порядок вышел бы именно тем, что записан ниже.
func TestEmitFollowsRuleOrderAxis(t *testing.T) {
	ctx := PresetMergeContext{
		Presets: emitOrderPresets(),
		Rules: []corestate.Rule{
			presetStateRule("russian", 1120),
			inlineStateRule("mine", 955, "proxy-out"), // между private-ips и block-ads
			presetStateRule("block-ads", 960),
			presetStateRule("traffic-processing", 0),
			presetStateRule("private-ips", 950),
		},
	}
	out, err := MergePresetsIntoRoute(json.RawMessage(`{"rules":[],"rule_set":[]}`), ctx)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := emittedRuleMarkers(t, out)
	want := []string{"traffic-processing", "private-ips", "inline:mine.example", "block-ads", "russian"}
	if len(got) != len(want) {
		t.Fatalf("эмитнуто %d правил (%v), ожидалось %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок route.rules = %v, ожидался %v", got, want)
		}
	}
	if got[0] != "traffic-processing" {
		t.Error("системное правило не первое — sniff не успеет отработать до матчинга")
	}
}

// Несортируемый пресет садится на якорь шаблона даже если в state он приехал
// без номера (state, записанный до SPEC 106).
func TestEmitAnchorsUnmarkedPresetFromTemplate(t *testing.T) {
	ctx := PresetMergeContext{
		Presets: emitOrderPresets(),
		Rules: []corestate.Rule{
			{Kind: corestate.RuleKindPreset, Ref: "russian", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
			{Kind: corestate.RuleKindPreset, Ref: "traffic-processing", Enabled: true, Body: json.RawMessage(`{"vars":{}}`)},
		},
	}
	out, err := MergePresetsIntoRoute(json.RawMessage(`{"rules":[],"rule_set":[]}`), ctx)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := emittedRuleMarkers(t, out)
	if len(got) == 0 || got[0] != "traffic-processing" {
		t.Fatalf("порядок route.rules = %v, системное правило обязано быть первым", got)
	}
}
