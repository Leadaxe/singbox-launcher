package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/template"
)

// Гейт «правило без условий» после Dropped-каскада (контракт 1.1.82,
// TEMPLATE_LANG §5.1): пустое значение ключа-условия условием не считается;
// правило, у которого ВСЕ ссылки rule_set висячие, выпадает целиком, даже с
// другими условиями (иначе снятое условие расширило бы совпадение);
// висячее имя рядом с живым просто убирается.
func TestPresetRuleConditionGate(t *testing.T) {
	raw := []byte(`{
		"id": "gate",
		"label": "Gate",
		"rule_set": [{"tag": "a", "type": "remote", "format": "binary", "url": "https://example.com/a.srs"}],
		"rules": [
			{"rule_set": ["a", "missing"], "outbound": "direct"},
			{"rule_set": ["missing"], "port": 443, "outbound": "direct"},
			{"domain_suffix": [], "outbound": "direct"},
			{"port": 443, "outbound": "direct"}
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
	if len(frags.RoutingRules) != 2 {
		t.Fatalf("want 2 rules (narrowed rule_set + port), got %d: %v", len(frags.RoutingRules), frags.RoutingRules)
	}
	rs, _ := frags.RoutingRules[0]["rule_set"].([]interface{})
	if len(rs) != 1 || rs[0] != "gate"+TagSeparator+"a" {
		t.Errorf("rule 0 rule_set = %v, want [gate%sa]", frags.RoutingRules[0]["rule_set"], TagSeparator)
	}
	if _, ok := frags.RoutingRules[1]["port"]; !ok {
		t.Errorf("rule 1 should be the port rule: %v", frags.RoutingRules[1])
	}
	dropped := 0
	for _, w := range warns {
		if tw, ok := w.TemplateWarning(); ok && tw.Code == "template_fragment_dropped" {
			dropped++
		}
	}
	if dropped != 2 {
		t.Errorf("want 2 template_fragment_dropped, got %d: %v", dropped, warns)
	}
}
