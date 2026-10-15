package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
)

// TestSrsRuleKeepsAxisPosition — srs-правило встаёт в route.rules по своему
// номеру оси, как inline. Без номера оно считалось системным якорем (num 0 <
// UserRuleNumStart) и уходило в голову списка — выше пользовательских правил,
// стоящих раньше него в списке Rules.
func TestSrsRuleKeepsAxisPosition(t *testing.T) {
	n1, n2 := 1000, 1010
	inline := state.NewInlineRule("first-inline", map[string]interface{}{"domain_suffix": []interface{}{"a.com"}}, "direct-out")
	inline.Enabled, inline.Num = true, &n1
	srs := state.NewSrsRule("second-srs", []string{"https://x/list.srs"}, "proxy-out")
	srs.Enabled, srs.Num = true, &n2
	ctx := PresetMergeContext{
		Rules:          []state.Rule{inline, srs},
		SrsCachedPaths: map[string][]string{state.StableRuleID(srs): {"/tmp/list.srs"}},
	}
	out, err := MergePresetsIntoRoute(json.RawMessage(`{"rules":[]}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Rules []map[string]interface{} }
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Rules) != 2 || r.Rules[0]["outbound"] != "direct-out" || r.Rules[1]["rule_set"] != "user:second-srs" {
		t.Errorf("route.rules = %v; want inline (1000), затем srs (1010)", r.Rules)
	}
}
