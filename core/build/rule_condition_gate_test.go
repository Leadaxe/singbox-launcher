package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/template"
)

// Гейт «правило без условий» (TEMPLATE_LANG §5.1, контракт 1.1.82 и 1.1.100,
// SPEC 152). Пустое значение ключа-условия условием не считается; правило, у
// которого ВСЕ ссылки rule_set висячие, выпадает целиком, даже с другими
// условиями; висячее имя рядом с живым просто убирается. Правило без условий
// по замыслу автора (голый action, литерал без значения) идёт в конфиг с
// template_rule_unconditional; правило, условия которого сняла пустая
// переменная или висячий rule_set, выпадает с template_fragment_dropped.
// Одинаково для route.rules и dns.rules.
func TestPresetRuleConditionGate(t *testing.T) {
	raw := []byte(`{
		"id": "gate",
		"label": "Gate",
		"vars": [{"name": "doms", "type": "text"}],
		"rule_set": [{"tag": "a", "type": "remote", "format": "binary", "url": "https://example.com/a.srs"}],
		"rules": [
			{"rule_set": ["a", "missing"], "outbound": "direct"},
			{"rule_set": ["missing"], "port": 443, "outbound": "direct"},
			{"rule_set": ["missing"], "outbound": "direct"},
			{"domain_suffix": "@doms", "outbound": "direct"},
			{"domain_suffix": [], "outbound": "direct"},
			{"action": "sniff"},
			{"port": 443, "outbound": "direct"}
		],
		"dns_rules": [
			{"rule_set": ["missing"], "server": "remote"},
			{"domain_suffix": "@doms", "server": "remote"},
			{"action": "predefined", "rcode": "NOERROR"},
			{"query_type": ["A"], "server": "remote"}
		]
	}`)
	var p template.Preset
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	frags, warns, ok := ExpandPreset(&p, nil, template.TargetSpec{GOOS: "darwin", GOARCH: "amd64"}.Normalized())
	if !ok {
		t.Fatalf("expand failed: %v", warns)
	}

	got, _ := json.Marshal(frags.RoutingRules)
	want := `[{"outbound":"direct","rule_set":["gate:a"]},` +
		`{"domain_suffix":[],"outbound":"direct"},` +
		`{"action":"sniff"},` +
		`{"outbound":"direct","port":443}]`
	if string(got) != want {
		t.Errorf("route rules:\n got  %s\n want %s", got, want)
	}
	gotDNS, _ := json.Marshal(frags.DNSRules)
	wantDNS := `[{"action":"predefined","rcode":"NOERROR"},{"query_type":["A"],"server":"remote"}]`
	if string(gotDNS) != wantDNS {
		t.Errorf("dns rules:\n got  %s\n want %s", gotDNS, wantDNS)
	}

	count := map[string]map[string]int{}
	for _, w := range warns {
		tw, ok := w.TemplateWarning()
		if !ok {
			continue
		}
		if count[tw.Code] == nil {
			count[tw.Code] = map[string]int{}
		}
		count[tw.Code][tw.Params["kind"]]++
	}
	wantCount := map[string]map[string]int{
		// route: два висячих rule_set + пустая переменная; dns: висячий
		// rule_set + пустая переменная.
		WarnTemplateFragmentDropped: {fragmentKindRule: 3, fragmentKindDNSRule: 2},
		// route: литерал [] и голый sniff; dns: голый predefined.
		WarnTemplateRuleUnconditional: {fragmentKindRule: 2, fragmentKindDNSRule: 1},
	}
	for code, kinds := range wantCount {
		for kind, n := range kinds {
			if count[code][kind] != n {
				t.Errorf("%s %s: got %d, want %d (all: %v)", code, kind, count[code][kind], n, warns)
			}
		}
	}
}
