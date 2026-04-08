package template

import (
	"encoding/json"
	"testing"
)

func TestApplyTemplateWithVars_emptyParamsSubstitutesConfig(t *testing.T) {
	rawCfg := json.RawMessage(`{"log":{"level":"@log_level"},"x":1}`)
	vars := []TemplateVar{{Name: "log_level", Type: "text", DefaultValue: VarDefaultValue{Scalar: "debug"}}}
	out, err := ApplyTemplateWithVars(rawCfg, nil, "linux", vars, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	log := m["log"].(map[string]interface{})
	if log["level"] != "debug" {
		t.Fatalf("log.level = %v", log["level"])
	}
}

func TestApplyTemplateWithVars_ifOr_prependsRouteRules(t *testing.T) {
	rawCfg := json.RawMessage(`{"route":{"rules":[{"outbound":"direct-out"}]}}`)
	vars := []TemplateVar{
		{Name: "tun_builtin", Type: "bool", DefaultValue: VarDefaultValue{Scalar: "true"}, Platforms: []string{"windows"}},
		{Name: "tun", Type: "bool", DefaultValue: VarDefaultValue{Scalar: "true"}, Platforms: []string{"darwin"}},
	}
	params := []TemplateParam{
		{
			Name:      "route.rules",
			Platforms: []string{"windows", "linux", "darwin"},
			IfOr:      []string{"tun_builtin", "tun"},
			Mode:      "prepend",
			Value: json.RawMessage(`[
				{"inbound":"tun-in","action":"resolve","strategy":"prefer_ipv4"},
				{"inbound":"tun-in","action":"sniff","timeout":"1s"}
			]`),
		},
	}
	out, err := ApplyTemplateWithVars(rawCfg, params, "windows", vars, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	route := root["route"].(map[string]interface{})
	rules := route["rules"].([]interface{})
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	m0 := rules[0].(map[string]interface{})
	if m0["inbound"] != "tun-in" {
		t.Fatalf("first rule = %#v", rules[0])
	}
}
