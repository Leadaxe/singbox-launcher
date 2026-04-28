package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func unmarshalRoute(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	return m
}

// TestMergeRouteSection_TemplateRulesPreserved — шаблонные rules / rule_set
// сохраняются (НЕ replace), даже если custom-rules пустые.
func TestMergeRouteSection_TemplateRulesPreserved(t *testing.T) {
	tmpl := json.RawMessage(`{
		"rules": [{"protocol":"dns","action":"hijack-dns"}],
		"rule_set": [{"tag":"ru","type":"inline"}],
		"final": "proxy-out"
	}`)
	got, err := MergeRouteSection(tmpl, RouteConfig{})
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)

	rules, _ := m["rules"].([]interface{})
	if len(rules) != 1 {
		t.Errorf("template rules must remain: %v", rules)
	}
	rsets, _ := m["rule_set"].([]interface{})
	if len(rsets) != 1 {
		t.Errorf("template rule_set must remain: %v", rsets)
	}
	if got, _ := m["final"].(string); got != "proxy-out" {
		t.Errorf("template final must remain when cfg.FinalOutbound empty: %q", got)
	}
}

// TestMergeRouteSection_CustomRuleAppended — custom rule добавляется в массив,
// outbound материализуется на правиле.
func TestMergeRouteSection_CustomRuleAppended(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "vpn-1",
				PrimaryRule: map[string]interface{}{"domain_suffix": []string{"example.com"}},
			},
		},
	}
	got, err := MergeRouteSection(json.RawMessage(`{"rules":[]}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)
	rules, _ := m["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(rules))
	}
	r := rules[0].(map[string]interface{})
	if r["outbound"] != "vpn-1" {
		t.Errorf("outbound should be vpn-1, got: %v", r["outbound"])
	}
}

// TestMergeRouteSection_DisabledRuleSkipped — disabled-правила не идут в out.
func TestMergeRouteSection_DisabledRuleSkipped(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: false, Outbound: "vpn-1", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
	}
	got, err := MergeRouteSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)
	if _, has := m["rules"]; has {
		t.Errorf("disabled rule must not produce rules key: %v", m)
	}
}

// TestMergeRouteSection_RejectAction — outbound=reject → action=reject, без outbound/method.
func TestMergeRouteSection_RejectAction(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "reject",
				PrimaryRule: map[string]interface{}{"domain": "ads.example"},
			},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	r := m["rules"].([]interface{})[0].(map[string]interface{})
	if r["action"] != "reject" {
		t.Errorf("action should be reject, got: %v", r["action"])
	}
	if _, has := r["outbound"]; has {
		t.Errorf("outbound must be removed for reject: %v", r)
	}
	if _, has := r["method"]; has {
		t.Errorf("method must NOT be set for plain reject: %v", r)
	}
}

// TestMergeRouteSection_DropAction — outbound=drop → action=reject, method=drop.
func TestMergeRouteSection_DropAction(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "drop", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	r := m["rules"].([]interface{})[0].(map[string]interface{})
	if r["action"] != "reject" || r["method"] != "drop" {
		t.Errorf("drop semantics: want action=reject method=drop, got: %v", r)
	}
}

// TestMergeRouteSection_MultiRules — RouteRule.Rules (несколько подправил) → каждое добавляется.
func TestMergeRouteSection_MultiRules(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:  true,
				Outbound: "vpn-1",
				Rules: []map[string]interface{}{
					{"domain": "a"},
					{"domain": "b"},
				},
			},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	rules := m["rules"].([]interface{})
	if len(rules) != 2 {
		t.Errorf("expected 2 rules from RouteRule.Rules, got %d", len(rules))
	}
	for _, r := range rules {
		ro := r.(map[string]interface{})
		if ro["outbound"] != "vpn-1" {
			t.Errorf("each subrule must have outbound=vpn-1: %v", ro)
		}
	}
}

// TestMergeRouteSection_FinalOverride — cfg.FinalOutbound перезаписывает шаблон.
func TestMergeRouteSection_FinalOverride(t *testing.T) {
	tmpl := json.RawMessage(`{"final":"old"}`)
	cfg := RouteConfig{FinalOutbound: "new"}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if got, _ := m["final"].(string); got != "new" {
		t.Errorf("FinalOutbound must override: got %q", got)
	}
}

// TestMergeRouteSection_OmitDefaultDomainResolver — ключ удаляется при флаге.
func TestMergeRouteSection_OmitDefaultDomainResolver(t *testing.T) {
	tmpl := json.RawMessage(`{"default_domain_resolver":"local"}`)
	cfg := RouteConfig{OmitDefaultDomainResolver: true}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if _, has := m["default_domain_resolver"]; has {
		t.Errorf("default_domain_resolver must be removed when Omit=true: %v", m)
	}
}

// TestMergeRouteSection_DefaultDomainResolverOverride — переопределение значения.
func TestMergeRouteSection_DefaultDomainResolverOverride(t *testing.T) {
	tmpl := json.RawMessage(`{"default_domain_resolver":"old"}`)
	cfg := RouteConfig{DefaultDomainResolver: "new"}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if got, _ := m["default_domain_resolver"].(string); got != "new" {
		t.Errorf("DefaultDomainResolver must override: got %q", got)
	}
}

// TestMergeRouteSection_RuleSetTemplatePreserved — шаблонные rule_sets идут вместе с custom.
func TestMergeRouteSection_RuleSetTemplatePreserved(t *testing.T) {
	tmpl := json.RawMessage(`{
		"rule_set": [{"tag":"tmpl","type":"inline"}]
	}`)
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:  true,
				Outbound: "vpn",
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"tag":"custom","type":"inline"}`),
				},
				PrimaryRule: map[string]interface{}{"x": "1"},
			},
		},
	}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	rsets := m["rule_set"].([]interface{})
	if len(rsets) != 2 {
		t.Errorf("expected template + custom rule_set: got %d", len(rsets))
	}
}

// TestMergeRouteSection_OriginalRuleNotMutated — clone, не mutation in-place.
// Регрессия — сейчас тест ловит баг "если outbound=reject, то in-place правило шаблона
// тоже становится reject" если бы copy не было.
func TestMergeRouteSection_OriginalRuleNotMutated(t *testing.T) {
	original := map[string]interface{}{"domain_suffix": []string{"x"}}
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "reject", PrimaryRule: original},
		},
	}
	_, _ = MergeRouteSection(json.RawMessage(`{}`), cfg)
	if _, has := original["action"]; has {
		t.Errorf("original PrimaryRule must NOT be mutated: %v", original)
	}
}

// TestApplyRouteOutbound_Direct — внутренний helper: явно проверим mapping.
func TestApplyRouteOutbound_Direct(t *testing.T) {
	cases := []struct {
		name     string
		in       map[string]interface{}
		outbound string
		check    func(t *testing.T, m map[string]interface{})
	}{
		{
			name:     "named outbound clears action",
			in:       map[string]interface{}{"action": "reject", "method": "drop"},
			outbound: "vpn-1",
			check: func(t *testing.T, m map[string]interface{}) {
				if m["outbound"] != "vpn-1" {
					t.Errorf("outbound: got %v", m["outbound"])
				}
				if _, has := m["action"]; has {
					t.Errorf("action must be cleared: %v", m)
				}
				if _, has := m["method"]; has {
					t.Errorf("method must be cleared: %v", m)
				}
			},
		},
		{
			name:     "empty outbound is no-op",
			in:       map[string]interface{}{"action": "reject"},
			outbound: "",
			check: func(t *testing.T, m map[string]interface{}) {
				if m["action"] != "reject" {
					t.Errorf("empty outbound must NOT touch existing fields: %v", m)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := shallowCopyStringMap(c.in)
			applyRouteOutbound(m, c.outbound)
			c.check(t, m)
		})
	}
}

// TestConvertRuleSetToLocalIfNeeded_NoExecDir — без execDir остаётся как есть.
func TestConvertRuleSetToLocalIfNeeded_NoExecDir(t *testing.T) {
	in := json.RawMessage(`{"tag":"x","type":"remote","url":"http://..."}`)
	got := convertRuleSetToLocalIfNeeded(in, "")
	m := got.(map[string]interface{})
	if m["type"] != "remote" {
		t.Errorf("no execDir → must remain remote: %v", m)
	}
}

// TestConvertRuleSetToLocalIfNeeded_NonRemoteUntouched — local rule-set не трогаем.
func TestConvertRuleSetToLocalIfNeeded_NonRemoteUntouched(t *testing.T) {
	in := json.RawMessage(`{"tag":"x","type":"inline","rules":[]}`)
	got := convertRuleSetToLocalIfNeeded(in, "/tmp")
	m := got.(map[string]interface{})
	if m["type"] != "inline" {
		t.Errorf("non-remote must be untouched: %v", m)
	}
}

// TestConvertRuleSetToLocalIfNeeded_BadJSONReturnsNil.
func TestConvertRuleSetToLocalIfNeeded_BadJSONReturnsNil(t *testing.T) {
	got := convertRuleSetToLocalIfNeeded(json.RawMessage("not json"), "/tmp")
	if got != nil {
		t.Errorf("bad json → expected nil, got %v", got)
	}
}

// TestMergeRouteSection_FormattedOutputValid — для регресс-проверки: вывод
// — валидный JSON-объект с ожидаемой структурой.
func TestMergeRouteSection_FormattedOutputValid(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "vpn-1", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
		FinalOutbound: "vpn-1",
	}
	got, err := MergeRouteSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(got)), "{") {
		t.Errorf("output not a JSON object: %s", got)
	}
	if !json.Valid(got) {
		t.Errorf("output not valid JSON: %s", got)
	}
}
